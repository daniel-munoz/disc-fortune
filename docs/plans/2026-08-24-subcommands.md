# Subcommand CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace disc-fortune's flat flag namespace with subcommands (`disc-fortune list --favorites` instead of `--list --favorites`), released as v2.0.0.

**Architecture:** A `command` table in a new `cli.go` drives dispatch and generates `help`. Each command splits into a pure `parse*` function returning a config struct and an executing `run*` function, so all argument handling is unit-testable without I/O or `os.Exit`. `main.go` sheds the Discogs sync helpers to a new `sync.go` and keeps only `main()`, `fatal`, `formatList`, and command orchestration.

**Tech Stack:** Go 1.24.3, stdlib only (`flag`, no third-party CLI library). Standard `testing` package, table-driven tests.

**Spec:** `docs/plans/2026-08-24-subcommands-design.md`

## Global Constraints

- **No new dependencies.** `go.mod` must keep an empty require block. stdlib `flag` only.
- **Go version:** 1.24.3 (per `go.mod`); do not raise it.
- **Package:** everything is `package main` in the repo root. No subdirectories.
- **Exit codes:** 0 means the command produced what was asked for; 1 means it could not. `fatal` exits 1. Never use `flag.ExitOnError` (it exits 2).
- **Error message style:** usage/validation errors are `<command>: <message>`; operational failures keep `Error: <message>` / `Error loading X: %v`.
- **User-facing text must never reference a v1 flag.** Say `disc-fortune sync`, not `disc-fortune --sync`.
- **Version string:** `2.0.0`, in the `version` const in `main.go`.
- Run `go build ./... && go vet ./... && go test ./...` before every commit.

---

### Task 1: Parsing primitives in `cli.go`

Pure helpers with no I/O. Nothing is wired up yet, so `main.go` still works unchanged and the build stays green.

**Files:**
- Create: `cli.go`
- Test: `cli_test.go`

**Interfaces:**
- Consumes: `ParseYearFilter(string) error` and `Filter` from `filter.go`.
- Produces: `parseInterspersed(*flag.FlagSet, []string) ([]string, error)`, `newFlagSet(string) *flag.FlagSet`, `filterFlags` with methods `Filter() (Filter, error)` and `any() bool`, and `addFilterFlags(*flag.FlagSet) *filterFlags`.

- [ ] **Step 1: Write the failing tests**

Create `cli_test.go`:

```go
package main

import (
	"testing"
)

func TestParseInterspersedFlagsAfterPositional(t *testing.T) {
	fs := newFlagSet("favorite")
	year := fs.String("year", "", "")

	rest, err := parseInterspersed(fs, []string{"miles", "--year", "1959"})
	if err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if len(rest) != 1 || rest[0] != "miles" {
		t.Errorf("positional = %v, want [miles]", rest)
	}
	if *year != "1959" {
		t.Errorf("year = %q, want 1959 (flag after positional was dropped)", *year)
	}
}

func TestParseInterspersedFlagsBeforePositional(t *testing.T) {
	fs := newFlagSet("favorite")
	year := fs.String("year", "", "")

	rest, err := parseInterspersed(fs, []string{"--year", "1959", "miles"})
	if err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if len(rest) != 1 || rest[0] != "miles" {
		t.Errorf("positional = %v, want [miles]", rest)
	}
	if *year != "1959" {
		t.Errorf("year = %q, want 1959", *year)
	}
}

func TestParseInterspersedFlagsSurroundingPositional(t *testing.T) {
	fs := newFlagSet("favorite")
	year := fs.String("year", "", "")
	genre := fs.String("genre", "", "")

	rest, err := parseInterspersed(fs, []string{"--genre", "jazz", "miles", "--year", "1959"})
	if err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if len(rest) != 1 || rest[0] != "miles" {
		t.Errorf("positional = %v, want [miles]", rest)
	}
	if *genre != "jazz" || *year != "1959" {
		t.Errorf("genre = %q, year = %q, want jazz/1959", *genre, *year)
	}
}

func TestParseInterspersedMultiplePositionals(t *testing.T) {
	fs := newFlagSet("favorite")
	rest, err := parseInterspersed(fs, []string{"kind", "of", "blue"})
	if err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if len(rest) != 3 {
		t.Errorf("positional = %v, want 3 items", rest)
	}
}

func TestParseInterspersedDashTerminator(t *testing.T) {
	fs := newFlagSet("favorite")
	rest, err := parseInterspersed(fs, []string{"--", "-live-"})
	if err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if len(rest) != 1 || rest[0] != "-live-" {
		t.Errorf("positional = %v, want [-live-]", rest)
	}
}

func TestParseInterspersedUnknownFlag(t *testing.T) {
	fs := newFlagSet("pick")
	if _, err := parseInterspersed(fs, []string{"--nope"}); err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestFilterFlagsBuildsFilter(t *testing.T) {
	fs := newFlagSet("pick")
	ff := addFilterFlags(fs)
	if _, err := parseInterspersed(fs, []string{"--year", "1970-1980", "--genre", "jazz"}); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	filter, err := ff.Filter()
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if filter.Year != "1970-1980" || filter.Genre != "jazz" {
		t.Errorf("filter = %+v, want Year=1970-1980 Genre=jazz", filter)
	}
	if !ff.any() {
		t.Error("any() = false, want true")
	}
}

func TestFilterFlagsRejectsBadYear(t *testing.T) {
	fs := newFlagSet("pick")
	ff := addFilterFlags(fs)
	if _, err := parseInterspersed(fs, []string{"--year", "nineteen"}); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if _, err := ff.Filter(); err == nil {
		t.Fatal("expected error for non-numeric year")
	}
}

func TestFilterFlagsAnyFalseWhenUnset(t *testing.T) {
	fs := newFlagSet("pick")
	ff := addFilterFlags(fs)
	if _, err := parseInterspersed(fs, nil); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if ff.any() {
		t.Error("any() = true, want false when no filter flags set")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run 'TestParseInterspersed|TestFilterFlags' -v`
Expected: FAIL — build error, `undefined: newFlagSet`, `undefined: parseInterspersed`, `undefined: addFilterFlags`.

- [ ] **Step 3: Write the implementation**

Create `cli.go`:

```go
package main

import (
	"flag"
	"io"
)

// newFlagSet builds a FlagSet that never prints or exits on its own, so the
// caller controls the message and the exit code.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	return fs
}

// parseInterspersed parses args allowing flags to appear before, after, or
// around positional arguments. Go's flag package stops at the first non-flag
// argument, which would silently drop trailing flags such as the --year in
// `favorite "miles" --year 1959`.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return positional, nil
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}

// filterFlags holds the four filter flags shared by pick, list, favorite, and
// unfavorite. Registering them in one place keeps their names and help text
// from drifting apart between commands.
type filterFlags struct {
	year   *string
	genre  *string
	label  *string
	format *string
}

func addFilterFlags(fs *flag.FlagSet) *filterFlags {
	return &filterFlags{
		year:   fs.String("year", "", "Filter by year or year range (e.g., 1975 or 1970-1980)"),
		genre:  fs.String("genre", "", "Filter by genre (case-insensitive substring match)"),
		label:  fs.String("label", "", "Filter by label (case-insensitive substring match)"),
		format: fs.String("format", "", "Filter by format (case-insensitive substring match)"),
	}
}

// Filter builds a Filter from the parsed flags, validating the year format.
func (ff *filterFlags) Filter() (Filter, error) {
	if err := ParseYearFilter(*ff.year); err != nil {
		return Filter{}, err
	}
	return Filter{
		Year:   *ff.year,
		Genre:  *ff.genre,
		Label:  *ff.label,
		Format: *ff.format,
	}, nil
}

// any reports whether any filter flag was set.
func (ff *filterFlags) any() bool {
	return *ff.year != "" || *ff.genre != "" || *ff.label != "" || *ff.format != ""
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go build ./... && go vet ./... && go test ./... -v`
Expected: PASS, including all pre-existing tests.

