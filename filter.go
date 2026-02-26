package main

import (
	"fmt"
	"strconv"
	"strings"
)

// Filter represents album filtering criteria.
type Filter struct {
	Year   string
	Genre  string
	Label  string
	Format string
}

// Apply filters albums based on criteria.
func (f Filter) Apply(albums []Album) []Album {
	if f.Year == "" && f.Genre == "" && f.Label == "" && f.Format == "" {
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
