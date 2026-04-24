package runner

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	"github.com/ZlatanOmerovic/onboardctl/internal/provider"
	"github.com/ZlatanOmerovic/onboardctl/internal/state"
)

// ParallelPoolSize is the upper bound on concurrently-running Install
// invocations for providers that don't hold a shared system lock.
const ParallelPoolSize = 4

// Runner orchestrates the install flow: it resolves selections, evaluates
// When gates, bootstraps any apt repos referenced by chosen providers,
// consults the provider registry for Check/Install, and persists results
// to state.
type Runner struct {
	Manifest     *manifest.Manifest
	Registry     provider.Registry
	State        *state.State
	StateFn      func(*state.State) error // write-back; defaults to noop if nil
	Bootstrapper *RepoBootstrapper        // optional; if nil, repo-bootstrap is skipped
	Env          Env                      // distro/desktop for When evaluation
	Out          io.Writer                // human-readable log; nil → discards

	// Values supplies user-collected input for items whose manifest has an
	// Input block. Keyed by item ID; the inner map is field-name → value.
	// When the runner finds values for an item, it substitutes {field}
	// tokens in the provider's Apply/Check commands before dispatch and
	// clears Item.Input so the provider doesn't re-reject on the interactive
	// gate. Items with Input but no Values here fall through to the
	// provider's own guard (typically Config.Install refusing).
	Values map[string]map[string]string

	// ProgressFn, if set, receives a ProgressEvent for each step of the
	// pipeline (item start/done, skip, repo bootstrap). Nil means the
	// runner only logs to Out. Used by the TUI to render a live progress
	// view during apply-mode installs.
	ProgressFn ProgressFn

	// Cmd runs arbitrary shell commands for runner-level hooks
	// (currently: post_install). If nil, provider.ExecRunner() is used.
	// Tests can inject a fake to capture or stub invocations.
	Cmd provider.Runner

	// mu serialises writes to r.State, runEntry.Installed, and sum.*
	// during the parallel-install phase. Unused in the serial pass.
	mu sync.Mutex
}

// chosenItem pairs the manifest-level provider spec with the registered
// implementation the runner dispatches to.
type chosenItem struct {
	provider manifest.Provider
	impl     provider.Provider
}

// todo is one item the phase-3 dispatcher will process, carrying the
// 1-based display index used in ProgressEvent.Index.
type todo struct {
	id    string
	index int
}

// isSerialKind reports whether a provider's Install must run serially.
// apt holds /var/lib/dpkg/lock-frontend; shell and config may have
// user-visible ordering expectations. Everything else (flatpak, npm_global,
// composer_global, binary_release) can run concurrently.
func isSerialKind(kind string) bool {
	switch kind {
	case manifest.KindAPT, manifest.KindShell, manifest.KindConfig:
		return true
	}
	return false
}

// kindTouchesNetwork reports whether a provider's Install requires the
// network. Used by --offline to reject items whose install would fetch
// from remote mirrors, package registries, or GitHub. config + shell
// items are assumed safe — they run arbitrary user-supplied commands,
// which the operator is responsible for.
func kindTouchesNetwork(kind string) bool {
	switch kind {
	case manifest.KindAPT, manifest.KindFlatpak,
		manifest.KindBinaryRelease, manifest.KindNPMGlobal,
		manifest.KindComposerGlobal:
		return true
	}
	return false
}

// Options controls a single Run invocation.
type Options struct {
	DryRun  bool
	Profile string // recorded in state; informational

	// SwapDrift, when true, instructs the runner to actively replace
	// items it detects as drift (Installed=true via a provider kind
	// other than the manifest's preferred one). Today only the apt←snap
	// swap is implemented; manifest preferring apt + system-snap is
	// resolved by `snap remove <pkg>` followed by the normal apt install.
	SwapDrift bool

	// RollbackOnFailure, when true, undoes the installs this run
	// completed if any subsequent item fails. Items that were already
	// installed before the run are untouched. Items whose provider
	// does not implement Uninstaller are skipped with a log message.
	RollbackOnFailure bool

	// Offline refuses items whose provider would touch the network
	// (apt, flatpak, binary_release, npm_global, composer_global).
	// config + shell items still run. Repo bootstrap is skipped.
	// Intended for re-runs on air-gapped hosts where apt is expected
	// to install from its local cache only.
	Offline bool
}

// Summary is what Run returns so the CLI can print a final report.
type Summary struct {
	Selected   []string          // ordered list of item IDs in the plan
	Installed  []string          // items installed (or "would install" in dry-run)
	AlreadyHad []string          // items Check() reported already installed
	Failed     map[string]string // item -> error message
	Skipped    []string          // items skipped because When didn't match
	DryRun     bool
}