- [ ] **Step 5: Commit**

```bash
git add cli.go cli_test.go
git commit -m "feat: add interspersed flag parsing and shared filter flags"
```

---

### Task 2: `unfavoriteByQuery` seam

Mirrors the existing `favoriteByQuery`. Purely additive; nothing calls it yet.

**Files:**
- Modify: `favorites.go` (append after `favoriteByQuery`)
- Test: `favorites_test.go` (append)

**Interfaces:**
- Consumes: `removeFavorite(path string, album Album) error`, `ErrNotInFavorites`, `Filter.Apply([]Album) []Album`.
- Produces: `UnfavoriteStatus` constants `UnfavoriteRemoved`, `UnfavoriteNoMatch`, `UnfavoriteMultiMatch`; `UnfavoriteOutcome{Status, Album, Matches}`; `unfavoriteByQuery(favorites []Album, query string, filter Filter, favPath string) (UnfavoriteOutcome, error)`.

- [ ] **Step 1: Write the failing tests**

Append to `favorites_test.go`:

```go
func TestUnfavoriteByQuerySingleMatch(t *testing.T) {
	favPath := filepath.Join(t.TempDir(), "favorites.json")
	album := Album{Artist: "Miles Davis", Title: "Kind of Blue", Year: 1959}
	if err := addFavorite(favPath, album); err != nil {
		t.Fatalf("addFavorite: %v", err)
	}

	favs, err := loadFavorites(favPath)
	if err != nil {
		t.Fatalf("loadFavorites: %v", err)
	}

	outcome, err := unfavoriteByQuery(favs, "kind of blue", Filter{}, favPath)
	if err != nil {
		t.Fatalf("unfavoriteByQuery: %v", err)
	}
	if outcome.Status != UnfavoriteRemoved {
		t.Fatalf("Status = %v, want UnfavoriteRemoved", outcome.Status)
	}
	if outcome.Album.Title != "Kind of Blue" {
		t.Errorf("Album.Title = %q, want Kind of Blue", outcome.Album.Title)
	}

	after, err := loadFavorites(favPath)
	if err != nil {
		t.Fatalf("loadFavorites: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("got %d favorites after removal, want 0", len(after))
	}
}

func TestUnfavoriteByQueryNoMatch(t *testing.T) {
	favPath := filepath.Join(t.TempDir(), "favorites.json")
	album := Album{Artist: "Miles Davis", Title: "Kind of Blue"}
	if err := addFavorite(favPath, album); err != nil {
		t.Fatalf("addFavorite: %v", err)
	}
	favs, _ := loadFavorites(favPath)

	outcome, err := unfavoriteByQuery(favs, "nonexistent", Filter{}, favPath)
	if err != nil {
		t.Fatalf("unfavoriteByQuery: %v", err)
	}
	if outcome.Status != UnfavoriteNoMatch {
		t.Fatalf("Status = %v, want UnfavoriteNoMatch", outcome.Status)
	}

	after, _ := loadFavorites(favPath)
	if len(after) != 1 {
		t.Errorf("got %d favorites, want 1 (unchanged)", len(after))
	}
}

func TestUnfavoriteByQueryMultiMatch(t *testing.T) {
	favPath := filepath.Join(t.TempDir(), "favorites.json")
	for _, a := range []Album{
		{Artist: "Miles Davis", Title: "Kind of Blue"},
		{Artist: "Miles Davis", Title: "Bitches Brew"},
	} {
		if err := addFavorite(favPath, a); err != nil {
			t.Fatalf("addFavorite: %v", err)
		}
	}
	favs, _ := loadFavorites(favPath)

	outcome, err := unfavoriteByQuery(favs, "miles", Filter{}, favPath)
	if err != nil {
		t.Fatalf("unfavoriteByQuery: %v", err)
	}
	if outcome.Status != UnfavoriteMultiMatch {
		t.Fatalf("Status = %v, want UnfavoriteMultiMatch", outcome.Status)
	}
	if len(outcome.Matches) != 2 {
		t.Errorf("got %d matches, want 2", len(outcome.Matches))
	}

	after, _ := loadFavorites(favPath)
	if len(after) != 2 {
		t.Errorf("got %d favorites, want 2 (unchanged)", len(after))
	}
}

func TestUnfavoriteByQueryNarrowedByFilter(t *testing.T) {
	favPath := filepath.Join(t.TempDir(), "favorites.json")
	for _, a := range []Album{
		{Artist: "Miles Davis", Title: "Kind of Blue", Year: 1959},
		{Artist: "Miles Davis", Title: "Bitches Brew", Year: 1970},
	} {
		if err := addFavorite(favPath, a); err != nil {
			t.Fatalf("addFavorite: %v", err)
		}
	}
	favs, _ := loadFavorites(favPath)

	outcome, err := unfavoriteByQuery(favs, "miles", Filter{Year: "1959"}, favPath)
	if err != nil {
		t.Fatalf("unfavoriteByQuery: %v", err)
	}
	if outcome.Status != UnfavoriteRemoved {
		t.Fatalf("Status = %v, want UnfavoriteRemoved", outcome.Status)
	}
	if outcome.Album.Title != "Kind of Blue" {
		t.Errorf("removed %q, want Kind of Blue", outcome.Album.Title)
	}
}

// An album present in the caller's slice but already gone from the file is a
// no-match, not an error: removal is idempotent.
func TestUnfavoriteByQueryAlreadyRemovedIsNoMatch(t *testing.T) {
	favPath := filepath.Join(t.TempDir(), "favorites.json")
	stale := []Album{{Artist: "Miles Davis", Title: "Kind of Blue"}}

	outcome, err := unfavoriteByQuery(stale, "kind of blue", Filter{}, favPath)
	if err != nil {
		t.Fatalf("unfavoriteByQuery: %v", err)
	}
	if outcome.Status != UnfavoriteNoMatch {
		t.Fatalf("Status = %v, want UnfavoriteNoMatch", outcome.Status)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestUnfavoriteByQuery -v`
Expected: FAIL — build error, `undefined: unfavoriteByQuery`, `undefined: UnfavoriteRemoved`.

- [ ] **Step 3: Write the implementation**

Append to `favorites.go`:

