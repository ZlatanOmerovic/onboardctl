package tui

import (
	"fmt"
	"strings"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// FormModel is a simple multi-field text-input form for items whose
// manifest Input.kind is "form" or "text".
//
// For kind="text", the caller should synthesise a single-field form
// whose field name is "value" (matching the convention in the manifest's
// config items that use {value} in Apply commands).
//
// Choice and bool kinds aren't handled here — they'll get their own
// models when the manifest grows items needing them.
type FormModel struct {
	itemID   string
	itemName string
	prompt   string
	fields   []manifest.Field
	inputs   []textinput.Model
	focusIdx int
	values   map[string]string
	result   FormResult
	quitting bool
}

// FormResult is what the caller reads after the form exits.
type FormResult struct {
	Confirmed bool              // user hit enter on the last field
	Cancelled bool              // user hit esc (goes back one step in the flow)
	Quit      bool              // user hit q/ctrl+c
	Values    map[string]string // field-name → value (only meaningful when Confirmed)
}

// NewFormModel builds a form from an item's Input block.
// The caller is responsible for passing only items whose input is
// one of kind=text / kind=form; anything else panics as a wiring bug.
func NewFormModel(itemID, itemName string, in *manifest.Input) FormModel {
	if in == nil {
		panic("tui.NewFormModel: nil input")
	}

	fields := normaliseFields(in)
	inputs := make([]textinput.Model, len(fields))
	for i, f := range fields {
		ti := textinput.New()
		ti.Placeholder = f.Prompt
		ti.CharLimit = 200
		ti.Width = 40
		if f.Default != "" {
			ti.SetValue(f.Default)
		}
		if f.Secret {
			ti.EchoMode = textinput.EchoPassword
			ti.EchoCharacter = '•'
		}
		inputs[i] = ti
	}
	if len(inputs) > 0 {
		inputs[0].Focus()
	}

	return FormModel{
		itemID:   itemID,
		itemName: itemName,
		prompt:   in.Prompt,
		fields:   fields,
		inputs:   inputs,
		values:   make(map[string]string, len(fields)),
	}
}

// normaliseFields returns a []Field for both kind=text (one synthetic
// field named "value") and kind=form (as-declared).
func normaliseFields(in *manifest.Input) []manifest.Field {
	if in.Kind == manifest.InputForm {
		return in.Fields
	}
	if in.Kind == manifest.InputText {
		return []manifest.Field{{Name: "value", Prompt: in.Prompt, Default: defaultAsString(in.Default)}}
	}
	// Defensive: unsupported kinds get an empty field set so the form
	// renders a "no fields to fill" message rather than crashing.
	return nil
}

func defaultAsString(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// Init implements tea.Model.
func (m FormModel) Init() tea.Cmd { return textinput.Blink }

// Update implements tea.Model.
func (m FormModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.result.Quit = true
			m.quitting = true
			return m, tea.Quit
		case "esc":
			m.result.Cancelled = true
			m.quitting = true
			return m, tea.Quit
		case "tab", "down":
			cmd := m.advanceFocus(+1)
			return m, cmd
		case "shift+tab", "up":
			cmd := m.advanceFocus(-1)
			return m, cmd
		case "enter":
			// If we're on the last field, confirm; otherwise advance.
			if m.focusIdx == len(m.inputs)-1 {
				m.result.Confirmed = true
				for i, f := range m.fields {
					m.values[f.Name] = strings.TrimSpace(m.inputs[i].Value())
				}
				m.result.Values = m.values
				m.quitting = true
				return m, tea.Quit
			}
			cmd := m.advanceFocus(+1)
			return m, cmd
		}
	}

	// Let the focused textinput consume the msg.
	var cmd tea.Cmd
	if m.focusIdx < len(m.inputs) {
		var ti textinput.Model
		ti, cmd = m.inputs[m.focusIdx].Update(msg)
		m.inputs[m.focusIdx] = ti
	}
	return m, cmd
}

// View implements tea.Model.
func (m FormModel) View() string {
	if m.quitting {
		return ""
	}
	var b strings.Builder
	b.WriteString(TitleStyle.Render(fmt.Sprintf("Configure — %s", m.itemName)))
	b.WriteString("\n\n")
	if m.prompt != "" {
		b.WriteString(DescriptionStyle.Render(m.prompt))
		b.WriteString("\n\n")
	}

	for i, f := range m.fields {
		label := f.Prompt
		if label == "" {
			label = f.Name
		}
		if i == m.focusIdx {
			label = SelectedStyle.Render(label)
		} else {
			label = NormalStyle.Render(label)
		}
		b.WriteString(fmt.Sprintf("  %s\n  %s\n\n", label, m.inputs[i].View()))
	}

	b.WriteString(HelpStyle.Render("tab/↑↓ next field · enter confirm · esc cancel · ctrl+c quit"))
	return b.String()
}

// Result returns what the user did with the form (zero value if quit).
func (m FormModel) Result() FormResult { return m.result }

// advanceFocus moves focus by delta (+1 for next, -1 for prev).
// Takes a pointer receiver so mutations stick; returns the focus tea.Cmd.
func (m *FormModel) advanceFocus(delta int) tea.Cmd {
	if len(m.inputs) == 0 {
		return nil
	}
	target := m.focusIdx + delta
	if target < 0 || target >= len(m.inputs) {
		return nil
	}
	m.inputs[m.focusIdx].Blur()
	m.focusIdx = target
	return m.inputs[m.focusIdx].Focus()
}
