package system

import (
	"os"
	"strings"
)

// Desktop is the user-facing desktop environment we detected.
// It is the predicate that gates DE-specific manifest actions
// (dark-mode commands, HiDPI scaling, etc.).
type Desktop string

// Known desktops. Anything else collapses to DesktopUnknown.
const (
	DesktopGNOME    Desktop = "gnome"
	DesktopKDE      Desktop = "kde"
	DesktopXfce     Desktop = "xfce"
	DesktopCinnamon Desktop = "cinnamon"
	DesktopMATE     Desktop = "mate"
	DesktopBudgie   Desktop = "budgie"
	DesktopLXQt     Desktop = "lxqt"
	DesktopPantheon Desktop = "pantheon" // Elementary
	DesktopSway     Desktop = "sway"
	DesktopHyprland Desktop = "hyprland"
	DesktopUnknown  Desktop = ""
)

// DetectDesktop inspects XDG_CURRENT_DESKTOP and XDG_SESSION_DESKTOP.
// XDG_CURRENT_DESKTOP may contain colon-separated tokens (e.g. "ubuntu:GNOME");
// any token is enough to pin the desktop.
func DetectDesktop() Desktop {
	candidates := []string{
		os.Getenv("XDG_CURRENT_DESKTOP"),
		os.Getenv("XDG_SESSION_DESKTOP"),
		os.Getenv("DESKTOP_SESSION"),
	}
	for _, c := range candidates {
		if d := classifyDesktopTokens(c); d != DesktopUnknown {
			return d
		}
	}
	return DesktopUnknown
}

func classifyDesktopTokens(s string) Desktop {
	s = strings.ToLower(s)
	for _, tok := range strings.FieldsFunc(s, func(r rune) bool { return r == ':' || r == ' ' }) {
		switch {
		case strings.Contains(tok, "gnome"), strings.Contains(tok, "unity"):
			return DesktopGNOME
		case strings.Contains(tok, "kde"), strings.Contains(tok, "plasma"):
			return DesktopKDE
		case strings.Contains(tok, "xfce"):
			return DesktopXfce
		case strings.Contains(tok, "cinnamon"):
			return DesktopCinnamon
		case strings.Contains(tok, "mate"):
			return DesktopMATE
		case strings.Contains(tok, "budgie"):
			return DesktopBudgie
		case strings.Contains(tok, "lxqt"):
			return DesktopLXQt
		case strings.Contains(tok, "pantheon"):
			return DesktopPantheon
		case strings.Contains(tok, "sway"):
			return DesktopSway
		case strings.Contains(tok, "hyprland"):
			return DesktopHyprland
		}
	}
	return DesktopUnknown
}

// String implements fmt.Stringer.
func (d Desktop) String() string {
	if d == DesktopUnknown {
		return "unknown"
	}
	return string(d)
}