```go
// UnfavoriteStatus represents the outcome of attempting to unfavorite an album by query.
type UnfavoriteStatus int

const (
	UnfavoriteRemoved UnfavoriteStatus = iota
	UnfavoriteNoMatch
	UnfavoriteMultiMatch
)

// UnfavoriteOutcome holds the result of unfavoriteByQuery.
type UnfavoriteOutcome struct {
	Status  UnfavoriteStatus
	Album   Album   // populated when Status is UnfavoriteRemoved
	Matches []Album // populated when Status is UnfavoriteMultiMatch
}

// unfavoriteByQuery is the testable core of `unfavorite QUERY`. It applies the
// query+filter to the favorites list — not the collection, since favorites is
// the set being removed from — and removes the album when exactly one matches.
// An album that is already absent is reported as UnfavoriteNoMatch rather than
// an error: removal is idempotent.
func unfavoriteByQuery(favorites []Album, query string, filter Filter, favPath string) (UnfavoriteOutcome, error) {
	filter.Query = query
	matches := filter.Apply(favorites)
	switch len(matches) {
	case 0:
		return UnfavoriteOutcome{Status: UnfavoriteNoMatch}, nil
	case 1:
		if err := removeFavorite(favPath, matches[0]); err != nil {
			if errors.Is(err, ErrNotInFavorites) {
				return UnfavoriteOutcome{Status: UnfavoriteNoMatch}, nil
			}
			return UnfavoriteOutcome{}, err
		}
		return UnfavoriteOutcome{Status: UnfavoriteRemoved, Album: matches[0]}, nil
	default:
		return UnfavoriteOutcome{Status: UnfavoriteMultiMatch, Matches: matches}, nil
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go build ./... && go vet ./... && go test ./... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add favorites.go favorites_test.go
git commit -m "feat: add unfavoriteByQuery seam with UnfavoriteOutcome result type"
```

---

### Task 3: Checked loaders for collection and favorites

The load-collection-or-explain block is copy-pasted three times in `main.go` and the load-favorites block twice. These helpers give each one testable core plus a single place where the new exit codes and the new `disc-fortune sync` wording live. Not wired up yet.

**Files:**
- Modify: `collection.go` (append)
- Modify: `favorites.go` (append)
- Test: `collection_test.go` (append), `favorites_test.go` (append)

**Interfaces:**
- Consumes: `loadCollectionFrom(path string) ([]Album, error)`, `loadFavorites(path string) ([]Album, error)`.
- Produces: `errNoCollection`, `errEmptyCollection`, `loadCollectionChecked(path string) ([]Album, error)`; `errNoFavorites`, `loadFavoritesChecked(path string) ([]Album, error)`.

- [ ] **Step 1: Write the failing tests**

Append to `collection_test.go`:

```go
func TestLoadCollectionCheckedMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "collection.json")
	_, err := loadCollectionChecked(path)
	if !errors.Is(err, errNoCollection) {
		t.Errorf("err = %v, want errNoCollection", err)
	}
}

func TestLoadCollectionCheckedEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "collection.json")
	if err := saveCollectionTo(path, []Album{}); err != nil {
		t.Fatalf("saveCollectionTo: %v", err)
	}
	_, err := loadCollectionChecked(path)
	if !errors.Is(err, errEmptyCollection) {
		t.Errorf("err = %v, want errEmptyCollection", err)
	}
}

func TestLoadCollectionCheckedPopulated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "collection.json")
	if err := saveCollectionTo(path, []Album{{Artist: "Ride", Title: "Nowhere"}}); err != nil {
		t.Fatalf("saveCollectionTo: %v", err)
	}
	albums, err := loadCollectionChecked(path)
	if err != nil {
		t.Fatalf("loadCollectionChecked: %v", err)
	}
	if len(albums) != 1 {
		t.Errorf("got %d albums, want 1", len(albums))
	}
}
```

Append to `favorites_test.go`:

```go
func TestLoadFavoritesCheckedEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "favorites.json")
	_, err := loadFavoritesChecked(path)
	if !errors.Is(err, errNoFavorites) {
		t.Errorf("err = %v, want errNoFavorites", err)
	}
}

func TestLoadFavoritesCheckedPopulated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "favorites.json")
	if err := addFavorite(path, Album{Artist: "Ride", Title: "Nowhere"}); err != nil {
		t.Fatalf("addFavorite: %v", err)
	}
	favs, err := loadFavoritesChecked(path)
	if err != nil {
		t.Fatalf("loadFavoritesChecked: %v", err)
	}
	if len(favs) != 1 {
		t.Errorf("got %d favorites, want 1", len(favs))
	}
}
```

Note: add `"errors"` to `collection_test.go`'s import block (it already has `path/filepath` and `testing`). `favorites_test.go` already imports all three.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run 'Checked' -v`
Expected: FAIL — build error, `undefined: loadCollectionChecked`, `undefined: errNoCollection`, `undefined: loadFavoritesChecked`.

- [ ] **Step 3: Write the implementation**

Append to `collection.go` (add `"errors"` to its import block):

```go
var (
	// errNoCollection means no collection file exists yet.
	errNoCollection = errors.New("no collection")
	// errEmptyCollection means the collection file exists but holds no albums.
	errEmptyCollection = errors.New("collection is empty")
)

// loadCollectionChecked loads the collection and distinguishes the two
// "nothing to work with" states from genuine load failures, so callers can
// print the right guidance without repeating the checks.
func loadCollectionChecked(path string) ([]Album, error) {
	albums, err := loadCollectionFrom(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errNoCollection
		}
		return nil, err
	}
	if len(albums) == 0 {
		return nil, errEmptyCollection
	}
	return albums, nil
}
```

Append to `favorites.go` (its import block already has `"errors"`):

```go
// errNoFavorites means the favorites list is empty or absent.
var errNoFavorites = errors.New("no favorites")

// loadFavoritesChecked loads favorites and reports an empty list as
// errNoFavorites, so callers can print guidance without repeating the check.
func loadFavoritesChecked(path string) ([]Album, error) {
	favorites, err := loadFavorites(path)
	if err != nil {
		return nil, err
	}
	if len(favorites) == 0 {
		return nil, errNoFavorites
	}
	return favorites, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go build ./... && go vet ./... && go test ./... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add collection.go collection_test.go favorites.go favorites_test.go
git commit -m "feat: add checked collection and favorites loaders"
```

---

### Task 4: Command table, dispatch, and rewired orchestration

The whole CLI switchover. The command table and the `run*` functions it calls have to land together — the table's `run` closures reference the new signatures — so this is one task with one commit rather than two that each leave a red build.

The spec sketches this split as `parsePick` / `parseList` returning a `pickConfig`. They parse identically, so the plan consolidates them into one `parseSelection(name, args)` returning a `selection`; the `name` parameter is what makes error messages say `pick:` or `list:`.

**Files:**
- Modify: `cli.go` (append), `main.go` (replace `main()` and all `run*`), `main_test.go` (remove the moved tests)
- Create: `sync.go`, `sync_test.go`
- Test: `cli_test.go` (append)

**Interfaces:**
- Consumes: `newFlagSet`, `parseInterspersed`, `addFilterFlags`, `filterFlags` (Task 1); `unfavoriteByQuery`, `UnfavoriteOutcome` (Task 2); `loadCollectionChecked`, `loadFavoritesChecked`, `errNoCollection`, `errEmptyCollection`, `errNoFavorites` (Task 3).
- Produces: `command` struct; `commands []command`; `lookup(name string) *command`; `resolve(args []string) (*command, []string, error)`; `dispatch([]string)`; config types `selection{favoritesOnly bool, filter Filter}`, `favoriteConfig{query string, filter Filter}`, `historyConfig{limit int}`, `syncConfig{folders []string}`; parsers `parseSelection(name string, args []string) (selection, error)`, `parseFavorite(name string, args []string) (favoriteConfig, error)`, `parseHistory(args []string) (historyConfig, error)`, `parseSync(args []string) (syncConfig, error)`, `parseNoArgs(name string, args []string) error`; `helpText(topic string) (string, error)`; `runPick(selection)`, `runList(selection)`, `runHistory(historyConfig)`, `runFavorite(favoriteConfig)`, `runUnfavorite(favoriteConfig)`, `runSync(syncConfig)`, `runFolders()`, `loadCollectionOrExit() []Album`, `loadFavoritesOrExit() []Album`, `selectAlbums(selection) []Album`.

