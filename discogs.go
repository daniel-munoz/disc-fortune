package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/daniel-munoz/disc-fortune/v2/internal/disc"
)

var discogsBaseURL = "https://api.discogs.com"

const perPage = 100

// Retry policy. Discogs allows 60 authenticated requests per minute and
// answers a breach with 429. The four waits below sum to ~15s before jitter,
// which comfortably outlasts a one-minute rate-limit window's tail without
// letting a single page stall a sync for minutes.
const (
	maxAttempts     = 5
	baseBackoff     = time.Second
	maxBackoffStep  = 16 * time.Second
	maxTotalBackoff = 60 * time.Second
	// backoffJitter is the fraction of the computed delay that is added at
	// random, so a sync resuming after a shared outage does not hammer the
	// API in lockstep with every other client.
	backoffJitter = 0.25
)

// userAgent identifies the tool to Discogs, whose API terms ask for accurate
// identification. Deriving it from version means it cannot drift out of date
// the way the hardcoded "disc-fortune/1.0" did.
var userAgent = "disc-fortune/" + version

// setBaseURL overrides the Discogs API base URL (used by tests).
func setBaseURL(url string) { discogsBaseURL = url }

// progressFunc reports incremental progress during a long fetch. A nil
// progressFunc means progress reporting is off.
type progressFunc func(format string, args ...any)

// discogsClient wraps authenticated HTTP access to the Discogs API.
type discogsClient struct {
	token      string
	httpClient *http.Client
	// sleep waits out a backoff delay. nil means time.Sleep; tests replace it
	// so retry behavior can be asserted without real waiting.
	sleep func(time.Duration)
	// progress, when non-nil, receives page-by-page fetch progress.
	progress progressFunc
}

// newDiscogsClient creates a client using the DISCOGS_TOKEN env var.
func newDiscogsClient() (*discogsClient, error) {
	token := os.Getenv("DISCOGS_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("DISCOGS_TOKEN environment variable is not set")
	}
	return &discogsClient{
		token:      token,
		httpClient: &http.Client{},
	}, nil
}

func (c *discogsClient) nap(d time.Duration) {
	if c.sleep != nil {
		c.sleep(d)
		return
	}
	time.Sleep(d)
}

func (c *discogsClient) report(format string, args ...any) {
	if c.progress != nil {
		c.progress(format, args...)
	}
}

// httpError is a non-2xx response from the Discogs API.
type httpError struct {
	status     int
	body       string
	retryAfter time.Duration
}

func (e *httpError) Error() string {
	return fmt.Sprintf("Discogs API error (%d): %s", e.status, e.body)
}

// retryable reports whether repeating the request could plausibly succeed.
// 429 means we asked too fast; 5xx means the server is briefly unwell. A 4xx
// of any other kind is our fault and will fail identically forever.
func (e *httpError) retryable() bool {
	return e.status == http.StatusTooManyRequests || e.status >= 500
}

// transportError is a request that never produced a response. Worth retrying:
// a dropped connection mid-sync is not a permanent condition.
type transportError struct{ err error }

func (e *transportError) Error() string { return e.err.Error() }
func (e *transportError) Unwrap() error { return e.err }

// retryable reports whether err is worth another attempt.
func retryable(err error) bool {
	switch e := err.(type) {
	case *httpError:
		return e.retryable()
	case *transportError:
		return true
	default:
		return false
	}
}

// backoffDelay returns how long to wait before attempt+1, given how many
// attempts have already failed. It grows exponentially from baseBackoff up to
// maxBackoffStep, adds jitter, and defers to the server's Retry-After when
// that asks for longer.
func backoffDelay(failedAttempts int, retryAfter time.Duration) time.Duration {
	delay := baseBackoff << (failedAttempts - 1)
	if delay > maxBackoffStep {
		delay = maxBackoffStep
	}
	delay += time.Duration(rand.Float64() * backoffJitter * float64(delay))
	if retryAfter > delay {
		return retryAfter
	}
	return delay
}

// parseRetryAfter reads the delay-seconds form of the Retry-After header.
// Discogs sends seconds; the HTTP-date form is not honored, and an
// unparseable value simply falls back to our own backoff.
func parseRetryAfter(h string) time.Duration {
	secs, err := strconv.Atoi(h)
	if err != nil || secs < 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}

// get performs an authenticated GET request, retrying transient failures with
// exponential backoff, and returns the response body. Retries are bounded by
// both maxAttempts and maxTotalBackoff, so a persistently failing endpoint
// still fails — loudly, and in finite time.
func (c *discogsClient) get(url string) ([]byte, error) {
	var spent time.Duration

	for attempt := 1; ; attempt++ {
		body, err := c.attempt(url)
		if err == nil {
			return body, nil
		}
		if !retryable(err) || attempt >= maxAttempts {
			return nil, err
		}

		var retryAfter time.Duration
		if he, ok := err.(*httpError); ok {
			retryAfter = he.retryAfter
		}
		wait := backoffDelay(attempt, retryAfter)
		if spent+wait > maxTotalBackoff {
			return nil, fmt.Errorf("giving up after %v of retries: %w", spent, err)
		}

		c.report("  retrying in %s after %v\n", wait.Round(100*time.Millisecond), err)
		c.nap(wait)
		spent += wait
	}
}

