package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestSyncProgressDisabledReturnsNil(t *testing.T) {
	if got := syncProgress(io.Discard, false); got != nil {
		t.Error("progress must be nil when stderr is not a TTY, so nothing is emitted")
	}
}

func TestSyncProgressWritesToGivenWriter(t *testing.T) {
	var buf bytes.Buffer
	p := syncProgress(&buf, true)
	if p == nil {
		t.Fatal("progress must be non-nil when enabled")
	}
	p("page %d\n", 3)
	if got := buf.String(); got != "page 3\n" {
		t.Errorf("wrote %q, want %q", got, "page 3\n")
	}
}

// twoPageServer serves a two-page collection folder.
func twoPageServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/users/testuser/collection/folders/0/releases", func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		title := "Kind of Blue"
		if page == "2" {
			title = "Lanquidity"
		}
		var cp collectionPage
		cp.Pagination.Pages = 2
		cp.Releases = []collectionRelease{{BasicInformation: releaseInfo{
			Title:   title,
			Artists: []releaseArtist{{Name: "Someone"}},
		}}}
		json.NewEncoder(w).Encode(cp)
	})
	return httptest.NewServer(mux)
}

func TestGetCollectionReleasesReportsEachPage(t *testing.T) {
	srv := twoPageServer(t)
	defer srv.Close()
	origBase := discogsBaseURL
	setBaseURL(srv.URL)
	defer setBaseURL(origBase)

	var buf bytes.Buffer
	client := &discogsClient{
		token:      "test-token",
		httpClient: srv.Client(),
		progress:   syncProgress(&buf, true),
	}

	albums, err := client.getCollectionReleases("testuser", 0)
	if err != nil {
		t.Fatalf("getCollectionReleases: %v", err)
	}
	if len(albums) != 2 {
		t.Fatalf("albums = %d, want 2", len(albums))
	}

	out := buf.String()
	for _, want := range []string{"1/2", "2/2"} {
		if !strings.Contains(out, want) {
			t.Errorf("progress %q missing page marker %q", out, want)
		}
	}
}

// stdout is the data channel. Progress must never contaminate it, or piping
// `disc-fortune list` into another program breaks the moment a sync reports.
func TestProgressNeverTouchesStdout(t *testing.T) {
	srv := twoPageServer(t)
	defer srv.Close()
	origBase := discogsBaseURL
	setBaseURL(srv.URL)
	defer setBaseURL(origBase)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	origStdout := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = origStdout }()

	var buf bytes.Buffer
	client := &discogsClient{
		token:      "test-token",
		httpClient: srv.Client(),
		progress:   syncProgress(&buf, true),
	}
	if _, err := client.getCollectionReleases("testuser", 0); err != nil {
		t.Fatalf("getCollectionReleases: %v", err)
	}

	w.Close()
	os.Stdout = origStdout
	captured, _ := io.ReadAll(r)
	if len(captured) != 0 {
		t.Errorf("progress leaked to stdout: %q", captured)
	}
	if buf.Len() == 0 {
		t.Error("progress was on but produced nothing, so the check above proves nothing")
	}
	fmt.Fprint(io.Discard, "")
}

func TestNilProgressIsSilent(t *testing.T) {
	c := &discogsClient{}
	c.report("this must not panic %d\n", 1) // progress is nil
}