- [ ] **Step 1: Write the failing tests**

Append to `cli_test.go`:

```go
func TestParseSelectionFavoritesFlag(t *testing.T) {
	cfg, err := parseSelection("list", []string{"--favorites"})
	if err != nil {
		t.Fatalf("parseSelection: %v", err)
	}
	if !cfg.favoritesOnly {
		t.Error("favoritesOnly = false, want true")
	}
}

func TestParseSelectionFilters(t *testing.T) {
	cfg, err := parseSelection("pick", []string{"--year", "1970-1980", "--genre", "jazz"})
	if err != nil {
		t.Fatalf("parseSelection: %v", err)
	}
	if cfg.filter.Year != "1970-1980" || cfg.filter.Genre != "jazz" {
		t.Errorf("filter = %+v", cfg.filter)
	}
}

func TestParseSelectionRejectsPositional(t *testing.T) {
	if _, err := parseSelection("pick", []string{"1975"}); err == nil {
		t.Fatal("expected error for unexpected positional argument")
	}
}

func TestParseSelectionRejectsBadYear(t *testing.T) {
	if _, err := parseSelection("pick", []string{"--year", "nineteen"}); err == nil {
		t.Fatal("expected error for invalid year")
	}
}

func TestParseFavoriteBareMeansLastPick(t *testing.T) {
	cfg, err := parseFavorite("favorite", nil)
	if err != nil {
		t.Fatalf("parseFavorite: %v", err)
	}
	if cfg.query != "" {
		t.Errorf("query = %q, want empty (last pick)", cfg.query)
	}
}

func TestParseFavoriteWithQuery(t *testing.T) {
	cfg, err := parseFavorite("favorite", []string{"kind of blue"})
	if err != nil {
		t.Fatalf("parseFavorite: %v", err)
	}
	if cfg.query != "kind of blue" {
		t.Errorf("query = %q, want 'kind of blue'", cfg.query)
	}
}

func TestParseFavoriteQueryWithTrailingFilter(t *testing.T) {
	cfg, err := parseFavorite("favorite", []string{"miles", "--year", "1959"})
	if err != nil {
		t.Fatalf("parseFavorite: %v", err)
	}
	if cfg.query != "miles" {
		t.Errorf("query = %q, want miles", cfg.query)
	}
	if cfg.filter.Year != "1959" {
		t.Errorf("filter.Year = %q, want 1959 (trailing filter was dropped)", cfg.filter.Year)
	}
}

func TestParseFavoriteEmptyQueryRejected(t *testing.T) {
	if _, err := parseFavorite("favorite", []string{""}); err == nil {
		t.Fatal("expected error for explicit empty query")
	}
}

func TestParseFavoriteTooManyArguments(t *testing.T) {
	if _, err := parseFavorite("favorite", []string{"kind", "of", "blue"}); err == nil {
		t.Fatal("expected error for unquoted multi-word query")
	}
}

func TestParseFavoriteFiltersRequireQuery(t *testing.T) {
	if _, err := parseFavorite("favorite", []string{"--year", "1959"}); err == nil {
		t.Fatal("expected error for filters with no query")
	}
}

func TestParseFavoriteRejectsFavoritesFlag(t *testing.T) {
	if _, err := parseFavorite("favorite", []string{"--favorites", "miles"}); err == nil {
		t.Fatal("expected error: --favorites is not registered on favorite")
	}
}

func TestParseHistoryDefault(t *testing.T) {
	cfg, err := parseHistory(nil)
	if err != nil {
		t.Fatalf("parseHistory: %v", err)
	}
	if cfg.limit != defaultHistoryLimit {
		t.Errorf("limit = %d, want %d", cfg.limit, defaultHistoryLimit)
	}
}

func TestParseHistoryExplicit(t *testing.T) {
	cfg, err := parseHistory([]string{"25"})
	if err != nil {
		t.Fatalf("parseHistory: %v", err)
	}
	if cfg.limit != 25 {
		t.Errorf("limit = %d, want 25", cfg.limit)
	}
}

func TestParseHistoryZeroMeansAll(t *testing.T) {
	cfg, err := parseHistory([]string{"0"})
	if err != nil {
		t.Fatalf("parseHistory: %v", err)
	}
	if cfg.limit != 0 {
		t.Errorf("limit = %d, want 0 (all)", cfg.limit)
	}
}

func TestParseHistoryNonNumeric(t *testing.T) {
	if _, err := parseHistory([]string{"abc"}); err == nil {
		t.Fatal("expected error for non-numeric count")
	}
}

func TestParseHistoryNegative(t *testing.T) {
	if _, err := parseHistory([]string{"-5"}); err == nil {
		t.Fatal("expected error for negative count")
	}
}

func TestParseSyncFolders(t *testing.T) {
	cfg, err := parseSync([]string{"--folder", "Vinyl 12\"", "--folder", "Vinyl 7\""})
	if err != nil {
		t.Fatalf("parseSync: %v", err)
	}
	if len(cfg.folders) != 2 || cfg.folders[0] != "Vinyl 12\"" {
		t.Errorf("folders = %v, want two entries", cfg.folders)
	}
}

func TestParseSyncRejectsListFolders(t *testing.T) {
	if _, err := parseSync([]string{"--list-folders"}); err == nil {
		t.Fatal("expected error: --list-folders is now the `folders` command")
	}
}

func TestParseNoArgsRejectsArgument(t *testing.T) {
	if err := parseNoArgs("folders", []string{"extra"}); err == nil {
		t.Fatal("expected error for unexpected argument")
	}
}

func TestResolveEmptyArgsIsPick(t *testing.T) {
	cmd, rest, err := resolve(nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if cmd.name != "pick" {
		t.Errorf("command = %q, want pick", cmd.name)
	}
	if len(rest) != 0 {
		t.Errorf("rest = %v, want empty", rest)
	}
}

func TestResolveLeadingFlagIsPick(t *testing.T) {
	cmd, rest, err := resolve([]string{"--year", "1975"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if cmd.name != "pick" {
		t.Errorf("command = %q, want pick", cmd.name)
	}
	if len(rest) != 2 || rest[0] != "--year" {
		t.Errorf("rest = %v, want [--year 1975]", rest)
	}
}

func TestResolveNamedCommand(t *testing.T) {
	cmd, rest, err := resolve([]string{"list", "--favorites"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if cmd.name != "list" {
		t.Errorf("command = %q, want list", cmd.name)
	}
	if len(rest) != 1 || rest[0] != "--favorites" {
		t.Errorf("rest = %v, want [--favorites]", rest)
	}
}

func TestResolveHelpFlags(t *testing.T) {
	for _, arg := range []string{"-h", "--help", "-help"} {
		cmd, _, err := resolve([]string{arg})
		if err != nil {
			t.Fatalf("resolve(%q): %v", arg, err)
		}
		if cmd.name != "help" {
			t.Errorf("resolve(%q) = %q, want help", arg, cmd.name)
		}
	}
}

func TestResolveVersionFlagIsSignpost(t *testing.T) {
	for _, arg := range []string{"-v", "--version", "-version"} {
		_, _, err := resolve([]string{arg})
		if err == nil {
			t.Fatalf("resolve(%q): expected an error pointing at `disc-fortune version`", arg)
		}
		if !strings.Contains(err.Error(), "disc-fortune version") {
			t.Errorf("resolve(%q) error = %q, want it to name `disc-fortune version`", arg, err)
		}
	}
}

func TestResolveUnknownCommand(t *testing.T) {
	_, _, err := resolve([]string{"frobnicate"})
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
	if !strings.Contains(err.Error(), "frobnicate") {
		t.Errorf("error = %q, want it to name the unknown command", err)
	}
}

func TestEveryCommandIsDocumented(t *testing.T) {
	if len(commands) == 0 {
		t.Fatal("commands table is empty")
	}
	for _, c := range commands {
		if c.name == "" {
			t.Error("a command has an empty name")
		}
		if c.summary == "" {
			t.Errorf("command %q has no summary", c.name)
		}
		if c.usage == "" {
			t.Errorf("command %q has no usage text", c.name)
		}
		if c.run == nil {
			t.Errorf("command %q has no run function", c.name)
		}
	}
}

func TestHelpListsEveryCommand(t *testing.T) {
	out, err := helpText("")
	if err != nil {
		t.Fatalf("helpText: %v", err)
	}
	for _, c := range commands {
		if !strings.Contains(out, c.name) {
			t.Errorf("help output missing command %q", c.name)
		}
	}
}

func TestHelpForOneCommand(t *testing.T) {
	out, err := helpText("sync")
	if err != nil {
		t.Fatalf("helpText: %v", err)
	}
	if !strings.Contains(out, "--folder") {
		t.Errorf("help sync output missing --folder: %q", out)
	}
}

func TestHelpUnknownTopic(t *testing.T) {
	if _, err := helpText("frobnicate"); err == nil {
		t.Fatal("expected error for unknown help topic")
	}
}
```

