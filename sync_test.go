package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/daniel-munoz/disc-fortune/v2/internal/disc"
	"github.com/daniel-munoz/disc-fortune/v2/internal/discogs"
)

// testArtist, testRelease and testPage mirror the shape of a Discogs
// collection page response. They are sync_test.go's own copy rather than a
// reference to internal/discogs's unexported wire types -- the two packages
// cannot share an unexported type any more than they can share an unexported
// helper, so each keeps its own, the way cli_test.go's include/years already
// do for internal/disc.
type testArtist struct {
	Name string `json:"name"`
}

type testRelease struct {
	BasicInformation struct {
		ID      int          `json:"id"`
		Title   string       `json:"title"`
		Artists []testArtist `json:"artists"`
		Year    int          `json:"year"`
	} `json:"basic_information"`
}

type testPage struct {
	Pagination struct {
		Pages int `json:"pages"`
	} `json:"pagination"`
	Releases []testRelease `json:"releases"`
}

// newTestRelease builds one collection-page release fixture.
func newTestRelease(id int, title, artist string, year int) testRelease {
	var r testRelease
	r.BasicInformation.ID = id
	r.BasicInformation.Title = title
	r.BasicInformation.Artists = []testArtist{{Name: artist}}
	r.BasicInformation.Year = year
	return r
}

// newDiscogsTestClient returns a discogs.Client pointed at srv, restoring the
// package's base URL once the test ends. Shared with progress_test.go, which
// needs the same wiring for its own httptest servers.
func newDiscogsTestClient(t *testing.T, srv *httptest.Server) *discogs.Client {
	t.Helper()
	discogs.SetBaseURL(srv.URL)
	t.Cleanup(func() { discogs.SetBaseURL("https://api.discogs.com") })

	t.Setenv("DISCOGS_TOKEN", "test-token")
	client, err := discogs.New("disc-fortune/test")
	if err != nil {
		t.Fatalf("discogs.New: %v", err)
	}
	return client
}

// newSyncTestClient returns a discogs.Client pointed at a test server built
// from handler, closing the server once the test ends.
func newSyncTestClient(t *testing.T, handler http.Handler) (*discogs.Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return newDiscogsTestClient(t, srv), srv
}

func TestResolveFolderNames(t *testing.T) {
	folders := []discogs.Folder{
		{ID: 0, Name: "All"},
		{ID: 1, Name: "Uncategorized"},
		{ID: 2, Name: "Vinyl 12\""},
	}

	ids, err := resolveFolderNames([]string{"Vinyl 12\""}, folders)
	if err != nil {
		t.Fatalf("resolveFolderNames: %v", err)
	}
	if len(ids) != 1 || ids[0] != 2 {
		t.Errorf("ids = %v, want [2]", ids)
	}
}

func TestResolveFolderNamesMultiple(t *testing.T) {
	folders := []discogs.Folder{
		{ID: 0, Name: "All"},
		{ID: 1, Name: "Uncategorized"},
		{ID: 2, Name: "Vinyl 12\""},
	}

	ids, err := resolveFolderNames([]string{"Uncategorized", "Vinyl 12\""}, folders)
	if err != nil {
		t.Fatalf("resolveFolderNames: %v", err)
	}
	if len(ids) != 2 || ids[0] != 1 || ids[1] != 2 {
		t.Errorf("ids = %v, want [1, 2]", ids)
	}
}

func TestResolveFolderNamesNotFound(t *testing.T) {
	folders := []discogs.Folder{
		{ID: 0, Name: "All"},
	}

	_, err := resolveFolderNames([]string{"Nonexistent"}, folders)
	if err == nil {
		t.Fatal("expected error for unknown folder name")
	}
}

func TestCollectAlbumsDeduplicates(t *testing.T) {
	mux := http.NewServeMux()

	// Both folders return the same album, plus one unique each.
	makeHandler := func(releases []testRelease) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			resp := testPage{Releases: releases}
			resp.Pagination.Pages = 1
			json.NewEncoder(w).Encode(resp)
		}
	}

	souvlaki := newTestRelease(0, "Souvlaki", "Slowdive", 0)
	nowhere := newTestRelease(0, "Nowhere", "Ride", 0)
	pygmalion := newTestRelease(0, "Pygmalion", "Slowdive", 0)

	mux.HandleFunc("/users/testuser/collection/folders/1/releases", makeHandler([]testRelease{souvlaki, nowhere}))
	mux.HandleFunc("/users/testuser/collection/folders/2/releases", makeHandler([]testRelease{souvlaki, pygmalion}))

	client, _ := newSyncTestClient(t, mux)

	albums, err := collectAlbums(client, "testuser", []int{1, 2})
	if err != nil {
		t.Fatalf("collectAlbums: %v", err)
	}
	// Souvlaki appears in both folders but should be deduplicated.
	if len(albums) != 3 {
		t.Errorf("got %d albums, want 3 (deduplicated)", len(albums))
		for _, a := range albums {
			t.Logf("  %s", a.Key())
		}
	}
}

