package cli

import (
	"context"
	"fmt"
	"io"
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
	reg.Register(provider.NewFlatpak())
	reg.Register(provider.NewNPMGlobal())

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

		// Collect inputs for any selected item with an Input block.
		values, cancelled, err := collectInputs(m, reviewChoice.ItemIDs)
		if err != nil {
			return err
		}
		if cancelled {
			// User pressed esc in a form → bounce back to review.
			// For now we restart the whole picker loop (simpler than
			// persisting review state). Keeps the wizard predictable.
			continue
		}

		// User confirmed: run as dry-run or apply.
		return executeReview(out, m, reg, st, d, env, choice, reviewChoice, values)
	}
}

// collectInputs walks the selected item IDs, and for each whose manifest
// declares an Input (kind=form or kind=text), runs a FormModel and gathers
// the user's responses. Returns a cancelled=true if the user pressed esc
// on any form — the caller re-shows the picker/review in that case.
func collectInputs(m *manifest.Manifest, itemIDs []string) (map[string]map[string]string, bool, error) {
	values := make(map[string]map[string]string)
	for _, id := range itemIDs {
		it, ok := m.Items[id]
		if !ok || it.Input == nil {
			continue
		}
		if it.Input.Kind == manifest.InputBool {
			// bool isn't wired yet; let the provider refuse with a clear error.
			continue
		}
		r, err := tui.RunForm(context.Background(), id, it.Name, it.Input, os.Stderr, os.Stderr)
		if err != nil {
			return nil, false, err
		}
		if r.Quit {
			return nil, true, fmt.Errorf("cancelled")
		}
		if r.Cancelled {
			return nil, true, nil
		}
		if r.Confirmed {
			values[id] = r.Values
		}
	}
	return values, false, nil
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
	values map[string]map[string]string,
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

	// Dry-run: simple text summary, no TUI.
	if !apply {
		return executeHeadless(out, m, reg, st, d, env, choice, rc, values, bootstrapper)
	}

	return executeWithProgressTUI(m, reg, st, env, choice, rc, values, bootstrapper)
}

// executeHeadless runs the dry-run path with plain stdout logging.
func executeHeadless(
	out interface{ Write([]byte) (int, error) },
	m *manifest.Manifest,
	reg provider.Registry,
	st *state.State,
	_ system.Distro,
	env runner.Env,
	choice tui.ProfileChoice,
	rc tui.ReviewChoice,
	values map[string]map[string]string,
	bootstrapper *runner.RepoBootstrapper,
) error {
	r := &runner.Runner{
		Manifest:     m,
		Registry:     reg,
		State:        st,
		Bootstrapper: bootstrapper,
		Env:          env,
		Values:       values,
		Out:          out,
		StateFn:      func(s *state.State) error { return state.Save("", s) },
	}

	fmt.Fprintf(out, "\nDry-run: %d item(s) selected\n\n", len(rc.ItemIDs))
	sum, err := r.Run(context.Background(), runner.Selection{Items: rc.ItemIDs}, runner.Options{
		DryRun: true, Profile: choice.ID,
	})
	if err != nil {
		return err
	}
	return printHeadlessSummary(out, sum)
}

// executeWithProgressTUI runs apply-mode with the live Bubble Tea progress UI.
func executeWithProgressTUI(
	m *manifest.Manifest,
	reg provider.Registry,
	st *state.State,
	env runner.Env,
	choice tui.ProfileChoice,
	rc tui.ReviewChoice,
	values map[string]map[string]string,
	bootstrapper *runner.RepoBootstrapper,
) error {
	prog, wait := tui.RunInstallProgress(choice.Name, len(rc.ItemIDs), os.Stderr)

	r := &runner.Runner{
		Manifest:     m,
		Registry:     reg,
		State:        st,
		Bootstrapper: bootstrapper,
		Env:          env,
		Values:       values,
		Out:          io.Discard, // the TUI owns the screen; text log would scramble it
		StateFn:      func(s *state.State) error { return state.Save("", s) },
		ProgressFn:   func(e runner.ProgressEvent) { prog.Send(e) },
	}

	// Run the install on its own goroutine; the tea.Program blocks on wait().
	go func() {
		sum, err := r.Run(context.Background(), runner.Selection{Items: rc.ItemIDs}, runner.Options{
			DryRun: false, Profile: choice.ID,
		})
		prog.Send(tui.ProgressFinishedMsg{Summary: sum, Err: err})
	}()

	final, err := wait()
	if err != nil {
		return fmt.Errorf("progress tui: %w", err)
	}
	if final.FinalErr() != nil {
		return final.FinalErr()
	}
	if sum := final.Summary(); sum != nil && len(sum.Failed) > 0 {
		return fmt.Errorf("%d item(s) failed", len(sum.Failed))
	}
	return nil
}

func printHeadlessSummary(out interface{ Write([]byte) (int, error) }, sum *runner.Summary) error {
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

