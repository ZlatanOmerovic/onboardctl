package tui

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	"github.com/ZlatanOmerovic/onboardctl/internal/runner"
	tea "github.com/charmbracelet/bubbletea"
)

// RunProfilePicker shows the profile picker and returns the user's choice.
// The caller supplies the loaded manifest plus a Resolver callback that
// lets the picker pre-compute item counts per profile (so the view renders
// "(N items)" without embedding resolver logic in the TUI package).
//
// out/err are where the tea program renders; pass os.Stdout / os.Stderr
// for normal use.
func RunProfilePicker(
	ctx context.Context,
	m *manifest.Manifest,
	out io.Writer, err io.Writer,
) (ProfileChoice, error) {
	if m == nil {
		return ProfileChoice{}, errors.New("tui: nil manifest")
	}

	counts := precomputeItemCounts(m)
	model := NewProfileModel(m, counts)

	opts := []tea.ProgramOption{tea.WithOutput(out)}
	_ = err // reserved for future stderr-bound diagnostics

	prog := tea.NewProgram(model, opts...)
	final, runErr := prog.Run()
	if runErr != nil {
		return ProfileChoice{}, fmt.Errorf("tui: %w", runErr)
	}

	pm, ok := final.(ProfileModel)
	if !ok {
		return ProfileChoice{}, fmt.Errorf("tui: unexpected final model type %T", final)
	}
	_ = ctx // reserved for cancellable flows in later iterations
	return pm.Choice(), nil
}

// precomputeItemCounts walks each profile through the same expansion the
// runner uses at install time, so the count the TUI shows matches reality
// (including 'extends' inheritance).
func precomputeItemCounts(m *manifest.Manifest) map[string]int {
	out := make(map[string]int, len(m.Profiles))
	for id := range m.Profiles {
		ids, err := runner.Resolve(m, runner.Selection{Profile: id})
		if err != nil {
			out[id] = 0
			continue
		}
		out[id] = len(ids)
	}
	return out
}
