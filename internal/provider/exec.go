package provider

import (
	"bytes"
	"context"
	"os/exec"
)

// Runner abstracts command execution so providers can be unit-tested
// without spawning real processes. The real runner is execRunner, which
// simply wraps exec.CommandContext.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecRunner returns the default command runner for production use.
func ExecRunner() Runner { return execRunner{} }

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.Bytes(), err
}
