package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	"github.com/ZlatanOmerovic/onboardctl/internal/provider"
	"github.com/ZlatanOmerovic/onboardctl/internal/system"
)

// RepoBootstrapper materialises manifest.Repo entries into /etc/apt/
// files. It's stateful within a single Run: once a repo is "done",
// subsequent references no-op.
//
// Requires root. The caller (CLI) enforces this.
type RepoBootstrapper struct {
	Repos   map[string]manifest.Repo
	Runner  provider.Runner // shelled commands (curl, gpg)
	Distro  system.Distro   // for codename / arch substitution
	KeyDir  string          // defaults to /etc/apt/keyrings
	SrcDir  string          // defaults to /etc/apt/sources.list.d
	Out     io.Writer       // progress log; nil → discard

	done          map[string]bool
	anyNew        bool // set when a new repo was added this run → triggers apt-get update
}

// NewRepoBootstrapper returns a bootstrapper with sensible defaults.
func NewRepoBootstrapper(repos map[string]manifest.Repo, r provider.Runner, d system.Distro) *RepoBootstrapper {
	return &RepoBootstrapper{
		Repos:  repos,
		Runner: r,
		Distro: d,
		KeyDir: "/etc/apt/keyrings",
		SrcDir: "/etc/apt/sources.list.d",
		done:   make(map[string]bool),
	}
}

// Ensure guarantees the named repo's keyring and sources file exist on disk.
// Returns (addedNew, error).
//
// If the repo's When doesn't match the current environment, Ensure is a no-op
// and returns (false, nil).
func (rb *RepoBootstrapper) Ensure(ctx context.Context, name string, env Env) (bool, error) {
	if name == "" {
		return false, nil
	}
	if rb.done[name] {
		return false, nil
	}

	repo, ok := rb.Repos[name]
	if !ok {
		return false, fmt.Errorf("repo %q referenced but not defined in manifest", name)
	}
	if !Match(repo.When, env) {
		// Repo doesn't apply to this machine; pretend done.
		rb.done[name] = true
		return false, nil
	}

	keyPath := filepath.Join(rb.KeyDir, "onboardctl-"+name+".gpg")
	srcPath := filepath.Join(rb.SrcDir, "onboardctl-"+name+".list")

	// Already set up?
	if fileExists(keyPath) && fileExists(srcPath) {
		rb.done[name] = true
		return false, nil
	}

	rb.logf("repo %s: bootstrapping (%s)\n", name, repo.Source)

	if err := os.MkdirAll(rb.KeyDir, 0o755); err != nil {
		return false, fmt.Errorf("mkdir %s: %w", rb.KeyDir, err)
	}
	if err := rb.writeKeyring(ctx, repo, keyPath); err != nil {
		return false, fmt.Errorf("keyring for %s: %w", name, err)
	}
	if err := rb.writeSource(repo, keyPath, srcPath); err != nil {
		return false, fmt.Errorf("sources for %s: %w", name, err)
	}

	rb.done[name] = true
	rb.anyNew = true
	return true, nil
}

// AptUpdateIfNeeded runs apt-get update if any new repo was materialised.
// Callers invoke this once, after all Ensure() calls for a given run.
func (rb *RepoBootstrapper) AptUpdateIfNeeded(ctx context.Context) error {
	if !rb.anyNew {
		return nil
	}
	rb.logf("apt-get update (new repos added)...\n")
	out, err := rb.Runner.Run(ctx, "apt-get", "update")
	if err != nil {
		return fmt.Errorf("apt-get update failed: %w\n%s", err, string(out))
	}
	return nil
}

// writeKeyring downloads the keyring URL and writes it to keyPath.
// If KeyringDearmor is true, the download is piped through `gpg --dearmor`.
func (rb *RepoBootstrapper) writeKeyring(ctx context.Context, repo manifest.Repo, keyPath string) error {
	if repo.Keyring == "" {
		return fmt.Errorf("repo has no keyring URL")
	}
	// Shell pipeline keeps feature parity with the setup-dev-stack.sh script.
	var shellCmd string
	if repo.KeyringDearmor {
		shellCmd = fmt.Sprintf(
			"curl -fsSL %q | gpg --dearmor -o %q && chmod 0644 %q",
			repo.Keyring, keyPath, keyPath)
	} else {
		shellCmd = fmt.Sprintf(
			"curl -fsSL %q -o %q && chmod 0644 %q",
			repo.Keyring, keyPath, keyPath)
	}
	out, err := rb.Runner.Run(ctx, "bash", "-c", shellCmd)
	if err != nil {
		return fmt.Errorf("%w\n%s", err, string(out))
	}
	return nil
}

// writeSource renders the repo.Source template and writes it to srcPath.
// Substitutions: {keyring}, {codename}, {arch}.
func (rb *RepoBootstrapper) writeSource(repo manifest.Repo, keyPath, srcPath string) error {
	src := repo.Source
	src = strings.ReplaceAll(src, "{keyring}", keyPath)
	src = strings.ReplaceAll(src, "{codename}", rb.Distro.Codename)
	src = strings.ReplaceAll(src, "{arch}", rb.Distro.Arch)
	if !strings.HasSuffix(src, "\n") {
		src += "\n"
	}
	if err := os.WriteFile(srcPath, []byte(src), 0o644); err != nil {
		return err
	}
	return nil
}

func (rb *RepoBootstrapper) logf(format string, a ...any) {
	if rb.Out != nil {
		fmt.Fprintf(rb.Out, format, a...)
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
