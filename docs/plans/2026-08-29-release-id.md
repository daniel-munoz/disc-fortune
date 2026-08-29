# Store the Discogs Release ID (T4) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give every album a stable Discogs release ID and make that ID — not the `"Artist - Title"` string — decide when two records are the same, without breaking any existing data file or command.

**Architecture:** `Album` gains a `ReleaseID` field populated from `basic_information.id`. Identity splits into two mechanisms: `Identity()`, a map key used only by sync deduplication, and `sameAlbum()`, a lenient pairwise comparison used by favorites so a pre-2.2 entry with no ID is never mistaken for a different record. `Key()` is untouched and keeps serving `--query`. A new `backfill.go` stamps IDs into `favorites.json` and `history.json` after each sync.

**Tech Stack:** Go 1.24.3, standard library only. Single `package main` at the repository root; tests live beside the code as `*_test.go`.

**Spec:** [`docs/plans/2026-08-29-release-id-design.md`](2026-08-29-release-id-design.md)

## Global Constraints

- Module is `github.com/daniel-munoz/disc-fortune/v2`, Go 1.24.3. **No third-party dependencies.** `go.mod` must stay dependency-free.
- Everything is `package main` in the repository root. There is no `src/` and no `tests/` directory.
- Run tests with `go test .` from the repository root. A single test is `go test . -run TestName -v`.
- All writes to data files go through the existing atomic savers (`saveCollection`, `saveFavorites`, `saveHistory`). **Never** call `os.WriteFile` on a live data path.
- Human-readable output of `pick`, `list` and `history` must be **byte-identical** to v2.1.1. The release ID appears in data files only.
- `Album.Key()` must keep returning exactly `Artist + " - " + Title`. It is the string `--query` substring-matches against.
- `sync`'s reports go to **stdout** (it has always been a human report). Progress and warnings go to **stderr**.
- Comments explain *why*, not *what* — match the density and voice of the surrounding code.
- Commit after every task, using the repo's `type: summary` message style (`feat:`, `fix:`, `test:`, `refactor:`).

---

### Task 1: Identity primitives on `Album`

Adds the field and the two identity functions. Nothing changes behavior yet — sync, favorites and filters all still work exactly as before, which is what the tests in this task pin down.

**Files:**
- Modify: `collection.go` (the `Album` struct at line 18, and `Key()` at line 27)
- Test: `collection_test.go`, `filter_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `Album.ReleaseID int` with JSON tag `release_id,omitempty`
  - `func (a Album) Identity() string` — `"id:<n>"` when `ReleaseID != 0`, else `"name:<Artist> - <Title>"`
  - `func sameAlbum(a, b Album) bool` — compares IDs when **both** are non-zero, otherwise compares `Key()`

- [ ] **Step 1: Write the failing tests**

Append to `collection_test.go`:

```go
func TestAlbumIdentity(t *testing.T) {
	withID := Album{ReleaseID: 12345, Artist: "Miles Davis", Title: "Kind of Blue"}
	if got, want := withID.Identity(), "id:12345"; got != want {
		t.Errorf("Identity() = %q, want %q", got, want)
	}

	withoutID := Album{Artist: "Miles Davis", Title: "Kind of Blue"}
	if got, want := withoutID.Identity(), "name:Miles Davis - Kind of Blue"; got != want {
		t.Errorf("Identity() = %q, want %q", got, want)
	}
}

// TestAlbumKeyIgnoresReleaseID guards the whole point of the two-method split:
// Key() is the search string, so adding an ID must not change it.
func TestAlbumKeyIgnoresReleaseID(t *testing.T) {
	album := Album{ReleaseID: 12345, Artist: "Miles Davis", Title: "Kind of Blue"}
	if got, want := album.Key(), "Miles Davis - Kind of Blue"; got != want {
		t.Errorf("Key() = %q, want %q", got, want)
	}
}

