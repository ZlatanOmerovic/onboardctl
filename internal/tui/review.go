package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ZlatanOmerovic/onboardctl/internal/runner"
)

// ReviewChoice is what the user ended with on the review screen.
type ReviewChoice struct {
	Confirmed    bool     // user hit enter on the confirm screen
	BackToPicker bool     // user hit esc → caller should re-show the profile picker
	Quit         bool     // user hit q/ctrl-c → caller should exit cleanly
	ItemIDs      []string // item IDs the user chose to include in the install plan
}

// ReviewModel is a scrollable, toggleable list of the plan entries for a
// chosen profile. Items are grouped by bundle with section headers; status
// markers next to each item show whether it's installed, external, or
// missing. User can toggle items with space; enter confirms.
type ReviewModel struct {
	profileName string
	profileID   string
	rows        []reviewRow // flat list including headers
	cursor      int         // index into rows; always points at an item row
	viewTop     int         // scroll offset
	viewHeight  int         // visible rows; set by WindowSizeMsg
	choice      ReviewChoice
	quitting    bool
	counts      runner.PlanCounts
}

type reviewRow struct {
	isHeader   bool
	bundleName string // for header rows

	// item fields (empty when isHeader)
	itemID      string
	displayName string
	subText     string
	marker      string
	markerStyle lipgloss.Style
	selected    bool
	locked      bool // true when the row can't be toggled (skipped / no provider)
}

// NewReviewModel assembles a ReviewModel from a runner.Plan.
//
// Default selection: every item that isn't skipped or provider-less is
// pre-selected. The user then unchecks what they don't want.
func NewReviewModel(profileName, profileID string, plan *runner.Plan) ReviewModel {
	m := ReviewModel{
		profileName: profileName,
		profileID:   profileID,
		viewHeight:  20, // reasonable default until we get a WindowSizeMsg
		counts:      plan.Counts(),
	}

	// Group entries by bundle, preserving resolved order within bundle.
	byBundle := make(map[string][]runner.PlanEntry)
	var bundleOrder []string
	seenBundle := make(map[string]bool)
	for _, e := range plan.Entries {
		bn := e.Item.Bundle
		if bn == "" {
			bn = "(misc)"
		}
		if !seenBundle[bn] {
			seenBundle[bn] = true
			bundleOrder = append(bundleOrder, bn)
		}
		byBundle[bn] = append(byBundle[bn], e)
	}
	sort.Strings(bundleOrder) // stable, predictable across runs

	for _, bn := range bundleOrder {
		m.rows = append(m.rows, reviewRow{isHeader: true, bundleName: bn})
		for _, e := range byBundle[bn] {
			row := reviewRow{
				itemID:      e.ItemID,
				displayName: firstNonEmpty(e.Item.Name, e.ItemID),
			}
			row.marker, row.markerStyle, row.locked, row.selected, row.subText = statusForEntry(e)
			m.rows = append(m.rows, row)
		}
	}

	// Put cursor on the first item row (skip initial header).
	for i, r := range m.rows {
		if !r.isHeader {
			m.cursor = i
			break
		}
	}
	return m
}

func statusForEntry(e runner.PlanEntry) (marker string, style lipgloss.Style, locked bool, selected bool, sub string) {
	switch {
	case e.Skipped:
		return GlyphSkipped, StyleSkipped, true, false,
			DimStyle.Render("skipped: When gate excluded this item")
	case e.NoProvider:
		return "?", StyleNotInstalled, true, false,
			DimStyle.Render("skipped: no registered provider for any of its kinds")
	case e.Drift:
		v := firstNonEmpty(e.State.Version, "present")
		return GlyphDrift, StyleDrift, false, false,
			DimStyle.Render("installed via " + e.State.ProviderUsed +
				"; manifest prefers " + e.ProviderKind + " (" + v + ")")
	case e.State.Installed && e.TrackedByUs:
		v := firstNonEmpty(e.State.Version, e.TrackedByUsVer, "present")
		return GlyphInstalledByUs, StyleInstalled, false, true,
			DimStyle.Render(e.ProviderKind + " · " + v + " · installed by onboardctl")
	case e.State.Installed:
		v := firstNonEmpty(e.State.Version, "present")
		return GlyphExternal, StyleExternal, false, true,
			DimStyle.Render(e.ProviderKind + " · " + v + " · already installed")
	default:
		return GlyphNotInstalled, StyleNotInstalled, false, true,
			DimStyle.Render(e.ProviderKind + " · to install")
	}
}

