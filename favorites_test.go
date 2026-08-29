package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAddFavorite(t *testing.T) {
	tmpDir := t.TempDir()
	favPath := filepath.Join(tmpDir, "favorites.json")

	album := Album{Artist: "Miles Davis", Title: "Kind of Blue"}
	err := addFavorite(favPath, album)
	if err != nil {
		t.Fatalf("addFavorite failed: %v", err)
	}

	favs, err := loadFavorites(favPath)
	if err != nil {
		t.Fatalf("loadFavorites failed: %v", err)
	}
	if len(favs) != 1 {
		t.Fatalf("got %d favorites, want 1", len(favs))
	}
	if favs[0].Artist != "Miles Davis" {
		t.Errorf("Artist = %q, want Miles Davis", favs[0].Artist)
	}
}

func TestAddFavoriteDuplicate(t *testing.T) {
	tmpDir := t.TempDir()
	favPath := filepath.Join(tmpDir, "favorites.json")

	album := Album{Artist: "Miles Davis", Title: "Kind of Blue"}
	addFavorite(favPath, album)
	err := addFavorite(favPath, album)

	if !errors.Is(err, ErrAlreadyInFavorites) {
		t.Errorf("error = %v, want ErrAlreadyInFavorites", err)
	}
}

func TestRemoveFavorite(t *testing.T) {
	tmpDir := t.TempDir()
	favPath := filepath.Join(tmpDir, "favorites.json")

	album := Album{Artist: "Miles Davis", Title: "Kind of Blue"}
	addFavorite(favPath, album)

	err := removeFavorite(favPath, album)
	if err != nil {
		t.Fatalf("removeFavorite failed: %v", err)
	}

	favs, err := loadFavorites(favPath)
	if err != nil {
		t.Fatalf("loadFavorites failed: %v", err)
	}
	if len(favs) != 0 {
		t.Errorf("got %d favorites, want 0", len(favs))
	}
}

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
	favs, err := loadFavorites(favPath)
	if err != nil {
		t.Fatalf("loadFavorites: %v", err)
	}
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
	favs, err := loadFavorites(favPath)
	if err != nil {
		t.Fatalf("loadFavorites: %v", err)
	}
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

	if _, err := favoriteByQuery(collection, "kind of", Filter{}, favPath); err != nil {
		t.Fatalf("first favoriteByQuery: %v", err)
	}
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
	favs, err := loadFavorites(favPath)
	if err != nil {
		t.Fatalf("loadFavorites: %v", err)
	}
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

// TestRemoveFavoriteRemovesOnlyFirstMatch is the guard for the silent data
// loss a filter-all removal caused: sameAlbum is not transitive, so an
// un-ID'd target matches every stored pressing sharing its name. Removing
// all of them would delete records the user never named -- and the CLI would
// still print a single "Removed from favorites" line.
func TestRemoveFavoriteRemovesOnlyFirstMatch(t *testing.T) {
	favPath := filepath.Join(t.TempDir(), "favorites.json")

	stored := []Album{
		{ReleaseID: 1, Artist: "Miles Davis", Title: "Kind of Blue", Year: 1959},
		{ReleaseID: 2, Artist: "Miles Davis", Title: "Kind of Blue", Year: 1997},
	}
	if err := saveFavorites(favPath, stored); err != nil {
		t.Fatalf("saveFavorites: %v", err)
	}

	// The un-ID'd album `unfavorite` with no query hands over, taken from
	// the last history entry.
	target := Album{Artist: "Miles Davis", Title: "Kind of Blue"}
	if err := removeFavorite(favPath, target); err != nil {
		t.Fatalf("removeFavorite: %v", err)
	}

	favs, err := loadFavorites(favPath)
	if err != nil {
		t.Fatalf("loadFavorites: %v", err)
	}
	if len(favs) != 1 {
		t.Fatalf("got %d favorites, want 1 (only the first match removed)", len(favs))
	}
	if favs[0].ReleaseID != 2 {
		t.Errorf("surviving favorite ReleaseID = %d, want 2", favs[0].ReleaseID)
	}
}

// TestRemoveFavoriteNoMatchStillErrors pins the ErrNotInFavorites contract
// across the switch to a first-match scan.
func TestRemoveFavoriteNoMatchStillErrors(t *testing.T) {
	favPath := filepath.Join(t.TempDir(), "favorites.json")

	if err := saveFavorites(favPath, []Album{{ReleaseID: 1, Artist: "Slowdive", Title: "Souvlaki"}}); err != nil {
		t.Fatalf("saveFavorites: %v", err)
	}

	err := removeFavorite(favPath, Album{ReleaseID: 2, Artist: "Ride", Title: "Nowhere"})
	if !errors.Is(err, ErrNotInFavorites) {
		t.Errorf("error = %v, want ErrNotInFavorites", err)
	}
}

