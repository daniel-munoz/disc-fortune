# Favorite-by-Query Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `--favorite "query"` flag that adds a specific album to favorites by case-insensitive substring match against `Artist - Title`, composable with existing filter flags. Preserve `--favorite-last`.

**Architecture:** Extend the existing `Filter` struct with a `Query` field. Introduce a testable seam — `favoriteByQuery(collection, query, filter, favPath) (FavoriteOutcome, error)` — that holds the branch-on-match-count logic and the single side effect (writing to favorites). A thin `runFavorite` wrapper in `main.go` loads the collection, calls the seam, and translates the outcome into output + exit codes. No new files; pure stdlib.

**Tech Stack:** Go stdlib (`flag`, `errors`, `fmt`, `os`, `path/filepath`, `strings`)

**Spec:** `docs/plans/2026-05-26-favorite-by-query-design.md`

---

## Task 1: Add `Query` field to `Filter` (tests first)

**Files:**
- Modify: `filter_test.go` (add tests)
- Modify: `filter.go:9-15` (add field), `filter.go:18-30` (extend Apply early-return), `filter.go:32-46` (wire into matches)

- [ ] **Step 1: Write failing tests for `Filter.Query`**

Append to `filter_test.go`:

```go
func TestFilterByQueryArtist(t *testing.T) {
	albums := []Album{
		{Artist: "Miles Davis", Title: "Kind of Blue"},
		{Artist: "John Coltrane", Title: "Giant Steps"},
		{Artist: "Bill Evans", Title: "Sunday at the Village Vanguard"},
	}

	f := Filter{Query: "miles"}
	filtered := f.Apply(albums)
	if len(filtered) != 1 {
		t.Fatalf("got %d albums, want 1", len(filtered))
	}
	if filtered[0].Artist != "Miles Davis" {
		t.Errorf("Artist = %q, want Miles Davis", filtered[0].Artist)
	}
}

func TestFilterByQueryTitle(t *testing.T) {
	albums := []Album{
		{Artist: "Miles Davis", Title: "Kind of Blue"},
		{Artist: "John Coltrane", Title: "Giant Steps"},
	}

	f := Filter{Query: "giant"}
	filtered := f.Apply(albums)
	if len(filtered) != 1 {
		t.Fatalf("got %d albums, want 1", len(filtered))
	}
	if filtered[0].Title != "Giant Steps" {
		t.Errorf("Title = %q, want Giant Steps", filtered[0].Title)
	}
}

func TestFilterByQueryCaseInsensitive(t *testing.T) {
	albums := []Album{
		{Artist: "Miles Davis", Title: "Kind of Blue"},
	}

	f := Filter{Query: "MILES"}
	filtered := f.Apply(albums)
	if len(filtered) != 1 {
		t.Errorf("got %d albums, want 1 (case-insensitive)", len(filtered))
	}
}

func TestFilterByQueryEmptyIsNoOp(t *testing.T) {
	albums := []Album{
		{Artist: "A", Title: "1"},
		{Artist: "B", Title: "2"},
	}

	f := Filter{Query: ""}
	filtered := f.Apply(albums)
	if len(filtered) != 2 {
		t.Errorf("got %d albums, want 2 (empty query should not filter)", len(filtered))
	}
}

func TestFilterByQueryNoMatch(t *testing.T) {
	albums := []Album{
		{Artist: "Miles Davis", Title: "Kind of Blue"},
	}

	f := Filter{Query: "nonexistent"}
	filtered := f.Apply(albums)
	if len(filtered) != 0 {
		t.Errorf("got %d albums, want 0", len(filtered))
	}
}

func TestFilterByQueryMultipleMatches(t *testing.T) {
	albums := []Album{
		{Artist: "Miles Davis", Title: "Kind of Blue"},
		{Artist: "Miles Davis", Title: "Bitches Brew"},
		{Artist: "John Coltrane", Title: "Giant Steps"},
	}

	f := Filter{Query: "miles"}
	filtered := f.Apply(albums)
	if len(filtered) != 2 {
		t.Errorf("got %d albums, want 2", len(filtered))
	}
}

func TestFilterQueryComposesWithYear(t *testing.T) {
	albums := []Album{
		{Artist: "Miles Davis", Title: "Kind of Blue", Year: 1959},
		{Artist: "Miles Davis", Title: "Bitches Brew", Year: 1970},
	}

	f := Filter{Query: "miles", Year: "1959"}
	filtered := f.Apply(albums)
	if len(filtered) != 1 {
		t.Fatalf("got %d albums, want 1", len(filtered))
	}
	if filtered[0].Title != "Kind of Blue" {
		t.Errorf("Title = %q, want Kind of Blue", filtered[0].Title)
	}
}
```

