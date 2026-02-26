# disc-fortune CLI Improvements Design

**Date:** 2026-02-25
**Status:** Approved

## Overview

Enhance disc-fortune with metadata-rich output, flexible filtering, and favorites/history tracking while maintaining the tool's simplicity and stdlib-only design.

## Goals

- Display album metadata (year, label, catalog number, genres) with color-coded output
- Filter random selection by year, genre, label, and format
- Track pick history and allow users to mark favorites
- Maintain "no external dependencies" principle (stdlib only)
- Keep all data local for offline operation after initial sync

## Data Model

### Album Structure

Expand the `Album` struct to include metadata fetched from Discogs API:

```go
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

### Storage Files

Store three JSON files in `~/.config/disc-fortune/`:

1. **collection.json** - Enriched album data with metadata
2. **history.json** - Array of timestamped picks:
   ```json
   [
     {"album": {...}, "timestamp": "2026-02-25T14:30:00Z"},
     ...
   ]
   ```
3. **favorites.json** - Array of favorited albums (subset of collection)

### Metadata Extraction

During sync, extract metadata from the Discogs API collection endpoint response:
- Year: use the release year field
- Label: take the first label from the labels array
- Catalog number: from the basic information
- Genres: flatten the genres array
- Formats: flatten the formats array to strings

## Filtering System

### Filter Flags

Add command-line flags for filtering random selection:

- `--year <year>` or `--year <start-end>`
  - Examples: `--year 1975`, `--year 1970-1980`
- `--genre <genre>`
  - Case-insensitive substring match (e.g., `--genre jazz`)
- `--label <label>`
  - Case-insensitive substring match
- `--format <format>`
  - Case-insensitive substring match (e.g., `--format "12\""`)

### Filter Logic

- Filters use AND logic: albums must match all specified filters
- Albums with missing metadata fields won't match filters on those fields
- All filters work with both full collection and favorites subset
- Filtering happens in-memory at selection time

## Output & Display

### Colored Terminal Output

Use ANSI color codes for enhanced readability:
- **Artist:** bold cyan
- **Title:** bold white
- **Metadata line:** dim gray

Example output:
```
Miles Davis - Kind of Blue
1959 · Columbia · CL 1355 · Jazz
```

### History Display

`--history` (or `--history <N>`) shows numbered list, most recent first:

```
History (last 10 picks):
  1. 2 hours ago: Miles Davis - Kind of Blue
  2. yesterday: John Coltrane - A Love Supreme
  ...
```

Timestamps shown as:
- Relative time for recent picks (e.g., "2 hours ago", "yesterday")
- Full date for older picks

### Color Detection

Auto-disable color output when stdout is not a TTY (piped/redirected) to maintain script compatibility.

## History & Favorites

### History Tracking

- Every `disc-fortune` pick (without `--history` flag) appends to `history.json`
- History entry: `{album: Album, timestamp: "RFC3339"}`
- `--history` displays last 10 by default, accepts optional number argument
- No size limit initially (can add rotation later if needed)

### Favorites Workflow

**Adding favorites:**
1. User runs `disc-fortune` → sees a pick they love
2. User runs `disc-fortune --favorite-last` → adds that album to `favorites.json`
3. Success message: "Added to favorites: Artist - Title"
4. If already favorited: "Already in favorites"
5. If no history: error "No history to favorite"

**Selecting from favorites:**
- `--favorites` picks randomly from `favorites.json` only
- If no favorites exist: "No favorites yet. Use --favorite-last after a pick you like."
- Favorites picks are also added to history
- Filters work with favorites: `disc-fortune --favorites --year 1970-1980`

**Removing favorites:**
- `--unfavorite-last` removes the last history pick from favorites
- If not favorited: "Last pick was not in favorites"
- If no history: "No history to unfavorite"

## Error Handling

### Sync Behavior
- Missing metadata fields don't fail sync, store empty values
- Maintain deduplication using artist+title key
- Show summary: "Synced X albums (Y with full metadata)"

### Filter Edge Cases
- No matches: "No albums match the specified filters"
- Invalid year format: "Invalid year format. Use --year 1975 or --year 1970-1980"
- Backwards year range (1980-1970): auto-swap to correct order
- Empty collection: use existing "Collection is empty" message

### History Edge Cases
- No history file: `--history` shows "No history yet"
- `--favorite-last` with no history: "No history to favorite"

### Favorites Edge Cases
- `--favorites` with empty favorites: "No favorites yet. Use --favorite-last after a pick you like."
- `--unfavorite-last` with no history: "No history to unfavorite"
- `--unfavorite-last` when not favorited: "Last pick was not in favorites"

### File Operations
- Maintain existing error handling pattern for JSON operations
- Corrupted JSON: clear error with suggestion to re-sync or delete file

## Non-Goals

- SQLite or external database (breaks stdlib-only principle)
- On-demand metadata fetching (requires network for every pick)
- Album art display (complex, limited terminal support)
- Interactive mode (keep simple CLI interface)
- Weighted random selection by ratings (adds complexity)

## Implementation Notes

- All new functionality maintains backward compatibility with v1.0.0 collection files
- If old collection.json exists without metadata, sync will populate it
- Color codes should use standard ANSI escape sequences for portability
- Year range parsing should be defensive (handle spaces, validate numbers)
