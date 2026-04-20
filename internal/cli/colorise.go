package cli

// colorise wraps s in the given ANSI colour if colour output is enabled,
// and returns s unchanged otherwise. Use this for small one-shot colouring
// in headless commands; for anything richer, reach for lipgloss.
func colorise(s, color string) string {
	if !ColorEnabled() {
		return s
	}
	return color + s + ansiReset
}
