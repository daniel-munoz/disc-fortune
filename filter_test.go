package main

import (
	"strings"
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

// TestFilterReleaseIDMatchesExactly: the ID is an identity, not a search
// term, so it matches whole values rather than substrings -- 183 must not
// match 1839278.
func TestFilterReleaseIDMatchesExactly(t *testing.T) {
	albums := []Album{
		{ReleaseID: 1839278, Artist: "Slowdive", Title: "Souvlaki"},
		{ReleaseID: 183, Artist: "Ride", Title: "Nowhere"},
	}

	got := Filter{ReleaseID: 1839278}.Apply(albums)
	if len(got) != 1 || got[0].ReleaseID != 1839278 {
		t.Fatalf("Apply() = %+v, want only release 1839278", got)
	}

	if got := (Filter{ReleaseID: 999}).Apply(albums); len(got) != 0 {
		t.Errorf("Apply() = %+v, want no matches for an unknown ID", got)
	}
}

// TestFilterReleaseIDZeroIsUnset: zero means "not filtering", not "match the
// entries that have no ID".
func TestFilterReleaseIDZeroIsUnset(t *testing.T) {
	albums := []Album{
		{ReleaseID: 111, Artist: "Slowdive", Title: "Souvlaki"},
		{Artist: "Ride", Title: "Nowhere"},
	}

	if got := (Filter{}).Apply(albums); len(got) != 2 {
		t.Errorf("Apply() = %+v, want both albums", got)
	}
}

// include and exclude keep the filter literals in these tests readable.
// Production code never builds a FieldFilter by hand -- addFilterFlags does
// it from the flag values.
func include(vals ...string) FieldFilter { return FieldFilter{Include: vals} }
func exclude(vals ...string) FieldFilter { return FieldFilter{Exclude: vals} }

func TestFieldFilterMatches(t *testing.T) {
	tests := []struct {
		name   string
		ff     FieldFilter
		values []string
		want   bool
	}{
		{"unconstrained matches anything", FieldFilter{}, []string{"Jazz"}, true},
		{"include hit", include("jazz"), []string{"Jazz"}, true},
		{"include miss", include("funk"), []string{"Jazz"}, false},
		{"include is OR", include("funk", "jazz"), []string{"Jazz"}, true},
		{"include hits any album value", include("bebop"), []string{"Jazz", "Bebop"}, true},
		{"include is a substring, not a whole value", include("azz"), []string{"Jazz"}, true},
		{"include is case-insensitive", include("JAZZ"), []string{"jazz"}, true},
		{"exclude hit", exclude("rock"), []string{"Rock"}, false},
		{"exclude miss", exclude("rock"), []string{"Jazz"}, true},
		{"exclude hits any album value", exclude("rock"), []string{"Rock", "Jazz"}, false},
		{"exclude is OR", exclude("rock", "pop"), []string{"Pop"}, false},
		{"exclusion beats inclusion", FieldFilter{Include: []string{"jazz"}, Exclude: []string{"jazz"}}, []string{"Jazz"}, false},
		{"an empty album value is excluded by nothing", exclude("blue note"), []string{""}, true},
		{"no album values are excluded by nothing", exclude("rock"), nil, true},
		{"no album values satisfy no inclusion", include("jazz"), nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ff.matches(tt.values); got != tt.want {
				t.Errorf("matches(%q) = %v, want %v", tt.values, got, tt.want)
			}
		})
	}
}

// years builds an inclusive YearFilter from --year spellings, so the tests
// below read the way the command line does.
func years(t *testing.T, vals ...string) YearFilter {
	t.Helper()
	var yf YearFilter
	for _, v := range vals {
		r, err := parseYearValue(v)
		if err != nil {
			t.Fatalf("parseYearValue(%q): %v", v, err)
		}
		yf.Include = append(yf.Include, r)
	}
	return yf
}

