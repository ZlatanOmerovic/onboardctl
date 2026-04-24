package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
)

func TestComposerGlobalCheckInstalled(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"composer global show --name-only --no-interaction": {
			stdout: "laravel/installer\npsy/psysh\n",
		},
	}}
	c := NewComposerGlobalWith(f)
	st, err := c.Check(context.Background(), manifest.Item{}, manifest.Provider{Package: "laravel/installer"})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if !st.Installed {
		t.Error("expected Installed=true")
	}
}

func TestComposerGlobalCheckNotInstalled(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"composer global show --name-only --no-interaction": {stdout: "other/thing\n"},
	}}
	c := NewComposerGlobalWith(f)
	st, err := c.Check(context.Background(), manifest.Item{}, manifest.Provider{Package: "laravel/installer"})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if st.Installed {
		t.Error("expected Installed=false")
	}
}

func TestComposerGlobalCheckRequiresPackage(t *testing.T) {
	c := NewComposerGlobalWith(&fakeRunner{})
	_, err := c.Check(context.Background(), manifest.Item{}, manifest.Provider{})
	if err == nil {
		t.Fatal("expected error on empty package")
	}
}

func TestComposerGlobalCheckMissingComposerIsNotInstalled(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"composer global show --name-only --no-interaction": {err: errors.New("composer not found")},
	}}
	c := NewComposerGlobalWith(f)
	st, err := c.Check(context.Background(), manifest.Item{}, manifest.Provider{Package: "laravel/installer"})
	if err != nil {
		t.Fatalf("Check error (should not surface missing-composer error): %v", err)
	}
	if st.Installed {
		t.Error("expected Installed=false when composer itself is absent")
	}
}

func TestComposerGlobalInstall(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"composer global require --no-interaction laravel/installer": {stdout: "Using version ^5.0"},
	}}
	c := NewComposerGlobalWith(f)
	if err := c.Install(context.Background(), manifest.Item{Name: "Laravel"},
		manifest.Provider{Package: "laravel/installer"}); err != nil {
		t.Fatalf("Install error: %v", err)
	}
}

func TestComposerGlobalInstallPinnedVersion(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"composer global require --no-interaction laravel/installer:^5.0": {stdout: "OK"},
	}}
	c := NewComposerGlobalWith(f)
	if err := c.Install(context.Background(), manifest.Item{Name: "Laravel"},
		manifest.Provider{Package: "laravel/installer", Version: "^5.0"}); err != nil {
		t.Fatalf("Install error: %v", err)
	}
	if len(f.calls) != 1 || f.calls[0] != "composer global require --no-interaction laravel/installer:^5.0" {
		t.Errorf("expected pinned composer require, got: %v", f.calls)
	}
}

func TestComposerGlobalInstallSurfacesError(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"composer global require --no-interaction bogus/package": {
			stdout: "Could not find package", err: errors.New("exit 2"),
		},
	}}
	c := NewComposerGlobalWith(f)
	err := c.Install(context.Background(), manifest.Item{Name: "bogus"},
		manifest.Provider{Package: "bogus/package"})
	if err == nil {
		t.Fatal("expected install error, got nil")
	}
}
