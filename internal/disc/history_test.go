package disc

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAddToHistory(t *testing.T) {
	tmpDir := t.TempDir()
	historyPath := filepath.Join(tmpDir, "history.json")

	album := Album{Artist: "Miles Davis", Title: "Kind of Blue"}
	err := AddToHistory(historyPath, album)
	if err != nil {
		t.Fatalf("AddToHistory failed: %v", err)
	}

	entries, err := LoadHistory(historyPath)
	if err != nil {
		t.Fatalf("LoadHistory failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Album.Artist != "Miles Davis" {
		t.Errorf("Artist = %q, want Miles Davis", entries[0].Album.Artist)
	}
	if entries[0].Timestamp.IsZero() {
		t.Error("Timestamp is zero")
	}
}

func TestLoadHistoryEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	historyPath := filepath.Join(tmpDir, "nonexistent.json")

	entries, err := LoadHistory(historyPath)
	if err != nil {
		t.Fatalf("LoadHistory failed: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0", len(entries))
	}
}

func TestFormatTimestamp(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name string
		ts   time.Time
		want string
	}{
		{"2 hours ago", now.Add(-2 * time.Hour), "2 hours ago"},
		{"yesterday", now.Add(-25 * time.Hour), "yesterday"},
		{"2 days ago", now.Add(-48 * time.Hour), "2 days ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatTimestamp(tt.ts)
			if !strings.Contains(got, tt.want) && !strings.Contains(got, "/") {
				t.Errorf("FormatTimestamp(%v) = %q, want something like %q", tt.ts, got, tt.want)
			}
		})
	}
}

func TestReplaceLastHistoryReplacesTheLastEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")

	first := Album{Artist: "Alice Coltrane", Title: "Journey in Satchidananda"}
	second := Album{Artist: "Miles Davis", Title: "Kind of Blue"}
	for _, album := range []Album{first, second} {
		if err := AddToHistory(path, album); err != nil {
			t.Fatalf("AddToHistory(%q): %v", album.Title, err)
		}
	}
	entries, err := LoadHistory(path)
	if err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}

	replacement := Album{Artist: "Sun Ra", Title: "Space Is the Place"}
	if err := ReplaceLastHistory(path, entries[len(entries)-1], replacement); err != nil {
		t.Fatalf("ReplaceLastHistory: %v", err)
	}

	got, err := LoadHistory(path)
	if err != nil {
		t.Fatalf("LoadHistory after replace: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2 -- a replace must not change the count", len(got))
	}
	if got[0].Album.Title != first.Title {
		t.Errorf("entry 0 = %q, want %q -- earlier entries must be untouched",
			got[0].Album.Title, first.Title)
	}
	if got[1].Album.Title != replacement.Title {
		t.Errorf("entry 1 = %q, want %q", got[1].Album.Title, replacement.Title)
	}
}

// The replacement is a new pick, made now, so it carries its own timestamp
// rather than inheriting the dropped entry's. Written directly with an old
// timestamp so "now" and "inherited" cannot be confused.
func TestReplaceLastHistoryStampsTheReplacementWithNow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")

	old := HistoryEntry{
		Album:     Album{Artist: "Miles Davis", Title: "Kind of Blue"},
		Timestamp: time.Now().Add(-365 * 24 * time.Hour),
	}
	if err := SaveHistory(path, []HistoryEntry{old}); err != nil {
		t.Fatalf("SaveHistory: %v", err)
	}
	entries, err := LoadHistory(path)
	if err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}

	if err := ReplaceLastHistory(path, entries[0], Album{Artist: "Sun Ra", Title: "Lanquidity"}); err != nil {
		t.Fatalf("ReplaceLastHistory: %v", err)
	}

	got, err := LoadHistory(path)
	if err != nil {
		t.Fatalf("LoadHistory after replace: %v", err)
	}
	if since := time.Since(got[0].Timestamp); since > time.Minute {
		t.Errorf("replacement timestamp is %v old; it should be stamped with now, not inherited", since)
	}
}

// The caller decided from a history it read without the lock. If another
// process appended in between, the last entry is no longer the one the caller
// was shown, and replacing it would delete the wrong record.
func TestReplaceLastHistoryRefusesWhenTheLastEntryMoved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")

	if err := AddToHistory(path, Album{Artist: "Miles Davis", Title: "Kind of Blue"}); err != nil {
		t.Fatalf("AddToHistory: %v", err)
	}
	entries, err := LoadHistory(path)
	if err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}
	stale := entries[len(entries)-1]

	// Another process picks while we were deciding.
	if err := AddToHistory(path, Album{Artist: "Sun Ra", Title: "Space Is the Place"}); err != nil {
		t.Fatalf("AddToHistory (concurrent): %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading history: %v", err)
	}

	err = ReplaceLastHistory(path, stale, Album{Artist: "Pharoah Sanders", Title: "Karma"})
	if !errors.Is(err, ErrHistoryChanged) {
		t.Fatalf("got %v, want ErrHistoryChanged", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("re-reading history: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("history.json changed on a refused replace:\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestReplaceLastHistoryRefusesOnEmptyHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")

	err := ReplaceLastHistory(path, HistoryEntry{}, Album{Artist: "Sun Ra", Title: "Lanquidity"})
	if !errors.Is(err, ErrHistoryChanged) {
		t.Fatalf("got %v, want ErrHistoryChanged", err)
	}
	// withFileLock creates a .lock sidecar, which is expected and permanent.
	// history.json itself must not be created.
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Errorf("a refused replace must not create history.json (stat: %v)", statErr)
	}
}
