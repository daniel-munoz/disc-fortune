package main

import (
	"io"
	"path/filepath"
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
	loc    configLocation
	stdout io.Writer
	stderr io.Writer
}

// newApp resolves the config location for this run. getenv and homeDir are
// injected so the decision can be tested without touching the real
// environment, matching resolveConfigDir's existing contract. stdout and
// stderr are injected the same way, for the same reason.
func newApp(getenv func(string) string, homeDir func() (string, error), stdout, stderr io.Writer) (app, error) {
	loc, err := resolveConfigDir(getenv, homeDir)
	if err != nil {
		return app{stdout: stdout, stderr: stderr}, err
	}
	return app{loc: loc, stdout: stdout, stderr: stderr}, nil
}

func (a app) collectionPath() string { return filepath.Join(a.loc.Dir, "collection.json") }
func (a app) favoritesPath() string  { return filepath.Join(a.loc.Dir, "favorites.json") }
func (a app) historyPath() string    { return filepath.Join(a.loc.Dir, "history.json") }
func (a app) metaPath() string       { return filepath.Join(a.loc.Dir, "meta.json") }
