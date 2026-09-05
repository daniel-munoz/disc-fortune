package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const version = "2.4.0"

// fatal prints an error message to stderr and exits.
func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func main() {
	dispatch(os.Args[1:])
}

// loadCollectionOrExit loads the collection, explaining what to do and exiting 1
// when there is nothing to work with.
func loadCollectionOrExit() []Album {
	albums, err := loadCollectionChecked(collectionPath())
	switch {
	case errors.Is(err, errNoCollection):
		fatal("No collection found. Run `disc-fortune sync` to fetch your Discogs collection.")
	case errors.Is(err, errEmptyCollection):
		fatal("Collection is empty. Run `disc-fortune sync` to fetch your Discogs collection.")
	case err != nil:
		fatal("Error loading collection: %v", err)
	}
	return albums
}

// loadFavoritesOrExit loads favorites, explaining what to do and exiting 1 when
// there are none.
func loadFavoritesOrExit() []Album {
	favorites, err := loadFavoritesChecked(favoritesPath())
	switch {
	case errors.Is(err, errNoFavorites):
		fatal("No favorites yet. Use `disc-fortune favorite` after a pick you like.")
	case err != nil:
		fatal("Error loading favorites: %v", err)
	}
	return favorites
}

// stdoutColor resolves whether stdout gets escape sequences, combining the
// --color flag, NO_COLOR, and whether stdout is a terminal.
func stdoutColor(mode colorMode) bool {
	return useColor(mode, isTTY(os.Stdout), os.Getenv)
}

// selectAlbums loads the collection or favorites per cfg and applies its filter.
func selectAlbums(cfg selection) []Album {
	var albums []Album
	if cfg.favoritesOnly {
		albums = loadFavoritesOrExit()
	} else {
		albums = loadCollectionOrExit()
	}
	return cfg.filter.Apply(albums)
}

// formatMatch formats one candidate of an ambiguous query: the album, plus
// its release ID on a dim line of its own. Two pressings of a title can be
// identical in artist, title, year, label, catalogue number and genre -- two
// store-exclusive colours, say -- and then the ID is the only thing that
// tells them apart, as well as the only thing --release-id can act on.
func formatMatch(album Album, useColor bool) string {
	out := formatAlbum(album, useColor)
	if album.ReleaseID == 0 {
		return out
	}
	line := fmt.Sprintf("release %d", album.ReleaseID)
	if useColor {
		line = colorDim + line + colorReset
	}
	return out + "\n" + line
}

// formatList formats a slice of albums for list display.
// Albums are separated by blank lines; a count summary is appended.
//
// showIDs is set only where the user has to choose between candidates. Plain
// `list` leaves it off, so everyday output is unchanged.
func formatList(albums []Album, useColor, showIDs bool) string {
	if len(albums) == 0 {
		return "No albums match the specified filters\n"
	}
	var sb strings.Builder
	for i, album := range albums {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		if showIDs {
			sb.WriteString(formatMatch(album, useColor))
		} else {
			sb.WriteString(formatAlbum(album, useColor))
		}
	}
	noun := "albums"
	if len(albums) == 1 {
		noun = "album"
	}
	sb.WriteString(fmt.Sprintf("\n\n%d %s\n", len(albums), noun))
	return sb.String()
}

func runPick(cfg selection) {
	albums := selectAlbums(cfg)
	if len(albums) == 0 {
		fatal("No albums match the specified filters")
	}

	// History is read for the decision and then read again by addToHistory,
	// which takes its own lock. Deciding from a marginally stale history is
	// harmless, and it means no lock is held across the decision.
	entries, err := loadHistory(historyPath())
	if err != nil {
		fatal("Error loading history: %v", err)
	}

	if cfg.unheard {
		albums = unheardOnly(albums, entries)
		if len(albums) == 0 {
			fatal("Every album matching your filters has already been played.\n" +
				"Drop --unheard, or try `disc-fortune pick --draw stale` for whatever you have left longest.")
		}
	}

	album := pickAlbum(albums, entries, cfg.draw, newRNG())

	if err := addToHistory(historyPath(), album); err != nil {
		fatal("Error saving history: %v", err)
	}

	if cfg.json {
		if err := writeJSON(os.Stdout, pickPayload{Album: newJSONAlbum(album)}); err != nil {
			fatal("Error writing JSON: %v", err)
		}
	} else {
		fmt.Println(formatAlbum(album, stdoutColor(cfg.color)))
	}

	// Advisory, and therefore on stderr and only for a human at a terminal:
	// stdout is the data channel and must stay parseable.
	fmt.Fprint(os.Stderr, syncNotice(metaPath(), time.Now(), isTTY(os.Stderr)))
}

func runList(cfg selection) {
	albums := selectAlbums(cfg)

	// Only load history when it is actually needed: `list` has never
	// required a readable history.json and must not start now.
	if cfg.unheard && len(albums) > 0 {
		entries, err := loadHistory(historyPath())
		if err != nil {
			fatal("Error loading history: %v", err)
		}
		albums = unheardOnly(albums, entries)
		if len(albums) == 0 {
			fmt.Fprintln(os.Stderr, "Every album matching your filters has already been played.")
			os.Exit(1)
		}
	}

	// The empty case below is deliberately left alone: an empty list has
	// always been a failure, with its message on stderr and exit 1. --json
	// changes the format, not the semantics.
	if cfg.json && len(albums) > 0 {
		if err := writeJSON(os.Stdout, newListPayload(albums)); err != nil {
			fatal("Error writing JSON: %v", err)
		}
		return
	}

	out := formatList(albums, stdoutColor(cfg.color), false)
	if len(albums) == 0 {
		fmt.Fprint(os.Stderr, out)
		os.Exit(1)
	}
	fmt.Print(out)
}

