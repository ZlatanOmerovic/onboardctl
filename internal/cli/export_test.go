package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/ZlatanOmerovic/onboardctl/internal/state"
	"github.com/ZlatanOmerovic/onboardctl/internal/system"
)

func mkStateWithTwoItems() *state.State {
	st := state.New()
	st.Profile = "fullstack-web"
	ts := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	st.RecordInstall("jq", "apt", "1.7.1", state.ByOnboardctl, ts)
	st.RecordInstall("vlc", "apt", "3.0.23", state.ByOnboardctl, ts)
	// One external item we shouldn't export by default.
	st.Items["chromium"] = state.Record{
		Status: state.StatusInstalled, Provider: "apt", Version: "147.0",
		InstalledBy: state.ByExternal, LastRun: ts,
	}
	return st
}

func TestCollectExportItemsDefault(t *testing.T) {
	st := mkStateWithTwoItems()
	got := collectExportItems(st, false)
	want := []string{"jq", "vlc"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i, id := range got {
		if id != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, id, want[i])
		}
	}
}

func TestCollectExportItemsIncludeExternal(t *testing.T) {
	st := mkStateWithTwoItems()
	got := collectExportItems(st, true)
	want := []string{"chromium", "jq", "vlc"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (%v)", len(got), len(want), got)
	}
}

func TestWriteExportYAMLFormat(t *testing.T) {
	st := mkStateWithTwoItems()
	d := system.Distro{Name: "Debian GNU/Linux", Version: "13", Codename: "trixie", Arch: "amd64"}
	var buf bytes.Buffer
	writeExportYAML(&buf, []string{"jq", "vlc"}, d, st)
	out := buf.String()

	expected := []string{
		"# onboardctl export",
		"# Distro:",
		"# Profile: fullstack-web",
		"version: 1",
		"items:",
		"  - jq",
		"  - vlc",
	}
	for _, sub := range expected {
		if !strings.Contains(out, sub) {
			t.Errorf("yaml missing %q:\n%s", sub, out)
		}
	}
}

func TestWriteExportListFormat(t *testing.T) {
	var buf bytes.Buffer
	writeExportList(&buf, []string{"jq", "vlc"})
	want := "jq\nvlc\n"
	if buf.String() != want {
		t.Errorf("list format = %q, want %q", buf.String(), want)
	}
}
