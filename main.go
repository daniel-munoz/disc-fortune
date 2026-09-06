package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/daniel-munoz/disc-fortune/v2/internal/disc"
	"github.com/daniel-munoz/disc-fortune/v2/internal/pick"
	"github.com/daniel-munoz/disc-fortune/v2/internal/term"
)

const version = "2.5.0"

// discogsUserAgent is the single place the version reaches the API client.
func discogsUserAgent() string { return "disc-fortune/" + version }

// fatal prints an error message to stderr and exits.
func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func main() {
	dispatch(os.Args[1:])
}

// The guidance these carry used to be printed by loadCollectionOrExit and
// loadFavoritesOrExit immediately before os.Exit(1). It is now attached to
// the error so dispatch can print it at the one remaining exit point. The
// wording is asserted by tests and must not drift.
var (
	errNoCollectionGuidance    = errors.New("No collection found. Run `disc-fortune sync` to fetch your Discogs collection.")
	errEmptyCollectionGuidance = errors.New("Collection is empty. Run `disc-fortune sync` to fetch your Discogs collection.")
	errNoFavoritesGuidance     = errors.New("No favorites yet. Use `disc-fortune favorite` after a pick you like.")
)

// collection loads the collection, turning "nothing to work with" into the
// guidance error that explains what to do about it.
func (a app) collection() ([]disc.Album, error) {
	albums, err := disc.LoadCollectionChecked(a.collectionPath())
	switch {
	case errors.Is(err, disc.ErrNoCollection):
		return nil, errNoCollectionGuidance
	case errors.Is(err, disc.ErrEmptyCollection):
		return nil, errEmptyCollectionGuidance
	case err != nil:
		return nil, fmt.Errorf("Error loading collection: %v", err)
	}
	return albums, nil
}

// favorites loads favorites, turning "there are none" into the guidance error
// that explains what to do about it.
func (a app) favorites() ([]disc.Album, error) {
	favorites, err := disc.LoadFavoritesChecked(a.favoritesPath())
	switch {
	case errors.Is(err, disc.ErrNoFavorites):
		return nil, errNoFavoritesGuidance
	case err != nil:
		return nil, fmt.Errorf("Error loading favorites: %v", err)
	}
	return favorites, nil
}

// stdoutColor resolves whether stdout gets escape sequences, combining the
// --color flag, NO_COLOR, and whether stdout is a terminal. It deliberately
// still asks os.Stdout rather than a.stdout: colour depends on where output
// actually lands, and a test's bytes.Buffer is never a terminal anyway.
func (a app) stdoutColor(mode term.Mode) bool {
	return term.Use(mode, term.IsTTY(os.Stdout), os.Getenv)
}

// selectAlbums loads the collection or favorites per cfg and applies its filter.
func (a app) selectAlbums(cfg selection) ([]disc.Album, error) {
	var (
		albums []disc.Album
		err    error
	)
	if cfg.favoritesOnly {
		albums, err = a.favorites()
	} else {
		albums, err = a.collection()
	}
	if err != nil {
		return nil, err
	}
	return cfg.filter.Apply(albums), nil
}

// formatMatch formats one candidate of an ambiguous query: the album, plus
// its release ID on a dim line of its own. Two pressings of a title can be
// identical in artist, title, year, label, catalogue number and genre -- two
// store-exclusive colours, say -- and then the ID is the only thing that
// tells them apart, as well as the only thing --release-id can act on.
func formatMatch(album disc.Album, useColor bool) string {
	out := formatAlbum(album, useColor)
	if album.ReleaseID == 0 {
		return out
	}
	line := fmt.Sprintf("release %d", album.ReleaseID)
	if useColor {
		line = term.Dim + line + term.Reset
	}
	return out + "\n" + line
}

