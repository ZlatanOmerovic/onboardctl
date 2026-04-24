package runner

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	"github.com/ZlatanOmerovic/onboardctl/internal/provider"
	"github.com/ZlatanOmerovic/onboardctl/internal/state"
)

// stubProvider lets runner tests dictate Check/Install outcomes without
// spawning real commands.
type stubProvider struct {
	kind         string
	checkState   provider.State
	checkErr     error
	installErr   error
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

// cmdRunnerStub captures bash-level command invocations made by the runner
// (currently only post_install hooks go through it).
type cmdRunnerStub struct {
	calls []string
	err   error
}

func (c *cmdRunnerStub) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	c.calls = append(c.calls, name+" "+joinArgs(args))
	return nil, c.err
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

func TestRunnerRunsPostInstallAfterInstall(t *testing.T) {
	m := mkTestManifest()
	jq := m.Items["jq"]
	jq.PostInstall = []string{"echo hello", "systemctl --user daemon-reload"}
	m.Items["jq"] = jq

	stub := &stubProvider{kind: "apt", checkState: provider.State{Installed: false}}
	cmd := &cmdRunnerStub{}
	r := &Runner{
		Manifest: m,
		Registry: buildRegistry(stub),
		State:    state.New(),
		StateFn:  func(*state.State) error { return nil },
		Out:      io.Discard,
		Cmd:      cmd,
	}
	if _, err := r.Run(context.Background(), Selection{Bundle: "base"}, Options{}); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	// jq has two post_install commands; vlc has none.
	want := []string{
		"bash -c echo hello",
		"bash -c systemctl --user daemon-reload",
	}
	if len(cmd.calls) != len(want) {
		t.Fatalf("post_install calls = %v, want %v", cmd.calls, want)
	}
	for i, c := range cmd.calls {
		if c != want[i] {
			t.Errorf("call[%d] = %q, want %q", i, c, want[i])
		}
	}
}

func TestRunnerPostInstallFailureRecordedAsItemFailure(t *testing.T) {
	m := mkTestManifest()
	jq := m.Items["jq"]
	jq.PostInstall = []string{"false"}
	m.Items["jq"] = jq

	stub := &stubProvider{kind: "apt", checkState: provider.State{Installed: false}}
	cmd := &cmdRunnerStub{err: errors.New("exit status 1")}
	r := &Runner{
		Manifest: m,
		Registry: buildRegistry(stub),
		State:    state.New(),
		StateFn:  func(*state.State) error { return nil },
		Out:      io.Discard,
		Cmd:      cmd,
	}
	sum, err := r.Run(context.Background(), Selection{Bundle: "base"}, Options{})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if _, ok := sum.Failed["jq"]; !ok {
		t.Errorf("expected jq to fail due to post_install, Failed=%v", sum.Failed)
	}
}

func TestRunnerSkipsPostInstallInDryRun(t *testing.T) {
	m := mkTestManifest()
	jq := m.Items["jq"]
	jq.PostInstall = []string{"echo should-not-run"}
	m.Items["jq"] = jq

	stub := &stubProvider{kind: "apt", checkState: provider.State{Installed: false}}
	cmd := &cmdRunnerStub{}
	r := &Runner{
		Manifest: m,
		Registry: buildRegistry(stub),
		State:    state.New(),
		StateFn:  func(*state.State) error { return nil },
		Out:      io.Discard,
		Cmd:      cmd,
	}
	if _, err := r.Run(context.Background(), Selection{Bundle: "base"}, Options{DryRun: true}); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(cmd.calls) != 0 {
		t.Errorf("post_install should be skipped in dry-run, got calls: %v", cmd.calls)
	}
}

// stubUninstallProvider extends stubProvider with an Uninstall method
// so the runner can type-assert Uninstaller.
type stubUninstallProvider struct {
	stubProvider
	uninstallCount int
	uninstallIDs   []string
	uninstallErr   error
}

func (s *stubUninstallProvider) Uninstall(_ context.Context, it manifest.Item, _ manifest.Provider) error {
	s.uninstallCount++
	s.uninstallIDs = append(s.uninstallIDs, it.Name)
	return s.uninstallErr
}

func TestRunnerRollbackOnFailureUndoesPriorInstalls(t *testing.T) {
	m := mkTestManifest()
	// Make vlc's install fail so rollback fires after jq installs successfully.
	perItem := &perItemStub{
		kind: "apt",
		checkState: map[string]provider.State{
			"jq": {Installed: false}, "vlc": {Installed: false},
		},
		installErr: map[string]error{
			"vlc": errors.New("apt-get fell over"),
		},
	}
	r := &Runner{
		Manifest: m,
		Registry: buildRegistry(perItem),
		State:    state.New(),
		StateFn:  func(*state.State) error { return nil },
		Out:      io.Discard,
	}
	sum, err := r.Run(context.Background(), Selection{Bundle: "base"}, Options{RollbackOnFailure: true})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if _, failed := sum.Failed["vlc"]; !failed {
		t.Errorf("expected vlc to be marked failed, got Failed=%v", sum.Failed)
	}
	if perItem.uninstallCount != 1 {
		t.Errorf("expected 1 rollback uninstall, got %d", perItem.uninstallCount)
	}
	// jq should have been rolled back → removed from state.
	if _, has := r.State.Items["jq"]; has {
		t.Error("jq should have been removed from state after rollback")
	}
}

func TestRollbackLastRunRepliesLIFO(t *testing.T) {
	m := mkTestManifest()
	perItem := &perItemStub{
		kind: "apt",
		checkState: map[string]provider.State{
			"jq": {Installed: false}, "vlc": {Installed: false},
		},
	}
	st := state.New()
	r := &Runner{
		Manifest: m,
		Registry: buildRegistry(perItem),
		State:    st,
		StateFn:  func(*state.State) error { return nil },
		Out:      io.Discard,
	}
	if _, err := r.Run(context.Background(), Selection{Bundle: "base"}, Options{}); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	n, _, err := r.RollbackLastRun(context.Background())
	if err != nil {
		t.Fatalf("RollbackLastRun error: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 items rolled back, got %d", n)
	}
	if len(perItem.uninstallIDs) != 2 {
		t.Fatalf("expected 2 uninstalls, got %v", perItem.uninstallIDs)
	}
	// LIFO: last installed (vlc) must be first uninstalled.
	if perItem.uninstallIDs[0] != "vlc" || perItem.uninstallIDs[1] != "jq" {
		t.Errorf("expected LIFO (vlc, jq), got %v", perItem.uninstallIDs)
	}
	// The Run should now be marked rolled back.
	if st.Runs[len(st.Runs)-1].RolledBackAt.IsZero() {
		t.Error("expected run to be marked RolledBackAt")
	}
}

func TestRollbackLastRunErrorsWhenNothingToRollBack(t *testing.T) {
	r := &Runner{Manifest: mkTestManifest(), Registry: provider.NewRegistry(), State: state.New(), Out: io.Discard}
	if _, _, err := r.RollbackLastRun(context.Background()); err == nil {
		t.Fatal("expected error on empty state")
	}
}

// perItemStub dispatches Check/Install/Uninstall per-item via a lookup table.
// Parallel install tests hit the stub concurrently, so all counters live
// under mu.
type perItemStub struct {
	kind           string
	checkState     map[string]provider.State
	installErr     map[string]error
	mu             sync.Mutex
	installCount   int
	uninstallCount int
	uninstallIDs   []string
	uninstallErr   error
}

func (p *perItemStub) Kind() string { return p.kind }

func (p *perItemStub) Check(_ context.Context, it manifest.Item, _ manifest.Provider) (provider.State, error) {
	return p.checkState[it.Name], nil
}

func (p *perItemStub) Install(_ context.Context, it manifest.Item, _ manifest.Provider) error {
	p.mu.Lock()
	p.installCount++
	p.mu.Unlock()
	if err := p.installErr[it.Name]; err != nil {
		return err
	}
	return nil
}

func (p *perItemStub) Uninstall(_ context.Context, it manifest.Item, _ manifest.Provider) error {
	p.mu.Lock()
	p.uninstallCount++
	p.uninstallIDs = append(p.uninstallIDs, it.Name)
	p.mu.Unlock()
	return p.uninstallErr
}

// TestRunnerParallelInstallsFlatpakAndAPTCorrectly verifies that when a
// mix of serial (apt) and parallel (flatpak) items are planned, both
// groups install, state is consistent, and the per-run rollback list
// contains every successful install.
func TestRunnerParallelInstallsAllItems(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Items: map[string]manifest.Item{
			"jq":      {Name: "jq", Providers: []manifest.Provider{{Type: "apt", Package: "jq"}}},
			"vlc":     {Name: "vlc", Providers: []manifest.Provider{{Type: "apt", Package: "vlc"}}},
			"firefox": {Name: "firefox", Providers: []manifest.Provider{{Type: "flatpak", ID: "org.mozilla.firefox"}}},
			"code":    {Name: "code", Providers: []manifest.Provider{{Type: "flatpak", ID: "com.visualstudio.code"}}},
		},
		Bundles: map[string]manifest.Bundle{
			"mixed": {Name: "mixed", Items: []string{"jq", "vlc", "firefox", "code"}},
		},
		Profiles: map[string]manifest.Profile{
			"p": {Name: "p", Bundles: []string{"mixed"}},
		},
	}
	apt := &perItemStub{kind: "apt", checkState: map[string]provider.State{
		"jq": {Installed: false}, "vlc": {Installed: false},
	}}
	flat := &perItemStub{kind: "flatpak", checkState: map[string]provider.State{
		"firefox": {Installed: false}, "code": {Installed: false},
	}}
	reg := provider.NewRegistry()
	reg.Register(apt)
	reg.Register(flat)

	r := &Runner{
		Manifest: m,
		Registry: reg,
		State:    state.New(),
		StateFn:  func(*state.State) error { return nil },
		Out:      io.Discard,
	}
	sum, err := r.Run(context.Background(), Selection{Bundle: "mixed"}, Options{})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(sum.Installed) != 4 {
		t.Errorf("expected 4 installs, got %v", sum.Installed)
	}
	if len(sum.Failed) != 0 {
		t.Errorf("unexpected failures: %v", sum.Failed)
	}
	if apt.installCount != 2 {
		t.Errorf("apt installs = %d, want 2", apt.installCount)
	}
	if flat.installCount != 2 {
		t.Errorf("flatpak installs = %d, want 2", flat.installCount)
	}
	// Run record should include all four (serial + parallel).
	if got := len(r.State.Runs[0].Installed); got != 4 {
		t.Errorf("run.Installed = %d, want 4", got)
	}
}

