package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
)

// The order we like to surface profiles in. Anything unknown (user extras,
// etc.) sorts to the end alphabetically.
var profileOrder = []string{
	"essentials",
	"fullstack-web",
	"devops",
	"polyglot-dev",
	"everything",
}

// ProfileChoice captures what the user picked. The zero value means
// "no selection" — the user quit without choosing.
type ProfileChoice struct {
	Picked bool
	ID     string
	Name   string
}

// ProfileModel is the top-level picker. It presents the manifest's
// profiles as a vertical list, with a header and help footer.
type ProfileModel struct {
	profiles []profileEntry
	cursor   int
	chosen   ProfileChoice
	quitting bool
}

type profileEntry struct {
	id          string
	name        string
	description string
	itemCount   int
}

// NewProfileModel builds the picker from a loaded manifest plus pre-resolved
// per-profile item counts (so the caller can use the runner's Resolve to
// compute accurate counts including 'extends' inheritance).
func NewProfileModel(m *manifest.Manifest, itemCounts map[string]int) ProfileModel {
	entries := make([]profileEntry, 0, len(m.Profiles))
	for id, p := range m.Profiles {
		entries = append(entries, profileEntry{
			id:          id,
			name:        p.Name,
			description: p.Description,
			itemCount:   itemCounts[id],
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return profileRank(entries[i].id) < profileRank(entries[j].id) ||
			(profileRank(entries[i].id) == profileRank(entries[j].id) && entries[i].id < entries[j].id)
	})
	return ProfileModel{profiles: entries}
}

// profileRank maps known IDs to the preferred display order, and unknowns
// to a large number so they sort after the curated set.
func profileRank(id string) int {
	for i, known := range profileOrder {
		if known == id {
			return i
		}
	}
	return 1000
}

// Init implements tea.Model.
func (m ProfileModel) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m ProfileModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.profiles)-1 {
				m.cursor++
			}
		case "home", "g":
			m.cursor = 0
		case "end", "G":
			m.cursor = len(m.profiles) - 1
		case "enter":
			if len(m.profiles) > 0 {
				p := m.profiles[m.cursor]
				m.chosen = ProfileChoice{Picked: true, ID: p.id, Name: p.name}
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

// View implements tea.Model.
func (m ProfileModel) View() string {
	if m.quitting && !m.chosen.Picked {
		return DimStyle.Render("cancelled") + "\n"
	}
	if m.quitting && m.chosen.Picked {
		// tea.Quit was requested; leaving a clean line lets the caller print.
		return ""
	}

	var b strings.Builder
	b.WriteString(TitleStyle.Render("onboardctl — pick a profile"))
	b.WriteString("\n\n")

	for i, p := range m.profiles {
		cursor := "  "
		name := p.name
		desc := p.description
		if i == m.cursor {
			cursor = CursorStyle.Render("❯ ")
			name = SelectedStyle.Render(name)
			desc = DescriptionStyle.Render(desc)
		} else {
			name = NormalStyle.Render(name)
			desc = DimStyle.Render(desc)
		}
		count := DimStyle.Render(fmt.Sprintf(" (%d items)", p.itemCount))
		b.WriteString(cursor + name + count + "\n")
		if desc != "" {
			b.WriteString("    " + desc + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(HelpStyle.Render("↑/↓ or k/j navigate · enter select · q/esc cancel"))
	return b.String()
}

// Choice returns what the user picked (zero value if they quit).
func (m ProfileModel) Choice() ProfileChoice { return m.chosen }
