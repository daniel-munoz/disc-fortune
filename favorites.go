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
	return writeFileAtomic(path, data, collectionFilePerms)
}

// addFavorite adds an album to favorites if not already present.
//
// Locked for the same reason as addToHistory: `sync`'s backfill rewrites
// favorites.json wholesale, and without the lock one of the two writes is lost.
func addFavorite(path string, album Album) error {
	return withFileLock(path, func() error {
		favorites, err := loadFavorites(path)
		if err != nil {
			return err
		}

		// sameAlbum rather than Key: two pressings of one title are two
		// favorites, but an entry written before release IDs existed is still
		// the same record as its freshly synced self.
		for i, fav := range favorites {
			if !sameAlbum(fav, album) {
				continue
			}

			// Replace a stored entry that predates release IDs with the one the
			// user just named, so naming a specific pressing actually resolves
			// an ambiguous favorite instead of reporting it forever. This is
			// safe rather than a guess: an un-ID'd favorite that can still be
			// re-favorited from the collection is necessarily an ambiguous one,
			// because a unique match would already have been stamped by the
			// backfill. The stored entry is therefore exactly the one the user
			// is now disambiguating, and either way they end up with one
			// favorite for that name.
			//
			// The whole record is replaced, not just the ID. Stamping the ID
			// alone would leave the entry asserting one pressing while carrying
			// another's year, label and catalogue number -- and permanently, as
			// backfillAlbums skips every entry that already has an ID.
			if album.ReleaseID != 0 && fav.ReleaseID == 0 {
				favorites[i] = album
				if err := saveFavorites(path, favorites); err != nil {
					return err
				}
			}
			return ErrAlreadyInFavorites
		}

		favorites = append(favorites, album)
		return saveFavorites(path, favorites)
	})
}

// removeFavorite removes the first favorite matching album.
//
// First match only, never every match: sameAlbum is not transitive, so an
// entry with no release ID matches every stored pressing sharing its name.
// Filtering all matches out would silently delete distinct pressings the
// user never named -- and `unfavorite` would still report removing one.
func removeFavorite(path string, album Album) error {
	return withFileLock(path, func() error {
		favorites, err := loadFavorites(path)
		if err != nil {
			return err
		}

		for i, fav := range favorites {
			if sameAlbum(fav, album) {
				// The three-index slice forces append to allocate rather than
				// alias favorites' backing array and clobber it in place.
				return saveFavorites(path, append(favorites[:i:i], favorites[i+1:]...))
			}
		}

		return ErrNotInFavorites
	})
}

// FavoriteStatus represents the outcome of attempting to favorite an album by query.
type FavoriteStatus int

const (
	FavoriteAdded FavoriteStatus = iota
	FavoriteAlreadyFav
	FavoriteNoMatch
	FavoriteMultiMatch
)

// FavoriteOutcome holds the result of favoriteByQuery.
type FavoriteOutcome struct {
	Status  FavoriteStatus
	Album   Album   // populated when Status is FavoriteAdded or FavoriteAlreadyFav
	Matches []Album // populated when Status is FavoriteMultiMatch
}

// favoriteByQuery is the testable core of `favorite QUERY`. It applies the query+filter
// to the provided collection and, if exactly one album matches, adds it to the
// favorites file at favPath. The caller is responsible for loading the collection,
// printing output, and choosing exit codes.
func favoriteByQuery(collection []Album, query string, filter Filter, favPath string) (FavoriteOutcome, error) {
	if query != "" {
		filter.Query.Include = append(filter.Query.Include, query)
	}
	matches := filter.Apply(collection)
	switch len(matches) {
	case 0:
		return FavoriteOutcome{Status: FavoriteNoMatch}, nil
	case 1:
		if err := addFavorite(favPath, matches[0]); err != nil {
			if errors.Is(err, ErrAlreadyInFavorites) {
				return FavoriteOutcome{Status: FavoriteAlreadyFav, Album: matches[0]}, nil
			}
			return FavoriteOutcome{}, err
		}
		return FavoriteOutcome{Status: FavoriteAdded, Album: matches[0]}, nil
	default:
		return FavoriteOutcome{Status: FavoriteMultiMatch, Matches: matches}, nil
	}
}

// UnfavoriteStatus represents the outcome of attempting to unfavorite an album by query.
type UnfavoriteStatus int

const (
	UnfavoriteRemoved UnfavoriteStatus = iota
	UnfavoriteNoMatch
	UnfavoriteMultiMatch
)

// UnfavoriteOutcome holds the result of unfavoriteByQuery.
type UnfavoriteOutcome struct {
	Status  UnfavoriteStatus
	Album   Album   // populated when Status is UnfavoriteRemoved
	Matches []Album // populated when Status is UnfavoriteMultiMatch
}

// unfavoriteByQuery is the testable core of `unfavorite QUERY`. It applies the
// query+filter to the favorites list — not the collection, since favorites is
// the set being removed from — and removes the album when exactly one matches.
// An album that is already absent is reported as UnfavoriteNoMatch rather than
// an error: removal is idempotent.
func unfavoriteByQuery(favorites []Album, query string, filter Filter, favPath string) (UnfavoriteOutcome, error) {
	if query != "" {
		filter.Query.Include = append(filter.Query.Include, query)
	}
	matches := filter.Apply(favorites)
	switch len(matches) {
	case 0:
		return UnfavoriteOutcome{Status: UnfavoriteNoMatch}, nil
	case 1:
		if err := removeFavorite(favPath, matches[0]); err != nil {
			if errors.Is(err, ErrNotInFavorites) {
				return UnfavoriteOutcome{Status: UnfavoriteNoMatch}, nil
			}
			return UnfavoriteOutcome{}, err
		}
		return UnfavoriteOutcome{Status: UnfavoriteRemoved, Album: matches[0]}, nil
	default:
		return UnfavoriteOutcome{Status: UnfavoriteMultiMatch, Matches: matches}, nil
	}
}

// errNoFavorites means the favorites list is empty or absent.
var errNoFavorites = errors.New("no favorites")

// loadFavoritesChecked loads favorites and reports an empty list as
// errNoFavorites, so callers can print guidance without repeating the check.
func loadFavoritesChecked(path string) ([]Album, error) {
	favorites, err := loadFavorites(path)
	if err != nil {
		return nil, err
	}
	if len(favorites) == 0 {
		return nil, errNoFavorites
	}
	return favorites, nil
}
