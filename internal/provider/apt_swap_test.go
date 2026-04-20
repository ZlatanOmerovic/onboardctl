package provider

import (
	"context"
	"errors"
	"testing"
)

func TestRemoveSnapCounterpartHappyPath(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"snap remove firefox": {stdout: "firefox removed"},
	}}
	a := NewAPTWith(f)
	if err := a.RemoveSnapCounterpart(context.Background(), "firefox"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.calls) != 1 || f.calls[0] != "snap remove firefox" {
		t.Errorf("unexpected call sequence: %v", f.calls)
	}
}

func TestRemoveSnapCounterpartSurfacesError(t *testing.T) {
	f := &fakeRunner{responses: map[string]fakeResp{
		"snap remove bogus": {stdout: "not installed", err: errors.New("exit 1")},
	}}
	a := NewAPTWith(f)
	err := a.RemoveSnapCounterpart(context.Background(), "bogus")
	if err == nil {
		t.Fatal("expected error when snap remove fails")
	}
}

func TestRemoveSnapCounterpartRequiresPkg(t *testing.T) {
	a := NewAPTWith(&fakeRunner{})
	if err := a.RemoveSnapCounterpart(context.Background(), ""); err == nil {
		t.Fatal("expected error when pkg is empty")
	}
}
