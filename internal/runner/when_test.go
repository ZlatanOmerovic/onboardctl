package runner

import (
	"testing"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	"github.com/ZlatanOmerovic/onboardctl/internal/system"
)

func env(id, family, codename, arch string, de system.Desktop) Env {
	return Env{
		Distro:  system.Distro{ID: id, Family: family, Codename: codename, Arch: arch},
		Desktop: de,
	}
}

func TestMatchNilWhen(t *testing.T) {
	if !Match(nil, env("debian", "debian", "trixie", "amd64", system.DesktopGNOME)) {
		t.Error("nil When should always match")
	}
}

func TestMatchFamily(t *testing.T) {
	w := &manifest.When{DistroFamily: []string{"debian"}}
	if !Match(w, env("ubuntu", "debian", "noble", "amd64", system.DesktopGNOME)) {
		t.Error("ubuntu→debian family should match")
	}
	if Match(w, env("fedora", "fedora", "", "amd64", system.DesktopGNOME)) {
		t.Error("fedora should NOT match debian family")
	}
}

func TestMatchDistroID(t *testing.T) {
	w := &manifest.When{DistroID: []string{"debian", "ubuntu"}}
	if !Match(w, env("ubuntu", "debian", "noble", "amd64", system.DesktopGNOME)) {
		t.Error("ubuntu should match list containing ubuntu")
	}
	if Match(w, env("linuxmint", "debian", "wilma", "amd64", system.DesktopGNOME)) {
		t.Error("linuxmint should NOT match list {debian, ubuntu}")
	}
}

func TestMatchCodenameAndDesktopAndArch(t *testing.T) {
	w := &manifest.When{
		Codename: []string{"trixie"},
		Desktop:  []string{"gnome", "kde"},
		Arch:     []string{"amd64"},
	}
	if !Match(w, env("debian", "debian", "trixie", "amd64", system.DesktopGNOME)) {
		t.Error("matching env should match")
	}
	if Match(w, env("debian", "debian", "bookworm", "amd64", system.DesktopGNOME)) {
		t.Error("codename mismatch should fail")
	}
	if Match(w, env("debian", "debian", "trixie", "arm64", system.DesktopGNOME)) {
		t.Error("arch mismatch should fail")
	}
	if Match(w, env("debian", "debian", "trixie", "amd64", system.DesktopXfce)) {
		t.Error("desktop mismatch should fail")
	}
}

func TestMatchEmptyListIsWildcard(t *testing.T) {
	w := &manifest.When{} // no fields set
	if !Match(w, env("anything", "anything", "anything", "anything", system.DesktopUnknown)) {
		t.Error("empty When should match everything")
	}
}
