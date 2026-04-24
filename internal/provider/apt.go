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
//
// Beyond the native dpkg lookup, Check also queries `snap list` when
// snap is installed. If snap reports the package but apt doesn't, we
// surface the drift by returning Installed=true + ProviderUsed="snap"
// so the UI can render a ⚠ marker — the user then knows the manifest
// wants an apt .deb but the system has the snap. Common on Ubuntu
// since 22.04 for Firefox / Chromium / Thunderbird.
func (a *APT) Check(ctx context.Context, _ manifest.Item, p manifest.Provider) (State, error) {
	if p.Package == "" {
		return State{}, errors.New("apt provider: provider.package is required")
	}
	// dpkg-query -W -f='${db:Status-Abbrev} ${Version}\n' <pkg>
	// Exits non-zero when the package is unknown to dpkg.
	out, err := a.runner.Run(ctx, "dpkg-query", "-W",
		"-f", "${db:Status-Abbrev} ${Version}", p.Package)
	if err == nil {
		status, version := parseDpkgStatus(string(out))
		if strings.HasPrefix(status, "ii") {
			return State{
				Installed:    true,
				Version:      version,
				ProviderUsed: manifest.KindAPT,
			}, nil
		}
	}

	// Not installed via apt — look for a snap-delivered alternative.
	if snapVer, found := a.findSnapVersion(ctx, p.Package); found {
		return State{
			Installed:    true,
			Version:      snapVer,
			ProviderUsed: "snap", // signals drift to the TUI
			InstalledBy:  "external",
		}, nil
	}
	return State{Installed: false}, nil
}

// findSnapVersion runs `snap list <pkg>` and returns the installed
// version if snap knows it. Any error or "not installed" response
// yields (_, false).
func (a *APT) findSnapVersion(ctx context.Context, pkg string) (string, bool) {
	out, err := a.runner.Run(ctx, "snap", "list", pkg)
	if err != nil {
		return "", false
	}
	// snap list output:
	//   Name     Version  Rev  Tracking  Publisher  Notes
	//   firefox  147.0    4820  latest/stable  mozilla✓  -
	// We want the second column of the first data row.
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Name") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == pkg {
			return fields[1], true
		}
	}
	return "", false
}

// Install implements Provider.
//
// When p.Version is set, the install target becomes "pkg=version", which
// apt-get interprets as an exact version pin. Note that if the version
// is not present in any configured apt source, the install will fail with
// apt's standard "Version not found" error — we pass that through.
func (a *APT) Install(ctx context.Context, item manifest.Item, p manifest.Provider) error {
	if p.Package == "" {
		return errors.New("apt provider: provider.package is required")
	}
	target := p.Package
	if p.Version != "" {
		target = p.Package + "=" + p.Version
	}
	out, err := a.runner.Run(ctx, "apt-get", "install", "-y", target)
	if err != nil {
		return fmt.Errorf("apt-get install %s for %q failed: %w\n%s",
			target, item.Name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Uninstall implements Uninstaller. Runs `apt-get purge -y <pkg>` which
// removes the package and its config files. Autoremove is not called —
// we don't want to collateral-damage other hand-installed packages.
func (a *APT) Uninstall(ctx context.Context, item manifest.Item, p manifest.Provider) error {
	if p.Package == "" {
		return errors.New("apt provider: provider.package is required")
	}
	out, err := a.runner.Run(ctx, "apt-get", "purge", "-y", p.Package)
	if err != nil {
		return fmt.Errorf("apt-get purge %s for %q failed: %w\n%s",
			p.Package, item.Name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// RemoveSnapCounterpart removes the snap whose name matches pkg.
// Intended for --swap-drift: the manifest wants the apt version of a
// package, but the machine currently has the snap equivalent. The
// runner calls this before the normal apt install. Errors are
// returned verbatim so callers can decide whether a missing snap
// (already not installed) counts as success or failure.
func (a *APT) RemoveSnapCounterpart(ctx context.Context, pkg string) error {
	if pkg == "" {
		return errors.New("apt provider: pkg is required")
	}
	out, err := a.runner.Run(ctx, "snap", "remove", pkg)
	if err != nil {
		return fmt.Errorf("snap remove %s: %w\n%s", pkg, err, strings.TrimSpace(string(out)))
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
