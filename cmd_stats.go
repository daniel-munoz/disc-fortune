package main

import (
	"errors"
	"flag"
	"fmt"

	"github.com/daniel-munoz/disc-fortune/v2/internal/cli"
	"github.com/daniel-munoz/disc-fortune/v2/internal/disc"
	"github.com/daniel-munoz/disc-fortune/v2/internal/stats"
	"github.com/daniel-munoz/disc-fortune/v2/internal/term"
)

var statsSpec = cli.Spec[app]{
	Name:        "stats",
	NeedsConfig: true,
	Summary:     "Summarize your collection",
	Usage: `Usage: disc-fortune stats [flags]

Summarizes whatever the filters describe: a decade histogram, your most
common genres and labels, and how much of the set you have ever played.
Reads only files already on disk.

Flags:
  --favorites      Describe favorites only
  --json           Emit machine-readable JSON instead of text
` + filterFlagHelp,
	New: func() cli.Command[app] { return &statsCmd{} },
}

// statsConfig is the parsed form of stats.
type statsConfig struct {
	favoritesOnly bool
	filter        disc.Filter
	color         term.Mode
	json          bool
}

// statsFlags holds the flags stats registers.
//
// No --unheard: that flag is defined by history, and "share ever picked" over
// an unheard-only set is 0% by construction, so its only effect would be to
// make one of the headline figures meaningless. No --draw either: that is a
// draw strategy and stats draws nothing.
type statsFlags struct {
	favoritesOnly *bool
	asJSON        *bool
	filters       *filterFlags
}

type statsCmd struct {
	sf  *statsFlags
	g   *cli.Globals
	cfg statsConfig
}

func (c *statsCmd) Flags(fs *flag.FlagSet, g *cli.Globals) {
	c.sf = &statsFlags{
		favoritesOnly: fs.Bool("favorites", false, "Describe favorites only"),
		asJSON:        fs.Bool("json", false, "Emit machine-readable JSON instead of text"),
		filters:       addFilterFlags(fs),
	}
	c.g = g
}

// Parse validates stats's arguments.
//
// Unlike favorite, unfavorite and open, it does not apply the
// "filters require a query" rule. Those commands act on exactly one record,
// and a filter alone does not say which; stats is set-oriented like list and
// pick, so `stats --genre jazz` is a complete request.
func (c *statsCmd) Parse(rest []string) error {
	if len(rest) > 0 {
		return fmt.Errorf("stats: unexpected argument %q", rest[0])
	}
	filter, err := c.sf.filters.Filter()
	if err != nil {
		return fmt.Errorf("stats: %v", err)
	}
	color, err := c.g.Mode()
	if err != nil {
		return fmt.Errorf("stats: %v", err)
	}
	c.cfg = statsConfig{
		favoritesOnly: *c.sf.favoritesOnly,
		filter:        filter,
		color:         color,
		json:          *c.sf.asJSON,
	}
	return nil
}

func (c *statsCmd) Run(a app) error { return a.runStats(c.cfg) }

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

	s := stats.Compute(pool, favorites, entries, len(source), m, cfg.favoritesOnly)

	if cfg.json {
		if err := writeJSON(a.stdout, newStatsPayload(s)); err != nil {
			return fmt.Errorf("Error writing JSON: %v", err)
		}
		return nil
	}
	fmt.Fprint(a.stdout, stats.Format(s, a.stdoutColor(cfg.color)))
	return nil
}
