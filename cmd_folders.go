package main

import (
	"fmt"

	"github.com/daniel-munoz/disc-fortune/v2/internal/cli"
	"github.com/daniel-munoz/disc-fortune/v2/internal/discogs"
)

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

// runFolders lists the user's Discogs collection folders.
func (a app) runFolders() error {
	client, err := discogs.New(discogsUserAgent())
	if err != nil {
		return fmt.Errorf("Error: %v", err)
	}
	username, err := client.Username()
	if err != nil {
		return fmt.Errorf("Error: %v", err)
	}
	return printFolders(a.stdout, client, username)
}
