package stats

import (
	"strings"
	"testing"
	"time"

	"github.com/daniel-munoz/disc-fortune/v2/internal/disc"
	"github.com/daniel-munoz/disc-fortune/v2/internal/term"
)

func TestComputeStatsCountsAndFavorites(t *testing.T) {
	a := disc.Album{ReleaseID: 1, Artist: "Miles Davis", Title: "Kind of Blue", Year: 1959, Label: "Columbia", Genres: []string{"Jazz"}}
	b := disc.Album{ReleaseID: 2, Artist: "Ride", Title: "Nowhere", Year: 1990, Label: "Creation", Genres: []string{"Shoegaze"}}
	c := disc.Album{ReleaseID: 3, Artist: "Slowdive", Title: "Souvlaki", Year: 1993, Label: "Creation", Genres: []string{"Shoegaze", "Dream Pop"}}

	s := Compute([]disc.Album{a, b, c}, []disc.Album{b}, nil, 10, disc.Meta{}, false)

	if s.Count != 3 {
		t.Errorf("Count = %d, want 3", s.Count)
	}
	if s.Total != 10 {
		t.Errorf("Total = %d, want 10", s.Total)
	}
	if s.Favorites != 1 {
		t.Errorf("Favorites = %d, want 1", s.Favorites)
	}
	if s.FavoritesOnly {
		t.Error("FavoritesOnly = true, want false")
	}
}

// An album listing several genres counts once in each; a label counts once
// per album.
func TestComputeStatsCountsGenresAndLabels(t *testing.T) {
	pool := []disc.Album{
		{Artist: "A", Title: "1", Genres: []string{"Shoegaze", "Dream Pop"}, Label: "Creation"},
		{Artist: "B", Title: "2", Genres: []string{"Shoegaze"}, Label: "Creation"},
		{Artist: "C", Title: "3", Genres: []string{"Jazz"}, Label: "Columbia"},
	}
	s := Compute(pool, nil, nil, len(pool), disc.Meta{}, false)

	want := []NameCount{{"Shoegaze", 2}, {"Dream Pop", 1}, {"Jazz", 1}}
	if len(s.Genres) != len(want) {
		t.Fatalf("Genres = %+v, want %+v", s.Genres, want)
	}
	for i := range want {
		if s.Genres[i] != want[i] {
			t.Errorf("Genres[%d] = %+v, want %+v", i, s.Genres[i], want[i])
		}
	}

	if len(s.Labels) != 2 || s.Labels[0] != (NameCount{"Creation", 2}) || s.Labels[1] != (NameCount{"Columbia", 1}) {
		t.Errorf("Labels = %+v, want [{Creation 2} {Columbia 1}]", s.Labels)
	}
}

// Equal counts sort by name ascending. Without that, map iteration order
// would make the output differ between runs.
func TestComputeStatsTiesSortByName(t *testing.T) {
	pool := []disc.Album{
		{Artist: "A", Title: "1", Genres: []string{"Zydeco"}},
		{Artist: "B", Title: "2", Genres: []string{"Ambient"}},
		{Artist: "C", Title: "3", Genres: []string{"Metal"}},
	}
	for i := 0; i < 20; i++ {
		s := Compute(pool, nil, nil, len(pool), disc.Meta{}, false)
		got := []string{s.Genres[0].Name, s.Genres[1].Name, s.Genres[2].Name}
		want := []string{"Ambient", "Metal", "Zydeco"}
		for j := range want {
			if got[j] != want[j] {
				t.Fatalf("run %d: Genres = %v, want %v", i, got, want)
			}
		}
	}
}

func TestComputeStatsTopNCapsAtFive(t *testing.T) {
	var pool []disc.Album
	for _, g := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		pool = append(pool, disc.Album{Artist: g, Title: g, Genres: []string{g}})
	}
	s := Compute(pool, nil, nil, len(pool), disc.Meta{}, false)
	if len(s.Genres) != topN {
		t.Errorf("Genres = %d rows, want %d", len(s.Genres), topN)
	}
}

