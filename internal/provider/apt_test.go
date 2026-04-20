package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
)

// fakeRunner scripts command outputs keyed on "cmd+args" for assertions.
type fakeRunner struct {
	// responses: key "cmd arg1 arg2" -> {stdout, err}
	responses map[string]fakeResp
	calls     []string
}

type fakeResp struct {
	stdout string
	err    error
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	key := name + " " + joinArgs(args)
	f.calls = append(f.calls, key)
	if r, ok := f.responses[key]; ok {
		return []byte(r.stdout), r.err
	}
	return nil, errors.New("fakeRunner: no response for: " + key)
}

func joinArgs(args []string) string {
	out := ""
	for i, a := range args {
		if i > 0 {
			out += " "
		}
		out += a
	}
	return out
}

func TestAPTCheckInstalled(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"dpkg-query -W -f ${db:Status-Abbrev} ${Version} jq": {stdout: "ii  1.7.1-6"},
	}}
	a := NewAPTWith(f)
	st, err := a.Check(context.Background(), manifest.Item{}, manifest.Provider{Package: "jq"})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if !st.Installed {
		t.Errorf("expected installed=true, got %+v", st)
	}
	if st.Version != "1.7.1-6" {
		t.Errorf("version = %q, want 1.7.1-6", st.Version)
	}
	if st.ProviderUsed != manifest.KindAPT {
		t.Errorf("provider_used = %q, want %q", st.ProviderUsed, manifest.KindAPT)
	}
}

func TestAPTCheckNotInstalled(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"dpkg-query -W -f ${db:Status-Abbrev} ${Version} zzzz": {err: errors.New("no packages found")},
	}}
	a := NewAPTWith(f)
	st, err := a.Check(context.Background(), manifest.Item{}, manifest.Provider{Package: "zzzz"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.Installed {
		t.Errorf("expected installed=false, got %+v", st)
	}
}

func TestAPTCheckRequiresPackage(t *testing.T) {
	a := NewAPTWith(&fakeRunner{})
	_, err := a.Check(context.Background(), manifest.Item{}, manifest.Provider{})
	if err == nil {
		t.Fatal("expected error on missing package, got nil")
	}
}

func TestAPTInstall(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"apt-get install -y vlc": {stdout: "Reading package lists... Done"},
	}}
	a := NewAPTWith(f)
	if err := a.Install(context.Background(), manifest.Item{Name: "VLC"},
		manifest.Provider{Package: "vlc"}); err != nil {
		t.Fatalf("Install error: %v", err)
	}
	if len(f.calls) != 1 || f.calls[0] != "apt-get install -y vlc" {
		t.Errorf("unexpected call sequence: %v", f.calls)
	}
}

func TestAPTInstallSurfacesError(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"apt-get install -y bogus": {stdout: "E: Unable to locate package", err: errors.New("exit 100")},
	}}
	a := NewAPTWith(f)
	err := a.Install(context.Background(), manifest.Item{}, manifest.Provider{Package: "bogus"})
	if err == nil {
		t.Fatal("expected error propagation, got nil")
	}
}

func TestAPTCheckDetectsSnapDrift(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"dpkg-query -W -f ${db:Status-Abbrev} ${Version} firefox": {err: errors.New("no packages found")},
		"snap list firefox": {stdout: "Name     Version  Rev   Tracking       Publisher  Notes\nfirefox  147.0    4820  latest/stable  mozilla**  -\n"},
	}}
	a := NewAPTWith(f)
	st, err := a.Check(context.Background(), manifest.Item{}, manifest.Provider{Package: "firefox"})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if !st.Installed {
		t.Error("expected Installed=true (via snap)")
	}
	if st.ProviderUsed != "snap" {
		t.Errorf("ProviderUsed = %q, want 'snap'", st.ProviderUsed)
	}
	if st.Version != "147.0" {
		t.Errorf("Version = %q, want 147.0", st.Version)
	}
	if !st.IsProviderDrift(manifest.KindAPT) {
		t.Error("expected IsProviderDrift(apt) = true")
	}
}

func TestAPTCheckSnapMissingMeansNotInstalled(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"dpkg-query -W -f ${db:Status-Abbrev} ${Version} bogus": {err: errors.New("no packages found")},
		"snap list bogus":   {err: errors.New("error: no matching snaps installed")},
	}}
	a := NewAPTWith(f)
	st, err := a.Check(context.Background(), manifest.Item{}, manifest.Provider{Package: "bogus"})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if st.Installed {
		t.Error("expected Installed=false when neither apt nor snap has it")
	}
}

func TestAPTCheckAptBeatsSnapWhenBothPresent(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"dpkg-query -W -f ${db:Status-Abbrev} ${Version} firefox": {stdout: "ii  127.0.2-1"},
		"snap list firefox": {stdout: "Name     Version\nfirefox  147.0  4820 latest/stable mozilla -\n"},
	}}
	a := NewAPTWith(f)
	st, err := a.Check(context.Background(), manifest.Item{}, manifest.Provider{Package: "firefox"})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if st.ProviderUsed != manifest.KindAPT {
		t.Errorf("apt-installed should win over snap: ProviderUsed = %q", st.ProviderUsed)
	}
	if st.Version != "127.0.2-1" {
		t.Errorf("Version = %q, want 127.0.2-1", st.Version)
	}
}

func TestParseDpkgStatus(t *testing.T) {
	cases := []struct{ in, wantS, wantV string }{
		{"ii  1.2.3", "ii", "1.2.3"},
		{"ii 4.5", "ii", "4.5"},
		{"un ", "un", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		s, v := parseDpkgStatus(c.in)
		if s != c.wantS || v != c.wantV {
			t.Errorf("parseDpkgStatus(%q) = (%q, %q), want (%q, %q)",
				c.in, s, v, c.wantS, c.wantV)
		}
	}
}
