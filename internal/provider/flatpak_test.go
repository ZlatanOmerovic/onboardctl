package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
)

func TestFlatpakCheckInstalledUserScope(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"flatpak --version":                              {stdout: "Flatpak 1.15.8"},
		"flatpak info --user org.mozilla.firefox":         {stdout: flatpakInfoOutput("147.0")},
	}}
	p := NewFlatpakWith(f)
	st, err := p.Check(context.Background(), manifest.Item{}, manifest.Provider{ID: "org.mozilla.firefox"})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if !st.Installed {
		t.Error("expected Installed=true")
	}
	if st.Version != "147.0" {
		t.Errorf("Version = %q, want 147.0", st.Version)
	}
	if st.ProviderUsed != manifest.KindFlatpak {
		t.Errorf("ProviderUsed = %q, want flatpak", st.ProviderUsed)
	}
}

func TestFlatpakCheckInstalledSystemScopeFallback(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"flatpak --version":                        {stdout: "Flatpak 1.15.8"},
		"flatpak info --user org.mozilla.firefox":   {err: errors.New("not installed user")},
		"flatpak info org.mozilla.firefox":          {stdout: flatpakInfoOutput("126.0")},
	}}
	p := NewFlatpakWith(f)
	st, err := p.Check(context.Background(), manifest.Item{}, manifest.Provider{ID: "org.mozilla.firefox"})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if !st.Installed || st.Version != "126.0" {
		t.Errorf("state = %+v, want installed=true version=126.0", st)
	}
}

func TestFlatpakCheckNotInstalled(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"flatpak --version":                {stdout: "Flatpak 1.15.8"},
		"flatpak info --user com.bogus":    {err: errors.New("not installed")},
		"flatpak info com.bogus":           {err: errors.New("not installed")},
	}}
	p := NewFlatpakWith(f)
	st, err := p.Check(context.Background(), manifest.Item{}, manifest.Provider{ID: "com.bogus"})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if st.Installed {
		t.Error("expected Installed=false")
	}
}

func TestFlatpakCheckBinaryMissingReturnsNotInstalled(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"flatpak --version": {err: errors.New("command not found")},
	}}
	p := NewFlatpakWith(f)
	st, err := p.Check(context.Background(), manifest.Item{}, manifest.Provider{ID: "org.foo.bar"})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if st.Installed {
		t.Error("expected Installed=false when flatpak binary is absent")
	}
}

func TestFlatpakCheckRequiresID(t *testing.T) {
	p := NewFlatpakWith(&fakeRunner{})
	_, err := p.Check(context.Background(), manifest.Item{}, manifest.Provider{})
	if err == nil {
		t.Fatal("expected error when provider.id is empty")
	}
}

func TestFlatpakInstallHappyPath(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"flatpak --version": {stdout: "Flatpak 1.15.8"},
		"flatpak remote-add --if-not-exists flathub https://flathub.org/repo/flathub.flatpakrepo": {stdout: ""},
		"flatpak install -y --noninteractive flathub org.mozilla.firefox":                        {stdout: "Installation complete."},
	}}
	p := NewFlatpakWith(f)
	err := p.Install(context.Background(),
		manifest.Item{Name: "Firefox"},
		manifest.Provider{ID: "org.mozilla.firefox"},
	)
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}
}

func TestFlatpakInstallRemoteOverride(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"flatpak --version":                                             {stdout: "Flatpak 1.15.8"},
		"flatpak install -y --noninteractive my-remote org.example.App": {stdout: "Installation complete."},
	}}
	p := NewFlatpakWith(f)
	err := p.Install(context.Background(),
		manifest.Item{Name: "Example"},
		manifest.Provider{ID: "org.example.App", Extra: map[string]string{"remote": "my-remote"}},
	)
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}
	// flathub ensure should NOT be in the call list.
	for _, call := range f.calls {
		if call == "flatpak remote-add --if-not-exists flathub https://flathub.org/repo/flathub.flatpakrepo" {
			t.Errorf("flathub remote-add should not run when remote is overridden (calls: %v)", f.calls)
		}
	}
}

func TestFlatpakInstallUserScope(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"flatpak --version": {stdout: "Flatpak 1.15.8"},
		"flatpak remote-add --if-not-exists flathub https://flathub.org/repo/flathub.flatpakrepo": {stdout: ""},
		"flatpak install -y --noninteractive --user flathub org.gnome.Calendar":                  {stdout: "Installation complete."},
	}}
	p := NewFlatpakWith(f)
	err := p.Install(context.Background(),
		manifest.Item{Name: "GNOME Calendar"},
		manifest.Provider{ID: "org.gnome.Calendar", Extra: map[string]string{"scope": "user"}},
	)
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}
}

func TestFlatpakInstallErrorOnMissingBinary(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"flatpak --version": {err: errors.New("command not found")},
	}}
	p := NewFlatpakWith(f)
	err := p.Install(context.Background(),
		manifest.Item{Name: "Firefox"},
		manifest.Provider{ID: "org.mozilla.firefox"},
	)
	if err == nil {
		t.Fatal("expected error when flatpak binary missing")
	}
}

func TestFlatpakRemoteAddIsCachedPerInstance(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"flatpak --version": {stdout: "Flatpak 1.15.8"},
		"flatpak remote-add --if-not-exists flathub https://flathub.org/repo/flathub.flatpakrepo": {stdout: ""},
		"flatpak install -y --noninteractive flathub org.one.App":                                 {stdout: "ok"},
		"flatpak install -y --noninteractive flathub org.two.App":                                 {stdout: "ok"},
	}}
	p := NewFlatpakWith(f)
	ctx := context.Background()
	_ = p.Install(ctx, manifest.Item{}, manifest.Provider{ID: "org.one.App"})
	_ = p.Install(ctx, manifest.Item{}, manifest.Provider{ID: "org.two.App"})

	count := 0
	for _, c := range f.calls {
		if c == "flatpak remote-add --if-not-exists flathub https://flathub.org/repo/flathub.flatpakrepo" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected remote-add to run exactly once, got %d (calls: %v)", count, f.calls)
	}
}

func TestParseFlatpakVersion(t *testing.T) {
	cases := map[string]string{
		flatpakInfoOutput("147.0"): "147.0",
		"":                         "",
		"no version line here":     "",
	}
	for in, want := range cases {
		if got := parseFlatpakVersion(in); got != want {
			t.Errorf("parseFlatpakVersion = %q, want %q (input: %q)", got, want, in)
		}
	}
}

// flatpakInfoOutput returns a plausible `flatpak info` payload.
func flatpakInfoOutput(version string) string {
	return `
Firefox - The fast, private and safe web browser

          ID: org.mozilla.firefox
         Ref: app/org.mozilla.firefox/x86_64/stable
        Arch: x86_64
      Branch: stable
     Version: ` + version + `
       Origin: flathub
`
}
