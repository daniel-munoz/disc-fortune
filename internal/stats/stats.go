package stats

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/daniel-munoz/disc-fortune/v2/internal/disc"
	"github.com/daniel-munoz/disc-fortune/v2/internal/pick"
	"github.com/daniel-munoz/disc-fortune/v2/internal/term"
)

// topN is how many genres and how many labels `stats` lists.
const topN = 5

// Stats is everything `stats` reports, computed once and rendered twice.
// Format and newStatsPayload both read from this value, which is what
// keeps the text and JSON views from disagreeing about a figure -- unlike
// disc.FormatHistory and newHistoryPayload, which duplicate their clamp and
// need a test to stay in step.
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

// Compute derives every figure from values already in memory. It reads
// no files and no clock, which is what makes it testable without fixtures.
//
// total arrives separately because pool has already been filtered by the time
// it gets here, and the header needs both numbers to say "312 of 1247".
func Compute(pool, favorites []disc.Album, entries []disc.HistoryEntry, total int, m disc.Meta, favoritesOnly bool) Stats {
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
		if pick.ContainsAlbum(favorites, album) {
			s.Favorites++
		}

		// pick.LastPlayedIndex, not a map: disc.SameAlbum is not transitive
		// when an entry has no release ID, and a map key would silently
		// assume it was. This is the same backwards first-match scan every
		// other history comparison goes through.
		idx, played := pick.LastPlayedIndex(entries, album)
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
func decadeBuckets(pool []disc.Album) []DecadeBucket {
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
func countGenres(pool []disc.Album) map[string]int {
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
func countLabels(pool []disc.Album) map[string]int {
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

// maxBarWidth is how many columns the largest decade bucket's bar fills.
// Fixed rather than derived from the terminal: the output has to be
// deterministic to be golden-tested, and a width that follows the terminal
// is not.
const maxBarWidth = 24

// Format renders the text view. Headings are bold and bars are dim;
// nothing else is coloured, and the JSON view is never coloured at all.
func Format(s Stats, useColor bool) string {
	var sb strings.Builder

	sb.WriteString(statsHeader(s))
	sb.WriteString("\n")
	if !s.SyncedAt.IsZero() {
		sb.WriteString("last synced " + disc.FormatTimestamp(s.SyncedAt) + "\n")
	}

	writeDecades(&sb, s.Decades, useColor)
	writeNameTable(&sb, "Top genres", s.Genres, useColor)
	writeNameTable(&sb, "Top labels", s.Labels, useColor)

	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("%d of %d %s picked at least once (%d%%)\n",
		s.Picked.Count, s.Count, disc.Plural(s.Count, "album", "albums"),
		int(math.Round(s.Share()*100))))
	if !s.Picked.LastPicked.IsZero() {
		sb.WriteString("  last picked " + disc.FormatTimestamp(s.Picked.LastPicked) + "\n")
	}

	return sb.String()
}

// statsHeader is the first line: how many records are described, out of how
// many, and how many of those are favorites. Under --favorites the described
// set already is favorites, so the tail would only repeat the count.
func statsHeader(s Stats) string {
	noun := disc.Plural(s.Count, "album", "albums")
	if s.FavoritesOnly {
		noun = disc.Plural(s.Count, "favorite", "favorites")
	}

	head := fmt.Sprintf("%d %s", s.Count, noun)
	if s.Count != s.Total {
		head = fmt.Sprintf("%d of %d %s", s.Count, s.Total, noun)
	}
	if !s.FavoritesOnly {
		head += fmt.Sprintf(" · %d %s", s.Favorites, disc.Plural(s.Favorites, "favorite", "favorites"))
	}
	return head
}

func writeDecades(sb *strings.Builder, buckets []DecadeBucket, useColor bool) {
	if len(buckets) == 0 {
		return
	}
	sb.WriteString("\n" + heading("Decades", useColor) + "\n")

	max, labelW, countW := 0, 0, 0
	for _, b := range buckets {
		if b.Count > max {
			max = b.Count
		}
		if n := len(decadeLabel(b.Decade)); n > labelW {
			labelW = n
		}
		if n := len(strconv.Itoa(b.Count)); n > countW {
			countW = n
		}
	}

	for _, b := range buckets {
		row := fmt.Sprintf("  %s  %*d  %s",
			pad(decadeLabel(b.Decade), labelW), countW, b.Count,
			dim(bar(b.Count, max), useColor))
		// A zero-count row has no bar, and padding it out to the bar column
		// would put trailing spaces in every golden file.
		sb.WriteString(strings.TrimRight(row, " ") + "\n")
	}
}

func writeNameTable(sb *strings.Builder, title string, rows []NameCount, useColor bool) {
	if len(rows) == 0 {
		return
	}
	sb.WriteString("\n" + heading(title, useColor) + "\n")

	nameW, countW := 0, 0
	for _, r := range rows {
		if n := len([]rune(r.Name)); n > nameW {
			nameW = n
		}
		if n := len(strconv.Itoa(r.Count)); n > countW {
			countW = n
		}
	}
	for _, r := range rows {
		sb.WriteString(fmt.Sprintf("  %s  %*d\n", pad(r.Name, nameW), countW, r.Count))
	}
}

// decadeLabel names a bucket. Decade 0 is the albums Discogs gave us no year
// for, which is not a decade and must not be printed as "0s".
func decadeLabel(decade int) string {
	if decade == 0 {
		return "unknown"
	}
	return fmt.Sprintf("%ds", decade)
}

// pad right-pads s to width columns, counting runes rather than bytes. A
// label like "Café Records" is shorter on screen than len() claims, and fmt's
// %-*s would misalign the column.
func pad(s string, width int) string {
	if n := len([]rune(s)); n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}

// bar renders count as eighth-blocks, scaled so max fills maxBarWidth
// columns. A zero count renders as nothing, which is what makes a gap decade
// legible as a gap.
func bar(count, max int) string {
	if max <= 0 || count <= 0 {
		return ""
	}

	eighths := count * maxBarWidth * 8 / max
	full, rem := eighths/8, eighths%8

	s := strings.Repeat("█", full)
	if rem > 0 {
		// The block characters run from U+2588 FULL BLOCK down to U+258F
		// LEFT ONE EIGHTH BLOCK, so the glyph for rem eighths is
		// U+2588 + (8 - rem).
		s += string(rune(0x2588 + (8 - rem)))
	}
	if s == "" {
		// A bucket too small to earn even one eighth still exists. Draw the
		// narrowest block rather than nothing, which would read as zero.
		s = "▏"
	}
	return s
}

func heading(s string, useColor bool) string {
	if !useColor {
		return s
	}
	return term.BoldWhite + s + term.Reset
}

func dim(s string, useColor bool) string {
	if !useColor || s == "" {
		return s
	}
	return term.Dim + s + term.Reset
}
