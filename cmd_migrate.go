package main

import "github.com/daniel-munoz/disc-fortune/v2/internal/cli"

var migrateSpec = cli.Spec[app]{
	Name:        "migrate",
	NeedsConfig: true,
	Summary:     "Move config to the XDG location",
	Usage: `Usage: disc-fortune migrate

Moves disc-fortune's data files from the legacy ~/.config/disc-fortune to the
directory XDG_CONFIG_HOME points at.

disc-fortune keeps using the legacy directory when it already holds your data,
even with XDG_CONFIG_HOME set, so that an upgrade never appears to lose your
collection. This command performs the move once you are ready. It refuses to
run if the destination already contains files.`,
	New: func() cli.Command[app] { return &migrateCmd{noArgsCmd{name: "migrate"}} },
}

type migrateCmd struct{ noArgsCmd }

func (c *migrateCmd) Run(a app) error { return a.runMigrate() }
