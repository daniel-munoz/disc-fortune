package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestResolveFolderNames(t *testing.T) {
	folders := []folder{
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
	folders := []folder{
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
	folders := []folder{
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
	makeHandler := func(albums []collectionRelease) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			resp := collectionPage{Releases: albums}
			resp.Pagination.Pages = 1
			json.NewEncoder(w).Encode(resp)
		}
	}

	mux.HandleFunc("/users/testuser/collection/folders/1/releases", makeHandler([]collectionRelease{
		{BasicInformation: releaseInfo{Title: "Souvlaki", Artists: []releaseArtist{{Name: "Slowdive"}}}},
		{BasicInformation: releaseInfo{Title: "Nowhere", Artists: []releaseArtist{{Name: "Ride"}}}},
	}))
	mux.HandleFunc("/users/testuser/collection/folders/2/releases", makeHandler([]collectionRelease{
		{BasicInformation: releaseInfo{Title: "Souvlaki", Artists: []releaseArtist{{Name: "Slowdive"}}}},
		{BasicInformation: releaseInfo{Title: "Pygmalion", Artists: []releaseArtist{{Name: "Slowdive"}}}},
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
