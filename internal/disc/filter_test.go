package disc

import (
	"reflect"
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
			var f Filter
			if tt.yearFilter != "" {
				f = Filter{Year: years(t, tt.yearFilter)}
			}
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

	f := Filter{Genre: include("jazz")}
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

	f := Filter{Year: years(t, "1970"), Genre: include("jazz")}
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

	f := Filter{Query: include("miles")}
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

	f := Filter{Query: include("giant")}
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

	f := Filter{Query: include("MILES")}
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

	f := Filter{}
	filtered := f.Apply(albums)
	if len(filtered) != 2 {
		t.Errorf("got %d albums, want 2 (empty query should not filter)", len(filtered))
	}
}

func TestFilterByQueryNoMatch(t *testing.T) {
	albums := []Album{
		{Artist: "Miles Davis", Title: "Kind of Blue"},
	}

	f := Filter{Query: include("nonexistent")}
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

	f := Filter{Query: include("miles")}
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

	f := Filter{Query: include("miles"), Year: years(t, "1959")}
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

	got := Filter{Query: include("miles")}.Apply(albums)
	if len(got) != 1 || got[0].ReleaseID != 111 {
		t.Fatalf("Apply() = %+v, want the Miles Davis release", got)
	}

	got = Filter{Query: include("souvlaki")}.Apply(albums)
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
		r, err := ParseYearValue(v)
		if err != nil {
			t.Fatalf("ParseYearValue(%q): %v", v, err)
		}
		yf.Include = append(yf.Include, r)
	}
	return yf
}

