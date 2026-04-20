package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func sampleOptions() []OneOfOption {
	return []OneOfOption{
		{Value: "", Label: "Keep current", Description: "no change"},
		{Value: "kitty", Label: "Kitty", Description: "GPU terminal"},
		{Value: "alacritty", Label: "Alacritty", Description: "lean terminal"},
	}
}

func TestOneOfModelInitialCursor(t *testing.T) {
	m := NewOneOfModel("Terminal", "Pick one", sampleOptions())
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.cursor)
	}
}

func TestOneOfModelNavigation(t *testing.T) {
	m := NewOneOfModel("Terminal", "", sampleOptions())
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = next.(OneOfModel)
	if m.cursor != 1 {
		t.Errorf("after j, cursor = %d, want 1", m.cursor)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = next.(OneOfModel)
	if m.cursor != 2 {
		t.Errorf("end key did not jump to last; cursor = %d", m.cursor)
	}
}

func TestOneOfModelEnterReturnsSelection(t *testing.T) {
	m := NewOneOfModel("Terminal", "", sampleOptions())
	// Move to "kitty" (index 1)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = next.(OneOfModel)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(OneOfModel)
	if cmd == nil {
		t.Error("expected tea.Quit cmd")
	}
	if m.Result().Cancelled {
		t.Error("should not be cancelled on enter")
	}
	if m.Result().Value != "kitty" {
		t.Errorf("Value = %q, want kitty", m.Result().Value)
	}
	if m.Result().Label != "Kitty" {
		t.Errorf("Label = %q, want Kitty", m.Result().Label)
	}
}

func TestOneOfModelEscCancels(t *testing.T) {
	m := NewOneOfModel("Terminal", "", sampleOptions())
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(OneOfModel)
	if cmd == nil {
		t.Error("expected tea.Quit cmd")
	}
	if !m.Result().Cancelled {
		t.Error("expected Cancelled=true")
	}
}

func TestOneOfModelQKeyCancels(t *testing.T) {
	m := NewOneOfModel("Terminal", "", sampleOptions())
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = next.(OneOfModel)
	if !m.Result().Cancelled {
		t.Error("expected Cancelled=true after q")
	}
}

func TestOneOfModelViewShowsAll(t *testing.T) {
	m := NewOneOfModel("Terminal", "Which one?", sampleOptions())
	v := m.View()
	for _, sub := range []string{"Terminal", "Which one?", "Keep current", "Kitty", "Alacritty"} {
		if !strings.Contains(v, sub) {
			t.Errorf("view missing %q", sub)
		}
	}
}
