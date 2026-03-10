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

func TestRunListOutput(t *testing.T) {
	albums := []Album{
		{Artist: "Slowdive", Title: "Souvlaki", Year: 1993, Label: "Creation Records", Genres: []string{"Shoegaze"}},
		{Artist: "Ride", Title: "Nowhere", Year: 1990, Label: "Creation Records", Genres: []string{"Shoegaze"}},
	}
	out := formatList(albums, false)
	if !strings.Contains(out, "Slowdive") {
		t.Errorf("output missing Slowdive: %q", out)
	}
	if !strings.Contains(out, "Ride") {
		t.Errorf("output missing Ride: %q", out)
	}
	if !strings.Contains(out, "2 albums") {
		t.Errorf("output missing count summary: %q", out)
	}
}

func TestRunListEmpty(t *testing.T) {
	out := formatList([]Album{}, false)
	if !strings.Contains(out, "No albums") {
		t.Errorf("expected empty message, got: %q", out)
	}
}

func TestRunListSeparator(t *testing.T) {
	albums := []Album{
		{Artist: "A", Title: "X"},
		{Artist: "B", Title: "Y"},
	}
	out := formatList(albums, false)
	// There should be a blank line between the two entries
	if !strings.Contains(out, "\n\n") {
		t.Errorf("expected blank line separator between entries: %q", out)
	}
}
