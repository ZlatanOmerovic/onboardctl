package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	"github.com/ZlatanOmerovic/onboardctl/internal/provider"
	"github.com/ZlatanOmerovic/onboardctl/internal/runner"
	"github.com/ZlatanOmerovic/onboardctl/internal/state"
	"github.com/ZlatanOmerovic/onboardctl/internal/system"
)

var installOpts struct {
	extras            string
	profile           string
	bundle            string
	items             []string
	fromExport        string
	skip              []string
	dryRun            bool
	assumeYes         bool
	swapDrift         bool
	rollbackOnFailure bool
	offline           bool
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install items, bundles, or a whole profile (headless)",
	Long: `install resolves a selection (profile, bundle, or explicit item list)
to a concrete install plan and dispatches it through the provider registry.

Exactly one of --profile, --bundle, or --items is required.

Dry-run by default when no selection mutation is requested; pass --yes to
actually apply changes. Safety net, not a strict contract — once the TUI
in Phase 3 lands, interactive confirmation replaces this flag.

Examples:
  onboardctl install --profile essentials --dry-run
  onboardctl install --bundle base-system --yes
  onboardctl install --items jq,vlc --skip vlc --yes`,
	RunE: runInstall,
}

func init() {
	installCmd.Flags().StringVar(&installOpts.extras, "extras", "", "path to user extras YAML (default: XDG)")
	installCmd.Flags().StringVar(&installOpts.profile, "profile", "", "profile to install (e.g. essentials, fullstack-web)")
	installCmd.Flags().StringVar(&installOpts.bundle, "bundle", "", "single bundle to install")
	installCmd.Flags().StringSliceVar(&installOpts.items, "items", nil, "explicit item IDs (comma-separated)")
	installCmd.Flags().StringVar(&installOpts.fromExport, "from-export", "", "replay items from a file produced by 'onboardctl export'")
	installCmd.Flags().StringSliceVar(&installOpts.skip, "skip", nil, "item IDs to omit from the plan")
	installCmd.Flags().BoolVar(&installOpts.dryRun, "dry-run", false, "print the plan; don't install")
	installCmd.Flags().BoolVarP(&installOpts.assumeYes, "yes", "y", false, "apply changes (without --yes and without --dry-run, install defaults to dry-run)")
	installCmd.Flags().BoolVar(&installOpts.swapDrift, "swap-drift", false, "replace foreign-installed versions (e.g. snap) with the manifest-preferred one")
	installCmd.Flags().BoolVar(&installOpts.rollbackOnFailure, "rollback-on-failure", false, "on first item failure, undo this run's successful installs")
	installCmd.Flags().BoolVar(&installOpts.offline, "offline", false, "refuse items whose provider touches the network; useful for air-gapped re-runs")
	rootCmd.AddCommand(installCmd)
}

