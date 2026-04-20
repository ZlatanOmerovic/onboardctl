package cli

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/ZlatanOmerovic/onboardctl/internal/state"
	"github.com/ZlatanOmerovic/onboardctl/internal/system"
)

var exportOpts struct {
	output          string
	format          string
	includeExternal bool
}

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export the items onboardctl has installed on this machine",
	Long: `export writes the set of installed items to stdout (or --output PATH),
so you can hand it to 'onboardctl install --from-export PATH' on another box.

Two formats:
  --format=yaml  (default) A YAML document with machine metadata in comments.
  --format=list            One item ID per line. Pipe-friendly.

By default only items onboardctl installed itself are emitted. Pass
--include-external to also include items detected as present but managed
by someone else.`,
	Args: cobra.NoArgs,
	RunE: runExport,
}

func init() {
	exportCmd.Flags().StringVarP(&exportOpts.output, "output", "o", "", "write to this file (default: stdout)")
	exportCmd.Flags().StringVar(&exportOpts.format, "format", "yaml", "output format: yaml | list")
	exportCmd.Flags().BoolVar(&exportOpts.includeExternal, "include-external", false, "include items detected as present but not installed by onboardctl")
	rootCmd.AddCommand(exportCmd)
}

func runExport(cmd *cobra.Command, _ []string) error {
	st, err := state.Load("")
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	d, err := system.DetectDistro()
	if err != nil {
		return fmt.Errorf("detect distro: %w", err)
	}

	items := collectExportItems(st, exportOpts.includeExternal)
	if len(items) == 0 {
		return fmt.Errorf("no items to export (state is empty; run an install first, or pass --include-external)")
	}

	var buf bytes.Buffer
	switch exportOpts.format {
	case "yaml":
		writeExportYAML(&buf, items, d, st)
	case "list":
		writeExportList(&buf, items)
	default:
		return fmt.Errorf("unknown --format %q (yaml or list)", exportOpts.format)
	}

	if exportOpts.output == "" {
		_, err := cmd.OutOrStdout().Write(buf.Bytes())
		return err
	}
	return os.WriteFile(exportOpts.output, buf.Bytes(), 0o644)
}

// collectExportItems picks which state-file items to include and returns
// a sorted, deduped slice of item IDs.
func collectExportItems(st *state.State, includeExternal bool) []string {
	ids := make([]string, 0, len(st.Items))
	for id, rec := range st.Items {
		if rec.Status != state.StatusInstalled {
			continue
		}
		if !includeExternal && rec.InstalledBy != state.ByOnboardctl {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func writeExportYAML(buf *bytes.Buffer, items []string, d system.Distro, st *state.State) {
	fmt.Fprintf(buf, "# onboardctl export — %s\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(buf, "# Distro:  %s %s (%s) %s\n", d.Name, d.Version, d.Codename, d.Arch)
	if st.Profile != "" {
		fmt.Fprintf(buf, "# Profile: %s\n", st.Profile)
	}
	fmt.Fprintln(buf, "#")
	fmt.Fprintln(buf, "# Replay on another machine with:")
	fmt.Fprintln(buf, "#   onboardctl install --from-export <path>")
	fmt.Fprintln(buf, "")
	fmt.Fprintln(buf, "version: 1")
	fmt.Fprintln(buf, "items:")
	for _, id := range items {
		fmt.Fprintf(buf, "  - %s\n", id)
	}
}

func writeExportList(buf *bytes.Buffer, items []string) {
	for _, id := range items {
		fmt.Fprintln(buf, id)
	}
}
