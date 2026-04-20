package cli

import "testing"

func TestAnyTrue(t *testing.T) {
	state := map[string]bool{
		"kitty":     true,
		"alacritty": false,
		"zsh":       false,
		"fish":      false,
		"starship":  true,
	}
	cases := []struct {
		keys []string
		want bool
	}{
		{[]string{"kitty", "alacritty"}, true},      // kitty is true
		{[]string{"alacritty"}, false},              // lone false
		{[]string{"zsh", "fish"}, false},            // both false
		{[]string{"starship"}, true},                // single true
		{[]string{"missing-key"}, false},            // absent treated as false
		{nil, false},                                // empty keys
		{[]string{"zsh", "fish", "starship"}, true}, // last true wins
	}
	for _, c := range cases {
		if got := anyTrue(state, c.keys...); got != c.want {
			t.Errorf("anyTrue(%v) = %v, want %v", c.keys, got, c.want)
		}
	}
}

func TestMarkerFor(t *testing.T) {
	if markerFor(true) != "✓" {
		t.Error("markerFor(true) should be ✓")
	}
	if markerFor(false) != "" {
		t.Error("markerFor(false) should be empty")
	}
}
