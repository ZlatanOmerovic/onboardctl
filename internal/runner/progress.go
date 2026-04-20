package runner

// ProgressKind classifies a ProgressEvent so the UI can pick an icon/style.
type ProgressKind int

const (
	// ProgressStart is emitted right before an item's Check (and Install)
	// so the UI can show a spinner against the current item.
	ProgressStart ProgressKind = iota
	// ProgressAlready — Check reported Installed=true; no work done.
	ProgressAlready
	// ProgressInstalled — Install completed successfully (apply mode only).
	ProgressInstalled
	// ProgressWould — dry-run path: would install.
	ProgressWould
	// ProgressFailed — Install returned an error.
	ProgressFailed
	// ProgressSkippedWhen — item's When gate excluded it for this machine.
	ProgressSkippedWhen
	// ProgressNoProvider — no registered provider for any of the item's kinds.
	ProgressNoProvider
	// ProgressBootstrapStart — a named apt repo is about to be materialised.
	ProgressBootstrapStart
	// ProgressBootstrapDone — all repos materialised; apt-get update ran.
	ProgressBootstrapDone
)

// ProgressEvent is what the Runner emits to a caller-supplied ProgressFn.
// All fields are optional except Kind; consumers should tolerate missing
// detail so we can evolve the payload without breaking them.
type ProgressEvent struct {
	Kind     ProgressKind
	ItemID   string
	Name     string
	Version  string
	ErrMsg   string
	Detail   string // used by Bootstrap* events (repo name)
	Total    int    // total items in the run (set on every event)
	Index    int    // 1-based index of this item in the run (set on item events)
}

// ProgressFn is the Runner.ProgressFn callback type. It is invoked
// synchronously from the runner goroutine, so implementations must not
// block on anything that depends on the runner making progress.
//
// The canonical consumer is a Bubble Tea program wrapping the runner:
// it calls p.Send(event) inside the callback, which the tea.Program
// forwards to the model's Update loop on its own event goroutine.
type ProgressFn func(ProgressEvent)

// emit is a nil-safe wrapper used by runner internals.
func (r *Runner) emit(e ProgressEvent) {
	if r.ProgressFn != nil {
		r.ProgressFn(e)
	}
}
