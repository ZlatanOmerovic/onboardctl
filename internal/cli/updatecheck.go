package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/ZlatanOmerovic/onboardctl/internal/update"
)

// maybePrintUpdateNotice calls update.Check (disabled by --no-update-check
// or the corresponding env var) and prints a single short line if a
// newer release exists. Network or parse errors are swallowed — the
// tool must never block its main output on a best-effort lookup.
func maybePrintUpdateNotice(out io.Writer) {
	if !UpdateCheckEnabled() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	r := update.Check(ctx, Version, update.Options{})
	if !r.HasUpdate {
		return
	}
	fmt.Fprintf(out, "\nA newer release is available: %s (current: %s) — run `onboardctl upgrade`.\n",
		r.LatestTag, r.Current)
}