// Decades run contiguously from the earliest to the latest present, so a
// decade with nothing in it shows as a zero row. Year 0 is a Decade 0 row,
// always last.
func TestComputeStatsDecadeBuckets(t *testing.T) {
	pool := []disc.Album{
		{Artist: "A", Title: "1", Year: 1959},
		{Artist: "B", Title: "2", Year: 1979},
		{Artist: "C", Title: "3", Year: 1971},
		{Artist: "D", Title: "4"}, // no year
	}
	s := Compute(pool, nil, nil, len(pool), disc.Meta{}, false)

	want := []DecadeBucket{{1950, 1}, {1960, 0}, {1970, 2}, {0, 1}}
	if len(s.Decades) != len(want) {
		t.Fatalf("Decades = %+v, want %+v", s.Decades, want)
	}
	for i := range want {
		if s.Decades[i] != want[i] {
			t.Errorf("Decades[%d] = %+v, want %+v", i, s.Decades[i], want[i])
		}
	}
}

func TestComputeStatsDecadesOmitsUnknownRowWhenEmpty(t *testing.T) {
	s := Compute([]disc.Album{{Artist: "A", Title: "1", Year: 1971}}, nil, nil, 1, disc.Meta{}, false)
	for _, b := range s.Decades {
		if b.Decade == 0 {
			t.Errorf("unexpected unknown row in %+v", s.Decades)
		}
	}
}

func TestComputeStatsPicked(t *testing.T) {
	a := disc.Album{ReleaseID: 1, Artist: "Miles Davis", Title: "Kind of Blue"}
	b := disc.Album{ReleaseID: 2, Artist: "Ride", Title: "Nowhere"}
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	entries := []disc.HistoryEntry{{Album: a, Timestamp: older}, {Album: a, Timestamp: newer}}

	s := Compute([]disc.Album{a, b}, nil, entries, 2, disc.Meta{}, false)

	if s.Picked.Count != 1 {
		t.Errorf("Picked.Count = %d, want 1", s.Picked.Count)
	}
	if !s.Picked.LastPicked.Equal(newer) {
		t.Errorf("Picked.LastPicked = %v, want %v", s.Picked.LastPicked, newer)
	}
	if got := s.Share(); got != 0.5 {
		t.Errorf("Share() = %v, want 0.5", got)
	}
}

// A history entry with no release ID is a name wildcard: it matches every
// pressing of its title, so all of them count as picked. This mirrors what
// pick.UnheardOnly already does and is why the share is conservative.
func TestComputeStatsUnIDdHistoryEntryCountsEveryPressing(t *testing.T) {
	one := disc.Album{ReleaseID: 1, Artist: "Miles Davis", Title: "Kind of Blue"}
	two := disc.Album{ReleaseID: 2, Artist: "Miles Davis", Title: "Kind of Blue"}
	legacy := disc.Album{Artist: "Miles Davis", Title: "Kind of Blue"}
	entries := []disc.HistoryEntry{{Album: legacy, Timestamp: time.Now()}}

	s := Compute([]disc.Album{one, two}, nil, entries, 2, disc.Meta{}, false)
	if s.Picked.Count != 2 {
		t.Errorf("Picked.Count = %d, want 2 -- an un-ID'd entry is a name wildcard", s.Picked.Count)
	}
}

func TestStatsShareOfEmptySetIsZero(t *testing.T) {
	var s Stats
	if got := s.Share(); got != 0 {
		t.Errorf("Share() = %v, want 0 for an empty set", got)
	}
}

