package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

const (
	configDirPerms      = 0755
	collectionFilePerms = 0644
)

// Album represents a single record with metadata.
type Album struct {
	// ReleaseID is the Discogs release ID. It is zero for entries written
	// before v2.2.0, which is what Identity and sameAlbum fall back for.
	ReleaseID int      `json:"release_id,omitempty"`
	Artist    string   `json:"artist"`
	Title     string   `json:"title"`
	Year      int      `json:"year,omitempty"`
	Label     string   `json:"label,omitempty"`
	CatNo     string   `json:"catno,omitempty"`
	Genres    []string `json:"genres,omitempty"`
	Formats   []string `json:"formats,omitempty"`
}

// Key returns the human-readable "Artist - Title" label. It is also the
// legacy identity: it is what --query substring-matches against, and what
// identifies entries written before release IDs existed. It deliberately
// ignores ReleaseID -- an ID-preferring Key would break every query.
func (a Album) Key() string {
	return a.Artist + " - " + a.Title
}

// Identity returns a map key that distinguishes two records. Sync
// deduplication is its only caller, and there every album comes straight
// from the API, so the ID is always present. The "id:"/"name:" prefixes keep
// a numeric-looking artist name from ever colliding with an ID.
func (a Album) Identity() string {
	if a.ReleaseID != 0 {
		return "id:" + strconv.Itoa(a.ReleaseID)
	}
	return "name:" + a.Key()
}

// sameAlbum reports whether two entries are the same record. It is
// deliberately lenient when either side predates the release ID: a pre-2.2
// favorite and that same record freshly synced must not look like two
// different albums, or favoriting it again would append a duplicate.
//
// The consequence is that sameAlbum is not transitive -- an entry with no ID
// acts as a wildcard for its name. That is fine inside a linear "is this
// already in the list?" scan, and it is exactly why Identity, not sameAlbum,
// is what sync dedup uses: a non-transitive comparison there would make the
// surviving set depend on fetch order.
func sameAlbum(a, b Album) bool {
	if a.ReleaseID != 0 && b.ReleaseID != 0 {
		return a.ReleaseID == b.ReleaseID
	}
	return a.Key() == b.Key()
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
