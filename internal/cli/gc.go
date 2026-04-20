package cli

import (
	"fmt"
	"sort"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	"github.com/ZlatanOmerovic/onboardctl/internal/state"
	"github.com/spf13/cobra"
)

var gcOpts struct {
	extras string
	dryRun bool
	yes    bool
}

var gcCmd = &cobra.Command{
	Use:   "gc",
	Short: "Prune state.yaml of items no longer defined in the manifest",
	Long: `gc (garbage collect) scans state.yaml for item IDs that are absent
from the bundled manifest and any merged user extras. Such entries
accumulate when you drop an item from your extras.yaml or when a
bundled item is renamed upstream — they don't hurt anything, but they
clutter status output and 'onboardctl export' emits them.

By default gc is interactive: it lists what would be removed and asks
for confirmation. Pass --dry-run to preview without modifying anything,
or --yes to skip the prompt (suitable for scripts).

gc never touches the system — no packages are removed, no configs
reverted. It only edits state.yaml.`,
	Args: cobra.NoArgs,
	RunE: runGC,
}

func init() {
	gcCmd.Flags().StringVar(&gcOpts.extras, "extras", "", "path to user extras YAML (default: XDG)")
	gcCmd.Flags().BoolVar(&gcOpts.dryRun, "dry-run", false, "print what would be removed; don't modify state.yaml")
	gcCmd.Flags().BoolVarP(&gcOpts.yes, "yes", "y", false, "skip the confirmation prompt")
	rootCmd.AddCommand(gcCmd)
}

func runGC(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()

	m, err := manifest.Load(gcOpts.extras)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}

	st, err := state.Load("")
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	stale := findStaleItems(st, m)
	if len(stale) == 0 {
		fmt.Fprintln(out, "state.yaml is clean — nothing to gc.")
		return nil
	}

	fmt.Fprintf(out, "Stale items in state.yaml (%d):\n", len(stale))
	for _, id := range stale {
		rec := st.Items[id]
		fmt.Fprintf(out, "  - %-22s (was %s via %s)\n", id, rec.Status, rec.Provider)
	}

	if gcOpts.dryRun {
		fmt.Fprintln(out, "\n--dry-run: no changes made.")
		return nil
	}

	if !gcOpts.yes {
		fmt.Fprintf(out, "\nRemove %d stale entr%s? [y/N] ",
			len(stale), pluralYies(len(stale)))
		var answer string
		// Using Fscan reads a single whitespace-delimited token from stdin.
		_, _ = fmt.Fscan(cmd.InOrStdin(), &answer)
		if answer != "y" && answer != "Y" && answer != "yes" {
			fmt.Fprintln(out, "Cancelled.")
			return nil
		}
	}

	for _, id := range stale {
		delete(st.Items, id)
	}
	if err := state.Save("", st); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	fmt.Fprintf(out, "Removed %d stale entr%s from state.yaml.\n",
		len(stale), pluralYies(len(stale)))
	return nil
}

// findStaleItems returns the state item IDs that aren't present in the
// manifest, sorted alphabetically.
func findStaleItems(st *state.State, m *manifest.Manifest) []string {
	var out []string
	for id := range st.Items {
		if _, ok := m.Items[id]; !ok {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// pluralYies returns "y" for 1, "ies" for anything else. Used to render
// "entry" vs "entries" in confirmation prompts.
func pluralYies(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}