- [ ] **Step 2: Run tests and confirm they fail**

Run: `go test ./... -run TestFilterByQuery -v`
Expected: compilation error — `Filter` has no `Query` field.

- [ ] **Step 3: Add `Query` field to `Filter`**

In `filter.go`, modify the struct (currently lines 9-15):

```go
// Filter represents album filtering criteria.
type Filter struct {
	Query  string
	Year   string
	Genre  string
	Label  string
	Format string
}
```

- [ ] **Step 4: Extend the Apply early-return to consider Query**

In `filter.go`, modify the `Apply` method (currently around lines 18-21):

```go
// Apply filters albums based on criteria.
func (f Filter) Apply(albums []Album) []Album {
	if f.Query == "" && f.Year == "" && f.Genre == "" && f.Label == "" && f.Format == "" {
		return albums
	}

	var filtered []Album
	for _, album := range albums {
		if f.matches(album) {
			filtered = append(filtered, album)
		}
	}
	return filtered
}
```

- [ ] **Step 5: Wire Query into matches() and add matchesQuery**

In `filter.go`, modify the `matches` method to check Query first, and add the new `matchesQuery` helper:

```go
func (f Filter) matches(album Album) bool {
	if f.Query != "" && !f.matchesQuery(album) {
		return false
	}
	if f.Year != "" && !f.matchesYear(album.Year) {
		return false
	}
	if f.Genre != "" && !f.matchesGenre(album.Genres) {
		return false
	}
	if f.Label != "" && !f.matchesString(album.Label, f.Label) {
		return false
	}
	if f.Format != "" && !f.matchesFormats(album.Formats) {
		return false
	}
	return true
}

func (f Filter) matchesQuery(album Album) bool {
	return f.matchesString(album.Key(), f.Query)
}
```

`Album.Key()` returns `Artist + " - " + Title` (defined in `collection.go:28-30`), so this matches across both fields with a single substring check.

- [ ] **Step 6: Run all tests and confirm they pass**

Run: `go test ./... -v`
Expected: all tests pass, including the new `TestFilterByQuery*` tests and the pre-existing `TestFilterByYear`, `TestFilterByGenre`, `TestFilterCombined`.

- [ ] **Step 7: Commit**

```bash
git add filter.go filter_test.go
git commit -m "feat: add Query field to Filter for artist+title substring matching"
```

---

## Task 2: Add testable seam `favoriteByQuery` in `favorites.go` (tests first)

**Files:**
- Modify: `favorites_test.go` (add tests)
- Modify: `favorites.go` (add types + function)

- [ ] **Step 1: Write failing tests for `favoriteByQuery`**

Append to `favorites_test.go`:

