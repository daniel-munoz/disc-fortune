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
