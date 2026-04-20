package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
)

// Shell is the escape-hatch provider for manifest items that don't fit the
// other kinds cleanly — vendor .deb downloads, one-off install scripts,
// anything else expressible as a short sequence of shell commands.
//
// Provider fields used:
//
//	apply: ordered list of shell commands to run via `bash -c`.
//	check: single shell command; exit-0 means "installed".
//
// Commands run with the current user's privileges; if apply needs root,
// the caller is responsible for being root (the install subcommand
// enforces this before dispatch).
type Shell struct {
	runner Runner
}

// NewShell returns a Shell provider backed by real exec.Command.
func NewShell() *Shell { return &Shell{runner: ExecRunner()} }

// NewShellWith injects a Runner — primarily for tests.
func NewShellWith(r Runner) *Shell { return &Shell{runner: r} }

// Kind implements Provider.
func (s *Shell) Kind() string { return manifest.KindShell }

// Check implements Provider. If p.Check is empty, Check returns
// Installed=false — we can't prove presence without a check predicate,
// so re-running is safe.
func (s *Shell) Check(ctx context.Context, _ manifest.Item, p manifest.Provider) (State, error) {
	if p.Check == "" {
		return State{Installed: false}, nil
	}
	_, err := s.runner.Run(ctx, "bash", "-c", p.Check)
	if err != nil {
		return State{Installed: false}, nil
	}
	return State{Installed: true, ProviderUsed: manifest.KindShell}, nil
}

// Install implements Provider. Runs each apply command sequentially;
// aborts on first failure with a composite error that preserves the
// failing command and its output for debugging.
func (s *Shell) Install(ctx context.Context, item manifest.Item, p manifest.Provider) error {
	if len(p.Apply) == 0 {
		return errors.New("shell provider: provider.apply is empty")
	}
	for i, cmd := range p.Apply {
		if strings.TrimSpace(cmd) == "" {
			continue
		}
		out, err := s.runner.Run(ctx, "bash", "-c", cmd)
		if err != nil {
			return fmt.Errorf("shell apply[%d] for %q failed: %s\n  cmd: %s\n  output: %s",
				i, item.Name, err, cmd, strings.TrimSpace(string(out)))
		}
	}
	return nil
}
