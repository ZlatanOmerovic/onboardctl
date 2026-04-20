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

// RunForm renders a single input form for one manifest item with an
// Input block. Used between the review screen and dispatch to collect
// user-supplied values for config items like git-identity.
func RunForm(
	ctx context.Context,
	itemID, itemName string,
	in *manifest.Input,
	out io.Writer, errOut io.Writer,
) (FormResult, error) {
	if in == nil {
		return FormResult{}, errors.New("tui: nil input")
	}
	if in.Kind != manifest.InputForm && in.Kind != manifest.InputText {
		return FormResult{}, fmt.Errorf("tui: unsupported input kind %q (only form/text wired today)", in.Kind)
	}
	model := NewFormModel(itemID, itemName, in)
	opts := []tea.ProgramOption{tea.WithOutput(out)}
	_ = errOut
	prog := tea.NewProgram(model, opts...)
	final, runErr := prog.Run()
	if runErr != nil {
		return FormResult{}, fmt.Errorf("tui: %w", runErr)
	}
	fm, ok := final.(FormModel)
	if !ok {
		return FormResult{}, fmt.Errorf("tui: unexpected final model type %T", final)
	}
	_ = ctx
	return fm.Result(), nil
}

// RunItemReview renders the per-item review screen for a profile's plan
// and returns the user's final selection.
func RunItemReview(
	ctx context.Context,
	profileName, profileID string,
	plan *runner.Plan,
	out io.Writer, errOut io.Writer,
) (ReviewChoice, error) {
	if plan == nil {
		return ReviewChoice{}, errors.New("tui: nil plan")
	}
	model := NewReviewModel(profileName, profileID, plan)
	opts := []tea.ProgramOption{tea.WithOutput(out)}
	_ = errOut
	prog := tea.NewProgram(model, opts...)
	final, runErr := prog.Run()
	if runErr != nil {
		return ReviewChoice{}, fmt.Errorf("tui: %w", runErr)
	}
	rm, ok := final.(ReviewModel)
	if !ok {
		return ReviewChoice{}, fmt.Errorf("tui: unexpected final model type %T", final)
	}
	_ = ctx
	return rm.Choice(), nil
}

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
