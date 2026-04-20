package runner

import "testing"

func TestSubstituteReplacesKnownTokens(t *testing.T) {
	got := substitute("git config --global user.name {name}", map[string]string{
		"name": "Zlatan",
	})
	want := "git config --global user.name Zlatan"
	if got != want {
		t.Errorf("substitute = %q, want %q", got, want)
	}
}

func TestSubstituteLeavesUnknownTokensAlone(t *testing.T) {
	got := substitute("hello {name}, your {thing}", map[string]string{"name": "world"})
	want := "hello world, your {thing}"
	if got != want {
		t.Errorf("substitute = %q, want %q", got, want)
	}
}

func TestSubstituteEmptyInputsPassThrough(t *testing.T) {
	if substitute("", map[string]string{"a": "b"}) != "" {
		t.Error("empty text should pass through")
	}
	if substitute("hi {x}", nil) != "hi {x}" {
		t.Error("nil values should leave text unchanged")
	}
}

func TestSubstituteAllDoesNotMutate(t *testing.T) {
	orig := []string{"{a}", "{b}"}
	snapshot := []string{"{a}", "{b}"}
	_ = substituteAll(orig, map[string]string{"a": "X", "b": "Y"})
	for i, v := range orig {
		if v != snapshot[i] {
			t.Errorf("input mutated: orig[%d] = %q, snapshot = %q", i, v, snapshot[i])
		}
	}
}
