package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/daniel-munoz/disc-fortune/v2/internal/cli"
	"github.com/daniel-munoz/disc-fortune/v2/internal/disc"
	"github.com/daniel-munoz/disc-fortune/v2/internal/term"
)

var openSpec = cli.Spec[app]{
	Name:        "open",
	NeedsConfig: true,
	Summary:     "Open a record's Discogs page",
	Usage: `Usage: disc-fortune open [QUERY] [flags]

With no QUERY, opens the last pick. With a QUERY, opens the one album in your
collection whose "Artist - Title" contains it, case-insensitively. If the
query matches several albums, they are listed with their release IDs and
nothing is opened; narrow it with filters, or name one with --release-id.

With no browser to launch into -- no launcher on PATH, or no display -- the
URL is printed instead and the command still succeeds.

Flags:
  --print          Print the URL instead of opening a browser
` + filterFlagHelp,
	New: func() cli.Command[app] { return &openCmd{} },
}

// openConfig is the parsed form of open. As with favoriteConfig, an empty
// query means "the last pick".
type openConfig struct {
	query     string
	filter    disc.Filter
	color     term.Mode
	printOnly bool
}

// openCmd shares favorite's query grammar and adds --print.
type openCmd struct {
	printOnly *bool
	ff        *filterFlags
	g         *cli.Globals
	cfg       openConfig
}

func (c *openCmd) Flags(fs *flag.FlagSet, g *cli.Globals) {
	c.printOnly = fs.Bool("print", false, "Print the URL instead of opening a browser")
	c.ff = addFilterFlags(fs)
	c.g = g
}

func (c *openCmd) Parse(rest []string) error {
	q, err := parseQueryCommand("open", c.g, c.ff, rest)
	if err != nil {
		return err
	}
	c.cfg = openConfig{
		query:     q.query,
		filter:    q.filter,
		color:     q.color,
		printOnly: *c.printOnly,
	}
	return nil
}

func (c *openCmd) Run(a app) error { return a.runOpen(c.cfg) }

func (cfg openConfig) describe() string {
	return describeSelection(cfg.query, cfg.filter.ReleaseID)
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
		// own text: Execute's printer adds none.
		fmt.Fprintln(a.stdout, url)
		return fmt.Errorf("disc-fortune: could not launch %s: %v", plan.Launch[0], err)
	}
	return nil
}
