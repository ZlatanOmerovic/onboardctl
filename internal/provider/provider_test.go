package provider

import (
	"context"
	"testing"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
)

// fakeProvider is a zero-side-effect stub used to exercise the Registry.
type fakeProvider struct{ kind string }

func (f *fakeProvider) Kind() string { return f.kind }
func (f *fakeProvider) Check(context.Context, manifest.Item, manifest.Provider) (State, error) {
	return State{}, nil
}
func (f *fakeProvider) Install(context.Context, manifest.Item, manifest.Provider) error {
	return nil
}

func TestRegistryRegisterAndLookup(t *testing.T) {
	r := NewRegistry()
	r.Register(&fakeProvider{kind: "apt"})
	r.Register(&fakeProvider{kind: "flatpak"})

	if got := r.Lookup("apt"); got == nil || got.Kind() != "apt" {
		t.Errorf("Lookup(apt) = %v, want apt provider", got)
	}
	if got := r.Lookup("nonexistent"); got != nil {
		t.Errorf("Lookup(nonexistent) = %v, want nil", got)
	}
	if n := len(r.Kinds()); n != 2 {
		t.Errorf("Kinds() len = %d, want 2", n)
	}
}

func TestRegistryRegisterDuplicatePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic on duplicate Register, got none")
		}
	}()
	r := NewRegistry()
	r.Register(&fakeProvider{kind: "apt"})
	r.Register(&fakeProvider{kind: "apt"})
}

func TestStateHelpers(t *testing.T) {
	cases := []struct {
		name      string
		s         State
		preferred string
		wantInst  bool
		wantExt   bool
		wantDrift bool
	}{
		{"not-installed", State{}, "apt", false, false, false},
		{"installed-by-us", State{Installed: true, InstalledBy: "onboardctl", ProviderUsed: "apt"}, "apt", true, false, false},
		{"external-match", State{Installed: true, InstalledBy: "external", ProviderUsed: "apt"}, "apt", true, true, false},
		{"drift-snap", State{Installed: true, InstalledBy: "external", ProviderUsed: "snap"}, "apt", true, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.s.IsInstalled() != c.wantInst {
				t.Errorf("IsInstalled = %v, want %v", c.s.IsInstalled(), c.wantInst)
			}
			if c.s.IsExternal() != c.wantExt {
				t.Errorf("IsExternal = %v, want %v", c.s.IsExternal(), c.wantExt)
			}
			if c.s.IsProviderDrift(c.preferred) != c.wantDrift {
				t.Errorf("IsProviderDrift(%q) = %v, want %v", c.preferred, c.s.IsProviderDrift(c.preferred), c.wantDrift)
			}
		})
	}
}
