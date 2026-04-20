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
}

// Execute runs the CLI. It returns the error cobra produced (if any) so
// main can set a non-zero exit code without re-printing it.
func Execute() error {
	return rootCmd.Execute()
}