// Running with -race checks that concurrent state/sum writes are
// protected by r.mu. A flaky race detector on this test would surface
// a missing lock.

func TestRunnerOfflineRefusesNetworkProviders(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Items: map[string]manifest.Item{
			"jq":     {Name: "jq", Providers: []manifest.Provider{{Type: "apt", Package: "jq"}}},
			"config": {Name: "config", Providers: []manifest.Provider{{Type: "config", Apply: []string{"echo ok"}}}},
		},
		Bundles: map[string]manifest.Bundle{
			"mix": {Name: "mix", Items: []string{"jq", "config"}},
		},
		Profiles: map[string]manifest.Profile{"p": {Name: "p", Bundles: []string{"mix"}}},
	}
	apt := &perItemStub{kind: "apt", checkState: map[string]provider.State{
		"jq": {Installed: false},
	}}
	cfg := &perItemStub{kind: "config", checkState: map[string]provider.State{
		"config": {Installed: false},
	}}
	reg := provider.NewRegistry()
	reg.Register(apt)
	reg.Register(cfg)

	r := &Runner{
		Manifest: m,
		Registry: reg,
		State:    state.New(),
		StateFn:  func(*state.State) error { return nil },
		Out:      io.Discard,
	}
	sum, err := r.Run(context.Background(), Selection{Bundle: "mix"}, Options{Offline: true})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if _, failed := sum.Failed["jq"]; !failed {
		t.Errorf("expected jq to fail under --offline, Failed=%v", sum.Failed)
	}
	if apt.installCount != 0 {
		t.Errorf("apt install should not have run under --offline, count=%d", apt.installCount)
	}
	if cfg.installCount != 1 {
		t.Errorf("config install should still run under --offline, count=%d", cfg.installCount)
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