// NewSummary makes a zeroed summary.
func NewSummary() *Summary {
	return &Summary{Failed: map[string]string{}}
}

// Run executes the install pipeline for a selection. Individual item
// failures are recorded in Summary.Failed; the pipeline does not abort
// unless a framework-level error (manifest, resolver, bootstrap) occurs.
func (r *Runner) Run(ctx context.Context, sel Selection, opts Options) (*Summary, error) {
	if r.Manifest == nil {
		return nil, errNilManifest
	}
	if r.Registry == nil {
		return nil, errNilRegistry
	}
	if r.State == nil {
		r.State = state.New()
	}

	ids, err := Resolve(r.Manifest, sel)
	if err != nil {
		return nil, fmt.Errorf("resolve: %w", err)
	}

	sum := NewSummary()
	sum.Selected = ids
	sum.DryRun = opts.DryRun

	// Phase 1: for each item, determine the chosen provider (first whose
	// When matches AND kind is registered). Collect repos to bootstrap.
	chosenFor := make(map[string]*chosenItem, len(ids))
	var repoNames []string
	seenRepo := make(map[string]bool)

	for _, id := range ids {
		it := r.Manifest.Items[id]
		if !Match(it.When, r.Env) {
			sum.Skipped = append(sum.Skipped, id)
			r.logf("  - %-20s skipped (when doesn't match)\n", id)
			continue
		}
		p, impl := r.choose(it)
		if p == nil {
			sum.Failed[id] = fmt.Sprintf("item %q: no registered provider for kinds %s",
				id, kindList(it.Providers))
			continue
		}
		chosenFor[id] = &chosenItem{provider: *p, impl: impl}
		if p.Repo != "" && !seenRepo[p.Repo] {
			seenRepo[p.Repo] = true
			repoNames = append(repoNames, p.Repo)
		}
	}

	// Phase 2: bootstrap referenced repos (if we have a bootstrapper).
	// On dry-run, announce but don't materialise. Offline mode skips
	// repo bootstrap entirely — by definition we can't fetch new
	// signing keys or run apt-get update.
	if r.Bootstrapper != nil && len(repoNames) > 0 && !opts.Offline {
		r.logf("\nRepos needed: %v\n", repoNames)
		if !opts.DryRun {
			for _, name := range repoNames {
				r.emit(ProgressEvent{Kind: ProgressBootstrapStart, Detail: name, Total: len(ids)})
				if _, err := r.Bootstrapper.Ensure(ctx, name, r.Env); err != nil {
					return sum, fmt.Errorf("bootstrap repo %s: %w", name, err)
				}
			}
			if err := r.Bootstrapper.AptUpdateIfNeeded(ctx); err != nil {
				return sum, err
			}
			r.emit(ProgressEvent{Kind: ProgressBootstrapDone, Total: len(ids)})
		}
	} else if opts.Offline && len(repoNames) > 0 {
		r.logf("\nOffline mode: skipping repo bootstrap for %v\n", repoNames)
	}

	// Phase 3: per-item Check / Install.
	//
	// Items are partitioned into serial (apt/shell/config — shared system
	// locks or user-visible ordering) and parallel (flatpak/npm/composer/
	// binary_release — independent state). Serial runs first so parallel
	// items that depend on apt-installed tooling (npm CLIs on nodejs, etc.)
	// see it ready. Rollback-on-failure and dry-run both stay serial.
	runEntry := state.Run{
		StartedAt: time.Now().UTC(),
		Profile:   opts.Profile,
		Selection: append([]string(nil), ids...),
		DryRun:    opts.DryRun,
	}

	// Emit skipped / no-provider events up front so the UI gets a row per
	// item regardless of dispatch phase. These items don't do real work.
	var serial, parallel []todo
	for idx, id := range ids {
		if contains(sum.Skipped, id) {
			it := r.Manifest.Items[id]
			r.emit(ProgressEvent{
				Kind: ProgressSkippedWhen, ItemID: id, Name: it.Name,
				Total: len(ids), Index: idx + 1,
			})
			continue
		}
		c := chosenFor[id]
		if c == nil {
			it := r.Manifest.Items[id]
			r.emit(ProgressEvent{
				Kind: ProgressNoProvider, ItemID: id, Name: it.Name,
				Total: len(ids), Index: idx + 1,
			})
			continue
		}
		if opts.RollbackOnFailure || opts.DryRun || isSerialKind(c.provider.Type) {
			serial = append(serial, todo{id, idx + 1})
		} else {
			parallel = append(parallel, todo{id, idx + 1})
		}
	}

	// Serial pass.
	bailOut := false
	for _, t := range serial {
		c := chosenFor[t.id]
		it, prov := r.prepareItem(t.id, r.Manifest.Items[t.id], c.provider)
		if err := r.handleItem(ctx, t.id, it, prov, c.impl, opts, sum, &runEntry, t.index, len(ids)); err != nil {
			r.recordFailure(sum, t.id, it.Name, err, t.index, len(ids))
			if opts.RollbackOnFailure && !opts.DryRun && len(runEntry.Installed) > 0 {
				r.logf("\n! rolling back %d installed item(s) due to failure...\n", len(runEntry.Installed))
				r.rollbackRun(ctx, &runEntry)
				bailOut = true
				break
			}
		}
	}

	// Parallel pass. Skipped entirely if the serial pass triggered rollback.
	if !bailOut && len(parallel) > 0 {
		r.runParallel(ctx, parallel, chosenFor, opts, sum, &runEntry, len(ids))
	}

	runEntry.CompletedAt = time.Now().UTC()
	if !opts.DryRun {
		r.State.AppendRun(runEntry)
		if r.StateFn != nil {
			if err := r.StateFn(r.State); err != nil {
				return sum, fmt.Errorf("state save: %w", err)
			}
		}
	}
	return sum, nil
}

