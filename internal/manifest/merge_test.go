package manifest

import (
	"reflect"
	"testing"
)

func TestMergeNilInputs(t *testing.T) {
	got := Merge(nil, nil)
	if got == nil || got.Version != SchemaVersion {
		t.Errorf("Merge(nil, nil) = %+v, want Version=%d", got, SchemaVersion)
	}
}

func TestMergeExtrasWins(t *testing.T) {
	base := &Manifest{
		Version: 1,
		Items: map[string]Item{
			"vlc": {Name: "VLC (base)"},
			"jq":  {Name: "jq (base)"},
		},
	}
	extras := &Manifest{
		Version: 1,
		Items: map[string]Item{
			"vlc":  {Name: "VLC (extras override)"},
			"htop": {Name: "htop (new)"},
		},
	}
	got := Merge(base, extras)
	if got.Items["vlc"].Name != "VLC (extras override)" {
		t.Errorf("extras did not override base for 'vlc': got %q", got.Items["vlc"].Name)
	}
	if got.Items["jq"].Name != "jq (base)" {
		t.Errorf("base item lost: got %q", got.Items["jq"].Name)
	}
	if _, ok := got.Items["htop"]; !ok {
		t.Errorf("new item from extras not added")
	}
}

func TestMergeDoesNotMutateInputs(t *testing.T) {
	base := &Manifest{
		Version: 1,
		Items:   map[string]Item{"a": {Name: "a"}},
	}
	extras := &Manifest{
		Version: 1,
		Items:   map[string]Item{"b": {Name: "b"}},
	}
	snapshot := map[string]Item{"a": {Name: "a"}}
	_ = Merge(base, extras)
	if !reflect.DeepEqual(base.Items, snapshot) {
		t.Errorf("Merge mutated base: got %+v, want %+v", base.Items, snapshot)
	}
}

func TestLoadBundledParses(t *testing.T) {
	m, err := LoadBundled()
	if err != nil {
		t.Fatalf("LoadBundled error: %v", err)
	}
	if m.Version != SchemaVersion {
		t.Errorf("bundled version = %d, want %d", m.Version, SchemaVersion)
	}
	if len(m.Items) == 0 {
		t.Error("bundled manifest has no items")
	}
	if len(m.Profiles) == 0 {
		t.Error("bundled manifest has no profiles")
	}
	// Every bundle referenced by any profile must exist.
	for pn, p := range m.Profiles {
		for _, b := range p.Bundles {
			if _, ok := m.Bundles[b]; !ok {
				t.Errorf("profile %q references missing bundle %q", pn, b)
			}
		}
		if p.Extends != "" {
			if _, ok := m.Profiles[p.Extends]; !ok {
				t.Errorf("profile %q extends missing profile %q", pn, p.Extends)
			}
		}
	}
	// Every item referenced by any bundle must exist.
	for bn, b := range m.Bundles {
		for _, it := range b.Items {
			if _, ok := m.Items[it]; !ok {
				t.Errorf("bundle %q references missing item %q", bn, it)
			}
		}
	}
}
