package main

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// jsonAlbum is the wire representation of an Album. It is deliberately a
// separate type: the on-disk format and the machine-readable output are two
// contracts with two owners, and serialising the storage struct directly
// would make every future storage change a breaking change for anyone
// scripting against the output.
//
// Every key is always present, so a consumer can model a fixed type and tell
// a missing value from a typo. A nil pointer marshals to null, which says
// "Discogs did not tell us" -- something "" and 0 cannot: "year": 0 sorts
// before 1959, and "release_id": 0 looks like an ID.
type jsonAlbum struct {
	ReleaseID *int     `json:"release_id"`
	Artist    string   `json:"artist"`
	Title     string   `json:"title"`
	Year      *int     `json:"year"`
	Label     *string  `json:"label"`
	CatNo     *string  `json:"catno"`
	Genres    []string `json:"genres"`
	Formats   []string `json:"formats"`
}

// newJSONAlbum converts a stored Album to its wire form. Artist and Title are
// never null: they are the one pair every entry has, and Album.Key() -- the
// identity for anything written before release IDs existed -- is built from
// them.
func newJSONAlbum(a Album) jsonAlbum {
	return jsonAlbum{
		ReleaseID: intOrNull(a.ReleaseID),
		Artist:    a.Artist,
		Title:     a.Title,
		Year:      intOrNull(a.Year),
		Label:     stringOrNull(a.Label),
		CatNo:     stringOrNull(a.CatNo),
		Genres:    listOrEmpty(a.Genres),
		Formats:   listOrEmpty(a.Formats),
	}
}

func intOrNull(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}

func stringOrNull(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// listOrEmpty turns a nil slice into an empty one, so a record with no genres
// emits [] rather than null and a consumer's loop needs no nil check.
func listOrEmpty(vals []string) []string {
	if vals == nil {
		return []string{}
	}
	return vals
}

// The payloads below are objects rather than bare arrays for one reason: a key
// can be added to an object without breaking a consumer, and a top-level array
// can never become an object. For a schema meant to be permanent, that
// asymmetry decides it.

// pickPayload is `pick --json`.
type pickPayload struct {
	Album jsonAlbum `json:"album"`
}

// listPayload is `list --json`. Count is how many albums were emitted.
type listPayload struct {
	Albums []jsonAlbum `json:"albums"`
	Count  int         `json:"count"`
}

// jsonHistoryEntry pairs an album with when it was picked. The timestamp is
// RFC 3339 exactly as stored, so the wire value and the history.json value are
// the same string.
type jsonHistoryEntry struct {
	Album     jsonAlbum `json:"album"`
	Timestamp time.Time `json:"timestamp"`
}

// historyPayload is `history --json`. Count is how many entries were emitted,
// not how many the file holds.
type historyPayload struct {
	Entries []jsonHistoryEntry `json:"entries"`
	Count   int                `json:"count"`
}

func newListPayload(albums []Album) listPayload {
	out := make([]jsonAlbum, 0, len(albums))
	for _, a := range albums {
		out = append(out, newJSONAlbum(a))
	}
	return listPayload{Albums: out, Count: len(out)}
}

// newHistoryPayload returns the last limit entries, most recent first -- the
// same records formatHistory prints, in the same order. entries arrives in
// storage order, oldest first. The clamping mirrors formatHistory's, so the
// two can never disagree about what "the last N picks" means.
func newHistoryPayload(entries []HistoryEntry, limit int) historyPayload {
	if limit <= 0 || limit > len(entries) {
		limit = len(entries)
	}
	out := make([]jsonHistoryEntry, 0, limit)
	for i := len(entries) - 1; i >= len(entries)-limit; i-- {
		out = append(out, jsonHistoryEntry{
			Album:     newJSONAlbum(entries[i].Album),
			Timestamp: entries[i].Timestamp,
		})
	}
	return historyPayload{Entries: out, Count: len(out)}
}

// writeJSON emits v as two-space indented JSON with a trailing newline:
// readable without jq installed, and jq normalises anyway.
//
// Nothing here consults the colour mode. An ANSI escape inside a JSON string
// is a parse hazard for no benefit, so --color has no effect on this path.
func writeJSON(w io.Writer, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", data)
	return err
}

// statsPayload is `stats --json`.
//
// count is the described set, after filters; total is the source set before
// them -- the collection, or favorites under --favorites. picked.count and
// picked.share are measured against count, not total, so `stats --genre jazz`
// reports the share of your jazz.
type statsPayload struct {
	Count     int             `json:"count"`
	Total     int             `json:"total"`
	Favorites int             `json:"favorites"`
	SyncedAt  *time.Time      `json:"synced_at"`
	Decades   []jsonDecade    `json:"decades"`
	Genres    []jsonNameCount `json:"genres"`
	Labels    []jsonNameCount `json:"labels"`
	Picked    jsonPicked      `json:"picked"`
}

// jsonDecade is one histogram row. A null decade means the year was unknown,
// matching jsonAlbum's convention that null says "Discogs did not tell us" --
// something 0 cannot, since it would read as the year zero.
type jsonDecade struct {
	Decade *int `json:"decade"`
	Count  int  `json:"count"`
}

// jsonNameCount is one row of the top-genres or top-labels table.
type jsonNameCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// jsonPicked is an object rather than three flat keys so a later figure about
// picking has somewhere to go without crowding the top level.
type jsonPicked struct {
	Count      int        `json:"count"`
	Share      float64    `json:"share"`
	LastPicked *time.Time `json:"last_picked"`
}

// timeOrNull turns a zero time into null, which says "never" in a way a zero
// timestamp string cannot.
func timeOrNull(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func newStatsPayload(s Stats) statsPayload {
	decades := make([]jsonDecade, 0, len(s.Decades))
	for _, b := range s.Decades {
		// intOrNull turns the unknown-year bucket's 0 into null, which is
		// exactly what it means here.
		decades = append(decades, jsonDecade{Decade: intOrNull(b.Decade), Count: b.Count})
	}

	return statsPayload{
		Count:     s.Count,
		Total:     s.Total,
		Favorites: s.Favorites,
		SyncedAt:  timeOrNull(s.SyncedAt),
		Decades:   decades,
		Genres:    nameCounts(s.Genres),
		Labels:    nameCounts(s.Labels),
		Picked: jsonPicked{
			Count:      s.Picked.Count,
			Share:      s.Share(),
			LastPicked: timeOrNull(s.Picked.LastPicked),
		},
	}
}

// nameCounts converts a table to its wire form, emitting [] rather than null
// when empty so a consumer's loop needs no nil check -- the same rule
// listOrEmpty applies to an album's genres.
func nameCounts(rows []NameCount) []jsonNameCount {
	out := make([]jsonNameCount, 0, len(rows))
	for _, r := range rows {
		out = append(out, jsonNameCount{Name: r.Name, Count: r.Count})
	}
	return out
}
