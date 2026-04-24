package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
)

// ComposerGlobal installs PHP packages globally via Composer — typically
// CLIs like laravel/installer whose binaries land in
// ~/.config/composer/vendor/bin.
//
// The provider does not bootstrap Composer itself; that's a separate apt
// item users pull in alongside (already declared as `composer` in the
// bundled manifest).
type ComposerGlobal struct {
	runner Runner
}

// NewComposerGlobal returns a ComposerGlobal provider backed by real exec.Command.
func NewComposerGlobal() *ComposerGlobal { return &ComposerGlobal{runner: ExecRunner()} }

// NewComposerGlobalWith injects a Runner — primarily for tests.
func NewComposerGlobalWith(r Runner) *ComposerGlobal { return &ComposerGlobal{runner: r} }

// Kind implements Provider.
func (c *ComposerGlobal) Kind() string { return manifest.KindComposerGlobal }

// Check implements Provider. Runs `composer global show --name-only <pkg>`
// and treats exit-0 + non-empty output as installed.
func (c *ComposerGlobal) Check(ctx context.Context, _ manifest.Item, p manifest.Provider) (State, error) {
	if p.Package == "" {
		return State{}, errors.New("composer_global provider: provider.package is required")
	}
	out, err := c.runner.Run(ctx, "composer", "global", "show", "--name-only", "--no-interaction")
	if err != nil {
		return State{Installed: false}, nil
	}
	if lineContains(string(out), p.Package) {
		return State{Installed: true, ProviderUsed: manifest.KindComposerGlobal}, nil
	}
	return State{Installed: false}, nil
}

// Install implements Provider. Calls `composer global require <pkg>`.
//
// When p.Version is set, the install target becomes "pkg:version",
// Composer's version-constraint form (accepts exact versions, ranges,
// or dev-branch specifiers like "dev-main").
func (c *ComposerGlobal) Install(ctx context.Context, item manifest.Item, p manifest.Provider) error {
	if p.Package == "" {
		return errors.New("composer_global provider: provider.package is required")
	}
	target := p.Package
	if p.Version != "" {
		target = p.Package + ":" + p.Version
	}
	out, err := c.runner.Run(ctx, "composer", "global", "require", "--no-interaction", target)
	if err != nil {
		return fmt.Errorf("composer global require %s for %q failed: %w\n%s",
			target, item.Name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Uninstall implements Uninstaller. Runs `composer global remove <pkg>`.
func (c *ComposerGlobal) Uninstall(ctx context.Context, item manifest.Item, p manifest.Provider) error {
	if p.Package == "" {
		return errors.New("composer_global provider: provider.package is required")
	}
	out, err := c.runner.Run(ctx, "composer", "global", "remove", "--no-interaction", p.Package)
	if err != nil {
		return fmt.Errorf("composer global remove %s for %q failed: %w\n%s",
			p.Package, item.Name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// lineContains reports whether any line (trimmed) equals the needle.
func lineContains(stdout, needle string) bool {
	for _, line := range strings.Split(stdout, "\n") {
		if strings.TrimSpace(line) == needle {
			return true
		}
	}
	return false
}