// TestAddFavoriteStampsIDOntoLegacyMatch is the documented remedy for an
// ambiguous favorite: naming the specific pressing must actually resolve it.
func TestAddFavoriteStampsIDOntoLegacyMatch(t *testing.T) {
	favPath := filepath.Join(t.TempDir(), "favorites.json")

	if err := saveFavorites(favPath, []Album{{Artist: "Miles Davis", Title: "Kind of Blue"}}); err != nil {
		t.Fatalf("saveFavorites: %v", err)
	}

	err := addFavorite(favPath, Album{ReleaseID: 111, Artist: "Miles Davis", Title: "Kind of Blue"})
	if !errors.Is(err, ErrAlreadyInFavorites) {
		t.Fatalf("error = %v, want ErrAlreadyInFavorites", err)
	}

	favs, err := loadFavorites(favPath)
	if err != nil {
		t.Fatalf("loadFavorites: %v", err)
	}
	if len(favs) != 1 {
		t.Fatalf("got %d favorites, want 1", len(favs))
	}
	if favs[0].ReleaseID != 111 {
		t.Errorf("stored ReleaseID = %d, want 111 (stamped and persisted)", favs[0].ReleaseID)
	}
}

// TestAddFavoriteDoesNotOverwriteAnExistingID: stamping targets the entry
// that has no ID, never one the user has already disambiguated.
func TestAddFavoriteDoesNotOverwriteAnExistingID(t *testing.T) {
	favPath := filepath.Join(t.TempDir(), "favorites.json")

	stored := []Album{
		{ReleaseID: 111, Artist: "Miles Davis", Title: "Kind of Blue"},
		{Artist: "Miles Davis", Title: "Kind of Blue"},
	}
	if err := saveFavorites(favPath, stored); err != nil {
		t.Fatalf("saveFavorites: %v", err)
	}

	err := addFavorite(favPath, Album{ReleaseID: 222, Artist: "Miles Davis", Title: "Kind of Blue"})
	if !errors.Is(err, ErrAlreadyInFavorites) {
		t.Fatalf("error = %v, want ErrAlreadyInFavorites", err)
	}

	favs, err := loadFavorites(favPath)
	if err != nil {
		t.Fatalf("loadFavorites: %v", err)
	}
	if len(favs) != 2 {
		t.Fatalf("got %d favorites, want 2", len(favs))
	}
	if favs[0].ReleaseID != 111 {
		t.Errorf("first ReleaseID = %d, want 111 (untouched)", favs[0].ReleaseID)
	}
	if favs[1].ReleaseID != 222 {
		t.Errorf("second ReleaseID = %d, want 222 (stamped)", favs[1].ReleaseID)
	}
}

// TestAddFavoriteWithoutIDStampsNothing: an incoming album that carries no
// ID has nothing to contribute, so the file must not be rewritten at all.
func TestAddFavoriteWithoutIDStampsNothing(t *testing.T) {
	favPath := filepath.Join(t.TempDir(), "favorites.json")

	if err := saveFavorites(favPath, []Album{{Artist: "Miles Davis", Title: "Kind of Blue"}}); err != nil {
		t.Fatalf("saveFavorites: %v", err)
	}
	before, err := os.ReadFile(favPath)
	if err != nil {
		t.Fatalf("read favorites: %v", err)
	}

	err = addFavorite(favPath, Album{Artist: "Miles Davis", Title: "Kind of Blue"})
	if !errors.Is(err, ErrAlreadyInFavorites) {
		t.Fatalf("error = %v, want ErrAlreadyInFavorites", err)
	}

	after, err := os.ReadFile(favPath)
	if err != nil {
		t.Fatalf("read favorites: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("favorites.json changed:\nbefore: %s\nafter:  %s", before, after)
	}
}

// TestAddFavoriteAlreadyIDdStampsNothing: both sides carry the same ID, so
// there is nothing to fill in and no reason to touch the file.
func TestAddFavoriteAlreadyIDdStampsNothing(t *testing.T) {
	favPath := filepath.Join(t.TempDir(), "favorites.json")

	if err := saveFavorites(favPath, []Album{{ReleaseID: 111, Artist: "Miles Davis", Title: "Kind of Blue"}}); err != nil {
		t.Fatalf("saveFavorites: %v", err)
	}
	before, err := os.ReadFile(favPath)
	if err != nil {
		t.Fatalf("read favorites: %v", err)
	}

	err = addFavorite(favPath, Album{ReleaseID: 111, Artist: "Miles Davis", Title: "Kind of Blue (1959)"})
	if !errors.Is(err, ErrAlreadyInFavorites) {
		t.Fatalf("error = %v, want ErrAlreadyInFavorites", err)
	}

	after, err := os.ReadFile(favPath)
	if err != nil {
		t.Fatalf("read favorites: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("favorites.json changed:\nbefore: %s\nafter:  %s", before, after)
	}
}
