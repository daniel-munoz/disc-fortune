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

// Filter represents album filtering criteria.
type Filter struct {
	Query string
	Year  string
	Genre string
	Label string
	// Format matches any entry of Album.Formats, which includes the format
	// name, its descriptions, and its free text -- the last being where
	// Discogs records a pressing's colour.
	Format string
	// ReleaseID selects one exact record. Zero means unset. Unlike the
	// filters above it identifies rather than narrows, which is why it is
	// compared whole rather than by substring and why it needs no query
	// alongside it.
	ReleaseID int
}

// Apply filters albums based on criteria.
func (f Filter) Apply(albums []Album) []Album {
	if f.Query == "" && f.Year == "" && f.Genre == "" && f.Label == "" && f.Format == "" && f.ReleaseID == 0 {
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

func (f Filter) matches(album Album) bool {
	if f.ReleaseID != 0 && album.ReleaseID != f.ReleaseID {
		return false
	}
	if f.Query != "" && !f.matchesQuery(album) {
		return false
	}
	if f.Year != "" && !f.matchesYear(album.Year) {
		return false
	}
	if f.Genre != "" && !f.matchesGenre(album.Genres) {
		return false
	}
	if f.Label != "" && !f.matchesString(album.Label, f.Label) {
		return false
	}
	if f.Format != "" && !f.matchesFormats(album.Formats) {
		return false
	}
	return true
}

func (f Filter) matchesQuery(album Album) bool {
	return f.matchesString(album.Key(), f.Query)
}

func (f Filter) matchesYear(year int) bool {
	if year == 0 {
		return false
	}

	// Parse year or year range
	if strings.Contains(f.Year, "-") {
		parts := strings.Split(f.Year, "-")
		if len(parts) != 2 {
			return false
		}
		start, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		end, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil {
			return false
		}
		// Auto-swap if backwards
		if start > end {
			start, end = end, start
		}
		return year >= start && year <= end
	}

	// Single year
	targetYear, err := strconv.Atoi(strings.TrimSpace(f.Year))
	if err != nil {
		return false
	}
	return year == targetYear
}

func (f Filter) matchesGenre(genres []string) bool {
	for _, g := range genres {
		if f.matchesString(g, f.Genre) {
			return true
		}
	}
	return false
}

func (f Filter) matchesFormats(formats []string) bool {
	for _, format := range formats {
		if f.matchesString(format, f.Format) {
			return true
		}
	}
	return false
}

func (f Filter) matchesString(value, filter string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(filter))
}

// ParseYearFilter validates year filter format.
func ParseYearFilter(yearStr string) error {
	if yearStr == "" {
		return nil
	}

	if strings.Contains(yearStr, "-") {
		parts := strings.Split(yearStr, "-")
		if len(parts) != 2 {
			return fmt.Errorf("invalid year format. Use --year 1975 or --year 1970-1980")
		}
		_, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		_, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil {
			return fmt.Errorf("invalid year format. Use --year 1975 or --year 1970-1980")
		}
		return nil
	}

	_, err := strconv.Atoi(strings.TrimSpace(yearStr))
	if err != nil {
		return fmt.Errorf("invalid year format. Use --year 1975 or --year 1970-1980")
	}
	return nil
}
