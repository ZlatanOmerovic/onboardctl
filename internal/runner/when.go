package runner

import (
	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	"github.com/ZlatanOmerovic/onboardctl/internal/system"
)

// Env is the facts a When is evaluated against.
// PackageExists is intentionally not a field here — it is a runtime query
// handled by the evaluator via the apt-provider path when needed.
type Env struct {
	Distro  system.Distro
	Desktop system.Desktop
}

// Match reports whether a When matches the environment.
// A nil When is an unconditional yes.
// An empty list on any field is an unconditional yes for that field.
//
// PackageExists is currently treated as "assume true" — a proper
// implementation needs an apt query and will land alongside the Phase 3 TUI.
func Match(w *manifest.When, env Env) bool {
	if w == nil {
		return true
	}
	if len(w.DistroID) > 0 && !contains(w.DistroID, env.Distro.ID) {
		return false
	}
	if len(w.DistroFamily) > 0 && !contains(w.DistroFamily, env.Distro.Family) {
		return false
	}
	if len(w.Codename) > 0 && !contains(w.Codename, env.Distro.Codename) {
		return false
	}
	if len(w.Desktop) > 0 && !contains(w.Desktop, string(env.Desktop)) {
		return false
	}
	if len(w.Arch) > 0 && !contains(w.Arch, env.Distro.Arch) {
		return false
	}
	return true
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
