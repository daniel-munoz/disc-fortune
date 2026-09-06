package disc

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

// LoadFavorites loads favorite albums from disk.
func LoadFavorites(path string) ([]Album, error) {
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

// SaveFavorites saves favorite albums to disk.
func SaveFavorites(path string, albums []Album) error {
	if err := os.MkdirAll(filepath.Dir(path), configDirPerms); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	data, err := json.MarshalIndent(albums, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding favorites: %w", err)
	}
	return writeFileAtomic(path, data, collectionFilePerms)
}

// AddFavorite adds an album to favorites if not already present.
//
// Locked for the same reason as AddToHistory: `sync`'s backfill rewrites
// favorites.json wholesale, and without the lock one of the two writes is lost.
func AddFavorite(path string, album Album) error {
	return withFileLock(path, func() error {
		favorites, err := LoadFavorites(path)
		if err != nil {
			return err
		}

		// SameAlbum rather than Key: two pressings of one title are two
		// favorites, but an entry written before release IDs existed is still
		// the same record as its freshly synced self.
		for i, fav := range favorites {
			if !SameAlbum(fav, album) {
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
				if err := SaveFavorites(path, favorites); err != nil {
					return err
				}
			}
			return ErrAlreadyInFavorites
		}

		favorites = append(favorites, album)
		return SaveFavorites(path, favorites)
	})
}

// RemoveFavorite removes the first favorite matching album.
//
// First match only, never every match: SameAlbum is not transitive, so an
// entry with no release ID matches every stored pressing sharing its name.
// Filtering all matches out would silently delete distinct pressings the
// user never named -- and `unfavorite` would still report removing one.
func RemoveFavorite(path string, album Album) error {
	return withFileLock(path, func() error {
		favorites, err := LoadFavorites(path)
		if err != nil {
			return err
		}

		for i, fav := range favorites {
			if SameAlbum(fav, album) {
				// The three-index slice forces append to allocate rather than
				// alias favorites' backing array and clobber it in place.
				return SaveFavorites(path, append(favorites[:i:i], favorites[i+1:]...))
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

// FavoriteOutcome holds the result of FavoriteByQuery.
type FavoriteOutcome struct {
	Status  FavoriteStatus
	Album   Album   // populated when Status is FavoriteAdded or FavoriteAlreadyFav
	Matches []Album // populated when Status is FavoriteMultiMatch
}

// FavoriteByQuery is the testable core of `favorite QUERY`. The query is
// already part of filter (parseFavorite puts the positional QUERY and --query
// in the same place), so this only applies it and acts on the result.
func FavoriteByQuery(collection []Album, filter Filter, favPath string) (FavoriteOutcome, error) {
	album, matches, status := MatchAlbums(collection, filter)
	switch status {
	case MatchedNone:
		return FavoriteOutcome{Status: FavoriteNoMatch}, nil
	case MatchedMany:
		return FavoriteOutcome{Status: FavoriteMultiMatch, Matches: matches}, nil
	}

	if err := AddFavorite(favPath, album); err != nil {
		if errors.Is(err, ErrAlreadyInFavorites) {
			return FavoriteOutcome{Status: FavoriteAlreadyFav, Album: album}, nil
		}
		return FavoriteOutcome{}, err
	}
	return FavoriteOutcome{Status: FavoriteAdded, Album: album}, nil
}

// UnfavoriteStatus represents the outcome of attempting to unfavorite an album by query.
type UnfavoriteStatus int

const (
	UnfavoriteRemoved UnfavoriteStatus = iota
	UnfavoriteNoMatch
	UnfavoriteMultiMatch
)

// UnfavoriteOutcome holds the result of UnfavoriteByQuery.
type UnfavoriteOutcome struct {
	Status  UnfavoriteStatus
	Album   Album   // populated when Status is UnfavoriteRemoved
	Matches []Album // populated when Status is UnfavoriteMultiMatch
}

// UnfavoriteByQuery is the testable core of `unfavorite QUERY`. The query is
// already part of filter (parseFavorite puts the positional QUERY and --query
// in the same place), so this applies filter to the favorites list — not the
// collection, since favorites is the set being removed from — and removes the
// album when exactly one matches. An album that is already absent is reported
// as UnfavoriteNoMatch rather than an error: removal is idempotent.
func UnfavoriteByQuery(favorites []Album, filter Filter, favPath string) (UnfavoriteOutcome, error) {
	album, matches, status := MatchAlbums(favorites, filter)
	switch status {
	case MatchedNone:
		return UnfavoriteOutcome{Status: UnfavoriteNoMatch}, nil
	case MatchedMany:
		return UnfavoriteOutcome{Status: UnfavoriteMultiMatch, Matches: matches}, nil
	}

	if err := RemoveFavorite(favPath, album); err != nil {
		if errors.Is(err, ErrNotInFavorites) {
			return UnfavoriteOutcome{Status: UnfavoriteNoMatch}, nil
		}
		return UnfavoriteOutcome{}, err
	}
	return UnfavoriteOutcome{Status: UnfavoriteRemoved, Album: album}, nil
}

// ErrNoFavorites means the favorites list is empty or absent.
var ErrNoFavorites = errors.New("no favorites")

// LoadFavoritesChecked loads favorites and reports an empty list as
// ErrNoFavorites, so callers can print guidance without repeating the check.
func LoadFavoritesChecked(path string) ([]Album, error) {
	favorites, err := LoadFavorites(path)
	if err != nil {
		return nil, err
	}
	if len(favorites) == 0 {
		return nil, ErrNoFavorites
	}
	return favorites, nil
}
