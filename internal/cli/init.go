package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/user"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	"github.com/ZlatanOmerovic/onboardctl/internal/provider"
	"github.com/ZlatanOmerovic/onboardctl/internal/runner"
	"github.com/ZlatanOmerovic/onboardctl/internal/state"
	"github.com/ZlatanOmerovic/onboardctl/internal/system"
	"github.com/ZlatanOmerovic/onboardctl/internal/tui"
	"github.com/spf13/cobra"
)

var initOpts struct {
	extras string
	apply  bool
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Guided first-boot wizard: pick a terminal, shell, and prompt",
	Long: `init walks through the foundational environment setup for a fresh
Debian-based machine. Four steps, each with a "keep current" option so
the wizard is non-destructive by default:

  1. Terminal — kitty / alacritty / keep current
  2. Shell    — zsh / fish / keep current
  3. Prompt   — starship / keep current
  4. Confirm

After confirmation the picks become a Selection that flows through the
usual runner pipeline (same Check/Install providers as 'onboardctl profile').

This is v1: theme picking, oh-my-zsh / powerlevel10k / pure, and
wezterm / ghostty / foot are follow-up iterations. The picks here cover
80%+ of fresh-machine cases while keeping the wizard one-screen-per-step.`,
	Args: cobra.NoArgs,
	RunE: runInit,
}

func init() {
	initCmd.Flags().StringVar(&initOpts.extras, "extras", "", "path to user extras YAML (default: XDG)")
	initCmd.Flags().BoolVar(&initOpts.apply, "apply", false, "apply the selection instead of dry-run (requires sudo)")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()

	m, err := manifest.Load(initOpts.extras)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}
	d, err := system.DetectDistro()
	if err != nil {
		return fmt.Errorf("detect distro: %w", err)
	}
	if !d.InDebianFamily() {
		return fmt.Errorf("unsupported distro %q (Debian family only)", d.ID)
	}

	// Step 0: welcome screen — just a quick environment summary.
	renderWelcome(out, d)

	// Step 1: terminal
	terminal, err := pickTerminal(os.Stderr)
	if err != nil || terminal.Cancelled {
		return err // nil on graceful cancel
	}

	// Step 2: shell
	shell, err := pickShell(os.Stderr)
	if err != nil || shell.Cancelled {
		return nil
	}

	// Step 3: prompt
	prompt, err := pickPrompt(os.Stderr)
	if err != nil || prompt.Cancelled {
		return nil
	}

	// Build selection of item IDs (empty Value means "keep current" → no item).
	var itemIDs []string
	for _, pick := range []tui.OneOfResult{terminal, shell, prompt} {
		if pick.Value != "" {
			itemIDs = append(itemIDs, pick.Value)
		}
	}
	if len(itemIDs) == 0 {
		fmt.Fprintln(out, "\nAll picks were 'keep current' — nothing to install.")
		return nil
	}

	// Summary.
	fmt.Fprintln(out, "\nPicks:")
	if terminal.Value != "" {
		fmt.Fprintf(out, "  Terminal: %s\n", terminal.Label)
	}
	if shell.Value != "" {
		fmt.Fprintf(out, "  Shell:    %s\n", shell.Label)
	}
	if prompt.Value != "" {
		fmt.Fprintf(out, "  Prompt:   %s\n", prompt.Label)
	}
	fmt.Fprintln(out)

	return executeInit(out, m, d, itemIDs)
}

func renderWelcome(out io.Writer, d system.Distro) {
	u, _ := user.Current()
	name := "friend"
	if u != nil {
		name = u.Username
	}
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "──────────────────────────────────────────────────────────────")
	fmt.Fprintf(out, "  onboardctl init — welcome, %s\n", name)
	fmt.Fprintln(out, "──────────────────────────────────────────────────────────────")
	fmt.Fprintln(out, "")
	fmt.Fprintf(out, "  Distro:  %s %s (%s) %s\n", d.Name, d.Version, d.Codename, d.Arch)
	fmt.Fprintf(out, "  Desktop: %s\n", system.DetectDesktop())
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  Four questions coming up. 'Keep current' is always an option.")
	fmt.Fprintln(out, "")
}

func pickTerminal(out io.Writer) (tui.OneOfResult, error) {
	opts := []tui.OneOfOption{
		{Value: "", Label: "Keep current", Description: "no change"},
		{Value: "kitty", Label: "Kitty", Description: "GPU, native Wayland, built-in tabs/splits"},
		{Value: "alacritty", Label: "Alacritty", Description: "leanest GPU-accelerated terminal"},
	}
	return tui.RunOneOf(context.Background(), "Terminal", "Which terminal emulator do you want?", opts, out)
}

func pickShell(out io.Writer) (tui.OneOfResult, error) {
	opts := []tui.OneOfOption{
		{Value: "", Label: "Keep current", Description: "no change"},
		{Value: "zsh", Label: "Zsh", Description: "bash-compatible, large plugin ecosystem"},
		{Value: "fish", Label: "Fish", Description: "user-friendly, smart autosuggestions"},
	}
	return tui.RunOneOf(context.Background(), "Shell", "Which interactive shell do you want installed?", opts, out)
}

func pickPrompt(out io.Writer) (tui.OneOfResult, error) {
	opts := []tui.OneOfOption{
		{Value: "", Label: "Keep current", Description: "no change"},
		{Value: "starship", Label: "Starship", Description: "minimal, fast, cross-shell — starship.rs"},
	}
	return tui.RunOneOf(context.Background(), "Prompt", "Which prompt do you want installed?", opts, out)
}

func executeInit(
	out io.Writer,
	m *manifest.Manifest,
	d system.Distro,
	itemIDs []string,
) error {
	apply := initOpts.apply
	if apply && os.Geteuid() != 0 {
		return fmt.Errorf("--apply needs root — re-run with sudo")
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
	st.Distro = state.DistroSnapshot{
		ID: d.ID, Codename: d.Codename, Version: d.Version, Family: d.Family, Arch: d.Arch,
	}

	var bootstrapper *runner.RepoBootstrapper
	if apply {
		bootstrapper = runner.NewRepoBootstrapper(m.Repos, provider.ExecRunner(), d)
	}

	r := &runner.Runner{
		Manifest:     m,
		Registry:     reg,
		State:        st,
		Bootstrapper: bootstrapper,
		Env:          runner.Env{Distro: d, Desktop: system.DetectDesktop()},
		Out:          out,
		StateFn:      func(s *state.State) error { return state.Save("", s) },
	}

	mode := "Dry-run"
	if apply {
		mode = "Applying"
	}
	fmt.Fprintf(out, "%s: %v\n\n", mode, itemIDs)

	sum, err := r.Run(context.Background(), runner.Selection{Items: itemIDs}, runner.Options{
		DryRun:  !apply,
		Profile: "init",
	})
	if err != nil {
		return err
	}
	return printHeadlessSummary(out, sum)
}
