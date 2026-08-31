package main

import (
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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

// Concurrent appends must not lose entries. This is the race the roadmap
// parked for this phase: it stopped being cosmetic when history started
// deciding what pick avoids.
func TestAddToHistoryConcurrentAppendsDoNotLose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")

	const writers = 8
	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			album := Album{ReleaseID: i + 1, Artist: "A", Title: strconv.Itoa(i + 1)}
			if err := addToHistory(path, album); err != nil {
				t.Errorf("addToHistory: %v", err)
			}
		}()
	}
	wg.Wait()

	entries, err := loadHistory(path)
	if err != nil {
		t.Fatalf("loadHistory: %v", err)
	}
	if len(entries) != writers {
		t.Errorf("history has %d entries, want %d", len(entries), writers)
	}
}
