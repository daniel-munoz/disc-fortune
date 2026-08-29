package main

import (
	"encoding/json"
	"net/http"
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
