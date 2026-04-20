package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
)

// APT is the provider for manifest items whose provider.type is "apt".
//
// Check uses dpkg-query, which is cheap and read-only.
// Install shells out to apt-get install -y — the caller is expected
// to have already bootstrapped any named Repo (handled by the runner).
//
// The provider never runs apt-get update itself; that's part of repo
// bootstrap in the runner. This keeps Install idempotent and fast on
// re-run for already-installed packages.
type APT struct {
	runner Runner
}

// NewAPT returns an APT provider backed by real exec.Command.
func NewAPT() *APT { return &APT{runner: ExecRunner()} }

// NewAPTWith injects a Runner — primarily for tests.
func NewAPTWith(r Runner) *APT { return &APT{runner: r} }

// Kind implements Provider.
func (a *APT) Kind() string { return manifest.KindAPT }

// Check implements Provider.
func (a *APT) Check(ctx context.Context, _ manifest.Item, p manifest.Provider) (State, error) {
	if p.Package == "" {
		return State{}, errors.New("apt provider: provider.package is required")
	}
	// dpkg-query -W -f='${db:Status-Abbrev} ${Version}\n' <pkg>
	// Exits non-zero when the package is unknown to dpkg.
	out, err := a.runner.Run(ctx, "dpkg-query", "-W",
		"-f", "${db:Status-Abbrev} ${Version}", p.Package)
	if err != nil {
		// Not installed: dpkg-query prints "no packages found matching ..."
		return State{Installed: false}, nil
	}
	status, version := parseDpkgStatus(string(out))
	if !strings.HasPrefix(status, "ii") {
		return State{Installed: false}, nil
	}
	return State{
		Installed:    true,
		Version:      version,
		InstalledBy:  "", // fill in at the runner level from state.yaml
		ProviderUsed: manifest.KindAPT,
	}, nil
}

// Install implements Provider.
func (a *APT) Install(ctx context.Context, item manifest.Item, p manifest.Provider) error {
	if p.Package == "" {
		return errors.New("apt provider: provider.package is required")
	}
	out, err := a.runner.Run(ctx, "apt-get", "install", "-y", p.Package)
	if err != nil {
		return fmt.Errorf("apt-get install %s for %q failed: %w\n%s",
			p.Package, item.Name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// parseDpkgStatus splits "ii 1.7.1-6" into status "ii" and version "1.7.1-6".
// Handles lines like "un " (unknown) and variants with trailing whitespace.
func parseDpkgStatus(s string) (status, version string) {
	s = strings.TrimSpace(s)
	// The format template was "${db:Status-Abbrev} ${Version}" — Status-Abbrev
	// is a two- or three-char code, then a space, then version.
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return "", ""
	}
	status = fields[0]
	if len(fields) >= 2 {
		version = fields[1]
	}
	return status, version
}
