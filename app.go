package main

import "path/filepath"

// app carries what every command needs but no command's flags describe:
// where the data lives. Constructed once in dispatch and passed to each
// command, replacing the package-level activeConfig that used to back the
// path helpers.
type app struct {
	loc configLocation
}

// newApp resolves the config location for this run. getenv and homeDir are
// injected so the decision can be tested without touching the real
// environment, matching resolveConfigDir's existing contract.
func newApp(getenv func(string) string, homeDir func() (string, error)) (app, error) {
	loc, err := resolveConfigDir(getenv, homeDir)
	if err != nil {
		return app{}, err
	}
	return app{loc: loc}, nil
}

func (a app) collectionPath() string { return filepath.Join(a.loc.Dir, "collection.json") }
func (a app) favoritesPath() string  { return filepath.Join(a.loc.Dir, "favorites.json") }
func (a app) historyPath() string    { return filepath.Join(a.loc.Dir, "history.json") }
func (a app) metaPath() string       { return filepath.Join(a.loc.Dir, "meta.json") }
