package system

import "testing"

func TestClassifyDesktopTokens(t *testing.T) {
	cases := map[string]Desktop{
		"":                     DesktopUnknown,
		"ubuntu:GNOME":         DesktopGNOME,
		"X-Cinnamon":           DesktopCinnamon,
		"KDE":                  DesktopKDE,
		"plasma":               DesktopKDE,
		"XFCE":                 DesktopXfce,
		"MATE":                 DesktopMATE,
		"Budgie:GNOME":         DesktopBudgie,
		"LXQt":                 DesktopLXQt,
		"Pantheon":             DesktopPantheon,
		"sway":                 DesktopSway,
		"Hyprland":             DesktopHyprland,
		"unity":                DesktopGNOME,
		"ubuntu:something-odd": DesktopUnknown,
	}
	for input, want := range cases {
		got := classifyDesktopTokens(input)
		if got != want {
			t.Errorf("classifyDesktopTokens(%q) = %q, want %q", input, got, want)
		}
	}
}
