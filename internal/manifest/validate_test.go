package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLintRejectsMissingRequiredField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	// Missing "providers" on an item.
	if err := os.WriteFile(path, []byte(`
version: 1
items:
  foo:
    name: Foo
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Lint(path); err == nil {
		t.Fatal("expected lint error, got nil")
	}
}

func TestLintRejectsUnknownProviderType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte(`
version: 1
items:
  foo:
    name: Foo
    providers:
      - type: not_a_real_provider_kind
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Lint(path); err == nil {
		t.Fatal("expected lint error for bad provider type, got nil")
	}
}

func TestLintAcceptsValidManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "good.yaml")
	if err := os.WriteFile(path, []byte(`
version: 1
items:
  htop:
    name: htop
    description: Process viewer
    providers:
      - type: apt
        package: htop
bundles:
  monitoring:
    name: Monitoring
    items: [htop]
profiles:
  minimal:
    name: Minimal
    bundles: [monitoring]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Lint(path); err != nil {
		t.Fatalf("expected lint to pass, got: %v", err)
	}
}

func TestLintBundledDefaultPasses(t *testing.T) {
	// Write bundled yaml to a temp file and lint it — proves the bundled
	// manifest stays schema-valid as the file evolves.
	dir := t.TempDir()
	path := filepath.Join(dir, "default.yaml")
	if err := os.WriteFile(path, bundledManifest, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Lint(path); err != nil {
		t.Fatalf("bundled manifest failed schema validation: %v", err)
	}
}
