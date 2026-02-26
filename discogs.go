package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

var discogsBaseURL = "https://api.discogs.com"

const (
	userAgent = "disc-fortune/1.0"
	perPage   = 100
)

// setBaseURL overrides the Discogs API base URL (used by tests).
func setBaseURL(url string) { discogsBaseURL = url }

// discogsClient wraps authenticated HTTP access to the Discogs API.
type discogsClient struct {
	token      string
	httpClient *http.Client
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

// get performs an authenticated GET request and returns the response body.
func (c *discogsClient) get(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Discogs token="+c.token)
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Discogs API error (%d): %s", resp.StatusCode, string(body))
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
}

// releaseInfo represents the basic_information of a collection release.
type releaseInfo struct {
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
func (c *discogsClient) getCollectionReleases(username string, folderID int) ([]Album, error) {
	var albums []Album
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
			}

			albums = append(albums, Album{
				Artist:  artist,
				Title:   r.BasicInformation.Title,
				Year:    r.BasicInformation.Year,
				Label:   label,
				CatNo:   catno,
				Genres:  r.BasicInformation.Genres,
				Formats: formats,
			})
		}

		if page >= cp.Pagination.Pages {
			break
		}
		page++
	}

	return albums, nil
}