func TestParseYearValue(t *testing.T) {
	tests := []struct {
		in      string
		want    YearRange
		wantErr bool
	}{
		{in: "1975", want: YearRange{1975, 1975}},
		{in: " 1975 ", want: YearRange{1975, 1975}},
		{in: "1970-1980", want: YearRange{1970, 1980}},
		{in: "1980-1970", want: YearRange{1970, 1980}}, // auto-swap, as in v2.3.0
		{in: "1970 - 1980", want: YearRange{1970, 1980}},
		{in: "nineteen", wantErr: true},
		{in: "1970-", wantErr: true},
		{in: "1970-1980-1990", wantErr: true},
		{in: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseYearValue(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseYearValue(%q) = %v, want an error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseYearValue(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ParseYearValue(%q) = %v, want %v", tt.in, got, tt.want)
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
		{"include single hit", YearFilter{Include: []YearRange{{1975, 1975}}}, 1975, true},
		{"include single miss", YearFilter{Include: []YearRange{{1975, 1975}}}, 1976, false},
		{"include range hit", YearFilter{Include: []YearRange{{1970, 1980}}}, 1975, true},
		{"include is OR", YearFilter{Include: []YearRange{{1959, 1959}, {1970, 1979}}}, 1975, true},
		{"exclude hit", YearFilter{Exclude: []YearRange{{1970, 1979}}}, 1975, false},
		{"exclude miss", YearFilter{Exclude: []YearRange{{1970, 1979}}}, 1985, true},
		{"exclusion beats inclusion", YearFilter{Include: []YearRange{{1970, 1980}}, Exclude: []YearRange{{1975, 1975}}}, 1975, false},
		{"unknown year fails an inclusion", YearFilter{Include: []YearRange{{1970, 1980}}}, 0, false},
		{"unknown year survives an exclusion", YearFilter{Exclude: []YearRange{{1970, 1980}}}, 0, true},
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
		want YearRange
	}{
		{"70s", YearRange{1970, 1979}},
		{"1970s", YearRange{1970, 1979}},
		{"1970", YearRange{1970, 1979}},
		{"2020s", YearRange{2020, 2029}},
		{"30s", YearRange{1930, 1939}},
		{"90s", YearRange{1990, 1999}},
		{"70S", YearRange{1970, 1979}}, // case-insensitive
		{" 70s ", YearRange{1970, 1979}},
		{"1975", YearRange{1970, 1979}}, // any year names its decade
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseDecadeValue(tt.in)
			if err != nil {
				t.Fatalf("ParseDecadeValue(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ParseDecadeValue(%q) = %v, want %v", tt.in, got, tt.want)
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
		_, err := ParseDecadeValue(in)
		if err == nil {
			t.Errorf("ParseDecadeValue(%q) succeeded, want an ambiguity error", in)
			continue
		}
		if !strings.Contains(err.Error(), "ambiguous") {
			t.Errorf("ParseDecadeValue(%q) error = %q, want it to say ambiguous", in, err)
		}
		for _, spelling := range both {
			if !strings.Contains(err.Error(), spelling) {
				t.Errorf("ParseDecadeValue(%q) error = %q, want it to name %s", in, err, spelling)
			}
		}
	}
}

func TestParseDecadeValueRejectsGarbage(t *testing.T) {
	for _, in := range []string{"", "s", "7s", "197s", "abc", "-5", "19700s"} {
		if got, err := ParseDecadeValue(in); err == nil {
			t.Errorf("ParseDecadeValue(%q) = %v, want an error", in, got)
		}
	}
}

// QueryField is an index rather than a lookup, so a test pins the assumption.
func TestQueryIsTheFirstFilterField(t *testing.T) {
	if Fields[QueryField].Name != "query" {
		t.Fatalf("Fields[QueryField].Name = %q, want %q",
			Fields[QueryField].Name, "query")
	}
}

// Every table entry must read something off an album and point at a distinct
// part of a Filter. A copy-pasted entry that forgets to change one of the two
// closures is the failure this catches.
func TestFilterFieldsAreWiredDistinctly(t *testing.T) {
	var f Filter
	seen := map[*FieldFilter]string{}
	for _, field := range Fields {
		p := field.Part(&f)
		if p == nil {
			t.Errorf("%s: part() returned nil", field.Name)
			continue
		}
		if other, dup := seen[p]; dup {
			t.Errorf("%s and %s point at the same FieldFilter", field.Name, other)
		}
		seen[p] = field.Name
		if field.albumValue == nil {
			t.Errorf("%s: albumValue is nil", field.Name)
		}
		if field.Help == "" {
			t.Errorf("%s: help is empty", field.Name)
		}
	}
	if len(seen) != 6 {
		t.Errorf("wired %d fields, want 6 (query, artist, title, genre, label, format)", len(seen))
	}
}

func TestFilterFieldsReadTheRightAlbumValues(t *testing.T) {
	album := Album{
		Artist:  "Miles Davis",
		Title:   "Kind of Blue",
		Label:   "Columbia",
		Genres:  []string{"Jazz", "Bebop"},
		Formats: []string{"Vinyl", "LP", "Blue Translucent"},
	}
	want := map[string][]string{
		"query":  {"Miles Davis - Kind of Blue"},
		"artist": {"Miles Davis"},
		"title":  {"Kind of Blue"},
		"label":  {"Columbia"},
		"genre":  {"Jazz", "Bebop"},
		"format": {"Vinyl", "LP", "Blue Translucent"},
	}

	for _, field := range Fields {
		got := field.albumValue(album)
		exp := want[field.Name]
		if len(got) != len(exp) {
			t.Errorf("%s: albumValue = %q, want %q", field.Name, got, exp)
			continue
		}
		for i := range got {
			if got[i] != exp[i] {
				t.Errorf("%s: albumValue = %q, want %q", field.Name, got, exp)
				break
			}
		}
	}
}

// The grammar in one test: values within a field OR, different fields AND,
// and an exclusion removes a match outright.
func TestFilterGrammar(t *testing.T) {
	albums := []Album{
		{Artist: "Miles Davis", Title: "Kind of Blue", Year: 1959, Label: "Columbia", Genres: []string{"Jazz"}},
		{Artist: "Herbie Hancock", Title: "Head Hunters", Year: 1973, Label: "Columbia", Genres: []string{"Jazz", "Funk"}},
		{Artist: "Parliament", Title: "Mothership Connection", Year: 1975, Label: "Casablanca", Genres: []string{"Funk"}},
		{Artist: "Black Sabbath", Title: "Paranoid", Year: 1970, Label: "Vertigo", Genres: []string{"Rock"}},
		{Artist: "Unknown", Title: "Untitled"}, // no year, no label, no genres
	}

	tests := []struct {
		name   string
		filter Filter
		want   []string // artists, in order
	}{
		{
			name:   "one value behaves as it always has",
			filter: Filter{Genre: include("jazz")},
			want:   []string{"Miles Davis", "Herbie Hancock"},
		},
		{
			name:   "values within a field OR",
			filter: Filter{Genre: include("jazz", "funk")},
			want:   []string{"Miles Davis", "Herbie Hancock", "Parliament"},
		},
		{
			name:   "different fields AND",
			filter: Filter{Genre: include("jazz", "funk"), Label: include("columbia")},
			want:   []string{"Miles Davis", "Herbie Hancock"},
		},
		{
			name:   "an exclusion removes matches",
			filter: Filter{Genre: exclude("rock")},
			want:   []string{"Miles Davis", "Herbie Hancock", "Parliament", "Unknown"},
		},
		{
			name:   "an exclusion drops an album that also matches something else",
			filter: Filter{Genre: include("jazz"), Label: exclude("columbia")},
			want:   nil,
		},
		{
			name:   "exclusion beats inclusion",
			filter: Filter{Genre: FieldFilter{Include: []string{"jazz"}, Exclude: []string{"jazz"}}},
			want:   nil,
		},
		{
			name:   "an empty field is excluded by nothing",
			filter: Filter{Label: exclude("columbia")},
			want:   []string{"Parliament", "Black Sabbath", "Unknown"},
		},
		{
			name:   "an unknown year is excluded by nothing",
			filter: Filter{Year: YearFilter{Exclude: []YearRange{{1959, 1959}}}},
			want:   []string{"Herbie Hancock", "Parliament", "Black Sabbath", "Unknown"},
		},
		{
			name:   "year and decade ranges OR together",
			filter: Filter{Year: YearFilter{Include: []YearRange{{1959, 1959}, {1970, 1979}}}},
			want:   []string{"Miles Davis", "Herbie Hancock", "Parliament", "Black Sabbath"},
		},
		{
			name:   "artist and title are separate fields",
			filter: Filter{Artist: include("miles")},
			want:   []string{"Miles Davis"},
		},
		{
			name:   "title does not match the artist",
			filter: Filter{Title: include("miles")},
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.filter.Apply(albums)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d albums %v, want %d %v", len(got), artistsOf(got), len(tt.want), tt.want)
			}
			for i, a := range got {
				if a.Artist != tt.want[i] {
					t.Errorf("album %d = %q, want %q", i, a.Artist, tt.want[i])
				}
			}
		})
	}
}

func artistsOf(albums []Album) []string {
	out := make([]string, len(albums))
	for i, a := range albums {
		out[i] = a.Artist
	}
	return out
}

func TestMatchAlbumsClassifies(t *testing.T) {
	miles := Album{ReleaseID: 1, Artist: "Miles Davis", Title: "Kind of Blue"}
	milesAlt := Album{ReleaseID: 2, Artist: "Miles Davis", Title: "Kind of Blue"}
	ride := Album{ReleaseID: 3, Artist: "Ride", Title: "Nowhere"}
	pool := []Album{miles, milesAlt, ride}

	t.Run("one", func(t *testing.T) {
		album, matches, status := MatchAlbums(pool, Filter{ReleaseID: 3})
		if status != matchedOne {
			t.Fatalf("status = %v, want matchedOne", status)
		}
		if album.ReleaseID != 3 {
			t.Errorf("album = %+v, want release 3", album)
		}
		if matches != nil {
			t.Errorf("matches = %v, want nil for a single match", matches)
		}
	})

	t.Run("none", func(t *testing.T) {
		_, _, status := MatchAlbums(pool, Filter{ReleaseID: 999})
		if status != MatchedNone {
			t.Errorf("status = %v, want MatchedNone", status)
		}
	})

	t.Run("many", func(t *testing.T) {
		f := Filter{}
		f.Query.Include = []string{"miles"}
		album, matches, status := MatchAlbums(pool, f)
		if status != MatchedMany {
			t.Fatalf("status = %v, want MatchedMany", status)
		}
		if len(matches) != 2 {
			t.Errorf("matches = %d, want 2", len(matches))
		}
		if !reflect.DeepEqual(album, Album{}) {
			t.Errorf("album = %+v, want the zero Album for MatchedMany", album)
		}
	})
}

// An unset filter returns the input untouched, which is what keeps `list`
// from copying the whole collection for nothing.
func TestFilterAnyReportsWhetherAnythingIsSet(t *testing.T) {
	if (Filter{}).Any() {
		t.Error("empty Filter.Any() = true, want false")
	}
	for name, f := range map[string]Filter{
		"query include": {Query: include("miles")},
		"query exclude": {Query: exclude("bootleg")},
		"artist":        {Artist: include("miles")},
		"title":         {Title: include("blue")},
		"genre":         {Genre: include("jazz")},
		"label":         {Label: include("columbia")},
		"format":        {Format: include("vinyl")},
		"year include":  {Year: YearFilter{Include: []YearRange{{1975, 1975}}}},
		"year exclude":  {Year: YearFilter{Exclude: []YearRange{{1975, 1975}}}},
		"release id":    {ReleaseID: 1839278},
	} {
		if !f.Any() {
			t.Errorf("%s: any() = false, want true", name)
		}
	}
}