// attempt performs one authenticated GET, classifying the outcome so get can
// decide whether it is worth repeating.
func (c *discogsClient) attempt(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Discogs token="+c.token)
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &transportError{err: err}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &transportError{err: err}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &httpError{
			status:     resp.StatusCode,
			body:       string(body),
			retryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
		}
	}
	return body, nil
}

// identityResponse represents the /oauth/identity response.
type identityResponse struct {
	Username string `json:"username"`
}

// getUsername retrieves the authenticated user's Discogs username.
func (c *discogsClient) getUsername() (string, error) {
	body, err := c.get(discogsBaseURL + "/oauth/identity")
	if err != nil {
		return "", fmt.Errorf("fetching identity: %w", err)
	}
	var identity identityResponse
	if err := json.Unmarshal(body, &identity); err != nil {
		return "", fmt.Errorf("parsing identity: %w", err)
	}
	if identity.Username == "" {
		return "", fmt.Errorf("empty username in identity response")
	}
	return identity.Username, nil
}

// folder represents a Discogs collection folder.
type folder struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type foldersResponse struct {
	Folders []folder `json:"folders"`
}

// getFolders returns the user's collection folders.
func (c *discogsClient) getFolders(username string) ([]folder, error) {
	url := fmt.Sprintf("%s/users/%s/collection/folders", discogsBaseURL, username)
	body, err := c.get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching folders: %w", err)
	}
	var resp foldersResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing folders: %w", err)
	}
	return resp.Folders, nil
}

// releaseArtist represents an artist in a Discogs release.
type releaseArtist struct {
	Name string `json:"name"`
}

// releaseLabel represents a label in a Discogs release.
type releaseLabel struct {
	Name  string `json:"name"`
	CatNo string `json:"catno"`
}

// releaseFormat represents a format in a Discogs release.
type releaseFormat struct {
	Name         string   `json:"name"`
	Descriptions []string `json:"descriptions"`
	// Text is the format's free-text qualifier, and it is where Discogs
	// records a pressing's colour -- "Blue Translucent", "Coke Bottle
	// Clear". Two store-exclusive colour variants of one album can be
	// identical in every other field, so this is often the only thing that
	// tells them apart.
	Text string `json:"text"`
}

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

// collectionRelease represents a release in a collection folder.
type collectionRelease struct {
	BasicInformation releaseInfo `json:"basic_information"`
}

// collectionPage represents a page of collection releases.
type collectionPage struct {
	Pagination struct {
		Pages int `json:"pages"`
	} `json:"pagination"`
	Releases []collectionRelease `json:"releases"`
}

// getCollectionReleases paginates through all releases in a folder.
func (c *discogsClient) getCollectionReleases(username string, folderID int) ([]disc.Album, error) {
	var albums []disc.Album
	page := 1

	for {
		url := fmt.Sprintf("%s/users/%s/collection/folders/%d/releases?page=%d&per_page=%d",
			discogsBaseURL, username, folderID, page, perPage)

		body, err := c.get(url)
		if err != nil {
			return nil, fmt.Errorf("fetching releases (folder %d, page %d): %w", folderID, page, err)
		}

		var cp collectionPage
		if err := json.Unmarshal(body, &cp); err != nil {
			return nil, fmt.Errorf("parsing releases: %w", err)
		}

		for _, r := range cp.Releases {
			artist := "Unknown Artist"
			if len(r.BasicInformation.Artists) > 0 {
				artist = r.BasicInformation.Artists[0].Name
			}

			label := ""
			catno := ""
			if len(r.BasicInformation.Labels) > 0 {
				label = r.BasicInformation.Labels[0].Name
				catno = r.BasicInformation.Labels[0].CatNo
			}

			var formats []string
			for _, f := range r.BasicInformation.Formats {
				formats = append(formats, f.Name)
				formats = append(formats, f.Descriptions...)
				if f.Text != "" {
					formats = append(formats, f.Text)
				}
			}

			albums = append(albums, disc.Album{
				ReleaseID: r.BasicInformation.ID,
				Artist:    artist,
				Title:     r.BasicInformation.Title,
				Year:      r.BasicInformation.Year,
				Label:     label,
				CatNo:     catno,
				Genres:    r.BasicInformation.Genres,
				Formats:   formats,
			})
		}

		c.report("  fetched page %d/%d (%d albums)\n", page, cp.Pagination.Pages, len(albums))

		if page >= cp.Pagination.Pages {
			break
		}
		page++
	}

	return albums, nil
}
