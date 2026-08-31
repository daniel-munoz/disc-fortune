package main

import "fmt"

// drawMode selects how pick draws from the candidate pool.
type drawMode int

const (
	// drawFresh excludes the recently played. It is the zero value so a
	// selection built without an explicit mode gets the default rather than
	// silently falling back to an unfiltered draw.
	drawFresh drawMode = iota
	// drawAny is a uniform draw; history is not consulted at all. This is
	// what restores pre-2.3 behavior for anyone scripting against it.
	drawAny
	// drawStale is drawFresh's exclusion followed by a bias toward the
	// records left unplayed longest.
	drawStale
)

// parseDrawMode converts the --draw flag value to a drawMode.
func parseDrawMode(s string) (drawMode, error) {
	switch s {
	case "fresh":
		return drawFresh, nil
	case "any":
		return drawAny, nil
	case "stale":
		return drawStale, nil
	default:
		return drawFresh, fmt.Errorf("invalid --draw value %q (want any, fresh, or stale)", s)
	}
}

// maxAntiRepeatWindow caps how many recently played albums the default draw
// excludes, however large the collection is.
const maxAntiRepeatWindow = 10

// antiRepeatWindow returns how many recently played albums to exclude from a
// pool of poolSize candidates.
//
// Dividing by three is what makes the degradation automatic rather than a
// special case: a pool of one or two excludes nothing, so a heavily filtered
// query can never be narrowed into an empty set. Note that this bounds the
// number of excluded *names*, not of excluded albums -- see excludeRecent for
// why that distinction needs a guard.
func antiRepeatWindow(poolSize int) int {
	n := poolSize / 3
	if n > maxAntiRepeatWindow {
		return maxAntiRepeatWindow
	}
	return n
}

// lastPlayedIndex returns the index in entries of the most recent pick of
// album, and whether it was ever picked.
//
// This is the single point where picking decides what "the same record" means,
// and it scans backwards and stops at the first match. That is the only shape
// sameAlbum is safe in: an entry with no release ID is a wildcard for its
// name, so a comparison that kept scanning would conflate distinct pressings.
func lastPlayedIndex(entries []HistoryEntry, album Album) (int, bool) {
	for i := len(entries) - 1; i >= 0; i-- {
		if sameAlbum(album, entries[i].Album) {
			return i, true
		}
	}
	return 0, false
}

// containsAlbum reports whether album matches any entry of list.
func containsAlbum(list []Album, album Album) bool {
	for _, a := range list {
		if sameAlbum(a, album) {
			return true
		}
	}
	return false
}

// recentlyPlayed returns the last n distinct albums in entries, most recent
// first.
//
// Distinct albums rather than raw entries: playing one record ten times in a
// row should not spend the whole window on that one record.
func recentlyPlayed(entries []HistoryEntry, n int) []Album {
	if n <= 0 {
		return nil
	}
	var recent []Album
	for i := len(entries) - 1; i >= 0 && len(recent) < n; i-- {
		album := entries[i].Album
		if containsAlbum(recent, album) {
			continue
		}
		recent = append(recent, album)
	}
	return recent
}
