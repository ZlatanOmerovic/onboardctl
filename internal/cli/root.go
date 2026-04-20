// Package cli wires the cobra command tree for onboardctl.
//
// Subcommands register themselves via init() in their own files so that
// the root command here stays small and the file layout mirrors the CLI.
package cli

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:   "onboardctl",
	Short: "Provision a Debian-based workstation with profiles, bundles, and themes.",
	Long: `onboardctl is an interactive post-install provisioning tool for
Debian-family Linux distributions (Debian, Ubuntu, Mint, Pop!_OS, Elementary,
MX, Kali, and derivatives).

It organises apt packages, Flatpaks, binary releases, and system-config
commands into bundles; exposes opinionated profile presets (essentials,
fullstack-web, devops, polyglot-dev, everything); and tracks state across
runs so re-running is a diff, not a reinstall.`,
	SilenceUsage:  true,
	SilenceErrors: false,
	// PersistentPreRun fires before any subcommand's RunE. It converts
	// the declared --verbose / --no-color flags into process-wide state
	// (lipgloss colour profile, package-level getters).
	PersistentPreRun: func(cmd *cobra.Command, _ []string) {
		configureColors()
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verboseFlag, "verbose", "v", false, "print extra progress detail where applicable")
	rootCmd.PersistentFlags().BoolVar(&noColorFlag, "no-color", false, "disable ANSI colour output (honoured in addition to NO_COLOR env)")
}

// Execute runs the CLI. It returns the error cobra produced (if any) so
// main can set a non-zero exit code without re-printing it.
func Execute() error {
	return rootCmd.Execute()
}

// Root returns the configured cobra root command. It is exposed for
// out-of-process consumers that need the command tree — today that's
// cmd/gen, which generates shell completions and manpages at release
// time. Not intended for library use; the returned *cobra.Command is
// shared state.
func Root() *cobra.Command {
	return rootCmd
}
