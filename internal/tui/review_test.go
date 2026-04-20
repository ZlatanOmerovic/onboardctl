package tui

import (
	"strings"
	"testing"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	"github.com/ZlatanOmerovic/onboardctl/internal/provider"
	"github.com/ZlatanOmerovic/onboardctl/internal/runner"
	tea "github.com/charmbracelet/bubbletea"
)

func mkPlan() *runner.Plan {
	return &runner.Plan{
		Profile: "fullstack-web",
		Entries: []runner.PlanEntry{
			{
				ItemID:       "jq",
				Item:         manifest.Item{Name: "jq", Bundle: "base-system"},
				ProviderKind: "apt",
				State:        provider.State{Installed: true, Version: "1.7.1"},
				TrackedByUs:  true,
			},
			{
				ItemID:       "new-tool",
				Item:         manifest.Item{Name: "New Tool", Bundle: "base-system"},
				ProviderKind: "apt",
				State:        provider.State{Installed: false},
			},
			{
				ItemID:       "kitty",
				Item:         manifest.Item{Name: "Kitty", Bundle: "terminal-stack"},
				ProviderKind: "apt",
				State:        provider.State{Installed: true, Version: "0.41.1"},
			},
			{
				ItemID: "skipped-thing",
				Item:   manifest.Item{Name: "Skipped", Bundle: "base-system"},
				Skipped: true,
			},
		},
	}
}

func TestReviewModelInitialCursorOnFirstItem(t *testing.T) {
	m := NewReviewModel("Fullstack web", "fullstack-web", mkPlan())
	// rows[0] is the base-system header; rows[1] is the first item (jq).
	if m.rows[m.cursor].isHeader {
		t.Error("cursor starts on a header")
	}
}

func TestReviewModelNavigationSkipsHeaders(t *testing.T) {
	m := NewReviewModel("Fullstack web", "fullstack-web", mkPlan())
	// Press j twice: should land on kitty (over skipped+header gap).
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = next.(ReviewModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = next.(ReviewModel)
	// It should now be on a non-header row.
	if m.rows[m.cursor].isHeader {
		t.Error("cursor landed on header after j j")
	}
}

func TestReviewModelSpaceTogglesItem(t *testing.T) {
	m := NewReviewModel("Fullstack web", "fullstack-web", mkPlan())
	initial := m.rows[m.cursor].selected
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = next.(ReviewModel)
	if m.rows[m.cursor].selected == initial {
		t.Error("space did not toggle selection")
	}
}

func TestReviewModelSpaceDoesNotToggleLocked(t *testing.T) {
	m := NewReviewModel("Fullstack web", "fullstack-web", mkPlan())
	// Navigate to the skipped row (which is locked).
	for i, r := range m.rows {
		if !r.isHeader && r.itemID == "skipped-thing" {
			m.cursor = i
			break
		}
	}
	initial := m.rows[m.cursor].selected
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = next.(ReviewModel)
	if m.rows[m.cursor].selected != initial {
		t.Error("space toggled a locked row")
	}
}

func TestReviewModelAllSelectsAllNonLocked(t *testing.T) {
	m := NewReviewModel("Fullstack web", "fullstack-web", mkPlan())
	// First deselect everything.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(ReviewModel)
	for _, r := range m.rows {
		if !r.isHeader && !r.locked && r.selected {
			t.Errorf("after 'n', %s still selected", r.itemID)
		}
	}
	// Then select all.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = next.(ReviewModel)
	for _, r := range m.rows {
		if !r.isHeader && !r.locked && !r.selected {
			t.Errorf("after 'a', %s not selected", r.itemID)
		}
	}
}

func TestReviewModelEnterProducesFilteredSelection(t *testing.T) {
	m := NewReviewModel("Fullstack web", "fullstack-web", mkPlan())
	// Deselect everything then select only jq.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(ReviewModel)
	for i, r := range m.rows {
		if !r.isHeader && r.itemID == "jq" {
			m.cursor = i
			break
		}
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = next.(ReviewModel)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(ReviewModel)
	if !m.Choice().Confirmed {
		t.Error("expected Confirmed=true")
	}
	if cmd == nil {
		t.Error("expected Quit cmd")
	}
	if len(m.Choice().ItemIDs) != 1 || m.Choice().ItemIDs[0] != "jq" {
		t.Errorf("ItemIDs = %v, want [jq]", m.Choice().ItemIDs)
	}
}

func TestReviewModelEscGoesBackToPicker(t *testing.T) {
	m := NewReviewModel("Fullstack web", "fullstack-web", mkPlan())
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(ReviewModel)
	if !m.Choice().BackToPicker {
		t.Error("expected BackToPicker=true")
	}
	if cmd == nil {
		t.Error("expected Quit cmd")
	}
}

func TestReviewModelViewMentionsProfileAndBundles(t *testing.T) {
	m := NewReviewModel("Fullstack web", "fullstack-web", mkPlan())
	v := m.View()
	if !strings.Contains(v, "Fullstack web") {
		t.Errorf("view missing profile name: %q", v)
	}
	if !strings.Contains(v, "base-system") {
		t.Errorf("view missing bundle header: %q", v)
	}
	if !strings.Contains(v, "terminal-stack") {
		t.Errorf("view missing second bundle: %q", v)
	}
}
