package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// Build-time metadata, injected via -ldflags from the Makefile.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version, commit, and build information",
	Run: func(cmd *cobra.Command, _ []string) {
		out := cmd.OutOrStdout()
		fmt.Fprintf(out,
			"onboardctl %s\n  commit:  %s\n  built:   %s\n  go:      %s\n  os/arch: %s/%s\n",
			Version, Commit, BuildDate, runtime.Version(), runtime.GOOS, runtime.GOARCH,
		)
		maybePrintUpdateNotice(out)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
