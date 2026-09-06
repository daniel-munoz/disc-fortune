package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/daniel-munoz/disc-fortune/v2/internal/disc"
)

// The golden tests below pin the exact bytes of the wire format. They catch
// drift within jsonAlbum -- rename a key, reorder a field, change what null
// means -- immediately. They do NOT catch a field added to Album with no
// jsonAlbum counterpart: that omission never touches these bytes, so the
// suite would stay green. TestEveryAlbumFieldHasAWireDecision, below, is what
// catches that.

func TestJSONAlbumGoldenFullyPopulated(t *testing.T) {
	album := disc.Album{
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
	album := disc.Album{Artist: "Some Artist", Title: "Untitled"}

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
	album := disc.Album{ReleaseID: 42, Artist: "A", Title: "B"}

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
	albums := []disc.Album{
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

// Entries come back most recent first, matching what disc.FormatHistory prints,
// and count is how many were emitted rather than how many the file holds.
func TestHistoryPayloadIsMostRecentFirst(t *testing.T) {
	base := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	entries := []disc.HistoryEntry{
		{Album: disc.Album{Artist: "oldest", Title: "1"}, Timestamp: base},
		{Album: disc.Album{Artist: "middle", Title: "2"}, Timestamp: base.Add(time.Hour)},
		{Album: disc.Album{Artist: "newest", Title: "3"}, Timestamp: base.Add(2 * time.Hour)},
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
// clamping disc.FormatHistory does.
func TestHistoryPayloadClampsLimit(t *testing.T) {
	base := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	entries := []disc.HistoryEntry{
		{Album: disc.Album{Artist: "a", Title: "1"}, Timestamp: base},
		{Album: disc.Album{Artist: "b", Title: "2"}, Timestamp: base.Add(time.Hour)},
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
//
// disc.AddToHistory stamps with time.Now(), which is local, so real output
// is not always the UTC "Z" form these fixtures might suggest: it carries
// whatever UTC offset the machine was in, and Go prints fractional seconds at
// whatever length they actually have -- trailing zeros dropped, and no dot at
// all when there is no fraction. Both cases are exercised here so a fixed
// UTC assumption cannot creep back in undetected.
func TestHistoryPayloadTimestampIsRFC3339AsStored(t *testing.T) {
	tests := []struct {
		name string
		ts   time.Time
		want string
	}{
		{
			name: "UTC",
			ts:   time.Date(2026, 9, 3, 21, 45, 6, 123456789, time.UTC),
			want: `"2026-09-03T21:45:06.123456789Z"`,
		},
		{
			// A local offset, and a fractional-second length (6 digits, no
			// trailing zero) matching what disc.AddToHistory actually produces.
			name: "local offset with variable-length fraction",
			ts:   time.Date(2026, 9, 3, 18, 58, 59, 262156000, time.FixedZone("", -4*60*60)),
			want: `"2026-09-03T18:58:59.262156-04:00"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries := []disc.HistoryEntry{{Album: disc.Album{Artist: "a", Title: "1"}, Timestamp: tt.ts}}

			var buf bytes.Buffer
			if err := writeJSON(&buf, newHistoryPayload(entries, 1)); err != nil {
				t.Fatalf("writeJSON: %v", err)
			}
			if !strings.Contains(buf.String(), tt.want) {
				t.Errorf("timestamp not RFC 3339 as stored: want %s in:\n%s", tt.want, buf.String())
			}
		})
	}
}

// Output is not merely plausible: it parses.
func TestWriteJSONRoundTrips(t *testing.T) {
	albums := []disc.Album{
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
	if err := writeJSON(&buf, pickPayload{Album: newJSONAlbum(disc.Album{Artist: "A", Title: "B"})}); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	out := buf.String()
	if !strings.HasSuffix(out, "}\n") || strings.HasSuffix(out, "\n\n") {
		t.Errorf("want output ending in exactly one newline, got %q", out)
	}
}

// Album and jsonAlbum are hand-kept in step. A field added to storage must be
// a conscious decision about the wire format, not a silent omission: this test
// is the forcing function the golden tests cannot be.
func TestEveryAlbumFieldHasAWireDecision(t *testing.T) {
	storage := reflect.TypeOf(disc.Album{})
	wire := reflect.TypeOf(jsonAlbum{})
	if storage.NumField() != wire.NumField() {
		t.Fatalf("Album has %d fields, jsonAlbum has %d -- decide what the new "+
			"field does on the wire, then update this test",
			storage.NumField(), wire.NumField())
	}
	for i := range storage.NumField() {
		s, w := storage.Field(i), wire.Field(i)
		if s.Name != w.Name {
			t.Errorf("field %d: Album.%s vs jsonAlbum.%s -- order must match so "+
				"the golden key order is traceable to storage", i, s.Name, w.Name)
		}
	}
}

func TestStatsPayloadGolden(t *testing.T) {
	s := Stats{
		Count:     312,
		Total:     1247,
		Favorites: 28,
		SyncedAt:  time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
		Decades:   []DecadeBucket{{1970, 486}, {0, 22}},
		Genres:    []NameCount{{"Jazz", 412}},
		Labels:    []NameCount{{"Blue Note", 88}},
		Picked: PickedStats{
			Count:      78,
			LastPicked: time.Date(2026, 9, 4, 18, 0, 0, 0, time.UTC),
		},
	}

	want := `{
  "count": 312,
  "total": 1247,
  "favorites": 28,
  "synced_at": "2026-09-01T10:00:00Z",
  "decades": [
    {
      "decade": 1970,
      "count": 486
    },
    {
      "decade": null,
      "count": 22
    }
  ],
  "genres": [
    {
      "name": "Jazz",
      "count": 412
    }
  ],
  "labels": [
    {
      "name": "Blue Note",
      "count": 88
    }
  ],
  "picked": {
    "count": 78,
    "share": 0.25,
    "last_picked": "2026-09-04T18:00:00Z"
  }
}
`

	var buf bytes.Buffer
	if err := writeJSON(&buf, newStatsPayload(s)); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	if got := buf.String(); got != want {
		t.Errorf("stats wire format drifted.\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// An empty collection still carries every key, with [] rather than null for
// the tables and null for the two timestamps.
func TestStatsPayloadGoldenEmpty(t *testing.T) {
	want := `{
  "count": 0,
  "total": 0,
  "favorites": 0,
  "synced_at": null,
  "decades": [],
  "genres": [],
  "labels": [],
  "picked": {
    "count": 0,
    "share": 0,
    "last_picked": null
  }
}
`

	var buf bytes.Buffer
	if err := writeJSON(&buf, newStatsPayload(Stats{})); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	if got := buf.String(); got != want {
		t.Errorf("empty stats wire format drifted.\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// share is measured against the described set, not the source set. This is
// the figure a scripted consumer is most likely to get wrong, so it is
// pinned on its own.
func TestStatsPayloadShareIsAgainstCount(t *testing.T) {
	p := newStatsPayload(Stats{Count: 200, Total: 1000, Picked: PickedStats{Count: 50}})
	if p.Picked.Share != 0.25 {
		t.Errorf("share = %v, want 0.25 (50/200, not 50/1000)", p.Picked.Share)
	}
}

// FormatHistory (text) and newHistoryPayload (json.go) deliberately duplicate
// their clamp-and-reverse logic rather than share it, so that the two views
// can never disagree about "the last N picks". Nothing else enforces that
// promise: each is tested against its own expectations, so a divergence
// between the two loop bounds would leave both suites green. This test runs
// both over the same fixture and checks they picked the same records in the
// same order.
func TestFormatHistoryAgreesWithHistoryPayload(t *testing.T) {
	base := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	const n = 5
	entries := make([]disc.HistoryEntry, n)
	for i := 0; i < n; i++ {
		entries[i] = disc.HistoryEntry{
			Album:     disc.Album{Artist: fmt.Sprintf("artist%d", i), Title: fmt.Sprintf("title%d", i)},
			Timestamp: base.Add(time.Duration(i) * time.Hour),
		}
	}

	for _, limit := range []int{0, 1, n - 1, n, n + 1, -1} {
		text := disc.FormatHistory(entries, limit, false)
		payload := newHistoryPayload(entries, limit)

		wantHeader := fmt.Sprintf("last %d picks", payload.Count)
		if !strings.Contains(text, wantHeader) {
			t.Errorf("limit %d: FormatHistory header does not match payload.Count = %d:\n%s",
				limit, payload.Count, text)
		}

		lastIdx := -1
		for _, e := range payload.Entries {
			idx := strings.Index(text, e.Album.Artist)
			if idx == -1 {
				t.Fatalf("limit %d: FormatHistory output missing %q, present in newHistoryPayload:\n%s",
					limit, e.Album.Artist, text)
			}
			if idx <= lastIdx {
				t.Fatalf("limit %d: %q out of order between FormatHistory and newHistoryPayload:\n%s",
					limit, e.Album.Artist, text)
			}
			lastIdx = idx
		}
	}
}
