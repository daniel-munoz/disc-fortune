package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const version = "1.1.0"

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
	versionFlag := flag.Bool("version", false, "Print version and exit")
	syncFlag := flag.Bool("sync", false, "Sync collection from Discogs")
	listFoldersFlag := flag.Bool("list-folders", false, "List available Discogs folders (use with --sync)")
	var folderFlags arrayFlags
	flag.Var(&folderFlags, "folder", "Sync only specific folder(s) by name (repeatable, use with --sync)")

	// New flags
	historyFlag := flag.String("history", "", "Show pick history (e.g., --history 10 for last 10, --history 0 for all)")
	favoritesFlag := flag.Bool("favorites", false, "Pick randomly from favorites only")
	favoriteLast := flag.Bool("favorite-last", false, "Add last pick to favorites")
	unfavoriteLast := flag.Bool("unfavorite-last", false, "Remove last pick from favorites")
	listFlag := flag.Bool("list", false, "List all matching albums instead of picking one")

	yearFlag := flag.String("year", "", "Filter by year or year range (e.g., 1975 or 1970-1980)")
	genreFlag := flag.String("genre", "", "Filter by genre (case-insensitive substring match)")
	labelFlag := flag.String("label", "", "Filter by label (case-insensitive substring match)")
	formatFlag := flag.String("format", "", "Filter by format (case-insensitive substring match)")

	flag.Parse()

	if *versionFlag {
		fmt.Printf("disc-fortune %s\n", version)
		return
	}

	// Check if --history was explicitly used (flag will be set even if empty)
	historySet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "history" {
			historySet = true
		}
	})
	if historySet {
		runHistory(*historyFlag)
		return
	}

	if *favoriteLast {
		runFavoriteLast()
		return
	}

	if *unfavoriteLast {
		runUnfavoriteLast()
		return
	}

	if *listFlag {
		if err := ParseYearFilter(*yearFlag); err != nil {
			fatal("Error: %v", err)
		}
		filter := Filter{
			Year:   *yearFlag,
			Genre:  *genreFlag,
			Label:  *labelFlag,
			Format: *formatFlag,
		}
		runList(*favoritesFlag, filter)
		return
	}

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

	// Validate year filter
	if err := ParseYearFilter(*yearFlag); err != nil {
		fatal("Error: %v", err)
	}

	// Build filter
	filter := Filter{
		Year:   *yearFlag,
		Genre:  *genreFlag,
		Label:  *labelFlag,
		Format: *formatFlag,
	}

	runFortune(*favoritesFlag, filter)
}

// formatList formats a slice of albums for list display.
// Albums are separated by blank lines; a count summary is appended.
func formatList(albums []Album, useColor bool) string {
	if len(albums) == 0 {
		return "No albums match the specified filters\n"
	}
	var sb strings.Builder
	for i, album := range albums {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(formatAlbum(album, useColor))
	}
	sb.WriteString(fmt.Sprintf("\n\n%d albums\n", len(albums)))
	return sb.String()
}

func runList(favoritesOnly bool, filter Filter) {
	var albums []Album
	var err error

	if favoritesOnly {
		albums, err = loadFavorites(favoritesPath())
		if err != nil {
			fatal("Error loading favorites: %v", err)
		}
		if len(albums) == 0 {
			fmt.Println("No favorites yet. Use --favorite-last after a pick you like.")
			os.Exit(0)
		}
	} else {
		albums, err = loadCollection()
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
	}

	albums = filter.Apply(albums)
	useColor := isTTY(os.Stdout)
	fmt.Print(formatList(albums, useColor))
}

func runFortune(favoritesOnly bool, filter Filter) {
	var albums []Album
	var err error

	if favoritesOnly {
		albums, err = loadFavorites(favoritesPath())
		if err != nil {
			fatal("Error loading favorites: %v", err)
		}
		if len(albums) == 0 {
			fmt.Println("No favorites yet. Use --favorite-last after a pick you like.")
			os.Exit(0)
		}
	} else {
		albums, err = loadCollection()
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
	}

	// Apply filters
	albums = filter.Apply(albums)
	if len(albums) == 0 {
		fmt.Println("No albums match the specified filters")
		os.Exit(0)
	}

	album := randomAlbum(albums)

	// Add to history
	if err := addToHistory(historyPath(), album); err != nil {
		fatal("Error saving history: %v", err)
	}

	// Format and print
	useColor := isTTY(os.Stdout)
	fmt.Println(formatAlbum(album, useColor))
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

	withMetadata := 0
	for _, album := range albums {
		if album.Year != 0 || album.Label != "" || len(album.Genres) > 0 {
			withMetadata++
		}
	}

	fmt.Printf("Synced %d albums (%d with full metadata)\n", len(albums), withMetadata)
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

func runHistory(limitStr string) {
	entries, err := loadHistory(historyPath())
	if err != nil {
		fatal("Error loading history: %v", err)
	}

	// Parse limit: empty string or no value = 10, "0" = all, otherwise use the number
	limit := 10
	if limitStr != "" {
		parsedLimit, err := strconv.Atoi(strings.TrimSpace(limitStr))
		if err != nil {
			fatal("Error: --history requires a number (e.g., --history 20)")
		}
		if parsedLimit == 0 {
			limit = len(entries) // 0 means show all
		} else {
			limit = parsedLimit
		}
	}

	useColor := isTTY(os.Stdout)
	fmt.Print(formatHistory(entries, limit, useColor))
}

func runFavoriteLast() {
	entries, err := loadHistory(historyPath())
	if err != nil {
		fatal("Error loading history: %v", err)
	}
	if len(entries) == 0 {
		fatal("No history to favorite")
	}

	lastAlbum := entries[len(entries)-1].Album
	err = addFavorite(favoritesPath(), lastAlbum)
	if err != nil {
		if errors.Is(err, ErrAlreadyInFavorites) {
			fmt.Println("Already in favorites")
			return
		}
		fatal("Error adding favorite: %v", err)
	}

	fmt.Printf("Added to favorites: %s - %s\n", lastAlbum.Artist, lastAlbum.Title)
}

func runUnfavoriteLast() {
	entries, err := loadHistory(historyPath())
	if err != nil {
		fatal("Error loading history: %v", err)
	}
	if len(entries) == 0 {
		fatal("No history to unfavorite")
	}

	lastAlbum := entries[len(entries)-1].Album
	err = removeFavorite(favoritesPath(), lastAlbum)
	if err != nil {
		if errors.Is(err, ErrNotInFavorites) {
			fmt.Println("Last pick was not in favorites")
			return
		}
		fatal("Error removing favorite: %v", err)
	}

	fmt.Printf("Removed from favorites: %s - %s\n", lastAlbum.Artist, lastAlbum.Title)
}
