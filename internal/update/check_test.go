package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		cur, latest string
		want        bool
	}{
		{"0.1.0", "0.2.0", true},
		{"v0.1.0", "v0.1.1", true},
		{"0.2.0", "0.1.9", false},
		{"0.2.0", "0.2.0", false},
		{"1.0.0", "1.0.0-rc1", false}, // rc1 < 1.0.0 final
		{"1.0.0-rc1", "1.0.0", true},
		{"1.2.3", "1.2.3-beta", false},
	}
	for _, c := range cases {
		if got := isNewer(c.cur, c.latest); got != c.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", c.cur, c.latest, got, c.want)
		}
	}
}

func TestCheckDevSilent(t *testing.T) {
	r := Check(context.Background(), "dev", Options{})
	if r.HasUpdate {
		t.Error("dev build must never claim an update exists")
	}
	if r.SilentReason == "" {
		t.Error("expected SilentReason for dev build")
	}
}

func TestCheckHonoursDisableEnv(t *testing.T) {
	t.Setenv("ONBOARDCTL_NO_UPDATE_CHECK", "1")
	r := Check(context.Background(), "0.1.0", Options{})
	if r.HasUpdate {
		t.Error("env-disabled check must not report update")
	}
	if r.SilentReason == "" {
		t.Error("expected SilentReason when env var is set")
	}
}

func TestCheckHitsNetworkAndCaches(t *testing.T) {
	called := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v0.2.0"})
	}))
	defer srv.Close()

	cachePath := filepath.Join(t.TempDir(), "latest.json")
	client := srv.Client()
	// Rewire the repo URL to the test server by temporarily pointing
	// the hardcoded URL scheme at it — Check builds the URL from Repo,
	// so we can shim it via Options.Repo being ignored and using Host
	// rewriting. Simpler: use a RoundTripper that routes api.github.com
	// to the test server.
	client.Transport = rewriteTransport{to: srv.URL}

	r := Check(context.Background(), "v0.1.0", Options{
		HTTPClient: client,
		Repo:       "owner/repo",
		CachePath:  cachePath,
		Now:        func() time.Time { return time.Unix(1_700_000_000, 0) },
	})
	if !r.HasUpdate {
		t.Errorf("expected HasUpdate=true, got %+v", r)
	}
	if r.LatestTag != "v0.2.0" {
		t.Errorf("LatestTag = %q, want v0.2.0", r.LatestTag)
	}
	if called != 1 {
		t.Fatalf("expected 1 network call, got %d", called)
	}

	// Second call within TTL should hit the cache.
	r2 := Check(context.Background(), "v0.1.0", Options{
		HTTPClient: client,
		Repo:       "owner/repo",
		CachePath:  cachePath,
		Now:        func() time.Time { return time.Unix(1_700_000_100, 0) },
	})
	if !r2.FromCache {
		t.Errorf("second call should be from cache, got %+v", r2)
	}
	if called != 1 {
		t.Errorf("cache should have prevented the second network call, got %d", called)
	}
}

func TestCheckSilentOnNetworkError(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "latest.json")
	_ = os.WriteFile(cachePath, []byte("invalid"), 0o644) // cache unreadable → falls through
	client := &http.Client{Timeout: 100 * time.Millisecond, Transport: failingTransport{}}
	r := Check(context.Background(), "v0.1.0", Options{
		HTTPClient: client,
		Repo:       "owner/repo",
		CachePath:  cachePath,
	})
	if r.HasUpdate {
		t.Error("failing network must not claim update")
	}
	if r.SilentReason == "" {
		t.Error("expected SilentReason on network failure")
	}
}

// --- test transports ---

type rewriteTransport struct{ to string }

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	client := &http.Client{}
	// Rebuild the URL onto the test server, keeping path/query.
	u := req.URL
	u2 := *u
	// Parse the test server URL into scheme/host.
	// httptest returns http://127.0.0.1:port.
	for i := 0; i < len(t.to); i++ {
		if t.to[i] == ':' && i+3 <= len(t.to) && t.to[i:i+3] == "://" {
			u2.Scheme = t.to[:i]
			rest := t.to[i+3:]
			u2.Host = rest
			break
		}
	}
	req2 := req.Clone(req.Context())
	req2.URL = &u2
	req2.Host = u2.Host
	return client.Do(req2)
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, http.ErrHandlerTimeout
}
