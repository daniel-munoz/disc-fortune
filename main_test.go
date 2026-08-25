package main

import (
	"strings"
	"testing"
)

func TestRunListOutput(t *testing.T) {
	albums := []Album{
		{Artist: "Slowdive", Title: "Souvlaki", Year: 1993, Label: "Creation Records", Genres: []string{"Shoegaze"}},
		{Artist: "Ride", Title: "Nowhere", Year: 1990, Label: "Creation Records", Genres: []string{"Shoegaze"}},
	}
	out := formatList(albums, false)
	if !strings.Contains(out, "Slowdive") {
		t.Errorf("output missing Slowdive: %q", out)
	}
	if !strings.Contains(out, "Ride") {
		t.Errorf("output missing Ride: %q", out)
	}
	if !strings.Contains(out, "2 albums") {
		t.Errorf("output missing count summary: %q", out)
	}
}

func TestRunListEmpty(t *testing.T) {
	out := formatList([]Album{}, false)
	if !strings.Contains(out, "No albums") {
		t.Errorf("expected empty message, got: %q", out)
	}
}

func TestRunListSeparator(t *testing.T) {
	albums := []Album{
		{Artist: "A", Title: "X"},
		{Artist: "B", Title: "Y"},
	}
	out := formatList(albums, false)
	// There should be a blank line between the two entries
	if !strings.Contains(out, "\n\n") {
		t.Errorf("expected blank line separator between entries: %q", out)
	}
}

func TestRunListSingular(t *testing.T) {
	out := formatList([]Album{{Artist: "A", Title: "X"}}, false)
	if !strings.Contains(out, "1 album") {
		t.Errorf("expected singular 'album', got: %q", out)
	}
	if strings.Contains(out, "1 albums") {
		t.Errorf("unexpected plural '1 albums': %q", out)
	}
}
