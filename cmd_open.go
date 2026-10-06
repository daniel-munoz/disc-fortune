package main

import (
	"flag"

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

// addOpenFlags registers open's flags. See addSelectionFlags for why
// registration is factored out of the parse function.
func addOpenFlags(fs *flag.FlagSet) (*bool, *filterFlags) {
	printOnly := fs.Bool("print", false, "Print the URL instead of opening a browser")
	return printOnly, addFilterFlags(fs)
}

// openCmd shares favorite's query grammar and adds --print.
type openCmd struct {
	printOnly *bool
	ff        *filterFlags
	g         *cli.Globals
	cfg       openConfig
}

func (c *openCmd) Flags(fs *flag.FlagSet, g *cli.Globals) {
	c.printOnly, c.ff = addOpenFlags(fs)
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
