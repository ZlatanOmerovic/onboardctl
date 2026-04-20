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
	apply  bool // true = actually install; false = dry-run (the default)
}

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Interactively pick a profile, review its plan, and optionally apply",
	Long: `profile launches a two-screen Bubble Tea wizard:

  1. Profile picker — pick essentials, fullstack-web, devops, polyglot-dev,
     everything, or any custom profile from your extras.yaml.

  2. Review — grouped by bundle, each item shows a status marker
     (✓ installed-by-us · ● external · ∅ to-install · - skipped) and can
     be toggled. Confirm with Enter.

After confirm, the plan is either executed (with --apply, needs sudo) or
printed as a dry-run summary.`,
	RunE: runProfile,
}

func init() {
	profileCmd.Flags().StringVar(&profileOpts.extras, "extras", "", "path to user extras YAML (default: XDG)")
	profileCmd.Flags().BoolVar(&profileOpts.apply, "apply", false, "apply the plan instead of dry-run (requires sudo)")
	rootCmd.AddCommand(profileCmd)
}

func runProfile(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()

	m, err := manifest.Load(profileOpts.extras)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}

	// Env + state + registry — needed up-front so we can Plan before review.
	d, err := system.DetectDistro()
	if err != nil {
		return fmt.Errorf("detect distro: %w", err)
	}
	if !d.InDebianFamily() {
		return fmt.Errorf("unsupported distro %q (Debian family only)", d.ID)
	}

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

	// Loop picker ↔ review until user confirms or quits.
	for {
		choice, err := tui.RunProfilePicker(context.Background(), m, os.Stderr, os.Stderr)
		if err != nil {
			return err
		}
		if !choice.Picked {
			fmt.Fprintln(out, "No profile selected.")
			return nil
		}

		// Compute the plan for that profile.
		env := runner.Env{Distro: d, Desktop: system.DetectDesktop()}
		r := &runner.Runner{
			Manifest: m,
			Registry: reg,
			State:    st,
			Env:      env,
			Out:      out,
		}
		plan, err := r.Plan(context.Background(), runner.Selection{Profile: choice.ID})
		if err != nil {
			return fmt.Errorf("plan: %w", err)
		}

		// Review screen.
		reviewChoice, err := tui.RunItemReview(context.Background(), choice.Name, choice.ID, plan, os.Stderr, os.Stderr)
		if err != nil {
			return err
		}
		if reviewChoice.Quit {
			fmt.Fprintln(out, "Cancelled.")
			return nil
		}
		if reviewChoice.BackToPicker {
			continue
		}
		if !reviewChoice.Confirmed {
			return nil
		}

		// User confirmed: run as dry-run or apply.
		return executeReview(out, m, reg, st, d, env, choice, reviewChoice)
	}
}

func executeReview(
	out interface{ Write([]byte) (int, error) },
	m *manifest.Manifest,
	reg provider.Registry,
	st *state.State,
	d system.Distro,
	env runner.Env,
	choice tui.ProfileChoice,
	rc tui.ReviewChoice,
) error {
	apply := profileOpts.apply
	if apply && os.Geteuid() != 0 {
		return fmt.Errorf("--apply needs root — re-run with sudo")
	}

	// Pre-fill distro snapshot in state.
	st.Distro = state.DistroSnapshot{
		ID: d.ID, Codename: d.Codename, Version: d.Version, Family: d.Family, Arch: d.Arch,
	}
	st.Profile = choice.ID

	var bootstrapper *runner.RepoBootstrapper
	if apply {
		bootstrapper = runner.NewRepoBootstrapper(m.Repos, provider.ExecRunner(), d)
	}

	r := &runner.Runner{
		Manifest:     m,
		Registry:     reg,
		State:        st,
		Bootstrapper: bootstrapper,
		Env:          env,
		Out:          out,
		StateFn: func(s *state.State) error {
			return state.Save("", s)
		},
	}

	fmt.Fprintf(out, "\n%s: %d item(s) selected\n", modeString(apply), len(rc.ItemIDs))
	fmt.Fprintln(out)

	sum, err := r.Run(context.Background(), runner.Selection{Items: rc.ItemIDs}, runner.Options{
		DryRun:  !apply,
		Profile: choice.ID,
	})
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "\nSummary")
	fmt.Fprintf(out, "  Planned:       %d\n", len(sum.Selected))
	if sum.DryRun {
		fmt.Fprintf(out, "  Would install: %d\n", len(sum.Installed))
	} else {
		fmt.Fprintf(out, "  Installed:     %d\n", len(sum.Installed))
	}
	fmt.Fprintf(out, "  Already had:   %d\n", len(sum.AlreadyHad))
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

func modeString(apply bool) string {
	if apply {
		return "Applying"
	}
	return "Dry-run"
}