```go
func TestFavoriteByQuery_SingleMatch(t *testing.T) {
	tmpDir := t.TempDir()
	favPath := filepath.Join(tmpDir, "favorites.json")
	collection := []Album{
		{Artist: "Miles Davis", Title: "Kind of Blue"},
		{Artist: "John Coltrane", Title: "Giant Steps"},
	}

	outcome, err := favoriteByQuery(collection, "kind of", Filter{}, favPath)
	if err != nil {
		t.Fatalf("favoriteByQuery: %v", err)
	}
	if outcome.Status != FavoriteAdded {
		t.Errorf("Status = %v, want FavoriteAdded", outcome.Status)
	}
	if outcome.Album.Title != "Kind of Blue" {
		t.Errorf("Album.Title = %q, want Kind of Blue", outcome.Album.Title)
	}
	favs, err := loadFavorites(favPath)
	if err != nil {
		t.Fatalf("loadFavorites: %v", err)
	}
	if len(favs) != 1 || favs[0].Title != "Kind of Blue" {
		t.Errorf("favorites = %+v, want one Kind of Blue", favs)
	}
}

func TestFavoriteByQuery_NoMatch(t *testing.T) {
	tmpDir := t.TempDir()
	favPath := filepath.Join(tmpDir, "favorites.json")
	collection := []Album{
		{Artist: "Miles Davis", Title: "Kind of Blue"},
	}

	outcome, err := favoriteByQuery(collection, "zzzz", Filter{}, favPath)
	if err != nil {
		t.Fatalf("favoriteByQuery: %v", err)
	}
	if outcome.Status != FavoriteNoMatch {
		t.Errorf("Status = %v, want FavoriteNoMatch", outcome.Status)
	}
	favs, _ := loadFavorites(favPath)
	if len(favs) != 0 {
		t.Errorf("favorites should be empty after no match, got %d", len(favs))
	}
}

func TestFavoriteByQuery_MultiMatch(t *testing.T) {
	tmpDir := t.TempDir()
	favPath := filepath.Join(tmpDir, "favorites.json")
	collection := []Album{
		{Artist: "Miles Davis", Title: "Kind of Blue"},
		{Artist: "Miles Davis", Title: "Bitches Brew"},
		{Artist: "John Coltrane", Title: "Giant Steps"},
	}

	outcome, err := favoriteByQuery(collection, "miles", Filter{}, favPath)
	if err != nil {
		t.Fatalf("favoriteByQuery: %v", err)
	}
	if outcome.Status != FavoriteMultiMatch {
		t.Errorf("Status = %v, want FavoriteMultiMatch", outcome.Status)
	}
	if len(outcome.Matches) != 2 {
		t.Errorf("got %d matches, want 2", len(outcome.Matches))
	}
	favs, _ := loadFavorites(favPath)
	if len(favs) != 0 {
		t.Errorf("favorites should be empty after multi-match, got %d", len(favs))
	}
}

func TestFavoriteByQuery_AlreadyFavorited(t *testing.T) {
	tmpDir := t.TempDir()
	favPath := filepath.Join(tmpDir, "favorites.json")
	collection := []Album{
		{Artist: "Miles Davis", Title: "Kind of Blue"},
	}

	// First add succeeds
	if _, err := favoriteByQuery(collection, "kind of", Filter{}, favPath); err != nil {
		t.Fatalf("first favoriteByQuery: %v", err)
	}
	// Second add returns FavoriteAlreadyFav, favorites stays at 1
	outcome, err := favoriteByQuery(collection, "kind of", Filter{}, favPath)
	if err != nil {
		t.Fatalf("second favoriteByQuery: %v", err)
	}
	if outcome.Status != FavoriteAlreadyFav {
		t.Errorf("Status = %v, want FavoriteAlreadyFav", outcome.Status)
	}
	if outcome.Album.Title != "Kind of Blue" {
		t.Errorf("Album.Title = %q, want Kind of Blue", outcome.Album.Title)
	}
	favs, _ := loadFavorites(favPath)
	if len(favs) != 1 {
		t.Errorf("got %d favorites, want 1 (still only one)", len(favs))
	}
}

func TestFavoriteByQuery_ComposesWithFilter(t *testing.T) {
	tmpDir := t.TempDir()
	favPath := filepath.Join(tmpDir, "favorites.json")
	collection := []Album{
		{Artist: "Miles Davis", Title: "Kind of Blue", Year: 1959},
		{Artist: "Miles Davis", Title: "Bitches Brew", Year: 1970},
	}

	// "miles" alone matches both; "miles" + year 1959 narrows to one
	outcome, err := favoriteByQuery(collection, "miles", Filter{Year: "1959"}, favPath)
	if err != nil {
		t.Fatalf("favoriteByQuery: %v", err)
	}
	if outcome.Status != FavoriteAdded {
		t.Errorf("Status = %v, want FavoriteAdded", outcome.Status)
	}
	if outcome.Album.Title != "Kind of Blue" {
		t.Errorf("Album.Title = %q, want Kind of Blue", outcome.Album.Title)
	}
}
```

- [ ] **Step 2: Run tests and confirm they fail**

Run: `go test ./... -run TestFavoriteByQuery -v`
Expected: compilation error — `favoriteByQuery`, `FavoriteAdded`, `FavoriteNoMatch`, `FavoriteMultiMatch`, `FavoriteAlreadyFav`, `FavoriteOutcome` are undefined.

- [ ] **Step 3: Add `FavoriteStatus`, `FavoriteOutcome`, and `favoriteByQuery` to `favorites.go`**

Append to `favorites.go`:

