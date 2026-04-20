package provider

import (
	"context"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
)

// Config is the provider for manifest items that modify system settings
// via shell commands (timezone, git identity, unattended-upgrades, and so on).
//
// Functionally, Config is identical to Shell today: it runs provider.Apply
// sequentially and treats provider.Check as the installed-state predicate.
// The semantic distinction matters for the TUI in Phase 3 — config items
// with an Input block prompt the user for values before Apply runs — but
// in Phase 2 (headless only), there is no interactive prompt. Config items
// that require Input fields will fail at Install time because the {name},
// {email}, {value}, etc. placeholders won't resolve.
//
// Keeping Config as its own Kind now means users get a clear error message
// ("config items need interactive input; Phase 3 TUI adds it") rather than
// silent no-ops.
type Config struct {
	inner *Shell
}

// NewConfig returns a Config provider backed by real exec.Command.
func NewConfig() *Config { return &Config{inner: NewShell()} }

// NewConfigWith injects a Runner — primarily for tests.
func NewConfigWith(r Runner) *Config { return &Config{inner: NewShellWith(r)} }

// Kind implements Provider.
func (c *Config) Kind() string { return manifest.KindConfig }

// Check delegates to the Shell implementation (runs the check predicate).
func (c *Config) Check(ctx context.Context, item manifest.Item, p manifest.Provider) (State, error) {
	st, err := c.inner.Check(ctx, item, p)
	if st.Installed {
		st.ProviderUsed = manifest.KindConfig
	}
	return st, err
}

// Install runs the Apply commands. If the item has an Input block (required
// user prompts), it refuses until the TUI lands in Phase 3.
func (c *Config) Install(ctx context.Context, item manifest.Item, p manifest.Provider) error {
	if item.Input != nil {
		return errNeedsInteractiveInput{name: item.Name}
	}
	return c.inner.Install(ctx, item, p)
}

type errNeedsInteractiveInput struct{ name string }

func (e errNeedsInteractiveInput) Error() string {
	return "config item \"" + e.name + "\" needs interactive input (not available in headless Phase 2 — wait for the TUI in Phase 3)"
}
