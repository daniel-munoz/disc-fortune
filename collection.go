package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
)

const (
	configDirPerms      = 0755
	collectionFilePerms = 0644
)

// Album represents a single record with metadata.
type Album struct {
	Artist  string   `json:"artist"`
	Title   string   `json:"title"`
	Year    int      `json:"year,omitempty"`
	Label   string   `json:"label,omitempty"`
	CatNo   string   `json:"catno,omitempty"`
	Genres  []string `json:"genres,omitempty"`
	Formats []string `json:"formats,omitempty"`
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
	return writeFileAtomic(path, data, collectionFilePerms)
}

func randomAlbum(albums []Album) Album {
	return albums[rand.IntN(len(albums))]
}

var (
	// errNoCollection means no collection file exists yet.
	errNoCollection = errors.New("no collection")
	// errEmptyCollection means the collection file exists but holds no albums.
	errEmptyCollection = errors.New("collection is empty")
)

// loadCollectionChecked loads the collection and distinguishes the two
// "nothing to work with" states from genuine load failures, so callers can
// print the right guidance without repeating the checks.
func loadCollectionChecked(path string) ([]Album, error) {
	albums, err := loadCollectionFrom(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errNoCollection
		}
		return nil, err
	}
	if len(albums) == 0 {
		return nil, errEmptyCollection
	}
	return albums, nil
}
