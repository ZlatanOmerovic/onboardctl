package cli

import (
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	"github.com/ZlatanOmerovic/onboardctl/internal/system"
)

var statusOpts struct {
	extrasPath string
	verbose    bool
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show detected environment and manifest summary",
	Long: `status prints a concise snapshot of the current machine and the
loaded manifest. It does not install or modify anything.

When --extras is omitted, $XDG_CONFIG_HOME/onboardctl/extras.yaml
(or ~/.config/onboardctl/extras.yaml) is loaded if present.`,
	RunE: runStatus,
}

func init() {
	statusCmd.Flags().StringVar(&statusOpts.extrasPath, "extras", "", "path to user extras YAML (default: XDG extras location)")
	statusCmd.Flags().BoolVarP(&statusOpts.verbose, "verbose", "v", false, "print item lists per bundle")
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()

	d, err := system.DetectDistro()
	if err != nil {
		return fmt.Errorf("detect distro: %w", err)
	}
	de := system.DetectDesktop()

	fmt.Fprintln(out, "Environment")
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  Distro\t%s %s (%s)\n", d.Name, d.Version, d.Codename)
	fmt.Fprintf(tw, "  ID\t%s\n", d.ID)
	fmt.Fprintf(tw, "  Family\t%s\n", d.Family)
	if !d.InDebianFamily() {
		fmt.Fprintf(tw, "  Supported\t%s (Debian-family only)\n", colorise("no", ansiRed))
	} else {
		fmt.Fprintf(tw, "  Supported\t%s\n", colorise("yes", ansiGreen))
	}
	fmt.Fprintf(tw, "  Arch\t%s\n", d.Arch)
	fmt.Fprintf(tw, "  Desktop\t%s\n", de.String())
	tw.Flush()

	m, err := manifest.Load(statusOpts.extrasPath)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}

	extras := statusOpts.extrasPath
	if extras == "" {
		extras = manifest.DefaultExtrasPath()
	}

	fmt.Fprintln(out)
	fmt.Fprintf(out, "Manifest v%d\n", m.Version)
	tw = tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  Profiles\t%d\n", len(m.Profiles))
	fmt.Fprintf(tw, "  Bundles\t%d\n", len(m.Bundles))
	fmt.Fprintf(tw, "  Items\t%d\n", len(m.Items))
	fmt.Fprintf(tw, "  Repos\t%d\n", len(m.Repos))
	fmt.Fprintf(tw, "  Extras\t%s\n", extras)
	tw.Flush()

	if statusOpts.verbose {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Profiles")
		for _, name := range sortedKeys(m.Profiles) {
			p := m.Profiles[name]
			if p.Extends != "" {
				fmt.Fprintf(out, "  %s (extends %s): %v\n", name, p.Extends, p.Bundles)
			} else {
				fmt.Fprintf(out, "  %s: %v\n", name, p.Bundles)
			}
		}

		fmt.Fprintln(out)
		fmt.Fprintln(out, "Bundles")
		for _, name := range sortedKeys(m.Bundles) {
			b := m.Bundles[name]
			fmt.Fprintf(out, "  %s (%d): %v\n", name, len(b.Items), b.Items)
		}
	}

	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
