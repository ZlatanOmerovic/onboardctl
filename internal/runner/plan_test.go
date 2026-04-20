package runner

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	"github.com/ZlatanOmerovic/onboardctl/internal/provider"
	"github.com/ZlatanOmerovic/onboardctl/internal/state"
)

func TestPlanProducesEntryPerResolvedItem(t *testing.T) {
	m := mkTestManifest()
	stub := &stubProvider{kind: "apt", checkState: provider.State{Installed: false}}
	r := &Runner{
		Manifest: m,
		Registry: buildRegistry(stub),
		State:    state.New(),
		Out:      io.Discard,
	}

	plan, err := r.Plan(context.Background(), Selection{Bundle: "base"})
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}
	if len(plan.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(plan.Entries))
	}
	for _, e := range plan.Entries {
		if e.Skipped {
			t.Errorf("entry %s unexpectedly Skipped", e.ItemID)
		}
		if e.NoProvider {
			t.Errorf("entry %s unexpectedly NoProvider", e.ItemID)
		}
		if e.State.Installed {
			t.Errorf("entry %s unexpectedly Installed (stub said not installed)", e.ItemID)
		}
	}
}

func TestPlanMarksSkippedItems(t *testing.T) {
	m := mkTestManifest()
	// Give jq a When that excludes it on our env.
	it := m.Items["jq"]
	it.When = &manifest.When{DistroID: []string{"fedora"}}
	m.Items["jq"] = it

	stub := &stubProvider{kind: "apt"}
	r := &Runner{
		Manifest: m,
		Registry: buildRegistry(stub),
		State:    state.New(),
		Env:      Env{}, // empty env: distro.id == "", which doesn't match "fedora"
		Out:      io.Discard,
	}

	plan, err := r.Plan(context.Background(), Selection{Bundle: "base"})
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}
	skipped := 0
	for _, e := range plan.Entries {
		if e.Skipped {
			skipped++
			if e.ItemID != "jq" {
				t.Errorf("unexpected skipped item: %s", e.ItemID)
			}
		}
	}
	if skipped != 1 {
		t.Errorf("skipped count = %d, want 1", skipped)
	}
}

func TestPlanMarksNoProvider(t *testing.T) {
	m := mkTestManifest()
	// Registry has nothing, so every item has no provider.
	r := &Runner{
		Manifest: m,
		Registry: provider.NewRegistry(),
		State:    state.New(),
		Out:      io.Discard,
	}
	plan, err := r.Plan(context.Background(), Selection{Bundle: "base"})
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}
	for _, e := range plan.Entries {
		if !e.NoProvider {
			t.Errorf("entry %s should be NoProvider", e.ItemID)
		}
	}
}

func TestPlanAnnotatesTrackedByUs(t *testing.T) {
	m := mkTestManifest()
	stub := &stubProvider{kind: "apt", checkState: provider.State{Installed: true, Version: "1.7.1"}}
	st := state.New()
	st.RecordInstall("jq", "apt", "1.7.1", state.ByOnboardctl, timeForTest())

	r := &Runner{
		Manifest: m,
		Registry: buildRegistry(stub),
		State:    st,
		Out:      io.Discard,
	}
	plan, err := r.Plan(context.Background(), Selection{Bundle: "base"})
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}
	for _, e := range plan.Entries {
		if e.ItemID == "jq" {
			if !e.TrackedByUs {
				t.Error("jq should be TrackedByUs")
			}
			if e.TrackedByUsVer != "1.7.1" {
				t.Errorf("TrackedByUsVer = %q, want 1.7.1", e.TrackedByUsVer)
			}
		}
		if e.ItemID == "vlc" && e.TrackedByUs {
			t.Error("vlc should not be TrackedByUs")
		}
	}
}

func TestPlanCounts(t *testing.T) {
	p := &Plan{
		Entries: []PlanEntry{
			{TrackedByUs: true, State: provider.State{Installed: true}},
			{State: provider.State{Installed: true}},      // installed-external
			{},                                            // not-installed
			{Skipped: true},
			{NoProvider: true},
		},
	}
	c := p.Counts()
	if c.Total != 5 {
		t.Errorf("Total = %d, want 5", c.Total)
	}
	if c.InstalledByUs != 1 {
		t.Errorf("InstalledByUs = %d, want 1", c.InstalledByUs)
	}
	if c.InstalledExternal != 1 {
		t.Errorf("InstalledExternal = %d, want 1", c.InstalledExternal)
	}
	if c.NotInstalled != 1 {
		t.Errorf("NotInstalled = %d, want 1", c.NotInstalled)
	}
	if c.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", c.Skipped)
	}
	if c.NoProvider != 1 {
		t.Errorf("NoProvider = %d, want 1", c.NoProvider)
	}
}

func timeForTest() time.Time {
	return time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
}
