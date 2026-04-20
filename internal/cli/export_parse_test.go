package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseExportYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "export.yaml")
	content := `# onboardctl export — 2026-04-20T20:30:00Z
# Distro: Debian GNU/Linux 13 (trixie) amd64
# Profile: fullstack-web
version: 1
items:
  - jq
  - vlc
  - nodejs
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := parseExportFile(path)
	if err != nil {
		t.Fatalf("parseExportFile: %v", err)
	}
	want := []string{"jq", "vlc", "nodejs"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i, id := range got {
		if id != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, id, want[i])
		}
	}
}

func TestParseExportList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "list.txt")
	content := `# some comment
jq

# another comment
vlc
nodejs
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := parseExportFile(path)
	if err != nil {
		t.Fatalf("parseExportFile: %v", err)
	}
	want := []string{"jq", "vlc", "nodejs"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (%v)", len(got), len(want), got)
	}
}

func TestParseExportFormatSniff(t *testing.T) {
	cases := map[string]bool{
		"version: 1\nitems:\n  - a\n": true,
		"items:\n  - a\n":             true,
		"# comment only\n":            false,
		"jq\nvlc\n":                   false,
		"  # indented comment\njq\n":  false,
	}
	for in, want := range cases {
		if got := looksLikeExportYAML([]byte(in)); got != want {
			t.Errorf("looksLikeExportYAML(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseExportYAMLEmptyItemsErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.yaml")
	if err := os.WriteFile(path, []byte("version: 1\nitems: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := parseExportFile(path)
	if err == nil {
		t.Error("expected error for empty items list")
	}
}
