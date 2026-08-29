package main

import (
	"reflect"
	"testing"
	"time"
)

// testCollection is the shared fixture: one unambiguous record, and one
// title that resolves to two distinct pressings.
func testCollection() []Album {
	return []Album{
		{ReleaseID: 111, Artist: "Slowdive", Title: "Souvlaki"},
		{ReleaseID: 222, Artist: "Miles Davis", Title: "Kind of Blue"},
		{ReleaseID: 333, Artist: "Miles Davis", Title: "Kind of Blue"},
	}
}

func TestBackfillAlbumsUniqueMatch(t *testing.T) {
	entries := []Album{{Artist: "Slowdive", Title: "Souvlaki"}}

	got, res := backfillAlbums(entries, testCollection())

	if res.Updated != 1 {
		t.Errorf("Updated = %d, want 1", res.Updated)
	}
	if got[0].ReleaseID != 111 {
		t.Errorf("ReleaseID = %d, want 111", got[0].ReleaseID)
	}
	if len(res.Ambiguous) != 0 {
		t.Errorf("Ambiguous = %v, want none", res.Ambiguous)
	}
}

// A record no longer in the collection -- sold, or dropped from the synced
// folders -- is left alone and reported nowhere. There is nothing to do.
func TestBackfillAlbumsNoMatch(t *testing.T) {
	entries := []Album{{Artist: "Ride", Title: "Nowhere"}}

	got, res := backfillAlbums(entries, testCollection())

	if res.Updated != 0 {
		t.Errorf("Updated = %d, want 0", res.Updated)
	}
	if got[0].ReleaseID != 0 {
		t.Errorf("ReleaseID = %d, want 0 (left alone)", got[0].ReleaseID)
	}
	if len(res.Ambiguous) != 0 {
		t.Errorf("Ambiguous = %v, want none", res.Ambiguous)
	}
}

// Which pressing the user favorited is unknowable, so guess nothing and say
// so instead.
func TestBackfillAlbumsAmbiguous(t *testing.T) {
	entries := []Album{{Artist: "Miles Davis", Title: "Kind of Blue"}}

	got, res := backfillAlbums(entries, testCollection())

	if res.Updated != 0 {
		t.Errorf("Updated = %d, want 0", res.Updated)
	}
	if got[0].ReleaseID != 0 {
		t.Errorf("ReleaseID = %d, want 0 (left alone)", got[0].ReleaseID)
	}
	want := []string{"Miles Davis - Kind of Blue"}
	if !reflect.DeepEqual(res.Ambiguous, want) {
		t.Errorf("Ambiguous = %v, want %v", res.Ambiguous, want)
	}
}

// An ambiguous key repeated across many entries -- easy in history -- is
// reported once, not once per entry.
func TestBackfillAlbumsAmbiguousReportedOnce(t *testing.T) {
	entries := []Album{
		{Artist: "Miles Davis", Title: "Kind of Blue"},
		{Artist: "Miles Davis", Title: "Kind of Blue"},
	}

	_, res := backfillAlbums(entries, testCollection())

	if len(res.Ambiguous) != 1 {
		t.Errorf("Ambiguous = %v, want one entry", res.Ambiguous)
	}
}

func TestBackfillAlbumsSkipsEntriesThatHaveAnID(t *testing.T) {
	// 999 is not in the collection: if the pass touched entries that already
	// have an ID, this would be overwritten or reported.
	entries := []Album{{ReleaseID: 999, Artist: "Slowdive", Title: "Souvlaki"}}

	got, res := backfillAlbums(entries, testCollection())

	if res.Updated != 0 {
		t.Errorf("Updated = %d, want 0", res.Updated)
	}
	if got[0].ReleaseID != 999 {
		t.Errorf("ReleaseID = %d, want 999 (untouched)", got[0].ReleaseID)
	}
}

// Idempotence is the acceptance criterion for the migration: a second sync
// must change nothing.
func TestBackfillAlbumsIsIdempotent(t *testing.T) {
	entries := []Album{
		{Artist: "Slowdive", Title: "Souvlaki"},
		{Artist: "Miles Davis", Title: "Kind of Blue"},
		{Artist: "Ride", Title: "Nowhere"},
	}
	collection := testCollection()

	once, firstRes := backfillAlbums(entries, collection)
	twice, secondRes := backfillAlbums(once, collection)

	if !reflect.DeepEqual(once, twice) {
		t.Errorf("second pass changed the entries:\n once: %+v\ntwice: %+v", once, twice)
	}
	if firstRes.Updated != 1 {
		t.Errorf("first pass Updated = %d, want 1", firstRes.Updated)
	}
	if secondRes.Updated != 0 {
		t.Errorf("second pass Updated = %d, want 0", secondRes.Updated)
	}
}

func TestBackfillHistory(t *testing.T) {
	when := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{Album: Album{Artist: "Slowdive", Title: "Souvlaki"}, Timestamp: when},
		{Album: Album{Artist: "Ride", Title: "Nowhere"}, Timestamp: when},
	}

	got, res := backfillHistory(entries, testCollection())

	if res.Updated != 1 {
		t.Errorf("Updated = %d, want 1", res.Updated)
	}
	if got[0].Album.ReleaseID != 111 {
		t.Errorf("ReleaseID = %d, want 111", got[0].Album.ReleaseID)
	}
	if got[1].Album.ReleaseID != 0 {
		t.Errorf("unmatched entry got ReleaseID %d, want 0", got[1].Album.ReleaseID)
	}
	if !got[0].Timestamp.Equal(when) {
		t.Errorf("Timestamp = %v, want %v", got[0].Timestamp, when)
	}
}

func TestIndexByLegacyKeyCollapsesRepeatedIDs(t *testing.T) {
	// The same release listed twice must not read as two candidates.
	collection := []Album{
		{ReleaseID: 111, Artist: "Slowdive", Title: "Souvlaki"},
		{ReleaseID: 111, Artist: "Slowdive", Title: "Souvlaki"},
	}

	idx := indexByLegacyKey(collection)

	if got := idx["Slowdive - Souvlaki"]; len(got) != 1 || got[0] != 111 {
		t.Errorf("index = %v, want [111]", got)
	}
}
