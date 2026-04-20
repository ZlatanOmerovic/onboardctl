package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// OneOfOption is a single row in a OneOfModel.
//
// Value is what the caller gets back. A zero-value Value is the
// idiomatic "keep current / no action" sentinel.
type OneOfOption struct {
	Value       string
	Label       string
	Description string
}

// OneOfResult is returned by RunOneOf. Value is the chosen option's Value
// field. If the user quit without choosing, Cancelled is true.
type OneOfResult struct {
	Value     string
	Label     string
	Cancelled bool
}

// OneOfModel is a small vertical-list picker. Unlike ChoiceModel, this
// keeps the options static (no filter, no large list), making it
// appropriate for wizards whose stages have a handful of curated picks.
type OneOfModel struct {
	title    string
	prompt   string
	options  []OneOfOption
	cursor   int
	result   OneOfResult
	quitting bool
}

// NewOneOfModel builds the model from a title/prompt plus the list of
// options. Cursor starts on the first option.
func NewOneOfModel(title, prompt string, options []OneOfOption) OneOfModel {
	return OneOfModel{title: title, prompt: prompt, options: options}
}

// Init implements tea.Model.
func (m OneOfModel) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m OneOfModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.result.Cancelled = true
			m.quitting = true
			return m, tea.Quit
		case "esc":
			m.result.Cancelled = true
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		case "home", "g":
			m.cursor = 0
		case "end", "G":
			m.cursor = len(m.options) - 1
		case "enter":
			if len(m.options) == 0 {
				m.result.Cancelled = true
				m.quitting = true
				return m, tea.Quit
			}
			opt := m.options[m.cursor]
			m.result.Value = opt.Value
			m.result.Label = opt.Label
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, nil
}

// View implements tea.Model.
func (m OneOfModel) View() string {
	if m.quitting {
		return ""
	}
	var b strings.Builder
	b.WriteString(TitleStyle.Render(m.title))
	b.WriteString("\n")
	if m.prompt != "" {
		b.WriteString(DescriptionStyle.Render(m.prompt))
		b.WriteString("\n\n")
	}
	for i, opt := range m.options {
		cursor := "  "
		label := opt.Label
		if i == m.cursor {
			cursor = CursorStyle.Render("❯ ")
			label = SelectedStyle.Render(label)
		} else {
			label = NormalStyle.Render(label)
		}
		b.WriteString(cursor + label + "\n")
		if opt.Description != "" {
			b.WriteString("    " + DimStyle.Render(opt.Description) + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(HelpStyle.Render("↑/↓ or k/j navigate · enter select · esc/q cancel"))
	return b.String()
}

// Result returns the final result (zero-value Value if cancelled).
func (m OneOfModel) Result() OneOfResult { return m.result }

// RunOneOf is the one-shot helper: construct, run, return result.
func RunOneOf(ctx context.Context, title, prompt string, options []OneOfOption, out io.Writer) (OneOfResult, error) {
	if len(options) == 0 {
		return OneOfResult{}, errors.New("tui: RunOneOf needs at least one option")
	}
	model := NewOneOfModel(title, prompt, options)
	opts := []tea.ProgramOption{tea.WithOutput(out)}
	prog := tea.NewProgram(model, opts...)
	final, err := prog.Run()
	if err != nil {
		return OneOfResult{}, fmt.Errorf("tui: %w", err)
	}
	om, ok := final.(OneOfModel)
	if !ok {
		return OneOfResult{}, fmt.Errorf("tui: unexpected final model type %T", final)
	}
	_ = ctx
	return om.Result(), nil
}
