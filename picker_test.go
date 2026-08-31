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

// histOf builds a history whose entries are the given albums, oldest first.
// Timestamps are irrelevant to every function under test -- the window is
// counted in picks, not in time -- so they are left zero.
func histOf(albums ...Album) []HistoryEntry {
	entries := make([]HistoryEntry, len(albums))
	for i, a := range albums {
		entries[i] = HistoryEntry{Album: a}
	}
	return entries
}

func TestAntiRepeatWindowScalesToPool(t *testing.T) {
	cases := []struct{ pool, want int }{
		{0, 0},
		{1, 0},
		{2, 0},
		{3, 1},
		{9, 3},
		{30, 10},
		{100, 10},
	}
	for _, c := range cases {
		if got := antiRepeatWindow(c.pool); got != c.want {
			t.Errorf("antiRepeatWindow(%d) = %d, want %d", c.pool, got, c.want)
		}
	}
}

func TestLastPlayedIndexFindsMostRecent(t *testing.T) {
	a := Album{ReleaseID: 1, Artist: "Slowdive", Title: "Souvlaki"}
	b := Album{ReleaseID: 2, Artist: "Ride", Title: "Nowhere"}
	entries := histOf(a, b, a)

	idx, played := lastPlayedIndex(entries, a)
	if !played {
		t.Fatal("played = false, want true")
	}
	if idx != 2 {
		t.Errorf("idx = %d, want 2 (the most recent play, not the first)", idx)
	}
}

func TestLastPlayedIndexNeverPlayed(t *testing.T) {
	entries := histOf(Album{ReleaseID: 1, Artist: "Ride", Title: "Nowhere"})
	if _, played := lastPlayedIndex(entries, Album{ReleaseID: 2, Artist: "Lush", Title: "Spooky"}); played {
		t.Error("played = true for an album that is not in history")
	}
}

// A history entry written before release IDs existed carries only a name, and
// sameAlbum treats it as that name's wildcard. It must still match the
// ID-bearing album it refers to.
func TestLastPlayedIndexMatchesUnIDdEntry(t *testing.T) {
	stored := Album{Artist: "Slowdive", Title: "Souvlaki"}
	synced := Album{ReleaseID: 42, Artist: "Slowdive", Title: "Souvlaki"}
	if _, played := lastPlayedIndex(histOf(stored), synced); !played {
		t.Error("an un-ID'd history entry did not match its synced self")
	}
}

func TestRecentlyPlayedReturnsDistinctAlbums(t *testing.T) {
	a := Album{ReleaseID: 1, Artist: "A", Title: "1"}
	b := Album{ReleaseID: 2, Artist: "B", Title: "2"}
	// a played three times in a row must not consume the whole window.
	got := recentlyPlayed(histOf(b, a, a, a), 2)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2; got %+v", len(got), got)
	}
	if got[0].ReleaseID != 1 || got[1].ReleaseID != 2 {
		t.Errorf("got %+v, want a then b (most recent first)", got)
	}
}

func TestRecentlyPlayedShorterThanWindow(t *testing.T) {
	got := recentlyPlayed(histOf(Album{ReleaseID: 1, Artist: "A", Title: "1"}), 10)
	if len(got) != 1 {
		t.Errorf("len = %d, want 1", len(got))
	}
}

func TestRecentlyPlayedZeroWindow(t *testing.T) {
	if got := recentlyPlayed(histOf(Album{ReleaseID: 1, Artist: "A", Title: "1"}), 0); len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}
