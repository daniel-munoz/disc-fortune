package main

import (
	"testing"
	"time"
)

func TestComputeStatsCountsAndFavorites(t *testing.T) {
	a := Album{ReleaseID: 1, Artist: "Miles Davis", Title: "Kind of Blue", Year: 1959, Label: "Columbia", Genres: []string{"Jazz"}}
	b := Album{ReleaseID: 2, Artist: "Ride", Title: "Nowhere", Year: 1990, Label: "Creation", Genres: []string{"Shoegaze"}}
	c := Album{ReleaseID: 3, Artist: "Slowdive", Title: "Souvlaki", Year: 1993, Label: "Creation", Genres: []string{"Shoegaze", "Dream Pop"}}

	s := computeStats([]Album{a, b, c}, []Album{b}, nil, 10, Meta{}, false)

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
	pool := []Album{
		{Artist: "A", Title: "1", Genres: []string{"Shoegaze", "Dream Pop"}, Label: "Creation"},
		{Artist: "B", Title: "2", Genres: []string{"Shoegaze"}, Label: "Creation"},
		{Artist: "C", Title: "3", Genres: []string{"Jazz"}, Label: "Columbia"},
	}
	s := computeStats(pool, nil, nil, len(pool), Meta{}, false)

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
	pool := []Album{
		{Artist: "A", Title: "1", Genres: []string{"Zydeco"}},
		{Artist: "B", Title: "2", Genres: []string{"Ambient"}},
		{Artist: "C", Title: "3", Genres: []string{"Metal"}},
	}
	for i := 0; i < 20; i++ {
		s := computeStats(pool, nil, nil, len(pool), Meta{}, false)
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
	var pool []Album
	for _, g := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		pool = append(pool, Album{Artist: g, Title: g, Genres: []string{g}})
	}
	s := computeStats(pool, nil, nil, len(pool), Meta{}, false)
	if len(s.Genres) != topN {
		t.Errorf("Genres = %d rows, want %d", len(s.Genres), topN)
	}
}

// Decades run contiguously from the earliest to the latest present, so a
// decade with nothing in it shows as a zero row. Year 0 is a Decade 0 row,
// always last.
func TestComputeStatsDecadeBuckets(t *testing.T) {
	pool := []Album{
		{Artist: "A", Title: "1", Year: 1959},
		{Artist: "B", Title: "2", Year: 1979},
		{Artist: "C", Title: "3", Year: 1971},
		{Artist: "D", Title: "4"}, // no year
	}
	s := computeStats(pool, nil, nil, len(pool), Meta{}, false)

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
	s := computeStats([]Album{{Artist: "A", Title: "1", Year: 1971}}, nil, nil, 1, Meta{}, false)
	for _, b := range s.Decades {
		if b.Decade == 0 {
			t.Errorf("unexpected unknown row in %+v", s.Decades)
		}
	}
}

func TestComputeStatsPicked(t *testing.T) {
	a := Album{ReleaseID: 1, Artist: "Miles Davis", Title: "Kind of Blue"}
	b := Album{ReleaseID: 2, Artist: "Ride", Title: "Nowhere"}
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{{Album: a, Timestamp: older}, {Album: a, Timestamp: newer}}

	s := computeStats([]Album{a, b}, nil, entries, 2, Meta{}, false)

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
// unheardOnly already does and is why the share is conservative.
func TestComputeStatsUnIDdHistoryEntryCountsEveryPressing(t *testing.T) {
	one := Album{ReleaseID: 1, Artist: "Miles Davis", Title: "Kind of Blue"}
	two := Album{ReleaseID: 2, Artist: "Miles Davis", Title: "Kind of Blue"}
	legacy := Album{Artist: "Miles Davis", Title: "Kind of Blue"}
	entries := []HistoryEntry{{Album: legacy, Timestamp: time.Now()}}

	s := computeStats([]Album{one, two}, nil, entries, 2, Meta{}, false)
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
