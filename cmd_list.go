package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/daniel-munoz/disc-fortune/v2/internal/cli"
	"github.com/daniel-munoz/disc-fortune/v2/internal/disc"
	"github.com/daniel-munoz/disc-fortune/v2/internal/pick"
)

var listSpec = cli.Spec[app]{
	Name:        "list",
	NeedsConfig: true,
	Summary:     "List every matching album",
	Usage: `Usage: disc-fortune list [flags]

Prints every album matching the filters, with a count.

Flags:
  --favorites      List favorites only
  --unheard        List only albums you have never picked
  --json           Emit machine-readable JSON instead of text
` + filterFlagHelp,
	New: func() cli.Command[app] { return &listCmd{selectionCmd{name: "list"}} },
}

type listCmd struct{ selectionCmd }

func (c *listCmd) Run(a app) error { return a.runList(c.cfg) }

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

	// formatList ends in a newline; the error printer in Execute adds one of
	// its own, so it is trimmed off here to keep stderr byte-identical.
	out := formatList(albums, a.stdoutColor(cfg.color), false)
	if len(albums) == 0 {
		return errors.New(strings.TrimSuffix(out, "\n"))
	}
	fmt.Fprint(a.stdout, out)
	return nil
}
