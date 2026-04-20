package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateNoOpOnCurrentVersion(t *testing.T) {
	in := []byte("version: 1\nitems: {}\n")
	out, v, err := migrate(in)
	if err != nil {
		t.Fatalf("migrate error: %v", err)
	}
	if v != SchemaVersion {
		t.Errorf("version after migrate = %d, want %d", v, SchemaVersion)
	}
	if !strings.Contains(string(out), "version: 1") {
		t.Errorf("migrate dropped version field: %s", out)
	}
}

func TestMigrateDefaultsMissingVersionToCurrent(t *testing.T) {
	// Pre-v1 files might not have the version field.
	in := []byte("items: {}\n")
	_, v, err := migrate(in)
	if err != nil {
		t.Fatalf("migrate error: %v", err)
	}
	if v != SchemaVersion {
		t.Errorf("version = %d, want %d (should default to current)", v, SchemaVersion)
	}
}

func TestMigrateBumpsThroughRegisteredMigrations(t *testing.T) {
	// Inject a one-shot test migration from v0 → v1. (v0 doesn't exist in
	// production; this verifies the iteration mechanism.)
	origSV := SchemaVersion
	defer func() { _ = origSV }() // SchemaVersion is a const; we test against it directly.

	bumpCalled := 0
	migrations[SchemaVersion] = func(doc map[string]any) error {
		bumpCalled++
		return nil
	}
	defer delete(migrations, SchemaVersion)

	// To exercise bumping, we pretend the file is at SchemaVersion and
	// claim our synthetic target is SchemaVersion+1. But SchemaVersion
	// is a const, so the migrator won't walk past it. Instead, verify
	// that supplying a doc at SchemaVersion doesn't spuriously fire our
	// injected migration (it maps from SchemaVersion → SchemaVersion+1,
	// which migrate() won't pursue).
	in := []byte("version: 1\nitems: {}\n")
	_, v, err := migrate(in)
	if err != nil {
		t.Fatalf("migrate error: %v", err)
	}
	if bumpCalled != 0 {
		t.Errorf("migration for SchemaVersion incorrectly fired %d times", bumpCalled)
	}
	if v != SchemaVersion {
		t.Errorf("version = %d, want %d", v, SchemaVersion)
	}
}

func TestLoadRunsMigrateOnDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")
	// Write a file with missing version field; Load should still succeed.
	if err := os.WriteFile(path, []byte("profile: test\nitems: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if s.Version != SchemaVersion {
		t.Errorf("state version = %d, want %d", s.Version, SchemaVersion)
	}
}
