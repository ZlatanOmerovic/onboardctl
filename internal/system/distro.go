// Package system inspects the local machine to answer the questions that
// gate manifest items: which distro family, which codename, which desktop,
// which CPU architecture. Nothing here mutates state.
package system

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
)

// OSReleasePath is the canonical location of /etc/os-release. It is a
// package variable so tests can swap it out for a fixture.
var OSReleasePath = "/etc/os-release"

// Distro is the minimal fingerprint we need from /etc/os-release.
type Distro struct {
	ID       string   // "debian", "ubuntu", "linuxmint", "pop", "elementary", "kali", ...
	Name     string   // Pretty name: "Debian GNU/Linux"
	Version  string   // "13"  (VERSION_ID; may be empty on rolling distros)
	Codename string   // "trixie", "noble", "jammy", ...
	IDLike   []string // Parents declared by ID_LIKE, e.g. ["ubuntu", "debian"]
	Family   string   // Resolved top-level family. "debian" for anything in the Debian tree.
	Arch     string   // runtime.GOARCH: "amd64", "arm64", ...
}

// DetectDistro reads /etc/os-release and returns a populated Distro.
// An error is returned only when the file is missing or unreadable — a
// malformed line is silently skipped rather than failing the whole read.
func DetectDistro() (Distro, error) {
	f, err := os.Open(OSReleasePath)
	if err != nil {
		return Distro{}, fmt.Errorf("open %s: %w", OSReleasePath, err)
	}
	defer f.Close()
	return parseOSRelease(f)
}

func parseOSRelease(r io.Reader) (Distro, error) {
	kv := make(map[string]string, 16)
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := line[:eq]
		val := strings.Trim(line[eq+1:], `"'`)
		kv[key] = val
	}
	if err := sc.Err(); err != nil {
		return Distro{}, fmt.Errorf("scan os-release: %w", err)
	}
	if kv["ID"] == "" {
		return Distro{}, errors.New("os-release has no ID field")
	}

	d := Distro{
		ID:       kv["ID"],
		Name:     firstNonEmpty(kv["NAME"], kv["PRETTY_NAME"], kv["ID"]),
		Version:  firstNonEmpty(kv["VERSION_ID"], kv["VERSION"]),
		Codename: kv["VERSION_CODENAME"],
		IDLike:   strings.Fields(kv["ID_LIKE"]),
		Arch:     runtime.GOARCH,
	}
	d.Family = resolveFamily(d)
	return d, nil
}

// resolveFamily collapses Debian derivatives (Ubuntu, Mint, Pop!_OS, etc.)
// onto the common "debian" family so manifest gates can target the whole tree.
func resolveFamily(d Distro) string {
	if d.ID == "debian" {
		return "debian"
	}
	for _, p := range d.IDLike {
		if p == "debian" {
			return "debian"
		}
	}
	// Ubuntu declares ID_LIKE=debian, but for completeness: Ubuntu itself is
	// the parent for Kubuntu/Xubuntu/Mint-on-Ubuntu/Pop!_OS/etc. Those distros
	// name Ubuntu in ID_LIKE, so the loop above already catches them.
	// Anything that reaches here is not in the Debian tree.
	return d.ID
}

// InDebianFamily reports whether the distro descends from Debian.
// It is the predicate onboardctl's "only Debian-family" contract enforces.
func (d Distro) InDebianFamily() bool {
	return d.Family == "debian"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
