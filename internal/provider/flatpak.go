package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
)

// Flatpak installs apps from Flatpak remotes (Flathub by default).
//
// Relevant manifest.Provider fields:
//
//	id:    the Flatpak app ID (e.g. "org.mozilla.firefox"). Required.
//	extra: optional map. Keys honoured today:
//	         remote  — override the remote to install from (default: "flathub")
//	         scope   — "system" (default) or "user"
//
// The provider auto-adds the Flathub remote on first use within a run.
// It does not install `flatpak` itself — if the binary is missing, Install
// returns an error pointing at the apt package. We deliberately don't
// attempt to install the binary cross-provider; that coupling lives in
// the manifest (make flatpak-hosted items depend on an apt item for the
// `flatpak` package, or use a `when: package_exists: [flatpak]` gate).
type Flatpak struct {
	runner Runner

	mu          sync.Mutex
	remoteAdded bool // session cache: have we run `remote-add flathub` already?
}

// NewFlatpak returns a Flatpak provider backed by real exec.Command.
func NewFlatpak() *Flatpak { return &Flatpak{runner: ExecRunner()} }

// NewFlatpakWith injects a Runner — primarily for tests.
func NewFlatpakWith(r Runner) *Flatpak { return &Flatpak{runner: r} }

// Kind implements Provider.
func (f *Flatpak) Kind() string { return manifest.KindFlatpak }

// Check implements Provider. Tries `flatpak info` in user scope first,
// then system scope. Missing `flatpak` binary is treated as "not installed"
// rather than an error so the TUI can still render the item.
func (f *Flatpak) Check(ctx context.Context, _ manifest.Item, p manifest.Provider) (State, error) {
	if p.ID == "" {
		return State{}, errors.New("flatpak provider: provider.id is required")
	}
	if !f.flatpakAvailable(ctx) {
		return State{Installed: false}, nil
	}
	// User scope first (no sudo needed) then system scope.
	if out, err := f.runner.Run(ctx, "flatpak", "info", "--user", p.ID); err == nil {
		return State{
			Installed:    true,
			Version:      parseFlatpakVersion(string(out)),
			ProviderUsed: manifest.KindFlatpak,
		}, nil
	}
	if out, err := f.runner.Run(ctx, "flatpak", "info", p.ID); err == nil {
		return State{
			Installed:    true,
			Version:      parseFlatpakVersion(string(out)),
			ProviderUsed: manifest.KindFlatpak,
		}, nil
	}
	return State{Installed: false}, nil
}

// Install implements Provider. Ensures the named remote is registered
// (Flathub by default) and then runs a non-interactive install.
func (f *Flatpak) Install(ctx context.Context, item manifest.Item, p manifest.Provider) error {
	if p.ID == "" {
		return errors.New("flatpak provider: provider.id is required")
	}
	if !f.flatpakAvailable(ctx) {
		return errors.New("flatpak binary not found — install it first (e.g. 'sudo apt install flatpak')")
	}
	remote := firstNonEmpty(p.Extra["remote"], "flathub")
	// Only seed Flathub when we're actually installing from it — don't
	// clutter a user's remote list if they've pointed this item at a
	// private or bespoke remote.
	if remote == "flathub" {
		if err := f.ensureFlathub(ctx); err != nil {
			return err
		}
	}
	args := []string{"install", "-y", "--noninteractive"}
	if scope := p.Extra["scope"]; scope == "user" {
		args = append(args, "--user")
	}
	args = append(args, remote, p.ID)
	out, err := f.runner.Run(ctx, "flatpak", args...)
	if err != nil {
		return fmt.Errorf("flatpak install %s for %q failed: %w\n%s",
			p.ID, item.Name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ensureFlathub runs `flatpak remote-add --if-not-exists flathub …` once
// per Flatpak instance. No-op after the first successful call.
func (f *Flatpak) ensureFlathub(ctx context.Context) error {
	f.mu.Lock()
	already := f.remoteAdded
	f.mu.Unlock()
	if already {
		return nil
	}
	out, err := f.runner.Run(ctx, "flatpak", "remote-add", "--if-not-exists",
		"flathub", "https://flathub.org/repo/flathub.flatpakrepo")
	if err != nil {
		return fmt.Errorf("flatpak remote-add flathub failed: %w\n%s",
			err, strings.TrimSpace(string(out)))
	}
	f.mu.Lock()
	f.remoteAdded = true
	f.mu.Unlock()
	return nil
}

// flatpakAvailable reports whether the flatpak binary is callable.
// Cheap call; results aren't cached because the binary could be installed
// mid-run by a separate manifest item.
func (f *Flatpak) flatpakAvailable(ctx context.Context) bool {
	_, err := f.runner.Run(ctx, "flatpak", "--version")
	return err == nil
}

// parseFlatpakVersion pulls the version line out of `flatpak info` output.
func parseFlatpakVersion(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Version:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Version:"))
		}
	}
	return ""
}

// firstNonEmpty is local so we don't collide with runner's copy.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
