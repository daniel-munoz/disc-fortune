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
