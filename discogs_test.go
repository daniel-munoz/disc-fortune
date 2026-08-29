package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestClient returns a discogsClient pointed at a test server.
func newTestClient(handler http.Handler) (*discogsClient, *httptest.Server) {
	srv := httptest.NewServer(handler)
	return &discogsClient{
		token:      "test-token",
		httpClient: srv.Client(),
	}, srv
}

func TestGetUsername(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/identity", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Discogs token=test-token" {
			t.Errorf("auth header = %q", got)
		}
		json.NewEncoder(w).Encode(identityResponse{Username: "testuser"})
	})

	client, srv := newTestClient(mux)
	defer srv.Close()

	// Override base URL to point at test server.
	origBase := discogsBaseURL
	setBaseURL(srv.URL)
	defer setBaseURL(origBase)

	username, err := client.getUsername()
	if err != nil {
		t.Fatalf("getUsername: %v", err)
	}
	if username != "testuser" {
		t.Errorf("username = %q, want %q", username, "testuser")
	}
}

func TestGetUsernameEmpty(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/identity", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(identityResponse{Username: ""})
	})

	client, srv := newTestClient(mux)
	defer srv.Close()

	origBase := discogsBaseURL
	setBaseURL(srv.URL)
	defer setBaseURL(origBase)

	_, err := client.getUsername()
	if err == nil {
		t.Fatal("expected error for empty username")
	}
}

func TestGetFolders(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/users/testuser/collection/folders", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(foldersResponse{
			Folders: []folder{
				{ID: 0, Name: "All"},
				{ID: 1, Name: "Uncategorized"},
				{ID: 2, Name: "Vinyl 12\""},
			},
		})
	})

	client, srv := newTestClient(mux)
	defer srv.Close()

	origBase := discogsBaseURL
	setBaseURL(srv.URL)
	defer setBaseURL(origBase)

	folders, err := client.getFolders("testuser")
	if err != nil {
		t.Fatalf("getFolders: %v", err)
	}
	if len(folders) != 3 {
		t.Fatalf("got %d folders, want 3", len(folders))
	}
	if folders[2].Name != "Vinyl 12\"" {
		t.Errorf("folder[2].Name = %q", folders[2].Name)
	}
}

func TestGetCollectionReleases(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/users/testuser/collection/folders/0/releases", func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		var resp collectionPage
		if page == "" || page == "1" {
			resp.Pagination.Pages = 2
			resp.Releases = []collectionRelease{
				{BasicInformation: releaseInfo{
					Title:   "Souvlaki",
					Artists: []releaseArtist{{Name: "Slowdive"}},
				}},
			}
		} else {
			resp.Pagination.Pages = 2
			resp.Releases = []collectionRelease{
				{BasicInformation: releaseInfo{
					Title:   "Nowhere",
					Artists: []releaseArtist{{Name: "Ride"}},
				}},
			}
		}
		json.NewEncoder(w).Encode(resp)
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
	if len(albums) != 2 {
		t.Fatalf("got %d albums, want 2", len(albums))
	}
	if albums[0].Artist != "Slowdive" || albums[0].Title != "Souvlaki" {
		t.Errorf("album[0] = %+v", albums[0])
	}
	if albums[1].Artist != "Ride" || albums[1].Title != "Nowhere" {
		t.Errorf("album[1] = %+v", albums[1])
	}
}

func TestGetCollectionReleasesNoArtist(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/users/testuser/collection/folders/0/releases", func(w http.ResponseWriter, r *http.Request) {
		resp := collectionPage{
			Releases: []collectionRelease{
				{BasicInformation: releaseInfo{Title: "Compilation", Artists: nil}},
			},
		}
		resp.Pagination.Pages = 1
		json.NewEncoder(w).Encode(resp)
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
	if albums[0].Artist != "Unknown Artist" {
		t.Errorf("expected 'Unknown Artist', got %q", albums[0].Artist)
	}
}

func TestGetCollectionReleasesWithMetadata(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/users/testuser/collection/folders/0/releases", func(w http.ResponseWriter, r *http.Request) {
		resp := collectionPage{
			Pagination: struct {
				Pages int `json:"pages"`
			}{Pages: 1},
			Releases: []collectionRelease{{
				BasicInformation: releaseInfo{
					Title:   "Kind of Blue",
					Artists: []releaseArtist{{Name: "Miles Davis"}},
					Year:    1959,
					Labels:  []releaseLabel{{Name: "Columbia", CatNo: "CL 1355"}},
					Genres:  []string{"Jazz", "Bebop"},
					Formats: []releaseFormat{{Name: "Vinyl", Descriptions: []string{"12\""}}},
				},
			}},
		}
		json.NewEncoder(w).Encode(resp)
	})

	client, srv := newTestClient(mux)
	defer srv.Close()

	origBase := discogsBaseURL
	setBaseURL(srv.URL)
	defer setBaseURL(origBase)

	albums, err := client.getCollectionReleases("testuser", 0)
	if err != nil {
		t.Fatalf("getCollectionReleases failed: %v", err)
	}
	if len(albums) != 1 {
		t.Fatalf("got %d albums, want 1", len(albums))
	}

	album := albums[0]
	if album.Year != 1959 {
		t.Errorf("Year = %d, want 1959", album.Year)
	}
	if album.Label != "Columbia" {
		t.Errorf("Label = %q, want Columbia", album.Label)
	}
	if album.CatNo != "CL 1355" {
		t.Errorf("CatNo = %q, want CL 1355", album.CatNo)
	}
	if len(album.Genres) != 2 || album.Genres[0] != "Jazz" {
		t.Errorf("Genres = %v, want [Jazz Bebop]", album.Genres)
	}
	if len(album.Formats) != 2 || album.Formats[0] != "Vinyl" {
		t.Errorf("Formats = %v, want [Vinyl 12\"]", album.Formats)
	}
}

func TestGetAPIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/identity", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized"))
	})

	client, srv := newTestClient(mux)
	defer srv.Close()

	origBase := discogsBaseURL
	setBaseURL(srv.URL)
	defer setBaseURL(origBase)

	_, err := client.getUsername()
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}

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