func runInstall(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()

	// Exactly one selection source.
	n := 0
	if installOpts.profile != "" {
		n++
	}
	if installOpts.bundle != "" {
		n++
	}
	if len(installOpts.items) > 0 {
		n++
	}
	if installOpts.fromExport != "" {
		n++
	}
	if n == 0 {
		return errors.New("must pass one of --profile / --bundle / --items / --from-export")
	}
	if n > 1 {
		return errors.New("pass only one of --profile / --bundle / --items / --from-export")
	}

	// --from-export populates items by parsing the file.
	if installOpts.fromExport != "" {
		ids, err := parseExportFile(installOpts.fromExport)
		if err != nil {
			return fmt.Errorf("read %s: %w", installOpts.fromExport, err)
		}
		installOpts.items = ids
	}

	// Safety net: if neither --dry-run nor --yes is set, default to dry-run.
	effectiveDry := installOpts.dryRun || !installOpts.assumeYes
	if !installOpts.dryRun && !installOpts.assumeYes {
		fmt.Fprintln(out, "Note: neither --yes nor --dry-run passed; defaulting to --dry-run.")
		fmt.Fprintln(out, "      Re-run with --yes to apply the plan.")
	}

	// Load manifest.
	m, err := manifest.Load(installOpts.extras)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}

	// Build the selection.
	sel := runner.Selection{
		Profile: installOpts.profile,
		Bundle:  installOpts.bundle,
		Items:   installOpts.items,
		Skip:    installOpts.skip,
	}

	// Distro snapshot for state file.
	d, err := system.DetectDistro()
	if err != nil {
		return fmt.Errorf("detect distro: %w", err)
	}
	if !d.InDebianFamily() {
		return fmt.Errorf("unsupported distro %q (onboardctl targets the Debian family only)", d.ID)
	}

	// Load (or create) state.
	st, err := state.Load("")
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	st.Distro = state.DistroSnapshot{
		ID: d.ID, Codename: d.Codename, Version: d.Version, Family: d.Family, Arch: d.Arch,
	}
	if installOpts.profile != "" {
		st.Profile = installOpts.profile
	}

	// Root check: apply mode needs root (apt-get install writes /var/lib/dpkg,
	// /usr/local/bin, /etc/apt/...). Skipped on dry-run because those paths
	// aren't touched.
	if !effectiveDry && os.Geteuid() != 0 {
		return errors.New("apply mode needs root privileges — re-run with sudo (or pass --dry-run to preview)")
	}

	// Provider registry.
	reg := provider.NewRegistry()
	reg.Register(provider.NewAPT())
	reg.Register(provider.NewShell())
	reg.Register(provider.NewConfig())
	reg.Register(provider.NewBinaryRelease())
	reg.Register(provider.NewComposerGlobal())
	reg.Register(provider.NewFlatpak())
	reg.Register(provider.NewNPMGlobal())

	// Repo bootstrapper: only used in apply mode since it writes to /etc/apt.
	var bootstrapper *runner.RepoBootstrapper
	if !effectiveDry {
		bootstrapper = runner.NewRepoBootstrapper(m.Repos, provider.ExecRunner(), d)
		bootstrapper.Out = out
	}

	r := &runner.Runner{
		Manifest:     m,
		Registry:     reg,
		State:        st,
		Bootstrapper: bootstrapper,
		Env:          runner.Env{Distro: d, Desktop: system.DetectDesktop()},
		Out:          out,
		StateFn: func(s *state.State) error {
			return state.Save("", s)
		},
	}

	opts := runner.Options{
		DryRun:            effectiveDry,
		Profile:           installOpts.profile,
		SwapDrift:         installOpts.swapDrift,
		RollbackOnFailure: installOpts.rollbackOnFailure,
		Offline:           installOpts.offline,
	}

	fmt.Fprintln(out, "Plan:")
	sum, err := r.Run(context.Background(), sel, opts)
	if err != nil {
		return err
	}

	fmt.Fprintln(out)
	printSummary(out, sum, reg)
	if len(sum.Failed) > 0 {
		return fmt.Errorf("%d item(s) failed", len(sum.Failed))
	}
	return nil
}

func printSummary(w interface{ Write([]byte) (int, error) }, s *runner.Summary, reg provider.Registry) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Summary")
	fmt.Fprintf(w, "  Planned:       %d\n", len(s.Selected))
	if s.DryRun {
		fmt.Fprintf(w, "  Would install: %d\n", len(s.Installed))
	} else {
		fmt.Fprintf(w, "  Installed:     %d\n", len(s.Installed))
	}
	fmt.Fprintf(w, "  Already had:   %d\n", len(s.AlreadyHad))
	fmt.Fprintf(w, "  Failed:        %d\n", len(s.Failed))

	if len(s.Failed) > 0 {
		fmt.Fprintln(w, "\nFailures:")
		keys := make([]string, 0, len(s.Failed))
		for k := range s.Failed {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(w, "  - %s: %s\n", k, s.Failed[k])
		}
	}

	// Note when items are planned but no provider is registered for their kind.
	missing := map[string]struct{}{}
	_ = missing
	_ = reg
}
