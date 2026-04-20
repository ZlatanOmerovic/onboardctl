package provider

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
)

func TestBinaryReleaseCheckInInstallDir(t *testing.T) {
	dir := t.TempDir()
	// Drop a stub binary.
	target := filepath.Join(dir, "lazygit")
	if err := os.WriteFile(target, []byte("#!/bin/sh\necho 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	b := &BinaryRelease{InstallDir: dir}
	st, err := b.Check(context.Background(), manifest.Item{},
		manifest.Provider{Binary: "lazygit"})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if !st.Installed {
		t.Error("expected Installed=true when file exists in InstallDir")
	}
}

func TestBinaryReleaseCheckNotInstalled(t *testing.T) {
	dir := t.TempDir()
	b := &BinaryRelease{InstallDir: dir}
	st, err := b.Check(context.Background(), manifest.Item{},
		manifest.Provider{Binary: "definitely-not-installed-1234567"})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if st.Installed {
		t.Error("expected Installed=false")
	}
}

func TestPickAsset(t *testing.T) {
	assets := []githubAsset{
		{Name: "lazygit_0.61.1_Darwin_arm64.tar.gz"},
		{Name: "lazygit_0.61.1_Linux_x86_64.tar.gz"},
		{Name: "lazygit_0.61.1_Windows_x86_64.zip"},
	}
	pat := regexp.MustCompile(`Linux_x86_64\.tar\.gz$`)
	got := pickAsset(assets, pat)
	if got == nil {
		t.Fatal("expected a match")
	}
	if got.Name != "lazygit_0.61.1_Linux_x86_64.tar.gz" {
		t.Errorf("picked %q", got.Name)
	}
}

func TestExtractBinaryFromTarGz(t *testing.T) {
	// Build a tiny tar.gz: ./lazygit (regular file), ./README.md (regular).
	buf := &bytes.Buffer{}
	gz := gzip.NewWriter(buf)
	tw := tar.NewWriter(gz)
	mustWriteTar := func(name string, content []byte) {
		if err := tw.WriteHeader(&tar.Header{
			Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	mustWriteTar("lazygit", []byte("stub-binary"))
	mustWriteTar("README.md", []byte("# lazygit"))
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	out, err := extractBinaryFromTarGz(buf.Bytes(), "lazygit")
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}
	if string(out) != "stub-binary" {
		t.Errorf("extracted content = %q", out)
	}

	// Asking for a missing binary is an error.
	if _, err := extractBinaryFromTarGz(buf.Bytes(), "nope"); err == nil {
		t.Error("expected error for missing binary")
	}
}

// TestBinaryReleaseInstallEnd2End exercises the full download+extract
// pipeline against a local HTTP server that mimics GitHub.
func TestBinaryReleaseInstallEnd2End(t *testing.T) {
	// Build the tar.gz containing our stub binary.
	buf := &bytes.Buffer{}
	gz := gzip.NewWriter(buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "tool", Mode: 0o755, Size: int64(len("stub")), Typeflag: tar.TypeReg})
	_, _ = tw.Write([]byte("stub"))
	_ = tw.Close()
	_ = gz.Close()
	tarGz := buf.Bytes()

	// HTTP server answering:
	//   /repos/foo/bar/releases/latest → JSON with one asset
	//   /download/tool.tar.gz          → the tar.gz bytes
	mux := http.NewServeMux()
	var downloadURL string
	mux.HandleFunc("/repos/foo/bar/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		rel := githubRelease{
			TagName: "v1.0.0",
			Assets:  []githubAsset{{Name: "tool_Linux_x86_64.tar.gz", BrowserDownloadURL: downloadURL}},
		}
		_ = json.NewEncoder(w).Encode(rel)
	})
	mux.HandleFunc("/download/tool.tar.gz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.Copy(w, bytes.NewReader(tarGz))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	downloadURL = srv.URL + "/download/tool.tar.gz"

	// Point the provider's HTTP client at a transport that rewrites the
	// api.github.com host to the test server.
	realAPI := srv.URL + "/repos/foo/bar/releases/latest"
	client := &http.Client{
		Transport: urlRewriter{to: realAPI},
	}

	dir := t.TempDir()
	b := &BinaryRelease{HTTP: client, InstallDir: dir}
	err := b.Install(context.Background(), manifest.Item{Name: "tool"}, manifest.Provider{
		Source: "foo/bar",
		Asset:  `Linux_x86_64\.tar\.gz$`,
		Binary: "tool",
	})
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}
	target := filepath.Join(dir, "tool")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if string(data) != "stub" {
		t.Errorf("installed content = %q, want 'stub'", data)
	}
	info, _ := os.Stat(target)
	if info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %o, want 0755", info.Mode().Perm())
	}
}

// urlRewriter sends every request to a fixed URL — lets our test server
// stand in for api.github.com without network-level mocking.
type urlRewriter struct{ to string }

func (u urlRewriter) RoundTrip(req *http.Request) (*http.Response, error) {
	// For the api call, rewrite to the test server. For download URLs, pass through.
	if req.URL.Host == "api.github.com" {
		// Parse the `to` URL into scheme+host to preserve the path from the request.
		rewritten, err := http.NewRequest(req.Method, u.to, req.Body)
		if err != nil {
			return nil, err
		}
		rewritten.Header = req.Header.Clone()
		return http.DefaultTransport.RoundTrip(rewritten)
	}
	return http.DefaultTransport.RoundTrip(req)
}
