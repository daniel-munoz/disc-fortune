package main

import (
	"strings"
	"testing"
)

func TestParseDrawMode(t *testing.T) {
	cases := []struct {
		in   string
		want drawMode
	}{
		{"fresh", drawFresh},
		{"any", drawAny},
		{"stale", drawStale},
	}
	for _, c := range cases {
		got, err := parseDrawMode(c.in)
		if err != nil {
			t.Errorf("parseDrawMode(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseDrawMode(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseDrawModeRejectsUnknown(t *testing.T) {
	_, err := parseDrawMode("weighted")
	if err == nil {
		t.Fatal("expected an error for an unknown --draw value")
	}
	if !strings.Contains(err.Error(), "weighted") {
		t.Errorf("error %q does not name the offending value", err)
	}
}

// drawFresh must be the zero value: a selection built without an explicit
// mode has to get the default, not an unfiltered draw.
func TestDrawFreshIsZeroValue(t *testing.T) {
	var m drawMode
	if m != drawFresh {
		t.Errorf("zero drawMode = %v, want drawFresh", m)
	}
}
