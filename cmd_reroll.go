package main

import "github.com/daniel-munoz/disc-fortune/v2/internal/cli"

var rerollSpec = cli.Spec[app]{
	Name:        "reroll",
	NeedsConfig: true,
	Summary:     "Replace the last pick with a new one",
	Usage: `Usage: disc-fortune reroll [flags]

Draws a replacement for your last pick and writes it over that pick's history
entry instead of adding a new one. Use it when what you were handed is not what
you want to hear: the discarded pick leaves no trace, so it is neither avoided
by your next few picks nor counted as one you have already heard.

It takes every flag pick takes, so you can narrow things down while you are at
it. The record you turned down goes back in the pool, so a reroll can hand it
straight back to you -- reroll again.

Flags:
  --favorites      Pick from favorites only
  --unheard        Pick only from albums you have never picked
  --draw WHEN      How to draw: fresh (default), any, or stale.
                   fresh skips your recent picks; any ignores history
                   entirely; stale favors what you have left longest.
  --json           Emit machine-readable JSON instead of text
` + filterFlagHelp,
	New: func() cli.Command[app] { return &rerollCmd{selectionCmd{name: "reroll"}} },
}

type rerollCmd struct{ selectionCmd }

func (c *rerollCmd) Run(a app) error { return a.runReroll(c.cfg) }
