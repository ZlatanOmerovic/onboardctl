// Package update provides a cached lookup for the newest onboardctl
// release on GitHub. It is fire-and-forget: any error (no network,
// rate-limited, malformed JSON) is swallowed and reported as "no
// update available" rather than alarming the user.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CacheTTL is how long a positive result is trusted before the lookup
// hits the network again.
const CacheTTL = 24 * time.Hour

// DefaultRepo is the canonical upstream onboardctl repo. Overridable
// for tests; production callers don't need to touch it.
var DefaultRepo = "ZlatanOmerovic/onboardctl"

// Result is what Check returns. All fields are zero-valued on silent
// failure; callers check HasUpdate.
type Result struct {
	LatestTag    string    // e.g. "v0.2.0"; empty if the check failed
	Current      string    // current build version, unchanged from input
	HasUpdate    bool      // true iff LatestTag is strictly newer than Current
	FetchedAt    time.Time // when the backing result was obtained (may be from cache)
	FromCache    bool      // true if served from the on-disk cache rather than a live call
	SilentReason string    // populated when the result is empty and we want to log why
}

// Options controls a Check invocation.
type Options struct {
	HTTPClient *http.Client // default: http.Client{Timeout: 3s}
	Repo       string       // default: DefaultRepo
	CachePath  string       // default: XDG_CACHE_HOME/onboardctl/latest.json
	Now        func() time.Time
}

// Check returns whether a newer release exists upstream. "Newer" is a
// string compare against the tag after stripping a leading "v"; this
// is coarse but enough for the versions onboardctl actually ships.
//
// The check is disabled (empty Result, SilentReason set) when:
//   - ONBOARDCTL_NO_UPDATE_CHECK is set
//   - current == "dev" or "unknown" (pre-release builds never "need" an update)
//   - ctx is already cancelled
func Check(ctx context.Context, current string, opts Options) Result {
	res := Result{Current: current}

	if os.Getenv("ONBOARDCTL_NO_UPDATE_CHECK") != "" {
		res.SilentReason = "ONBOARDCTL_NO_UPDATE_CHECK set"
		return res
	}
	if current == "" || current == "dev" || current == "unknown" {
		res.SilentReason = "dev build"
		return res
	}
	if err := ctx.Err(); err != nil {
		res.SilentReason = err.Error()
		return res
	}

	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}

	cachePath := opts.CachePath
	if cachePath == "" {
		cachePath = DefaultCachePath()
	}

	// Cache hit path. A malformed or expired cache falls through to
	// a live call rather than surfacing an error.
	if cachePath != "" {
		if tag, when, ok := readCache(cachePath); ok && now().Sub(when) < CacheTTL {
			res.LatestTag = tag
			res.FetchedAt = when
			res.FromCache = true
			res.HasUpdate = isNewer(current, tag)
			return res
		}
	}

	repo := opts.Repo
	if repo == "" {
		repo = DefaultRepo
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}

	tag, err := fetchLatestTag(ctx, client, repo)
	if err != nil {
		res.SilentReason = err.Error()
		return res
	}
	if cachePath != "" {
		_ = writeCache(cachePath, tag, now())
	}
	res.LatestTag = tag
	res.FetchedAt = now()
	res.HasUpdate = isNewer(current, tag)
	return res
}

// DefaultCachePath returns $XDG_CACHE_HOME/onboardctl/latest.json or the
// $HOME/.cache fallback. Empty when neither is resolvable.
func DefaultCachePath() string {
	if x := os.Getenv("XDG_CACHE_HOME"); x != "" {
		return filepath.Join(x, "onboardctl", "latest.json")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".cache", "onboardctl", "latest.json")
}

type cachedResult struct {
	Tag       string    `json:"tag"`
	FetchedAt time.Time `json:"fetched_at"`
}

func readCache(path string) (tag string, when time.Time, ok bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", time.Time{}, false
	}
	var c cachedResult
	if err := json.Unmarshal(data, &c); err != nil {
		return "", time.Time{}, false
	}
	if c.Tag == "" {
		return "", time.Time{}, false
	}
	return c.Tag, c.FetchedAt, true
}

func writeCache(path, tag string, when time.Time) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(cachedResult{Tag: tag, FetchedAt: when})
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

type githubRelease struct {
	TagName string `json:"tag_name"`
}

func fetchLatestTag(ctx context.Context, client *http.Client, repo string) (string, error) {
	url := "https://api.github.com/repos/" + repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("github api: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	if err != nil {
		return "", err
	}
	var rel githubRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return "", fmt.Errorf("decode release: %w", err)
	}
	if rel.TagName == "" {
		return "", errors.New("release json had empty tag_name")
	}
	return rel.TagName, nil
}

// isNewer reports whether latest > current using semver-ish dotted
// integer comparison. Non-numeric suffixes (e.g. "-rc1") tie-break
// lexicographically, good enough for this tool's release cadence.
func isNewer(current, latest string) bool {
	a := strings.TrimPrefix(strings.TrimSpace(current), "v")
	b := strings.TrimPrefix(strings.TrimSpace(latest), "v")
	if a == b {
		return false
	}
	// Split "1.2.3-rc1" into the numeric prefix and the suffix.
	numA, sufA := splitSemver(a)
	numB, sufB := splitSemver(b)
	for i := 0; i < 3; i++ {
		if numB[i] > numA[i] {
			return true
		}
		if numB[i] < numA[i] {
			return false
		}
	}
	// Numeric prefix equal — a prerelease suffix ranks BELOW an empty one
	// (i.e. v1.0.0 > v1.0.0-rc1). Otherwise lexicographic.
	switch {
	case sufA == "" && sufB != "":
		return false
	case sufA != "" && sufB == "":
		return true
	}
	return sufB > sufA
}

func splitSemver(s string) (nums [3]int, suffix string) {
	dash := strings.Index(s, "-")
	core := s
	if dash >= 0 {
		core = s[:dash]
		suffix = s[dash+1:]
	}
	parts := strings.SplitN(core, ".", 3)
	for i, p := range parts {
		if i >= 3 {
			break
		}
		n := 0
		for _, c := range p {
			if c < '0' || c > '9' {
				break
			}
			n = n*10 + int(c-'0')
		}
		nums[i] = n
	}
	return nums, suffix
}
