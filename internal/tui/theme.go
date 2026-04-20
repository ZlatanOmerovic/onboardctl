// Package tui houses the Bubble Tea models and styles that power
// onboardctl's interactive commands.
package tui

import "github.com/charmbracelet/lipgloss"

// Catppuccin Mocha-flavoured palette so the tool feels of-a-piece with
// the terminal stack onboardctl installs.
var (
	ColBase     = lipgloss.Color("#1e1e2e")
	ColMantle   = lipgloss.Color("#181825")
	ColCrust    = lipgloss.Color("#11111b")
	ColSurface0 = lipgloss.Color("#313244")
	ColSurface1 = lipgloss.Color("#45475a")
	ColText     = lipgloss.Color("#cdd6f4")
	ColSubtext0 = lipgloss.Color("#a6adc8")
	ColOverlay1 = lipgloss.Color("#7f849c")
	ColMauve    = lipgloss.Color("#cba6f7")
	ColBlue     = lipgloss.Color("#89b4fa")
	ColGreen    = lipgloss.Color("#a6e3a1")
	ColYellow   = lipgloss.Color("#f9e2af")
	ColPeach    = lipgloss.Color("#fab387")
	ColRed      = lipgloss.Color("#f38ba8")
	ColTeal     = lipgloss.Color("#94e2d5")
)

// Styles used across screens.
var (
	TitleStyle       = lipgloss.NewStyle().Bold(true).Foreground(ColMauve).MarginBottom(1)
	DescriptionStyle = lipgloss.NewStyle().Foreground(ColSubtext0)
	CursorStyle      = lipgloss.NewStyle().Foreground(ColMauve).Bold(true)
	SelectedStyle    = lipgloss.NewStyle().Foreground(ColText).Bold(true)
	NormalStyle      = lipgloss.NewStyle().Foreground(ColText)
	DimStyle         = lipgloss.NewStyle().Foreground(ColOverlay1)
	HelpStyle        = lipgloss.NewStyle().Foreground(ColSubtext0).MarginTop(1)
	ErrorStyle       = lipgloss.NewStyle().Foreground(ColRed).Bold(true)
	BadgeStyle       = lipgloss.NewStyle().Foreground(ColCrust).Background(ColMauve).
				Padding(0, 1).MarginRight(1)
)

// Status-marker glyphs for the four install states we care about.
const (
	GlyphInstalledByUs = "✓" // installed, and our state.yaml recorded it
	GlyphExternal      = "●" // installed, but not by us
	GlyphDrift         = "⚠" // installed via a different provider than the manifest prefers
	GlyphNotInstalled  = "∅" // available, not installed
	GlyphSkipped       = "-" // When gate excluded it on this machine
)

// Styles per glyph so callers can render them without rebuilding a lipgloss
// style each time.
var (
	StyleInstalled    = lipgloss.NewStyle().Foreground(ColGreen).Bold(true)
	StyleExternal     = lipgloss.NewStyle().Foreground(ColBlue)
	StyleDrift        = lipgloss.NewStyle().Foreground(ColYellow).Bold(true)
	StyleNotInstalled = lipgloss.NewStyle().Foreground(ColOverlay1)
	StyleSkipped      = lipgloss.NewStyle().Foreground(ColOverlay1)
)
