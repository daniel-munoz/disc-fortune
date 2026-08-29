package main

import (
	"bytes"
	"os"
	"path/filepath"
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

func TestBackfillSummary(t *testing.T) {
	tests := []struct {
		name string
		fav  backfillResult
		hist backfillResult
		want string
	}{
		{
			name: "nothing to say",
			want: "",
		},
		{
			name: "both files",
			fav:  backfillResult{Updated: 12},
			hist: backfillResult{Updated: 106},
			want: "Filled in release IDs for 12 favorites and 106 history entries.\n",
		},
		{
			name: "favorites only",
			fav:  backfillResult{Updated: 12},
			want: "Filled in release IDs for 12 favorites.\n",
		},
		{
			name: "history only",
			hist: backfillResult{Updated: 106},
			want: "Filled in release IDs for 106 history entries.\n",
		},
		{
			name: "singular",
			fav:  backfillResult{Updated: 1},
			hist: backfillResult{Updated: 1},
			want: "Filled in release IDs for 1 favorite and 1 history entry.\n",
		},
		{
			name: "ambiguous favorites are listed",
			fav:  backfillResult{Ambiguous: []string{"Miles Davis - Kind of Blue"}},
			want: "These favorites matched more than one record and were left as-is:\n" +
				"  Miles Davis - Kind of Blue\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := backfillSummary(tt.fav, tt.hist); got != tt.want {
				t.Errorf("backfillSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

// History ambiguity is deliberately silent: a log has nothing to act on.
func TestBackfillSummaryIgnoresHistoryAmbiguity(t *testing.T) {
	hist := backfillResult{Ambiguous: []string{"Miles Davis - Kind of Blue"}}
	if got := backfillSummary(backfillResult{}, hist); got != "" {
		t.Errorf("backfillSummary() = %q, want empty", got)
	}
}

func TestRunBackfillWritesBothFiles(t *testing.T) {
	dir := t.TempDir()
	favPath := filepath.Join(dir, "favorites.json")
	histPath := filepath.Join(dir, "history.json")

	if err := saveFavorites(favPath, []Album{{Artist: "Slowdive", Title: "Souvlaki"}}); err != nil {
		t.Fatalf("saveFavorites: %v", err)
	}
	if err := saveHistory(histPath, []HistoryEntry{
		{Album: Album{Artist: "Slowdive", Title: "Souvlaki"}, Timestamp: time.Now()},
	}); err != nil {
		t.Fatalf("saveHistory: %v", err)
	}

	report, err := runBackfill(favPath, histPath, testCollection())
	if err != nil {
		t.Fatalf("runBackfill: %v", err)
	}
	if report != "Filled in release IDs for 1 favorite and 1 history entry.\n" {
		t.Errorf("report = %q", report)
	}

	favs, err := loadFavorites(favPath)
	if err != nil {
		t.Fatalf("loadFavorites: %v", err)
	}
	if favs[0].ReleaseID != 111 {
		t.Errorf("favorite ReleaseID = %d, want 111", favs[0].ReleaseID)
	}

	entries, err := loadHistory(histPath)
	if err != nil {
		t.Fatalf("loadHistory: %v", err)
	}
	if entries[0].Album.ReleaseID != 111 {
		t.Errorf("history ReleaseID = %d, want 111", entries[0].Album.ReleaseID)
	}
}

// The acceptance criterion: running sync twice leaves the files
// byte-identical, and the second run reports nothing.
func TestRunBackfillIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	favPath := filepath.Join(dir, "favorites.json")
	histPath := filepath.Join(dir, "history.json")

	if err := saveFavorites(favPath, []Album{{Artist: "Slowdive", Title: "Souvlaki"}}); err != nil {
		t.Fatalf("saveFavorites: %v", err)
	}
	if err := saveHistory(histPath, []HistoryEntry{
		{Album: Album{Artist: "Slowdive", Title: "Souvlaki"}, Timestamp: time.Now()},
	}); err != nil {
		t.Fatalf("saveHistory: %v", err)
	}

	if _, err := runBackfill(favPath, histPath, testCollection()); err != nil {
		t.Fatalf("first runBackfill: %v", err)
	}
	favBefore, err := os.ReadFile(favPath)
	if err != nil {
		t.Fatalf("read favorites: %v", err)
	}
	histBefore, err := os.ReadFile(histPath)
	if err != nil {
		t.Fatalf("read history: %v", err)
	}

	report, err := runBackfill(favPath, histPath, testCollection())
	if err != nil {
		t.Fatalf("second runBackfill: %v", err)
	}
	if report != "" {
		t.Errorf("second run reported %q, want nothing", report)
	}

	favAfter, err := os.ReadFile(favPath)
	if err != nil {
		t.Fatalf("read favorites: %v", err)
	}
	histAfter, err := os.ReadFile(histPath)
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	if !bytes.Equal(favBefore, favAfter) {
		t.Error("favorites.json changed on the second pass")
	}
	if !bytes.Equal(histBefore, histAfter) {
		t.Error("history.json changed on the second pass")
	}
}

// A user who has never favorited anything must not have empty files
// created for them.
func TestRunBackfillLeavesAbsentFilesAlone(t *testing.T) {
	dir := t.TempDir()
	favPath := filepath.Join(dir, "favorites.json")
	histPath := filepath.Join(dir, "history.json")

	report, err := runBackfill(favPath, histPath, testCollection())
	if err != nil {
		t.Fatalf("runBackfill: %v", err)
	}
	if report != "" {
		t.Errorf("report = %q, want nothing", report)
	}
	if _, err := os.Stat(favPath); !os.IsNotExist(err) {
		t.Error("favorites.json was created")
	}
	if _, err := os.Stat(histPath); !os.IsNotExist(err) {
		t.Error("history.json was created")
	}
}
