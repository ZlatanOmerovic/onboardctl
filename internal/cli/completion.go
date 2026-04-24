package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var completionOpts struct {
	shell  string
	stdout bool
	system bool
}

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish]",
	Short: "Generate and install shell completion",
	Long: `completion generates the completion script for the named shell and
writes it to a conventional location. With no argument and no --shell
flag, the current $SHELL is auto-detected.

By default the script is installed to the per-user path:
  bash  $XDG_DATA_HOME/bash-completion/completions/onboardctl
  zsh   $HOME/.zsh/completions/_onboardctl   (ensure this dir is in fpath)
  fish  $XDG_CONFIG_HOME/fish/completions/onboardctl.fish

Pass --system to install to the system-wide path (needs sudo):
  bash  /etc/bash_completion.d/onboardctl
  zsh   /usr/share/zsh/site-functions/_onboardctl
  fish  /usr/share/fish/vendor_completions.d/onboardctl.fish

Pass --stdout to print the script instead of writing it anywhere.`,
	Args:      cobra.MaximumNArgs(1),
	ValidArgs: []string{"bash", "zsh", "fish"},
	RunE:      runCompletion,
}

func init() {
	completionCmd.Flags().StringVar(&completionOpts.shell, "shell", "", "bash | zsh | fish (auto-detected from $SHELL when omitted)")
	completionCmd.Flags().BoolVar(&completionOpts.stdout, "stdout", false, "print script to stdout instead of installing")
	completionCmd.Flags().BoolVar(&completionOpts.system, "system", false, "install to a system-wide path (needs sudo)")
	rootCmd.AddCommand(completionCmd)
}

func runCompletion(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	shell := completionOpts.shell
	if shell == "" && len(args) > 0 {
		shell = args[0]
	}
	if shell == "" {
		s, err := detectShell()
		if err != nil {
			return err
		}
		shell = s
		fmt.Fprintf(out, "Auto-detected shell: %s\n", shell)
	}

	gen, err := completionGenerator(shell)
	if err != nil {
		return err
	}

	// Route to stdout for piping / manual install.
	if completionOpts.stdout {
		return gen(out)
	}

	target, err := completionTargetPath(shell, completionOpts.system)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(target), err)
	}

	f, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("create %s: %w", target, err)
	}
	if err := gen(f); err != nil {
		_ = f.Close()
		return fmt.Errorf("write completion: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", target, err)
	}
	fmt.Fprintf(out, "Installed %s completion at %s\n", shell, target)
	if !completionOpts.system && shell == "zsh" {
		fmt.Fprintln(out, "Note: ensure $HOME/.zsh/completions is in your fpath (add `fpath+=$HOME/.zsh/completions` before compinit).")
	}
	return nil
}

func completionGenerator(shell string) (func(io.Writer) error, error) {
	switch shell {
	case "bash":
		return rootCmd.GenBashCompletion, nil
	case "zsh":
		return rootCmd.GenZshCompletion, nil
	case "fish":
		return func(w io.Writer) error { return rootCmd.GenFishCompletion(w, true) }, nil
	}
	return nil, fmt.Errorf("unsupported shell %q (want bash, zsh, or fish)", shell)
}

// detectShell returns the shell basename from $SHELL. Errors when the
// env var is empty or names something we don't generate for.
func detectShell() (string, error) {
	s := strings.ToLower(filepath.Base(os.Getenv("SHELL")))
	switch s {
	case "bash", "zsh", "fish":
		return s, nil
	case "":
		return "", errors.New("cannot auto-detect shell: $SHELL is not set")
	}
	return "", fmt.Errorf("cannot auto-detect shell: $SHELL=%q is not bash/zsh/fish — pass --shell explicitly", s)
}

// completionTargetPath returns the path the completion script should
// land at, respecting --system.
func completionTargetPath(shell string, system bool) (string, error) {
	if system {
		switch shell {
		case "bash":
			return "/etc/bash_completion.d/onboardctl", nil
		case "zsh":
			return "/usr/share/zsh/site-functions/_onboardctl", nil
		case "fish":
			return "/usr/share/fish/vendor_completions.d/onboardctl.fish", nil
		}
		return "", fmt.Errorf("unsupported shell %q", shell)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	switch shell {
	case "bash":
		xdg := os.Getenv("XDG_DATA_HOME")
		if xdg == "" {
			xdg = filepath.Join(home, ".local", "share")
		}
		return filepath.Join(xdg, "bash-completion", "completions", "onboardctl"), nil
	case "zsh":
		return filepath.Join(home, ".zsh", "completions", "_onboardctl"), nil
	case "fish":
		xdg := os.Getenv("XDG_CONFIG_HOME")
		if xdg == "" {
			xdg = filepath.Join(home, ".config")
		}
		return filepath.Join(xdg, "fish", "completions", "onboardctl.fish"), nil
	}
	return "", fmt.Errorf("unsupported shell %q", shell)
}
