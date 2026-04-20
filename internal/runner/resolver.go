// Package runner turns a user selection (profile / bundle / item) into a
// concrete, ordered list of items to install, then dispatches them to
// the appropriate provider.
//
// Resolver: pure, no side effects. Turns a selection into an item-ID list.
// Runner:   stateful. Orchestrates repo bootstrap, provider.Check / Install,
//           and state-file updates.
package runner

import (
	"errors"
	"fmt"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
)

// Selection is what the caller asks the resolver to expand.
//
// Exactly one of Profile, Bundle, or Items must be non-empty. Skip is
// applied after expansion.
type Selection struct {
	Profile string   // "fullstack-web"
	Bundle  string   // "runtime-php"
	Items   []string // ["jq", "vlc"]
	Skip    []string // remove these item IDs from the expanded list
}

// Resolve expands a Selection against a Manifest into an ordered, deduped
// list of item IDs to install.
//
// Profile expansion follows `extends` chains transitively. Bundle references
// that don't exist in the manifest are a hard error — they almost always
// mean a typo in a user extras.yaml.
func Resolve(m *manifest.Manifest, sel Selection) ([]string, error) {
	if m == nil {
		return nil, errors.New("resolver: nil manifest")
	}

	var raw []string
	switch {
	case sel.Profile != "":
		ids, err := expandProfile(m, sel.Profile, map[string]bool{})
		if err != nil {
			return nil, err
		}
		raw = ids
	case sel.Bundle != "":
		b, ok := m.Bundles[sel.Bundle]
		if !ok {
			return nil, fmt.Errorf("bundle %q not found", sel.Bundle)
		}
		raw = append(raw, b.Items...)
	case len(sel.Items) > 0:
		raw = append(raw, sel.Items...)
	default:
		return nil, errors.New("resolver: need one of Profile/Bundle/Items")
	}

	// Validate every id exists, and drop skips.
	skip := make(map[string]bool, len(sel.Skip))
	for _, s := range sel.Skip {
		skip[s] = true
	}

	seen := make(map[string]bool, len(raw))
	out := make([]string, 0, len(raw))
	for _, id := range raw {
		if skip[id] {
			continue
		}
		if _, ok := m.Items[id]; !ok {
			return nil, fmt.Errorf("item %q referenced by selection but not defined in manifest", id)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, nil
}

// expandProfile walks a profile's bundles, and any `extends` chain, gathering
// items. `visited` guards against accidental extends-cycles in user extras.
func expandProfile(m *manifest.Manifest, name string, visited map[string]bool) ([]string, error) {
	if visited[name] {
		return nil, fmt.Errorf("profile %q extends-cycle", name)
	}
	visited[name] = true

	p, ok := m.Profiles[name]
	if !ok {
		return nil, fmt.Errorf("profile %q not found", name)
	}

	var out []string
	if p.Extends != "" {
		parent, err := expandProfile(m, p.Extends, visited)
		if err != nil {
			return nil, err
		}
		out = append(out, parent...)
	}

	for _, bn := range p.Bundles {
		b, ok := m.Bundles[bn]
		if !ok {
			return nil, fmt.Errorf("profile %q references missing bundle %q", name, bn)
		}
		out = append(out, b.Items...)
	}
	return out, nil
}
