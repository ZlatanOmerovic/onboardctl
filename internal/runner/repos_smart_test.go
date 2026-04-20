package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
)

func TestEnsureSkipsWhenUserManagedRepoCoversURL(t *testing.T) {
	// User already has /etc/apt/sources.list.d/github-cli.list referencing
	// https://cli.github.com/packages. Our manifest wants the same URL.
	repos := map[string]manifest.Repo{
		"github_cli": {
			Kind:    "apt",
			Keyring: "https://cli.github.com/packages/githubcli-archive-keyring.gpg",
			Source:  "deb [arch=amd64 signed-by={keyring}] https://cli.github.com/packages stable main",
		},
	}
	rb, fe, dir := newRB(t, repos)

	// Pre-populate a user-managed .list file with the same URL.
	userFile := filepath.Join(dir, "sources.list.d", "github-cli.list")
	if err := os.WriteFile(userFile,
		[]byte("deb [signed-by=/usr/share/keyrings/githubcli.gpg] https://cli.github.com/packages stable main\n"),
		0o644); err != nil {
		t.Fatal(err)
	}

	added, err := rb.Ensure(context.Background(), "github_cli", Env{Distro: rb.Distro})
	if err != nil {
		t.Fatalf("Ensure error: %v", err)
	}
	if added {
		t.Error("expected added=false when user already manages this repo")
	}
	if len(fe.calls) != 0 {
		t.Errorf("expected no exec (no download/dearmor needed), got %v", fe.calls)
	}
	// Make sure we didn't write our own .list alongside.
	ourFile := filepath.Join(dir, "sources.list.d", "onboardctl-github_cli.list")
	if _, err := os.Stat(ourFile); err == nil {
		t.Error("onboardctl-github_cli.list was written despite user-managed equivalent")
	}
}

func TestEnsureProceedsWhenUserFileIsForDifferentURL(t *testing.T) {
	repos := map[string]manifest.Repo{
		"github_cli": {
			Kind:    "apt",
			Keyring: "https://cli.github.com/packages/githubcli-archive-keyring.gpg",
			Source:  "deb [signed-by={keyring}] https://cli.github.com/packages stable main",
		},
	}
	rb, fe, dir := newRB(t, repos)

	// User has a file for a different repo — should NOT block our bootstrap.
	if err := os.WriteFile(
		filepath.Join(dir, "sources.list.d", "docker.list"),
		[]byte("deb [signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/debian bookworm stable\n"),
		0o644); err != nil {
		t.Fatal(err)
	}

	added, err := rb.Ensure(context.Background(), "github_cli", Env{Distro: rb.Distro})
	if err != nil {
		t.Fatalf("Ensure error: %v", err)
	}
	if !added {
		t.Error("expected added=true when no matching user file is present")
	}
	if len(fe.calls) == 0 {
		t.Error("expected exec calls (curl+dearmor) to run for fresh bootstrap")
	}
}

func TestEnsureIgnoresOwnOnboardctlFiles(t *testing.T) {
	// Onboardctl-prefixed files from a previous partial run shouldn't
	// be mistaken for user-managed coverage — they're handled by the
	// fileExists check at the top of Ensure.
	repos := map[string]manifest.Repo{
		"r": {Kind: "apt", Keyring: "http://x", Source: "deb https://example.com/r stable main"},
	}
	rb, fe, dir := newRB(t, repos)
	if err := os.WriteFile(
		filepath.Join(dir, "sources.list.d", "onboardctl-leftover.list"),
		[]byte("deb https://example.com/r stable main\n"),
		0o644); err != nil {
		t.Fatal(err)
	}
	added, err := rb.Ensure(context.Background(), "r", Env{Distro: rb.Distro})
	if err != nil {
		t.Fatalf("Ensure error: %v", err)
	}
	if !added {
		t.Error("expected added=true; onboardctl-* files should not satisfy coverage")
	}
	_ = fe
}

func TestExtractRepoURL(t *testing.T) {
	cases := map[string]string{
		"deb [signed-by=/etc/apt/keyrings/x.gpg] https://packages.sury.org/php/ trixie main": "https://packages.sury.org/php",
		"deb http://deb.anydesk.com/ all main":                                               "http://deb.anydesk.com",
		"deb [arch=amd64] https://cli.github.com/packages stable main":                       "https://cli.github.com/packages",
		"# commented out":                                                                    "",
		"":                                                                                   "",
	}
	for in, want := range cases {
		if got := extractRepoURL(in); got != want {
			t.Errorf("extractRepoURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFindExistingRepoFor_Deb822Sources(t *testing.T) {
	// User manages chrome via the newer deb822-format .sources file.
	repos := map[string]manifest.Repo{
		"chrome": {
			Kind:    "apt",
			Keyring: "https://dl.google.com/linux/linux_signing_key.pub",
			Source:  "deb [signed-by={keyring}] https://dl.google.com/linux/chrome/deb stable main",
		},
	}
	rb, _, dir := newRB(t, repos)
	deb822 := `
Types: deb
URIs: https://dl.google.com/linux/chrome/deb
Suites: stable
Components: main
Signed-By: /usr/share/keyrings/google-chrome.gpg
`
	if err := os.WriteFile(
		filepath.Join(dir, "sources.list.d", "google-chrome.sources"),
		[]byte(deb822), 0o644); err != nil {
		t.Fatal(err)
	}
	path, ok := rb.findExistingRepoFor(repos["chrome"])
	if !ok {
		t.Fatal("expected match inside .sources file")
	}
	if !strings.HasSuffix(path, "google-chrome.sources") {
		t.Errorf("match file = %q, want suffix google-chrome.sources", path)
	}
}
