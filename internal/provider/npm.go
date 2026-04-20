package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
)

// NPMGlobal installs Node.js packages globally via npm — typically CLIs
// like @vue/cli, vercel, netlify-cli, tsx, etc.
//
// Relevant manifest.Provider field:
//
//	package: the npm package name (bare or scoped, e.g. "yarn" or "@vue/cli"). Required.
//
// The provider does not install npm itself; the bundled manifest pulls
// it in through the `nodejs` apt item (NodeSource ships npm alongside node).
// When `npm` is absent, Check returns Installed=false without erroring so
// the TUI can still render the item.
type NPMGlobal struct {
	runner Runner
}

// NewNPMGlobal returns an NPMGlobal provider backed by real exec.Command.
func NewNPMGlobal() *NPMGlobal { return &NPMGlobal{runner: ExecRunner()} }

// NewNPMGlobalWith injects a Runner — primarily for tests.
func NewNPMGlobalWith(r Runner) *NPMGlobal { return &NPMGlobal{runner: r} }

// Kind implements Provider.
func (n *NPMGlobal) Kind() string { return manifest.KindNPMGlobal }

// Check implements Provider.
//
// `npm ls -g --depth=0 <pkg>` prints a tree whose package line looks like
// "├── yarn@1.22.22" or "└── @vue/cli@5.0.8". We detect presence by
// searching for "<pkg>@" in the output; the exit code is intentionally
// ignored because newer npm versions return 0 even when the package is
// missing (they just print an empty tree).
func (n *NPMGlobal) Check(ctx context.Context, _ manifest.Item, p manifest.Provider) (State, error) {
	if p.Package == "" {
		return State{}, errors.New("npm_global provider: provider.package is required")
	}
	out, _ := n.runner.Run(ctx, "npm", "ls", "-g", "--depth=0", p.Package)
	if version := parseNPMVersion(string(out), p.Package); version != "" {
		return State{
			Installed:    true,
			Version:      version,
			ProviderUsed: manifest.KindNPMGlobal,
		}, nil
	}
	return State{Installed: false}, nil
}

// Install implements Provider. Requires the npm binary — if missing,
// returns an error pointing at the apt-side `nodejs` item.
func (n *NPMGlobal) Install(ctx context.Context, item manifest.Item, p manifest.Provider) error {
	if p.Package == "" {
		return errors.New("npm_global provider: provider.package is required")
	}
	if _, err := n.runner.Run(ctx, "npm", "--version"); err != nil {
		return errors.New("npm binary not found — ensure the manifest's 'nodejs' item is selected first")
	}
	out, err := n.runner.Run(ctx, "npm", "install", "-g", p.Package)
	if err != nil {
		return fmt.Errorf("npm install -g %s for %q failed: %w\n%s",
			p.Package, item.Name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// parseNPMVersion scans `npm ls` output for the line matching pkgName
// and returns the version. Handles both bare names ("yarn") and scoped
// names ("@vue/cli"). Returns "" when nothing matches.
//
// Example inputs:
//
//	/usr/lib/node_modules
//	├── yarn@1.22.22
//	└── @vue/cli@5.0.8
func parseNPMVersion(out, pkgName string) string {
	marker := pkgName + "@"
	for _, line := range strings.Split(out, "\n") {
		idx := strings.Index(line, marker)
		if idx < 0 {
			continue
		}
		// Everything after "pkgName@" up to the first whitespace is the version.
		rest := line[idx+len(marker):]
		rest = strings.TrimSpace(rest)
		// Trim trailing characters like "(deprecated)" or " extraneous".
		if sp := strings.IndexAny(rest, " \t"); sp > 0 {
			rest = rest[:sp]
		}
		return rest
	}
	return ""
}
