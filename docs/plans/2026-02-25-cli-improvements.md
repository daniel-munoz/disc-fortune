# CLI Improvements Implementation Plan

**Goal:** Add metadata-rich output, filtering, history tracking, and favorites management to disc-fortune.

**Architecture:** Expand Album struct with metadata, add history/favorites JSON files alongside collection.json, implement in-memory filtering, add ANSI color output with TTY detection.

**Tech Stack:** Go stdlib only (encoding/json, flag, os, time), ANSI escape codes for colors

---

## Task 1: Expand Album struct with metadata fields

**Files:**
- Modify: `collection.go:16-20`
- Modify: `collection_test.go` (update test fixtures)

**Step 1: Write failing test for Album with metadata**

In `collection_test.go`, add test for new Album fields:

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./... -v`
Expected: Compilation error - Album struct doesn't have Year, Label, etc.

**Step 3: Update Album struct**

In `collection.go`, replace Album struct (lines 16-20):

```go
// Album represents a single record with metadata.
type Album struct {
	Artist  string   `json:"artist"`
	Title   string   `json:"title"`
	Year    int      `json:"year,omitempty"`
	Label   string   `json:"label,omitempty"`
	CatNo   string   `json:"catno,omitempty"`
	Genres  []string `json:"genres,omitempty"`
	Formats []string `json:"formats,omitempty"`
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./... -v`
Expected: PASS - Album now has metadata fields

**Step 5: Commit**

```bash
git add collection.go collection_test.go
git commit -m "feat: expand Album struct with metadata fields"
```

---

## Task 2: Update Discogs API to extract metadata

**Files:**
- Modify: `discogs.go:114-118` (releaseInfo struct)
- Modify: `discogs.go:152-161` (Album construction)
- Test: `discogs_test.go`

**Step 1: Write failing test for metadata extraction**

In `discogs_test.go`, add test:

```go
func TestGetCollectionReleasesWithMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(collectionPage{
			Pagination: struct{ Pages int `json:"pages"` }{Pages: 1},
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
		})
	}))
	defer server.Close()
	setBaseURL(server.URL)

	client := &discogsClient{token: "test", httpClient: &http.Client{}}
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./... -v`
Expected: FAIL - releaseInfo doesn't have Year, Labels, etc.

**Step 3: Update releaseInfo struct and parsing**

In `discogs.go`, expand releaseInfo (lines 114-118):

```go
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
```

Update Album construction in `getCollectionReleases` (lines 152-161):

```go
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
```

**Step 4: Run test to verify it passes**

Run: `go test ./... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add discogs.go discogs_test.go
git commit -m "feat: extract metadata from Discogs API"
```

---

## Task 3: Add history tracking

**Files:**
- Create: `history.go`
- Create: `history_test.go`

**Step 1: Write failing test for history**

Create `history_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAddToHistory(t *testing.T) {
	tmpDir := t.TempDir()
	historyPath := filepath.Join(tmpDir, "history.json")

	album := Album{Artist: "Miles Davis", Title: "Kind of Blue"}
	err := addToHistory(historyPath, album)
	if err != nil {
		t.Fatalf("addToHistory failed: %v", err)
	}

	entries, err := loadHistory(historyPath)
	if err != nil {
		t.Fatalf("loadHistory failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Album.Artist != "Miles Davis" {
		t.Errorf("Artist = %q, want Miles Davis", entries[0].Album.Artist)
	}
	if entries[0].Timestamp.IsZero() {
		t.Error("Timestamp is zero")
	}
}

func TestLoadHistoryEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	historyPath := filepath.Join(tmpDir, "nonexistent.json")

	entries, err := loadHistory(historyPath)
	if err != nil {
		t.Fatalf("loadHistory failed: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0", len(entries))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./... -v`
Expected: FAIL - undefined: addToHistory, loadHistory

**Step 3: Implement history tracking**

Create `history.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// HistoryEntry represents a single pick with timestamp.
type HistoryEntry struct {
	Album     Album     `json:"album"`
	Timestamp time.Time `json:"timestamp"`
}

func historyPath() string {
	return filepath.Join(configDir(), "history.json")
}

// loadHistory loads history entries from disk.
func loadHistory(path string) ([]HistoryEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []HistoryEntry{}, nil
		}
		return nil, err
	}
	var entries []HistoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parsing history.json: %w", err)
	}
	return entries, nil
}

