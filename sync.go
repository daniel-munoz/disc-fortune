package main

import (
	"fmt"
	"strings"
)

// arrayFlags collects a repeatable string flag (--folder).
type arrayFlags []string

func (a *arrayFlags) String() string { return strings.Join(*a, ", ") }
func (a *arrayFlags) Set(value string) error {
	*a = append(*a, value)
	return nil
}

// runSync fetches the collection from Discogs and caches it locally.
func runSync(cfg syncConfig) {
	client, err := newDiscogsClient()
	if err != nil {
		fatal("Error: %v", err)
	}

	username, err := client.getUsername()
	if err != nil {
		fatal("Error: %v", err)
	}

	folderIDs, err := resolveFolderIDs(client, username, cfg.folders)
	if err != nil {
		fatal("Error: %v", err)
	}

	albums, err := collectAlbums(client, username, folderIDs)
	if err != nil {
		fatal("Error: %v", err)
	}

	if err := saveCollection(albums); err != nil {
		fatal("Error saving collection: %v", err)
	}

	withMetadata := 0
	for _, album := range albums {
		if album.Year != 0 || album.Label != "" || len(album.Genres) > 0 {
			withMetadata++
		}
	}

	fmt.Printf("Synced %d albums (%d with full metadata)\n", len(albums), withMetadata)
}

// runFolders lists the user's Discogs collection folders.
func runFolders() {
	client, err := newDiscogsClient()
	if err != nil {
		fatal("Error: %v", err)
	}
	username, err := client.getUsername()
	if err != nil {
		fatal("Error: %v", err)
	}
	printFolders(client, username)
}

// printFolders lists the user's Discogs collection folders.
func printFolders(client *discogsClient, username string) {
	folders, err := client.getFolders(username)
	if err != nil {
		fatal("Error: %v", err)
	}
	fmt.Println("Available folders:")
	for _, f := range folders {
		fmt.Printf("  %s\n", f.Name)
	}
}

// resolveFolderIDs maps folder names to IDs, defaulting to folder 0 ("All").
func resolveFolderIDs(client *discogsClient, username string, names []string) ([]int, error) {
	if len(names) == 0 {
		return []int{0}, nil
	}

	folders, err := client.getFolders(username)
	if err != nil {
		return nil, err
	}
	return resolveFolderNames(names, folders)
}

// collectAlbums fetches releases from the given folders and deduplicates them.
func collectAlbums(client *discogsClient, username string, folderIDs []int) ([]Album, error) {
	seen := make(map[string]bool)
	var albums []Album

	for _, fid := range folderIDs {
		releases, err := client.getCollectionReleases(username, fid)
		if err != nil {
			return nil, err
		}
		for _, a := range releases {
			if key := a.Key(); !seen[key] {
				seen[key] = true
				albums = append(albums, a)
			}
		}
	}

	return albums, nil
}

func resolveFolderNames(names []string, folders []folder) ([]int, error) {
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
