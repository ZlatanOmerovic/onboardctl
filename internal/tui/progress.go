package tui

import (
	"fmt"
	"strings"

	"github.com/ZlatanOmerovic/onboardctl/internal/runner"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ProgressFinishedMsg is what callers send via program.Send() when the
// runner goroutine finishes. It carries the final Summary so the model
// can render a closing report.
type ProgressFinishedMsg struct {
	Summary *runner.Summary
	Err     error
}

// InstallProgressModel renders an install as it happens:
// a header with profile + progress bar, a scrolling log of completed items
// with status glyphs, the current in-flight item with a spinner, and a
// final summary when the runner signals completion.
type InstallProgressModel struct {
	profileName string
	total       int
	rows        []completedRow
	currentID   string
	currentName string
	bootstrap   string // current repo being bootstrapped, if any
	progress    progress.Model
	spinner     spinner.Model
	done        bool
	finalSum    *runner.Summary
	finalErr    error
	quitting    bool
}

type completedRow struct {
	id      string
	name    string
	marker  string
	detail  string
	style   lipgloss.Style
}

// NewInstallProgressModel constructs the model. total is the expected
// number of items (used for the progress bar); it's safe to pass 0 and
// let the first event update it.
func NewInstallProgressModel(profileName string, total int) InstallProgressModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(ColMauve)

	pb := progress.New(
		progress.WithDefaultGradient(),
		progress.WithoutPercentage(),
	)
	pb.Width = 40

	return InstallProgressModel{
		profileName: profileName,
		total:       total,
		progress:    pb,
		spinner:     sp,
	}
}

// Init implements tea.Model.
func (m InstallProgressModel) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update implements tea.Model.
func (m InstallProgressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.done {
				m.quitting = true
				return m, tea.Quit
			}
			// Refuse to quit mid-install — it's the user's machine.
			return m, nil
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case runner.ProgressEvent:
		return m.handleProgress(msg)
	case ProgressFinishedMsg:
		m.done = true
		m.finalSum = msg.Summary
		m.finalErr = msg.Err
		return m, nil
	}
	return m, nil
}

func (m InstallProgressModel) handleProgress(e runner.ProgressEvent) (tea.Model, tea.Cmd) {
	if e.Total > 0 {
		m.total = e.Total
	}
	switch e.Kind {
	case runner.ProgressBootstrapStart:
		m.bootstrap = e.Detail
	case runner.ProgressBootstrapDone:
		m.bootstrap = ""
	case runner.ProgressStart:
		m.currentID = e.ItemID
		m.currentName = e.Name
	case runner.ProgressAlready:
		m.appendRow(e.ItemID, e.Name, GlyphInstalledByUs, StyleInstalled, e.Version)
		m.currentID = ""
	case runner.ProgressInstalled:
		m.appendRow(e.ItemID, e.Name, GlyphInstalledByUs, StyleInstalled, e.Version+"  ("+e.Detail+")")
		m.currentID = ""
	case runner.ProgressWould:
		m.appendRow(e.ItemID, e.Name, GlyphNotInstalled, StyleNotInstalled, "would install via "+e.Detail)
		m.currentID = ""
	case runner.ProgressFailed:
		m.appendRow(e.ItemID, e.Name, "✗", ErrorStyle, e.ErrMsg)
		m.currentID = ""
	case runner.ProgressSkippedWhen:
		m.appendRow(e.ItemID, e.Name, GlyphSkipped, StyleSkipped, "When gate")
	case runner.ProgressNoProvider:
		m.appendRow(e.ItemID, e.Name, "?", StyleNotInstalled, "no registered provider")
	}

	var cmd tea.Cmd
	if m.total > 0 {
		pct := float64(len(m.rows)) / float64(m.total)
		cmd = m.progress.SetPercent(pct)
	}
	return m, cmd
}

func (m *InstallProgressModel) appendRow(id, name, marker string, style lipgloss.Style, detail string) {
	if name == "" {
		name = id
	}
	m.rows = append(m.rows, completedRow{
		id: id, name: name, marker: marker, style: style, detail: detail,
	})
}

// View implements tea.Model.
func (m InstallProgressModel) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Header.
	headline := fmt.Sprintf("Installing — %s", m.profileName)
	b.WriteString(TitleStyle.Render(headline))
	b.WriteString("\n")
	counter := fmt.Sprintf("%d / %d", len(m.rows), m.total)
	b.WriteString(DimStyle.Render(counter))
	b.WriteString("\n\n")

	// Progress bar.
	b.WriteString(m.progress.View())
	b.WriteString("\n\n")

	// Completed rows — most recent 15.
	visible := m.rows
	const maxVisible = 15
	if len(visible) > maxVisible {
		visible = visible[len(visible)-maxVisible:]
		b.WriteString(DimStyle.Render(fmt.Sprintf("  (showing last %d of %d)\n", maxVisible, len(m.rows))))
	}
	for _, r := range visible {
		marker := r.style.Render(r.marker)
		name := NormalStyle.Render(r.name)
		detail := DimStyle.Render(r.detail)
		b.WriteString(fmt.Sprintf("  %s %-22s %s\n", marker, name, detail))
	}

	// Current in-flight.
	if m.currentID != "" {
		spin := m.spinner.View()
		name := SelectedStyle.Render(firstNonEmptyString(m.currentName, m.currentID))
		b.WriteString(fmt.Sprintf("  %s %-22s %s\n", spin, name, DimStyle.Render("installing...")))
	}
	// Current bootstrap.
	if m.bootstrap != "" {
		spin := m.spinner.View()
		b.WriteString(fmt.Sprintf("  %s %-22s %s\n", spin,
			SelectedStyle.Render("repo "+m.bootstrap),
			DimStyle.Render("bootstrapping...")))
	}

	if m.done {
		b.WriteString("\n")
		b.WriteString(m.renderSummary())
		b.WriteString("\n")
		b.WriteString(HelpStyle.Render("press q to close"))
	} else {
		b.WriteString("\n")
		b.WriteString(HelpStyle.Render("press q to quit (only after install finishes)"))
	}
	return b.String()
}

func (m InstallProgressModel) renderSummary() string {
	if m.finalErr != nil {
		return ErrorStyle.Render("Run failed: " + m.finalErr.Error())
	}
	if m.finalSum == nil {
		return DimStyle.Render("(no summary)")
	}
	s := m.finalSum
	line := fmt.Sprintf("Done. Installed %d · already had %d · failed %d · skipped %d",
		len(s.Installed), len(s.AlreadyHad), len(s.Failed), len(s.Skipped))
	if len(s.Failed) > 0 {
		return ErrorStyle.Render(line)
	}
	return StyleInstalled.Render(line)
}

// Done reports whether the model has received the ProgressFinishedMsg yet.
func (m InstallProgressModel) Done() bool { return m.done }

// Summary returns the final Summary (nil if not done yet).
func (m InstallProgressModel) Summary() *runner.Summary { return m.finalSum }

// FinalErr returns the final error (nil if successful or not done).
func (m InstallProgressModel) FinalErr() error { return m.finalErr }

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
