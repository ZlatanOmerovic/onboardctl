package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

var manpageOpts struct {
	dir    string
	system bool
	stdout bool
}

var manpageCmd = &cobra.Command{
	Use:   "manpage",
	Short: "Generate and install onboardctl man pages",
	Long: `manpage generates one roff manpage per subcommand and writes them to
a man1 directory.

Default destination is the per-user path $HOME/.local/share/man/man1/
(ensure that tree is covered by manpath(1); it is on most distributions).

Pass --system to install to /usr/share/man/man1/ (needs sudo). Pass --dir
to target any directory explicitly; --dir takes precedence over --system.

Pass --stdout to print the primary onboardctl.1 page to stdout (only the
top-level page, not the per-subcommand pages — use --dir for the full set).`,
	RunE: runManpage,
}

func init() {
	manpageCmd.Flags().StringVar(&manpageOpts.dir, "dir", "", "install directory (overrides --system)")
	manpageCmd.Flags().BoolVar(&manpageOpts.system, "system", false, "install to /usr/share/man/man1/ (needs sudo)")
	manpageCmd.Flags().BoolVar(&manpageOpts.stdout, "stdout", false, "print the top-level onboardctl.1 page to stdout")
	rootCmd.AddCommand(manpageCmd)
}

func runManpage(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()

	now := time.Now()
	header := &doc.GenManHeader{
		Title:   "ONBOARDCTL",
		Section: "1",
		Date:    &now,
		Source:  "onboardctl",
		Manual:  "onboardctl Manual",
	}

	if manpageOpts.stdout {
		// Render only the top-level page. Cobra writes to an io.Writer
		// via GenMan, so we stream directly to stdout.
		rootCmd.DisableAutoGenTag = true
		return doc.GenMan(rootCmd, header, out)
	}

	target, err := manpageTargetDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", target, err)
	}

	rootCmd.DisableAutoGenTag = true
	if err := doc.GenManTree(rootCmd, header, target); err != nil {
		return fmt.Errorf("generate manpages: %w", err)
	}

	count, err := countManFiles(target)
	if err != nil {
		return fmt.Errorf("list generated pages: %w", err)
	}
	fmt.Fprintf(out, "Installed %d man page(s) in %s\n", count, target)
	if !manpageOpts.system && manpageOpts.dir == "" {
		fmt.Fprintln(out, "If `man onboardctl` still can't find the page, add $HOME/.local/share/man to your MANPATH.")
	}
	return nil
}

func manpageTargetDir() (string, error) {
	if manpageOpts.dir != "" {
		return manpageOpts.dir, nil
	}
	if manpageOpts.system {
		return "/usr/share/man/man1", nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	return filepath.Join(home, ".local", "share", "man", "man1"), nil
}

func countManFiles(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) == ".1" {
			n++
		}
	}
	return n, nil
}
