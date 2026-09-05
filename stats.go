package main

import (
	"sort"
	"time"
)

// topN is how many genres and how many labels `stats` lists.
const topN = 5

// Stats is everything `stats` reports, computed once and rendered twice.
// formatStats and newStatsPayload both read from this value, which is what
// keeps the text and JSON views from disagreeing about a figure -- unlike
// formatHistory and newHistoryPayload, which duplicate their clamp and need a
// test to stay in step.
type Stats struct {
	// Count is the described set, after filters. Total is the source set
	// before them: the collection, or favorites under --favorites. They are
	// equal when no filter narrowed anything.
	Count int
	Total int
	// Favorites is how many of the described set are favorited. Under
	// --favorites the described set is favorites, so it equals Count.
	Favorites int
	// FavoritesOnly records that the described set is favorites rather than
	// the collection. It changes only how the header reads, and lives here
	// rather than in a formatting parameter so both views agree about which
	// set they are describing.
	FavoritesOnly bool
	// SyncedAt is zero when nothing has ever been synced.
	SyncedAt time.Time
	Decades  []DecadeBucket
	Genres   []NameCount
	Labels   []NameCount
	Picked   PickedStats
}

// DecadeBucket is one row of the histogram. Decade is 0 for the albums whose
// year Discogs did not give us; that row renders as "unknown" and always
// sorts last.
type DecadeBucket struct {
	Decade int
	Count  int
}

// NameCount is one row of the top-genres or top-labels table.
type NameCount struct {
	Name  string
	Count int
}

// PickedStats is how much of the described set has ever been picked. Count is
// measured against Stats.Count, not Stats.Total: `stats --genre jazz` reports
// the share of your jazz.
type PickedStats struct {
	Count int
	// LastPicked is zero when nothing in the described set was ever picked.
	LastPicked time.Time
}

// Share is the fraction of the described set ever picked. One definition,
// used by both views: the text view rounds it for display and the JSON view
// emits it as it is, so a consumer never has to un-round.
func (s Stats) Share() float64 {
	if s.Count == 0 {
		return 0
	}
	return float64(s.Picked.Count) / float64(s.Count)
}

// computeStats derives every figure from values already in memory. It reads
// no files and no clock, which is what makes it testable without fixtures.
//
// total arrives separately because pool has already been filtered by the time
// it gets here, and the header needs both numbers to say "312 of 1247".
func computeStats(pool, favorites []Album, entries []HistoryEntry, total int, m Meta, favoritesOnly bool) Stats {
	s := Stats{
		Count:         len(pool),
		Total:         total,
		FavoritesOnly: favoritesOnly,
		SyncedAt:      m.SyncedAt,
		Decades:       decadeBuckets(pool),
		Genres:        topNames(countGenres(pool)),
		Labels:        topNames(countLabels(pool)),
	}

	for _, album := range pool {
		if containsAlbum(favorites, album) {
			s.Favorites++
		}

		// lastPlayedIndex, not a map: sameAlbum is not transitive when an
		// entry has no release ID, and a map key would silently assume it
		// was. This is the same backwards first-match scan every other
		// history comparison goes through.
		idx, played := lastPlayedIndex(entries, album)
		if !played {
			continue
		}
		s.Picked.Count++
		if ts := entries[idx].Timestamp; ts.After(s.Picked.LastPicked) {
			s.Picked.LastPicked = ts
		}
	}

	return s
}

// decadeBuckets returns one row per decade from the earliest present to the
// latest, so a decade you own nothing from shows as a zero row -- in a
// histogram a gap is information. Albums with no year go in a Decade 0 row,
// appended last and only when there are any.
func decadeBuckets(pool []Album) []DecadeBucket {
	counts := make(map[int]int)
	unknown := 0
	var lo, hi int
	known := false

	for _, a := range pool {
		if a.Year == 0 {
			unknown++
			continue
		}
		d := a.Year / 10 * 10
		counts[d]++
		if !known || d < lo {
			lo = d
		}
		if !known || d > hi {
			hi = d
		}
		known = true
	}

	var out []DecadeBucket
	if known {
		for d := lo; d <= hi; d += 10 {
			out = append(out, DecadeBucket{Decade: d, Count: counts[d]})
		}
	}
	if unknown > 0 {
		out = append(out, DecadeBucket{Decade: 0, Count: unknown})
	}
	return out
}

// countGenres counts albums per genre. An album listing several genres counts
// once in each; a genre repeated on one album counts once.
func countGenres(pool []Album) map[string]int {
	counts := make(map[string]int)
	for _, a := range pool {
		seen := make(map[string]bool, len(a.Genres))
		for _, g := range a.Genres {
			if g == "" || seen[g] {
				continue
			}
			seen[g] = true
			counts[g]++
		}
	}
	return counts
}

// countLabels counts albums per label. Album.Label is a single string, so an
// album contributes at most one.
func countLabels(pool []Album) map[string]int {
	counts := make(map[string]int)
	for _, a := range pool {
		if a.Label == "" {
			continue
		}
		counts[a.Label]++
	}
	return counts
}

// topNames returns the topN highest counts, ties broken by name ascending.
//
// The tiebreak is not cosmetic. Go randomises map iteration order, so without
// a total order here two equally common genres would swap places between runs
// and no golden test of this output could exist.
func topNames(counts map[string]int) []NameCount {
	out := make([]NameCount, 0, len(counts))
	for name, n := range counts {
		out = append(out, NameCount{Name: name, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > topN {
		out = out[:topN]
	}
	return out
}
