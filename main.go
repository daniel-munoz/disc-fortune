package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

type arrayFlags []string

func (a *arrayFlags) String() string { return strings.Join(*a, ", ") }
func (a *arrayFlags) Set(value string) error {
	*a = append(*a, value)
	return nil
}

// fatal prints an error message to stderr and exits.
func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func main() {
	syncFlag := flag.Bool("sync", false, "Sync collection from Discogs")
	listFoldersFlag := flag.Bool("list-folders", false, "List available Discogs folders (use with --sync)")
	var folderFlags arrayFlags
	flag.Var(&folderFlags, "folder", "Sync only specific folder(s) by name (repeatable, use with --sync)")
	flag.Parse()

	if *syncFlag {
		runSync(folderFlags, *listFoldersFlag)
		return
	}

	if *listFoldersFlag {
		fatal("Error: --list-folders requires --sync")
	}
	if len(folderFlags) > 0 {
		fatal("Error: --folder requires --sync")
	}

	runFortune()
}

func runFortune() {
	albums, err := loadCollection()
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No collection found. Run `disc-fortune --sync` to fetch your Discogs collection.")
			os.Exit(0)
		}
		fatal("Error loading collection: %v", err)
	}
	if len(albums) == 0 {
		fmt.Println("Collection is empty. Run `disc-fortune --sync` to fetch your Discogs collection.")
		os.Exit(0)
	}
	album := randomAlbum(albums)
	fmt.Printf("%s - %s\n", album.Artist, album.Title)
}

// runSync orchestrates syncing the collection from Discogs.
func runSync(folderNames []string, listFolders bool) {
	client, err := newDiscogsClient()
	if err != nil {
		fatal("Error: %v", err)
	}

	username, err := client.getUsername()
	if err != nil {
		fatal("Error: %v", err)
	}

	if listFolders {
		printFolders(client, username)
		return
	}

	folderIDs, err := resolveFolderIDs(client, username, folderNames)
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

	fmt.Printf("Synced %d albums\n", len(albums))
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
