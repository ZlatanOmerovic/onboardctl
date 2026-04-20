package tui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// ChoiceModel picks a single value from a (potentially large) list
// using bubbles/list's built-in fuzzy filtering. The common case is
// the ~400-entry timezone list produced by `timedatectl list-timezones`.
//
// Result: FormResult.Values["value"] carries the user's pick, matching
// the manifest convention of {value} substitution in Apply commands.
type ChoiceModel struct {
	itemID   string
	itemName string
	prompt   string
	list     list.Model
	result   FormResult
	quitting bool
}

// choiceItem is the simplest list.Item possible — just a string.
type choiceItem string

func (c choiceItem) FilterValue() string { return string(c) }
func (c choiceItem) Title() string       { return string(c) }
func (c choiceItem) Description() string { return "" }

// NewChoiceModel builds the model from a resolved choice list. The
// caller is responsible for resolving Input.Source to an explicit
// []string via ResolveChoices before constructing the model.
func NewChoiceModel(itemID, itemName string, in *manifest.Input, choices []string) ChoiceModel {
	items := make([]list.Item, len(choices))
	for i, c := range choices {
		items[i] = choiceItem(c)
	}

	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.SetHeight(1)
	delegate.SetSpacing(0)

	l := list.New(items, delegate, 60, 18)
	l.Title = firstNonEmptyChoice(in.Prompt, "Choose:")
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)

	return ChoiceModel{
		itemID:   itemID,
		itemName: itemName,
		prompt:   in.Prompt,
		list:     l,
	}
}

// Init implements tea.Model.
func (m ChoiceModel) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m ChoiceModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetWidth(msg.Width - 4)
		if msg.Height > 8 {
			m.list.SetHeight(msg.Height - 6)
		}
	case tea.KeyMsg:
		// When the list is taking filter input, hand the key through
		// without our own interpretation — the user is typing.
		if m.list.FilterState() == list.Filtering {
			break
		}
		switch msg.String() {
		case "ctrl+c":
			m.result.Quit = true
			m.quitting = true
			return m, tea.Quit
		case "esc":
			m.result.Cancelled = true
			m.quitting = true
			return m, tea.Quit
		case "enter":
			if it, ok := m.list.SelectedItem().(choiceItem); ok {
				m.result.Confirmed = true
				m.result.Values = map[string]string{"value": string(it)}
				m.quitting = true
				return m, tea.Quit
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// View implements tea.Model.
func (m ChoiceModel) View() string {
	if m.quitting {
		return ""
	}
	var b strings.Builder
	b.WriteString(TitleStyle.Render(fmt.Sprintf("Configure — %s", m.itemName)))
	b.WriteString("\n")
	b.WriteString(m.list.View())
	b.WriteString("\n")
	b.WriteString(HelpStyle.Render("↑/↓ navigate · / filter · enter select · esc cancel"))
	return b.String()
}

// Result returns what the user picked (zero value if they quit).
func (m ChoiceModel) Result() FormResult { return m.result }

// ResolveChoices turns an Input into an explicit []string of options.
//
//   - in.Choices, if populated, wins (static list from the manifest).
//   - in.Source, otherwise, is shelled out (bash -c) and stdout is
//     split on newlines into choices.
//   - Missing both is an error.
func ResolveChoices(ctx context.Context, in *manifest.Input) ([]string, error) {
	if len(in.Choices) > 0 {
		return in.Choices, nil
	}
	if in.Source != "" {
		cmd := exec.CommandContext(ctx, "bash", "-c", in.Source)
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("resolve choices via source %q: %w", in.Source, err)
		}
		var choices []string
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				choices = append(choices, line)
			}
		}
		if len(choices) == 0 {
			return nil, fmt.Errorf("choice source %q produced no output", in.Source)
		}
		return choices, nil
	}
	return nil, fmt.Errorf("choice input needs either .choices or .source")
}

// firstNonEmptyChoice is a local helper to avoid collision with other TUI files.
func firstNonEmptyChoice(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