Add `"strings"` to the `cli_test.go` import block.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run 'TestParse|TestResolve|TestHelp|TestEveryCommand' -v`
Expected: FAIL — build error, `undefined: parseSelection`, `undefined: resolve`, `undefined: commands`, `undefined: defaultHistoryLimit`.

- [ ] **Step 3: Write the implementation**

Append to `cli.go`, and extend its import block to `"flag"`, `"fmt"`, `"io"`, `"strconv"`, `"strings"`:

```go
// defaultHistoryLimit is how many past picks `history` shows with no argument.
const defaultHistoryLimit = 10

// command is one subcommand in the CLI. help is generated from the table, so a
// command cannot ship undocumented.
type command struct {
	name    string
	summary string // one line, listed by `help`
	usage   string // full block, shown by `help <cmd>` and on usage error
	run     func(args []string)
}

// commands is the full CLI surface. It is populated in init rather than as a
// package-level literal because help reads from it, which would otherwise be
// an initialization cycle.
var commands []command

// selection is the parsed form of the flags shared by pick and list.
type selection struct {
	favoritesOnly bool
	filter        Filter
}

// favoriteConfig is the parsed form of favorite and unfavorite. An empty query
// means "the last pick".
type favoriteConfig struct {
	query  string
	filter Filter
}

// historyConfig is the parsed form of history. A limit of 0 means "all".
type historyConfig struct {
	limit int
}

// syncConfig is the parsed form of sync.
type syncConfig struct {
	folders []string
}

func parseSelection(name string, args []string) (selection, error) {
	fs := newFlagSet(name)
	favoritesOnly := fs.Bool("favorites", false, "Restrict to favorites only")
	ff := addFilterFlags(fs)

	rest, err := parseInterspersed(fs, args)
	if err != nil {
		return selection{}, fmt.Errorf("%s: %w", name, err)
	}
	if len(rest) > 0 {
		return selection{}, fmt.Errorf("%s: unexpected argument %q", name, rest[0])
	}
	filter, err := ff.Filter()
	if err != nil {
		return selection{}, fmt.Errorf("%s: %v", name, err)
	}
	return selection{favoritesOnly: *favoritesOnly, filter: filter}, nil
}

func parseFavorite(name string, args []string) (favoriteConfig, error) {
	fs := newFlagSet(name)
	ff := addFilterFlags(fs)

	rest, err := parseInterspersed(fs, args)
	if err != nil {
		return favoriteConfig{}, fmt.Errorf("%s: %w", name, err)
	}
	if len(rest) > 1 {
		return favoriteConfig{}, fmt.Errorf(
			"%s: too many arguments (quote the query: %s %q)",
			name, name, strings.Join(rest, " "))
	}
	filter, err := ff.Filter()
	if err != nil {
		return favoriteConfig{}, fmt.Errorf("%s: %v", name, err)
	}

	if len(rest) == 0 {
		if ff.any() {
			return favoriteConfig{}, fmt.Errorf("%s: filters require a query", name)
		}
		return favoriteConfig{filter: filter}, nil
	}

	query := strings.TrimSpace(rest[0])
	if query == "" {
		return favoriteConfig{}, fmt.Errorf("%s: requires a query", name)
	}
	return favoriteConfig{query: query, filter: filter}, nil
}

func parseHistory(args []string) (historyConfig, error) {
	fs := newFlagSet("history")
	rest, err := parseInterspersed(fs, args)
	if err != nil {
		return historyConfig{}, fmt.Errorf("history: %w", err)
	}
	if len(rest) > 1 {
		return historyConfig{}, fmt.Errorf("history: too many arguments")
	}
	limit := defaultHistoryLimit
	if len(rest) == 1 {
		n, err := strconv.Atoi(strings.TrimSpace(rest[0]))
		if err != nil {
			return historyConfig{}, fmt.Errorf("history: requires a number (e.g., history 20)")
		}
		if n < 0 {
			return historyConfig{}, fmt.Errorf("history: count cannot be negative")
		}
		limit = n
	}
	return historyConfig{limit: limit}, nil
}

func parseSync(args []string) (syncConfig, error) {
	fs := newFlagSet("sync")
	var folders arrayFlags
	fs.Var(&folders, "folder", "Sync only specific folder(s) by name (repeatable)")

	rest, err := parseInterspersed(fs, args)
	if err != nil {
		return syncConfig{}, fmt.Errorf("sync: %w", err)
	}
	if len(rest) > 0 {
		return syncConfig{}, fmt.Errorf("sync: unexpected argument %q", rest[0])
	}
	return syncConfig{folders: folders}, nil
}

// parseNoArgs validates that a command was invoked with no flags and no arguments.
func parseNoArgs(name string, args []string) error {
	fs := newFlagSet(name)
	rest, err := parseInterspersed(fs, args)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if len(rest) > 0 {
		return fmt.Errorf("%s: unexpected argument %q", name, rest[0])
	}
	return nil
}

// lookup returns the named command, or nil.
func lookup(name string) *command {
	for i := range commands {
		if commands[i].name == name {
			return &commands[i]
		}
	}
	return nil
}

// resolve maps raw argv (without the program name) to a command and its
// arguments. Empty argv, or a leading flag, means the implicit pick.
func resolve(args []string) (*command, []string, error) {
	if len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "-help":
			return lookup("help"), nil, nil
		case "-v", "--version", "-version":
			return nil, nil, fmt.Errorf(
				"there is no %s flag; use `disc-fortune version`", args[0])
		}
	}
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		args = append([]string{"pick"}, args...)
	}
	cmd := lookup(args[0])
	if cmd == nil {
		return nil, nil, fmt.Errorf(
			"unknown command %q\nRun `disc-fortune help` for usage.", args[0])
	}
	return cmd, args[1:], nil
}

