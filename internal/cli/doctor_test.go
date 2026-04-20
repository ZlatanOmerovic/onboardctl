package cli

import (
	"os"
	"strings"
	"testing"
)

func TestDoctorReportAddUpdatesCounters(t *testing.T) {
	r := &DoctorReport{}
	r.Add(DoctorCheck{Status: CheckOK})
	r.Add(DoctorCheck{Status: CheckOK})
	r.Add(DoctorCheck{Status: CheckWarn})
	r.Add(DoctorCheck{Status: CheckFail})
	r.Add(DoctorCheck{Status: CheckInfo})

	if r.OKs != 2 {
		t.Errorf("OKs = %d, want 2", r.OKs)
	}
	if r.Warns != 1 {
		t.Errorf("Warns = %d, want 1", r.Warns)
	}
	if r.Fails != 1 {
		t.Errorf("Fails = %d, want 1", r.Fails)
	}
	if r.Infos != 1 {
		t.Errorf("Infos = %d, want 1", r.Infos)
	}
	if len(r.Checks) != 5 {
		t.Errorf("Checks len = %d, want 5", len(r.Checks))
	}
}

func TestCheckStyleCoversAllStatuses(t *testing.T) {
	for _, s := range []CheckStatus{CheckOK, CheckInfo, CheckWarn, CheckFail} {
		g, c := checkStyle(s)
		if g == "" || c == "" {
			t.Errorf("checkStyle(%v) returned empty values (glyph=%q, color=%q)", s, g, c)
		}
	}
}

func TestFormatCheckHonoursNoColor(t *testing.T) {
	// Temporarily override the global flag state.
	orig := noColorFlag
	defer func() { noColorFlag = orig }()

	c := DoctorCheck{Name: "Distro", Status: CheckOK, Message: "trixie"}

	noColorFlag = true
	plain := formatCheck(c)
	if strings.Contains(plain, "\x1b[") {
		t.Errorf("plain output contains ANSI escape: %q", plain)
	}
	if !strings.Contains(plain, "Distro") || !strings.Contains(plain, "trixie") {
		t.Errorf("plain output missing content: %q", plain)
	}

	noColorFlag = false
	// Ensure NO_COLOR env isn't forcing plain output for this assertion.
	origEnv, hadEnv := os.LookupEnv("NO_COLOR")
	_ = os.Unsetenv("NO_COLOR")
	defer func() {
		if hadEnv {
			os.Setenv("NO_COLOR", origEnv)
		}
	}()

	coloured := formatCheck(c)
	if !strings.Contains(coloured, "\x1b[") {
		t.Errorf("coloured output missing ANSI escape: %q", coloured)
	}
}

// Smoke-test the individual check functions. These talk to the real
// system, so they don't assert specifics — just that they don't panic
// and that they return a reasonable DoctorCheck with a Name set.
func TestIndividualChecksReturnNamed(t *testing.T) {
	checks := []struct {
		name string
		fn   func() DoctorCheck
	}{
		{"Distro", checkDistro},
		{"RequiredTools", checkRequiredTools},
		{"State", checkState},
		{"BundledManifest", checkBundledManifest},
		{"UserExtras", checkUserExtras},
		{"Flatpak", checkFlatpak},
		{"Sudo", checkSudo},
		{"BinaryOnPath", checkBinaryOnPath},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			got := c.fn()
			if got.Name == "" {
				t.Error("check returned empty Name")
			}
		})
	}
}
