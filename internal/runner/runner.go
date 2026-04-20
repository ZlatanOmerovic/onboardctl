package runner

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	"github.com/ZlatanOmerovic/onboardctl/internal/provider"
	"github.com/ZlatanOmerovic/onboardctl/internal/state"
)

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
}

// Options controls a single Run invocation.
type Options struct {
	DryRun  bool
	Profile string // recorded in state; informational
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
	type chosen struct {
		provider manifest.Provider
		impl     provider.Provider
	}
	chosenFor := make(map[string]*chosen, len(ids))
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
		chosenFor[id] = &chosen{provider: *p, impl: impl}
		if p.Repo != "" && !seenRepo[p.Repo] {
			seenRepo[p.Repo] = true
			repoNames = append(repoNames, p.Repo)
		}
	}

	// Phase 2: bootstrap referenced repos (if we have a bootstrapper).
	// On dry-run, announce but don't materialise.
	if r.Bootstrapper != nil && len(repoNames) > 0 {
		r.logf("\nRepos needed: %v\n", repoNames)
		if !opts.DryRun {
			for _, name := range repoNames {
				if _, err := r.Bootstrapper.Ensure(ctx, name, r.Env); err != nil {
					return sum, fmt.Errorf("bootstrap repo %s: %w", name, err)
				}
			}
			if err := r.Bootstrapper.AptUpdateIfNeeded(ctx); err != nil {
				return sum, err
			}
		}
	}

	// Phase 3: per-item Check / Install.
	runEntry := state.Run{
		StartedAt: time.Now().UTC(),
		Profile:   opts.Profile,
		Selection: append([]string(nil), ids...),
		DryRun:    opts.DryRun,
	}

	for _, id := range ids {
		c := chosenFor[id]
		if c == nil {
			continue // either skipped or failed during phase 1
		}
		if err := r.handleItem(ctx, id, r.Manifest.Items[id], c.provider, c.impl, opts, sum); err != nil {
			sum.Failed[id] = err.Error()
		}
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
	opts Options, sum *Summary) error {

	st, err := impl.Check(ctx, it, p)
	if err != nil {
		return fmt.Errorf("check: %w", err)
	}
	if st.Installed {
		sum.AlreadyHad = append(sum.AlreadyHad, id)
		r.logf("  = %-20s already installed (%s)\n", id, firstNonEmpty(st.Version, "present"))
		return nil
	}

	if opts.DryRun {
		r.logf("  + %-20s would install via %s (%s)\n", id, p.Type, describeProvider(p))
		sum.Installed = append(sum.Installed, id)
		return nil
	}

	r.logf("  + %-20s installing via %s...\n", id, p.Type)
	if err := impl.Install(ctx, it, p); err != nil {
		return err
	}
	st, _ = impl.Check(ctx, it, p)
	r.State.RecordInstall(id, p.Type, st.Version, state.ByOnboardctl, time.Now().UTC())
	sum.Installed = append(sum.Installed, id)
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
