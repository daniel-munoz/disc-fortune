package main

import (
	"flag"
	"fmt"
	"strconv"
	"strings"

	"github.com/daniel-munoz/disc-fortune/v2/internal/cli"
	"github.com/daniel-munoz/disc-fortune/v2/internal/disc"
	"github.com/daniel-munoz/disc-fortune/v2/internal/term"
)

var historySpec = cli.Spec[app]{
	Name:        "history",
	NeedsConfig: true,
	Summary:     "Show recent picks",
	Usage: `Usage: disc-fortune history [N] [flags]

Shows the last N picks. N defaults to 10; 0 shows all of them.

Flags:
  --json           Emit machine-readable JSON instead of text`,
	New: func() cli.Command[app] { return &historyCmd{} },
}

// defaultHistoryLimit is how many past picks `history` shows with no argument.
const defaultHistoryLimit = 10

// historyConfig is the parsed form of history. A limit of 0 means "all".
type historyConfig struct {
	limit int
	color term.Mode
	json  bool
}

// addHistoryFlags registers history's flags. See addSelectionFlags for why
// registration is factored out of the parse function.
func addHistoryFlags(fs *flag.FlagSet) *bool {
	return fs.Bool("json", false, "Emit machine-readable JSON instead of text")
}

type historyCmd struct {
	asJSON *bool
	g      *cli.Globals
	cfg    historyConfig
}

func (c *historyCmd) Flags(fs *flag.FlagSet, g *cli.Globals) {
	c.asJSON, c.g = addHistoryFlags(fs), g
}

func (c *historyCmd) Parse(rest []string) error {
	if len(rest) > 1 {
		return fmt.Errorf("history: too many arguments")
	}
	limit := defaultHistoryLimit
	if len(rest) == 1 {
		n, err := strconv.Atoi(strings.TrimSpace(rest[0]))
		if err != nil {
			return fmt.Errorf("history: requires a number (e.g., history 20)")
		}
		if n < 0 {
			return fmt.Errorf("history: count cannot be negative")
		}
		limit = n
	}
	color, err := c.g.Mode()
	if err != nil {
		return fmt.Errorf("history: %v", err)
	}
	c.cfg = historyConfig{limit: limit, color: color, json: *c.asJSON}
	return nil
}

func (c *historyCmd) Run(a app) error { return a.runHistory(c.cfg) }

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
