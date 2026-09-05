package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// FieldFilter is one field's constraint: values to require (any of), and
// values that disqualify. Both empty means unconstrained.
type FieldFilter struct {
	Include []string
	Exclude []string
}

// matches reports whether an album passes this field's constraint. values is
// what the album has for the field: a one-element slice for a scalar like
// Label, every element for a list like Genres.
//
// Exclusion is checked first and wins outright, so --genre jazz
// --exclude-genre jazz is an empty filter rather than a conflict to resolve.
// An album with nothing in this field matches no value, and so is excluded by
// nothing -- absence is not a match, which is what stops one --exclude-label
// from deleting every record Discogs left unlabelled.
func (ff FieldFilter) matches(values []string) bool {
	for _, ex := range ff.Exclude {
		if containsAny(values, ex) {
			return false
		}
	}
	if len(ff.Include) == 0 {
		return true
	}
	for _, in := range ff.Include {
		if containsAny(values, in) {
			return true
		}
	}
	return false
}

// containsAny reports whether needle is a case-insensitive substring of any
// of values. An empty needle matches everything, which is why empty flag
// values are dropped at parse time rather than defended against here.
func containsAny(values []string, needle string) bool {
	needle = strings.ToLower(needle)
	for _, v := range values {
		if strings.Contains(strings.ToLower(v), needle) {
			return true
		}
	}
	return false
}

// yearRange is an inclusive span of years. A single year is a range of one.
type yearRange struct{ start, end int }

func (r yearRange) contains(year int) bool { return year >= r.start && year <= r.end }

// YearFilter is FieldFilter's shape over parsed ranges, because --year
// compares numerically rather than by substring. --decade appends to it: they
// are two spellings of one field, so --year 1959 --decade 70s means "1959 or
// the 70s" rather than the empty intersection of two AND-ed fields.
type YearFilter struct {
	Include []yearRange
	Exclude []yearRange
}

// matches reports whether an album's year passes. A zero year means Discogs
// gave none: it falls in no range, so an inclusion never accepts it and an
// exclusion never rejects it.
func (yf YearFilter) matches(year int) bool {
	if year == 0 {
		return len(yf.Include) == 0
	}
	for _, r := range yf.Exclude {
		if r.contains(year) {
			return false
		}
	}
	if len(yf.Include) == 0 {
		return true
	}
	for _, r := range yf.Include {
		if r.contains(year) {
			return true
		}
	}
	return false
}

// errBadYearFormat keeps the wording v2.3.0 shipped, because it is what users
// have already seen and scripted against.
var errBadYearFormat = errors.New("invalid year format. Use --year 1975 or --year 1970-1980")

// parseYearValue parses one --year value: a single year, or a "start-end"
// range whose ends are swapped when given backwards.
func parseYearValue(s string) (yearRange, error) {
	s = strings.TrimSpace(s)

	if strings.Contains(s, "-") {
		parts := strings.Split(s, "-")
		if len(parts) != 2 {
			return yearRange{}, errBadYearFormat
		}
		start, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		end, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil {
			return yearRange{}, errBadYearFormat
		}
		if start > end {
			start, end = end, start
		}
		return yearRange{start, end}, nil
	}

	year, err := strconv.Atoi(s)
	if err != nil {
		return yearRange{}, errBadYearFormat
	}
	return yearRange{year, year}, nil
}

// parseDecadeValue parses one --decade value into the ten years it names.
// Accepted: 1970s, 1970, any year within a decade, and the two-digit forms
// 30s through 90s -- unambiguous because there are no 2030s pressings yet.
// Refused: 00s, 10s and 20s, which could name either century.
//
// The alternatives were "two digits always mean 19xx", which puts the 2020s
// permanently out of reach of a two-digit value, and "the most recent decade
// that has begun", which silently changes what --decade 30s means in 2030 and
// forces every test onto a fixed clock. Refusing the three genuinely
// ambiguous inputs is the only rule that is both stable and honest.
func parseDecadeValue(s string) (yearRange, error) {
	badFormat := fmt.Errorf("invalid decade %q. Use --decade 70s or --decade 1970s", s)

	v := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), "s")
	if len(v) != 2 && len(v) != 4 {
		return yearRange{}, badFormat
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return yearRange{}, badFormat
	}

	if len(v) == 2 {
		d := n - n%10
		if d < 30 {
			return yearRange{}, fmt.Errorf("ambiguous decade %q: write 19%02ds or 20%02ds", s, d, d)
		}
		n = 1900 + d
	}

	start := n - n%10
	return yearRange{start, start + 9}, nil
}

