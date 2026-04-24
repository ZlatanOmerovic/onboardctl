package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ZlatanOmerovic/onboardctl/internal/update"
)

var upgradeOpts struct {
	check   bool
	version string
	repo    string
}

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Download the latest release and replace the running binary",
	Long: `upgrade fetches the newest (or --version pinned) release of onboardctl
from GitHub, verifies its SHA-256 against the release's checksums.txt,
and atomically replaces the currently-running binary.

The replace step happens in the same directory as the live executable,
so if that directory is not writable (/usr/local/bin typically isn't),
run upgrade with sudo.

Use --check to preview what would happen without touching anything.`,
	RunE: runUpgrade,
}

func init() {
	upgradeCmd.Flags().BoolVar(&upgradeOpts.check, "check", false, "print the latest version and exit without installing")
	upgradeCmd.Flags().StringVar(&upgradeOpts.version, "version", "", "target release (e.g. v0.3.0); default: latest")
	upgradeCmd.Flags().StringVar(&upgradeOpts.repo, "repo", "", "override GitHub owner/repo (default: ZlatanOmerovic/onboardctl)")
	rootCmd.AddCommand(upgradeCmd)
}

func runUpgrade(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()

	repo := upgradeOpts.repo
	if repo == "" {
		repo = update.DefaultRepo
	}

	// Resolve target tag.
	var target string
	if upgradeOpts.version != "" {
		target = upgradeOpts.version
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		r := update.Check(ctx, Version, update.Options{Repo: repo})
		if r.LatestTag == "" {
			return fmt.Errorf("could not resolve latest release: %s", firstNonBlank(r.SilentReason, "unknown error"))
		}
		target = r.LatestTag
		if !r.HasUpdate && Version != "dev" && Version != "unknown" {
			fmt.Fprintf(out, "Already on the latest release (%s).\n", r.LatestTag)
			return nil
		}
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate current binary: %w", err)
	}
	// Resolve symlinks so we replace the real file, not a dangling symlink.
	if resolved, err := filepath.EvalSymlinks(exe); err == nil && resolved != "" {
		exe = resolved
	}

	fmt.Fprintf(out, "Current: %s (%s)\n", Version, exe)
	fmt.Fprintf(out, "Target:  %s\n", target)

	if upgradeOpts.check {
		fmt.Fprintln(out, "\n--check passed; not downloading.")
		return nil
	}

	// Guard: destination dir must be writable.
	dir := filepath.Dir(exe)
	if unix, err := canWriteDir(dir); err != nil || !unix {
		if err != nil {
			return fmt.Errorf("probe %s: %w", dir, err)
		}
		return fmt.Errorf("%s is not writable by this user — re-run with sudo", dir)
	}

	archiveURL, checksumURL, assetName := releaseURLs(repo, target)

	fmt.Fprintf(out, "\nDownloading %s...\n", assetName)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	archive, err := httpGet(ctx, archiveURL)
	if err != nil {
		return fmt.Errorf("download %s: %w", assetName, err)
	}

	// Verify checksum if the release publishes one. Missing checksum is
	// treated as a warning, not a hard failure — older releases may not
	// have shipped the sums file.
	if sums, err := httpGet(ctx, checksumURL); err == nil {
		if err := verifySHA256(archive, assetName, sums); err != nil {
			return fmt.Errorf("checksum verification: %w", err)
		}
		fmt.Fprintln(out, "Checksum verified.")
	} else {
		fmt.Fprintf(out, "Warning: could not fetch checksums.txt (%v); continuing without verification.\n", err)
	}

	fmt.Fprintln(out, "Extracting...")
	newBin, err := extractOnboardctl(archive)
	if err != nil {
		return fmt.Errorf("extract: %w", err)
	}

	fmt.Fprintf(out, "Replacing %s...\n", exe)
	if err := atomicReplace(exe, newBin); err != nil {
		return fmt.Errorf("install new binary: %w", err)
	}

	fmt.Fprintf(out, "\nUpgraded to %s.\n", target)
	return nil
}

// releaseURLs builds the expected GoReleaser asset URLs for this OS/arch.
func releaseURLs(repo, tag string) (archive, checksum, assetName string) {
	nov := strings.TrimPrefix(tag, "v")
	assetName = fmt.Sprintf("onboardctl_%s_%s_%s.tar.gz", nov, runtime.GOOS, runtime.GOARCH)
	base := fmt.Sprintf("https://github.com/%s/releases/download/%s", repo, tag)
	return base + "/" + assetName, base + "/onboardctl_" + nov + "_checksums.txt", assetName
}

func httpGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 128<<20)) // 128 MiB cap
}

// verifySHA256 finds the line for assetName in sums (`<hex>  <name>`) and
// compares it to the computed SHA-256 of archive.
func verifySHA256(archive []byte, assetName string, sums []byte) error {
	sum := sha256.Sum256(archive)
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == assetName {
			want = fields[0]
			break
		}
	}
	if want == "" {
		return fmt.Errorf("no checksum entry for %s in checksums.txt", assetName)
	}
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(want, got) {
		return fmt.Errorf("checksum mismatch: got %s, want %s", got, want)
	}
	return nil
}

// extractOnboardctl walks the gzipped tar and returns the bytes of the
// first regular file whose basename is "onboardctl".
func extractOnboardctl(archive []byte) ([]byte, error) {
	gzr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	defer func() { _ = gzr.Close() }()
	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, errors.New("archive did not contain an onboardctl binary")
		}
		if err != nil {
			return nil, fmt.Errorf("tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) == "onboardctl" {
			return io.ReadAll(io.LimitReader(tr, 128<<20))
		}
	}
}

// atomicReplace writes data to a sibling temp file and renames it over
// dst. On Linux, rename is atomic within a filesystem — the running
// binary keeps the old inode until it exits.
func atomicReplace(dst string, data []byte) error {
	dir := filepath.Dir(dst)
	tmp, err := os.CreateTemp(dir, ".onboardctl-upgrade-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, dst)
}

// canWriteDir probes whether the current user can write to dir by
// creating (and immediately removing) a dotfile inside it.
func canWriteDir(dir string) (bool, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("%s is not a directory", dir)
	}
	f, err := os.CreateTemp(dir, ".onboardctl-writetest-*.tmp")
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return false, nil
		}
		return false, err
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true, nil
}
