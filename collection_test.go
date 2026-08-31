package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
