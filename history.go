package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	return os.WriteFile(path, data, collectionFilePerms)
}

// addToHistory appends an album to history.
func addToHistory(path string, album Album) error {
	entries, err := loadHistory(path)
	if err != nil {
		return err
	}
	entries = append(entries, HistoryEntry{
		Album:     album,
		Timestamp: time.Now(),
	})
	return saveHistory(path, entries)
}