// saveHistory saves history entries to disk.
func saveHistory(path string, entries []HistoryEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), configDirPerms); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding history: %w", err)
	}
	return os.WriteFile(path, data, collectionFilePerms)
}

// addToHistory appends an album to history.
func addToHistory(path string, album Album) error {
	entries, err := loadHistory(path)
	if err != nil {
		return err
	}
	entries = append(entries, HistoryEntry{
		Album:     album,
		Timestamp: time.Now(),
	})
	return saveHistory(path, entries)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add history.go history_test.go
git commit -m "feat: add history tracking"
```

---

## Task 4: Add favorites management

**Files:**
- Create: `favorites.go`
- Create: `favorites_test.go`

**Step 1: Write failing test for favorites**

Create `favorites_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAddFavorite(t *testing.T) {
	tmpDir := t.TempDir()
	favPath := filepath.Join(tmpDir, "favorites.json")

	album := Album{Artist: "Miles Davis", Title: "Kind of Blue"}
	err := addFavorite(favPath, album)
	if err != nil {
		t.Fatalf("addFavorite failed: %v", err)
	}

	favs, err := loadFavorites(favPath)
	if err != nil {
		t.Fatalf("loadFavorites failed: %v", err)
	}
	if len(favs) != 1 {
		t.Fatalf("got %d favorites, want 1", len(favs))
	}
	if favs[0].Artist != "Miles Davis" {
		t.Errorf("Artist = %q, want Miles Davis", favs[0].Artist)
	}
}

func TestAddFavoriteDuplicate(t *testing.T) {
	tmpDir := t.TempDir()
	favPath := filepath.Join(tmpDir, "favorites.json")

	album := Album{Artist: "Miles Davis", Title: "Kind of Blue"}
	addFavorite(favPath, album)
	err := addFavorite(favPath, album)

	if err == nil {
		t.Fatal("expected error for duplicate favorite, got nil")
	}
	if err.Error() != "already in favorites" {
		t.Errorf("error = %q, want 'already in favorites'", err.Error())
	}
}

func TestRemoveFavorite(t *testing.T) {
	tmpDir := t.TempDir()
	favPath := filepath.Join(tmpDir, "favorites.json")

	album := Album{Artist: "Miles Davis", Title: "Kind of Blue"}
	addFavorite(favPath, album)

	err := removeFavorite(favPath, album)
	if err != nil {
		t.Fatalf("removeFavorite failed: %v", err)
	}

	favs, err := loadFavorites(favPath)
	if err != nil {
		t.Fatalf("loadFavorites failed: %v", err)
	}
	if len(favs) != 0 {
		t.Errorf("got %d favorites, want 0", len(favs))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./... -v`
Expected: FAIL - undefined: addFavorite, loadFavorites, removeFavorite

**Step 3: Implement favorites management**

Create `favorites.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func favoritesPath() string {
	return filepath.Join(configDir(), "favorites.json")
}

// loadFavorites loads favorite albums from disk.
func loadFavorites(path string) ([]Album, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Album{}, nil
		}
		return nil, err
	}
	var albums []Album
	if err := json.Unmarshal(data, &albums); err != nil {
		return nil, fmt.Errorf("parsing favorites.json: %w", err)
	}
	return albums, nil
}

// saveFavorites saves favorite albums to disk.
func saveFavorites(path string, albums []Album) error {
	if err := os.MkdirAll(filepath.Dir(path), configDirPerms); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	data, err := json.MarshalIndent(albums, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding favorites: %w", err)
	}
	return os.WriteFile(path, data, collectionFilePerms)
}

// addFavorite adds an album to favorites if not already present.
func addFavorite(path string, album Album) error {
	favorites, err := loadFavorites(path)
	if err != nil {
		return err
	}

	// Check for duplicates
	key := album.Key()
	for _, fav := range favorites {
		if fav.Key() == key {
			return fmt.Errorf("already in favorites")
		}
	}

	favorites = append(favorites, album)
	return saveFavorites(path, favorites)
}

// removeFavorite removes an album from favorites.
func removeFavorite(path string, album Album) error {
	favorites, err := loadFavorites(path)
	if err != nil {
		return err
	}

	key := album.Key()
	var filtered []Album
	found := false
	for _, fav := range favorites {
		if fav.Key() == key {
			found = true
			continue
		}
		filtered = append(filtered, fav)
	}

	if !found {
		return fmt.Errorf("not in favorites")
	}

	return saveFavorites(path, filtered)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add favorites.go favorites_test.go
git commit -m "feat: add favorites management"
```

---

## Task 5: Add filtering logic

**Files:**
- Create: `filter.go`
- Create: `filter_test.go`

**Step 1: Write failing test for filtering**

Create `filter_test.go`:

```go
package main

import (
	"testing"
)

func TestFilterByYear(t *testing.T) {
	albums := []Album{
		{Artist: "A", Title: "1", Year: 1970},
		{Artist: "B", Title: "2", Year: 1975},
		{Artist: "C", Title: "3", Year: 1980},
		{Artist: "D", Title: "4", Year: 0}, // no year
	}

	tests := []struct {
		yearFilter string
		want       int
	}{
		{"1975", 1},
		{"1970-1975", 2},
		{"1975-1970", 2}, // auto-swap
		{"", 4},          // no filter
	}

	for _, tt := range tests {
		t.Run(tt.yearFilter, func(t *testing.T) {
			f := Filter{Year: tt.yearFilter}
			filtered := f.Apply(albums)
			if len(filtered) != tt.want {
				t.Errorf("got %d albums, want %d", len(filtered), tt.want)
			}
		})
	}
}

func TestFilterByGenre(t *testing.T) {
	albums := []Album{
		{Artist: "A", Title: "1", Genres: []string{"Jazz", "Bebop"}},
		{Artist: "B", Title: "2", Genres: []string{"Rock"}},
		{Artist: "C", Title: "3", Genres: []string{}},
	}

	f := Filter{Genre: "jazz"}
	filtered := f.Apply(albums)
	if len(filtered) != 1 {
		t.Errorf("got %d albums, want 1", len(filtered))
	}
	if filtered[0].Artist != "A" {
		t.Errorf("Artist = %q, want A", filtered[0].Artist)
	}
}

func TestFilterCombined(t *testing.T) {
	albums := []Album{
		{Artist: "A", Title: "1", Year: 1970, Genres: []string{"Jazz"}},
		{Artist: "B", Title: "2", Year: 1970, Genres: []string{"Rock"}},
		{Artist: "C", Title: "3", Year: 1980, Genres: []string{"Jazz"}},
	}

	f := Filter{Year: "1970", Genre: "jazz"}
	filtered := f.Apply(albums)
	if len(filtered) != 1 {
		t.Errorf("got %d albums, want 1", len(filtered))
	}
	if filtered[0].Artist != "A" {
		t.Errorf("Artist = %q, want A", filtered[0].Artist)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./... -v`
Expected: FAIL - undefined: Filter

**Step 3: Implement filtering**

Create `filter.go`:

```go
package main

import (
	"fmt"
	"strconv"
	"strings"
)

// Filter represents album filtering criteria.
type Filter struct {
	Year   string
	Genre  string
	Label  string
	Format string
}

// Apply filters albums based on criteria.
func (f Filter) Apply(albums []Album) []Album {
	if f.Year == "" && f.Genre == "" && f.Label == "" && f.Format == "" {
		return albums
	}

	var filtered []Album
	for _, album := range albums {
		if f.matches(album) {
			filtered = append(filtered, album)
		}
	}
	return filtered
}

func (f Filter) matches(album Album) bool {
	if f.Year != "" && !f.matchesYear(album.Year) {
		return false
	}
	if f.Genre != "" && !f.matchesGenre(album.Genres) {
		return false
	}
	if f.Label != "" && !f.matchesString(album.Label, f.Label) {
		return false
	}
	if f.Format != "" && !f.matchesFormats(album.Formats) {
		return false
	}
	return true
}

func (f Filter) matchesYear(year int) bool {
	if year == 0 {
		return false
	}

	// Parse year or year range
	if strings.Contains(f.Year, "-") {
		parts := strings.Split(f.Year, "-")
		if len(parts) != 2 {
			return false
		}
		start, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		end, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil {
			return false
		}
		// Auto-swap if backwards
		if start > end {
			start, end = end, start
		}
		return year >= start && year <= end
	}

	// Single year
	targetYear, err := strconv.Atoi(f.Year)
	if err != nil {
		return false
	}
	return year == targetYear
}

func (f Filter) matchesGenre(genres []string) bool {
	for _, g := range genres {
		if f.matchesString(g, f.Genre) {
			return true
		}
	}
	return false
}

func (f Filter) matchesFormats(formats []string) bool {
	for _, format := range formats {
		if f.matchesString(format, f.Format) {
			return true
		}
	}
	return false
}

func (f Filter) matchesString(value, filter string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(filter))
}

// ParseYearFilter validates year filter format.
func ParseYearFilter(yearStr string) error {
	if yearStr == "" {
		return nil
	}

	if strings.Contains(yearStr, "-") {
		parts := strings.Split(yearStr, "-")
		if len(parts) != 2 {
			return fmt.Errorf("invalid year format. Use --year 1975 or --year 1970-1980")
		}
		_, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		_, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil {
			return fmt.Errorf("invalid year format. Use --year 1975 or --year 1970-1980")
		}
		return nil
	}

	_, err := strconv.Atoi(yearStr)
	if err != nil {
		return fmt.Errorf("invalid year format. Use --year 1975 or --year 1970-1980")
	}
	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add filter.go filter_test.go
git commit -m "feat: add album filtering logic"
```

---

## Task 6: Add colored output

**Files:**
- Create: `color.go`
- Create: `color_test.go`

**Step 1: Write failing test for color output**

Create `color_test.go`:

```go
package main

import (
	"os"
	"strings"
	"testing"
)

func TestFormatAlbum(t *testing.T) {
	album := Album{
		Artist:  "Miles Davis",
		Title:   "Kind of Blue",
		Year:    1959,
		Label:   "Columbia",
		CatNo:   "CL 1355",
		Genres:  []string{"Jazz"},
		Formats: []string{"Vinyl", "12\""},
	}

	// Test with color
	output := formatAlbum(album, true)
	if !strings.Contains(output, "Miles Davis") {
		t.Error("output missing artist")
	}
	if !strings.Contains(output, "Kind of Blue") {
		t.Error("output missing title")
	}
	if !strings.Contains(output, "1959") {
		t.Error("output missing year")
	}
	if !strings.Contains(output, "\033[") {
		t.Error("output missing ANSI codes")
	}

	// Test without color
	output = formatAlbum(album, false)
	if strings.Contains(output, "\033[") {
		t.Error("output should not have ANSI codes")
	}
}

func TestIsTTY(t *testing.T) {
	// Just verify function exists and returns bool
	_ = isTTY(os.Stdout)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./... -v`
Expected: FAIL - undefined: formatAlbum, isTTY

**Step 3: Implement color output**

Create `color.go`:

```go
package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"
)

const (
	colorReset      = "\033[0m"
	colorBoldCyan   = "\033[1;36m"
	colorBoldWhite  = "\033[1;37m"
	colorDim        = "\033[2m"
)

// isTTY returns true if the file is a terminal.
func isTTY(f *os.File) bool {
	_, err := syscall.IoctlGetTermios(int(f.Fd()), syscall.TIOCGETA)
	return err == nil
}

// formatAlbum formats an album for display with optional color.
func formatAlbum(album Album, useColor bool) string {
	var sb strings.Builder

	// First line: Artist - Title
	if useColor {
		sb.WriteString(colorBoldCyan)
		sb.WriteString(album.Artist)
		sb.WriteString(colorReset)
		sb.WriteString(" - ")
		sb.WriteString(colorBoldWhite)
		sb.WriteString(album.Title)
		sb.WriteString(colorReset)
	} else {
		sb.WriteString(album.Artist)
		sb.WriteString(" - ")
		sb.WriteString(album.Title)
	}

	// Second line: metadata (if any)
	var metadata []string
	if album.Year != 0 {
		metadata = append(metadata, fmt.Sprintf("%d", album.Year))
	}
	if album.Label != "" {
		metadata = append(metadata, album.Label)
	}
	if album.CatNo != "" {
		metadata = append(metadata, album.CatNo)
	}
	if len(album.Genres) > 0 {
		metadata = append(metadata, strings.Join(album.Genres, ", "))
	}

	if len(metadata) > 0 {
		sb.WriteString("\n")
		if useColor {
			sb.WriteString(colorDim)
		}
		sb.WriteString(strings.Join(metadata, " · "))
		if useColor {
			sb.WriteString(colorReset)
		}
	}

	return sb.String()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add color.go color_test.go
git commit -m "feat: add colored terminal output"
```

---

## Task 7: Add history display with relative timestamps

**Files:**
- Modify: `history.go` (add formatTimestamp, formatHistory functions)
- Modify: `history_test.go` (add tests)

**Step 1: Write failing test for history formatting**

In `history_test.go`, add:

```go
func TestFormatTimestamp(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name string
		ts   time.Time
		want string
	}{
		{"2 hours ago", now.Add(-2 * time.Hour), "2 hours ago"},
		{"yesterday", now.Add(-25 * time.Hour), "yesterday"},
		{"2 days ago", now.Add(-48 * time.Hour), "2 days ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTimestamp(tt.ts)
			if !strings.Contains(got, tt.want) && !strings.Contains(got, "/") {
				t.Errorf("formatTimestamp(%v) = %q, want something like %q", tt.ts, got, tt.want)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./... -v`
Expected: FAIL - undefined: formatTimestamp

**Step 3: Implement timestamp formatting**

In `history.go`, add:

```go
// formatTimestamp formats a timestamp as relative time or date.
func formatTimestamp(ts time.Time) string {
	now := time.Now()
	diff := now.Sub(ts)

	switch {
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins < 1 {
			return "just now"
		}
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case diff < 48*time.Hour:
		return "yesterday"
	case diff < 7*24*time.Hour:
		days := int(diff.Hours() / 24)
		return fmt.Sprintf("%d days ago", days)
	default:
		return ts.Format("2006-01-02")
	}
}

// formatHistory formats history entries for display.
func formatHistory(entries []HistoryEntry, limit int, useColor bool) string {
	if len(entries) == 0 {
		return "No history yet"
	}

	if limit <= 0 || limit > len(entries) {
		limit = len(entries)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("History (last %d picks):\n", limit))

	// Reverse order (most recent first)
	for i := len(entries) - 1; i >= len(entries)-limit; i-- {
		entry := entries[i]
		idx := len(entries) - i

		sb.WriteString(fmt.Sprintf("  %d. %s: ", idx, formatTimestamp(entry.Timestamp)))

		if useColor {
			sb.WriteString(colorBoldCyan)
		}
		sb.WriteString(entry.Album.Artist)
		if useColor {
			sb.WriteString(colorReset)
		}
		sb.WriteString(" - ")
		if useColor {
			sb.WriteString(colorBoldWhite)
		}
		sb.WriteString(entry.Album.Title)
		if useColor {
			sb.WriteString(colorReset)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add history.go history_test.go
git commit -m "feat: add history display with relative timestamps"
```

---

## Task 8: Wire up new flags and logic in main

**Files:**
- Modify: `main.go` (add flags, update runFortune, add new commands)

**Step 1: Write integration test**

In `main_test.go`, add test for new flags (this will be a smoke test):

```go
func TestFilterFlags(t *testing.T) {
	// Just verify flags parse without error
	// Actual filtering logic tested in filter_test.go
}
```

**Step 2: Update main.go to add flags**

Add new flags after existing ones (around line 31):

```go
var folderFlags arrayFlags
flag.Var(&folderFlags, "folder", "Sync only specific folder(s) by name (repeatable, use with --sync)")

// New flags
historyFlag := flag.Int("history", 0, "Show pick history (default 10, 0 shows all)")
favoritesFlag := flag.Bool("favorites", false, "Pick randomly from favorites only")
favoriteLast := flag.Bool("favorite-last", false, "Add last pick to favorites")
unfavoriteLast := flag.Bool("unfavorite-last", false, "Remove last pick from favorites")

yearFlag := flag.String("year", "", "Filter by year or year range (e.g., 1975 or 1970-1980)")
genreFlag := flag.String("genre", "", "Filter by genre (case-insensitive substring match)")
labelFlag := flag.String("label", "", "Filter by label (case-insensitive substring match)")
formatFlag := flag.String("format", "", "Filter by format (case-insensitive substring match)")

flag.Parse()
```

**Step 3: Add command handlers**

After version flag check, add:

```go
if *historyFlag != 0 || flag.Lookup("history").Value.String() != "" {
	runHistory(*historyFlag)
	return
}

if *favoriteLast {
	runFavoriteLast()
	return
}

if *unfavoriteLast {
	runUnfavoriteLast()
	return
}
```

**Step 4: Update runFortune to support filtering and favorites**

Replace `runFortune` function:

```go
func runFortune(favoritesOnly bool, filter Filter) {
	var albums []Album
	var err error

	if favoritesOnly {
		albums, err = loadFavorites(favoritesPath())
		if err != nil {
			fatal("Error loading favorites: %v", err)
		}
		if len(albums) == 0 {
			fmt.Println("No favorites yet. Use --favorite-last after a pick you like.")
			os.Exit(0)
		}
	} else {
		albums, err = loadCollection()
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("No collection found. Run `disc-fortune --sync` to fetch your Discogs collection.")
				os.Exit(0)
			}
			fatal("Error loading collection: %v", err)
		}
		if len(albums) == 0 {
			fmt.Println("Collection is empty. Run `disc-fortune --sync` to fetch your Discogs collection.")
			os.Exit(0)
		}
	}

	// Apply filters
	albums = filter.Apply(albums)
	if len(albums) == 0 {
		fmt.Println("No albums match the specified filters")
		os.Exit(0)
	}

	album := randomAlbum(albums)

	// Add to history
	if err := addToHistory(historyPath(), album); err != nil {
		fatal("Error saving history: %v", err)
	}

	// Format and print
	useColor := isTTY(os.Stdout)
	fmt.Println(formatAlbum(album, useColor))
}
```

**Step 5: Add new command functions**

Add these functions:

```go
func runHistory(limit int) {
	entries, err := loadHistory(historyPath())
	if err != nil {
		fatal("Error loading history: %v", err)
	}

	if limit == 0 {
		limit = 10
	}

	useColor := isTTY(os.Stdout)
	fmt.Print(formatHistory(entries, limit, useColor))
}

func runFavoriteLast() {
	entries, err := loadHistory(historyPath())
	if err != nil {
		fatal("Error loading history: %v", err)
	}
	if len(entries) == 0 {
		fatal("No history to favorite")
	}

	lastAlbum := entries[len(entries)-1].Album
	err = addFavorite(favoritesPath(), lastAlbum)
	if err != nil {
		if err.Error() == "already in favorites" {
			fmt.Println("Already in favorites")
			return
		}
		fatal("Error adding favorite: %v", err)
	}

	fmt.Printf("Added to favorites: %s - %s\n", lastAlbum.Artist, lastAlbum.Title)
}

func runUnfavoriteLast() {
	entries, err := loadHistory(historyPath())
	if err != nil {
		fatal("Error loading history: %v", err)
	}
	if len(entries) == 0 {
		fatal("No history to unfavorite")
	}

	lastAlbum := entries[len(entries)-1].Album
	err = removeFavorite(favoritesPath(), lastAlbum)
	if err != nil {
		if err.Error() == "not in favorites" {
			fmt.Println("Last pick was not in favorites")
			return
		}
		fatal("Error removing favorite: %v", err)
	}

	fmt.Printf("Removed from favorites: %s - %s\n", lastAlbum.Artist, lastAlbum.Title)
}
```

**Step 6: Update main function call to runFortune**

Replace the call to `runFortune()` (around line 51):

```go
// Validate year filter
if err := ParseYearFilter(*yearFlag); err != nil {
	fatal("Error: %v", err)
}

// Build filter
filter := Filter{
	Year:   *yearFlag,
	Genre:  *genreFlag,
	Label:  *labelFlag,
	Format: *formatFlag,
}

runFortune(*favoritesFlag, filter)
```

**Step 7: Run tests**

Run: `go test ./... -v`
Expected: PASS

**Step 8: Build and test manually**

```bash
go build -o disc-fortune .
./disc-fortune --help
```

**Step 9: Commit**

```bash
git add main.go main_test.go
git commit -m "feat: wire up filtering, history, and favorites flags"
```

---

## Task 9: Update sync summary to show metadata counts

**Files:**
- Modify: `main.go:102` (update sync summary message)

**Step 1: Implement metadata counting**

In `main.go`, replace the sync summary (line 102):

```go
withMetadata := 0
for _, album := range albums {
	if album.Year != 0 || album.Label != "" || len(album.Genres) > 0 {
		withMetadata++
	}
}

fmt.Printf("Synced %d albums (%d with full metadata)\n", len(albums), withMetadata)
```

**Step 2: Test manually**

```bash
go build -o disc-fortune .
# Test with actual Discogs account if available
```

**Step 3: Commit**

```bash
git add main.go
git commit -m "feat: show metadata count in sync summary"
```

---

## Task 10: Update README with new features

**Files:**
- Modify: `README.md`

**Step 1: Update README**

Replace the Usage section with:

```markdown
## Usage

```sh
# Sync your full Discogs collection
disc-fortune --sync

# List available folder names
disc-fortune --sync --list-folders

# Sync only specific folders
disc-fortune --sync --folder "Vinyl 12\"" --folder "Vinyl 7\""

# Print a random album
disc-fortune

# Filter by year or year range
disc-fortune --year 1975
disc-fortune --year 1970-1980

# Filter by genre, label, or format
disc-fortune --genre jazz
disc-fortune --label blue-note
disc-fortune --format 12\"

# Combine filters
disc-fortune --year 1970-1980 --genre jazz

# View pick history
disc-fortune --history        # last 10 picks
disc-fortune --history 25     # last 25 picks

# Mark the last pick as a favorite
disc-fortune --favorite-last

# Pick randomly from favorites only
disc-fortune --favorites

# Filter within favorites
disc-fortune --favorites --year 1970-1980

# Remove last pick from favorites
disc-fortune --unfavorite-last
```
```

Add after Usage section:

```markdown
## Features

- **Metadata-rich display** - Shows year, label, catalog number, and genres with color-coded output
- **Flexible filtering** - Filter by year, genre, label, or format
- **Pick history** - Track all your picks with timestamps
- **Favorites** - Mark albums you love and pick randomly from that subset
- **Offline operation** - All data stored locally after initial sync
```

**Step 2: Commit**

```bash
git add README.md
git commit -m "docs: update README with new features"
```

---

## Task 11: Update version and release notes

**Files:**
- Modify: `main.go:10` (version constant)
- Modify: `RELEASE_NOTES.md`

**Step 1: Update version**

In `main.go`, change version (line 10):

```go
const version = "2.0.0"
```

**Step 2: Update release notes**

In `RELEASE_NOTES.md`, add at the top:

```markdown
# disc-fortune v2.0.0

Major feature release adding metadata display, filtering, history, and favorites.

## New Features

- **Metadata display** - Albums now show year, label, catalog number, and genres
- **Colored output** - Artist, title, and metadata color-coded in terminal (auto-disabled when piped)
- **Year filtering** - `--year 1975` or `--year 1970-1980`
- **Genre filtering** - `--genre jazz` (case-insensitive substring match)
- **Label filtering** - `--label blue-note`
- **Format filtering** - `--format 12\"`
- **Pick history** - `--history` shows past picks with relative timestamps
- **Favorites** - `--favorite-last` to mark, `--favorites` to pick from favorites only
- **Unfavorite** - `--unfavorite-last` to remove last pick from favorites
- **Filter combinations** - All filters work together and with favorites

## Data Storage

Collection now includes metadata. After upgrading, run `disc-fortune --sync` to populate metadata for existing albums.

Three JSON files in `~/.config/disc-fortune/`:
- `collection.json` - enriched with metadata
- `history.json` - timestamped picks
- `favorites.json` - favorited albums

## Breaking Changes

None - v1.0.0 collection files are automatically compatible.

---

```

**Step 3: Commit**

```bash
git add main.go RELEASE_NOTES.md
git commit -m "chore: bump version to 2.0.0"
```

---

## Final Verification

**Step 1: Run all tests**

```bash
go test ./... -v -race
```

Expected: All tests pass

**Step 2: Build and test key workflows**

```bash
go build -o disc-fortune .

# Test basic pick (will need actual collection or mock)
./disc-fortune

# Test filtering
./disc-fortune --year 1970-1980

# Test history
./disc-fortune --history

# Test favorites workflow
./disc-fortune --favorite-last
./disc-fortune --favorites
```

**Step 3: Final commit and tag**

```bash
git add -A
git commit -m "chore: final v2.0.0 verification"
git tag v2.0.0
```

---

## Implementation Notes

- Follow test-driven development: write tests first, verify they fail, implement, verify they pass
- Commit frequently (after each passing test)
- Run full test suite before final verification
