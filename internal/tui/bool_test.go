package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
)

func TestBoolModelDefaultFalse(t *testing.T) {
	m := NewBoolModel("x", "X", &manifest.Input{Kind: manifest.InputBool})
	if m.value {
		t.Error("default should be false when Default is unset")
	}
}

func TestBoolModelDefaultFromString(t *testing.T) {
	cases := map[string]bool{
		"true":  true,
		"TRUE":  true,
		"yes":   true,
		"y":     true,
		"1":     true,
		"false": false,
		"no":    false,
		"":      false,
		"nope":  false,
	}
	for in, want := range cases {
		m := NewBoolModel("x", "X", &manifest.Input{Kind: manifest.InputBool, Default: in})
		if m.value != want {
			t.Errorf("default(%q) = %v, want %v", in, m.value, want)
		}
	}
}

func TestBoolModelDefaultFromBool(t *testing.T) {
	m := NewBoolModel("x", "X", &manifest.Input{Kind: manifest.InputBool, Default: true})
	if !m.value {
		t.Error("default bool(true) should set value=true")
	}
}

func TestBoolModelYKeySetsTrue(t *testing.T) {
	m := NewBoolModel("x", "X", &manifest.Input{Kind: manifest.InputBool})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = next.(BoolModel)
	if !m.value {
		t.Error("y should set value=true")
	}
}

func TestBoolModelNKeySetsFalse(t *testing.T) {
	m := NewBoolModel("x", "X", &manifest.Input{Kind: manifest.InputBool, Default: "true"})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = next.(BoolModel)
	if m.value {
		t.Error("n should set value=false")
	}
}

func TestBoolModelTabToggles(t *testing.T) {
	m := NewBoolModel("x", "X", &manifest.Input{Kind: manifest.InputBool})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(BoolModel)
	if !m.value {
		t.Error("tab from default(false) should flip to true")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(BoolModel)
	if m.value {
		t.Error("tab again should flip back to false")
	}
}

func TestBoolModelEnterConfirmsTrueValue(t *testing.T) {
	m := NewBoolModel("x", "X", &manifest.Input{Kind: manifest.InputBool, Default: "yes"})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(BoolModel)
	if !m.Result().Confirmed {
		t.Error("Confirmed should be true")
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd")
	}
	if m.Result().Values["value"] != "true" {
		t.Errorf("value = %q, want 'true'", m.Result().Values["value"])
	}
}

func TestBoolModelEnterConfirmsFalseValue(t *testing.T) {
	m := NewBoolModel("x", "X", &manifest.Input{Kind: manifest.InputBool})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(BoolModel)
	if m.Result().Values["value"] != "false" {
		t.Errorf("value = %q, want 'false'", m.Result().Values["value"])
	}
}

func TestBoolModelEscCancels(t *testing.T) {
	m := NewBoolModel("x", "X", &manifest.Input{Kind: manifest.InputBool})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(BoolModel)
	if !m.Result().Cancelled {
		t.Error("expected Cancelled=true")
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd")
	}
}

func TestBoolModelViewShowsBothOptions(t *testing.T) {
	m := NewBoolModel("night-light", "Night light", &manifest.Input{Kind: manifest.InputBool, Prompt: "Enable?"})
	v := m.View()
	for _, sub := range []string{"Night light", "Yes", "No", "Enable?"} {
		if !strings.Contains(v, sub) {
			t.Errorf("view missing %q: %q", sub, v)
		}
	}
}
