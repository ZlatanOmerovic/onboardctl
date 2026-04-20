package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ZlatanOmerovic/onboardctl/internal/state"
)

var forgetOpts struct {
	all    bool
	dryRun bool
	yes    bool
}

var forgetCmd = &cobra.Command{
	Use:   "forget [item-id...]",
	Short: "Remove entries from state.yaml without touching the system",
	Long: `forget deletes one or more items from state.yaml so onboardctl
stops remembering it installed them. Unlike gc, forget targets specific
items — useful when you've upgraded or replaced something outside
onboardctl and want state.yaml to reflect that.

  * Pass item IDs: 'onboardctl forget jq vlc lazygit'
  * Or --all to clear every entry.

Forget NEVER removes installed packages or reverts configs — it only
edits state.yaml. To remove the actual software, use your package
manager (apt, flatpak, composer global remove, etc.).

Default is interactive: prints the list of items that would be removed
and asks for confirmation. Pass --dry-run to preview without writing,
or --yes to skip the prompt (suitable for scripts).`,
	Args: cobra.ArbitraryArgs,
	RunE: runForget,
}

func init() {
	forgetCmd.Flags().BoolVar(&forgetOpts.all, "all", false, "remove every entry from state.yaml (requires confirmation)")
	forgetCmd.Flags().BoolVar(&forgetOpts.dryRun, "dry-run", false, "print what would be removed; don't modify state.yaml")
	forgetCmd.Flags().BoolVarP(&forgetOpts.yes, "yes", "y", false, "skip the confirmation prompt")
	rootCmd.AddCommand(forgetCmd)
}

func runForget(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	// Validate arguments.
	switch {
	case forgetOpts.all && len(args) > 0:
		return fmt.Errorf("pass --all OR specific item IDs, not both")
	case !forgetOpts.all && len(args) == 0:
		return fmt.Errorf("pass one or more item IDs, or --all")
	}

	st, err := state.Load("")
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	// Resolve which IDs to remove.
	var toRemove []string
	var missing []string
	if forgetOpts.all {
		toRemove = collectAllItemIDs(st)
	} else {
		for _, id := range args {
			if _, ok := st.Items[id]; ok {
				toRemove = append(toRemove, id)
			} else {
				missing = append(missing, id)
			}
		}
	}

	if len(missing) > 0 {
		fmt.Fprintln(out, "Not in state.yaml (ignored):")
		for _, id := range missing {
			fmt.Fprintf(out, "  - %s\n", id)
		}
		fmt.Fprintln(out)
	}

	if len(toRemove) == 0 {
		fmt.Fprintln(out, "Nothing to forget.")
		return nil
	}

	sort.Strings(toRemove)
	fmt.Fprintf(out, "Would remove %d entr%s from state.yaml:\n",
		len(toRemove), pluralYies(len(toRemove)))
	for _, id := range toRemove {
		rec := st.Items[id]
		fmt.Fprintf(out, "  - %-22s (was %s via %s, v%s)\n",
			id, rec.Status, rec.Provider, firstNonBlank(rec.Version, "?"))
	}

	if forgetOpts.dryRun {
		fmt.Fprintln(out, "\n--dry-run: no changes made.")
		return nil
	}

	if !forgetOpts.yes {
		fmt.Fprintf(out, "\nProceed? [y/N] ")
		var answer string
		_, _ = fmt.Fscan(cmd.InOrStdin(), &answer)
		if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
			fmt.Fprintln(out, "Cancelled.")
			return nil
		}
	}

	for _, id := range toRemove {
		delete(st.Items, id)
	}
	if err := state.Save("", st); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	fmt.Fprintf(out, "Forgot %d entr%s.\n", len(toRemove), pluralYies(len(toRemove)))
	return nil
}

// collectAllItemIDs returns every item ID currently in state, sorted.
func collectAllItemIDs(st *state.State) []string {
	ids := make([]string, 0, len(st.Items))
	for id := range st.Items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// firstNonBlank returns the first of values that is non-empty. Same
// semantics as firstNonEmpty elsewhere — kept local so package files
// stay self-contained.
func firstNonBlank(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
