package runner

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	"github.com/ZlatanOmerovic/onboardctl/internal/system"
)

// fakeExec captures invocations and returns scripted outputs.
type fakeExec struct {
	calls []string
	err   error
}

func (f *fakeExec) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	return nil, f.err
}

func newRB(t *testing.T, repos map[string]manifest.Repo) (*RepoBootstrapper, *fakeExec, string) {
	t.Helper()
	dir := t.TempDir()
	keyDir := filepath.Join(dir, "keyrings")
	srcDir := filepath.Join(dir, "sources.list.d")
	if err := os.MkdirAll(keyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fe := &fakeExec{}
	rb := &RepoBootstrapper{
		Repos:  repos,
		Runner: fe,
		Distro: system.Distro{ID: "debian", Family: "debian", Codename: "trixie", Arch: "amd64"},
		KeyDir: keyDir,
		SrcDir: srcDir,
		Out:    io.Discard,
		done:   make(map[string]bool),
	}
	return rb, fe, dir
}

func TestEnsureWritesKeyringAndSource(t *testing.T) {
	repos := map[string]manifest.Repo{
		"sury_php": {
			Kind:           "apt",
			Keyring:        "https://packages.sury.org/php/apt.gpg",
			KeyringDearmor: true,
			Source:         "deb [signed-by={keyring}] https://packages.sury.org/php/ {codename} main",
			When:           &manifest.When{DistroFamily: []string{"debian"}},
		},
	}
	rb, fe, dir := newRB(t, repos)
	env := Env{Distro: rb.Distro}

	added, err := rb.Ensure(context.Background(), "sury_php", env)
	if err != nil {
		t.Fatalf("Ensure error: %v", err)
	}
	if !added {
		t.Error("expected added=true for fresh bootstrap")
	}
	// Source file should exist with substituted template.
	src, err := os.ReadFile(filepath.Join(dir, "sources.list.d", "onboardctl-sury_php.list"))
	if err != nil {
		t.Fatalf("source file missing: %v", err)
	}
	if !strings.Contains(string(src), "trixie") {
		t.Errorf("codename not substituted: %s", src)
	}
	if !strings.Contains(string(src), filepath.Join(dir, "keyrings", "onboardctl-sury_php.gpg")) {
		t.Errorf("keyring path not substituted: %s", src)
	}
	// Exec was asked to bash-run a curl | gpg pipeline.
	if len(fe.calls) != 1 || !strings.HasPrefix(fe.calls[0], "bash -c ") {
		t.Errorf("unexpected exec calls: %v", fe.calls)
	}
	if !strings.Contains(fe.calls[0], "gpg --dearmor") {
		t.Errorf("expected dearmor in exec: %s", fe.calls[0])
	}
}

func TestEnsureSkipsWhenAlreadyOnDisk(t *testing.T) {
	repos := map[string]manifest.Repo{
		"sury_php": {Kind: "apt", Keyring: "url", Source: "deb foo main"},
	}
	rb, fe, dir := newRB(t, repos)

	// Pre-populate the expected files.
	keyPath := filepath.Join(dir, "keyrings", "onboardctl-sury_php.gpg")
	srcPath := filepath.Join(dir, "sources.list.d", "onboardctl-sury_php.list")
	if err := os.WriteFile(keyPath, []byte("fake key"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(srcPath, []byte("deb foo main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	added, err := rb.Ensure(context.Background(), "sury_php", Env{Distro: rb.Distro})
	if err != nil {
		t.Fatalf("Ensure error: %v", err)
	}
	if added {
		t.Error("expected added=false when already on disk")
	}
	if len(fe.calls) != 0 {
		t.Errorf("expected no exec calls, got: %v", fe.calls)
	}
}

func TestEnsureSkipsWhenWhenFails(t *testing.T) {
	// Ubuntu-only repo on a Debian box should no-op.
	repos := map[string]manifest.Repo{
		"ubuntu_only": {
			Kind: "apt", Keyring: "url", Source: "deb foo main",
			When: &manifest.When{DistroID: []string{"ubuntu"}},
		},
	}
	rb, fe, _ := newRB(t, repos)

	added, err := rb.Ensure(context.Background(), "ubuntu_only", Env{Distro: rb.Distro})
	if err != nil {
		t.Fatalf("Ensure error: %v", err)
	}
	if added {
		t.Error("should not be added when When rejects env")
	}
	if len(fe.calls) != 0 {
		t.Errorf("expected no exec, got: %v", fe.calls)
	}
}

func TestEnsureUnknownRepoIsError(t *testing.T) {
	rb, _, _ := newRB(t, map[string]manifest.Repo{})
	_, err := rb.Ensure(context.Background(), "nonesuch", Env{Distro: rb.Distro})
	if err == nil {
		t.Fatal("expected error for unknown repo name")
	}
}

func TestEnsureIsIdempotentWithinRun(t *testing.T) {
	repos := map[string]manifest.Repo{
		"r": {Kind: "apt", Keyring: "url", Source: "deb foo main"},
	}
	rb, fe, _ := newRB(t, repos)
	env := Env{Distro: rb.Distro}

	if _, err := rb.Ensure(context.Background(), "r", env); err != nil {
		t.Fatal(err)
	}
	callsAfterFirst := len(fe.calls)
	if _, err := rb.Ensure(context.Background(), "r", env); err != nil {
		t.Fatal(err)
	}
	if len(fe.calls) != callsAfterFirst {
		t.Errorf("second Ensure re-ran exec: %v", fe.calls)
	}
}

func TestAptUpdateIfNeeded(t *testing.T) {
	repos := map[string]manifest.Repo{
		"r": {Kind: "apt", Keyring: "url", Source: "deb foo main"},
	}
	rb, fe, _ := newRB(t, repos)
	env := Env{Distro: rb.Distro}

	// No new repos yet → no update.
	if err := rb.AptUpdateIfNeeded(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(fe.calls) != 0 {
		t.Errorf("unexpected exec: %v", fe.calls)
	}

	// After Ensure bootstraps, AptUpdateIfNeeded should run.
	if _, err := rb.Ensure(context.Background(), "r", env); err != nil {
		t.Fatal(err)
	}
	fe.calls = nil
	if err := rb.AptUpdateIfNeeded(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(fe.calls) != 1 || fe.calls[0] != "apt-get update" {
		t.Errorf("expected apt-get update, got: %v", fe.calls)
	}
}

func TestAptUpdateBubblesError(t *testing.T) {
	repos := map[string]manifest.Repo{
		"r": {Kind: "apt", Keyring: "url", Source: "deb foo main"},
	}
	rb, fe, _ := newRB(t, repos)
	env := Env{Distro: rb.Distro}
	if _, err := rb.Ensure(context.Background(), "r", env); err != nil {
		t.Fatal(err)
	}
	fe.err = errors.New("network down")
	if err := rb.AptUpdateIfNeeded(context.Background()); err == nil {
		t.Error("expected error propagation")
	}
}
