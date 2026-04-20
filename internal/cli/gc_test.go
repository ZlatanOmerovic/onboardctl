package cli

import (
	"testing"
	"time"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	"github.com/ZlatanOmerovic/onboardctl/internal/state"
)

func TestFindStaleItems(t *testing.T) {
	st := state.New()
	st.RecordInstall("jq", "apt", "1.7.1", state.ByOnboardctl, time.Time{})
	st.RecordInstall("old-tool", "apt", "0.1", state.ByOnboardctl, time.Time{})
	st.RecordInstall("also-old", "apt", "0.2", state.ByOnboardctl, time.Time{})

	m := &manifest.Manifest{
		Version: 1,
		Items:   map[string]manifest.Item{"jq": {Name: "jq"}},
	}
	stale := findStaleItems(st, m)
	if len(stale) != 2 {
		t.Fatalf("stale = %v, want 2 entries", stale)
	}
	// findStaleItems sorts alphabetically.
	if stale[0] != "also-old" || stale[1] != "old-tool" {
		t.Errorf("stale ordering: %v", stale)
	}
}

func TestFindStaleItemsEmpty(t *testing.T) {
	st := state.New()
	st.RecordInstall("jq", "apt", "1.7.1", state.ByOnboardctl, time.Time{})
	m := &manifest.Manifest{Version: 1, Items: map[string]manifest.Item{"jq": {Name: "jq"}}}
	if s := findStaleItems(st, m); len(s) != 0 {
		t.Errorf("expected empty, got %v", s)
	}
}

func TestPluralYies(t *testing.T) {
	if pluralYies(1) != "y" {
		t.Errorf("pluralYies(1) = %q, want y", pluralYies(1))
	}
	if pluralYies(0) != "ies" || pluralYies(2) != "ies" {
		t.Errorf("pluralYies plural cases wrong")
	}
}
