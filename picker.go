package main

import (
	"fmt"
	"math/rand/v2"
)

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

// unheardOnly returns the albums in pool that never appear in entries.
//
// Conservative by construction: a history entry with no release ID matches
// every pressing of its title, so none of them count as unheard. Nothing in
// the file says which pressing was actually played, and calling the others
// unheard would assert more than the data supports. The backfill retires
// these entries on the first sync after upgrade.
func unheardOnly(pool []Album, entries []HistoryEntry) []Album {
	var out []Album
	for _, album := range pool {
		if _, played := lastPlayedIndex(entries, album); !played {
			out = append(out, album)
		}
	}
	return out
}

// pickAlbum chooses one album from pool, consulting entries per mode.
//
// pool must not be empty; the caller reports that case with its own message
// and exit code, because what to say about it depends on which filters were
// responsible.
func pickAlbum(pool []Album, entries []HistoryEntry, mode drawMode, rng *rand.Rand) Album {
	if mode == drawAny {
		return pool[rng.IntN(len(pool))]
	}

	// drawStale is drawFresh plus a bias, not an alternative to it, so the
	// anti-repeat guarantee holds whatever --draw says.
	candidates := excludeRecent(pool, entries)

	if mode == drawStale {
		return candidates[weightedIndex(staleWeights(candidates, entries), rng)]
	}
	return candidates[rng.IntN(len(candidates))]
}

// excludeRecent drops the recently played from pool, falling back to the whole
// pool when that would leave nothing.
//
// The fallback is reachable, not padding. antiRepeatWindow bounds the number
// of excluded *names*, and a history entry with no release ID matches every
// pressing of its title: three identically-titled pressings and one un-ID'd
// entry empty a pool with a window of one.
func excludeRecent(pool []Album, entries []HistoryEntry) []Album {
	recent := recentlyPlayed(entries, antiRepeatWindow(len(pool)))
	if len(recent) == 0 {
		return pool
	}

	var kept []Album
	for _, album := range pool {
		if !containsAlbum(recent, album) {
			kept = append(kept, album)
		}
	}
	if len(kept) == 0 {
		return pool
	}
	return kept
}

// staleWeights scores each candidate by how long it has gone unplayed,
// measured in picks rather than in time. A never-played record outranks every
// played one; among played ones the least recent wins. The lowest weight is 1,
// so nothing is ever unreachable, and an empty history makes every weight
// equal, which degenerates to a uniform draw.
//
// Linear rather than exponential: the records that would justify a sharper
// curve are the recently played ones, and excludeRecent has already removed
// them.
func staleWeights(candidates []Album, entries []HistoryEntry) []int {
	weights := make([]int, len(candidates))
	for i, album := range candidates {
		idx, played := lastPlayedIndex(entries, album)
		if !played {
			weights[i] = len(entries) + 1
			continue
		}
		weights[i] = len(entries) - idx
	}
	return weights
}

// weightedIndex draws an index from weights with probability proportional to
// each weight. Every weight must be at least 1, which staleWeights guarantees.
func weightedIndex(weights []int, rng *rand.Rand) int {
	total := 0
	for _, w := range weights {
		total += w
	}
	target := rng.IntN(total)
	for i, w := range weights {
		target -= w
		if target < 0 {
			return i
		}
	}
	// Unreachable while every weight is positive; returning the last index
	// beats panicking if that ever stops being true.
	return len(weights) - 1
}

// newRNG seeds a generator from the global source. pickAlbum takes an explicit
// *rand.Rand rather than calling rand.IntN so that tests can pin the sequence;
// this is where production gets a real one.
func newRNG() *rand.Rand {
	return rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
}
