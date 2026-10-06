package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/daniel-munoz/disc-fortune/v2/internal/cli"
	"github.com/daniel-munoz/disc-fortune/v2/internal/disc"
	"github.com/daniel-munoz/disc-fortune/v2/internal/discogs"
	"github.com/daniel-munoz/disc-fortune/v2/internal/term"
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

type syncCmd struct {
	folders *arrayFlags
	g       *cli.Globals
	cfg     syncConfig
}

func (c *syncCmd) Flags(fs *flag.FlagSet, g *cli.Globals) {
	c.folders = new(arrayFlags)
	fs.Var(c.folders, "folder", "Sync only specific folder(s) by name (repeatable)")
	c.g = g
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

// runSync fetches the collection from Discogs and caches it locally.
func (a app) runSync(cfg syncConfig) error {
	client, err := discogs.New(discogsUserAgent())
	if err != nil {
		return fmt.Errorf("Error: %v", err)
	}
	client.Progress = syncProgress(a.stderr, term.IsTTY(os.Stderr))

	username, err := client.Username()
	if err != nil {
		return fmt.Errorf("Error: %v", err)
	}

	folderIDs, err := resolveFolderIDs(client, username, cfg.folders)
	if err != nil {
		return fmt.Errorf("Error: %v", err)
	}

	albums, err := collectAlbums(client, username, folderIDs)
	if err != nil {
		return fmt.Errorf("Error: %v", err)
	}

	// Read before the write below overwrites it: comparing the two is what
	// tells us whether this is the first sync after the identity change.
	// Failing to read it is not an error -- it just means no notice.
	previous, _ := disc.LoadCollectionFrom(a.collectionPath())

	if err := disc.SaveCollectionTo(a.collectionPath(), albums); err != nil {
		return fmt.Errorf("Error saving collection: %v", err)
	}

	// Recorded after the collection lands, so a stale timestamp never claims
	// a sync that did not actually persist.
	if err := disc.RecordSync(a.metaPath(), time.Now()); err != nil {
		return fmt.Errorf("Error saving sync metadata: %v", err)
	}

	// Also after the collection lands, so IDs are never stamped from a
	// collection that then failed to save. A failure here does not fail the
	// sync: the sync itself succeeded, the pass is idempotent, and the next
	// sync retries it. The report is kept and printed below either way --
	// a partial pass may have already rewritten favorites, and the user has
	// to be told what changed, not just that something went wrong.
	backfillReport, err := disc.RunBackfill(a.favoritesPath(), a.historyPath(), albums)
	if err != nil {
		fmt.Fprintf(a.stderr, "Warning: could not fill in release IDs: %v\n", err)
	}

	withMetadata := 0
	for _, album := range albums {
		if album.Year != 0 || album.Label != "" || len(album.Genres) > 0 {
			withMetadata++
		}
	}

	fmt.Fprintf(a.stdout, "Synced %d albums (%d with full metadata)\n", len(albums), withMetadata)
	fmt.Fprint(a.stdout, unmergeNotice(previous, albums))
	fmt.Fprint(a.stdout, backfillReport)
	return nil
}