// helpText renders the general help, or one command's usage block.
func helpText(topic string) (string, error) {
	if topic != "" {
		c := lookup(topic)
		if c == nil {
			return "", fmt.Errorf("help: unknown command %q", topic)
		}
		return c.usage, nil
	}

	var sb strings.Builder
	sb.WriteString("disc-fortune - randomly picks a record from your Discogs collection\n\n")
	sb.WriteString("Usage:\n  disc-fortune [command] [flags]\n\n")
	sb.WriteString("Commands:\n")
	for _, c := range commands {
		sb.WriteString(fmt.Sprintf("  %-11s %s\n", c.name, c.summary))
	}
	sb.WriteString("\nRun `disc-fortune help <command>` for details on a command.\n")
	sb.WriteString("With no command, disc-fortune picks a random album.\n")
	return sb.String(), nil
}

const filterFlagHelp = `  --year VALUE     Filter by year or year range (e.g., 1975 or 1970-1980)
  --genre VALUE    Filter by genre (case-insensitive substring match)
  --label VALUE    Filter by label (case-insensitive substring match)
  --format VALUE   Filter by format (case-insensitive substring match)`

func init() {
	commands = []command{
		{
			name:    "pick",
			summary: "Print a random album (default)",
			usage: `Usage: disc-fortune pick [flags]

Prints one random album from your collection and records it in history.
This is what runs when you give no command at all.

Flags:
  --favorites      Pick from favorites only
` + filterFlagHelp,
			run: func(args []string) {
				cfg, err := parseSelection("pick", args)
				if err != nil {
					fatal("%v", err)
				}
				runPick(cfg)
			},
		},
		{
			name:    "list",
			summary: "List every matching album",
			usage: `Usage: disc-fortune list [flags]

Prints every album matching the filters, with a count.

Flags:
  --favorites      List favorites only
` + filterFlagHelp,
			run: func(args []string) {
				cfg, err := parseSelection("list", args)
				if err != nil {
					fatal("%v", err)
				}
				runList(cfg)
			},
		},
		{
			name:    "sync",
			summary: "Fetch your collection from Discogs",
			usage: `Usage: disc-fortune sync [--folder NAME ...]

Fetches your Discogs collection and caches it locally. Requires DISCOGS_TOKEN
to be set. With no --folder, syncs everything.

Flags:
  --folder NAME    Sync only this folder (repeatable)

Run ` + "`disc-fortune folders`" + ` to see available folder names.`,
			run: func(args []string) {
				cfg, err := parseSync(args)
				if err != nil {
					fatal("%v", err)
				}
				runSync(cfg)
			},
		},
		{
			name:    "folders",
			summary: "List your Discogs folder names",
			usage: `Usage: disc-fortune folders

Lists the folder names in your Discogs collection, for use with
` + "`disc-fortune sync --folder`" + `. Requires DISCOGS_TOKEN to be set.`,
			run: func(args []string) {
				if err := parseNoArgs("folders", args); err != nil {
					fatal("%v", err)
				}
				runFolders()
			},
		},
		{
			name:    "history",
			summary: "Show recent picks",
			usage: `Usage: disc-fortune history [N]

Shows the last N picks. N defaults to 10; 0 shows all of them.`,
			run: func(args []string) {
				cfg, err := parseHistory(args)
				if err != nil {
					fatal("%v", err)
				}
				runHistory(cfg)
			},
		},
		{
			name:    "favorite",
			summary: "Add an album to favorites",
			usage: `Usage: disc-fortune favorite [QUERY] [flags]

With no QUERY, favorites the last pick. With a QUERY, favorites the one
album in your collection whose "Artist - Title" contains it, case-insensitively.
If the query matches several albums, they are listed and nothing is added;
narrow it with filters.

Flags (only valid alongside a QUERY):
` + filterFlagHelp,
			run: func(args []string) {
				cfg, err := parseFavorite("favorite", args)
				if err != nil {
					fatal("%v", err)
				}
				runFavorite(cfg)
			},
		},
		{
			name:    "unfavorite",
			summary: "Remove an album from favorites",
			usage: `Usage: disc-fortune unfavorite [QUERY] [flags]

With no QUERY, unfavorites the last pick. With a QUERY, removes the one
favorite whose "Artist - Title" contains it, case-insensitively. Removing
something that is not favorited succeeds quietly.

Flags (only valid alongside a QUERY):
` + filterFlagHelp,
			run: func(args []string) {
				cfg, err := parseFavorite("unfavorite", args)
				if err != nil {
					fatal("%v", err)
				}
				runUnfavorite(cfg)
			},
		},
		{
			name:    "version",
			summary: "Print the version",
			usage:   "Usage: disc-fortune version\n\nPrints the disc-fortune version and exits.",
			run: func(args []string) {
				if err := parseNoArgs("version", args); err != nil {
					fatal("%v", err)
				}
				fmt.Printf("disc-fortune %s\n", version)
			},
		},
		{
			name:    "help",
			summary: "Show help for a command",
			usage:   "Usage: disc-fortune help [COMMAND]\n\nShows general help, or detailed help for one command.",
			run: func(args []string) {
				topic := ""
				if len(args) > 1 {
					fatal("help: too many arguments")
				}
				if len(args) == 1 {
					topic = args[0]
				}
				out, err := helpText(topic)
				if err != nil {
					fatal("%v", err)
				}
				fmt.Println(out)
			},
		},
	}
}

// dispatch resolves argv and runs the chosen command.
func dispatch(args []string) {
	cmd, rest, err := resolve(args)
	if err != nil {
		fatal("disc-fortune: %v", err)
	}
	cmd.run(rest)
}
```

Note: this code references `runPick`, `runList`, `runSync`, `runFolders`, `runHistory`, `runFavorite`, and `runUnfavorite` with their new signatures. Those arrive in Step 6 of this same task, so the build stays red between Step 3 and Step 6. That is expected — do not commit mid-task.

- [ ] **Step 4: Create `sync.go` with the moved Discogs helpers**

Create `sync.go`, moving `arrayFlags`, `resolveFolderIDs`, `collectAlbums`, `resolveFolderNames`, and `printFolders` verbatim out of `main.go`, with `runSync` reshaped and `runFolders` split out:

```go
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
```

- [ ] **Step 5: Move the sync tests into `sync_test.go`**

Create `sync_test.go` by cutting `TestResolveFolderNames`, `TestResolveFolderNamesMultiple`, `TestResolveFolderNamesNotFound`, and `TestCollectAlbumsDeduplicates` out of `main_test.go` **with their bodies unchanged**. The new file's import block is:

```go
package main

import (
	"encoding/json"
	"net/http"
	"testing"
)
```

After the cut, `main_test.go` keeps only the four `formatList` tests (`TestRunListOutput`, `TestRunListEmpty`, `TestRunListSeparator`, `TestRunListSingular`), and its import block reduces to:

```go
package main

import (
	"strings"
	"testing"
)
```

- [ ] **Step 6: Rewrite `main.go`**

Replace the whole file with:

```go
package main

import (
	"errors"
	"fmt"
	"os"
)

