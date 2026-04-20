// Package state persists what onboardctl has installed on this machine.
//
// The file lives at $XDG_STATE_HOME/onboardctl/state.yaml (or
// ~/.local/state/onboardctl/state.yaml). It is YAML so a user can inspect
// and hand-edit it in an emergency, but routine reads/writes go through
// this package.
//
// State is additive-only. onboardctl never removes items from it except
// when the user explicitly calls `onboardctl forget <item>` (not in Phase 2).
package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// SchemaVersion is the state-file format version. Bumped when the shape
// of State changes incompatibly.
const SchemaVersion = 1

// InstalledBy values.
const (
	ByOnboardctl = "onboardctl"
	ByExternal   = "external"
)

// ItemStatus values.
const (
	StatusInstalled      = "installed"
	StatusDetectedExtern = "detected-external"
	StatusFailed         = "failed"
	StatusPending        = "pending"
)

// State is the top-level document persisted to disk.
type State struct {
	Version int               `yaml:"version"`
	Profile string            `yaml:"profile,omitempty"` // last-selected profile, if any
	Distro  DistroSnapshot    `yaml:"distro"`
	Runs    []Run             `yaml:"runs,omitempty"`
	Items   map[string]Record `yaml:"items,omitempty"`
}

// DistroSnapshot captures the machine fingerprint at the time of install.
type DistroSnapshot struct {
	ID       string `yaml:"id"`
	Codename string `yaml:"codename,omitempty"`
	Version  string `yaml:"version,omitempty"`
	Family   string `yaml:"family,omitempty"`
	Arch     string `yaml:"arch,omitempty"`
}

// Run records one invocation of the install pipeline.
type Run struct {
	StartedAt         time.Time `yaml:"started_at"`
	CompletedAt       time.Time `yaml:"completed_at,omitempty"`
	Profile           string    `yaml:"profile,omitempty"`
	Selection         []string  `yaml:"selection,omitempty"` // item IDs chosen for this run
	DryRun            bool      `yaml:"dry_run,omitempty"`
	OnboardctlVersion string    `yaml:"onboardctl_version,omitempty"`
}

// Record is the per-item install history line.
type Record struct {
	Status      string    `yaml:"status"`
	Provider    string    `yaml:"provider"`
	Version     string    `yaml:"version,omitempty"`
	InstalledBy string    `yaml:"installed_by"`
	LastRun     time.Time `yaml:"last_run"`
	Note        string    `yaml:"note,omitempty"`
}

// New returns an empty, current-version state.
func New() *State {
	return &State{
		Version: SchemaVersion,
		Items:   make(map[string]Record),
	}
}

// DefaultPath returns the XDG-correct state-file path or "" if neither
// $XDG_STATE_HOME nor $HOME are resolvable.
func DefaultPath() string {
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "onboardctl", "state.yaml")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".local", "state", "onboardctl", "state.yaml")
}

// Load reads state from path (or DefaultPath() if empty). A missing file
// returns a fresh empty State, not an error — first runs are legitimate.
func Load(path string) (*State, error) {
	if path == "" {
		path = DefaultPath()
	}
	if path == "" {
		return nil, errors.New("cannot resolve state path (no XDG_STATE_HOME or HOME)")
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return New(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state %s: %w", path, err)
	}

	// Walk migrations if the on-disk file is older than SchemaVersion.
	// migrate() is a no-op for current-version docs but gives us a
	// pre-wired upgrade path for the first schema bump.
	migrated, _, err := migrate(data)
	if err != nil {
		return nil, fmt.Errorf("migrate state %s: %w", path, err)
	}

	s := &State{}
	if err := yaml.Unmarshal(migrated, s); err != nil {
		return nil, fmt.Errorf("parse state %s: %w", path, err)
	}
	if s.Version == 0 {
		s.Version = SchemaVersion
	}
	if s.Items == nil {
		s.Items = make(map[string]Record)
	}
	return s, nil
}

// Save writes state to path (or DefaultPath() if empty), creating parent
// directories as needed. Writes are atomic via a temp-file + rename.
func Save(path string, s *State) error {
	if path == "" {
		path = DefaultPath()
	}
	if path == "" {
		return errors.New("cannot resolve state path (no XDG_STATE_HOME or HOME)")
	}
	if s == nil {
		return errors.New("nil state")
	}
	if s.Items == nil {
		s.Items = make(map[string]Record)
	}
	if s.Version == 0 {
		s.Version = SchemaVersion
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".state-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("tempfile: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }() // no-op after successful rename

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", tmpPath, path, err)
	}
	return nil
}

// RecordInstall updates the map with a successful install.
// ts is allowed to be the zero value — in that case time.Now() is used.
func (s *State) RecordInstall(itemID, providerKind, version, installedBy string, ts time.Time) {
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	if s.Items == nil {
		s.Items = make(map[string]Record)
	}
	s.Items[itemID] = Record{
		Status:      StatusInstalled,
		Provider:    providerKind,
		Version:     version,
		InstalledBy: installedBy,
		LastRun:     ts,
	}
}

// RecordFailure marks an item failed with an explanatory note.
func (s *State) RecordFailure(itemID, providerKind, note string, ts time.Time) {
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	if s.Items == nil {
		s.Items = make(map[string]Record)
	}
	s.Items[itemID] = Record{
		Status:   StatusFailed,
		Provider: providerKind,
		LastRun:  ts,
		Note:     note,
	}
}

// AppendRun adds a Run to the tail, bounded to the last 20 entries.
func (s *State) AppendRun(r Run) {
	s.Runs = append(s.Runs, r)
	const keep = 20
	if len(s.Runs) > keep {
		s.Runs = s.Runs[len(s.Runs)-keep:]
	}
}