// formatList formats a slice of albums for list display.
// Albums are separated by blank lines; a count summary is appended.
//
// showIDs is set only where the user has to choose between candidates. Plain
// `list` leaves it off, so everyday output is unchanged.
func formatList(albums []disc.Album, useColor, showIDs bool) string {
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

func (a app) runPick(cfg selection) error {
	albums, err := a.selectAlbums(cfg)
	if err != nil {
		return err
	}
	if len(albums) == 0 {
		return errors.New("No albums match the specified filters")
	}

	// History is read for the decision and then read again by disc.AddToHistory,
	// which takes its own lock. Deciding from a marginally stale history is
	// harmless, and it means no lock is held across the decision.
	entries, err := disc.LoadHistory(a.historyPath())
	if err != nil {
		return fmt.Errorf("Error loading history: %v", err)
	}

	if cfg.unheard {
		albums = pick.UnheardOnly(albums, entries)
		if len(albums) == 0 {
			return errors.New("Every album matching your filters has already been played.\n" +
				"Drop --unheard, or try `disc-fortune pick --draw stale` for whatever you have left longest.")
		}
	}

	album := pick.Draw(albums, entries, cfg.draw, pick.NewRNG())

	if err := disc.AddToHistory(a.historyPath(), album); err != nil {
		return fmt.Errorf("Error saving history: %v", err)
	}

	if cfg.json {
		if err := writeJSON(a.stdout, pickPayload{Album: newJSONAlbum(album)}); err != nil {
			return fmt.Errorf("Error writing JSON: %v", err)
		}
	} else {
		fmt.Fprintln(a.stdout, formatAlbum(album, a.stdoutColor(cfg.color)))
	}

	// Advisory, and therefore on stderr and only for a human at a terminal:
	// stdout is the data channel and must stay parseable.
	fmt.Fprint(a.stderr, disc.SyncNotice(a.metaPath(), time.Now(), term.IsTTY(os.Stderr)))
	return nil
}

func (a app) runList(cfg selection) error {
	albums, err := a.selectAlbums(cfg)
	if err != nil {
		return err
	}

	// Only load history when it is actually needed: `list` has never
	// required a readable history.json and must not start now.
	if cfg.unheard && len(albums) > 0 {
		entries, err := disc.LoadHistory(a.historyPath())
		if err != nil {
			return fmt.Errorf("Error loading history: %v", err)
		}
		albums = pick.UnheardOnly(albums, entries)
		if len(albums) == 0 {
			return errors.New("Every album matching your filters has already been played.")
		}
	}

	// The empty case below is deliberately left alone: an empty list has
	// always been a failure, with its message on stderr and exit 1. --json
	// changes the format, not the semantics.
	if cfg.json && len(albums) > 0 {
		if err := writeJSON(a.stdout, newListPayload(albums)); err != nil {
			return fmt.Errorf("Error writing JSON: %v", err)
		}
		return nil
	}

	// formatList ends in a newline; the error printer in dispatch adds one of
	// its own, so it is trimmed off here to keep stderr byte-identical.
	out := formatList(albums, a.stdoutColor(cfg.color), false)
	if len(albums) == 0 {
		return errors.New(strings.TrimSuffix(out, "\n"))
	}
	fmt.Fprint(a.stdout, out)
	return nil
}

func (a app) runHistory(cfg historyConfig) error {
	entries, err := disc.LoadHistory(a.historyPath())
	if err != nil {
		return fmt.Errorf("Error loading history: %v", err)
	}

	limit := cfg.limit
	if limit == 0 {
		limit = len(entries) // 0 means show all
	}

	if cfg.json {
		if err := writeJSON(a.stdout, newHistoryPayload(entries, limit)); err != nil {
			return fmt.Errorf("Error writing JSON: %v", err)
		}
		return nil
	}

	fmt.Fprint(a.stdout, disc.FormatHistory(entries, limit, a.stdoutColor(cfg.color)))
	return nil
}

func (a app) runStats(cfg statsConfig) error {
	var (
		source []disc.Album
		err    error
	)
	if cfg.favoritesOnly {
		source, err = a.favorites()
	} else {
		source, err = a.collection()
	}
	if err != nil {
		return err
	}
	pool := cfg.filter.Apply(source)
	if len(pool) == 0 {
		// Same as list: an empty match has always been a failure, on stderr
		// with exit 1, and --json changes the format rather than that.
		return errors.New("No albums match the specified filters")
	}

	// Metadata is advisory and never sinks the run. History is the
	// exception: it feeds a headline figure, so an unreadable history fails
	// loudly.
	entries, err := disc.LoadHistory(a.historyPath())
	if err != nil {
		return fmt.Errorf("Error loading history: %v", err)
	}

	// Favorites are counted, not required. Someone with none gets a zero,
	// not an error.
	favorites, err := disc.LoadFavorites(a.favoritesPath())
	if err != nil {
		return fmt.Errorf("Error loading favorites: %v", err)
	}

	m, err := disc.LoadMeta(a.metaPath())
	if err != nil {
		m = disc.Meta{}
	}

	s := computeStats(pool, favorites, entries, len(source), m, cfg.favoritesOnly)

	if cfg.json {
		if err := writeJSON(a.stdout, newStatsPayload(s)); err != nil {
			return fmt.Errorf("Error writing JSON: %v", err)
		}
		return nil
	}
	fmt.Fprint(a.stdout, formatStats(s, a.stdoutColor(cfg.color)))
	return nil
}

// describeSelection names what the user actually asked for, for messages
// about finding nothing. Without it a query-less --release-id is reported as
// an empty query.
func describeSelection(query string, releaseID int) string {
	if query == "" && releaseID != 0 {
		return fmt.Sprintf("release %d", releaseID)
	}
	return fmt.Sprintf("%q", query)
}

func (cfg favoriteConfig) describe() string {
	return describeSelection(cfg.query, cfg.filter.ReleaseID)
}

func (cfg openConfig) describe() string {
	return describeSelection(cfg.query, cfg.filter.ReleaseID)
}

// reportAmbiguous prints the candidates a query matched and returns the error
// that ends the run. It is the one piece favorite, unfavorite and open's
// ambiguous-match branches share verbatim; everything around it -- what counts
// as a match, what happens when there is none, what exit code that path takes
// -- differs per command and stays local to each.
//
// The candidate list stays on stdout, where it has always been: it is the
// answer to the query, and only the trailing advice belongs on stderr. So the
// list is printed here and just the advice is carried by the error, which
// dispatch prints to stderr before exiting 1.
func (a app) reportAmbiguous(matches []disc.Album, color term.Mode) error {
	fmt.Fprint(a.stdout, formatList(matches, a.stdoutColor(color), true))
	return errors.New("Be more specific, add filters, or use --release-id.")
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

// resolveOpenTarget picks the record to open: the last pick when nothing was
// named, or the single match for the query. Like favorite, it fails rather
// than returning a record on every empty or ambiguous outcome, because what
// to say about each depends on what was asked.
func resolveOpenTarget(a app, cfg openConfig) (disc.Album, error) {
	// As in runFavorite: --release-id is a selection, not a missing query.
	if cfg.query == "" && cfg.filter.ReleaseID == 0 {
		entries, err := disc.LoadHistory(a.historyPath())
		if err != nil {
			return disc.Album{}, fmt.Errorf("Error loading history: %v", err)
		}
		if len(entries) == 0 {
			return disc.Album{}, errors.New("No history to open. Run `disc-fortune pick` first, or name a record.")
		}
		return entries[len(entries)-1].Album, nil
	}

	albums, err := a.collection()
	if err != nil {
		return disc.Album{}, err
	}
	album, matches, status := disc.MatchAlbums(albums, cfg.filter)
	switch status {
	case disc.MatchedNone:
		return disc.Album{}, fmt.Errorf("No albums match %s", cfg.describe())
	case disc.MatchedMany:
		return disc.Album{}, a.reportAmbiguous(matches, cfg.color)
	}
	return album, nil
}

func (a app) runOpen(cfg openConfig) error {
	album, err := resolveOpenTarget(a, cfg)
	if err != nil {
		return err
	}
	if album.ReleaseID == 0 {
		return errors.New("This record predates release IDs and sync could not identify it.\n" +
			"Run `disc-fortune sync`, or name the record with --release-id.")
	}
	url := discogsReleaseURL(album.ReleaseID)

	plan := planOpen(url, cfg.printOnly, runtime.GOOS, exec.LookPath, os.Getenv)
	if plan.Launch == nil {
		// The URL is the data channel's answer; the note explaining why
		// nothing was launched is advisory and belongs on stderr. Exit 0:
		// the user got what they asked for.
		fmt.Fprintln(a.stdout, url)
		if plan.Note != "" {
			fmt.Fprintln(a.stderr, plan.Note)
		}
		return nil
	}

	if err := launchBrowser(plan.Launch); err != nil {
		// A launcher that exists but will not start is a real failure, not a
		// degradation -- but print the URL anyway so the user is not left
		// with nothing. The "disc-fortune: " prefix is part of this message's
		// own text: dispatch's printer adds none.
		fmt.Fprintln(a.stdout, url)
		return fmt.Errorf("disc-fortune: could not launch %s: %v", plan.Launch[0], err)
	}
	return nil
}

// runMigrate moves the data directory to its XDG-preferred location.
func (a app) runMigrate() error {
	if a.loc.Preferred == "" {
		fmt.Fprintf(a.stdout, "Nothing to migrate: disc-fortune is already using %s\n", a.loc.Dir)
		return nil
	}

	from, to := a.loc.Dir, a.loc.Preferred
	moved, err := disc.Migrate(from, to)
	if err != nil {
		return fmt.Errorf("Error migrating: %v", err)
	}

	noun := "files"
	if moved == 1 {
		noun = "file"
	}
	fmt.Fprintf(a.stdout, "Moved %d %s from %s to %s\n", moved, noun, from, to)
	return nil
}