const version = "2.0.0"

// fatal prints an error message to stderr and exits.
func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func main() {
	dispatch(os.Args[1:])
}

// loadCollectionOrExit loads the collection, explaining what to do and exiting 1
// when there is nothing to work with.
func loadCollectionOrExit() []Album {
	albums, err := loadCollectionChecked(collectionPath())
	switch {
	case errors.Is(err, errNoCollection):
		fatal("No collection found. Run `disc-fortune sync` to fetch your Discogs collection.")
	case errors.Is(err, errEmptyCollection):
		fatal("Collection is empty. Run `disc-fortune sync` to fetch your Discogs collection.")
	case err != nil:
		fatal("Error loading collection: %v", err)
	}
	return albums
}

// loadFavoritesOrExit loads favorites, explaining what to do and exiting 1 when
// there are none.
func loadFavoritesOrExit() []Album {
	favorites, err := loadFavoritesChecked(favoritesPath())
	switch {
	case errors.Is(err, errNoFavorites):
		fatal("No favorites yet. Use `disc-fortune favorite` after a pick you like.")
	case err != nil:
		fatal("Error loading favorites: %v", err)
	}
	return favorites
}

// selectAlbums loads the collection or favorites per cfg and applies its filter.
func selectAlbums(cfg selection) []Album {
	var albums []Album
	if cfg.favoritesOnly {
		albums = loadFavoritesOrExit()
	} else {
		albums = loadCollectionOrExit()
	}
	return cfg.filter.Apply(albums)
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
	noun := "albums"
	if len(albums) == 1 {
		noun = "album"
	}
	sb.WriteString(fmt.Sprintf("\n\n%d %s\n", len(albums), noun))
	return sb.String()
}

func runPick(cfg selection) {
	albums := selectAlbums(cfg)
	if len(albums) == 0 {
		fmt.Println("No albums match the specified filters")
		os.Exit(1)
	}

	album := randomAlbum(albums)

	if err := addToHistory(historyPath(), album); err != nil {
		fatal("Error saving history: %v", err)
	}

	fmt.Println(formatAlbum(album, isTTY(os.Stdout)))
}

func runList(cfg selection) {
	albums := selectAlbums(cfg)
	fmt.Print(formatList(albums, isTTY(os.Stdout)))
	if len(albums) == 0 {
		os.Exit(1)
	}
}

func runHistory(cfg historyConfig) {
	entries, err := loadHistory(historyPath())
	if err != nil {
		fatal("Error loading history: %v", err)
	}

	limit := cfg.limit
	if limit == 0 {
		limit = len(entries) // 0 means show all
	}

	fmt.Print(formatHistory(entries, limit, isTTY(os.Stdout)))
}

func runFavorite(cfg favoriteConfig) {
	if cfg.query == "" {
		favoriteLastPick()
		return
	}

	albums := loadCollectionOrExit()
	outcome, err := favoriteByQuery(albums, cfg.query, cfg.filter, favoritesPath())
	if err != nil {
		fatal("Error adding favorite: %v", err)
	}

	switch outcome.Status {
	case FavoriteAdded:
		fmt.Printf("Added to favorites: %s - %s\n", outcome.Album.Artist, outcome.Album.Title)
	case FavoriteAlreadyFav:
		fmt.Println("Already in favorites")
	case FavoriteNoMatch:
		fatal("No albums match query %q", cfg.query)
	case FavoriteMultiMatch:
		fmt.Print(formatList(outcome.Matches, isTTY(os.Stdout)))
		fmt.Println("Be more specific or add filters.")
		os.Exit(1)
	}
}

func runUnfavorite(cfg favoriteConfig) {
	if cfg.query == "" {
		unfavoriteLastPick()
		return
	}

	favorites := loadFavoritesOrExit()
	outcome, err := unfavoriteByQuery(favorites, cfg.query, cfg.filter, favoritesPath())
	if err != nil {
		fatal("Error removing favorite: %v", err)
	}

	switch outcome.Status {
	case UnfavoriteRemoved:
		fmt.Printf("Removed from favorites: %s - %s\n", outcome.Album.Artist, outcome.Album.Title)
	case UnfavoriteNoMatch:
		// Removal is idempotent: nothing to remove is a success.
		fmt.Printf("No favorites match %q - nothing to remove.\n", cfg.query)
	case UnfavoriteMultiMatch:
		fmt.Print(formatList(outcome.Matches, isTTY(os.Stdout)))
		fmt.Println("Be more specific or add filters.")
		os.Exit(1)
	}
}

func favoriteLastPick() {
	entries, err := loadHistory(historyPath())
	if err != nil {
		fatal("Error loading history: %v", err)
	}
	if len(entries) == 0 {
		fatal("No history to favorite")
	}

	lastAlbum := entries[len(entries)-1].Album
	if err := addFavorite(favoritesPath(), lastAlbum); err != nil {
		if errors.Is(err, ErrAlreadyInFavorites) {
			fmt.Println("Already in favorites")
			return
		}
		fatal("Error adding favorite: %v", err)
	}

	fmt.Printf("Added to favorites: %s - %s\n", lastAlbum.Artist, lastAlbum.Title)
}

func unfavoriteLastPick() {
	entries, err := loadHistory(historyPath())
	if err != nil {
		fatal("Error loading history: %v", err)
	}
	if len(entries) == 0 {
		fatal("No history to unfavorite")
	}

	lastAlbum := entries[len(entries)-1].Album
	if err := removeFavorite(favoritesPath(), lastAlbum); err != nil {
		if errors.Is(err, ErrNotInFavorites) {
			fmt.Println("Last pick was not in favorites")
			return
		}
		fatal("Error removing favorite: %v", err)
	}

	fmt.Printf("Removed from favorites: %s - %s\n", lastAlbum.Artist, lastAlbum.Title)
}
```

Note: `formatList` uses `strings.Builder`, so `main.go`'s import block is `"errors"`, `"fmt"`, `"os"`, `"strings"`. Fix the import block to match what the compiler asks for.

- [ ] **Step 7: Build and run the full suite**

Run: `go build ./... && go vet ./... && go test ./... -v`
Expected: PASS. All Task 1–4 tests now compile and pass, and the four `formatList` tests plus the moved sync tests still pass.

- [ ] **Step 8: Smoke-test the real binary and its exit codes**

Run each line and check the reported status. `$?` is echoed after every command:

```bash
go build -o /tmp/disc-fortune .

/tmp/disc-fortune version;              echo "want 0, got $?"
/tmp/disc-fortune help;                 echo "want 0, got $?"
/tmp/disc-fortune help sync;            echo "want 0, got $?"
/tmp/disc-fortune -h            >/dev/null; echo "want 0, got $?"
/tmp/disc-fortune frobnicate    2>&1 | head -2; echo "want 'unknown command'"
/tmp/disc-fortune --version     2>&1 | head -1; echo "want a pointer to 'disc-fortune version'"
/tmp/disc-fortune help frobnicate 2>&1 | head -1; echo "want 'help: unknown command'"
/tmp/disc-fortune favorite kind of blue 2>&1 | head -1; echo "want 'too many arguments'"
/tmp/disc-fortune favorite --year 1959  2>&1 | head -1; echo "want 'filters require a query'"
/tmp/disc-fortune pick 1975             2>&1 | head -1; echo "want 'unexpected argument'"
/tmp/disc-fortune history abc           2>&1 | head -1; echo "want 'requires a number'"
/tmp/disc-fortune sync --list-folders   2>&1 | head -1; echo "want a flag-not-defined error"
```

Then verify the exit code for a missing collection, using a throwaway HOME so your real one is untouched:

```bash
HOME=$(mktemp -d) /tmp/disc-fortune;      echo "want 1, got $?"
HOME=$(mktemp -d) /tmp/disc-fortune list; echo "want 1, got $?"
HOME=$(mktemp -d) /tmp/disc-fortune pick --favorites; echo "want 1, got $?"
```

Expected: every command prints the message named on its line, and the three `HOME=` invocations report `1` while telling you to run `disc-fortune sync`. If any message still says `--sync` or `--favorite-last`, fix it before committing.

- [ ] **Step 9: Commit**

```bash
git add cli.go cli_test.go main.go main_test.go sync.go sync_test.go
git commit -m "feat!: replace flags with subcommands

