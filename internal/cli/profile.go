package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	"github.com/ZlatanOmerovic/onboardctl/internal/provider"
	"github.com/ZlatanOmerovic/onboardctl/internal/runner"
	"github.com/ZlatanOmerovic/onboardctl/internal/state"
	"github.com/ZlatanOmerovic/onboardctl/internal/system"
	"github.com/ZlatanOmerovic/onboardctl/internal/tui"
	"github.com/spf13/cobra"
)

var profileOpts struct {
	extras string
	dryRun bool
}

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Interactively pick a profile and review its install plan",
	Long: `profile launches a Bubble Tea wizard: you pick a profile
(essentials / fullstack-web / devops / polyglot-dev / everything or any
user-defined profile from your extras.yaml), the wizard exits, and the
install plan for your selection is printed.

Phase 3 MVP: profile picker only. Per-item toggles, config-input forms,
and live install progress arrive in later Phase 3 increments.`,
	RunE: runProfile,
}

func init() {
	profileCmd.Flags().StringVar(&profileOpts.extras, "extras", "", "path to user extras YAML (default: XDG)")
	profileCmd.Flags().BoolVar(&profileOpts.dryRun, "dry-run", true, "dry-run by default; pass --dry-run=false to apply (also needs sudo)")
	rootCmd.AddCommand(profileCmd)
}

func runProfile(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()

	m, err := manifest.Load(profileOpts.extras)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}

	// Run the picker.
	choice, err := tui.RunProfilePicker(context.Background(), m, os.Stderr, os.Stderr)
	if err != nil {
		return err
	}
	if !choice.Picked {
		fmt.Fprintln(out, "No profile selected.")
		return nil
	}

	fmt.Fprintf(out, "\nPicked profile: %s (%s)\n\n", choice.Name, choice.ID)

	// Detect environment.
	d, err := system.DetectDistro()
	if err != nil {
		return fmt.Errorf("detect distro: %w", err)
	}
	if !d.InDebianFamily() {
		return fmt.Errorf("unsupported distro %q (Debian family only)", d.ID)
	}

	// Apply mode needs root.
	if !profileOpts.dryRun && os.Geteuid() != 0 {
		return fmt.Errorf("apply mode needs root — re-run with sudo, or keep --dry-run")
	}

	// Build the runner.
	reg := provider.NewRegistry()
	reg.Register(provider.NewAPT())
	reg.Register(provider.NewShell())
	reg.Register(provider.NewConfig())
	reg.Register(provider.NewBinaryRelease())
	reg.Register(provider.NewComposerGlobal())

	st, err := state.Load("")
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	st.Distro = state.DistroSnapshot{
		ID: d.ID, Codename: d.Codename, Version: d.Version, Family: d.Family, Arch: d.Arch,
	}
	st.Profile = choice.ID

	var bootstrapper *runner.RepoBootstrapper
	if !profileOpts.dryRun {
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

	fmt.Fprintln(out, "Plan:")
	sum, err := r.Run(context.Background(), runner.Selection{Profile: choice.ID}, runner.Options{
		DryRun:  profileOpts.dryRun,
		Profile: choice.ID,
	})
	if err != nil {
		return err
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "Summary")
	fmt.Fprintf(out, "  Planned:       %d\n", len(sum.Selected))
	if sum.DryRun {
		fmt.Fprintf(out, "  Would install: %d\n", len(sum.Installed))
	} else {
		fmt.Fprintf(out, "  Installed:     %d\n", len(sum.Installed))
	}
	fmt.Fprintf(out, "  Already had:   %d\n", len(sum.AlreadyHad))
	fmt.Fprintf(out, "  Skipped (when):%d\n", len(sum.Skipped))
	fmt.Fprintf(out, "  Failed:        %d\n", len(sum.Failed))
	if len(sum.Failed) > 0 {
		fmt.Fprintln(out, "\nFailures:")
		for id, msg := range sum.Failed {
			fmt.Fprintf(out, "  - %s: %s\n", id, msg)
		}
		return fmt.Errorf("%d item(s) failed", len(sum.Failed))
	}
	return nil
}
