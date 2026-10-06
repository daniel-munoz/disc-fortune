package main

import (
	"flag"
	"fmt"

	"github.com/daniel-munoz/disc-fortune/v2/internal/cli"
	"github.com/daniel-munoz/disc-fortune/v2/internal/disc"
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

// statsFlags holds the flags stats registers. See addSelectionFlags for why
// registration is factored out of the parse function.
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

func addStatsFlags(fs *flag.FlagSet) *statsFlags {
	return &statsFlags{
		favoritesOnly: fs.Bool("favorites", false, "Describe favorites only"),
		asJSON:        fs.Bool("json", false, "Emit machine-readable JSON instead of text"),
		filters:       addFilterFlags(fs),
	}
}

type statsCmd struct {
	sf  *statsFlags
	g   *cli.Globals
	cfg statsConfig
}

func (c *statsCmd) Flags(fs *flag.FlagSet, g *cli.Globals) {
	c.sf, c.g = addStatsFlags(fs), g
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
