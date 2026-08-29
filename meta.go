package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// staleAfter is how long a collection may go unsynced before disc-fortune
// mentions it. Long enough that a regular user never sees the notice, short
// enough to catch a collection that has quietly gone a season out of date.
const staleAfter = 90 * 24 * time.Hour

// Meta holds bookkeeping that is *about* the collection rather than in it. It
// lives in its own file so collection.json stays a plain array of albums --
// which also keeps it out of the way of the schema change T4 has planned.
type Meta struct {
	SyncedAt time.Time `json:"synced_at,omitempty"`
}

func metaPath() string {
	return filepath.Join(configDir(), "meta.json")
}

// loadMeta reads meta.json. A missing file is not an error: it means nothing
// has been recorded yet, which is what a fresh install looks like.
func loadMeta(path string) (Meta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Meta{}, nil
		}
		return Meta{}, err
	}
	var m Meta
	if err := json.Unmarshal(data, &m); err != nil {
		return Meta{}, fmt.Errorf("parsing meta.json: %w", err)
	}
	return m, nil
}

// saveMeta writes meta.json atomically.
func saveMeta(path string, m Meta) error {
	if err := os.MkdirAll(filepath.Dir(path), configDirPerms); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding meta: %w", err)
	}
	return writeFileAtomic(path, data, collectionFilePerms)
}

// recordSync stamps the time of a completed sync, preserving whatever other
// metadata is already on disk. Unreadable metadata is discarded rather than
// propagated: the timestamp is advisory, and a sync that fetched an entire
// collection successfully must not fail over it.
func recordSync(path string, at time.Time) error {
	m, err := loadMeta(path)
	if err != nil {
		m = Meta{}
	}
	m.SyncedAt = at
	return saveMeta(path, m)
}

// staleNotice returns a nudge when the collection has not been synced in a
// while, or "" when it is fresh or has never been synced at all. Someone who
// has never synced is already told so by the "No collection found" path;
// nagging them here would only be noise.
func staleNotice(m Meta, now time.Time) string {
	if m.SyncedAt.IsZero() {
		return ""
	}
	if now.Sub(m.SyncedAt) < staleAfter {
		return ""
	}
	return fmt.Sprintf("Your collection was last synced %s. Run `disc-fortune sync` to refresh it.\n",
		formatTimestamp(m.SyncedAt))
}

// syncNotice returns the staleness nudge for the metadata at path, or "" when
// notices are disabled or the metadata cannot be read. Advisory output is
// never allowed to break the command it is decorating.
func syncNotice(path string, now time.Time, enabled bool) string {
	if !enabled {
		return ""
	}
	m, err := loadMeta(path)
	if err != nil {
		return ""
	}
	return staleNotice(m, now)
}
