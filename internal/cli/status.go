package cli

import (
	"context"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	"github.com/ZlatanOmerovic/onboardctl/internal/provider"
	"github.com/ZlatanOmerovic/onboardctl/internal/runner"
	"github.com/ZlatanOmerovic/onboardctl/internal/state"
	"github.com/ZlatanOmerovic/onboardctl/internal/system"
)

var statusOpts struct {
	extrasPath string
	verbose    bool
	plan       string
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
	statusCmd.Flags().StringVar(&statusOpts.plan, "plan", "", "print the per-item plan for a profile (non-interactive; useful in scripts)")
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()

	d, err := system.DetectDistro()
	if err != nil {
		return fmt.Errorf("detect distro: %w", err)
	}
	de := system.DetectDesktop()

	// --plan shortcut: skip the environment summary (the user wants the
	// per-item plan, not the machine fingerprint) and print a flat
	// table of each resolved item with its current status marker.
	if statusOpts.plan != "" {
		return runStatusPlan(out, d, de, statusOpts.extrasPath, statusOpts.plan)
	}

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

// runStatusPlan prints a non-interactive, text-only version of the review
// TUI: for each item in the resolved profile, one line with a status
// marker, the item ID, the preferred provider kind, and a short note.
func runStatusPlan(out io.Writer, d system.Distro, de system.Desktop, extras, profileID string) error {
	m, err := manifest.Load(extras)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}
	if _, ok := m.Profiles[profileID]; !ok {
		return fmt.Errorf("profile %q not found (have: %v)", profileID, sortedKeys(m.Profiles))
	}

	st, err := state.Load("")
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	reg := provider.NewRegistry()
	reg.Register(provider.NewAPT())
	reg.Register(provider.NewShell())
	reg.Register(provider.NewConfig())
	reg.Register(provider.NewBinaryRelease())
	reg.Register(provider.NewComposerGlobal())
	reg.Register(provider.NewFlatpak())
	reg.Register(provider.NewNPMGlobal())

	r := &runner.Runner{
		Manifest: m,
		Registry: reg,
		State:    st,
		Env:      runner.Env{Distro: d, Desktop: de},
	}
	plan, err := r.Plan(context.Background(), runner.Selection{Profile: profileID})
	if err != nil {
		return fmt.Errorf("plan: %w", err)
	}

	fmt.Fprintf(out, "Plan for profile %q (%d items)\n\n", profileID, len(plan.Entries))
	for _, e := range plan.Entries {
		marker, note := planMarker(e)
		name := e.ItemID
		if e.Item.Name != "" {
			name = e.Item.Name
		}
		fmt.Fprintf(out, "  %s %-24s %s\n", marker, name, note)
	}

	c := plan.Counts()
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Summary: %d ours · %d external · %d drift · %d to-install · %d skipped · %d no-provider\n",
		c.InstalledByUs, c.InstalledExternal, c.Drift, c.NotInstalled, c.Skipped, c.NoProvider)
	return nil
}

// planMarker returns a glyph + short note for one plan entry. Mirrors
// the TUI's statusForEntry but renders as plain text with ansi colors
// only when ColorEnabled().
func planMarker(e runner.PlanEntry) (marker, note string) {
	switch {
	case e.Skipped:
		return colorise("-", ansiDim), "skipped by When gate"
	case e.NoProvider:
		return colorise("?", ansiDim), "no registered provider"
	case e.Drift:
		v := firstNonBlank(e.State.Version, "present")
		return colorise("⚠", ansiYellow), fmt.Sprintf("drift: via %s; manifest prefers %s (%s)",
			e.State.ProviderUsed, e.ProviderKind, v)
	case e.State.Installed && e.TrackedByUs:
		v := firstNonBlank(e.State.Version, e.TrackedByUsVer, "present")
		return colorise("✓", ansiGreen), fmt.Sprintf("installed by onboardctl (%s · %s)", e.ProviderKind, v)
	case e.State.Installed:
		v := firstNonBlank(e.State.Version, "present")
		return colorise("●", ansiGreen), fmt.Sprintf("installed externally (%s · %s)", e.ProviderKind, v)
	default:
		return colorise("∅", ansiDim), fmt.Sprintf("to install via %s", e.ProviderKind)
	}
}