func TestParseYearValue(t *testing.T) {
	tests := []struct {
		in      string
		want    yearRange
		wantErr bool
	}{
		{in: "1975", want: yearRange{1975, 1975}},
		{in: " 1975 ", want: yearRange{1975, 1975}},
		{in: "1970-1980", want: yearRange{1970, 1980}},
		{in: "1980-1970", want: yearRange{1970, 1980}}, // auto-swap, as in v2.3.0
		{in: "1970 - 1980", want: yearRange{1970, 1980}},
		{in: "nineteen", wantErr: true},
		{in: "1970-", wantErr: true},
		{in: "1970-1980-1990", wantErr: true},
		{in: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseYearValue(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseYearValue(%q) = %v, want an error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseYearValue(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("parseYearValue(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestYearFilterMatches(t *testing.T) {
	tests := []struct {
		name string
		yf   YearFilter
		year int
		want bool
	}{
		{"unconstrained", YearFilter{}, 1975, true},
		{"include single hit", YearFilter{Include: []yearRange{{1975, 1975}}}, 1975, true},
		{"include single miss", YearFilter{Include: []yearRange{{1975, 1975}}}, 1976, false},
		{"include range hit", YearFilter{Include: []yearRange{{1970, 1980}}}, 1975, true},
		{"include is OR", YearFilter{Include: []yearRange{{1959, 1959}, {1970, 1979}}}, 1975, true},
		{"exclude hit", YearFilter{Exclude: []yearRange{{1970, 1979}}}, 1975, false},
		{"exclude miss", YearFilter{Exclude: []yearRange{{1970, 1979}}}, 1985, true},
		{"exclusion beats inclusion", YearFilter{Include: []yearRange{{1970, 1980}}, Exclude: []yearRange{{1975, 1975}}}, 1975, false},
		{"unknown year fails an inclusion", YearFilter{Include: []yearRange{{1970, 1980}}}, 0, false},
		{"unknown year survives an exclusion", YearFilter{Exclude: []yearRange{{1970, 1980}}}, 0, true},
		{"unknown year with no year filter", YearFilter{}, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.yf.matches(tt.year); got != tt.want {
				t.Errorf("matches(%d) = %v, want %v", tt.year, got, tt.want)
			}
		})
	}
}

func TestParseDecadeValue(t *testing.T) {
	tests := []struct {
		in   string
		want yearRange
	}{
		{"70s", yearRange{1970, 1979}},
		{"1970s", yearRange{1970, 1979}},
		{"1970", yearRange{1970, 1979}},
		{"2020s", yearRange{2020, 2029}},
		{"30s", yearRange{1930, 1939}},
		{"90s", yearRange{1990, 1999}},
		{"70S", yearRange{1970, 1979}}, // case-insensitive
		{" 70s ", yearRange{1970, 1979}},
		{"1975", yearRange{1970, 1979}}, // any year names its decade
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseDecadeValue(tt.in)
			if err != nil {
				t.Fatalf("parseDecadeValue(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("parseDecadeValue(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// The three two-digit decades that could mean either century are refused
// outright, with a message naming both spellings. A rule that guesses would
// either put 2020s permanently out of reach or change what --decade 30s means
// in 2030.
func TestParseDecadeValueRejectsAmbiguous(t *testing.T) {
	for in, both := range map[string][2]string{
		"00s": {"1900s", "2000s"},
		"10s": {"1910s", "2010s"},
		"20s": {"1920s", "2020s"},
		"25s": {"1920s", "2020s"},
	} {
		_, err := parseDecadeValue(in)
		if err == nil {
			t.Errorf("parseDecadeValue(%q) succeeded, want an ambiguity error", in)
			continue
		}
		if !strings.Contains(err.Error(), "ambiguous") {
			t.Errorf("parseDecadeValue(%q) error = %q, want it to say ambiguous", in, err)
		}
		for _, spelling := range both {
			if !strings.Contains(err.Error(), spelling) {
				t.Errorf("parseDecadeValue(%q) error = %q, want it to name %s", in, err, spelling)
			}
		}
	}
}

func TestParseDecadeValueRejectsGarbage(t *testing.T) {
	for _, in := range []string{"", "s", "7s", "197s", "abc", "-5", "19700s"} {
		if got, err := parseDecadeValue(in); err == nil {
			t.Errorf("parseDecadeValue(%q) = %v, want an error", in, got)
		}
	}
}
