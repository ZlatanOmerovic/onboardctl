package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	tea "github.com/charmbracelet/bubbletea"
)

func TestChoiceModelFromStaticChoices(t *testing.T) {
	in := &manifest.Input{
		Kind:    manifest.InputChoice,
		Prompt:  "Shell?",
		Choices: []string{"bash", "zsh", "fish"},
	}
	m := NewChoiceModel("shell", "Shell", in, in.Choices)
	if m.list.Items() == nil || len(m.list.Items()) != 3 {
		t.Fatalf("expected 3 list items, got %d", len(m.list.Items()))
	}
}

func TestChoiceModelEnterSelects(t *testing.T) {
	choices := []string{"bash", "zsh", "fish"}
	m := NewChoiceModel("shell", "Shell",
		&manifest.Input{Kind: manifest.InputChoice, Prompt: "Shell?"},
		choices)
	// The default selection is item 0 (bash).
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(ChoiceModel)
	if !m.Result().Confirmed {
		t.Fatal("expected Confirmed=true after enter")
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd after enter")
	}
	if got := m.Result().Values["value"]; got != "bash" {
		t.Errorf("value = %q, want bash", got)
	}
}

func TestChoiceModelEscCancels(t *testing.T) {
	m := NewChoiceModel("shell", "Shell",
		&manifest.Input{Kind: manifest.InputChoice, Prompt: "Shell?"},
		[]string{"bash", "zsh"})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(ChoiceModel)
	if !m.Result().Cancelled {
		t.Error("expected Cancelled=true")
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd")
	}
}

func TestChoiceModelViewRendersName(t *testing.T) {
	m := NewChoiceModel("shell", "Shell",
		&manifest.Input{Kind: manifest.InputChoice, Prompt: "Shell?"},
		[]string{"bash", "zsh"})
	v := m.View()
	if !strings.Contains(v, "Shell") {
		t.Errorf("view missing item name: %q", v)
	}
}

func TestResolveChoicesFromStaticList(t *testing.T) {
	in := &manifest.Input{Kind: manifest.InputChoice, Choices: []string{"a", "b"}}
	got, err := ResolveChoices(context.Background(), in)
	if err != nil {
		t.Fatalf("ResolveChoices error: %v", err)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("got %v, want [a b]", got)
	}
}

func TestResolveChoicesFromSourceCommand(t *testing.T) {
	in := &manifest.Input{
		Kind:   manifest.InputChoice,
		Source: "printf 'alpha\\nbeta\\n\\ngamma\\n'",
	}
	got, err := ResolveChoices(context.Background(), in)
	if err != nil {
		t.Fatalf("ResolveChoices error: %v", err)
	}
	want := []string{"alpha", "beta", "gamma"}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d (%v)", len(got), len(want), got)
	}
	for i, s := range want {
		if got[i] != s {
			t.Errorf("got[%d] = %q, want %q", i, got[i], s)
		}
	}
}

func TestResolveChoicesNoSourceNoChoices(t *testing.T) {
	_, err := ResolveChoices(context.Background(), &manifest.Input{Kind: manifest.InputChoice})
	if err == nil {
		t.Fatal("expected error when neither Choices nor Source set")
	}
}

func TestResolveChoicesEmptySourceErrors(t *testing.T) {
	in := &manifest.Input{Kind: manifest.InputChoice, Source: "true"}
	_, err := ResolveChoices(context.Background(), in)
	if err == nil {
		t.Fatal("expected error when source produces no output")
	}
}