```go
// FavoriteStatus represents the outcome of attempting to favorite an album by query.
type FavoriteStatus int

const (
	FavoriteAdded FavoriteStatus = iota
	FavoriteAlreadyFav
	FavoriteNoMatch
	FavoriteMultiMatch
)

// FavoriteOutcome holds the result of favoriteByQuery.
type FavoriteOutcome struct {
	Status  FavoriteStatus
	Album   Album   // populated when Status is FavoriteAdded or FavoriteAlreadyFav
	Matches []Album // populated when Status is FavoriteMultiMatch
}

// favoriteByQuery is the testable core of --favorite. It applies the query+filter
// to the provided collection and, if exactly one album matches, adds it to the
// favorites file at favPath. The caller is responsible for loading the collection,
// printing output, and choosing exit codes.
func favoriteByQuery(collection []Album, query string, filter Filter, favPath string) (FavoriteOutcome, error) {
	filter.Query = query
	matches := filter.Apply(collection)
	switch len(matches) {
	case 0:
		return FavoriteOutcome{Status: FavoriteNoMatch}, nil
	case 1:
		if err := addFavorite(favPath, matches[0]); err != nil {
			if errors.Is(err, ErrAlreadyInFavorites) {
				return FavoriteOutcome{Status: FavoriteAlreadyFav, Album: matches[0]}, nil
			}
			return FavoriteOutcome{}, err
		}
		return FavoriteOutcome{Status: FavoriteAdded, Album: matches[0]}, nil
	default:
		return FavoriteOutcome{Status: FavoriteMultiMatch, Matches: matches}, nil
	}
}
```

The `errors` package is already imported in `favorites.go` (line 5).

- [ ] **Step 4: Run all tests and confirm they pass**

Run: `go test ./... -v`
Expected: all tests pass — the new `TestFavoriteByQuery_*` tests plus all pre-existing tests.

- [ ] **Step 5: Commit**

```bash
git add favorites.go favorites_test.go
git commit -m "feat: add favoriteByQuery seam with FavoriteOutcome result type"
```

---

## Task 3: Wire `--favorite` flag and `runFavorite` into `main.go`

**Files:**
- Modify: `main.go` (add flag declaration, dispatch, new wrapper function)

- [ ] **Step 1: Add the `--favorite` flag declaration**

In `main.go`, alongside the existing favorite flags (currently around lines 38-40), add:

```go
favoriteFlag := flag.String("favorite", "", "Add a specific album to favorites by query (e.g., --favorite \"kind of blue\")")
```

Place this immediately before the `listFlag` declaration so the favorite-related flags stay grouped.

- [ ] **Step 2: Detect whether `--favorite` was set (distinguish from empty)**

After `flag.Parse()` and after the existing `--history` detection block (currently lines 54-64), add a similar block for `--favorite`:

```go
// Check if --favorite was explicitly used (so we can reject --favorite "" with a clear error)
favoriteSet := false
flag.Visit(func(f *flag.Flag) {
	if f.Name == "favorite" {
		favoriteSet = true
	}
})
```

- [ ] **Step 3: Add dispatch for `--favorite` with conflict checks**

In `main.go`, immediately after the `--unfavoriteLast` dispatch (currently lines 71-74), insert:

```go
if favoriteSet {
	if *favoriteLast {
		fatal("Error: --favorite and --favorite-last are mutually exclusive")
	}
	if *favoritesFlag {
		fatal("Error: --favorites is for picking from favorites, not adding")
	}
	if strings.TrimSpace(*favoriteFlag) == "" {
		fatal("Error: --favorite requires a query")
	}
	if err := ParseYearFilter(*yearFlag); err != nil {
		fatal("Error: %v", err)
	}
	filter := Filter{
		Year:   *yearFlag,
		Genre:  *genreFlag,
		Label:  *labelFlag,
		Format: *formatFlag,
	}
	runFavorite(*favoriteFlag, filter)
	return
}
```

This mirrors the structure of the existing `--list` dispatch (lines 76-88).

- [ ] **Step 4: Add the `runFavorite` wrapper**

In `main.go`, add this function after `runUnfavoriteLast` (the current last function in the file):

```go
func runFavorite(query string, filter Filter) {
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

	outcome, err := favoriteByQuery(albums, query, filter, favoritesPath())
	if err != nil {
		fatal("Error adding favorite: %v", err)
	}

	switch outcome.Status {
	case FavoriteAdded:
		fmt.Printf("Added to favorites: %s - %s\n", outcome.Album.Artist, outcome.Album.Title)
	case FavoriteAlreadyFav:
		fmt.Println("Already in favorites")
	case FavoriteNoMatch:
		fatal("No albums match query %q", query)
	case FavoriteMultiMatch:
		useColor := isTTY(os.Stdout)
		fmt.Print(formatList(outcome.Matches, useColor))
		fmt.Println("Be more specific or add filters.")
		os.Exit(1)
	}
}
```

