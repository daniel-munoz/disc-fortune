package pick

import (
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"

	"github.com/daniel-munoz/disc-fortune/v2/internal/disc"
)

func TestParseDrawMode(t *testing.T) {
	cases := []struct {
		in   string
		want Mode
	}{
		{"fresh", Fresh},
		{"any", Any},
		{"stale", Stale},
	}
	for _, c := range cases {
		got, err := ParseMode(c.in)
		if err != nil {
			t.Errorf("ParseMode(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseMode(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseDrawModeRejectsUnknown(t *testing.T) {
	_, err := ParseMode("weighted")
	if err == nil {
		t.Fatal("expected an error for an unknown --draw value")
	}
	if !strings.Contains(err.Error(), "weighted") {
		t.Errorf("error %q does not name the offending value", err)
	}
}

// Fresh must be the zero value: a selection built without an explicit
// mode has to get the default, not an unfiltered draw.
func TestDrawFreshIsZeroValue(t *testing.T) {
	var m Mode
	if m != Fresh {
		t.Errorf("zero Mode = %v, want Fresh", m)
	}
}

// histOf builds a history whose entries are the given albums, oldest first.
// Timestamps are irrelevant to every function under test -- the window is
// counted in picks, not in time -- so they are left zero.
func histOf(albums ...disc.Album) []disc.HistoryEntry {
	entries := make([]disc.HistoryEntry, len(albums))
	for i, a := range albums {
		entries[i] = disc.HistoryEntry{Album: a}
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
	a := disc.Album{ReleaseID: 1, Artist: "Slowdive", Title: "Souvlaki"}
	b := disc.Album{ReleaseID: 2, Artist: "Ride", Title: "Nowhere"}
	entries := histOf(a, b, a)

	idx, played := LastPlayedIndex(entries, a)
	if !played {
		t.Fatal("played = false, want true")
	}
	if idx != 2 {
		t.Errorf("idx = %d, want 2 (the most recent play, not the first)", idx)
	}
}

func TestLastPlayedIndexNeverPlayed(t *testing.T) {
	entries := histOf(disc.Album{ReleaseID: 1, Artist: "Ride", Title: "Nowhere"})
	if _, played := LastPlayedIndex(entries, disc.Album{ReleaseID: 2, Artist: "Lush", Title: "Spooky"}); played {
		t.Error("played = true for an album that is not in history")
	}
}

// A history entry written before release IDs existed carries only a name, and
// disc.SameAlbum treats it as that name's wildcard. It must still match the
// ID-bearing album it refers to.
func TestLastPlayedIndexMatchesUnIDdEntry(t *testing.T) {
	stored := disc.Album{Artist: "Slowdive", Title: "Souvlaki"}
	synced := disc.Album{ReleaseID: 42, Artist: "Slowdive", Title: "Souvlaki"}
	if _, played := LastPlayedIndex(histOf(stored), synced); !played {
		t.Error("an un-ID'd history entry did not match its synced self")
	}
}

func TestRecentlyPlayedReturnsDistinctAlbums(t *testing.T) {
	a := disc.Album{ReleaseID: 1, Artist: "A", Title: "1"}
	b := disc.Album{ReleaseID: 2, Artist: "B", Title: "2"}
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
	got := recentlyPlayed(histOf(disc.Album{ReleaseID: 1, Artist: "A", Title: "1"}), 10)
	if len(got) != 1 {
		t.Errorf("len = %d, want 1", len(got))
	}
}

func TestRecentlyPlayedZeroWindow(t *testing.T) {
	if got := recentlyPlayed(histOf(disc.Album{ReleaseID: 1, Artist: "A", Title: "1"}), 0); len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestUnheardOnlyKeepsNeverPlayed(t *testing.T) {
	played := disc.Album{ReleaseID: 1, Artist: "A", Title: "1"}
	fresh := disc.Album{ReleaseID: 2, Artist: "B", Title: "2"}

	got := UnheardOnly([]disc.Album{played, fresh}, histOf(played))
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1; got %+v", len(got), got)
	}
	if got[0].ReleaseID != 2 {
		t.Errorf("kept release %d, want 2", got[0].ReleaseID)
	}
}

func TestUnheardOnlyEmptyHistoryKeepsEverything(t *testing.T) {
	pool := []disc.Album{{ReleaseID: 1, Artist: "A", Title: "1"}, {ReleaseID: 2, Artist: "B", Title: "2"}}
	if got := UnheardOnly(pool, nil); len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
}

// A history entry with no release ID does not say which pressing was played,
// so --unheard must not claim any of them is unheard.
func TestUnheardOnlyIsConservativeAboutUnIDdEntries(t *testing.T) {
	pool := []disc.Album{
		{ReleaseID: 1, Artist: "Slowdive", Title: "Souvlaki"},
		{ReleaseID: 2, Artist: "Slowdive", Title: "Souvlaki"},
	}
	entries := histOf(disc.Album{Artist: "Slowdive", Title: "Souvlaki"})

	if got := UnheardOnly(pool, entries); len(got) != 0 {
		t.Errorf("len = %d, want 0; an un-ID'd entry must hide every pressing of its title", len(got))
	}
}

// seededRNG returns a generator pinned to a fixed sequence, which is what
// makes picking assertable at all.
func seededRNG() *rand.Rand {
	return rand.New(rand.NewPCG(1, 2))
}

func poolOf(n int) []disc.Album {
	pool := make([]disc.Album, n)
	for i := range pool {
		pool[i] = disc.Album{ReleaseID: i + 1, Artist: "A", Title: strconv.Itoa(i + 1)}
	}
	return pool
}

func TestPickAlbumIsDeterministicUnderASeed(t *testing.T) {
	pool := poolOf(20)
	entries := histOf(pool[0], pool[1], pool[2])

	for _, mode := range []Mode{Any, Fresh, Stale} {
		first := Draw(pool, entries, mode, seededRNG())
		second := Draw(pool, entries, mode, seededRNG())
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
		got := Draw(pool, entries, Fresh, rng)
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
		seen[Draw(pool, entries, Any, rng).ReleaseID] = true
	}
	if len(seen) != 3 {
		t.Errorf("saw %d distinct albums, want 3; --draw any must not consult history", len(seen))
	}
}

// The guard that matters. antiRepeatWindow bounds excluded *names*, and one
// un-ID'd history entry is a wildcard matching every pressing of its title,
// so exclusion really can empty a pool.
func TestPickAlbumFallsBackWhenExclusionEmptiesThePool(t *testing.T) {
	pool := []disc.Album{
		{ReleaseID: 1, Artist: "Slowdive", Title: "Souvlaki"},
		{ReleaseID: 2, Artist: "Slowdive", Title: "Souvlaki"},
		{ReleaseID: 3, Artist: "Slowdive", Title: "Souvlaki"},
	}
	entries := histOf(disc.Album{Artist: "Slowdive", Title: "Souvlaki"})

	got := Draw(pool, entries, Fresh, seededRNG())
	if got.ReleaseID == 0 {
		t.Fatal("Draw returned the zero Album instead of falling back to the full pool")
	}
}

func TestPickAlbumSinglePool(t *testing.T) {
	pool := poolOf(1)
	got := Draw(pool, histOf(pool[0]), Fresh, seededRNG())
	if got.ReleaseID != 1 {
		t.Errorf("got release %d, want 1", got.ReleaseID)
	}
}

func TestStaleWeightsRankNeverPlayedHighest(t *testing.T) {
	old := disc.Album{ReleaseID: 1, Artist: "A", Title: "1"}
	recent := disc.Album{ReleaseID: 2, Artist: "B", Title: "2"}
	never := disc.Album{ReleaseID: 3, Artist: "C", Title: "3"}
	entries := histOf(old, recent)

	w := staleWeights([]disc.Album{old, recent, never}, entries)
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

// The uniform draw, which is what --draw any restores. This replaces the old
// TestRandomAlbum from collection_test.go.
func TestPickAlbumAnyReturnsValidAlbums(t *testing.T) {
	albums := []disc.Album{
		{Artist: "A", Title: "1"},
		{Artist: "B", Title: "2"},
		{Artist: "C", Title: "3"},
	}

	seen := make(map[string]bool)
	rng := seededRNG()
	for range 100 {
		seen[Draw(albums, nil, Any, rng).Key()] = true
	}
	if len(seen) < 2 {
		t.Errorf("expected multiple different albums over 100 picks, got %d unique", len(seen))
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

// The window is sized from the filtered pool, so it has to be filled from that
// same pool. A plain `pick` between two `pick --favorites` must not consume the
// window and let a just-played favorite straight back in.
func TestExcludeRecentFillsTheWindowFromThePoolNotGlobalHistory(t *testing.T) {
	pool := []disc.Album{
		{ReleaseID: 1, Artist: "Fav", Title: "One"},
		{ReleaseID: 2, Artist: "Fav", Title: "Two"},
		{ReleaseID: 3, Artist: "Fav", Title: "Three"},
	}
	outsider := disc.Album{ReleaseID: 99, Artist: "Other", Title: "X"}

	// Pool of 3 gives a window of 1. Release 1 is the most recent pick from
	// within the pool; the outsider is the most recent pick overall.
	kept := excludeRecent(pool, histOf(pool[0], outsider))

	for _, a := range kept {
		if a.ReleaseID == 1 {
			t.Fatalf("release 1 was the most recent pick within the pool but survived exclusion; kept = %+v", kept)
		}
	}
	if len(kept) != 2 {
		t.Errorf("len(kept) = %d, want 2", len(kept))
	}
}

// A history entry for a record that is no longer a candidate -- sold, or
// outside the current filter -- must not occupy a window slot either.
func TestExcludeRecentIgnoresHistoryOutsideThePool(t *testing.T) {
	pool := poolOf(9) // window 3
	gone := []disc.Album{
		{ReleaseID: 101, Artist: "Sold", Title: "A"},
		{ReleaseID: 102, Artist: "Sold", Title: "B"},
		{ReleaseID: 103, Artist: "Sold", Title: "C"},
	}
	// Three departed records are the most recent picks, then release 1.
	entries := histOf(pool[0], gone[0], gone[1], gone[2])

	kept := excludeRecent(pool, entries)
	for _, a := range kept {
		if a.ReleaseID == 1 {
			t.Fatalf("release 1 survived exclusion; departed records consumed the window. kept = %+v", kept)
		}
	}
}