// The golden test pins a Stats with no timestamps, so no relative-time line
// appears and the bytes are stable. The two relative lines are covered
// separately below.
func TestFormatStatsGolden(t *testing.T) {
	s := Stats{
		Count:     4,
		Total:     4,
		Favorites: 1,
		Decades: []DecadeBucket{
			{1950, 1}, {1960, 0}, {1970, 2}, {0, 1},
		},
		Genres: []NameCount{{"Shoegaze", 2}, {"Jazz", 1}},
		Labels: []NameCount{{"Creation", 2}, {"Columbia", 1}},
		Picked: PickedStats{Count: 1},
	}

	want := `4 albums · 1 favorite

Decades
  1950s    1  ████████████
  1960s    0
  1970s    2  ████████████████████████
  unknown  1  ████████████

Top genres
  Shoegaze  2
  Jazz      1

Top labels
  Creation  2
  Columbia  1

1 of 4 albums picked at least once (25%)
`

	if got := Format(s, false); got != want {
		t.Errorf("Format drifted.\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatStatsHeaderVariants(t *testing.T) {
	tests := []struct {
		name string
		s    Stats
		want string
	}{
		{"unfiltered", Stats{Count: 1247, Total: 1247, Favorites: 84}, "1247 albums · 84 favorites"},
		{"filtered", Stats{Count: 312, Total: 1247, Favorites: 28}, "312 of 1247 albums · 28 favorites"},
		{"favorites only", Stats{Count: 84, Total: 84, FavoritesOnly: true}, "84 favorites"},
		{"favorites filtered", Stats{Count: 12, Total: 84, FavoritesOnly: true}, "12 of 84 favorites"},
		{"singular album", Stats{Count: 1, Total: 1, Favorites: 1}, "1 album · 1 favorite"},
		{"singular favorite set", Stats{Count: 1, Total: 1, FavoritesOnly: true}, "1 favorite"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.SplitN(Format(tc.s, false), "\n", 2)[0]
			if got != tc.want {
				t.Errorf("header = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatStatsRelativeLines(t *testing.T) {
	s := Stats{
		Count:    2,
		Total:    2,
		SyncedAt: time.Now().Add(-3 * 24 * time.Hour),
		Picked:   PickedStats{Count: 1, LastPicked: time.Now().Add(-2 * time.Hour)},
	}
	out := Format(s, false)
	if !strings.Contains(out, "last synced 3 days ago") {
		t.Errorf("missing sync line:\n%s", out)
	}
	if !strings.Contains(out, "last picked 2 hours ago") {
		t.Errorf("missing last-picked line:\n%s", out)
	}
}

func TestFormatStatsOmitsAbsentLines(t *testing.T) {
	out := Format(Stats{Count: 1, Total: 1}, false)
	if strings.Contains(out, "last synced") {
		t.Errorf("sync line present with a zero SyncedAt:\n%s", out)
	}
	if strings.Contains(out, "last picked") {
		t.Errorf("last-picked line present with nothing picked:\n%s", out)
	}
	if strings.Contains(out, "Top genres") {
		t.Errorf("empty genre section rendered:\n%s", out)
	}
}

func TestFormatStatsColorsHeadingsAndBars(t *testing.T) {
	s := Stats{Count: 1, Total: 1, Decades: []DecadeBucket{{1970, 1}}}
	out := Format(s, true)
	if !strings.Contains(out, term.BoldWhite+"Decades"+term.Reset) {
		t.Errorf("heading not bold:\n%q", out)
	}
	if !strings.Contains(out, term.Dim) {
		t.Errorf("bar not dimmed:\n%q", out)
	}
}

// A bucket too small to earn a whole eighth still exists, and must not read
// as zero.
func TestFormatStatsTinyBucketStillDrawsABar(t *testing.T) {
	s := Stats{Count: 1000, Total: 1000, Decades: []DecadeBucket{{1970, 1000}, {1980, 1}}}
	lines := strings.Split(Format(s, false), "\n")
	var got string
	for _, l := range lines {
		if strings.Contains(l, "1980s") {
			got = l
		}
	}
	if !strings.Contains(got, "▏") {
		t.Errorf("tiny bucket drew no bar: %q", got)
	}
}

// No line may end in whitespace: a zero-count row has no bar, and padding it
// out to the bar column would put trailing spaces in every golden file.
func TestFormatStatsHasNoTrailingWhitespace(t *testing.T) {
	s := Stats{
		Count:   3,
		Total:   3,
		Decades: []DecadeBucket{{1950, 1}, {1960, 0}, {1970, 2}},
		Genres:  []NameCount{{"Jazz", 1}},
	}
	for i, line := range strings.Split(Format(s, false), "\n") {
		if line != strings.TrimRight(line, " \t") {
			t.Errorf("line %d has trailing whitespace: %q", i, line)
		}
	}
}