// TestCollectAlbumsKeepsDistinctPressings is the bug T4 exists to fix: two
// different releases sharing an artist and title used to collapse into one.
func TestCollectAlbumsKeepsDistinctPressings(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/users/testuser/collection/folders/0/releases", func(w http.ResponseWriter, r *http.Request) {
		resp := testPage{Releases: []testRelease{
			newTestRelease(111, "Kind of Blue", "Miles Davis", 1959),
			newTestRelease(222, "Kind of Blue", "Miles Davis", 1997),
		}}
		resp.Pagination.Pages = 1
		json.NewEncoder(w).Encode(resp)
	})

	client, _ := newSyncTestClient(t, mux)

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
	makeHandler := func(releases []testRelease) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			resp := testPage{Releases: releases}
			resp.Pagination.Pages = 1
			json.NewEncoder(w).Encode(resp)
		}
	}

	// The same release ID, but the two folders report slightly different
	// titles -- an ID match must win over a name mismatch.
	mux.HandleFunc("/users/testuser/collection/folders/1/releases", makeHandler([]testRelease{
		newTestRelease(333, "Souvlaki", "Slowdive", 0),
	}))
	mux.HandleFunc("/users/testuser/collection/folders/2/releases", makeHandler([]testRelease{
		newTestRelease(333, "Souvlaki (Reissue)", "Slowdive", 0),
	}))

	client, _ := newSyncTestClient(t, mux)

	albums, err := collectAlbums(client, "testuser", []int{1, 2})
	if err != nil {
		t.Fatalf("collectAlbums: %v", err)
	}
	if len(albums) != 1 {
		t.Fatalf("got %d albums, want 1 (same release in two folders)", len(albums))
	}
}

func TestUnmergedCount(t *testing.T) {
	// Two Kind of Blue pressings and one Souvlaki: two records share a name.
	albums := []disc.Album{
		{ReleaseID: 111, Artist: "Miles Davis", Title: "Kind of Blue"},
		{ReleaseID: 222, Artist: "Miles Davis", Title: "Kind of Blue"},
		{ReleaseID: 333, Artist: "Slowdive", Title: "Souvlaki"},
	}

	if got := unmergedCount(albums); got != 2 {
		t.Errorf("unmergedCount() = %d, want 2", got)
	}
}

func TestUnmergedCountNoCollisions(t *testing.T) {
	albums := []disc.Album{
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
	prev := []disc.Album{{Artist: "Miles Davis", Title: "Kind of Blue"}}
	next := []disc.Album{
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
	prev := []disc.Album{
		{ReleaseID: 111, Artist: "Miles Davis", Title: "Kind of Blue"},
		{ReleaseID: 222, Artist: "Miles Davis", Title: "Kind of Blue"},
	}
	next := prev

	if got := unmergeNotice(prev, next); got != "" {
		t.Errorf("notice = %q, want empty on the second sync", got)
	}
}

func TestUnmergeNoticeSilentWithoutCollisions(t *testing.T) {
	prev := []disc.Album{{Artist: "Slowdive", Title: "Souvlaki"}}
	next := []disc.Album{{ReleaseID: 111, Artist: "Slowdive", Title: "Souvlaki"}}

	if got := unmergeNotice(prev, next); got != "" {
		t.Errorf("notice = %q, want empty when nothing un-merged", got)
	}
}

// A fresh install has no previous collection and nothing was ever merged,
// so there is nothing to explain.
func TestUnmergeNoticeSilentOnFreshInstall(t *testing.T) {
	next := []disc.Album{
		{ReleaseID: 111, Artist: "Miles Davis", Title: "Kind of Blue"},
		{ReleaseID: 222, Artist: "Miles Davis", Title: "Kind of Blue"},
	}

	if got := unmergeNotice(nil, next); got != "" {
		t.Errorf("notice = %q, want empty with no previous collection", got)
	}
}
