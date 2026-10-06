package main

import (
	"errors"
	"fmt"

	"github.com/daniel-munoz/disc-fortune/v2/internal/cli"
	"github.com/daniel-munoz/disc-fortune/v2/internal/disc"
)

var favoriteSpec = cli.Spec[app]{
	Name:        "favorite",
	NeedsConfig: true,
	Summary:     "Add an album to favorites",
	Usage: `Usage: disc-fortune favorite [QUERY] [flags]

With no QUERY, favorites the last pick. With a QUERY, favorites the one
album in your collection whose "Artist - Title" contains it, case-insensitively.
If the query matches several albums, they are listed with their release IDs
and nothing is added; narrow it with filters, or name one with --release-id.

Two pressings of a title can be identical in every other field -- two
store-exclusive colours, say -- so --release-id is the one that always works.

The QUERY can also be given as --query, which is the only difference between
the two spellings. --release-id needs neither.
` + filterFlagHelp,
	New: func() cli.Command[app] { return &favoriteCmd{queryCmd{name: "favorite"}} },
}

var unfavoriteSpec = cli.Spec[app]{
	Name:        "unfavorite",
	NeedsConfig: true,
	Summary:     "Remove an album from favorites",
	Usage: `Usage: disc-fortune unfavorite [QUERY] [flags]

With no QUERY, unfavorites the last pick. With a QUERY, removes the one
favorite whose "Artist - Title" contains it, case-insensitively. Removing
something that is not favorited succeeds quietly.

The QUERY can also be given as --query, which is the only difference between
the two spellings. --release-id needs neither.
` + filterFlagHelp,
	New: func() cli.Command[app] { return &unfavoriteCmd{queryCmd{name: "unfavorite"}} },
}

type favoriteCmd struct{ queryCmd }

func (c *favoriteCmd) Run(a app) error { return a.runFavorite(c.cfg) }

type unfavoriteCmd struct{ queryCmd }

func (c *unfavoriteCmd) Run(a app) error { return a.runUnfavorite(c.cfg) }

func (cfg favoriteConfig) describe() string {
	return describeSelection(cfg.query, cfg.filter.ReleaseID)
}

func (a app) runFavorite(cfg favoriteConfig) error {
	// An empty query means "the last pick" -- unless --release-id already
	// names a record, which is a selection in its own right.
	if cfg.query == "" && cfg.filter.ReleaseID == 0 {
		return a.favoriteLastPick()
	}

	albums, err := a.collection()
	if err != nil {
		return err
	}
	outcome, err := disc.FavoriteByQuery(albums, cfg.filter, a.favoritesPath())
	if err != nil {
		return fmt.Errorf("Error adding favorite: %v", err)
	}

	switch outcome.Status {
	case disc.FavoriteAdded:
		fmt.Fprintf(a.stdout, "Added to favorites: %s - %s\n", outcome.Album.Artist, outcome.Album.Title)
	case disc.FavoriteAlreadyFav:
		fmt.Fprintln(a.stdout, "Already in favorites")
	case disc.FavoriteNoMatch:
		return fmt.Errorf("No albums match %s", cfg.describe())
	case disc.FavoriteMultiMatch:
		return a.reportAmbiguous(outcome.Matches, cfg.color)
	}
	return nil
}

func (a app) runUnfavorite(cfg favoriteConfig) error {
	// As in runFavorite: --release-id is a selection, not a missing query.
	if cfg.query == "" && cfg.filter.ReleaseID == 0 {
		return a.unfavoriteLastPick()
	}

	// Unlike the read-only commands, unfavorite does not treat an empty or
	// absent favorites file as a failure: removing something from a favorites
	// list that has nothing in it (or nothing matching) is a no-op, not an
	// error. Load directly rather than through the favorites helper so that
	// case reaches UnfavoriteNoMatch instead of failing.
	favorites, err := disc.LoadFavoritesChecked(a.favoritesPath())
	if err != nil && !errors.Is(err, disc.ErrNoFavorites) {
		return fmt.Errorf("Error loading favorites: %v", err)
	}
	if errors.Is(err, disc.ErrNoFavorites) {
		fmt.Fprintf(a.stdout, "No favorites match %s - nothing to remove.\n", cfg.describe())
		return nil
	}

	outcome, err := disc.UnfavoriteByQuery(favorites, cfg.filter, a.favoritesPath())
	if err != nil {
		return fmt.Errorf("Error removing favorite: %v", err)
	}

	switch outcome.Status {
	case disc.UnfavoriteRemoved:
		fmt.Fprintf(a.stdout, "Removed from favorites: %s - %s\n", outcome.Album.Artist, outcome.Album.Title)
	case disc.UnfavoriteNoMatch:
		// Removal is idempotent: nothing to remove is a success.
		fmt.Fprintf(a.stdout, "No favorites match %s - nothing to remove.\n", cfg.describe())
	case disc.UnfavoriteMultiMatch:
		return a.reportAmbiguous(outcome.Matches, cfg.color)
	}
	return nil
}

func (a app) favoriteLastPick() error {
	entries, err := disc.LoadHistory(a.historyPath())
	if err != nil {
		return fmt.Errorf("Error loading history: %v", err)
	}
	if len(entries) == 0 {
		return errors.New("No history to favorite")
	}

	lastAlbum := entries[len(entries)-1].Album
	if err := disc.AddFavorite(a.favoritesPath(), lastAlbum); err != nil {
		if errors.Is(err, disc.ErrAlreadyInFavorites) {
			fmt.Fprintln(a.stdout, "Already in favorites")
			return nil
		}
		return fmt.Errorf("Error adding favorite: %v", err)
	}

	fmt.Fprintf(a.stdout, "Added to favorites: %s - %s\n", lastAlbum.Artist, lastAlbum.Title)
	return nil
}

func (a app) unfavoriteLastPick() error {
	entries, err := disc.LoadHistory(a.historyPath())
	if err != nil {
		return fmt.Errorf("Error loading history: %v", err)
	}
	if len(entries) == 0 {
		return errors.New("No history to unfavorite")
	}

	lastAlbum := entries[len(entries)-1].Album
	if err := disc.RemoveFavorite(a.favoritesPath(), lastAlbum); err != nil {
		if errors.Is(err, disc.ErrNotInFavorites) {
			fmt.Fprintln(a.stdout, "Last pick was not in favorites")
			return nil
		}
		return fmt.Errorf("Error removing favorite: %v", err)
	}

	fmt.Fprintf(a.stdout, "Removed from favorites: %s - %s\n", lastAlbum.Artist, lastAlbum.Title)
	return nil
}