// handleDriftSwap removes a foreign-installed version of an item so the
// runner's next call to impl.Install installs the manifest-preferred one.
//
// Today only the apt←snap case is wired. Manifest.KindAPT + ProviderUsed=snap
// triggers `snap remove <pkg>`; other pairs are no-ops (returns handled=false)
// so the caller can fall back to the already-installed path.
func (r *Runner) handleDriftSwap(ctx context.Context, id string, it manifest.Item,
	p manifest.Provider, impl provider.Provider, st provider.State,
	_ Options) (handled bool, err error) {

	if p.Type == manifest.KindAPT && st.ProviderUsed == "snap" {
		apt, ok := impl.(*provider.APT)
		if !ok {
			return false, nil // shouldn't happen, but play safe
		}
		r.logf("  ~ %-20s drift swap: removing snap before apt install\n", id)
		if err := apt.RemoveSnapCounterpart(ctx, p.Package); err != nil {
			return false, fmt.Errorf("swap drift for %q: %w", it.Name, err)
		}
		return true, nil
	}
	return false, nil
}

// prepareItem returns copies of the item and provider ready for dispatch.
// If Values supplies input for this item, {field} tokens are substituted
// in provider.Apply/Check and the copy's Input is cleared so the provider
// treats it as headless.
func (r *Runner) prepareItem(id string, it manifest.Item, p manifest.Provider) (manifest.Item, manifest.Provider) {
	if it.Input == nil {
		return it, p
	}
	vals, ok := r.Values[id]
	if !ok || len(vals) == 0 {
		return it, p
	}
	itemCopy := it
	itemCopy.Input = nil
	provCopy := p
	provCopy.Apply = substituteAll(p.Apply, vals)
	provCopy.Check = substitute(p.Check, vals)
	return itemCopy, provCopy
}

// choose walks an item's providers and returns the first (Provider, impl)
// whose When matches the environment AND whose kind is in the registry.
func (r *Runner) choose(it manifest.Item) (*manifest.Provider, provider.Provider) {
	for i := range it.Providers {
		p := &it.Providers[i]
		if !Match(p.When, r.Env) {
			continue
		}
		if impl := r.Registry.Lookup(p.Type); impl != nil {
			return p, impl
		}
	}
	return nil, nil
}

