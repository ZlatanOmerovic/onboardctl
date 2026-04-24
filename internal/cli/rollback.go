package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	"github.com/ZlatanOmerovic/onboardctl/internal/provider"
	"github.com/ZlatanOmerovic/onboardctl/internal/runner"
	"github.com/ZlatanOmerovic/onboardctl/internal/state"
	"github.com/ZlatanOmerovic/onboardctl/internal/system"
)

var rollbackOpts struct {
	extras    string
	assumeYes bool
}

var rollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "Undo the last non-dry-run install",
	Long: `rollback walks the most recent non-dry-run entry in state.yaml and
invokes each provider's Uninstall for items that run installed (LIFO).

Items whose provider has no Uninstall capability (config, shell) are
skipped with a log line. A rolled-back run is marked with a timestamp
in state so repeated invocations are no-ops.

Requires root because the providers it dispatches to (apt, flatpak,
binary_release) mutate system paths. Without --yes, rollback prints
what it would do and exits without touching anything.`,
	RunE: runRollback,
}

func init() {
	rollbackCmd.Flags().StringVar(&rollbackOpts.extras, "extras", "", "path to user extras YAML (default: XDG)")
	rollbackCmd.Flags().BoolVarP(&rollbackOpts.assumeYes, "yes", "y", false, "actually perform the rollback (default: dry-run)")
	rootCmd.AddCommand(rollbackCmd)
}

func runRollback(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()

	st, err := state.Load("")
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	run := findRollbackTarget(st)
	if run == nil {
		return errors.New("no run in state is eligible for rollback (either no runs, or the last run is already rolled back or was dry-run)")
	}

	fmt.Fprintf(out, "Rollback target: run started %s (%d installed item(s))\n",
		run.StartedAt.Format(time.RFC3339), len(run.Installed))
	for i := len(run.Installed) - 1; i >= 0; i-- {
		fmt.Fprintf(out, "  ~ %-20s via %s\n", run.Installed[i].ItemID, run.Installed[i].Kind)
	}

	if !rollbackOpts.assumeYes {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Dry-run. Re-run with --yes to apply.")
		return nil
	}

	if os.Geteuid() != 0 {
		return errors.New("rollback --yes needs root privileges — re-run with sudo")
	}

	m, err := manifest.Load(rollbackOpts.extras)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}
	d, err := system.DetectDistro()
	if err != nil {
		return fmt.Errorf("detect distro: %w", err)
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
		Env:      runner.Env{Distro: d, Desktop: system.DetectDesktop()},
		Out:      out,
	}

	n, when, err := r.RollbackLastRun(context.Background())
	if err != nil {
		return err
	}
	if err := state.Save("", st); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	fmt.Fprintf(out, "\nRolled back %d item(s) from run at %s.\n", n, when.Format(time.RFC3339))
	return nil
}

// findRollbackTarget mirrors the search RollbackLastRun will perform,
// so the CLI can preview without actually running the rollback.
func findRollbackTarget(st *state.State) *state.Run {
	for i := len(st.Runs) - 1; i >= 0; i-- {
		run := &st.Runs[i]
		if run.DryRun {
			continue
		}
		if !run.RolledBackAt.IsZero() {
			continue
		}
		if len(run.Installed) == 0 {
			continue
		}
		return run
	}
	return nil
}
