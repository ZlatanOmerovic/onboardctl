package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
)

var lintCmd = &cobra.Command{
	Use:   "lint [path]",
	Short: "Validate a manifest or extras YAML against the bundled JSON Schema",
	Long: `lint validates a manifest or extras file against onboardctl's bundled
JSON Schema. With no arguments, it targets the default user extras location
($XDG_CONFIG_HOME/onboardctl/extras.yaml).`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var path string
		if len(args) > 0 {
			path = args[0]
		}
		if err := manifest.Lint(path); err != nil {
			return err
		}
		target := path
		if target == "" {
			target = manifest.DefaultExtrasPath()
		}
		fmt.Fprintf(cmd.OutOrStdout(), "OK: %s is valid (schema v%d)\n", target, manifest.SchemaVersion)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(lintCmd)
}
