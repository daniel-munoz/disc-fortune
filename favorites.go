package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var (
	// ErrAlreadyInFavorites is returned when trying to add an album that's already favorited.
	ErrAlreadyInFavorites = errors.New("already in favorites")
	// ErrNotInFavorites is returned when trying to remove an album that's not favorited.
	ErrNotInFavorites = errors.New("not in favorites")
)

func favoritesPath() string {
	return filepath.Join(configDir(), "favorites.json")
}

// loadFavorites loads favorite albums from disk.
func loadFavorites(path string) ([]Album, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Album{}, nil
		}
		return nil, err
	}
	var albums []Album
	if err := json.Unmarshal(data, &albums); err != nil {
		return nil, fmt.Errorf("parsing favorites.json: %w", err)
	}
	return albums, nil
}

// saveFavorites saves favorite albums to disk.
func saveFavorites(path string, albums []Album) error {
	if err := os.MkdirAll(filepath.Dir(path), configDirPerms); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	data, err := json.MarshalIndent(albums, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding favorites: %w", err)
	}
	return os.WriteFile(path, data, collectionFilePerms)
}

// addFavorite adds an album to favorites if not already present.
func addFavorite(path string, album Album) error {
	favorites, err := loadFavorites(path)
	if err != nil {
		return err
	}

	// Check for duplicates
	key := album.Key()
	for _, fav := range favorites {
		if fav.Key() == key {
			return ErrAlreadyInFavorites
		}
	}

	favorites = append(favorites, album)
	return saveFavorites(path, favorites)
}

// removeFavorite removes an album from favorites.
func removeFavorite(path string, album Album) error {
	favorites, err := loadFavorites(path)
	if err != nil {
		return err
	}

	key := album.Key()
	var filtered []Album
	found := false
	for _, fav := range favorites {
		if fav.Key() == key {
			found = true
			continue
		}
		filtered = append(filtered, fav)
	}

	if !found {
		return ErrNotInFavorites
	}

	return saveFavorites(path, filtered)
}
