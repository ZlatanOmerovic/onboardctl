package tui

import (
	"context"
	"errors"
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ZlatanOmerovic/onboardctl/internal/manifest"
	"github.com/ZlatanOmerovic/onboardctl/internal/runner"
)

// RunInstallProgress renders the live install TUI. The caller runs the
// actual install in a goroutine, publishing progress events (and a
// final ProgressFinishedMsg) via the returned program.
//
// The returned stopFn should be called to wait for the TUI to exit; it
// returns the model's final state.
func RunInstallProgress(
	profileName string,
	total int,
	out io.Writer,
) (*tea.Program, func() (InstallProgressModel, error)) {
	model := NewInstallProgressModel(profileName, total)
	opts := []tea.ProgramOption{tea.WithOutput(out)}
	prog := tea.NewProgram(model, opts...)
	wait := func() (InstallProgressModel, error) {
		final, err := prog.Run()
		if err != nil {
			return InstallProgressModel{}, err
		}
		pm, ok := final.(InstallProgressModel)
		if !ok {
			return InstallProgressModel{}, fmt.Errorf("tui: unexpected final model type %T", final)
		}
		return pm, nil
	}
	return prog, wait
}

// RunForm renders an input UI for one manifest item and returns the
// user's values. Dispatches by Input.Kind:
//
//   - form / text  → FormModel (multi-field / single-field text input)
//   - choice       → ChoiceModel (filterable list, backed by bubbles/list)
//   - bool         → not yet wired
//
// The caller passes the manifest item's Input unchanged; RunForm handles
// resolving Source commands and building the appropriate model.
func RunForm(
	ctx context.Context,
	itemID, itemName string,
	in *manifest.Input,
	out io.Writer, errOut io.Writer,
) (FormResult, error) {
	if in == nil {
		return FormResult{}, errors.New("tui: nil input")
	}
	switch in.Kind {
	case manifest.InputForm, manifest.InputText:
		return runFormModel(itemID, itemName, in, out)
	case manifest.InputChoice:
		choices, err := ResolveChoices(ctx, in)
		if err != nil {
			return FormResult{}, err
		}
		return runChoiceModel(itemID, itemName, in, choices, out)
	case manifest.InputBool:
		return runBoolModel(itemID, itemName, in, out)
	default:
		return FormResult{}, fmt.Errorf("tui: input kind %q not yet supported", in.Kind)
	}
	// _ = errOut   // reserved; keep in case we split stderr later
}

func runBoolModel(itemID, itemName string, in *manifest.Input, out io.Writer) (FormResult, error) {
	model := NewBoolModel(itemID, itemName, in)
	opts := []tea.ProgramOption{tea.WithOutput(out)}
	prog := tea.NewProgram(model, opts...)
	final, runErr := prog.Run()
	if runErr != nil {
		return FormResult{}, fmt.Errorf("tui: %w", runErr)
	}
	bm, ok := final.(BoolModel)
	if !ok {
		return FormResult{}, fmt.Errorf("tui: unexpected final model type %T", final)
	}
	return bm.Result(), nil
}

func runFormModel(itemID, itemName string, in *manifest.Input, out io.Writer) (FormResult, error) {
	model := NewFormModel(itemID, itemName, in)
	opts := []tea.ProgramOption{tea.WithOutput(out)}
	prog := tea.NewProgram(model, opts...)
	final, runErr := prog.Run()
	if runErr != nil {
		return FormResult{}, fmt.Errorf("tui: %w", runErr)
	}
	fm, ok := final.(FormModel)
	if !ok {
		return FormResult{}, fmt.Errorf("tui: unexpected final model type %T", final)
	}
	return fm.Result(), nil
}

func runChoiceModel(itemID, itemName string, in *manifest.Input, choices []string, out io.Writer) (FormResult, error) {
	model := NewChoiceModel(itemID, itemName, in, choices)
	opts := []tea.ProgramOption{tea.WithOutput(out), tea.WithAltScreen()}
	prog := tea.NewProgram(model, opts...)
	final, runErr := prog.Run()
	if runErr != nil {
		return FormResult{}, fmt.Errorf("tui: %w", runErr)
	}
	cm, ok := final.(ChoiceModel)
	if !ok {
		return FormResult{}, fmt.Errorf("tui: unexpected final model type %T", final)
	}
	return cm.Result(), nil
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
