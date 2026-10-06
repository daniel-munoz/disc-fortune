package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/daniel-munoz/disc-fortune/v2/internal/cli"
	"github.com/daniel-munoz/disc-fortune/v2/internal/disc"
	"github.com/daniel-munoz/disc-fortune/v2/internal/pick"
	"github.com/daniel-munoz/disc-fortune/v2/internal/term"
)

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

// recordMode is what a draw does to history once it has chosen. Named
// constants rather than a boolean so the call sites say which they mean.
type recordMode int

const (
	// recordAppend adds an entry: `pick`.
	recordAppend recordMode = iota
	// recordReplace writes over the most recent entry: `reroll`.
	recordReplace
)

var (
	errNothingToReroll = errors.New("Nothing to reroll. Run `disc-fortune pick` first.")
	errRerollRaced     = errors.New("The last pick changed while rerolling; nothing was replaced.\n" +
		"Run `disc-fortune history` to see what happened.")
)

func (a app) runPick(cfg selection) error { return a.drawAndRecord(cfg, recordAppend) }

// drawAndRecord is the body of both `pick` and `reroll`. They differ in
// exactly two places -- whether the last history entry is dropped before the
// draw, and which writer records the result -- so they share one path rather
// than two that drift apart.
func (a app) drawAndRecord(cfg selection, mode recordMode) error {
	albums, err := a.selectAlbums(cfg)
	if err != nil {
		return err
	}
	if len(albums) == 0 {
		return errors.New("No albums match the specified filters")
	}

	// History is read for the decision and then read again by the writer,
	// which takes its own lock. Deciding from a marginally stale history is
	// harmless for an append; a replace cannot be so relaxed, which is what
	// ReplaceLastHistory's expected argument guards.
	entries, err := disc.LoadHistory(a.historyPath())
	if err != nil {
		return fmt.Errorf("Error loading history: %v", err)
	}

	// The drop happens before --unheard and before the draw, and that
	// ordering is the whole of `reroll`: the declined record stops counting
	// as recently played, and becomes unheard again -- because it is.
	var dropped disc.HistoryEntry
	if mode == recordReplace {
		if len(entries) == 0 {
			return errNothingToReroll
		}
		dropped = entries[len(entries)-1]
		entries = entries[:len(entries)-1]
	}

	if cfg.unheard {
		albums = pick.UnheardOnly(albums, entries)
		if len(albums) == 0 {
			return errors.New("Every album matching your filters has already been played.\n" +
				"Drop --unheard, or try `disc-fortune pick --draw stale` for whatever you have left longest.")
		}
	}

	album := pick.Draw(albums, entries, cfg.draw, pick.NewRNG())

	// Recorded before it is printed: a failed write must never report an
	// album the tool did not save.
	if err := a.record(mode, dropped, album); err != nil {
		return err
	}

	if cfg.json {
		if err := writeJSON(a.stdout, pickPayload{Album: newJSONAlbum(album)}); err != nil {
			return fmt.Errorf("Error writing JSON: %v", err)
		}
	} else {
		fmt.Fprintln(a.stdout, formatAlbum(album, a.stdoutColor(cfg.color)))
	}

	// A receipt for a destructive action, so -- unlike the advisory notice
	// below -- it is printed whether or not stderr is a terminal. Someone
	// redirecting stderr to a log is exactly who should still get it.
	if mode == recordReplace {
		fmt.Fprintf(a.stderr, "Replaced: %s (%s)\n",
			dropped.Album.Key(), disc.FormatTimestamp(dropped.Timestamp))
	}

	// Advisory, and therefore on stderr and only for a human at a terminal:
	// stdout is the data channel and must stay parseable.
	fmt.Fprint(a.stderr, disc.SyncNotice(a.metaPath(), time.Now(), term.IsTTY(os.Stderr)))
	return nil
}

// record writes the drawn album to history, appending or replacing per mode.
// dropped is meaningful only for recordReplace.
func (a app) record(mode recordMode, dropped disc.HistoryEntry, album disc.Album) error {
	if mode == recordAppend {
		if err := disc.AddToHistory(a.historyPath(), album); err != nil {
			return fmt.Errorf("Error saving history: %v", err)
		}
		return nil
	}
	err := disc.ReplaceLastHistory(a.historyPath(), dropped, album)
	switch {
	case errors.Is(err, disc.ErrHistoryChanged):
		return errRerollRaced
	case err != nil:
		return fmt.Errorf("Error saving history: %v", err)
	}
	return nil
}