func (r *Runner) handleItem(ctx context.Context, id string, it manifest.Item,
	p manifest.Provider, impl provider.Provider,
	opts Options, sum *Summary, runEntry *state.Run, index, total int) error {

	r.emit(ProgressEvent{
		Kind: ProgressStart, ItemID: id, Name: it.Name,
		Total: total, Index: index,
	})

	// Offline mode: refuse items whose provider would need the network.
	// Surfaces as a per-item failure with an explanatory reason so the
	// summary lists what couldn't install.
	if opts.Offline && kindTouchesNetwork(p.Type) {
		return fmt.Errorf("offline mode: provider %q needs network", p.Type)
	}

	st, err := impl.Check(ctx, it, p)
	if err != nil {
		return fmt.Errorf("check: %w", err)
	}
	if st.Installed {
		// Drift swap: installed, but via a different provider kind than the
		// manifest prefers. When opts.SwapDrift is set AND we know how to
		// swap this pair, remove the foreign version and fall through to
		// the normal install below. Otherwise, treat as already-installed.
		if opts.SwapDrift && st.ProviderUsed != "" && st.ProviderUsed != p.Type {
			handled, swapErr := r.handleDriftSwap(ctx, id, it, p, impl, st, opts)
			if swapErr != nil {
				return swapErr
			}
			if !handled {
				r.mu.Lock()
				sum.AlreadyHad = append(sum.AlreadyHad, id)
				r.mu.Unlock()
				r.logf("  = %-20s already installed (%s) — drift not swappable\n",
					id, firstNonEmpty(st.Version, "present"))
				r.emit(ProgressEvent{
					Kind: ProgressAlready, ItemID: id, Name: it.Name,
					Version: firstNonEmpty(st.Version, "present"),
					Total:   total, Index: index,
				})
				return nil
			}
			// Fall through to install.
		} else {
			r.mu.Lock()
			sum.AlreadyHad = append(sum.AlreadyHad, id)
			r.mu.Unlock()
			r.logf("  = %-20s already installed (%s)\n", id, firstNonEmpty(st.Version, "present"))
			r.emit(ProgressEvent{
				Kind: ProgressAlready, ItemID: id, Name: it.Name,
				Version: firstNonEmpty(st.Version, "present"),
				Total:   total, Index: index,
			})
			return nil
		}
	}

	if opts.DryRun {
		r.logf("  + %-20s would install via %s (%s)\n", id, p.Type, describeProvider(p))
		r.mu.Lock()
		sum.Installed = append(sum.Installed, id)
		r.mu.Unlock()
		r.emit(ProgressEvent{
			Kind: ProgressWould, ItemID: id, Name: it.Name,
			Detail: p.Type, Total: total, Index: index,
		})
		return nil
	}

	r.logf("  + %-20s installing via %s...\n", id, p.Type)
	if err := impl.Install(ctx, it, p); err != nil {
		return err
	}
	if err := r.runPostInstall(ctx, id, it); err != nil {
		return err
	}
	st, _ = impl.Check(ctx, it, p)
	r.mu.Lock()
	r.State.RecordInstall(id, p.Type, st.Version, state.ByOnboardctl, time.Now().UTC())
	runEntry.Installed = append(runEntry.Installed, installedRecord(id, p))
	sum.Installed = append(sum.Installed, id)
	r.mu.Unlock()
	r.emit(ProgressEvent{
		Kind: ProgressInstalled, ItemID: id, Name: it.Name,
		Version: firstNonEmpty(st.Version, "installed"),
		Detail:  p.Type, Total: total, Index: index,
	})
	return nil
}

// recordFailure writes sum.Failed under the shared mutex and emits a
// ProgressFailed event.
func (r *Runner) recordFailure(sum *Summary, id, name string, err error, index, total int) {
	r.mu.Lock()
	sum.Failed[id] = err.Error()
	r.mu.Unlock()
	r.emit(ProgressEvent{
		Kind: ProgressFailed, ItemID: id, Name: name,
		ErrMsg: err.Error(), Total: total, Index: index,
	})
}

// runParallel dispatches the parallel todo list through a bounded pool.
// The pool size is capped at ParallelPoolSize; if there are fewer items,
// we spawn one goroutine per item.
func (r *Runner) runParallel(ctx context.Context, todos []todo,
	chosenFor map[string]*chosenItem, opts Options,
	sum *Summary, runEntry *state.Run, total int) {

	workers := ParallelPoolSize
	if len(todos) < workers {
		workers = len(todos)
	}
	ch := make(chan todo, len(todos))
	for _, t := range todos {
		ch <- t
	}
	close(ch)

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for t := range ch {
				c := chosenFor[t.id]
				it, prov := r.prepareItem(t.id, r.Manifest.Items[t.id], c.provider)
				if err := r.handleItem(ctx, t.id, it, prov, c.impl, opts, sum, runEntry, t.index, total); err != nil {
					r.recordFailure(sum, t.id, it.Name, err, t.index, total)
				}
			}
		}()
	}
	wg.Wait()
}

// installedRecord builds a minimal state.Installed descriptor from the
// chosen provider, keeping only the fields needed to reconstruct the
// manifest.Provider for rollback dispatch.
func installedRecord(id string, p manifest.Provider) state.Installed {
	scope := ""
	if p.Extra != nil {
		scope = p.Extra["scope"]
	}
	return state.Installed{
		ItemID:  id,
		Kind:    p.Type,
		Package: p.Package,
		ID:      p.ID,
		Binary:  p.Binary,
		Scope:   scope,
	}
}

