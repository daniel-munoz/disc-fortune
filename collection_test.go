package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAlbumKey(t *testing.T) {
	album := Album{
		Artist:  "Miles Davis",
		Title:   "Kind of Blue",
		Year:    1959,
		Label:   "Columbia",
		CatNo:   "CL 1355",
		Genres:  []string{"Jazz"},
		Formats: []string{"Vinyl", "12\""},
	}
	want := "Miles Davis - Kind of Blue"
	if got := album.Key(); got != want {
		t.Errorf("Key() = %q, want %q", got, want)
	}
}

func TestSaveAndLoadCollection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "collection.json")

	albums := []Album{
		{Artist: "Slowdive", Title: "Souvlaki"},
		{Artist: "Cocteau Twins", Title: "Heaven or Las Vegas"},
	}

	data, err := json.MarshalIndent(albums, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, collectionFilePerms); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := loadCollectionFrom(path)
	if err != nil {
		t.Fatalf("loadCollectionFrom: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d albums, want 2", len(got))
	}
	if got[0].Artist != "Slowdive" || got[1].Title != "Heaven or Las Vegas" {
		t.Errorf("unexpected albums: %+v", got)
	}
}

func TestLoadCollectionMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.json")
	_, err := loadCollectionFrom(path)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected not-exist error, got: %v", err)
	}
}

func TestLoadCollectionInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("{not json"), collectionFilePerms); err != nil {
		t.Fatal(err)
	}
	_, err := loadCollectionFrom(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestSaveCollectionCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	path := filepath.Join(dir, "collection.json")

	albums := []Album{{Artist: "Deafheaven", Title: "Sunbather"}}
	if err := saveCollectionTo(path, albums); err != nil {
		t.Fatalf("saveCollectionTo: %v", err)
	}

	got, err := loadCollectionFrom(path)
	if err != nil {
		t.Fatalf("loadCollectionFrom: %v", err)
	}
	if len(got) != 1 || got[0].Artist != "Deafheaven" {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestRandomAlbum(t *testing.T) {
	albums := []Album{
		{Artist: "A", Title: "1"},
		{Artist: "B", Title: "2"},
		{Artist: "C", Title: "3"},
	}

	// Run enough times to confirm it doesn't panic and returns valid albums.
	seen := make(map[string]bool)
	for range 100 {
		a := randomAlbum(albums)
		seen[a.Key()] = true
	}
	if len(seen) < 2 {
		t.Errorf("expected multiple different albums over 100 picks, got %d unique", len(seen))
	}
}
