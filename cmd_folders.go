package main

import "github.com/daniel-munoz/disc-fortune/v2/internal/cli"

var foldersSpec = cli.Spec[app]{
	Name:    "folders",
	Summary: "List your Discogs folder names",
	Usage: `Usage: disc-fortune folders

Lists the folder names in your Discogs collection, for use with
` + "`disc-fortune sync --folder`" + `. Requires DISCOGS_TOKEN to be set.`,
	New: func() cli.Command[app] { return &foldersCmd{noArgsCmd{name: "folders"}} },
}

type foldersCmd struct{ noArgsCmd }

func (c *foldersCmd) Run(a app) error { return a.runFolders() }
