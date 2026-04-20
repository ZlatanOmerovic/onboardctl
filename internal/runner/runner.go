package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	"github.com/ZlatanOmerovic/onboardctl/internal/provider"
	"github.com/ZlatanOmerovic/onboardctl/internal/state"
)

// Runner orchestrates the install flow: it resolves selections, consults
// the provider registry for Check/Install, and persists results to state.
type Runner struct {
	Manifest *manifest.Manifest
	Registry provider.Registry
	State    *state.State
	StateFn  func(*state.State) error // write-back hook; defaults to state.Save
	Out      io.Writer                // human-readable log; nil → discards
}

// Options controls a single Run invocation.
type Options struct {
	DryRun bool
	// Profile captures the high-level selection name for logging / state.
	Profile string
}

// Summary is what Run returns so the CLI can print a final report.
type Summary struct {
	Selected    []string          // ordered list of item IDs in the plan
	Installed   []string          // items installed or reinstalled this run
	AlreadyHad  []string          // items Check() reported already installed
	Failed      map[string]string // item -> error message
	DryRun      bool
}

// NewSummary makes a zeroed summary.
func NewSummary() *Summary {
	return &Summary{Failed: map[string]string{}}
}

// Run executes the install pipeline for a selection. Individual item
// failures are recorded in Summary.Failed; the pipeline does not abort
// unless a framework-level error (manifest load, resolver, etc.) occurs.
func (r *Runner) Run(ctx context.Context, sel Selection, opts Options) (*Summary, error) {
	if r.Manifest == nil {
		return nil, errors.New("runner: nil manifest")
	}
	if r.Registry == nil {
		return nil, errors.New("runner: nil registry")
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

	runEntry := state.Run{
		StartedAt: time.Now().UTC(),
		Profile:   opts.Profile,
		Selection: append([]string(nil), ids...),
		DryRun:    opts.DryRun,
	}

	for _, id := range ids {
		item := r.Manifest.Items[id]
		if err := r.handleItem(ctx, id, item, opts, sum); err != nil {
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

// handleItem walks the providers of one item, picks the first whose
// kind is registered (Phase 2 skips When evaluation; that arrives in
// Phase 3 with full TUI context), checks and either skips or installs.
func (r *Runner) handleItem(ctx context.Context, id string, it manifest.Item,
	opts Options, sum *Summary) error {

	if len(it.Providers) == 0 {
		return fmt.Errorf("item %q has no providers", id)
	}

	var chosen *manifest.Provider
	var impl provider.Provider
	for i := range it.Providers {
		p := &it.Providers[i]
		if got := r.Registry.Lookup(p.Type); got != nil {
			chosen = p
			impl = got
			break
		}
	}
	if chosen == nil {
		return fmt.Errorf("item %q: no registered provider for kinds %s", id, kindList(it.Providers))
	}

	st, err := impl.Check(ctx, it, *chosen)
	if err != nil {
		return fmt.Errorf("check: %w", err)
	}
	if st.Installed {
		sum.AlreadyHad = append(sum.AlreadyHad, id)
		r.logf("  = %-20s already installed (%s)\n", id, firstNonEmpty(st.Version, "present"))
		return nil
	}

	if opts.DryRun {
		r.logf("  + %-20s would install via %s (%s)\n", id, chosen.Type, describeProvider(*chosen))
		sum.Installed = append(sum.Installed, id)
		return nil
	}

	r.logf("  + %-20s installing via %s...\n", id, chosen.Type)
	if err := impl.Install(ctx, it, *chosen); err != nil {
		return err
	}
	// Re-check so state.yaml records the real post-install version.
	st, _ = impl.Check(ctx, it, *chosen)
	r.State.RecordInstall(id, chosen.Type, st.Version, state.ByOnboardctl, time.Now().UTC())
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
