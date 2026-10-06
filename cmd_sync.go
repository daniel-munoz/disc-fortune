package main

import (
	"flag"
	"fmt"

	"github.com/daniel-munoz/disc-fortune/v2/internal/cli"
)

var syncSpec = cli.Spec[app]{
	Name:        "sync",
	NeedsConfig: true,
	Summary:     "Fetch your collection from Discogs",
	Usage: `Usage: disc-fortune sync [--folder NAME ...]

Fetches your Discogs collection and caches it locally. Requires DISCOGS_TOKEN
to be set. With no --folder, syncs everything.

Flags:
  --folder NAME    Sync only this folder (repeatable)

Run ` + "`disc-fortune folders`" + ` to see available folder names.`,
	New: func() cli.Command[app] { return &syncCmd{} },
}

// syncConfig is the parsed form of sync.
type syncConfig struct {
	folders []string
}

// addSyncFlags registers sync's flags. See addSelectionFlags for why
// registration is factored out of the parse function.
func addSyncFlags(fs *flag.FlagSet) *arrayFlags {
	folders := new(arrayFlags)
	fs.Var(folders, "folder", "Sync only specific folder(s) by name (repeatable)")
	return folders
}

type syncCmd struct {
	folders *arrayFlags
	g       *cli.Globals
	cfg     syncConfig
}

func (c *syncCmd) Flags(fs *flag.FlagSet, g *cli.Globals) {
	c.folders, c.g = addSyncFlags(fs), g
}

func (c *syncCmd) Parse(rest []string) error {
	if len(rest) > 0 {
		return fmt.Errorf("sync: unexpected argument %q", rest[0])
	}
	// sync colorizes nothing, but a bad --color value is still a typo and
	// must be reported rather than ignored.
	if _, err := c.g.Mode(); err != nil {
		return fmt.Errorf("sync: %v", err)
	}
	c.cfg = syncConfig{folders: *c.folders}
	return nil
}

func (c *syncCmd) Run(a app) error { return a.runSync(c.cfg) }