`errors`, `fmt`, `os`, and `strings` are already imported in `main.go` (lines 3-10). No new imports needed.

- [ ] **Step 5: Verify the project builds**

Run: `go build -o /tmp/disc-fortune .`
Expected: clean build, no errors.

- [ ] **Step 6: Run all tests**

Run: `go test ./... -v`
Expected: all tests pass — Filter.Query tests, favoriteByQuery tests, and pre-existing tests.

- [ ] **Step 7: Manual smoke test for CLI-level conflict cases and end-to-end behavior**

Run a smoke check against a synced collection. The conflict cases and exit codes are best verified via the actual binary, since they live in the dispatch layer (not the testable seam).

```sh
# Single match path (substitute a query you know matches exactly one album)
./disc-fortune --favorite "kind of blue"

# Multi-match path (substitute a query you know matches several)
./disc-fortune --favorite "miles"

# Zero-match path
./disc-fortune --favorite "zzzzzzz-no-match"

# Conflict: empty query
./disc-fortune --favorite ""
# Expected: "Error: --favorite requires a query"

# Conflict: --favorite-last
./disc-fortune --favorite "miles" --favorite-last
# Expected: "Error: --favorite and --favorite-last are mutually exclusive"

# Conflict: --favorites
./disc-fortune --favorite "miles" --favorites
# Expected: "Error: --favorites is for picking from favorites, not adding"

# Composition with filters
./disc-fortune --favorite "miles" --year 1959

# Already-favorited (run the single-match command twice)
./disc-fortune --favorite "kind of blue"
# Expected on second run: "Already in favorites"
```

For each command, verify the output matches the spec (`docs/plans/2026-05-26-favorite-by-query-design.md`).

- [ ] **Step 8: Commit**

```bash
git add main.go
git commit -m "feat: add --favorite flag wired to runFavorite + favoriteByQuery"
```

---

## Task 4: Update README

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add `--favorite` examples to the Usage section**

In `README.md`, locate the favorites-related examples (currently lines 49-59). Insert the new `--favorite` examples between the `--favorite-last` block and the `--favorites` block, keeping the file's existing tone and code-block style.

Replace the block (currently `README.md:49-59`):

```markdown
# Mark the last pick as a favorite
disc-fortune --favorite-last

# Pick randomly from favorites only
disc-fortune --favorites

# Filter within favorites
disc-fortune --favorites --year 1970-1980

# Remove last pick from favorites
disc-fortune --unfavorite-last
```

with:

```markdown
# Mark the last pick as a favorite
disc-fortune --favorite-last

# Favorite a specific album by query (case-insensitive substring of "Artist - Title")
disc-fortune --favorite "kind of blue"

# Favorite by query, narrowed with filters when the query alone is ambiguous
disc-fortune --favorite "miles" --year 1959
disc-fortune --favorite "coltrane" --genre jazz

# Pick randomly from favorites only
disc-fortune --favorites

# Filter within favorites
disc-fortune --favorites --year 1970-1980

# Remove last pick from favorites
disc-fortune --unfavorite-last
```

- [ ] **Step 2: Update the Features list to mention query-based favoriting**

In `README.md`, locate the Features section (currently around lines 68-75). Modify the `**Favorites**` bullet:

Replace:

```markdown
- **Favorites** - Mark albums you love and pick randomly from that subset
```

with:

```markdown
- **Favorites** - Mark albums you love (by last pick or by query) and pick randomly from that subset
```

- [ ] **Step 3: Verify the README reads sensibly**

Open `README.md` and confirm the new examples sit between `--favorite-last` and `--favorites`, and the Features bullet reads naturally.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: document --favorite flag in README"
```

---

## Final verification

- [ ] **Step 1: Run the full test suite**

Run: `go test ./... -v`
Expected: all tests pass — `TestFilterByQuery*`, `TestFavoriteByQuery_*`, and all pre-existing tests.

- [ ] **Step 2: Build cleanly**

Run: `go build -o /tmp/disc-fortune .`
Expected: no errors, no warnings.

- [ ] **Step 3: Confirm git log shows four focused commits**

Run: `git log --oneline -6`
Expected (in order from most recent):
```
<sha> docs: document --favorite flag in README
<sha> feat: add --favorite flag wired to runFavorite + favoriteByQuery
<sha> feat: add favoriteByQuery seam with FavoriteOutcome result type
<sha> feat: add Query field to Filter for artist+title substring matching
<sha> docs: add implementation plan for --favorite by query
<sha> docs: add design for --favorite by query
```
