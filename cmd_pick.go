package main

import "github.com/daniel-munoz/disc-fortune/v2/internal/cli"

var pickSpec = cli.Spec[app]{
	Name:        "pick",
	NeedsConfig: true,
	Summary:     "Print a random album (default)",
	Usage: `Usage: disc-fortune pick [flags]

Prints one random album from your collection and records it in history.
This is what runs when you give no command at all.

By default a pick avoids the records you played most recently, so the same
album does not come back around twice in a week. --draw any turns that off.

Flags:
  --favorites      Pick from favorites only
  --unheard        Pick only from albums you have never picked
  --draw WHEN      How to draw: fresh (default), any, or stale.
                   fresh skips your recent picks; any ignores history
                   entirely; stale favors what you have left longest.
  --json           Emit machine-readable JSON instead of text
` + filterFlagHelp,
	New: func() cli.Command[app] { return &pickCmd{selectionCmd{name: "pick"}} },
}

type pickCmd struct{ selectionCmd }

func (c *pickCmd) Run(a app) error { return a.runPick(c.cfg) }
