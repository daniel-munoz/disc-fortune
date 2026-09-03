package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// HistoryEntry represents a single pick with timestamp.
type HistoryEntry struct {
	Album     Album     `json:"album"`
	Timestamp time.Time `json:"timestamp"`
}

func historyPath() string {
	return filepath.Join(configDir(), "history.json")
}

// loadHistory loads history entries from disk.
func loadHistory(path string) ([]HistoryEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []HistoryEntry{}, nil
		}
		return nil, err
	}
	var entries []HistoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parsing history.json: %w", err)
	}
	return entries, nil
}

// saveHistory saves history entries to disk.
func saveHistory(path string, entries []HistoryEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), configDirPerms); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding history: %w", err)
	}
	return writeFileAtomic(path, data, collectionFilePerms)
}

// addToHistory appends an album to history.
//
// The whole load-append-save runs under the file lock: `sync`'s backfill
// rewrites this same file, and without the lock one of the two writes is lost.
func addToHistory(path string, album Album) error {
	return withFileLock(path, func() error {
		entries, err := loadHistory(path)
		if err != nil {
			return err
		}
		entries = append(entries, HistoryEntry{
			Album:     album,
			Timestamp: time.Now(),
		})
		return saveHistory(path, entries)
	})
}

// formatTimestamp formats a timestamp as relative time or date.
func formatTimestamp(ts time.Time) string {
	now := time.Now()
	diff := now.Sub(ts)

	switch {
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins < 1 {
			return "just now"
		}
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case diff < 48*time.Hour:
		return "yesterday"
	case diff < 7*24*time.Hour:
		days := int(diff.Hours() / 24)
		return fmt.Sprintf("%d days ago", days)
	default:
		return ts.Format("2006-01-02")
	}
}

// formatHistory formats history entries for display.
func formatHistory(entries []HistoryEntry, limit int, useColor bool) string {
	if len(entries) == 0 {
		return "No history yet\n"
	}

	// newHistoryPayload (json.go) mirrors this clamp and reverse. Change both
	// together, or the --json and text views will disagree about "the last N
	// picks".
	if limit <= 0 || limit > len(entries) {
		limit = len(entries)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("History (last %d picks):\n", limit))

	// Reverse order (most recent first)
	for i := len(entries) - 1; i >= len(entries)-limit; i-- {
		entry := entries[i]
		idx := len(entries) - i

		sb.WriteString(fmt.Sprintf("  %d. %s: ", idx, formatTimestamp(entry.Timestamp)))

		if useColor {
			sb.WriteString(colorBoldCyan)
		}
		sb.WriteString(entry.Album.Artist)
		if useColor {
			sb.WriteString(colorReset)
		}
		sb.WriteString(" - ")
		if useColor {
			sb.WriteString(colorBoldWhite)
		}
		sb.WriteString(entry.Album.Title)
		if useColor {
			sb.WriteString(colorReset)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
