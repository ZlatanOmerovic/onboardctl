package runner

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	"github.com/ZlatanOmerovic/onboardctl/internal/provider"
	"github.com/ZlatanOmerovic/onboardctl/internal/state"
)

// stubProvider lets runner tests dictate Check/Install outcomes without
// spawning real commands.
type stubProvider struct {
	kind       string
	checkState provider.State
	checkErr   error
	installErr error
	installCount int
}

func (s *stubProvider) Kind() string { return s.kind }
func (s *stubProvider) Check(_ context.Context, _ manifest.Item, _ manifest.Provider) (provider.State, error) {
	return s.checkState, s.checkErr
}
func (s *stubProvider) Install(_ context.Context, _ manifest.Item, _ manifest.Provider) error {
	s.installCount++
	return s.installErr
}

func buildRegistry(p provider.Provider) provider.Registry {
	r := provider.NewRegistry()
	r.Register(p)
	return r
}

func TestRunnerDryRunDoesNotInstallOrSaveState(t *testing.T) {
	m := mkTestManifest()
	stub := &stubProvider{kind: "apt", checkState: provider.State{Installed: false}}
	saved := 0
	r := &Runner{
		Manifest: m,
		Registry: buildRegistry(stub),
		State:    state.New(),
		StateFn:  func(*state.State) error { saved++; return nil },
		Out:      io.Discard,
	}
	sum, err := r.Run(context.Background(), Selection{Bundle: "base"}, Options{DryRun: true})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if stub.installCount != 0 {
		t.Errorf("expected 0 installs in dry-run, got %d", stub.installCount)
	}
	if saved != 0 {
		t.Errorf("expected no state save in dry-run, got %d saves", saved)
	}
	if len(sum.Installed) != 2 { // planned, not executed — "would install"
		t.Errorf("expected 2 planned items, got %v", sum.Installed)
	}
}

func TestRunnerInstallsAndSavesState(t *testing.T) {
	m := mkTestManifest()
	stub := &stubProvider{kind: "apt", checkState: provider.State{Installed: false}}
	saved := false
	r := &Runner{
		Manifest: m,
		Registry: buildRegistry(stub),
		State:    state.New(),
		StateFn:  func(*state.State) error { saved = true; return nil },
		Out:      io.Discard,
	}
	sum, err := r.Run(context.Background(), Selection{Bundle: "base"}, Options{})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if stub.installCount != 2 {
		t.Errorf("expected 2 installs, got %d", stub.installCount)
	}
	if !saved {
		t.Error("state was not saved")
	}
	if len(sum.Installed) != 2 {
		t.Errorf("summary installed = %v, want 2", sum.Installed)
	}
	// Both jq and vlc should be in state.Items now.
	for _, id := range []string{"jq", "vlc"} {
		if _, ok := r.State.Items[id]; !ok {
			t.Errorf("state missing item %q after install", id)
		}
	}
}

func TestRunnerSkipsAlreadyInstalled(t *testing.T) {
	m := mkTestManifest()
	stub := &stubProvider{kind: "apt", checkState: provider.State{Installed: true, Version: "1.0"}}
	r := &Runner{
		Manifest: m,
		Registry: buildRegistry(stub),
		State:    state.New(),
		StateFn:  func(*state.State) error { return nil },
		Out:      io.Discard,
	}
	sum, err := r.Run(context.Background(), Selection{Bundle: "base"}, Options{})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if stub.installCount != 0 {
		t.Errorf("expected 0 installs when already-installed, got %d", stub.installCount)
	}
	if len(sum.AlreadyHad) != 2 {
		t.Errorf("AlreadyHad = %v, want 2", sum.AlreadyHad)
	}
}

func TestRunnerRecordsFailuresButContinues(t *testing.T) {
	m := mkTestManifest()
	stub := &stubProvider{
		kind:       "apt",
		checkState: provider.State{Installed: false},
		installErr: errors.New("apt-get fell over"),
	}
	r := &Runner{
		Manifest: m,
		Registry: buildRegistry(stub),
		State:    state.New(),
		StateFn:  func(*state.State) error { return nil },
		Out:      io.Discard,
	}
	sum, err := r.Run(context.Background(), Selection{Bundle: "base"}, Options{})
	if err != nil {
		t.Fatalf("Run unexpectedly errored: %v", err)
	}
	if len(sum.Failed) != 2 {
		t.Errorf("Failed = %v, want 2 entries", sum.Failed)
	}
}

func TestRunnerUnregisteredProviderKind(t *testing.T) {
	m := mkTestManifest()
	// Registry has nothing — no apt provider registered.
	r := &Runner{
		Manifest: m,
		Registry: provider.NewRegistry(),
		State:    state.New(),
		StateFn:  func(*state.State) error { return nil },
		Out:      io.Discard,
	}
	sum, err := r.Run(context.Background(), Selection{Bundle: "base"}, Options{})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(sum.Failed) != 2 {
		t.Errorf("expected both items failed (no provider), got Failed=%v", sum.Failed)
	}
}