func runHistory(cfg historyConfig) {
	entries, err := loadHistory(historyPath())
	if err != nil {
		fatal("Error loading history: %v", err)
	}

	limit := cfg.limit
	if limit == 0 {
		limit = len(entries) // 0 means show all
	}

	if cfg.json {
		if err := writeJSON(os.Stdout, newHistoryPayload(entries, limit)); err != nil {
			fatal("Error writing JSON: %v", err)
		}
		return
	}

	fmt.Print(formatHistory(entries, limit, stdoutColor(cfg.color)))
}

// describe names what the user actually asked for, for messages about
// finding nothing. Without it a query-less --release-id is reported as an
// empty query.
func (cfg favoriteConfig) describe() string {
	if cfg.query == "" && cfg.filter.ReleaseID != 0 {
		return fmt.Sprintf("release %d", cfg.filter.ReleaseID)
	}
	return fmt.Sprintf("%q", cfg.query)
}

func runFavorite(cfg favoriteConfig) {
	// An empty query means "the last pick" -- unless --release-id already
	// names a record, which is a selection in its own right.
	if cfg.query == "" && cfg.filter.ReleaseID == 0 {
		favoriteLastPick()
		return
	}

	albums := loadCollectionOrExit()
	outcome, err := favoriteByQuery(albums, cfg.filter, favoritesPath())
	if err != nil {
		fatal("Error adding favorite: %v", err)
	}

	switch outcome.Status {
	case FavoriteAdded:
		fmt.Printf("Added to favorites: %s - %s\n", outcome.Album.Artist, outcome.Album.Title)
	case FavoriteAlreadyFav:
		fmt.Println("Already in favorites")
	case FavoriteNoMatch:
		fatal("No albums match %s", cfg.describe())
	case FavoriteMultiMatch:
		fmt.Print(formatList(outcome.Matches, stdoutColor(cfg.color), true))
		fmt.Fprintln(os.Stderr, "Be more specific, add filters, or use --release-id.")
		os.Exit(1)
	}
}

func runUnfavorite(cfg favoriteConfig) {
	// As in runFavorite: --release-id is a selection, not a missing query.
	if cfg.query == "" && cfg.filter.ReleaseID == 0 {
		unfavoriteLastPick()
		return
	}

	// Unlike the read-only commands, unfavorite does not treat an empty or
	// absent favorites file as a failure: removing something from a favorites
	// list that has nothing in it (or nothing matching) is a no-op, not an
	// error. Load directly rather than through loadFavoritesOrExit so that
	// case reaches UnfavoriteNoMatch instead of fatal-ing.
	favorites, err := loadFavoritesChecked(favoritesPath())
	if err != nil && !errors.Is(err, errNoFavorites) {
		fatal("Error loading favorites: %v", err)
	}
	if errors.Is(err, errNoFavorites) {
		fmt.Printf("No favorites match %s - nothing to remove.\n", cfg.describe())
		return
	}

	outcome, err := unfavoriteByQuery(favorites, cfg.filter, favoritesPath())
	if err != nil {
		fatal("Error removing favorite: %v", err)
	}

	switch outcome.Status {
	case UnfavoriteRemoved:
		fmt.Printf("Removed from favorites: %s - %s\n", outcome.Album.Artist, outcome.Album.Title)
	case UnfavoriteNoMatch:
		// Removal is idempotent: nothing to remove is a success.
		fmt.Printf("No favorites match %s - nothing to remove.\n", cfg.describe())
	case UnfavoriteMultiMatch:
		fmt.Print(formatList(outcome.Matches, stdoutColor(cfg.color), true))
		fmt.Fprintln(os.Stderr, "Be more specific, add filters, or use --release-id.")
		os.Exit(1)
	}
}

func favoriteLastPick() {
	entries, err := loadHistory(historyPath())
	if err != nil {
		fatal("Error loading history: %v", err)
	}
	if len(entries) == 0 {
		fatal("No history to favorite")
	}

	lastAlbum := entries[len(entries)-1].Album
	if err := addFavorite(favoritesPath(), lastAlbum); err != nil {
		if errors.Is(err, ErrAlreadyInFavorites) {
			fmt.Println("Already in favorites")
			return
		}
		fatal("Error adding favorite: %v", err)
	}

	fmt.Printf("Added to favorites: %s - %s\n", lastAlbum.Artist, lastAlbum.Title)
}

func unfavoriteLastPick() {
	entries, err := loadHistory(historyPath())
	if err != nil {
		fatal("Error loading history: %v", err)
	}
	if len(entries) == 0 {
		fatal("No history to unfavorite")
	}

	lastAlbum := entries[len(entries)-1].Album
	if err := removeFavorite(favoritesPath(), lastAlbum); err != nil {
		if errors.Is(err, ErrNotInFavorites) {
			fmt.Println("Last pick was not in favorites")
			return
		}
		fatal("Error removing favorite: %v", err)
	}

	fmt.Printf("Removed from favorites: %s - %s\n", lastAlbum.Artist, lastAlbum.Title)
}
