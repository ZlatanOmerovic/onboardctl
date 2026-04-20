package manifest

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

//go:embed assets/default.yaml
var bundledManifest []byte

// Load returns the live manifest: bundled defaults merged with user extras.
//
// If extrasPath is "", the default user location (XDG_CONFIG_HOME/onboardctl/extras.yaml
// or ~/.config/onboardctl/extras.yaml) is consulted. A missing extras file is not
// an error — the bundled manifest is returned unchanged.
func Load(extrasPath string) (*Manifest, error) {
	base, err := LoadBundled()
	if err != nil {
		return nil, fmt.Errorf("bundled manifest: %w", err)
	}

	if extrasPath == "" {
		extrasPath = DefaultExtrasPath()
	}
	if extrasPath == "" {
		return base, nil
	}

	if _, err := os.Stat(extrasPath); errors.Is(err, os.ErrNotExist) {
		return base, nil
	} else if err != nil {
		return nil, fmt.Errorf("stat extras %s: %w", extrasPath, err)
	}

	extras, err := LoadFromFile(extrasPath)
	if err != nil {
		return nil, fmt.Errorf("extras %s: %w", extrasPath, err)
	}
	return Merge(base, extras), nil
}

// LoadBundled parses the embedded default manifest.
func LoadBundled() (*Manifest, error) {
	m := &Manifest{}
	if err := yaml.Unmarshal(bundledManifest, m); err != nil {
		return nil, fmt.Errorf("unmarshal bundled: %w", err)
	}
	if m.Version == 0 {
		return nil, errors.New("bundled manifest has no version field")
	}
	return m, nil
}

// LoadFromFile parses a manifest file from disk.
func LoadFromFile(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	m := &Manifest{}
	if err := yaml.Unmarshal(data, m); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", path, err)
	}
	return m, nil
}

// DefaultExtrasPath returns the XDG-correct path for the user's extras file.
// Returns "" if neither $XDG_CONFIG_HOME nor $HOME are resolvable.
func DefaultExtrasPath() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "onboardctl", "extras.yaml")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "onboardctl", "extras.yaml")
}
