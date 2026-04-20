package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/ZlatanOmerovic/onboardctl/internal/runner"
)

func TestProgressStartSetsCurrent(t *testing.T) {
	m := NewInstallProgressModel("fullstack-web", 10)
	next, _ := m.handleProgress(runner.ProgressEvent{
		Kind: runner.ProgressStart, ItemID: "jq", Name: "jq", Total: 10, Index: 1,
	})
	m = next.(InstallProgressModel)
	if m.currentID != "jq" {
		t.Errorf("currentID = %q, want jq", m.currentID)
	}
}

func TestProgressInstalledAppendsRowAndClearsCurrent(t *testing.T) {
	m := NewInstallProgressModel("fullstack-web", 10)
	next, _ := m.handleProgress(runner.ProgressEvent{Kind: runner.ProgressStart, ItemID: "jq", Name: "jq"})
	m = next.(InstallProgressModel)
	next, _ = m.handleProgress(runner.ProgressEvent{
		Kind: runner.ProgressInstalled, ItemID: "jq", Name: "jq",
		Version: "1.7.1", Detail: "apt",
	})
	m = next.(InstallProgressModel)
	if m.currentID != "" {
		t.Errorf("currentID = %q, want empty", m.currentID)
	}
	if len(m.rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(m.rows))
	}
	if m.rows[0].id != "jq" {
		t.Errorf("row[0].id = %q, want jq", m.rows[0].id)
	}
}

func TestProgressFailedRecordsError(t *testing.T) {
	m := NewInstallProgressModel("fullstack-web", 10)
	next, _ := m.handleProgress(runner.ProgressEvent{
		Kind: runner.ProgressFailed, ItemID: "lazygit", Name: "lazygit",
		ErrMsg: "network timeout",
	})
	m = next.(InstallProgressModel)
	if len(m.rows) != 1 {
		t.Fatal("expected 1 failed row")
	}
	if !strings.Contains(m.rows[0].detail, "network timeout") {
		t.Errorf("detail missing error: %q", m.rows[0].detail)
	}
}

func TestProgressBootstrapEvents(t *testing.T) {
	m := NewInstallProgressModel("fullstack-web", 10)
	next, _ := m.handleProgress(runner.ProgressEvent{Kind: runner.ProgressBootstrapStart, Detail: "sury_php"})
	m = next.(InstallProgressModel)
	if m.bootstrap != "sury_php" {
		t.Errorf("bootstrap = %q, want sury_php", m.bootstrap)
	}
	next, _ = m.handleProgress(runner.ProgressEvent{Kind: runner.ProgressBootstrapDone})
	m = next.(InstallProgressModel)
	if m.bootstrap != "" {
		t.Errorf("bootstrap = %q, want empty after Done", m.bootstrap)
	}
}

func TestProgressFinishedMessage(t *testing.T) {
	m := NewInstallProgressModel("fullstack-web", 2)
	sum := &runner.Summary{
		Installed:  []string{"jq", "vlc"},
		AlreadyHad: []string{"git-lfs"},
	}
	next, _ := m.Update(ProgressFinishedMsg{Summary: sum})
	m = next.(InstallProgressModel)
	if !m.Done() {
		t.Error("expected Done=true after ProgressFinishedMsg")
	}
	if m.Summary() == nil || len(m.Summary().Installed) != 2 {
		t.Errorf("summary not set correctly: %+v", m.Summary())
	}
}

func TestProgressFinishedWithError(t *testing.T) {
	m := NewInstallProgressModel("fullstack-web", 0)
	next, _ := m.Update(ProgressFinishedMsg{Err: errors.New("boom")})
	m = next.(InstallProgressModel)
	if !m.Done() {
		t.Error("expected Done=true")
	}
	if m.FinalErr() == nil {
		t.Error("expected FinalErr set")
	}
}

func TestProgressViewRendersHeaderAndCounter(t *testing.T) {
	m := NewInstallProgressModel("devops", 5)
	next, _ := m.handleProgress(runner.ProgressEvent{
		Kind: runner.ProgressInstalled, ItemID: "kubectl", Name: "kubectl",
		Version: "1.32", Detail: "apt", Total: 5, Index: 1,
	})
	m = next.(InstallProgressModel)
	v := m.View()
	if !strings.Contains(v, "devops") {
		t.Errorf("view missing profile name: %q", v)
	}
	if !strings.Contains(v, "1 / 5") {
		t.Errorf("view missing counter: %q", v)
	}
	if !strings.Contains(v, "kubectl") {
		t.Errorf("view missing installed item: %q", v)
	}
}
