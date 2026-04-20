package provider

import (
	"context"
	"testing"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
)

func TestConfigKind(t *testing.T) {
	c := NewConfigWith(&fakeRunner{})
	if c.Kind() != manifest.KindConfig {
		t.Errorf("Kind = %q, want %q", c.Kind(), manifest.KindConfig)
	}
}

func TestConfigInstallRefusesItemWithInput(t *testing.T) {
	c := NewConfigWith(&fakeRunner{})
	err := c.Install(context.Background(),
		manifest.Item{Name: "System timezone", Input: &manifest.Input{Kind: "choice"}},
		manifest.Provider{Apply: []string{"timedatectl set-timezone {value}"}})
	if err == nil {
		t.Fatal("expected Install to refuse item with Input, got nil error")
	}
}

func TestConfigInstallRunsApplyWhenNoInput(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"bash -c echo hello": {stdout: "hello"},
	}}
	c := NewConfigWith(f)
	err := c.Install(context.Background(),
		manifest.Item{Name: "noop"},
		manifest.Provider{Apply: []string{"echo hello"}})
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}
}

func TestConfigCheckStampsProviderUsed(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"bash -c timedatectl show -p Timezone --value": {stdout: "Europe/Sarajevo"},
	}}
	c := NewConfigWith(f)
	st, err := c.Check(context.Background(),
		manifest.Item{Name: "System timezone"},
		manifest.Provider{Check: "timedatectl show -p Timezone --value"})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if !st.Installed {
		t.Fatal("expected Installed=true")
	}
	if st.ProviderUsed != manifest.KindConfig {
		t.Errorf("ProviderUsed = %q, want %q", st.ProviderUsed, manifest.KindConfig)
	}
}
