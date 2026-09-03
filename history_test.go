package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAddToHistory(t *testing.T) {
	tmpDir := t.TempDir()
	historyPath := filepath.Join(tmpDir, "history.json")

	album := Album{Artist: "Miles Davis", Title: "Kind of Blue"}
	err := addToHistory(historyPath, album)
	if err != nil {
		t.Fatalf("addToHistory failed: %v", err)
	}

	entries, err := loadHistory(historyPath)
	if err != nil {
		t.Fatalf("loadHistory failed: %v", err)
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

	entries, err := loadHistory(historyPath)
	if err != nil {
		t.Fatalf("loadHistory failed: %v", err)
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
			got := formatTimestamp(tt.ts)
			if !strings.Contains(got, tt.want) && !strings.Contains(got, "/") {
				t.Errorf("formatTimestamp(%v) = %q, want something like %q", tt.ts, got, tt.want)
			}
		})
	}
}

// formatHistory (text) and newHistoryPayload (json.go) deliberately duplicate
// their clamp-and-reverse logic rather than share it, so that the two views
// can never disagree about "the last N picks". Nothing else enforces that
// promise: each is tested against its own expectations, so a divergence
// between the two loop bounds would leave both suites green. This test runs
// both over the same fixture and checks they picked the same records in the
// same order.
func TestFormatHistoryAgreesWithHistoryPayload(t *testing.T) {
	base := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	const n = 5
	entries := make([]HistoryEntry, n)
	for i := 0; i < n; i++ {
		entries[i] = HistoryEntry{
			Album:     Album{Artist: fmt.Sprintf("artist%d", i), Title: fmt.Sprintf("title%d", i)},
			Timestamp: base.Add(time.Duration(i) * time.Hour),
		}
	}

	for _, limit := range []int{0, 1, n - 1, n, n + 1, -1} {
		text := formatHistory(entries, limit, false)
		payload := newHistoryPayload(entries, limit)

		wantHeader := fmt.Sprintf("last %d picks", payload.Count)
		if !strings.Contains(text, wantHeader) {
			t.Errorf("limit %d: formatHistory header does not match payload.Count = %d:\n%s",
				limit, payload.Count, text)
		}

		lastIdx := -1
		for _, e := range payload.Entries {
			idx := strings.Index(text, e.Album.Artist)
			if idx == -1 {
				t.Fatalf("limit %d: formatHistory output missing %q, present in newHistoryPayload:\n%s",
					limit, e.Album.Artist, text)
			}
			if idx <= lastIdx {
				t.Fatalf("limit %d: %q out of order between formatHistory and newHistoryPayload:\n%s",
					limit, e.Album.Artist, text)
			}
			lastIdx = idx
		}
	}
}