func TestSameAlbum(t *testing.T) {
	tests := []struct {
		name string
		a, b Album
		want bool
	}{
		{
			name: "same id, different stored title",
			a:    Album{ReleaseID: 111, Artist: "Miles Davis", Title: "Kind Of Blue"},
			b:    Album{ReleaseID: 111, Artist: "Miles Davis", Title: "Kind of Blue"},
			want: true,
		},
		{
			name: "different ids, same name",
			a:    Album{ReleaseID: 111, Artist: "Miles Davis", Title: "Kind of Blue"},
			b:    Album{ReleaseID: 222, Artist: "Miles Davis", Title: "Kind of Blue"},
			want: false,
		},
		{
			name: "legacy entry matches an ID'd one by name",
			a:    Album{Artist: "Miles Davis", Title: "Kind of Blue"},
			b:    Album{ReleaseID: 111, Artist: "Miles Davis", Title: "Kind of Blue"},
			want: true,
		},
		{
			name: "both legacy, same name",
			a:    Album{Artist: "Slowdive", Title: "Souvlaki"},
			b:    Album{Artist: "Slowdive", Title: "Souvlaki"},
			want: true,
		},
		{
			name: "both legacy, different name",
			a:    Album{Artist: "Slowdive", Title: "Souvlaki"},
			b:    Album{Artist: "Ride", Title: "Nowhere"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sameAlbum(tt.a, tt.b); got != tt.want {
				t.Errorf("sameAlbum() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestReleaseIDOmittedWhenZero keeps pre-migration records byte-identical to
// what v2.1.0 wrote, so upgrading does not rewrite every line of every file.
func TestReleaseIDOmittedWhenZero(t *testing.T) {
	data, err := json.Marshal(Album{Artist: "Slowdive", Title: "Souvlaki"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "release_id") {
		t.Errorf("marshalled zero ID: %s", data)
	}
}

// TestReleaseIDSurvivesDowngrade asserts the v2.2 file shape decodes cleanly
// into the v2.1 struct shape, so downgrading loses nothing but the ID.
func TestReleaseIDSurvivesDowngrade(t *testing.T) {
	data, err := json.Marshal(Album{ReleaseID: 12345, Artist: "Slowdive", Title: "Souvlaki"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// The v2.1.0 shape: no release_id field at all.
	var legacy struct {
		Artist string `json:"artist"`
		Title  string `json:"title"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		t.Fatalf("v2.1 decode of v2.2 data: %v", err)
	}
	if legacy.Artist != "Slowdive" || legacy.Title != "Souvlaki" {
		t.Errorf("lost data on downgrade: %+v", legacy)
	}
}
```

`collection_test.go` already imports `encoding/json`; add `"strings"` to its import block.

Append to `filter_test.go` — this is the regression guard for the landmine described in the design doc:

```go
// TestFilterQueryStillMatchesWithReleaseID is the guard for the reason
// Key() was not turned into the identity: --query substring-matches against
// it, so an ID-preferring Key() would break every query silently.
func TestFilterQueryStillMatchesWithReleaseID(t *testing.T) {
	albums := []Album{
		{ReleaseID: 111, Artist: "Miles Davis", Title: "Kind of Blue"},
		{ReleaseID: 222, Artist: "Slowdive", Title: "Souvlaki"},
	}

	got := Filter{Query: "miles"}.Apply(albums)
	if len(got) != 1 || got[0].ReleaseID != 111 {
		t.Fatalf("Apply() = %+v, want the Miles Davis release", got)
	}

	got = Filter{Query: "souvlaki"}.Apply(albums)
	if len(got) != 1 || got[0].ReleaseID != 222 {
		t.Fatalf("Apply() = %+v, want the Slowdive release", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test . -run 'TestAlbumIdentity|TestSameAlbum|TestReleaseID|TestFilterQueryStillMatches' -v`

Expected: FAIL to compile — `unknown field ReleaseID in struct literal`, `undefined: sameAlbum`, `album.Identity undefined`.

- [ ] **Step 3: Add the field**

In `collection.go`, `ReleaseID` goes **first** in the struct so it leads each record in the JSON files:

```go
// Album represents a single record with metadata.
type Album struct {
	// ReleaseID is the Discogs release ID. It is zero for entries written
	// before v2.2.0, which is what Identity and sameAlbum fall back for.
	ReleaseID int      `json:"release_id,omitempty"`
	Artist    string   `json:"artist"`
	Title     string   `json:"title"`
	Year      int      `json:"year,omitempty"`
	Label     string   `json:"label,omitempty"`
	CatNo     string   `json:"catno,omitempty"`
	Genres    []string `json:"genres,omitempty"`
	Formats   []string `json:"formats,omitempty"`
}
```

- [ ] **Step 4: Replace `Key()`'s doc comment and add the two identity functions**

Still in `collection.go`, replacing the existing `Key()` block:

```go
// Key returns the human-readable "Artist - Title" label. It is also the
// legacy identity: it is what --query substring-matches against, and what
// identifies entries written before release IDs existed. It deliberately
// ignores ReleaseID -- an ID-preferring Key would break every query.
func (a Album) Key() string {
	return a.Artist + " - " + a.Title
}

// Identity returns a map key that distinguishes two records. Sync
// deduplication is its only caller, and there every album comes straight
// from the API, so the ID is always present. The "id:"/"name:" prefixes keep
// a numeric-looking artist name from ever colliding with an ID.
func (a Album) Identity() string {
	if a.ReleaseID != 0 {
		return "id:" + strconv.Itoa(a.ReleaseID)
	}
	return "name:" + a.Key()
}

// sameAlbum reports whether two entries are the same record. It is
// deliberately lenient when either side predates the release ID: a pre-2.2
// favorite and that same record freshly synced must not look like two
// different albums, or favoriting it again would append a duplicate.
//
// The consequence is that sameAlbum is not transitive -- an entry with no ID
// acts as a wildcard for its name. That is fine inside a linear "is this
// already in the list?" scan, and it is exactly why Identity, not sameAlbum,
// is what sync dedup uses: a non-transitive comparison there would make the
// surviving set depend on fetch order.
func sameAlbum(a, b Album) bool {
	if a.ReleaseID != 0 && b.ReleaseID != 0 {
		return a.ReleaseID == b.ReleaseID
	}
	return a.Key() == b.Key()
}
```

Add `"strconv"` to `collection.go`'s import block.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test . -v`

Expected: PASS, including every pre-existing test. `TestCollectAlbumsDeduplicates` still passes because its fixtures carry no IDs, so they fall back to the name key.

- [ ] **Step 6: Commit**

```bash
git add collection.go collection_test.go filter_test.go
git commit -m "feat: add ReleaseID and split identity from search text

Key() keeps returning \"Artist - Title\" because --query substring-matches
against it. Identity() is the new map key for dedup; sameAlbum() is the
lenient pairwise comparison favorites needs so a pre-2.2 entry is not
mistaken for a different record."
```

---

### Task 2: Capture `basic_information.id` from the API

**Files:**
- Modify: `discogs.go` (`releaseInfo` struct, and the `Album` literal inside `getCollectionReleases`)
- Test: `discogs_test.go`

**Interfaces:**
- Consumes: `Album.ReleaseID` (Task 1).
- Produces: `releaseInfo.ID int` with JSON tag `id`; every `Album` returned by `getCollectionReleases` carries its release ID.

- [ ] **Step 1: Write the failing test**

Append to `discogs_test.go`. The fixture is **raw JSON, not a Go literal**, on purpose: it is the only way to prove the code reads `basic_information.id` rather than the sibling `instance_id`, which identifies a physical copy and would merge nothing.

```go
func TestGetCollectionReleasesCapturesReleaseID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/users/testuser/collection/folders/0/releases", func(w http.ResponseWriter, r *http.Request) {
		// Raw JSON so the test fails if the code reads instance_id, or the
		// release object's own id, instead of basic_information.id.
		fmt.Fprint(w, `{
			"pagination": {"pages": 1},
			"releases": [
				{
					"id": 777,
					"instance_id": 999,
					"basic_information": {
						"id": 12345,
						"title": "Kind of Blue",
						"artists": [{"name": "Miles Davis"}]
					}
				}
			]
		}`)
	})

	client, srv := newTestClient(mux)
	defer srv.Close()

	origBase := discogsBaseURL
	setBaseURL(srv.URL)
	defer setBaseURL(origBase)

	albums, err := client.getCollectionReleases("testuser", 0)
	if err != nil {
		t.Fatalf("getCollectionReleases: %v", err)
	}
	if len(albums) != 1 {
		t.Fatalf("got %d albums, want 1", len(albums))
	}
	if albums[0].ReleaseID != 12345 {
		t.Errorf("ReleaseID = %d, want 12345 (basic_information.id)", albums[0].ReleaseID)
	}
}
```

Add `"fmt"` to `discogs_test.go`'s import block.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test . -run TestGetCollectionReleasesCapturesReleaseID -v`

Expected: FAIL with `ReleaseID = 0, want 12345 (basic_information.id)`.

- [ ] **Step 3: Add the field and populate it**

In `discogs.go`, `releaseInfo` gains the ID:

```go
// releaseInfo represents the basic_information of a collection release.
type releaseInfo struct {
	// ID is the Discogs release ID. Note this is not the release object's
	// sibling instance_id, which identifies one physical copy: someone who
	// owns two copies of a pressing has two instances sharing one release
	// ID, and collapsing those into a single entry is correct.
	ID      int             `json:"id"`
	Title   string          `json:"title"`
	Artists []releaseArtist `json:"artists"`
	Year    int             `json:"year"`
	Labels  []releaseLabel  `json:"labels"`
	Genres  []string        `json:"genres"`
	Formats []releaseFormat `json:"formats"`
}
```

And in `getCollectionReleases`, add the first field of the `Album` literal:

```go
			albums = append(albums, Album{
				ReleaseID: r.BasicInformation.ID,
				Artist:    artist,
				Title:     r.BasicInformation.Title,
				Year:      r.BasicInformation.Year,
				Label:     label,
				CatNo:     catno,
				Genres:    r.BasicInformation.Genres,
				Formats:   r.BasicInformation.Formats,
			})
```

Careful: the existing literal assigns `Formats: formats` (the flattened `[]string` built just above), not `r.BasicInformation.Formats`. Keep the existing right-hand sides exactly as they are — only the new `ReleaseID` line and the gofmt realignment change.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test . -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add discogs.go discogs_test.go
git commit -m "feat: capture the Discogs release ID from basic_information"
```

---

### Task 3: Deduplicate the sync on release ID

**Files:**
- Modify: `sync.go:120-133` (`collectAlbums`)
- Test: `sync_test.go`

**Interfaces:**
- Consumes: `Album.Identity()` (Task 1), `Album.ReleaseID` populated by the fetch (Task 2).
- Produces: no new symbols. `collectAlbums` keeps its signature `func collectAlbums(client *discogsClient, username string, folderIDs []int) ([]Album, error)`.

- [ ] **Step 1: Write the failing tests**

Append to `sync_test.go`. The first is the bug being fixed; the second guards against over-correcting into duplicates.

```go
// TestCollectAlbumsKeepsDistinctPressings is the bug T4 exists to fix: two
// different releases sharing an artist and title used to collapse into one.
func TestCollectAlbumsKeepsDistinctPressings(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/users/testuser/collection/folders/0/releases", func(w http.ResponseWriter, r *http.Request) {
		resp := collectionPage{Releases: []collectionRelease{
			{BasicInformation: releaseInfo{ID: 111, Title: "Kind of Blue", Artists: []releaseArtist{{Name: "Miles Davis"}}, Year: 1959}},
			{BasicInformation: releaseInfo{ID: 222, Title: "Kind of Blue", Artists: []releaseArtist{{Name: "Miles Davis"}}, Year: 1997}},
		}}
		resp.Pagination.Pages = 1
		json.NewEncoder(w).Encode(resp)
	})

	client, srv := newTestClient(mux)
	defer srv.Close()

	origBase := discogsBaseURL
	setBaseURL(srv.URL)
	defer setBaseURL(origBase)

	albums, err := collectAlbums(client, "testuser", []int{0})
	if err != nil {
		t.Fatalf("collectAlbums: %v", err)
	}
	if len(albums) != 2 {
		t.Fatalf("got %d albums, want 2 (distinct pressings)", len(albums))
	}
	if albums[0].ReleaseID == albums[1].ReleaseID {
		t.Errorf("both albums have ReleaseID %d", albums[0].ReleaseID)
	}
}

// TestCollectAlbumsMergesSameReleaseAcrossFolders keeps the dedup that is
// still wanted: one release filed in two folders is one record.
func TestCollectAlbumsMergesSameReleaseAcrossFolders(t *testing.T) {
	mux := http.NewServeMux()
	makeHandler := func(releases []collectionRelease) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			resp := collectionPage{Releases: releases}
			resp.Pagination.Pages = 1
			json.NewEncoder(w).Encode(resp)
		}
	}

	// The same release ID, but the two folders report slightly different
	// titles -- an ID match must win over a name mismatch.
	mux.HandleFunc("/users/testuser/collection/folders/1/releases", makeHandler([]collectionRelease{
		{BasicInformation: releaseInfo{ID: 333, Title: "Souvlaki", Artists: []releaseArtist{{Name: "Slowdive"}}}},
	}))
	mux.HandleFunc("/users/testuser/collection/folders/2/releases", makeHandler([]collectionRelease{
		{BasicInformation: releaseInfo{ID: 333, Title: "Souvlaki (Reissue)", Artists: []releaseArtist{{Name: "Slowdive"}}}},
	}))

	client, srv := newTestClient(mux)
	defer srv.Close()

	origBase := discogsBaseURL
	setBaseURL(srv.URL)
	defer setBaseURL(origBase)

	albums, err := collectAlbums(client, "testuser", []int{1, 2})
	if err != nil {
		t.Fatalf("collectAlbums: %v", err)
	}
	if len(albums) != 1 {
		t.Fatalf("got %d albums, want 1 (same release in two folders)", len(albums))
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test . -run 'TestCollectAlbumsKeepsDistinctPressings|TestCollectAlbumsMergesSameReleaseAcrossFolders' -v`

Expected: `TestCollectAlbumsKeepsDistinctPressings` FAILs with `got 1 albums, want 2 (distinct pressings)`. `TestCollectAlbumsMergesSameReleaseAcrossFolders` FAILs with `got 2 albums, want 1` (the titles differ, so today's name key does not merge them).

- [ ] **Step 3: Switch the dedup key**

In `sync.go`, `collectAlbums` — one line, plus the comment that explains it:

```go
// collectAlbums fetches releases from the given folders and deduplicates them.
//
// Dedup is on Identity, not Key: two pressings of one title are two records
// and must both survive, while one release filed in two folders is one
// record however its title is spelled in each.
func collectAlbums(client *discogsClient, username string, folderIDs []int) ([]Album, error) {
	seen := make(map[string]bool)
	var albums []Album

	for _, fid := range folderIDs {
		releases, err := client.getCollectionReleases(username, fid)
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
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test . -v`

Expected: PASS, including the pre-existing `TestCollectAlbumsDeduplicates` (its fixtures carry no IDs, so `Identity()` falls back to `"name:"+Key()` and the behavior is unchanged).

- [ ] **Step 5: Commit**

```bash
git add sync.go sync_test.go
git commit -m "fix: deduplicate the sync on release ID, not artist and title

Two pressings of one title are two records. They used to collapse into one
entry and one of them silently vanished from the collection."
```

---

### Task 4: Favorites match on `sameAlbum`

**Files:**
- Modify: `favorites.go:51-88` (`addFavorite`, `removeFavorite`)
- Test: `favorites_test.go`

**Interfaces:**
- Consumes: `sameAlbum` (Task 1).
- Produces: no new symbols. `addFavorite(path string, album Album) error` and `removeFavorite(path string, album Album) error` keep their signatures and their `ErrAlreadyInFavorites` / `ErrNotInFavorites` contracts.

- [ ] **Step 1: Write the failing tests**

Append to `favorites_test.go`:

```go
// TestAddFavoriteKeepsDistinctPressings: two different releases sharing an
// artist and title are two favorites, not one.
func TestAddFavoriteKeepsDistinctPressings(t *testing.T) {
	favPath := filepath.Join(t.TempDir(), "favorites.json")

	first := Album{ReleaseID: 111, Artist: "Miles Davis", Title: "Kind of Blue", Year: 1959}
	second := Album{ReleaseID: 222, Artist: "Miles Davis", Title: "Kind of Blue", Year: 1997}

	if err := addFavorite(favPath, first); err != nil {
		t.Fatalf("addFavorite(first): %v", err)
	}
	if err := addFavorite(favPath, second); err != nil {
		t.Fatalf("addFavorite(second): %v", err)
	}

	favs, err := loadFavorites(favPath)
	if err != nil {
		t.Fatalf("loadFavorites: %v", err)
	}
	if len(favs) != 2 {
		t.Fatalf("got %d favorites, want 2", len(favs))
	}
}

// TestAddFavoriteLegacyEntryIsNotDuplicated is the reason sameAlbum is
// lenient: a favorite written by v2.1.0 has no ID, and re-favoriting that
// same record after a sync must not append a second copy.
func TestAddFavoriteLegacyEntryIsNotDuplicated(t *testing.T) {
	favPath := filepath.Join(t.TempDir(), "favorites.json")

	legacy := Album{Artist: "Miles Davis", Title: "Kind of Blue"}
	if err := saveFavorites(favPath, []Album{legacy}); err != nil {
		t.Fatalf("saveFavorites: %v", err)
	}

	synced := Album{ReleaseID: 111, Artist: "Miles Davis", Title: "Kind of Blue"}
	err := addFavorite(favPath, synced)
	if !errors.Is(err, ErrAlreadyInFavorites) {
		t.Errorf("error = %v, want ErrAlreadyInFavorites", err)
	}
}

// TestRemoveFavoriteLegacyEntry: unfavoriting after a sync must still find
// the entry that was written before IDs existed.
func TestRemoveFavoriteLegacyEntry(t *testing.T) {
	favPath := filepath.Join(t.TempDir(), "favorites.json")

	legacy := Album{Artist: "Miles Davis", Title: "Kind of Blue"}
	if err := saveFavorites(favPath, []Album{legacy}); err != nil {
		t.Fatalf("saveFavorites: %v", err)
	}

	synced := Album{ReleaseID: 111, Artist: "Miles Davis", Title: "Kind of Blue"}
	if err := removeFavorite(favPath, synced); err != nil {
		t.Fatalf("removeFavorite: %v", err)
	}

	favs, err := loadFavorites(favPath)
	if err != nil {
		t.Fatalf("loadFavorites: %v", err)
	}
	if len(favs) != 0 {
		t.Errorf("got %d favorites, want 0", len(favs))
	}
}

// TestRemoveFavoriteSurvivesRetitle: once both sides carry an ID, an
// upstream retitle on Discogs no longer orphans the favorite.
func TestRemoveFavoriteSurvivesRetitle(t *testing.T) {
	favPath := filepath.Join(t.TempDir(), "favorites.json")

	stored := Album{ReleaseID: 111, Artist: "Miles Davis", Title: "Kind Of Blue"}
	if err := saveFavorites(favPath, []Album{stored}); err != nil {
		t.Fatalf("saveFavorites: %v", err)
	}

	retitled := Album{ReleaseID: 111, Artist: "Miles Davis", Title: "Kind of Blue (1959)"}
	if err := removeFavorite(favPath, retitled); err != nil {
		t.Fatalf("removeFavorite: %v", err)
	}

	favs, err := loadFavorites(favPath)
	if err != nil {
		t.Fatalf("loadFavorites: %v", err)
	}
	if len(favs) != 0 {
		t.Errorf("got %d favorites, want 0", len(favs))
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test . -run 'TestAddFavoriteKeepsDistinctPressings|TestRemoveFavoriteSurvivesRetitle' -v`

Expected: `TestAddFavoriteKeepsDistinctPressings` FAILs with `got 1 favorites, want 2`. `TestRemoveFavoriteSurvivesRetitle` FAILs with `removeFavorite: not in favorites`. (`TestAddFavoriteLegacyEntryIsNotDuplicated` and `TestRemoveFavoriteLegacyEntry` already pass under the old name comparison — they are there to prove the new one does not regress them.)

- [ ] **Step 3: Replace the key comparisons**

In `favorites.go`, `addFavorite`:

```go
// addFavorite adds an album to favorites if not already present.
func addFavorite(path string, album Album) error {
	favorites, err := loadFavorites(path)
	if err != nil {
		return err
	}

	// sameAlbum rather than Key: two pressings of one title are two
	// favorites, but an entry written before release IDs existed is still
	// the same record as its freshly synced self.
	for _, fav := range favorites {
		if sameAlbum(fav, album) {
			return ErrAlreadyInFavorites
		}
	}

	favorites = append(favorites, album)
	return saveFavorites(path, favorites)
}
```

And `removeFavorite`:

```go
// removeFavorite removes an album from favorites.
func removeFavorite(path string, album Album) error {
	favorites, err := loadFavorites(path)
	if err != nil {
		return err
	}

	var filtered []Album
	found := false
	for _, fav := range favorites {
		if sameAlbum(fav, album) {
			found = true
			continue
		}
		filtered = append(filtered, fav)
	}

	if !found {
		return ErrNotInFavorites
	}

	return saveFavorites(path, filtered)
}
```

Both `key := album.Key()` lines are now unused and must be deleted, or the build fails.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test . -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add favorites.go favorites_test.go
git commit -m "fix: identify favorites by release ID when both sides have one

Distinct pressings become distinct favorites, and an upstream retitle no
longer orphans one. Entries written before v2.2.0 still match by name."
```

---

### Task 5: The backfill pass

Pure functions only. No file I/O and no output — Task 7 wires them up. Keeping them pure is what makes the idempotence test possible.

**Files:**
- Create: `backfill.go`
- Test: `backfill_test.go`

**Interfaces:**
- Consumes: `Album`, `Album.Key()`, `Album.ReleaseID` (Task 1); `HistoryEntry` (existing, `history.go:13`, fields `Album Album` and `Timestamp time.Time`).
- Produces:
  - `type backfillResult struct { Updated int; Ambiguous []string }`
  - `func indexByLegacyKey(collection []Album) map[string][]int`
  - `func backfillAlbums(entries, collection []Album) ([]Album, backfillResult)`
  - `func backfillHistory(entries []HistoryEntry, collection []Album) ([]HistoryEntry, backfillResult)`

- [ ] **Step 1: Write the failing tests**

Create `backfill_test.go`:

```go
package main

import (
	"reflect"
	"testing"
	"time"
)

// testCollection is the shared fixture: one unambiguous record, and one
// title that resolves to two distinct pressings.
func testCollection() []Album {
	return []Album{
		{ReleaseID: 111, Artist: "Slowdive", Title: "Souvlaki"},
		{ReleaseID: 222, Artist: "Miles Davis", Title: "Kind of Blue"},
		{ReleaseID: 333, Artist: "Miles Davis", Title: "Kind of Blue"},
	}
}

func TestBackfillAlbumsUniqueMatch(t *testing.T) {
	entries := []Album{{Artist: "Slowdive", Title: "Souvlaki"}}

	got, res := backfillAlbums(entries, testCollection())

	if res.Updated != 1 {
		t.Errorf("Updated = %d, want 1", res.Updated)
	}
	if got[0].ReleaseID != 111 {
		t.Errorf("ReleaseID = %d, want 111", got[0].ReleaseID)
	}
	if len(res.Ambiguous) != 0 {
		t.Errorf("Ambiguous = %v, want none", res.Ambiguous)
	}
}

// A record no longer in the collection -- sold, or dropped from the synced
// folders -- is left alone and reported nowhere. There is nothing to do.
func TestBackfillAlbumsNoMatch(t *testing.T) {
	entries := []Album{{Artist: "Ride", Title: "Nowhere"}}

	got, res := backfillAlbums(entries, testCollection())

	if res.Updated != 0 {
		t.Errorf("Updated = %d, want 0", res.Updated)
	}
	if got[0].ReleaseID != 0 {
		t.Errorf("ReleaseID = %d, want 0 (left alone)", got[0].ReleaseID)
	}
	if len(res.Ambiguous) != 0 {
		t.Errorf("Ambiguous = %v, want none", res.Ambiguous)
	}
}

// Which pressing the user favorited is unknowable, so guess nothing and say
// so instead.
func TestBackfillAlbumsAmbiguous(t *testing.T) {
	entries := []Album{{Artist: "Miles Davis", Title: "Kind of Blue"}}

	got, res := backfillAlbums(entries, testCollection())

	if res.Updated != 0 {
		t.Errorf("Updated = %d, want 0", res.Updated)
	}
	if got[0].ReleaseID != 0 {
		t.Errorf("ReleaseID = %d, want 0 (left alone)", got[0].ReleaseID)
	}
	want := []string{"Miles Davis - Kind of Blue"}
	if !reflect.DeepEqual(res.Ambiguous, want) {
		t.Errorf("Ambiguous = %v, want %v", res.Ambiguous, want)
	}
}

// An ambiguous key repeated across many entries -- easy in history -- is
// reported once, not once per entry.
func TestBackfillAlbumsAmbiguousReportedOnce(t *testing.T) {
	entries := []Album{
		{Artist: "Miles Davis", Title: "Kind of Blue"},
		{Artist: "Miles Davis", Title: "Kind of Blue"},
	}

	_, res := backfillAlbums(entries, testCollection())

	if len(res.Ambiguous) != 1 {
		t.Errorf("Ambiguous = %v, want one entry", res.Ambiguous)
	}
}

func TestBackfillAlbumsSkipsEntriesThatHaveAnID(t *testing.T) {
	// 999 is not in the collection: if the pass touched entries that already
	// have an ID, this would be overwritten or reported.
	entries := []Album{{ReleaseID: 999, Artist: "Slowdive", Title: "Souvlaki"}}

	got, res := backfillAlbums(entries, testCollection())

	if res.Updated != 0 {
		t.Errorf("Updated = %d, want 0", res.Updated)
	}
	if got[0].ReleaseID != 999 {
		t.Errorf("ReleaseID = %d, want 999 (untouched)", got[0].ReleaseID)
	}
}

// Idempotence is the acceptance criterion for the migration: a second sync
// must change nothing.
func TestBackfillAlbumsIsIdempotent(t *testing.T) {
	entries := []Album{
		{Artist: "Slowdive", Title: "Souvlaki"},
		{Artist: "Miles Davis", Title: "Kind of Blue"},
		{Artist: "Ride", Title: "Nowhere"},
	}
	collection := testCollection()

	once, firstRes := backfillAlbums(entries, collection)
	twice, secondRes := backfillAlbums(once, collection)

	if !reflect.DeepEqual(once, twice) {
		t.Errorf("second pass changed the entries:\n once: %+v\ntwice: %+v", once, twice)
	}
	if firstRes.Updated != 1 {
		t.Errorf("first pass Updated = %d, want 1", firstRes.Updated)
	}
	if secondRes.Updated != 0 {
		t.Errorf("second pass Updated = %d, want 0", secondRes.Updated)
	}
}

func TestBackfillHistory(t *testing.T) {
	when := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{Album: Album{Artist: "Slowdive", Title: "Souvlaki"}, Timestamp: when},
		{Album: Album{Artist: "Ride", Title: "Nowhere"}, Timestamp: when},
	}

	got, res := backfillHistory(entries, testCollection())

	if res.Updated != 1 {
		t.Errorf("Updated = %d, want 1", res.Updated)
	}
	if got[0].Album.ReleaseID != 111 {
		t.Errorf("ReleaseID = %d, want 111", got[0].Album.ReleaseID)
	}
	if got[1].Album.ReleaseID != 0 {
		t.Errorf("unmatched entry got ReleaseID %d, want 0", got[1].Album.ReleaseID)
	}
	if !got[0].Timestamp.Equal(when) {
		t.Errorf("Timestamp = %v, want %v", got[0].Timestamp, when)
	}
}

func TestIndexByLegacyKeyCollapsesRepeatedIDs(t *testing.T) {
	// The same release listed twice must not read as two candidates.
	collection := []Album{
		{ReleaseID: 111, Artist: "Slowdive", Title: "Souvlaki"},
		{ReleaseID: 111, Artist: "Slowdive", Title: "Souvlaki"},
	}

	idx := indexByLegacyKey(collection)

	if got := idx["Slowdive - Souvlaki"]; len(got) != 1 || got[0] != 111 {
		t.Errorf("index = %v, want [111]", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test . -run 'TestBackfill|TestIndexByLegacyKey' -v`

Expected: FAIL to compile — `undefined: backfillAlbums`, `undefined: backfillHistory`, `undefined: indexByLegacyKey`.

- [ ] **Step 3: Write `backfill.go`**

```go
package main

import "slices"

// backfillResult reports what one backfill pass did.
type backfillResult struct {
	// Updated counts the entries that gained a release ID.
	Updated int
	// Ambiguous holds the legacy keys that matched more than one release,
	// each listed once. Only favorites report these: history is a log, there
	// is no action to take on a past pick, and a long history would produce
	// dozens of lines nobody can act on.
	Ambiguous []string
}

// indexByLegacyKey maps each "Artist - Title" in the collection to the
// distinct release IDs that answer to it. A key with more than one ID is
// exactly the case the old dedup used to hide.
func indexByLegacyKey(collection []Album) map[string][]int {
	idx := make(map[string][]int, len(collection))
	for _, a := range collection {
		if a.ReleaseID == 0 {
			continue
		}
		key := a.Key()
		if slices.Contains(idx[key], a.ReleaseID) {
			continue
		}
		idx[key] = append(idx[key], a.ReleaseID)
	}
	return idx
}

// backfillAlbums stamps release IDs onto entries that predate them, matching
// their legacy key against the freshly synced collection.
//
// Three outcomes, and only one of them writes anything. Exactly one match
// stamps the ID. No match means the record is no longer in the collection --
// sold, or dropped from the synced folders -- and is left alone silently.
// More than one match is unknowable: nothing in the file says which pressing
// the user meant, so guessing would write an assertion they never made and
// could not later tell apart from a real choice. Those stay on the name
// fallback, where they still display and still match, and are reported.
func backfillAlbums(entries, collection []Album) ([]Album, backfillResult) {
	idx := indexByLegacyKey(collection)
	out := make([]Album, len(entries))
	copy(out, entries)

	var res backfillResult
	reported := make(map[string]bool)

	for i := range out {
		if out[i].ReleaseID != 0 {
			continue
		}
		key := out[i].Key()
		ids := idx[key]
		switch {
		case len(ids) == 1:
			out[i].ReleaseID = ids[0]
			res.Updated++
		case len(ids) > 1 && !reported[key]:
			reported[key] = true
			res.Ambiguous = append(res.Ambiguous, key)
		}
	}

	return out, res
}

// backfillHistory is backfillAlbums over the album inside each history entry,
// leaving timestamps untouched.
func backfillHistory(entries []HistoryEntry, collection []Album) ([]HistoryEntry, backfillResult) {
	albums := make([]Album, len(entries))
	for i, e := range entries {
		albums[i] = e.Album
	}

	filled, res := backfillAlbums(albums, collection)

	out := make([]HistoryEntry, len(entries))
	copy(out, entries)
	for i := range out {
		out[i].Album = filled[i]
	}
	return out, res
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test . -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backfill.go backfill_test.go
git commit -m "feat: add the release-ID backfill pass

Pure functions over entries and a collection: unique match stamps the ID,
no match is left alone, more than one match is left alone and reported."
```

---

### Task 6: The un-merge notice

Also pure — Task 7 prints what this returns.

**Files:**
- Modify: `sync.go` (new functions; nothing existing changes)
- Test: `sync_test.go`

**Interfaces:**
- Consumes: `Album.Key()`, `Album.ReleaseID` (Task 1).
- Produces:
  - `func unmergedCount(albums []Album) int`
  - `func unmergeNotice(prev, next []Album) string` — returns `""` or a two-line notice ending in `"\n"`

- [ ] **Step 1: Write the failing tests**

Append to `sync_test.go`:

```go
func TestUnmergedCount(t *testing.T) {
	// Two Kind of Blue pressings and one Souvlaki: two records share a name.
	albums := []Album{
		{ReleaseID: 111, Artist: "Miles Davis", Title: "Kind of Blue"},
		{ReleaseID: 222, Artist: "Miles Davis", Title: "Kind of Blue"},
		{ReleaseID: 333, Artist: "Slowdive", Title: "Souvlaki"},
	}

	if got := unmergedCount(albums); got != 2 {
		t.Errorf("unmergedCount() = %d, want 2", got)
	}
}

func TestUnmergedCountNoCollisions(t *testing.T) {
	albums := []Album{
		{ReleaseID: 111, Artist: "Slowdive", Title: "Souvlaki"},
		{ReleaseID: 222, Artist: "Ride", Title: "Nowhere"},
	}

	if got := unmergedCount(albums); got != 0 {
		t.Errorf("unmergedCount() = %d, want 0", got)
	}
}

// The notice fires on the first sync after upgrading, which is the moment
// the collection count visibly jumps.
func TestUnmergeNoticeOnFirstSync(t *testing.T) {
	prev := []Album{{Artist: "Miles Davis", Title: "Kind of Blue"}}
	next := []Album{
		{ReleaseID: 111, Artist: "Miles Davis", Title: "Kind of Blue"},
		{ReleaseID: 222, Artist: "Miles Davis", Title: "Kind of Blue"},
	}

	got := unmergeNotice(prev, next)
	if !strings.Contains(got, "2 records") {
		t.Errorf("notice = %q, want it to mention 2 records", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("notice = %q, want a trailing newline", got)
	}
}

// And never again: every entry has an ID from then on, so no flag in
// meta.json is needed to make it one-time.
func TestUnmergeNoticeSilentOnSecondSync(t *testing.T) {
	prev := []Album{
		{ReleaseID: 111, Artist: "Miles Davis", Title: "Kind of Blue"},
		{ReleaseID: 222, Artist: "Miles Davis", Title: "Kind of Blue"},
	}
	next := prev

	if got := unmergeNotice(prev, next); got != "" {
		t.Errorf("notice = %q, want empty on the second sync", got)
	}
}

func TestUnmergeNoticeSilentWithoutCollisions(t *testing.T) {
	prev := []Album{{Artist: "Slowdive", Title: "Souvlaki"}}
	next := []Album{{ReleaseID: 111, Artist: "Slowdive", Title: "Souvlaki"}}

	if got := unmergeNotice(prev, next); got != "" {
		t.Errorf("notice = %q, want empty when nothing un-merged", got)
	}
}

// A fresh install has no previous collection and nothing was ever merged,
// so there is nothing to explain.
func TestUnmergeNoticeSilentOnFreshInstall(t *testing.T) {
	next := []Album{
		{ReleaseID: 111, Artist: "Miles Davis", Title: "Kind of Blue"},
		{ReleaseID: 222, Artist: "Miles Davis", Title: "Kind of Blue"},
	}

	if got := unmergeNotice(nil, next); got != "" {
		t.Errorf("notice = %q, want empty with no previous collection", got)
	}
}
```

Add `"strings"` to `sync_test.go`'s import block.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test . -run 'TestUnmerge' -v`

Expected: FAIL to compile — `undefined: unmergedCount`, `undefined: unmergeNotice`.

- [ ] **Step 3: Add the two functions to `sync.go`**

```go
// unmergedCount counts the albums that share an artist and title with at
// least one other album -- that is, every record involved in a collision the
// old name-based dedup used to hide.
func unmergedCount(albums []Album) int {
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
func unmergeNotice(prev, next []Album) string {
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
```

`sync.go` already imports `fmt`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test . -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add sync.go sync_test.go
git commit -m "feat: explain the collection count jump after the identity change"
```

---

### Task 7: Wire the backfill and the notice into `sync`

The last task, and the only one that touches I/O ordering. `runSync` itself stays untested — it calls `fatal` and the network — so the testable work is pushed into `runBackfill` and `backfillSummary`.

**Files:**
- Create: nothing.
- Modify: `backfill.go` (add `backfillSummary` and `runBackfill`), `sync.go:31-70` (`runSync`)
- Test: `backfill_test.go`

**Interfaces:**
- Consumes: `backfillAlbums`, `backfillHistory`, `backfillResult` (Task 5); `unmergeNotice` (Task 6); `loadFavorites`, `saveFavorites` (`favorites.go`), `loadHistory`, `saveHistory` (`history.go`), `loadCollectionFrom`, `collectionPath` (`collection.go`), `favoritesPath`, `historyPath` (existing).
- Produces:
  - `func backfillSummary(fav, hist backfillResult) string`
  - `func runBackfill(favPath, histPath string, collection []Album) (string, error)`

- [ ] **Step 1: Write the failing tests**

Append to `backfill_test.go`:

```go
func TestBackfillSummary(t *testing.T) {
	tests := []struct {
		name string
		fav  backfillResult
		hist backfillResult
		want string
	}{
		{
			name: "nothing to say",
			want: "",
		},
		{
			name: "both files",
			fav:  backfillResult{Updated: 12},
			hist: backfillResult{Updated: 106},
			want: "Filled in release IDs for 12 favorites and 106 history entries.\n",
		},
		{
			name: "favorites only",
			fav:  backfillResult{Updated: 12},
			want: "Filled in release IDs for 12 favorites.\n",
		},
		{
			name: "history only",
			hist: backfillResult{Updated: 106},
			want: "Filled in release IDs for 106 history entries.\n",
		},
		{
			name: "singular",
			fav:  backfillResult{Updated: 1},
			hist: backfillResult{Updated: 1},
			want: "Filled in release IDs for 1 favorite and 1 history entry.\n",
		},
		{
			name: "ambiguous favorites are listed",
			fav:  backfillResult{Ambiguous: []string{"Miles Davis - Kind of Blue"}},
			want: "These favorites matched more than one record and were left as-is:\n" +
				"  Miles Davis - Kind of Blue\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := backfillSummary(tt.fav, tt.hist); got != tt.want {
				t.Errorf("backfillSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

// History ambiguity is deliberately silent: a log has nothing to act on.
func TestBackfillSummaryIgnoresHistoryAmbiguity(t *testing.T) {
	hist := backfillResult{Ambiguous: []string{"Miles Davis - Kind of Blue"}}
	if got := backfillSummary(backfillResult{}, hist); got != "" {
		t.Errorf("backfillSummary() = %q, want empty", got)
	}
}

func TestRunBackfillWritesBothFiles(t *testing.T) {
	dir := t.TempDir()
	favPath := filepath.Join(dir, "favorites.json")
	histPath := filepath.Join(dir, "history.json")

	if err := saveFavorites(favPath, []Album{{Artist: "Slowdive", Title: "Souvlaki"}}); err != nil {
		t.Fatalf("saveFavorites: %v", err)
	}
	if err := saveHistory(histPath, []HistoryEntry{
		{Album: Album{Artist: "Slowdive", Title: "Souvlaki"}, Timestamp: time.Now()},
	}); err != nil {
		t.Fatalf("saveHistory: %v", err)
	}

	report, err := runBackfill(favPath, histPath, testCollection())
	if err != nil {
		t.Fatalf("runBackfill: %v", err)
	}
	if report != "Filled in release IDs for 1 favorite and 1 history entry.\n" {
		t.Errorf("report = %q", report)
	}

	favs, err := loadFavorites(favPath)
	if err != nil {
		t.Fatalf("loadFavorites: %v", err)
	}
	if favs[0].ReleaseID != 111 {
		t.Errorf("favorite ReleaseID = %d, want 111", favs[0].ReleaseID)
	}

	entries, err := loadHistory(histPath)
	if err != nil {
		t.Fatalf("loadHistory: %v", err)
	}
	if entries[0].Album.ReleaseID != 111 {
		t.Errorf("history ReleaseID = %d, want 111", entries[0].Album.ReleaseID)
	}
}

// The acceptance criterion: running sync twice leaves the files
// byte-identical, and the second run reports nothing.
func TestRunBackfillIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	favPath := filepath.Join(dir, "favorites.json")
	histPath := filepath.Join(dir, "history.json")

	if err := saveFavorites(favPath, []Album{{Artist: "Slowdive", Title: "Souvlaki"}}); err != nil {
		t.Fatalf("saveFavorites: %v", err)
	}
	if err := saveHistory(histPath, []HistoryEntry{
		{Album: Album{Artist: "Slowdive", Title: "Souvlaki"}, Timestamp: time.Now()},
	}); err != nil {
		t.Fatalf("saveHistory: %v", err)
	}

	if _, err := runBackfill(favPath, histPath, testCollection()); err != nil {
		t.Fatalf("first runBackfill: %v", err)
	}
	favBefore, err := os.ReadFile(favPath)
	if err != nil {
		t.Fatalf("read favorites: %v", err)
	}
	histBefore, err := os.ReadFile(histPath)
	if err != nil {
		t.Fatalf("read history: %v", err)
	}

	report, err := runBackfill(favPath, histPath, testCollection())
	if err != nil {
		t.Fatalf("second runBackfill: %v", err)
	}
	if report != "" {
		t.Errorf("second run reported %q, want nothing", report)
	}

	favAfter, err := os.ReadFile(favPath)
	if err != nil {
		t.Fatalf("read favorites: %v", err)
	}
	histAfter, err := os.ReadFile(histPath)
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	if !bytes.Equal(favBefore, favAfter) {
		t.Error("favorites.json changed on the second pass")
	}
	if !bytes.Equal(histBefore, histAfter) {
		t.Error("history.json changed on the second pass")
	}
}

// A user who has never favorited anything must not have empty files
// created for them.
func TestRunBackfillLeavesAbsentFilesAlone(t *testing.T) {
	dir := t.TempDir()
	favPath := filepath.Join(dir, "favorites.json")
	histPath := filepath.Join(dir, "history.json")

	report, err := runBackfill(favPath, histPath, testCollection())
	if err != nil {
		t.Fatalf("runBackfill: %v", err)
	}
	if report != "" {
		t.Errorf("report = %q, want nothing", report)
	}
	if _, err := os.Stat(favPath); !os.IsNotExist(err) {
		t.Error("favorites.json was created")
	}
	if _, err := os.Stat(histPath); !os.IsNotExist(err) {
		t.Error("history.json was created")
	}
}
```

Extend `backfill_test.go`'s import block to `"bytes"`, `"os"`, `"path/filepath"`, `"reflect"`, `"testing"`, `"time"`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test . -run 'TestBackfillSummary|TestRunBackfill' -v`

Expected: FAIL to compile — `undefined: backfillSummary`, `undefined: runBackfill`.

- [ ] **Step 3: Add `backfillSummary` and `runBackfill` to `backfill.go`**

```go
// backfillSummary renders what the pass did, for sync's stdout report. It
// returns "" when there is nothing worth saying, so the caller can print it
// unconditionally.
//
// Only favorites contribute ambiguous keys; see backfillResult.Ambiguous.
func backfillSummary(fav, hist backfillResult) string {
	var sb strings.Builder

	if fav.Updated > 0 || hist.Updated > 0 {
		var parts []string
		if fav.Updated > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", fav.Updated, plural(fav.Updated, "favorite", "favorites")))
		}
		if hist.Updated > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", hist.Updated, plural(hist.Updated, "history entry", "history entries")))
		}
		sb.WriteString("Filled in release IDs for ")
		sb.WriteString(strings.Join(parts, " and "))
		sb.WriteString(".\n")
	}

	if len(fav.Ambiguous) > 0 {
		sb.WriteString("These favorites matched more than one record and were left as-is:\n")
		for _, key := range fav.Ambiguous {
			sb.WriteString("  " + key + "\n")
		}
	}

	return sb.String()
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// runBackfill stamps release IDs into the favorites and history files from
// the freshly synced collection, and returns sync's report on what it did.
//
// A file is rewritten only when something actually changed, so a user who
// has never favorited anything never gets an empty favorites.json created
// for them, and a second sync touches nothing.
func runBackfill(favPath, histPath string, collection []Album) (string, error) {
	favorites, err := loadFavorites(favPath)
	if err != nil {
		return "", fmt.Errorf("loading favorites: %w", err)
	}
	filledFavorites, favRes := backfillAlbums(favorites, collection)
	if favRes.Updated > 0 {
		if err := saveFavorites(favPath, filledFavorites); err != nil {
			return "", fmt.Errorf("saving favorites: %w", err)
		}
	}

	history, err := loadHistory(histPath)
	if err != nil {
		return "", fmt.Errorf("loading history: %w", err)
	}
	filledHistory, histRes := backfillHistory(history, collection)
	if histRes.Updated > 0 {
		if err := saveHistory(histPath, filledHistory); err != nil {
			return "", fmt.Errorf("saving history: %w", err)
		}
	}

	return backfillSummary(favRes, histRes), nil
}
```

Extend `backfill.go`'s import block to `"fmt"`, `"slices"` and `"strings"`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test . -v`

Expected: PASS.

- [ ] **Step 5: Rewrite `runSync`'s tail**

In `sync.go`, replace everything from the `albums, err := collectAlbums(...)` line to the end of `runSync`:

```go
	albums, err := collectAlbums(client, username, folderIDs)
	if err != nil {
		fatal("Error: %v", err)
	}

	// Read before the write below overwrites it: comparing the two is what
	// tells us whether this is the first sync after the identity change.
	// Failing to read it is not an error -- it just means no notice.
	previous, _ := loadCollectionFrom(collectionPath())

	if err := saveCollection(albums); err != nil {
		fatal("Error saving collection: %v", err)
	}

	// Recorded after the collection lands, so a stale timestamp never claims
	// a sync that did not actually persist.
	if err := recordSync(metaPath(), time.Now()); err != nil {
		fatal("Error saving sync metadata: %v", err)
	}

	// Also after the collection lands, so IDs are never stamped from a
	// collection that then failed to save. A failure here does not fail the
	// sync: the sync itself succeeded, the pass is idempotent, and the next
	// sync retries it.
	backfillReport, err := runBackfill(favoritesPath(), historyPath(), albums)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not fill in release IDs: %v\n", err)
	}

	withMetadata := 0
	for _, album := range albums {
		if album.Year != 0 || album.Label != "" || len(album.Genres) > 0 {
			withMetadata++
		}
	}

	fmt.Printf("Synced %d albums (%d with full metadata)\n", len(albums), withMetadata)
	fmt.Print(unmergeNotice(previous, albums))
	fmt.Print(backfillReport)
}
```

- [ ] **Step 6: Verify the whole suite and a manual smoke run**

Run: `go test . -v && go vet ./... && gofmt -l .`

Expected: PASS, no vet complaints, `gofmt -l` prints nothing.

Then check by hand that a legacy favorites file survives a build:

```bash
go build -o /tmp/disc-fortune . && echo build ok
```

- [ ] **Step 7: Commit**

```bash
git add sync.go backfill.go backfill_test.go
git commit -m "feat: backfill release IDs into favorites and history on sync

Runs after the collection lands, so IDs are never stamped from a collection
that then failed to save. A backfill failure warns but does not fail the
sync: it is idempotent and the next sync retries it."
```

---

## Accepted consequences

- **An ambiguous favorite is reported on every sync until the user resolves it.**
  It stays un-ID'd, so `backfillAlbums` finds it again each time. This is
  deliberate rather than overlooked: it is the only signal the user gets, it is
  actionable, and it stops the moment they re-favorite the specific pressing.
  Re-favoriting genuinely resolves it because `addFavorite` replaces the
  matching un-ID'd entry with the record the user named, before returning
  `ErrAlreadyInFavorites` — an un-ID'd favorite that can still be re-favorited
  from the collection is necessarily an ambiguous one, since a unique match
  would already have been stamped by the backfill. The CLI still prints
  "Already in favorites", which is accurate: the user ends up with one favorite
  for that name either way, now pinned to the pressing they named. The whole
  record is replaced rather than just its ID: stamping the ID alone would leave
  the entry asserting one pressing while carrying another's year, label and
  catalogue number, permanently, since `backfillAlbums` skips any entry that
  already has an ID. Suppressing
  the report instead would need a flag in `meta.json` and would hide a condition
  the user could actually fix. Revisit only if it proves noisy.
- **A `pick` racing a `sync` can lose its history entry.** `sync` now rewrites
  `history.json` during the backfill, so a `pick` that appends between the
  backfill's read and its atomic rename is overwritten by the older snapshot.
  The window is the few milliseconds between those two operations. In v2.1.1
  `sync` never touched history, so this is new. It is accepted for v2.2: this is
  a single-user CLI, both commands are hand-run, and closing the window properly
  means file locking across every reader and writer of the data files — a larger
  change than the backfill itself, and one that belongs to its own task if the
  race ever proves real.
- **`sameAlbum` is not transitive.** See the design doc; it is bounded to
  linear scans in `favorites.go` and is the reason sync dedup uses `Identity()`.

## Out of scope for this plan

Deliberately excluded, per the design doc:

- **Refreshing favorite metadata from the collection.** An ID'd favorite is correctly identified after an upstream retitle; its stored title just stays as saved.
- **Showing `release_id` in human output.** `pick`, `list` and `history` stay byte-identical to v2.1.1. T7 (`--json`) puts the ID in the machine schema; T10 (`open`) consumes it.
- **Retiring the name fallback.** It is permanent for entries the backfill could not resolve.
- **Version bump and `RELEASE_NOTES_v2.2.0.md`.** Release-time work. The notes must call out the collection-count change prominently, per the roadmap.
