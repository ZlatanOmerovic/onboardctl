package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	"github.com/ZlatanOmerovic/onboardctl/internal/state"
	"github.com/ZlatanOmerovic/onboardctl/internal/system"
)

// CheckStatus classifies one doctor result.
type CheckStatus int

const (
	CheckOK CheckStatus = iota
	CheckInfo
	CheckWarn
	CheckFail
)

// DoctorCheck is one row in the doctor report.
type DoctorCheck struct {
	Name    string
	Status  CheckStatus
	Message string
}

// DoctorReport aggregates all checks for the final summary and exit code.
type DoctorReport struct {
	Checks []DoctorCheck
	OKs    int
	Infos  int
	Warns  int
	Fails  int
}

// Add records a check and updates counters.
func (r *DoctorReport) Add(c DoctorCheck) {
	r.Checks = append(r.Checks, c)
	switch c.Status {
	case CheckOK:
		r.OKs++
	case CheckInfo:
		r.Infos++
	case CheckWarn:
		r.Warns++
	case CheckFail:
		r.Fails++
	}
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run environment sanity checks",
	Long: `doctor runs a short battery of checks against the current machine
to catch common problems before you hit them during an install:

  * distro detection and family (Debian / Ubuntu / Mint / Pop!_OS / etc.)
  * required CLI tools (curl, gpg, dpkg-query, apt-get, tar, bash)
  * state-file path readable/writable
  * bundled manifest self-validates against the JSON Schema
  * user extras (if present) passes lint
  * flatpak binary present (if any manifest items use it)
  * sudo usability (NOPASSWD for apt, or already root)
  * onboardctl binary is on PATH

Exits 0 on all-green, 1 on any failure. Warnings and informational lines
don't affect exit code.`,
	Args: cobra.NoArgs,
	RunE: runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()

	rep := &DoctorReport{}
	rep.Add(checkDistro())
	rep.Add(checkRequiredTools())
	rep.Add(checkState())
	rep.Add(checkBundledManifest())
	rep.Add(checkUserExtras())
	rep.Add(checkFlatpak())
	rep.Add(checkSudo())
	rep.Add(checkBinaryOnPath())

	fmt.Fprintln(out, "onboardctl doctor")
	fmt.Fprintln(out)
	for _, c := range rep.Checks {
		fmt.Fprintln(out, formatCheck(c))
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Summary: %d OK · %d info · %d warn · %d fail\n",
		rep.OKs, rep.Infos, rep.Warns, rep.Fails)

	maybePrintUpdateNotice(out)

	if rep.Fails > 0 {
		return fmt.Errorf("%d check(s) failed", rep.Fails)
	}
	return nil
}

// ---- Individual checks ----------------------------------------------------

func checkDistro() DoctorCheck {
	d, err := system.DetectDistro()
	if err != nil {
		return DoctorCheck{Name: "Distro", Status: CheckFail, Message: err.Error()}
	}
	if !d.InDebianFamily() {
		return DoctorCheck{
			Name:    "Distro",
			Status:  CheckFail,
			Message: fmt.Sprintf("%s (not in Debian family — onboardctl is Debian-based only)", d.Name),
		}
	}
	return DoctorCheck{
		Name:    "Distro",
		Status:  CheckOK,
		Message: fmt.Sprintf("%s %s (%s) %s", d.Name, d.Version, d.Codename, d.Arch),
	}
}

func checkRequiredTools() DoctorCheck {
	need := []string{"curl", "gpg", "dpkg-query", "apt-get", "tar", "bash"}
	var missing []string
	for _, tool := range need {
		if _, err := exec.LookPath(tool); err != nil {
			missing = append(missing, tool)
		}
	}
	if len(missing) == 0 {
		return DoctorCheck{
			Name:    "Required tools",
			Status:  CheckOK,
			Message: "all present (" + strings.Join(need, ", ") + ")",
		}
	}
	return DoctorCheck{
		Name:    "Required tools",
		Status:  CheckFail,
		Message: "missing: " + strings.Join(missing, ", "),
	}
}

func checkState() DoctorCheck {
	path := state.DefaultPath()
	if path == "" {
		return DoctorCheck{Name: "State file", Status: CheckFail, Message: "no XDG_STATE_HOME or HOME set"}
	}
	// If it exists, try reading.
	if _, err := os.Stat(path); err == nil {
		if _, err := state.Load(path); err != nil {
			return DoctorCheck{Name: "State file", Status: CheckFail,
				Message: fmt.Sprintf("%s: %v", path, err)}
		}
		return DoctorCheck{Name: "State file", Status: CheckOK, Message: path + " (readable)"}
	}
	// Doesn't exist yet — check that parent dir is creatable (by trying MkdirAll with test path).
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return DoctorCheck{Name: "State file", Status: CheckFail,
			Message: fmt.Sprintf("cannot create %s: %v", dir, err)}
	}
	return DoctorCheck{Name: "State file", Status: CheckInfo, Message: path + " (will be created on first install)"}
}