// Init implements tea.Model.
func (m ReviewModel) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m ReviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Reserve 8 rows for header, counts, and help; remaining is list.
		h := msg.Height - 8
		if h > 5 {
			m.viewHeight = h
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.choice.Quit = true
			m.quitting = true
			return m, tea.Quit
		case "esc":
			m.choice.BackToPicker = true
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			m.cursor = m.prevItem(m.cursor)
			m.ensureVisible()
		case "down", "j":
			m.cursor = m.nextItem(m.cursor)
			m.ensureVisible()
		case "home", "g":
			for i, r := range m.rows {
				if !r.isHeader {
					m.cursor = i
					break
				}
			}
			m.viewTop = 0
		case "end", "G":
			for i := len(m.rows) - 1; i >= 0; i-- {
				if !m.rows[i].isHeader {
					m.cursor = i
					break
				}
			}
			m.ensureVisible()
		case " ":
			if m.cursor < len(m.rows) && !m.rows[m.cursor].isHeader && !m.rows[m.cursor].locked {
				m.rows[m.cursor].selected = !m.rows[m.cursor].selected
			}
		case "a":
			for i := range m.rows {
				if !m.rows[i].isHeader && !m.rows[i].locked {
					m.rows[i].selected = true
				}
			}
		case "n":
			for i := range m.rows {
				if !m.rows[i].isHeader && !m.rows[i].locked {
					m.rows[i].selected = false
				}
			}
		case "i":
			for i := range m.rows {
				if !m.rows[i].isHeader && !m.rows[i].locked {
					m.rows[i].selected = !m.rows[i].selected
				}
			}
		case "enter":
			m.choice.Confirmed = true
			for _, r := range m.rows {
				if r.isHeader || r.locked {
					continue
				}
				if r.selected {
					m.choice.ItemIDs = append(m.choice.ItemIDs, r.itemID)
				}
			}
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, nil
}

// View implements tea.Model.
func (m ReviewModel) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder
	b.WriteString(TitleStyle.Render(fmt.Sprintf("Review — %s", m.profileName)))
	b.WriteString("\n")
	b.WriteString(m.renderCounts())
	b.WriteString("\n\n")

	// Clamp the visible window.
	start, end := m.viewTop, m.viewTop+m.viewHeight
	if end > len(m.rows) {
		end = len(m.rows)
	}
	for i := start; i < end; i++ {
		b.WriteString(m.renderRow(i))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(HelpStyle.Render(
		"↑/↓ navigate · space toggle · a all · n none · i invert · enter confirm · esc back · q quit"))
	return b.String()
}

func (m ReviewModel) renderCounts() string {
	selected := 0
	for _, r := range m.rows {
		if !r.isHeader && !r.locked && r.selected {
			selected++
		}
	}
	c := m.counts
	parts := []string{
		BadgeStyle.Background(ColMauve).Render(fmt.Sprintf("%d selected", selected)),
		StyleInstalled.Render(fmt.Sprintf("%s %d ours", GlyphInstalledByUs, c.InstalledByUs)),
		StyleExternal.Render(fmt.Sprintf("%s %d external", GlyphExternal, c.InstalledExternal)),
		StyleNotInstalled.Render(fmt.Sprintf("%s %d to install", GlyphNotInstalled, c.NotInstalled)),
	}
	if c.Drift > 0 {
		parts = append(parts, StyleDrift.Render(fmt.Sprintf("%s %d drift", GlyphDrift, c.Drift)))
	}
	if c.Skipped > 0 {
		parts = append(parts, StyleSkipped.Render(fmt.Sprintf("%s %d skipped", GlyphSkipped, c.Skipped)))
	}
	if c.NoProvider > 0 {
		parts = append(parts, StyleNotInstalled.Render(fmt.Sprintf("? %d no-provider", c.NoProvider)))
	}
	return strings.Join(parts, "  ")
}

func (m ReviewModel) renderRow(i int) string {
	r := m.rows[i]
	if r.isHeader {
		title := TitleStyle.MarginBottom(0).Render("# " + r.bundleName)
		divider := DimStyle.Render(strings.Repeat("─", maxInt(0, 40-len(r.bundleName))))
		return title + " " + divider
	}

	cursor := "  "
	if i == m.cursor {
		cursor = CursorStyle.Render("❯ ")
	}
	box := "[ ]"
	if r.selected {
		box = "[x]"
	}
	if r.locked {
		box = "[-]"
	}
	marker := r.markerStyle.Render(r.marker)
	name := r.displayName
	if i == m.cursor {
		name = SelectedStyle.Render(name)
	} else {
		name = NormalStyle.Render(name)
	}
	line := fmt.Sprintf("%s%s %s %-24s", cursor, box, marker, name)
	if r.subText != "" {
		line += "  " + r.subText
	}
	return line
}

// prevItem returns the index of the previous non-header row, or cur if
// we're already at the first item.
func (m ReviewModel) prevItem(cur int) int {
	for i := cur - 1; i >= 0; i-- {
		if !m.rows[i].isHeader {
			return i
		}
	}
	return cur
}

// nextItem returns the index of the next non-header row, or cur if
// we're at the last item.
func (m ReviewModel) nextItem(cur int) int {
	for i := cur + 1; i < len(m.rows); i++ {
		if !m.rows[i].isHeader {
			return i
		}
	}
	return cur
}

// ensureVisible scrolls the viewport so m.cursor is on screen.
func (m *ReviewModel) ensureVisible() {
	if m.cursor < m.viewTop {
		m.viewTop = m.cursor
	}
	if m.cursor >= m.viewTop+m.viewHeight {
		m.viewTop = m.cursor - m.viewHeight + 1
	}
	if m.viewTop < 0 {
		m.viewTop = 0
	}
}

// Choice returns the final choice (zero value if user cancelled via q/ctrl-c).
func (m ReviewModel) Choice() ReviewChoice { return m.choice }

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
