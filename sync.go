package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/daniel-munoz/disc-fortune/v2/internal/disc"
	"github.com/daniel-munoz/disc-fortune/v2/internal/discogs"
)

// syncProgress returns a discogs.ProgressFunc writing to w, or nil when
// progress is unwanted. Progress goes to stderr and only when stderr is a
// terminal: stdout is the data channel, and a redirected stderr means the
// output is being captured by something that does not want a page counter
// in it.
func syncProgress(w io.Writer, enabled bool) discogs.ProgressFunc {
	if !enabled {
		return nil
	}
	return func(format string, args ...any) {
		fmt.Fprintf(w, format, args...)
	}
}

// arrayFlags collects a repeatable string flag's values. --folder was its
// first user; every repeatable filter flag (--genre, --exclude-artist, and
// the rest) is built on it too.
type arrayFlags []string

func (a *arrayFlags) String() string { return strings.Join(*a, ", ") }

func (a *arrayFlags) Set(value string) error {
	*a = append(*a, value)
	return nil
}

// printFolders lists the user's Discogs collection folders.
func printFolders(w io.Writer, client *discogs.Client, username string) error {
	folders, err := client.Folders(username)
	if err != nil {
		return fmt.Errorf("Error: %v", err)
	}
	fmt.Fprintln(w, "Available folders:")
	for _, f := range folders {
		fmt.Fprintf(w, "  %s\n", f.Name)
	}
	return nil
}

// resolveFolderIDs maps folder names to IDs, defaulting to folder 0 ("All").
func resolveFolderIDs(client *discogs.Client, username string, names []string) ([]int, error) {
	if len(names) == 0 {
		return []int{0}, nil
	}

	folders, err := client.Folders(username)
	if err != nil {
		return nil, err
	}
	return resolveFolderNames(names, folders)
}

// collectAlbums fetches releases from the given folders and deduplicates them.
//
// Dedup is on Identity, not Key: two pressings of one title are two records
// and must both survive, while one release filed in two folders is one
// record however its title is spelled in each.
func collectAlbums(client *discogs.Client, username string, folderIDs []int) ([]disc.Album, error) {
	seen := make(map[string]bool)
	var albums []disc.Album

	for _, fid := range folderIDs {
		releases, err := client.CollectionReleases(username, fid)
		if err != nil {
			return nil, err
		}
		for _, a := range releases {
			if id := a.Identity(); !seen[id] {
				seen[id] = true
				albums = append(albums, a)
			}
		}
	}

	return albums, nil
}

func resolveFolderNames(names []string, folders []discogs.Folder) ([]int, error) {
	nameToID := make(map[string]int)
	for _, f := range folders {
		nameToID[f.Name] = f.ID
	}

	var ids []int
	for _, name := range names {
		id, ok := nameToID[name]
		if !ok {
			available := make([]string, len(folders))
			for i, f := range folders {
				available[i] = fmt.Sprintf("  %s", f.Name)
			}
			return nil, fmt.Errorf("folder %q not found. Available folders:\n%s", name, strings.Join(available, "\n"))
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// unmergedCount counts the albums that share an artist and title with at
// least one other album -- that is, every record involved in a collision the
// old name-based dedup used to hide.
func unmergedCount(albums []disc.Album) int {
	byKey := make(map[string]int, len(albums))
	for _, a := range albums {
		byKey[a.Key()]++
	}

	count := 0
	for _, n := range byKey {
		if n > 1 {
			count += n
		}
	}
	return count
}

// unmergeNotice explains the collection count that is about to jump, or ""
// when there is nothing to explain.
//
// Three conditions together: a previous collection existed, at least one of
// its entries had no release ID, and the fresh collection has a collision.
// That fires exactly once, on the first sync after upgrading, and suppresses
// itself forever afterwards because every entry has an ID from then on -- so
// no flag in meta.json is needed to make it one-time.
//
// The wording states the collision count as a fact rather than blaming it
// for the whole change in size. Someone who also bought records since their
// last sync would otherwise be told something false.
func unmergeNotice(prev, next []disc.Album) string {
	if len(prev) == 0 {
		return ""
	}

	legacy := false
	for _, a := range prev {
		if a.ReleaseID == 0 {
			legacy = true
			break
		}
	}
	if !legacy {
		return ""
	}

	n := unmergedCount(next)
	if n == 0 {
		return ""
	}

	return fmt.Sprintf(
		"Note: %d records share an artist and title with another record. Before v2.2.0\n"+
			"      these were merged into one entry; they are now listed separately.\n", n)
}