// filterField describes one substring-matched filter: the flag name it is
// spelled with, its line of help, what it reads off an album, and where its
// parsed values land in a Filter. Flag registration, help generation and
// matching all loop over the table, so a new substring filter is one entry
// here rather than four edits across three files.
//
// --year is deliberately not in the table: it parses its values and compares
// them numerically, and two flag names (--year and --decade) feed it, so
// forcing it into this shape would cost more than the duplication saves.
type filterField struct {
	name       string
	help       string
	albumValue func(Album) []string
	part       func(*Filter) *FieldFilter
}

// queryField is the index of the query entry below. Query is special twice
// over: it is the one field that satisfies favorite's "requires a query"
// rule, and the one whose inclusions do not count as narrowing.
// TestQueryIsTheFirstFilterField pins this.
const queryField = 0

var filterFields = []filterField{
	{
		name:       "query",
		help:       `Filter by "Artist - Title" (case-insensitive substring)`,
		albumValue: func(a Album) []string { return []string{a.Key()} },
		part:       func(f *Filter) *FieldFilter { return &f.Query },
	},
	{
		name:       "artist",
		help:       "Filter by artist",
		albumValue: func(a Album) []string { return []string{a.Artist} },
		part:       func(f *Filter) *FieldFilter { return &f.Artist },
	},
	{
		name:       "title",
		help:       "Filter by title",
		albumValue: func(a Album) []string { return []string{a.Title} },
		part:       func(f *Filter) *FieldFilter { return &f.Title },
	},
	{
		name:       "genre",
		help:       "Filter by genre",
		albumValue: func(a Album) []string { return a.Genres },
		part:       func(f *Filter) *FieldFilter { return &f.Genre },
	},
	{
		name:       "label",
		help:       "Filter by label",
		albumValue: func(a Album) []string { return []string{a.Label} },
		part:       func(f *Filter) *FieldFilter { return &f.Label },
	},
	{
		name: "format",
		// Format matches any entry of Album.Formats, which includes the
		// format name, its descriptions, and its free text -- the last
		// being where Discogs records a pressing's colour.
		help:       "Filter by format or colour",
		albumValue: func(a Album) []string { return a.Formats },
		part:       func(f *Filter) *FieldFilter { return &f.Format },
	},
}

// Filter narrows a collection. Values within a field OR together, different
// fields AND together, and any exclusion removes a match outright.
type Filter struct {
	Query, Artist, Title, Genre, Label, Format FieldFilter
	Year                                       YearFilter
	// ReleaseID selects one exact record. Zero means unset. Unlike the
	// fields above it identifies rather than narrows, which is why it is
	// compared whole, needs no query alongside it, and takes neither
	// several values nor an exclusion.
	ReleaseID int
}

// Apply returns the albums matching the filter. An unset filter returns the
// input untouched rather than copying it.
func (f Filter) Apply(albums []Album) []Album {
	if !f.any() {
		return albums
	}

	var filtered []Album
	for _, album := range albums {
		if f.matches(album) {
			filtered = append(filtered, album)
		}
	}
	return filtered
}

// any reports whether the filter constrains anything at all.
func (f Filter) any() bool {
	if f.ReleaseID != 0 || len(f.Year.Include) > 0 || len(f.Year.Exclude) > 0 {
		return true
	}
	for _, field := range filterFields {
		p := field.part(&f)
		if len(p.Include) > 0 || len(p.Exclude) > 0 {
			return true
		}
	}
	return false
}

func (f Filter) matches(album Album) bool {
	if f.ReleaseID != 0 && album.ReleaseID != f.ReleaseID {
		return false
	}
	for _, field := range filterFields {
		if !field.part(&f).matches(field.albumValue(album)) {
			return false
		}
	}
	return f.Year.matches(album.Year)
}

// matchStatus classifies what a filter selected: exactly one record, none, or
// several. favorite, unfavorite and open all act on a single record and all
// need the same three-way answer, so the classification lives here rather
// than being spelled out at each call site.
type matchStatus int

const (
	matchedOne matchStatus = iota
	matchedNone
	matchedMany
)

// matchAlbums applies filter and classifies the result. The returned Album is
// meaningful only for matchedOne, and the slice only for matchedMany; the
// other is left at its zero value so a caller reading the wrong one gets
// nothing rather than something plausible.
func matchAlbums(albums []Album, filter Filter) (Album, []Album, matchStatus) {
	matches := filter.Apply(albums)
	switch len(matches) {
	case 0:
		return Album{}, nil, matchedNone
	case 1:
		return matches[0], nil, matchedOne
	default:
		return Album{}, matches, matchedMany
	}
}
