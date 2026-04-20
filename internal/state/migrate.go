package state

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Migration transforms a raw YAML document from one schema version to
// the next. It operates on a map[string]any (the decoded YAML tree) so
// migrations can add, remove, or rename fields without needing parallel
// Go structs for every historical shape.
//
// Current behaviour: no migrations are registered because v1 is the
// first version. The plumbing is in place so the next schema-breaking
// change (say, renaming Record.Provider to Record.ProviderKind) can
// simply register a migration and the loader Just Works for old files.
type Migration func(doc map[string]any) error

// migrations is a map from source-version → migration that moves it to
// version (source+1). Applying migrations sequentially walks a file up
// to SchemaVersion.
var migrations = map[int]Migration{
	// Example shape for when we need one:
	//
	// 1: func(doc map[string]any) error {
	//     // Rename Record.Provider → Record.ProviderKind, etc.
	//     return nil
	// },
}

// migrate applies migrations in sequence until the doc reaches SchemaVersion.
// Returns a descriptive error if an intermediate version has no migration.
func migrate(data []byte) ([]byte, int, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, 0, fmt.Errorf("parse state for migrate: %w", err)
	}

	v := versionOf(raw)
	if v == 0 {
		// No version field means "pre-v1 or empty"; assume current.
		v = SchemaVersion
		raw["version"] = v
	}

	for v < SchemaVersion {
		mig, ok := migrations[v]
		if !ok {
			return nil, v, fmt.Errorf("no migration registered for state schema v%d → v%d", v, v+1)
		}
		if err := mig(raw); err != nil {
			return nil, v, fmt.Errorf("state migration v%d → v%d: %w", v, v+1, err)
		}
		v++
		raw["version"] = v
	}

	out, err := yaml.Marshal(raw)
	if err != nil {
		return nil, v, fmt.Errorf("re-marshal migrated state: %w", err)
	}
	return out, v, nil
}

// versionOf reads the "version" field from a raw YAML map. Returns 0 if
// missing or not an integer.
func versionOf(doc map[string]any) int {
	v, ok := doc["version"]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case uint64:
		return int(n)
	}
	return 0
}
