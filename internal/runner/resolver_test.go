package runner

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
)

// mkTestManifest builds a small manifest used by multiple tests.
//
//	profiles:
//	  essentials → [base]
//	  dev        → extends essentials, bundles [tools]
//	  everything → extends dev, bundles [messengers]
//	bundles:
//	  base      → [jq, vlc]
//	  tools     → [gh]
//	  messengers→ [viber]
func mkTestManifest() *manifest.Manifest {
	item := func(id string) manifest.Item {
		return manifest.Item{Name: id, Providers: []manifest.Provider{{Type: "apt", Package: id}}}
	}
	return &manifest.Manifest{
		Version: 1,
		Items: map[string]manifest.Item{
			"jq": item("jq"), "vlc": item("vlc"), "gh": item("gh"), "viber": item("viber"),
		},
		Bundles: map[string]manifest.Bundle{
			"base":       {Name: "base", Items: []string{"jq", "vlc"}},
			"tools":      {Name: "tools", Items: []string{"gh"}},
			"messengers": {Name: "messengers", Items: []string{"viber"}},
		},
		Profiles: map[string]manifest.Profile{
			"essentials": {Name: "essentials", Bundles: []string{"base"}},
			"dev":        {Name: "dev", Extends: "essentials", Bundles: []string{"tools"}},
			"everything": {Name: "everything", Extends: "dev", Bundles: []string{"messengers"}},
		},
	}
}

func TestResolveProfile(t *testing.T) {
	m := mkTestManifest()
	got, err := Resolve(m, Selection{Profile: "everything"})
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	want := []string{"jq", "vlc", "gh", "viber"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Resolve(everything) = %v, want %v", got, want)
	}
}

func TestResolveBundle(t *testing.T) {
	m := mkTestManifest()
	got, err := Resolve(m, Selection{Bundle: "base"})
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	want := []string{"jq", "vlc"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Resolve(base) = %v, want %v", got, want)
	}
}

func TestResolveItems(t *testing.T) {
	m := mkTestManifest()
	got, err := Resolve(m, Selection{Items: []string{"gh", "jq", "jq"}})
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	want := []string{"gh", "jq"} // dedupes in order
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Resolve items dedupe = %v, want %v", got, want)
	}
}

func TestResolveSkip(t *testing.T) {
	m := mkTestManifest()
	got, err := Resolve(m, Selection{Profile: "everything", Skip: []string{"viber", "gh"}})
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	want := []string{"jq", "vlc"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Resolve skip = %v, want %v", got, want)
	}
}

func TestResolveUnknownBundle(t *testing.T) {
	m := mkTestManifest()
	_, err := Resolve(m, Selection{Bundle: "missing"})
	if err == nil {
		t.Fatal("expected error on missing bundle, got nil")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("error = %v, want bundle-name in message", err)
	}
}

func TestResolveUnknownItemFromExtras(t *testing.T) {
	// Simulate a user extras.yaml with a bundle referencing a non-existent item.
	m := mkTestManifest()
	m.Bundles["broken"] = manifest.Bundle{Name: "broken", Items: []string{"does-not-exist"}}
	_, err := Resolve(m, Selection{Bundle: "broken"})
	if err == nil {
		t.Fatal("expected error on dangling item, got nil")
	}
}

func TestResolveCycleDetected(t *testing.T) {
	m := mkTestManifest()
	// Make dev and essentials extend each other.
	m.Profiles["essentials"] = manifest.Profile{Name: "essentials", Extends: "dev", Bundles: nil}
	_, err := Resolve(m, Selection{Profile: "dev"})
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
}

func TestResolveEmptySelection(t *testing.T) {
	m := mkTestManifest()
	_, err := Resolve(m, Selection{})
	if err == nil {
		t.Fatal("expected error on empty selection")
	}
}
