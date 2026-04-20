package runner

import (
	"context"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	"github.com/ZlatanOmerovic/onboardctl/internal/provider"
	"github.com/ZlatanOmerovic/onboardctl/internal/state"
)

// Plan is the read-only result of walking a selection against the current
// system: for each resolved item, which provider we'd pick and whether
// it's already installed. Plans never mutate state; the TUI uses them to
// render the review screen before the user commits to an apply.
type Plan struct {
	Profile string      // informational; whatever was requested
	Entries []PlanEntry // in resolved order
}

// PlanEntry is one item's pre-flight summary.
type PlanEntry struct {
	ItemID         string
	Item           manifest.Item
	Provider       manifest.Provider // only meaningful when Skipped == false && NoProvider == false
	ProviderKind   string            // convenience: Provider.Type
	State          provider.State    // Check() result (zero value if Skipped/NoProvider)
	CheckErr       string            // populated if Check errored (kept as string so Plan is YAML-safe)
	Skipped        bool              // item-level When gate excluded this
	NoProvider     bool              // no registered provider for any of the item's kinds
	TrackedByUs    bool              // state.yaml records this item as installed by onboardctl
	TrackedByUsVer string            // version recorded in state.yaml (empty if not tracked)
	Drift          bool              // installed, but via a provider kind different from manifest preferred
}

// Plan walks a selection and produces a PlanEntry per resolved item.
// It runs Check() on each item's chosen provider but does not touch state
// or trigger Install. Safe to call at any time.
//
// The runner's State (if set) is consulted to mark TrackedByUs — this is
// what distinguishes "we installed it" from "someone else installed it"
// in the TUI's status markers.
func (r *Runner) Plan(ctx context.Context, sel Selection) (*Plan, error) {
	if r.Manifest == nil {
		return nil, errNilManifest
	}
	if r.Registry == nil {
		return nil, errNilRegistry
	}

	ids, err := Resolve(r.Manifest, sel)
	if err != nil {
		return nil, err
	}

	p := &Plan{Profile: sel.Profile, Entries: make([]PlanEntry, 0, len(ids))}
	for _, id := range ids {
		it := r.Manifest.Items[id]
		entry := PlanEntry{ItemID: id, Item: it}

		// Annotate state-tracking up-front so even skipped/no-provider rows
		// show whether we've touched them before.
		if r.State != nil {
			if rec, ok := r.State.Items[id]; ok && rec.Status == state.StatusInstalled {
				entry.TrackedByUs = true
				entry.TrackedByUsVer = rec.Version
			}
		}

		if !Match(it.When, r.Env) {
			entry.Skipped = true
			p.Entries = append(p.Entries, entry)
			continue
		}

		prov, impl := r.choose(it)
		if prov == nil {
			entry.NoProvider = true
			p.Entries = append(p.Entries, entry)
			continue
		}

		entry.Provider = *prov
		entry.ProviderKind = prov.Type

		st, cerr := impl.Check(ctx, it, *prov)
		entry.State = st
		if cerr != nil {
			entry.CheckErr = cerr.Error()
		}
		// Drift: installed, but via a different provider kind than the
		// manifest prefers. Surfaces as a ⚠ in the review screen.
		if st.Installed && st.ProviderUsed != "" && st.ProviderUsed != prov.Type {
			entry.Drift = true
		}
		p.Entries = append(p.Entries, entry)
	}
	return p, nil
}

// Counts returns a small summary of the plan for headline rendering.
func (p *Plan) Counts() PlanCounts {
	var c PlanCounts
	for _, e := range p.Entries {
		c.Total++
		switch {
		case e.Skipped:
			c.Skipped++
		case e.NoProvider:
			c.NoProvider++
		case e.Drift:
			c.Drift++
		case e.State.Installed && e.TrackedByUs:
			c.InstalledByUs++
		case e.State.Installed:
			c.InstalledExternal++
		default:
			c.NotInstalled++
		}
	}
	return c
}

// PlanCounts is a compact counter ready for headline rendering.
type PlanCounts struct {
	Total             int
	InstalledByUs     int
	InstalledExternal int
	NotInstalled      int
	Skipped           int
	NoProvider        int
	Drift             int
}

// Sentinel errors kept here so both Run and Plan share them.
var (
	errNilManifest = newRunnerError("runner: nil manifest")
	errNilRegistry = newRunnerError("runner: nil registry")
)

type runnerError struct{ s string }

func newRunnerError(s string) *runnerError { return &runnerError{s: s} }
func (e *runnerError) Error() string       { return e.s }
