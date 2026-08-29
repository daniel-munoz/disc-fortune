package main

import (
	"testing"
)

func TestFilterByYear(t *testing.T) {
	albums := []Album{
		{Artist: "A", Title: "1", Year: 1970},
		{Artist: "B", Title: "2", Year: 1975},
		{Artist: "C", Title: "3", Year: 1980},
		{Artist: "D", Title: "4", Year: 0}, // no year
	}

	tests := []struct {
		yearFilter string
		want       int
	}{
		{"1975", 1},
		{"1970-1975", 2},
		{"1975-1970", 2}, // auto-swap
		{"", 4},          // no filter
	}

	for _, tt := range tests {
		t.Run(tt.yearFilter, func(t *testing.T) {
			f := Filter{Year: tt.yearFilter}
			filtered := f.Apply(albums)
			if len(filtered) != tt.want {
				t.Errorf("got %d albums, want %d", len(filtered), tt.want)
			}
		})
	}
}

func TestFilterByGenre(t *testing.T) {
	albums := []Album{
		{Artist: "A", Title: "1", Genres: []string{"Jazz", "Bebop"}},
		{Artist: "B", Title: "2", Genres: []string{"Rock"}},
		{Artist: "C", Title: "3", Genres: []string{}},
	}

	f := Filter{Genre: "jazz"}
	filtered := f.Apply(albums)
	if len(filtered) != 1 {
		t.Errorf("got %d albums, want 1", len(filtered))
	}
	if filtered[0].Artist != "A" {
		t.Errorf("Artist = %q, want A", filtered[0].Artist)
	}
}

func TestFilterCombined(t *testing.T) {
	albums := []Album{
		{Artist: "A", Title: "1", Year: 1970, Genres: []string{"Jazz"}},
		{Artist: "B", Title: "2", Year: 1970, Genres: []string{"Rock"}},
		{Artist: "C", Title: "3", Year: 1980, Genres: []string{"Jazz"}},
	}

	f := Filter{Year: "1970", Genre: "jazz"}
	filtered := f.Apply(albums)
	if len(filtered) != 1 {
		t.Errorf("got %d albums, want 1", len(filtered))
	}
	if filtered[0].Artist != "A" {
		t.Errorf("Artist = %q, want A", filtered[0].Artist)
	}
}

func TestFilterByQueryArtist(t *testing.T) {
	albums := []Album{
		{Artist: "Miles Davis", Title: "Kind of Blue"},
		{Artist: "John Coltrane", Title: "Giant Steps"},
		{Artist: "Bill Evans", Title: "Sunday at the Village Vanguard"},
	}

	f := Filter{Query: "miles"}
	filtered := f.Apply(albums)
	if len(filtered) != 1 {
		t.Fatalf("got %d albums, want 1", len(filtered))
	}
	if filtered[0].Artist != "Miles Davis" {
		t.Errorf("Artist = %q, want Miles Davis", filtered[0].Artist)
	}
}

func TestFilterByQueryTitle(t *testing.T) {
	albums := []Album{
		{Artist: "Miles Davis", Title: "Kind of Blue"},
		{Artist: "John Coltrane", Title: "Giant Steps"},
	}

	f := Filter{Query: "giant"}
	filtered := f.Apply(albums)
	if len(filtered) != 1 {
		t.Fatalf("got %d albums, want 1", len(filtered))
	}
	if filtered[0].Title != "Giant Steps" {
		t.Errorf("Title = %q, want Giant Steps", filtered[0].Title)
	}
}

func TestFilterByQueryCaseInsensitive(t *testing.T) {
	albums := []Album{
		{Artist: "Miles Davis", Title: "Kind of Blue"},
	}

	f := Filter{Query: "MILES"}
	filtered := f.Apply(albums)
	if len(filtered) != 1 {
		t.Errorf("got %d albums, want 1 (case-insensitive)", len(filtered))
	}
}

func TestFilterByQueryEmptyIsNoOp(t *testing.T) {
	albums := []Album{
		{Artist: "A", Title: "1"},
		{Artist: "B", Title: "2"},
	}

	f := Filter{Query: ""}
	filtered := f.Apply(albums)
	if len(filtered) != 2 {
		t.Errorf("got %d albums, want 2 (empty query should not filter)", len(filtered))
	}
}

func TestFilterByQueryNoMatch(t *testing.T) {
	albums := []Album{
		{Artist: "Miles Davis", Title: "Kind of Blue"},
	}

	f := Filter{Query: "nonexistent"}
	filtered := f.Apply(albums)
	if len(filtered) != 0 {
		t.Errorf("got %d albums, want 0", len(filtered))
	}
}

func TestFilterByQueryMultipleMatches(t *testing.T) {
	albums := []Album{
		{Artist: "Miles Davis", Title: "Kind of Blue"},
		{Artist: "Miles Davis", Title: "Bitches Brew"},
		{Artist: "John Coltrane", Title: "Giant Steps"},
	}

	f := Filter{Query: "miles"}
	filtered := f.Apply(albums)
	if len(filtered) != 2 {
		t.Errorf("got %d albums, want 2", len(filtered))
	}
}

func TestFilterQueryComposesWithYear(t *testing.T) {
	albums := []Album{
		{Artist: "Miles Davis", Title: "Kind of Blue", Year: 1959},
		{Artist: "Miles Davis", Title: "Bitches Brew", Year: 1970},
	}

	f := Filter{Query: "miles", Year: "1959"}
	filtered := f.Apply(albums)
	if len(filtered) != 1 {
		t.Fatalf("got %d albums, want 1", len(filtered))
	}
	if filtered[0].Title != "Kind of Blue" {
		t.Errorf("Title = %q, want Kind of Blue", filtered[0].Title)
	}
}

// TestFilterQueryStillMatchesWithReleaseID is the guard for the reason
// Key() was not turned into the identity: --query substring-matches against
// it, so an ID-preferring Key() would break every query silently.
func TestFilterQueryStillMatchesWithReleaseID(t *testing.T) {
	albums := []Album{
		{ReleaseID: 111, Artist: "Miles Davis", Title: "Kind of Blue"},
		{ReleaseID: 222, Artist: "Slowdive", Title: "Souvlaki"},
	}

	got := Filter{Query: "miles"}.Apply(albums)
	if len(got) != 1 || got[0].ReleaseID != 111 {
		t.Fatalf("Apply() = %+v, want the Miles Davis release", got)
	}

	got = Filter{Query: "souvlaki"}.Apply(albums)
	if len(got) != 1 || got[0].ReleaseID != 222 {
		t.Fatalf("Apply() = %+v, want the Slowdive release", got)
	}
}
