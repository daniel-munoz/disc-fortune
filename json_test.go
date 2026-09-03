package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// The golden tests below pin the exact bytes of the wire format. They are
// what stops it drifting when Album changes: a field added to storage
// without a decision about the output fails here rather than silently
// altering what every script sees.

func TestJSONAlbumGoldenFullyPopulated(t *testing.T) {
	album := Album{
		ReleaseID: 1839278,
		Artist:    "Miles Davis",
		Title:     "Kind of Blue",
		Year:      1959,
		Label:     "Columbia",
		CatNo:     "CL 1355",
		Genres:    []string{"Jazz"},
		Formats:   []string{"Vinyl", "LP", "Album"},
	}

	want := `{
  "release_id": 1839278,
  "artist": "Miles Davis",
  "title": "Kind of Blue",
  "year": 1959,
  "label": "Columbia",
  "catno": "CL 1355",
  "genres": [
    "Jazz"
  ],
  "formats": [
    "Vinyl",
    "LP",
    "Album"
  ]
}
`

	var buf bytes.Buffer
	if err := writeJSON(&buf, newJSONAlbum(album)); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	if got := buf.String(); got != want {
		t.Errorf("wire format drifted.\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// An album Discogs told us almost nothing about still carries all eight
// keys. null says "we were not told", which "" and 0 cannot: "year": 0
// sorts before 1959 and "release_id": 0 looks like an ID.
func TestJSONAlbumGoldenEverythingAbsent(t *testing.T) {
	album := Album{Artist: "Some Artist", Title: "Untitled"}

	want := `{
  "release_id": null,
  "artist": "Some Artist",
  "title": "Untitled",
  "year": null,
  "label": null,
  "catno": null,
  "genres": [],
  "formats": []
}
`

	var buf bytes.Buffer
	if err := writeJSON(&buf, newJSONAlbum(album)); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	if got := buf.String(); got != want {
		t.Errorf("wire format drifted.\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestPickPayloadGolden(t *testing.T) {
	album := Album{ReleaseID: 42, Artist: "A", Title: "B"}

	want := `{
  "album": {
    "release_id": 42,
    "artist": "A",
    "title": "B",
    "year": null,
    "label": null,
    "catno": null,
    "genres": [],
    "formats": []
  }
}
`

	var buf bytes.Buffer
	if err := writeJSON(&buf, pickPayload{Album: newJSONAlbum(album)}); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	if got := buf.String(); got != want {
		t.Errorf("wire format drifted.\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestListPayloadCountsWhatItEmits(t *testing.T) {
	albums := []Album{
		{Artist: "A", Title: "1"},
		{Artist: "B", Title: "2"},
	}
	got := newListPayload(albums)
	if got.Count != 2 {
		t.Errorf("Count = %d, want 2", got.Count)
	}
	if len(got.Albums) != 2 {
		t.Fatalf("Albums has %d entries, want 2", len(got.Albums))
	}
	if got.Albums[0].Artist != "A" || got.Albums[1].Artist != "B" {
		t.Errorf("albums out of order: %q, %q", got.Albums[0].Artist, got.Albums[1].Artist)
	}
}

// An empty list must marshal as [], never null: a consumer's loop should
// need no nil check.
func TestListPayloadEmptyIsAnEmptyArray(t *testing.T) {
	var buf bytes.Buffer
	if err := writeJSON(&buf, newListPayload(nil)); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	want := `{
  "albums": [],
  "count": 0
}
`
	if got := buf.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// Entries come back most recent first, matching what formatHistory prints,
// and count is how many were emitted rather than how many the file holds.
func TestHistoryPayloadIsMostRecentFirst(t *testing.T) {
	base := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{Album: Album{Artist: "oldest", Title: "1"}, Timestamp: base},
		{Album: Album{Artist: "middle", Title: "2"}, Timestamp: base.Add(time.Hour)},
		{Album: Album{Artist: "newest", Title: "3"}, Timestamp: base.Add(2 * time.Hour)},
	}

	got := newHistoryPayload(entries, 2)
	if got.Count != 2 {
		t.Errorf("Count = %d, want 2", got.Count)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("Entries has %d, want 2", len(got.Entries))
	}
	if got.Entries[0].Album.Artist != "newest" {
		t.Errorf("Entries[0] = %q, want newest", got.Entries[0].Album.Artist)
	}
	if got.Entries[1].Album.Artist != "middle" {
		t.Errorf("Entries[1] = %q, want middle", got.Entries[1].Album.Artist)
	}
}

// A limit larger than the history, or zero, means "all of it" -- the same
// clamping formatHistory does.
func TestHistoryPayloadClampsLimit(t *testing.T) {
	base := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{Album: Album{Artist: "a", Title: "1"}, Timestamp: base},
		{Album: Album{Artist: "b", Title: "2"}, Timestamp: base.Add(time.Hour)},
	}

	for _, limit := range []int{0, 2, 99, -1} {
		got := newHistoryPayload(entries, limit)
		if got.Count != 2 {
			t.Errorf("limit %d: Count = %d, want 2", limit, got.Count)
		}
	}
}

func TestHistoryPayloadEmptyIsAnEmptyArray(t *testing.T) {
	var buf bytes.Buffer
	if err := writeJSON(&buf, newHistoryPayload(nil, 0)); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	want := `{
  "entries": [],
  "count": 0
}
`
	if got := buf.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// The timestamp on the wire and the timestamp in history.json must be the
// same string, so no rounding can make them disagree.
func TestHistoryPayloadTimestampIsRFC3339AsStored(t *testing.T) {
	ts := time.Date(2026, 9, 3, 21, 45, 6, 123456789, time.UTC)
	entries := []HistoryEntry{{Album: Album{Artist: "a", Title: "1"}, Timestamp: ts}}

	var buf bytes.Buffer
	if err := writeJSON(&buf, newHistoryPayload(entries, 1)); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	if !strings.Contains(buf.String(), `"2026-09-03T21:45:06.123456789Z"`) {
		t.Errorf("timestamp not RFC 3339 as stored:\n%s", buf.String())
	}
}

// Output is not merely plausible: it parses.
func TestWriteJSONRoundTrips(t *testing.T) {
	albums := []Album{
		{ReleaseID: 1, Artist: "A", Title: "1", Year: 1970, Genres: []string{"Jazz"}},
		{Artist: "B", Title: "2"},
	}

	var buf bytes.Buffer
	if err := writeJSON(&buf, newListPayload(albums)); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}

	var back listPayload
	if err := json.Unmarshal(buf.Bytes(), &back); err != nil {
		t.Fatalf("emitted JSON does not parse: %v\n%s", err, buf.String())
	}
	if back.Count != 2 || len(back.Albums) != 2 {
		t.Errorf("round trip lost data: %+v", back)
	}
	if back.Albums[0].ReleaseID == nil || *back.Albums[0].ReleaseID != 1 {
		t.Errorf("release_id did not survive the round trip: %+v", back.Albums[0])
	}
	if back.Albums[1].ReleaseID != nil {
		t.Errorf("absent release_id should stay null, got %v", *back.Albums[1].ReleaseID)
	}
}

func TestWriteJSONEndsWithExactlyOneNewline(t *testing.T) {
	var buf bytes.Buffer
	if err := writeJSON(&buf, pickPayload{Album: newJSONAlbum(Album{Artist: "A", Title: "B"})}); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	out := buf.String()
	if !strings.HasSuffix(out, "}\n") || strings.HasSuffix(out, "\n\n") {
		t.Errorf("want output ending in exactly one newline, got %q", out)
	}
}
