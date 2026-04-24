package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseURLs(t *testing.T) {
	arch, cs, name := releaseURLs("ZlatanOmerovic/onboardctl", "v0.2.0")
	wantName := "onboardctl_0.2.0_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
	if name != wantName {
		t.Errorf("asset name = %q, want %q", name, wantName)
	}
	if !strings.Contains(arch, "v0.2.0") || !strings.HasSuffix(arch, wantName) {
		t.Errorf("archive URL wrong: %s", arch)
	}
	if !strings.HasSuffix(cs, "onboardctl_0.2.0_checksums.txt") {
		t.Errorf("checksum URL wrong: %s", cs)
	}
}

func TestVerifySHA256Pass(t *testing.T) {
	archive := []byte("fake release tarball bytes")
	sum := sha256.Sum256(archive)
	sums := hex.EncodeToString(sum[:]) + "  onboardctl_0.2.0_linux_amd64.tar.gz\n"
	if err := verifySHA256(archive, "onboardctl_0.2.0_linux_amd64.tar.gz", []byte(sums)); err != nil {
		t.Errorf("expected pass, got %v", err)
	}
}

func TestVerifySHA256Mismatch(t *testing.T) {
	archive := []byte("fake bytes")
	sums := strings.Repeat("0", 64) + "  onboardctl_0.2.0_linux_amd64.tar.gz\n"
	err := verifySHA256(archive, "onboardctl_0.2.0_linux_amd64.tar.gz", []byte(sums))
	if err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Errorf("expected mismatch error, got %v", err)
	}
}

func TestVerifySHA256MissingEntry(t *testing.T) {
	sums := "abc123  other-file.tar.gz\n"
	err := verifySHA256([]byte("x"), "wanted.tar.gz", []byte(sums))
	if err == nil || !strings.Contains(err.Error(), "no checksum entry") {
		t.Errorf("expected missing-entry error, got %v", err)
	}
}

func TestExtractOnboardctl(t *testing.T) {
	bin := []byte("this is the onboardctl binary\n")
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	// First a decoy file to prove we skip non-matching entries.
	_ = tw.WriteHeader(&tar.Header{Name: "README.md", Mode: 0o644, Size: 3, Typeflag: tar.TypeReg})
	_, _ = tw.Write([]byte("hi\n"))
	_ = tw.WriteHeader(&tar.Header{Name: "onboardctl", Mode: 0o755, Size: int64(len(bin)), Typeflag: tar.TypeReg})
	_, _ = tw.Write(bin)
	_ = tw.Close()
	_ = gzw.Close()

	got, err := extractOnboardctl(buf.Bytes())
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}
	if !bytes.Equal(got, bin) {
		t.Errorf("extracted %q, want %q", got, bin)
	}
}
