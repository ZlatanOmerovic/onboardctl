package tui

import (
	"strings"
	"testing"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	tea "github.com/charmbracelet/bubbletea"
)

func TestNewFormModelForm(t *testing.T) {
	in := &manifest.Input{
		Kind:   manifest.InputForm,
		Prompt: "Set your git identity",
		Fields: []manifest.Field{
			{Name: "name", Prompt: "Full name"},
			{Name: "email", Prompt: "Git email"},
			{Name: "branch", Prompt: "Default branch", Default: "main"},
		},
	}
	m := NewFormModel("git-identity", "Git identity", in)
	if len(m.fields) != 3 {
		t.Errorf("fields = %d, want 3", len(m.fields))
	}
	if len(m.inputs) != 3 {
		t.Errorf("inputs = %d, want 3", len(m.inputs))
	}
	if m.inputs[2].Value() != "main" {
		t.Errorf("default not applied: inputs[2].Value() = %q", m.inputs[2].Value())
	}
}

func TestNewFormModelText(t *testing.T) {
	in := &manifest.Input{Kind: manifest.InputText, Prompt: "Hostname?"}
	m := NewFormModel("hostname", "Hostname", in)
	if len(m.fields) != 1 {
		t.Fatalf("fields = %d, want 1", len(m.fields))
	}
	if m.fields[0].Name != "value" {
		t.Errorf("synthetic field name = %q, want 'value'", m.fields[0].Name)
	}
}

func TestFormModelEscCancels(t *testing.T) {
	m := NewFormModel("foo", "Foo", &manifest.Input{Kind: manifest.InputForm,
		Fields: []manifest.Field{{Name: "a"}}})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(FormModel)
	if !m.Result().Cancelled {
		t.Error("expected Cancelled=true")
	}
	if cmd == nil {
		t.Error("expected Quit cmd")
	}
}

func TestFormModelEnterOnLastFieldConfirms(t *testing.T) {
	in := &manifest.Input{Kind: manifest.InputForm,
		Fields: []manifest.Field{
			{Name: "name"},
			{Name: "email"},
		}}
	m := NewFormModel("git-identity", "Git identity", in)

	// Set values on both fields.
	m.inputs[0].SetValue("Zlatan")
	m.inputs[1].SetValue("z@example.com")

	// Move focus to the last field (via tab).
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(FormModel)
	if m.focusIdx != 1 {
		t.Fatalf("focusIdx = %d after tab, want 1", m.focusIdx)
	}

	// Enter on last field → confirm.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(FormModel)
	if !m.Result().Confirmed {
		t.Error("expected Confirmed=true after enter on last field")
	}
	if cmd == nil {
		t.Error("expected Quit cmd after confirm")
	}
	if m.Result().Values["name"] != "Zlatan" || m.Result().Values["email"] != "z@example.com" {
		t.Errorf("values drifted: %+v", m.Result().Values)
	}
}

func TestFormModelEnterOnMiddleFieldAdvances(t *testing.T) {
	in := &manifest.Input{Kind: manifest.InputForm,
		Fields: []manifest.Field{
			{Name: "name"},
			{Name: "email"},
			{Name: "branch"},
		}}
	m := NewFormModel("git-identity", "Git identity", in)
	// Enter while on first field should advance, not confirm.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(FormModel)
	if m.Result().Confirmed {
		t.Error("should not have confirmed on non-last field")
	}
	if m.focusIdx != 1 {
		t.Errorf("focusIdx = %d after enter on field 0, want 1", m.focusIdx)
	}
}

func TestFormModelViewMentionsFieldsAndPrompt(t *testing.T) {
	in := &manifest.Input{Kind: manifest.InputForm, Prompt: "Set identity",
		Fields: []manifest.Field{
			{Name: "name", Prompt: "Full name"},
			{Name: "email", Prompt: "Git email"},
		}}
	m := NewFormModel("git-identity", "Git identity", in)
	v := m.View()
	if !strings.Contains(v, "Git identity") {
		t.Errorf("view missing item name: %q", v)
	}
	if !strings.Contains(v, "Set identity") {
		t.Errorf("view missing prompt: %q", v)
	}
	if !strings.Contains(v, "Full name") {
		t.Errorf("view missing field prompt: %q", v)
	}
}