// rollbackRun walks runEntry.Installed in LIFO order and invokes each
// provider's Uninstall. Items whose provider does not implement
// Uninstaller are skipped with a log line. Failures are logged but do
// not abort the rollback — we try every item and report.
func (r *Runner) rollbackRun(ctx context.Context, runEntry *state.Run) {
	for i := len(runEntry.Installed) - 1; i >= 0; i-- {
		inst := runEntry.Installed[i]
		impl := r.Registry.Lookup(inst.Kind)
		if impl == nil {
			r.logf("  - %-20s rollback skipped (no provider for kind %q)\n", inst.ItemID, inst.Kind)
			continue
		}
		un, ok := impl.(provider.Uninstaller)
		if !ok {
			r.logf("  - %-20s rollback skipped (provider %q cannot uninstall)\n", inst.ItemID, inst.Kind)
			continue
		}
		prov := manifest.Provider{
			Type:    inst.Kind,
			Package: inst.Package,
			ID:      inst.ID,
			Binary:  inst.Binary,
		}
		if inst.Scope != "" {
			prov.Extra = map[string]string{"scope": inst.Scope}
		}
		item := r.Manifest.Items[inst.ItemID] // may be zero if manifest changed; Uninstall only uses item.Name
		if item.Name == "" {
			item.Name = inst.ItemID
		}
		r.logf("  ~ %-20s rollback: uninstall via %s\n", inst.ItemID, inst.Kind)
		if err := un.Uninstall(ctx, item, prov); err != nil {
			r.logf("  ! %-20s rollback failed: %v\n", inst.ItemID, err)
			continue
		}
		delete(r.State.Items, inst.ItemID)
	}
	runEntry.RolledBackAt = time.Now().UTC()
}

// RollbackLastRun rolls back the most recent non-dry-run entry in
// r.State.Runs that has not already been rolled back. Returns the
// number of items uninstalled and the Run's StartedAt for reporting.
// Callers (the CLI) are responsible for persisting state afterwards.
func (r *Runner) RollbackLastRun(ctx context.Context) (int, time.Time, error) {
	if r.State == nil || len(r.State.Runs) == 0 {
		return 0, time.Time{}, fmt.Errorf("no runs recorded in state")
	}
	for i := len(r.State.Runs) - 1; i >= 0; i-- {
		run := &r.State.Runs[i]
		if run.DryRun {
			continue
		}
		if !run.RolledBackAt.IsZero() {
			continue
		}
		if len(run.Installed) == 0 {
			return 0, run.StartedAt, fmt.Errorf("last real run at %s installed nothing to roll back", run.StartedAt.Format(time.RFC3339))
		}
		before := len(run.Installed)
		r.rollbackRun(ctx, run)
		return before, run.StartedAt, nil
	}
	return 0, time.Time{}, fmt.Errorf("no rollback-able run found in state")
}

// runPostInstall executes it.PostInstall commands sequentially via
// `bash -c`. A failure aborts the sequence and is returned to the
// caller, so the item ends up in sum.Failed. Commands run with the
// current user's privileges (same as the install itself).
func (r *Runner) runPostInstall(ctx context.Context, id string, it manifest.Item) error {
	if len(it.PostInstall) == 0 {
		return nil
	}
	cmd := r.Cmd
	if cmd == nil {
		cmd = provider.ExecRunner()
	}
	for i, c := range it.PostInstall {
		s := strings.TrimSpace(c)
		if s == "" {
			continue
		}
		r.logf("  · %-20s post_install[%d]: %s\n", id, i, s)
		if out, err := cmd.Run(ctx, "bash", "-c", s); err != nil {
			return fmt.Errorf("post_install[%d] for %q failed: %w\n  cmd: %s\n  output: %s",
				i, it.Name, err, s, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func (r *Runner) logf(format string, a ...any) {
	if r.Out == nil {
		return
	}
	fmt.Fprintf(r.Out, format, a...)
}

func kindList(ps []manifest.Provider) string {
	out := ""
	for i, p := range ps {
		if i > 0 {
			out += ", "
		}
		out += p.Type
	}
	return out
}

func describeProvider(p manifest.Provider) string {
	switch p.Type {
	case manifest.KindAPT:
		return p.Package
	case manifest.KindFlatpak:
		return p.ID
	case manifest.KindBinaryRelease:
		return p.Source
	case manifest.KindComposerGlobal, manifest.KindNPMGlobal:
		return p.Package
	default:
		return p.Type
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
