package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/daniel-munoz/disc-fortune/v2/internal/disc"
	"github.com/daniel-munoz/disc-fortune/v2/internal/term"
)

// app carries what every command needs but no command's flags describe:
// where the data lives, and where its output goes. Constructed once in
// dispatch and passed to each command, replacing the package-level
// activeConfig that used to back the path helpers.
//
// stdout and stderr are injected rather than hard-coded so a command can be
// called directly in a test and its output read back from a buffer, instead
// of only being exercisable by spawning a subprocess.
type app struct {
	loc    disc.Location
	stdout io.Writer
	stderr io.Writer
}

// newApp resolves the config location for this run. getenv and homeDir are
// injected so the decision can be tested without touching the real
// environment, matching disc.ResolveDir's existing contract. stdout and
// stderr are injected the same way, for the same reason.
func newApp(getenv func(string) string, homeDir func() (string, error), stdout, stderr io.Writer) (app, error) {
	loc, err := disc.ResolveDir(getenv, homeDir)
	if err != nil {
		return app{stdout: stdout, stderr: stderr}, err
	}
	return app{loc: loc, stdout: stdout, stderr: stderr}, nil
}

func (a app) collectionPath() string { return filepath.Join(a.loc.Dir, "collection.json") }
func (a app) favoritesPath() string  { return filepath.Join(a.loc.Dir, "favorites.json") }
func (a app) historyPath() string    { return filepath.Join(a.loc.Dir, "history.json") }
func (a app) metaPath() string       { return filepath.Join(a.loc.Dir, "meta.json") }

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

// describeSelection names what the user actually asked for, for messages
// about finding nothing. Without it a query-less --release-id is reported as
// an empty query.
func describeSelection(query string, releaseID int) string {
	if query == "" && releaseID != 0 {
		return fmt.Sprintf("release %d", releaseID)
	}
	return fmt.Sprintf("%q", query)
}