func checkBundledManifest() DoctorCheck {
	m, err := manifest.LoadBundled()
	if err != nil {
		return DoctorCheck{Name: "Bundled manifest", Status: CheckFail, Message: err.Error()}
	}
	return DoctorCheck{
		Name:   "Bundled manifest",
		Status: CheckOK,
		Message: fmt.Sprintf("v%d — %d profiles, %d bundles, %d items, %d repos",
			m.Version, len(m.Profiles), len(m.Bundles), len(m.Items), len(m.Repos)),
	}
}

func checkUserExtras() DoctorCheck {
	path := manifest.DefaultExtrasPath()
	if path == "" {
		return DoctorCheck{Name: "User extras", Status: CheckInfo, Message: "no XDG_CONFIG_HOME or HOME set"}
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return DoctorCheck{Name: "User extras", Status: CheckInfo, Message: "not present (optional) — " + path}
	}
	if err := manifest.Lint(path); err != nil {
		return DoctorCheck{Name: "User extras", Status: CheckWarn,
			Message: fmt.Sprintf("%s failed lint (run 'onboardctl lint' for details)", path)}
	}
	return DoctorCheck{Name: "User extras", Status: CheckOK, Message: path + " (schema-valid)"}
}

func checkFlatpak() DoctorCheck {
	m, err := manifest.LoadBundled()
	if err != nil {
		return DoctorCheck{Name: "Flatpak", Status: CheckInfo, Message: "skipped (bundled manifest unreadable)"}
	}
	usesFlatpak := false
	for _, it := range m.Items {
		for _, p := range it.Providers {
			if p.Type == manifest.KindFlatpak {
				usesFlatpak = true
				break
			}
		}
		if usesFlatpak {
			break
		}
	}
	if !usesFlatpak {
		return DoctorCheck{Name: "Flatpak", Status: CheckInfo, Message: "no flatpak-kind items in manifest"}
	}
	if _, err := exec.LookPath("flatpak"); err != nil {
		return DoctorCheck{Name: "Flatpak", Status: CheckWarn,
			Message: "binary not installed — flatpak-kind items will fail (install with 'sudo apt install flatpak')"}
	}
	return DoctorCheck{Name: "Flatpak", Status: CheckOK, Message: "binary available"}
}

func checkSudo() DoctorCheck {
	if os.Geteuid() == 0 {
		return DoctorCheck{Name: "Privileges", Status: CheckOK, Message: "running as root"}
	}
	// Try a passwordless sudo on apt-get (harmless introspection).
	cmd := exec.Command("sudo", "-n", "apt-get", "--version")
	if err := cmd.Run(); err == nil {
		return DoctorCheck{Name: "Privileges", Status: CheckOK, Message: "NOPASSWD for apt-get (apply mode works)"}
	}
	return DoctorCheck{Name: "Privileges", Status: CheckWarn,
		Message: "not root and no NOPASSWD for apt-get — apply mode will need 'sudo onboardctl ...'"}
}

func checkBinaryOnPath() DoctorCheck {
	path, err := exec.LookPath("onboardctl")
	if err != nil {
		return DoctorCheck{Name: "Binary on PATH", Status: CheckWarn,
			Message: "not found (drop the binary in $HOME/.local/bin and ensure PATH picks it up)"}
	}
	return DoctorCheck{Name: "Binary on PATH", Status: CheckOK, Message: path}
}

// ---- Rendering ------------------------------------------------------------

const (
	ansiReset  = "\x1b[0m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
	ansiDim    = "\x1b[2m"
)

func formatCheck(c DoctorCheck) string {
	glyph, color := checkStyle(c.Status)
	if ColorEnabled() {
		return fmt.Sprintf("  %s%s%s %-22s %s", color, glyph, ansiReset, c.Name, c.Message)
	}
	return fmt.Sprintf("  %s %-22s %s", glyph, c.Name, c.Message)
}

func checkStyle(s CheckStatus) (glyph, color string) {
	switch s {
	case CheckOK:
		return "✓", ansiGreen
	case CheckInfo:
		return "○", ansiDim
	case CheckWarn:
		return "⚠", ansiYellow
	case CheckFail:
		return "✗", ansiRed
	}
	return "?", ansiDim
}

// private helper: allow io.Writer for testing, though runDoctor uses cobra's OutOrStdout.
var _ io.Writer
