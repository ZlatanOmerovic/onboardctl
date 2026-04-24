package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/user"

	"github.com/spf13/cobra"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	"github.com/ZlatanOmerovic/onboardctl/internal/provider"
	"github.com/ZlatanOmerovic/onboardctl/internal/runner"
	"github.com/ZlatanOmerovic/onboardctl/internal/state"
	"github.com/ZlatanOmerovic/onboardctl/internal/system"
	"github.com/ZlatanOmerovic/onboardctl/internal/tui"
)

var initOpts struct {
	extras        string
	apply         bool
	skipInstalled bool
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Guided first-boot wizard: pick a terminal, shell, prompt, and theme",
	Long: `init walks through the foundational environment setup for a fresh
Debian-based machine. Every step has a "keep current" option so the
wizard is non-destructive by default:

  1. Terminal — kitty / alacritty / keep current
  2. Shell    — zsh / fish / keep current
  3. Prompt   — starship / keep current
  4. Theme    — dark / light / keep current   (GNOME today; other DEs no-op)

After confirmation the picks become a Selection that flows through the
usual runner pipeline (same Check/Install providers as 'onboardctl profile').

Re-running on a machine that already has some of these installed shows
the current picks with a ✓ marker; --skip-installed bypasses a whole
pick when any of its options is already installed.`,
	Args: cobra.NoArgs,
	RunE: runInit,
}

