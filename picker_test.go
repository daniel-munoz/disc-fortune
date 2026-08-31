package main

import (
	"math/rand/v2"
	"strconv"
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

func TestUnheardOnlyKeepsNeverPlayed(t *testing.T) {
	played := Album{ReleaseID: 1, Artist: "A", Title: "1"}
	fresh := Album{ReleaseID: 2, Artist: "B", Title: "2"}

	got := unheardOnly([]Album{played, fresh}, histOf(played))
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1; got %+v", len(got), got)
	}
	if got[0].ReleaseID != 2 {
		t.Errorf("kept release %d, want 2", got[0].ReleaseID)
	}
}

func TestUnheardOnlyEmptyHistoryKeepsEverything(t *testing.T) {
	pool := []Album{{ReleaseID: 1, Artist: "A", Title: "1"}, {ReleaseID: 2, Artist: "B", Title: "2"}}
	if got := unheardOnly(pool, nil); len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
}

// A history entry with no release ID does not say which pressing was played,
// so --unheard must not claim any of them is unheard.
func TestUnheardOnlyIsConservativeAboutUnIDdEntries(t *testing.T) {
	pool := []Album{
		{ReleaseID: 1, Artist: "Slowdive", Title: "Souvlaki"},
		{ReleaseID: 2, Artist: "Slowdive", Title: "Souvlaki"},
	}
	entries := histOf(Album{Artist: "Slowdive", Title: "Souvlaki"})

	if got := unheardOnly(pool, entries); len(got) != 0 {
		t.Errorf("len = %d, want 0; an un-ID'd entry must hide every pressing of its title", len(got))
	}
}

// seededRNG returns a generator pinned to a fixed sequence, which is what
// makes picking assertable at all.
func seededRNG() *rand.Rand {
	return rand.New(rand.NewPCG(1, 2))
}

func poolOf(n int) []Album {
	pool := make([]Album, n)
	for i := range pool {
		pool[i] = Album{ReleaseID: i + 1, Artist: "A", Title: strconv.Itoa(i + 1)}
	}
	return pool
}

func TestPickAlbumIsDeterministicUnderASeed(t *testing.T) {
	pool := poolOf(20)
	entries := histOf(pool[0], pool[1], pool[2])

	for _, mode := range []drawMode{drawAny, drawFresh, drawStale} {
		first := pickAlbum(pool, entries, mode, seededRNG())
		second := pickAlbum(pool, entries, mode, seededRNG())
		if first.ReleaseID != second.ReleaseID {
			t.Errorf("mode %v: got %d then %d from the same seed", mode, first.ReleaseID, second.ReleaseID)
		}
	}
}

func TestPickAlbumFreshExcludesRecent(t *testing.T) {
	pool := poolOf(9) // window = 3
	// The three most recent picks are releases 1, 2 and 3.
	entries := histOf(pool[2], pool[1], pool[0])

	// One generator for the whole loop: re-seeding inside it would draw the
	// same value 200 times and assert nothing the first draw did not.
	rng := seededRNG()
	for range 200 {
		got := pickAlbum(pool, entries, drawFresh, rng)
		if got.ReleaseID <= 3 {
			t.Fatalf("fresh returned release %d, which is inside the anti-repeat window", got.ReleaseID)
		}
	}
}

func TestPickAlbumAnyIgnoresHistory(t *testing.T) {
	pool := poolOf(3)
	entries := histOf(pool[0], pool[1], pool[2])

	seen := make(map[int]bool)
	rng := seededRNG()
	for range 200 {
		seen[pickAlbum(pool, entries, drawAny, rng).ReleaseID] = true
	}
	if len(seen) != 3 {
		t.Errorf("saw %d distinct albums, want 3; --draw any must not consult history", len(seen))
	}
}

// The guard that matters. antiRepeatWindow bounds excluded *names*, and one
// un-ID'd history entry is a wildcard matching every pressing of its title,
// so exclusion really can empty a pool.
func TestPickAlbumFallsBackWhenExclusionEmptiesThePool(t *testing.T) {
	pool := []Album{
		{ReleaseID: 1, Artist: "Slowdive", Title: "Souvlaki"},
		{ReleaseID: 2, Artist: "Slowdive", Title: "Souvlaki"},
		{ReleaseID: 3, Artist: "Slowdive", Title: "Souvlaki"},
	}
	entries := histOf(Album{Artist: "Slowdive", Title: "Souvlaki"})

	got := pickAlbum(pool, entries, drawFresh, seededRNG())
	if got.ReleaseID == 0 {
		t.Fatal("pickAlbum returned the zero Album instead of falling back to the full pool")
	}
}

func TestPickAlbumSinglePool(t *testing.T) {
	pool := poolOf(1)
	got := pickAlbum(pool, histOf(pool[0]), drawFresh, seededRNG())
	if got.ReleaseID != 1 {
		t.Errorf("got release %d, want 1", got.ReleaseID)
	}
}

func TestStaleWeightsRankNeverPlayedHighest(t *testing.T) {
	old := Album{ReleaseID: 1, Artist: "A", Title: "1"}
	recent := Album{ReleaseID: 2, Artist: "B", Title: "2"}
	never := Album{ReleaseID: 3, Artist: "C", Title: "3"}
	entries := histOf(old, recent)

	w := staleWeights([]Album{old, recent, never}, entries)
	if !(w[2] > w[0] && w[0] > w[1]) {
		t.Errorf("weights = %v, want never-played > long-unplayed > recent", w)
	}
	for i, x := range w {
		if x < 1 {
			t.Errorf("weights[%d] = %d, want at least 1 so nothing is unreachable", i, x)
		}
	}
}

func TestStaleWeightsEmptyHistoryIsUniform(t *testing.T) {
	w := staleWeights(poolOf(3), nil)
	if w[0] != w[1] || w[1] != w[2] {
		t.Errorf("weights = %v, want all equal for an empty history", w)
	}
}

func TestWeightedIndexRespectsWeights(t *testing.T) {
	counts := make([]int, 2)
	rng := seededRNG()
	for range 2000 {
		counts[weightedIndex([]int{1, 9}, rng)]++
	}
	if counts[1] <= counts[0]*3 {
		t.Errorf("counts = %v, want index 1 drawn far more often", counts)
	}
}
