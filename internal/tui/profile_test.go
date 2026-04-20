package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
)

func mkManifest() *manifest.Manifest {
	return &manifest.Manifest{
		Version: 1,
		Profiles: map[string]manifest.Profile{
			"everything":    {Name: "Everything", Description: "All of it."},
			"essentials":    {Name: "Essentials", Description: "Minimum."},
			"fullstack-web": {Name: "Fullstack web", Description: "LAMP-ish."},
			"custom-one":    {Name: "Custom One", Description: "User extra."},
		},
	}
}

func TestProfileModelOrdering(t *testing.T) {
	m := NewProfileModel(mkManifest(), map[string]int{})
	if len(m.profiles) != 4 {
		t.Fatalf("want 4 profiles, got %d", len(m.profiles))
	}
	// Known order first: essentials, fullstack-web, then unknown (custom-one), then everything.
	// Wait — everything has rank 4 (known), custom-one is unknown (1000). So:
	// essentials (0), fullstack-web (1), everything (4), custom-one (1000).
	want := []string{"essentials", "fullstack-web", "everything", "custom-one"}
	for i, exp := range want {
		if m.profiles[i].id != exp {
			t.Errorf("profiles[%d].id = %q, want %q", i, m.profiles[i].id, exp)
		}
	}
}

func TestProfileModelNavigation(t *testing.T) {
	m := NewProfileModel(mkManifest(), map[string]int{"essentials": 10})

	// Initial cursor at 0.
	if m.cursor != 0 {
		t.Errorf("initial cursor = %d", m.cursor)
	}

	// Down once.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = next.(ProfileModel)
	if m.cursor != 1 {
		t.Errorf("after j, cursor = %d, want 1", m.cursor)
	}

	// Up past zero clamps.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = next.(ProfileModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = next.(ProfileModel)
	if m.cursor != 0 {
		t.Errorf("cursor = %d after over-up, want 0", m.cursor)
	}
}

func TestProfileModelEnterPicks(t *testing.T) {
	m := NewProfileModel(mkManifest(), map[string]int{})
	// Move to 'fullstack-web' (index 1 after ordering).
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = next.(ProfileModel)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(ProfileModel)
	if !m.Choice().Picked {
		t.Fatal("Choice().Picked = false after enter")
	}
	if m.Choice().ID != "fullstack-web" {
		t.Errorf("Choice().ID = %q, want fullstack-web", m.Choice().ID)
	}
	if cmd == nil {
		t.Error("expected Quit command after enter")
	}
}

func TestProfileModelQuitWithoutSelection(t *testing.T) {
	m := NewProfileModel(mkManifest(), map[string]int{})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = next.(ProfileModel)
	if m.Choice().Picked {
		t.Error("Choice().Picked should be false after q")
	}
	if cmd == nil {
		t.Error("expected Quit command after q")
	}
}

func TestProfileModelView(t *testing.T) {
	m := NewProfileModel(mkManifest(), map[string]int{
		"essentials": 25,
	})
	v := m.View()
	if !strings.Contains(v, "pick a profile") {
		t.Errorf("view missing title: %q", v)
	}
	if !strings.Contains(v, "Essentials") {
		t.Errorf("view missing essentials: %q", v)
	}
	if !strings.Contains(v, "25 items") {
		t.Errorf("view missing item count: %q", v)
	}
}
