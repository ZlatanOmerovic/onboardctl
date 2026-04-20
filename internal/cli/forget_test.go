package cli

import (
	"testing"
	"time"

	"github.com/ZlatanOmerovic/onboardctl/internal/state"
)

func TestCollectAllItemIDsSorted(t *testing.T) {
	st := state.New()
	ts := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	st.RecordInstall("vlc", "apt", "3.0", state.ByOnboardctl, ts)
	st.RecordInstall("jq", "apt", "1.7", state.ByOnboardctl, ts)
	st.RecordInstall("aardvark", "apt", "1.0", state.ByOnboardctl, ts)

	got := collectAllItemIDs(st)
	want := []string{"aardvark", "jq", "vlc"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i, id := range got {
		if id != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, id, want[i])
		}
	}
}

func TestCollectAllItemIDsEmpty(t *testing.T) {
	st := state.New()
	if ids := collectAllItemIDs(st); len(ids) != 0 {
		t.Errorf("expected empty, got %v", ids)
	}
}

func TestFirstNonBlank(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"", "", "x"}, "x"},
		{[]string{"first", "second"}, "first"},
		{[]string{"", ""}, ""},
		{nil, ""},
	}
	for _, c := range cases {
		if got := firstNonBlank(c.in...); got != c.want {
			t.Errorf("firstNonBlank(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
