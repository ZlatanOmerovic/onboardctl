package runner

import "strings"

// substitute replaces {placeholder} tokens in text with values from the
// supplied map. Tokens whose keys aren't in the map are left as-is, so
// callers can spot them in the final command and react.
//
// Case is preserved; unknown tokens don't panic or error. This is a
// deliberately simple string-replace — the manifest is user-authored so
// we want behaviour that matches their intuition, not a full template
// engine with branching or loops.
func substitute(text string, values map[string]string) string {
	if text == "" || len(values) == 0 {
		return text
	}
	out := text
	for k, v := range values {
		out = strings.ReplaceAll(out, "{"+k+"}", v)
	}
	return out
}

// substituteAll applies substitute to each string in a slice, returning
// a new slice (never mutates the input).
func substituteAll(cmds []string, values map[string]string) []string {
	if len(cmds) == 0 || len(values) == 0 {
		return cmds
	}
	out := make([]string, len(cmds))
	for i, c := range cmds {
		out[i] = substitute(c, values)
	}
	return out
}
