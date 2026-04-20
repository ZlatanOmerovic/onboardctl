package provider

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
)

// BinaryRelease installs CLIs published as GitHub releases — lazygit, k9s,
// hcloud, gitleaks, and the like.
//
// Lifecycle:
//  1. Call the GitHub API for the latest release of provider.source.
//  2. Pick the asset whose name matches provider.asset (regex).
//  3. Download, extract (tar.gz), find the binary named provider.binary
//     inside, install it to /usr/local/bin with mode 0755.
//
// Install requires root because /usr/local/bin typically is not writable by
// the user. The install subcommand enforces this before dispatch.
type BinaryRelease struct {
	HTTP       *http.Client
	InstallDir string // defaults to /usr/local/bin
}

// NewBinaryRelease returns a BinaryRelease provider with reasonable defaults.
func NewBinaryRelease() *BinaryRelease {
	return &BinaryRelease{
		HTTP:       &http.Client{Timeout: 60 * time.Second},
		InstallDir: "/usr/local/bin",
	}
}

// Kind implements Provider.
func (b *BinaryRelease) Kind() string { return manifest.KindBinaryRelease }

// Check reports installed if a file named provider.binary exists on PATH
// or directly at InstallDir. We don't try to parse --version output here —
// cheap liveness is all Check needs to convey.
func (b *BinaryRelease) Check(_ context.Context, _ manifest.Item, p manifest.Provider) (State, error) {
	if p.Binary == "" {
		return State{}, errors.New("binary_release: provider.binary is required")
	}
	candidate := filepath.Join(b.InstallDir, p.Binary)
	if _, err := os.Stat(candidate); err == nil {
		return State{Installed: true, ProviderUsed: manifest.KindBinaryRelease}, nil
	}
	// Fall back to PATH lookup for binaries installed elsewhere.
	if path, err := lookPath(p.Binary); err == nil && path != "" {
		return State{
			Installed:    true,
			ProviderUsed: manifest.KindBinaryRelease,
			InstalledBy:  "", // unknown — not ours
		}, nil
	}
	return State{Installed: false}, nil
}

// Install downloads the latest matching release asset, extracts the named
// binary, and places it in InstallDir.
func (b *BinaryRelease) Install(ctx context.Context, item manifest.Item, p manifest.Provider) error {
	if p.Source == "" {
		return errors.New("binary_release: provider.source (owner/repo) is required")
	}
	if p.Asset == "" {
		return errors.New("binary_release: provider.asset (regex) is required")
	}
	if p.Binary == "" {
		return errors.New("binary_release: provider.binary is required")
	}

	assetPattern, err := regexp.Compile(p.Asset)
	if err != nil {
		return fmt.Errorf("binary_release: bad asset regex %q: %w", p.Asset, err)
	}

	rel, err := b.fetchLatestRelease(ctx, p.Source)
	if err != nil {
		return fmt.Errorf("github api (%s): %w", p.Source, err)
	}
	asset := pickAsset(rel.Assets, assetPattern)
	if asset == nil {
		return fmt.Errorf("no asset in %s matches %q (assets: %s)",
			p.Source, p.Asset, listAssetNames(rel.Assets))
	}

	tarData, err := b.downloadAsset(ctx, asset.BrowserDownloadURL)
	if err != nil {
		return fmt.Errorf("download %s: %w", asset.Name, err)
	}

	binData, err := extractBinaryFromTarGz(tarData, p.Binary)
	if err != nil {
		return fmt.Errorf("extract %q from %s: %w", p.Binary, asset.Name, err)
	}

	if err := os.MkdirAll(b.InstallDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", b.InstallDir, err)
	}
	target := filepath.Join(b.InstallDir, p.Binary)
	if err := writeFileAtomic(target, binData, 0o755); err != nil {
		return fmt.Errorf("install %s: %w", target, err)
	}
	_ = item // reserved for future use (logging, notes)
	return nil
}

// --- GitHub API types/helpers ---

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func (b *BinaryRelease) fetchLatestRelease(ctx context.Context, source string) (*githubRelease, error) {
	url := "https://api.github.com/repos/" + source + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := b.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // 4 MiB cap
	if err != nil {
		return nil, err
	}
	var rel githubRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, fmt.Errorf("decode release json: %w", err)
	}
	return &rel, nil
}

func pickAsset(assets []githubAsset, pat *regexp.Regexp) *githubAsset {
	for i := range assets {
		if pat.MatchString(assets[i].Name) {
			return &assets[i]
		}
	}
	return nil
}

func listAssetNames(assets []githubAsset) string {
	out := ""
	for i, a := range assets {
		if i > 0 {
			out += ", "
		}
		out += a.Name
	}
	return out
}

func (b *BinaryRelease) downloadAsset(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 256<<20)) // 256 MiB cap
}

// extractBinaryFromTarGz walks a gzipped tar archive and returns the
// contents of the first regular file whose basename matches bin.
func extractBinaryFromTarGz(data []byte, bin string) ([]byte, error) {
	gzr, err := gzip.NewReader(bytesReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	defer func() { _ = gzr.Close() }()

	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("binary %q not found in archive", bin)
		}
		if err != nil {
			return nil, fmt.Errorf("tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) == bin {
			return io.ReadAll(io.LimitReader(tr, 128<<20))
		}
	}
}

// writeFileAtomic writes data to path via a sibling temp file + rename.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".onboardctl-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// lookPath is a thin wrapper; kept separate so tests can stub it in future.
func lookPath(name string) (string, error) { return lookPathImpl(name) }
