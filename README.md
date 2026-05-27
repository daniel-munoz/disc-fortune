# disc-fortune

A Unix `fortune`-style CLI that randomly picks a vinyl record from your Discogs collection.

## Setup

1. Get a Discogs personal access token from https://www.discogs.com/settings/developers
2. Export it:
   ```sh
   export DISCOGS_TOKEN=your_token_here
   ```
3. Build:
   ```sh
   go build -o disc-fortune .
   ```

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
disc-fortune --history 10     # last 10 picks
disc-fortune --history 25     # last 25 picks
disc-fortune --history 0      # all picks

# Mark the last pick as a favorite
disc-fortune --favorite-last

# Favorite a specific album by query (case-insensitive substring of "Artist - Title")
disc-fortune --favorite "kind of blue"

# Favorite by query, narrowed with filters when the query alone is ambiguous
disc-fortune --favorite "miles" --year 1959
disc-fortune --favorite "coltrane" --genre jazz

# Pick randomly from favorites only
disc-fortune --favorites

# Filter within favorites
disc-fortune --favorites --year 1970-1980

# Remove last pick from favorites
disc-fortune --unfavorite-last

# List all albums matching filters
disc-fortune --list
disc-fortune --list --favorites
disc-fortune --list --genre jazz --year 1970-1980
disc-fortune --list --format 12\"
```

## Features

- **Metadata-rich display** - Shows year, label, catalog number, and genres with color-coded output
- **Flexible filtering** - Filter by year, genre, label, or format
- **Pick history** - Track all your picks with timestamps
- **Favorites** - Mark albums you love (by last pick or by query) and pick randomly from that subset
- **List mode** - Browse all albums (or filtered subsets) without picking one
- **Offline operation** - All data stored locally after initial sync

## Data

Your data is stored locally in `~/.config/disc-fortune/`:
- `collection.json` - Your full collection with metadata (artist, title, year, label, catalog number, genres, formats)
- `history.json` - Timestamped history of all your picks
- `favorites.json` - Albums you've marked as favorites
