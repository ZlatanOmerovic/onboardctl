package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadMissingFileReturnsEmptyState(t *testing.T) {
	dir := t.TempDir()
	s, err := Load(filepath.Join(dir, "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if s == nil || s.Version != SchemaVersion {
		t.Errorf("want empty state with Version=%d, got %+v", SchemaVersion, s)
	}
	if s.Items == nil {
		t.Error("Items map must be initialised")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "state.yaml")

	s := New()
	s.Distro = DistroSnapshot{ID: "debian", Codename: "trixie", Version: "13", Family: "debian", Arch: "amd64"}
	s.Profile = "fullstack-web"
	ts := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	s.RecordInstall("jq", "apt", "1.7.1-6", ByOnboardctl, ts)
	s.RecordFailure("lazygit", "binary_release", "rate limit", ts)
	s.AppendRun(Run{StartedAt: ts, CompletedAt: ts, Profile: "fullstack-web", Selection: []string{"jq"}})

	if err := Save(path, s); err != nil {
		t.Fatalf("Save error: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file at %s: %v", path, err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if loaded.Version != SchemaVersion {
		t.Errorf("version = %d, want %d", loaded.Version, SchemaVersion)
	}
	if loaded.Distro.ID != "debian" || loaded.Distro.Codename != "trixie" {
		t.Errorf("distro roundtrip lost: %+v", loaded.Distro)
	}
	rec, ok := loaded.Items["jq"]
	if !ok {
		t.Fatal("jq record missing after roundtrip")
	}
	if rec.Status != StatusInstalled || rec.Provider != "apt" || rec.Version != "1.7.1-6" {
		t.Errorf("jq record drifted: %+v", rec)
	}
	if loaded.Items["lazygit"].Status != StatusFailed {
		t.Errorf("lazygit status = %q, want %q", loaded.Items["lazygit"].Status, StatusFailed)
	}
	if len(loaded.Runs) != 1 {
		t.Errorf("runs len = %d, want 1", len(loaded.Runs))
	}
}

func TestAppendRunBounds(t *testing.T) {
	s := New()
	for i := 0; i < 30; i++ {
		s.AppendRun(Run{StartedAt: time.Unix(int64(i), 0)})
	}
	if got := len(s.Runs); got != 20 {
		t.Errorf("runs cap = %d, want 20", got)
	}
	// Last run should be i=29 (StartedAt=unix(29))
	if s.Runs[len(s.Runs)-1].StartedAt.Unix() != 29 {
		t.Errorf("tail run drifted: %v", s.Runs[len(s.Runs)-1])
	}
}

func TestSaveIsAtomic(t *testing.T) {
	// After Save, the temp files should not linger.
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")
	s := New()
	if err := Save(path, s); err != nil {
		t.Fatalf("Save: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "state.yaml" {
			t.Errorf("stray file after Save: %s", e.Name())
		}
	}
}
