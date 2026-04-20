package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
)

func TestShellCheckNilCheckSaysNotInstalled(t *testing.T) {
	s := NewShellWith(&fakeRunner{})
	st, err := s.Check(context.Background(), manifest.Item{}, manifest.Provider{})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if st.Installed {
		t.Error("empty Check should report Installed=false")
	}
}

func TestShellCheckZeroExitIsInstalled(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"bash -c command -v mise >/dev/null 2>&1": {stdout: ""},
	}}
	s := NewShellWith(f)
	st, err := s.Check(context.Background(), manifest.Item{},
		manifest.Provider{Check: "command -v mise >/dev/null 2>&1"})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if !st.Installed {
		t.Error("expected Installed=true")
	}
}

func TestShellCheckNonZeroIsNotInstalled(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"bash -c command -v bogus": {err: errors.New("exit 1")},
	}}
	s := NewShellWith(f)
	st, err := s.Check(context.Background(), manifest.Item{},
		manifest.Provider{Check: "command -v bogus"})
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if st.Installed {
		t.Error("expected Installed=false on non-zero Check exit")
	}
}

func TestShellInstallRunsApplyInOrder(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"bash -c step1": {stdout: "ok"},
		"bash -c step2": {stdout: "ok"},
		"bash -c step3": {stdout: "ok"},
	}}
	s := NewShellWith(f)
	err := s.Install(context.Background(), manifest.Item{Name: "foo"},
		manifest.Provider{Apply: []string{"step1", "step2", "step3"}})
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}
	if len(f.calls) != 3 {
		t.Fatalf("call count = %d, want 3", len(f.calls))
	}
	for i, want := range []string{"bash -c step1", "bash -c step2", "bash -c step3"} {
		if f.calls[i] != want {
			t.Errorf("call[%d] = %q, want %q", i, f.calls[i], want)
		}
	}
}

func TestShellInstallAbortsOnFirstFailure(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"bash -c step1": {stdout: "ok"},
		"bash -c step2": {stdout: "boom", err: errors.New("exit 2")},
	}}
	s := NewShellWith(f)
	err := s.Install(context.Background(), manifest.Item{Name: "foo"},
		manifest.Provider{Apply: []string{"step1", "step2", "step3"}})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if len(f.calls) != 2 {
		t.Errorf("expected abort after step2; calls = %v", f.calls)
	}
}

func TestShellInstallSkipsBlankCommands(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"bash -c real-cmd": {stdout: "ok"},
	}}
	s := NewShellWith(f)
	err := s.Install(context.Background(), manifest.Item{Name: "foo"},
		manifest.Provider{Apply: []string{"", "  ", "real-cmd"}})
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}
	if len(f.calls) != 1 {
		t.Errorf("expected blank commands skipped; calls = %v", f.calls)
	}
}

func TestShellInstallRequiresApply(t *testing.T) {
	s := NewShellWith(&fakeRunner{})
	err := s.Install(context.Background(), manifest.Item{Name: "foo"}, manifest.Provider{})
	if err == nil {
		t.Fatal("expected error on empty Apply")
	}
}
