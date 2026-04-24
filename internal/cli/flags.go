package cli

import (
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Package-level state for global flags.
var (
	verboseFlag       bool
	noColorFlag       bool
	noUpdateCheckFlag bool
)

// UpdateCheckEnabled reports whether the upstream release-check is
// allowed to run this invocation. It returns false when either the
// --no-update-check flag is set or the ONBOARDCTL_NO_UPDATE_CHECK
// env var is non-empty.
func UpdateCheckEnabled() bool {
	if noUpdateCheckFlag {
		return false
	}
	if os.Getenv("ONBOARDCTL_NO_UPDATE_CHECK") != "" {
		return false
	}
	return true
}

// Verbose reports whether --verbose / -v was passed. Subcommands consult
// this to dial up log detail without threading flags through every helper.
func Verbose() bool { return verboseFlag }

// ColorEnabled reports whether ANSI colour output should be rendered.
// It is false when any of these apply:
//   - --no-color was passed
//   - NO_COLOR env var is set to anything non-empty
//     (see https://no-color.org)
//
// Call this at render time, not at package init — the root command's
// PersistentPreRun configures flag state after the package has loaded.
func ColorEnabled() bool {
	if noColorFlag {
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return true
}

// configureColors applies the current colour policy to libraries that
// cache a renderer (lipgloss). Call from the root command's
// PersistentPreRun so every subcommand sees a consistent setting.
func configureColors() {
	if ColorEnabled() {
		// Leave lipgloss with its auto-detected profile.
		return
	}
	// Force ASCII (no colour, no styling). This affects every lipgloss
	// Style rendered after this call — including those in TUI models.
	lipgloss.DefaultRenderer().SetColorProfile(termenv.Ascii)
}