BREAKING CHANGE: disc-fortune now uses subcommands. --list becomes list,
--sync becomes sync, --sync --list-folders becomes folders, --favorite-last
becomes favorite, and --version becomes version. Bare invocation still picks
a random album. Exit codes are normalized: 0 when the command produced what
was asked for, 1 when it could not."
```

---

### Task 5: Documentation and release notes

**Files:**
- Modify: `README.md`
- Create: `RELEASE_NOTES_v2.0.0.md`

**Interfaces:**
- Consumes: the final command surface from Task 4's table.
- Produces: nothing code depends on.

- [ ] **Step 1: Rewrite the README Usage section**

Replace everything from the `## Usage` heading to the end of that section with:

````markdown
## Usage

```sh
# Print a random album (the default — no command needed)
disc-fortune

# Filter by year or year range
disc-fortune --year 1975
disc-fortune --year 1970-1980

# Filter by genre, label, or format
disc-fortune --genre jazz
disc-fortune --label blue-note
disc-fortune --format 12\"

# Combine filters
disc-fortune --year 1970-1980 --genre jazz

# Pick randomly from favorites only
disc-fortune pick --favorites
```

### Commands

| Command | What it does |
|---|---|
| `pick` | Print a random album. Runs by default when you give no command. |
| `list` | List every matching album, with a count. |
| `sync` | Fetch your collection from Discogs. |
| `folders` | List your Discogs folder names. |
| `history` | Show recent picks. |
| `favorite` | Add an album to favorites. |
| `unfavorite` | Remove an album from favorites. |
| `version` | Print the version. |
| `help` | Show help for a command. |

Run `disc-fortune help <command>` for details on any of them.

### Syncing

```sh
# Sync your full Discogs collection
disc-fortune sync

# List available folder names
disc-fortune folders

# Sync only specific folders
disc-fortune sync --folder "Vinyl 12\"" --folder "Vinyl 7\""
```

### Listing

```sh
# Every album in the collection
disc-fortune list

# Everything matching a filter
disc-fortune list --year 1970-1980 --genre jazz

# Favorites only
disc-fortune list --favorites
```

### History

```sh
disc-fortune history        # last 10 picks
disc-fortune history 25     # last 25 picks
disc-fortune history 0      # all picks
```

### Favorites

```sh
# Favorite the last pick
disc-fortune favorite

# Favorite a specific album (case-insensitive substring of "Artist - Title")
disc-fortune favorite "kind of blue"

# Narrow an ambiguous query with filters
disc-fortune favorite "miles" --year 1959
disc-fortune favorite "coltrane" --genre jazz

# Remove the last pick from favorites
disc-fortune unfavorite

# Remove a specific album from favorites
disc-fortune unfavorite "kind of blue"
```

### Exit codes

`disc-fortune` exits 0 when the command produced what you asked for, and 1 when
it could not — no collection synced yet, no albums matching your filters, an
ambiguous favorite query, or a usage error. Removing a favorite that is not
there exits 0, since the end state you asked for already holds.
````

- [ ] **Step 2: Write the release notes**

Create `RELEASE_NOTES_v2.0.0.md`:

```markdown
# disc-fortune v2.0.0

**Breaking release.** disc-fortune now uses subcommands instead of a flat set of
flags. Running `disc-fortune` with no arguments still prints a random album, so
the most common invocation is unchanged, but every other v1 flag has moved.

## Migration

| v1 | v2 |
|---|---|
| `disc-fortune` | `disc-fortune` (or `disc-fortune pick`) |
| `disc-fortune --year 1975 --genre jazz` | `disc-fortune --year 1975 --genre jazz` (unchanged) |
| `disc-fortune --list` | `disc-fortune list` |
| `disc-fortune --list --favorites` | `disc-fortune list --favorites` |
| `disc-fortune --favorites` | `disc-fortune pick --favorites` |
| `disc-fortune --sync` | `disc-fortune sync` |
| `disc-fortune --sync --folder "Vinyl 12\""` | `disc-fortune sync --folder "Vinyl 12\""` |
| `disc-fortune --sync --list-folders` | `disc-fortune folders` |
| `disc-fortune --history 25` | `disc-fortune history 25` |
| `disc-fortune --favorite-last` | `disc-fortune favorite` |
| `disc-fortune --unfavorite-last` | `disc-fortune unfavorite` |
| `disc-fortune --favorite "kind of blue"` | `disc-fortune favorite "kind of blue"` |
| `disc-fortune --version` | `disc-fortune version` |

Filter flags (`--year`, `--genre`, `--label`, `--format`) keep their names and
now work on `pick`, `list`, `favorite`, and `unfavorite`.

## New

- `disc-fortune help` and `disc-fortune help <command>`, generated from the
  command table
- `disc-fortune folders`, promoted from `--sync --list-folders` — it never
  synced anything anyway
- `disc-fortune unfavorite "query"`, to remove a specific album from favorites
  without it being the last pick
- Filters can now appear before or after a query, so both
  `favorite "miles" --year 1959` and `favorite --year 1959 "miles"` work

## Changed exit codes

v1 exited 0 in several cases where nothing had happened. v2 uses one rule:
**0 means the command produced what you asked for, 1 means it could not.**

| Situation | v1 | v2 |
|---|---|---|
| No collection file, or collection empty | 0 | 1 |
| No favorites yet | 0 | 1 |
| `pick` — no albums match | 0 | 1 |
| `list` — no albums match | 0 | 1 |
| `unfavorite` — no match | 1 | 0 |

Removing a favorite that is not there now exits 0: the end state you asked for
already holds, the same way `rm -f` succeeds on a missing file. Scripts that
check `$?` may need updating.

## Unchanged

Your `collection.json`, `favorites.json`, and `history.json` are untouched. You
can downgrade to v1 without touching your data.
```

- [ ] **Step 3: Verify no stale flag references remain**

Run:

```bash
grep -n 'disc-fortune --' README.md RELEASE_NOTES_v2.0.0.md *.go
```

Expected: no output. (`docs/plans/` and `RELEASE_NOTES_v1.*.md` are historical records and are intentionally left alone, so they are not searched.)

Then confirm the README's command list matches the binary:

```bash
go build -o /tmp/disc-fortune . && /tmp/disc-fortune help
```

Expected: every command in the README's Commands table appears in the output, and vice versa.

- [ ] **Step 4: Commit**

```bash
git add README.md RELEASE_NOTES_v2.0.0.md
git commit -m "docs: document subcommand CLI and v2.0.0 migration"
```
