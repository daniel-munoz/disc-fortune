package main

import "github.com/daniel-munoz/disc-fortune/v2/internal/cli"

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
