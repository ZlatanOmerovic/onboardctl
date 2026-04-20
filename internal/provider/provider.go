// Package provider declares the contract every item-installer implements.
//
// Phase 1 ships the interface and a Registry only; concrete providers
// (apt, flatpak, binary_release, config, shell, composer_global, npm_global)
// arrive in Phase 2. Locking the interface now means the TUI in Phase 3 can
// be written against a stable surface.
package provider

import (
	"context"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
)

// Provider is the strategy a runner delegates to when installing or
// configuring one manifest item. Implementations must be safe to call
// concurrently for different items but may serialise internally when
// talking to system tools (apt, for example, holds a global lock).
type Provider interface {
	// Kind returns the manifest provider.type string this implementation
	// handles (e.g. "apt", "flatpak", "binary_release").
	Kind() string

	// Check reports whether the item is already installed or applied on
	// this machine. It must be read-only — callers depend on being able
	// to run Check freely in the TUI at startup without side effects.
	Check(ctx context.Context, item manifest.Item, p manifest.Provider) (State, error)

	// Install applies the provider for the given item. If the state is
	// already Installed, implementations should no-op and return nil.
	Install(ctx context.Context, item manifest.Item, p manifest.Provider) error
}

// State is what Check reports back.
type State struct {
	// Installed is true if the item is considered present on the system.
	Installed bool

	// Version is the best-effort installed version string. May be empty
	// when the provider can't determine it cheaply.
	Version string

	// InstalledBy is one of:
	//   "onboardctl" — we installed it (read from state file)
	//   "external"   — present but we didn't install it
	//   ""           — not installed
	InstalledBy string

	// ProviderUsed names the actual provider kind that fulfils this item
	// today — useful when detecting divergence (e.g., Firefox present as
	// a snap when the manifest prefers an apt .deb).
	ProviderUsed string
}

// Installed is a convenience matcher used in tests and the TUI.
func (s State) IsInstalled() bool { return s.Installed }

// IsExternal reports whether the item exists but wasn't installed by us.
func (s State) IsExternal() bool { return s.Installed && s.InstalledBy == "external" }

// IsProviderDrift reports whether the actual provider differs from what
// the manifest prefers (e.g., manifest wants apt, system has snap).
func (s State) IsProviderDrift(preferred string) bool {
	return s.Installed && s.ProviderUsed != "" && preferred != "" && s.ProviderUsed != preferred
}

// Registry maps a provider kind to its implementation. The runner builds
// one Registry at start and hands it to every phase of the TUI/install flow.
type Registry map[string]Provider

// NewRegistry returns an empty registry. Callers wire providers into it via
// Registry.Register.
func NewRegistry() Registry { return make(Registry) }

// Register adds a provider to the registry. It panics on duplicate kinds to
// catch wiring bugs at startup — we never want two implementations racing
// for the same manifest key.
func (r Registry) Register(p Provider) {
	k := p.Kind()
	if _, exists := r[k]; exists {
		panic("provider registered twice: " + k)
	}
	r[k] = p
}

// Lookup returns the provider for a kind, or nil if none is registered.
func (r Registry) Lookup(kind string) Provider { return r[kind] }

// Kinds returns the set of registered kinds. Handy for diagnostics.
func (r Registry) Kinds() []string {
	out := make([]string, 0, len(r))
	for k := range r {
		out = append(out, k)
	}
	return out
}
