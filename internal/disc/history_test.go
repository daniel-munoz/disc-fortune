package disc

import (
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