func init() {
	initCmd.Flags().StringVar(&initOpts.extras, "extras", "", "path to user extras YAML (default: XDG)")
	initCmd.Flags().BoolVar(&initOpts.apply, "apply", false, "apply the selection instead of dry-run (requires sudo)")
	initCmd.Flags().BoolVar(&initOpts.skipInstalled, "skip-installed", false, "skip a pick entirely when one of its options is already installed (suitable for scripted reruns)")
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

	// Pre-flight: probe the install state of every candidate item so each
	// picker can annotate options with ✓ for "already installed". Re-running
	// init on a machine that already has the target packages becomes a
	// no-op tour instead of a blind wizard.
	installed := probeInitCandidates(m, d)

	// Step 0: welcome screen — detected environment + install state.
	renderWelcome(out, d, installed)

	// When --skip-installed is set, bypass an entire category if any of
	// its options is already on the system. Result: re-running init on a
	// configured machine shows pickers only for categories the user
	// hasn't filled in yet.
	skipIfInstalled := func(pick func(io.Writer, map[string]bool) (tui.OneOfResult, error), ids ...string) (tui.OneOfResult, error) {
		if initOpts.skipInstalled && anyTrue(installed, ids...) {
			return tui.OneOfResult{Value: "", Label: "Keep current"}, nil
		}
		return pick(os.Stderr, installed)
	}

	// Step 1: terminal
	terminal, err := skipIfInstalled(pickTerminal, "kitty", "alacritty")
	if err != nil || terminal.Cancelled {
		return err // nil on graceful cancel
	}

	// Step 2: shell
	shell, err := skipIfInstalled(pickShell, "zsh", "fish")
	if err != nil || shell.Cancelled {
		return nil
	}

	// Step 3: prompt
	prompt, err := skipIfInstalled(pickPrompt, "starship")
	if err != nil || prompt.Cancelled {
		return nil
	}

	// Step 4: desktop theme — only surface when the desktop maps to a
	// theme provider we know. Today that's GNOME only; other DEs skip
	// the picker gracefully.
	theme, err := pickThemeForDesktop(os.Stderr, installed)
	if err != nil || theme.Cancelled {
		return nil
	}

	// Build selection of item IDs (empty Value means "keep current" → no item).
	var itemIDs []string
	for _, pick := range []tui.OneOfResult{terminal, shell, prompt, theme} {
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
	if theme.Value != "" {
		fmt.Fprintf(out, "  Theme:    %s\n", theme.Label)
	}
	fmt.Fprintln(out)

	return executeInit(out, m, d, itemIDs)
}

func renderWelcome(out io.Writer, d system.Distro, installed map[string]bool) {
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
	fmt.Fprintf(out, "  Distro:   %s %s (%s) %s\n", d.Name, d.Version, d.Codename, d.Arch)
	fmt.Fprintf(out, "  Desktop:  %s\n", system.DetectDesktop())
	fmt.Fprintf(out, "  Shell:    %s%s\n", currentShellName(), loginShellHint())
	fmt.Fprintf(out, "  Terminal: %s\n", currentTerminalName())

	// Install-state one-liner: tell the user upfront which of the
	// init-wizard candidates are already present. ✓ for yes, dim for no.
	installedList := listInstalled(installed, []string{"kitty", "alacritty", "zsh", "fish", "starship"})
	if installedList != "" {
		fmt.Fprintf(out, "  Detected: %s\n", installedList)
	}

	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  Four quick picks. 'Keep current' is always an option.")
	fmt.Fprintln(out, "")
}

// currentShellName returns the bare name of the user's login shell
// (e.g. "zsh", "bash", "fish") from /etc/passwd.
func currentShellName() string {
	u, err := user.Current()
	if err != nil || u == nil {
		return "unknown"
	}
	sh := os.Getenv("SHELL")
	if sh == "" {
		return "unknown"
	}
	// Trim dir: "/usr/bin/zsh" -> "zsh"
	base := sh
	for i := len(sh) - 1; i >= 0; i-- {
		if sh[i] == '/' {
			base = sh[i+1:]
			break
		}
	}
	return base
}

func loginShellHint() string {
	// Hint placeholder; SHELL usually matches the login shell. Kept
	// as a separate helper so later versions can compare with
	// getent passwd output if we want to be precise about default vs
	// current interactive shell.
	return ""
}

// currentTerminalName infers the terminal emulator from env hints that
// terminals commonly set for processes they launch.
func currentTerminalName() string {
	switch {
	case os.Getenv("KITTY_WINDOW_ID") != "":
		return "kitty"
	case os.Getenv("ALACRITTY_SOCKET") != "", os.Getenv("ALACRITTY_LOG") != "":
		return "alacritty"
	case os.Getenv("WEZTERM_PANE") != "":
		return "wezterm"
	case os.Getenv("GHOSTTY_RESOURCES_DIR") != "":
		return "ghostty"
	case os.Getenv("VTE_VERSION") != "":
		return "gnome-terminal / vte-based"
	}
	if t := os.Getenv("TERM_PROGRAM"); t != "" {
		return t
	}
	if t := os.Getenv("TERM"); t != "" {
		return t
	}
	return "unknown"
}

// listInstalled produces a compact comma-separated list with markers so
// the welcome screen can show detected state in one line.
func listInstalled(state map[string]bool, ids []string) string {
	var parts []string
	for _, id := range ids {
		if state[id] {
			parts = append(parts, "✓ "+id)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return joinComma(parts)
}

func joinComma(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}

// probeInitCandidates runs Check on each candidate item via the normal
// provider pipeline and returns a map of itemID → installed bool. Any
// manifest item not present in the loaded manifest is silently skipped
// (user may have dropped it from extras).
func probeInitCandidates(m *manifest.Manifest, d system.Distro) map[string]bool {
	out := map[string]bool{}
	if m == nil {
		return out
	}
	candidateIDs := []string{
		"kitty", "alacritty", "zsh", "fish", "starship",
		"gnome-dark-mode", "gnome-light-mode",
		"kde-dark-mode", "kde-light-mode",
		"xfce-dark-mode", "xfce-light-mode",
		"mate-dark-mode", "mate-light-mode",
		"cinnamon-dark-mode", "cinnamon-light-mode",
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
		Env:      runner.Env{Distro: d, Desktop: system.DetectDesktop()},
	}
	plan, err := r.Plan(context.Background(), runner.Selection{Items: candidateIDs})
	if err != nil {
		return out
	}
	for _, entry := range plan.Entries {
		out[entry.ItemID] = entry.State.Installed
	}
	return out
}

func markerFor(installed bool) string {
	if installed {
		return "✓"
	}
	return ""
}

// anyTrue reports whether any of the named keys maps to true in the
// install-state map. Used by --skip-installed to decide whether to
// bypass a picker.
func anyTrue(state map[string]bool, keys ...string) bool {
	for _, k := range keys {
		if state[k] {
			return true
		}
	}
	return false
}

func pickTerminal(out io.Writer, installed map[string]bool) (tui.OneOfResult, error) {
	opts := []tui.OneOfOption{
		{Value: "", Label: "Keep current", Description: "no change"},
		{Value: "kitty", Label: "Kitty", Description: "GPU, native Wayland, built-in tabs/splits", Marker: markerFor(installed["kitty"])},
		{Value: "alacritty", Label: "Alacritty", Description: "leanest GPU-accelerated terminal", Marker: markerFor(installed["alacritty"])},
	}
	return tui.RunOneOf(context.Background(), "Terminal", "Which terminal emulator do you want?", opts, out)
}

func pickShell(out io.Writer, installed map[string]bool) (tui.OneOfResult, error) {
	opts := []tui.OneOfOption{
		{Value: "", Label: "Keep current", Description: "no change"},
		{Value: "zsh", Label: "Zsh", Description: "bash-compatible, large plugin ecosystem", Marker: markerFor(installed["zsh"])},
		{Value: "fish", Label: "Fish", Description: "user-friendly, smart autosuggestions", Marker: markerFor(installed["fish"])},
	}
	return tui.RunOneOf(context.Background(), "Shell", "Which interactive shell do you want installed?", opts, out)
}

func pickPrompt(out io.Writer, installed map[string]bool) (tui.OneOfResult, error) {
	opts := []tui.OneOfOption{
		{Value: "", Label: "Keep current", Description: "no change"},
		{Value: "starship", Label: "Starship", Description: "minimal, fast, cross-shell — starship.rs", Marker: markerFor(installed["starship"])},
	}
	return tui.RunOneOf(context.Background(), "Prompt", "Which prompt do you want installed?", opts, out)
}

// pickThemeForDesktop surfaces dark/light options gated by the detected
// desktop environment. Unsupported desktops (Sway, Hyprland, Pantheon,
// LXQt, Budgie, or unknown) get a silent no-op — the zero-value result
// tells the caller "no theme pick".
func pickThemeForDesktop(out io.Writer, installed map[string]bool) (tui.OneOfResult, error) {
	de := system.DetectDesktop()
	dark, light, descDark, descLight, ok := themeItemsForDesktop(de)
	if !ok {
		return tui.OneOfResult{Value: "", Label: "Keep current"}, nil
	}
	opts := []tui.OneOfOption{
		{Value: "", Label: "Keep current", Description: "no change"},
		{Value: dark, Label: "Dark", Description: descDark, Marker: markerFor(installed[dark])},
		{Value: light, Label: "Light", Description: descLight, Marker: markerFor(installed[light])},
	}
	return tui.RunOneOf(context.Background(), "Theme", "Dark or light?", opts, out)
}

// themeItemsForDesktop maps the detected desktop to the two manifest item
// IDs that apply dark / light mode for it. Returns ok=false when we
// haven't wired support for this desktop — the init wizard then skips
// the theme step silently.
func themeItemsForDesktop(de system.Desktop) (dark, light, descDark, descLight string, ok bool) {
	switch de {
	case system.DesktopGNOME:
		return "gnome-dark-mode", "gnome-light-mode",
			"prefer-dark + Adwaita-dark", "default color-scheme + Adwaita", true
	case system.DesktopKDE:
		return "kde-dark-mode", "kde-light-mode",
			"Breeze Dark via lookandfeeltool", "Breeze (light) via lookandfeeltool", true
	case system.DesktopXfce:
		return "xfce-dark-mode", "xfce-light-mode",
			"Adwaita-dark via xfconf-query", "Adwaita (light) via xfconf-query", true
	case system.DesktopMATE:
		return "mate-dark-mode", "mate-light-mode",
			"Yaru-dark via gsettings", "Yaru (light) via gsettings", true
	case system.DesktopCinnamon:
		return "cinnamon-dark-mode", "cinnamon-light-mode",
			"Mint-Y-Dark via gsettings", "Mint-Y via gsettings", true
	}
	return "", "", "", "", false
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
