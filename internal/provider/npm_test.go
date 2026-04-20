package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
)

func TestNPMCheckInstalled(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"npm ls -g --depth=0 yarn": {stdout: "/usr/lib/node_modules\n└── yarn@1.22.22\n"},
	}}
	p := NewNPMGlobalWith(f)
	st, err := p.Check(context.Background(), manifest.Item{}, manifest.Provider{Package: "yarn"})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if !st.Installed {
		t.Error("expected Installed=true")
	}
	if st.Version != "1.22.22" {
		t.Errorf("Version = %q, want 1.22.22", st.Version)
	}
	if st.ProviderUsed != manifest.KindNPMGlobal {
		t.Errorf("ProviderUsed = %q, want npm_global", st.ProviderUsed)
	}
}

func TestNPMCheckScopedPackage(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"npm ls -g --depth=0 @vue/cli": {stdout: "/usr/lib/node_modules\n└── @vue/cli@5.0.8\n"},
	}}
	p := NewNPMGlobalWith(f)
	st, err := p.Check(context.Background(), manifest.Item{}, manifest.Provider{Package: "@vue/cli"})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if !st.Installed || st.Version != "5.0.8" {
		t.Errorf("state = %+v, want installed=true version=5.0.8", st)
	}
}

func TestNPMCheckNotInstalledEmptyTree(t *testing.T) {
	// Newer npm returns exit 0 even when nothing matches; we detect by
	// absence of the marker in stdout.
	f := &fakeRunner{responses: map[string]fakeResp{
		"npm ls -g --depth=0 missing-pkg": {stdout: "/usr/lib/node_modules\n└── (empty)\n"},
	}}
	p := NewNPMGlobalWith(f)
	st, err := p.Check(context.Background(), manifest.Item{}, manifest.Provider{Package: "missing-pkg"})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if st.Installed {
		t.Error("expected Installed=false")
	}
}

func TestNPMCheckNotInstalledErrorExit(t *testing.T) {
	// Older npm (and `npm ls` when npm itself is absent) errors.
	f := &fakeRunner{responses: map[string]fakeResp{
		"npm ls -g --depth=0 anything": {err: errors.New("command not found"), stdout: ""},
	}}
	p := NewNPMGlobalWith(f)
	st, err := p.Check(context.Background(), manifest.Item{}, manifest.Provider{Package: "anything"})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if st.Installed {
		t.Error("expected Installed=false when ls errors")
	}
}

func TestNPMCheckRequiresPackage(t *testing.T) {
	p := NewNPMGlobalWith(&fakeRunner{})
	_, err := p.Check(context.Background(), manifest.Item{}, manifest.Provider{})
	if err == nil {
		t.Fatal("expected error when provider.package is empty")
	}
}

func TestNPMInstallHappyPath(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"npm --version":         {stdout: "10.9.2"},
		"npm install -g vercel": {stdout: "added 12 packages in 3s"},
	}}
	p := NewNPMGlobalWith(f)
	err := p.Install(context.Background(),
		manifest.Item{Name: "Vercel CLI"},
		manifest.Provider{Package: "vercel"},
	)
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}
}

func TestNPMInstallScopedPackage(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"npm --version":             {stdout: "10.9.2"},
		"npm install -g @vue/cli":   {stdout: "added 200 packages"},
	}}
	p := NewNPMGlobalWith(f)
	err := p.Install(context.Background(),
		manifest.Item{Name: "Vue CLI"},
		manifest.Provider{Package: "@vue/cli"},
	)
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}
}

func TestNPMInstallRequiresNPM(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"npm --version": {err: errors.New("command not found")},
	}}
	p := NewNPMGlobalWith(f)
	err := p.Install(context.Background(),
		manifest.Item{Name: "Anything"},
		manifest.Provider{Package: "anything"},
	)
	if err == nil {
		t.Fatal("expected error when npm missing")
	}
}

func TestNPMInstallSurfacesError(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"npm --version":           {stdout: "10.9.2"},
		"npm install -g bogus-pkg": {stdout: "npm ERR! 404", err: errors.New("exit 1")},
	}}
	p := NewNPMGlobalWith(f)
	err := p.Install(context.Background(),
		manifest.Item{Name: "Bogus"},
		manifest.Provider{Package: "bogus-pkg"},
	)
	if err == nil {
		t.Fatal("expected install error")
	}
}

func TestNPMInstallRequiresPackage(t *testing.T) {
	p := NewNPMGlobalWith(&fakeRunner{})
	err := p.Install(context.Background(), manifest.Item{}, manifest.Provider{})
	if err == nil {
		t.Fatal("expected error when provider.package is empty")
	}
}

func TestParseNPMVersion(t *testing.T) {
	cases := []struct {
		out, pkg, want string
	}{
		{"/usr/lib/node_modules\n└── yarn@1.22.22\n", "yarn", "1.22.22"},
		{"/usr/lib/node_modules\n└── @vue/cli@5.0.8\n", "@vue/cli", "5.0.8"},
		{"/usr/lib/node_modules\n├── yarn@1.22.22 extraneous\n", "yarn", "1.22.22"},
		{"/usr/lib/node_modules\n└── (empty)\n", "yarn", ""},
		{"", "yarn", ""},
	}
	for _, c := range cases {
		if got := parseNPMVersion(c.out, c.pkg); got != c.want {
			t.Errorf("parseNPMVersion(%q, %q) = %q, want %q", c.out, c.pkg, got, c.want)
		}
	}
}
