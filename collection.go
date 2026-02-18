package main

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
)

const (
	configDirPerms      = 0755
	collectionFilePerms = 0644
)

// Album represents a single record with artist and title.
type Album struct {
	Artist string `json:"artist"`
	Title  string `json:"title"`
}

// Key returns a deduplication key for the album.
func (a Album) Key() string {
	return a.Artist + " - " + a.Title
}

func configDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}
	return filepath.Join(home, ".config", "disc-fortune")
}

func collectionPath() string {
	return filepath.Join(configDir(), "collection.json")
}

func loadCollection() ([]Album, error) {
	return loadCollectionFrom(collectionPath())
}

func loadCollectionFrom(path string) ([]Album, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var albums []Album
	if err := json.Unmarshal(data, &albums); err != nil {
		return nil, fmt.Errorf("parsing collection.json: %w", err)
	}
	return albums, nil
}

func saveCollection(albums []Album) error {
	return saveCollectionTo(collectionPath(), albums)
}

func saveCollectionTo(path string, albums []Album) error {
	if err := os.MkdirAll(filepath.Dir(path), configDirPerms); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	data, err := json.MarshalIndent(albums, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding collection: %w", err)
	}
	return os.WriteFile(path, data, collectionFilePerms)
}

func randomAlbum(albums []Album) Album {
	return albums[rand.IntN(len(albums))]
}
