package tui

import (
	"fmt"
	"strings"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	tea "github.com/charmbracelet/bubbletea"
)

// BoolModel is the simplest input: a yes/no prompt.
//
// Result: FormResult.Values["value"] is "true" or "false", matching
// the convention used by config items that substitute {value} into
// Apply commands (e.g. "gsettings set ... enabled {value}").
type BoolModel struct {
	itemID   string
	itemName string
	prompt   string
	value    bool // current selection
	result   FormResult
	quitting bool
}

// NewBoolModel builds a BoolModel. The default selection comes from
// Input.Default (accepts bool, "true"/"false" strings, or "yes"/"no");
// falls back to false.
func NewBoolModel(itemID, itemName string, in *manifest.Input) BoolModel {
	return BoolModel{
		itemID:   itemID,
		itemName: itemName,
		prompt:   in.Prompt,
		value:    parseBoolDefault(in.Default),
	}
}

// Init implements tea.Model.
func (m BoolModel) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m BoolModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		case "y", "Y":
			m.value = true
		case "n", "N":
			m.value = false
		case "tab", "left", "right", "h", "l", " ":
			m.value = !m.value
		case "enter":
			m.result.Confirmed = true
			m.result.Values = map[string]string{"value": boolStr(m.value)}
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, nil
}

// View implements tea.Model.
func (m BoolModel) View() string {
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

	// Radio-style display. Selected option uses SelectedStyle; the other is dim.
	yes := "( ) Yes"
	no := "( ) No"
	if m.value {
		yes = CursorStyle.Render("(•) Yes")
		no = DimStyle.Render("( ) No")
	} else {
		yes = DimStyle.Render("( ) Yes")
		no = CursorStyle.Render("(•) No")
	}
	b.WriteString("  " + yes + "\n")
	b.WriteString("  " + no + "\n\n")

	b.WriteString(HelpStyle.Render("y/n to pick · tab/space toggle · enter confirm · esc cancel"))
	return b.String()
}

// Result returns what the user picked (zero value if they quit).
func (m BoolModel) Result() FormResult { return m.result }

// parseBoolDefault interprets the Input.Default payload. Accepts a
// native bool, or any of "true"/"false"/"yes"/"no" case-insensitive.
// Anything else falls back to false.
func parseBoolDefault(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "true", "yes", "y", "1":
			return true
		}
	}
	return false
}

// boolStr is strconv.FormatBool written out so we don't import strconv
// for one call.
func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
