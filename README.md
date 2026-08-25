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
# Print a random album (the default — no command needed)
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

# Pick randomly from favorites only
disc-fortune pick --favorites
```

### Commands

| Command | What it does |
|---|---|
| `pick` | Print a random album. Runs by default when you give no command. |
| `list` | List every matching album, with a count. |
| `sync` | Fetch your collection from Discogs. |
| `folders` | List your Discogs folder names. |
| `history` | Show recent picks. |
| `favorite` | Add an album to favorites. |
| `unfavorite` | Remove an album from favorites. |
| `version` | Print the version. |
| `help` | Show help for a command. |

Run `disc-fortune help <command>` for details on any of them.

### Syncing

```sh
# Sync your full Discogs collection
disc-fortune sync

# List available folder names
disc-fortune folders

# Sync only specific folders
disc-fortune sync --folder "Vinyl 12\"" --folder "Vinyl 7\""
```

### Listing

```sh
# Every album in the collection
disc-fortune list

# Everything matching a filter
disc-fortune list --year 1970-1980 --genre jazz

# Favorites only
disc-fortune list --favorites
```

### History

```sh
disc-fortune history        # last 10 picks
disc-fortune history 25     # last 25 picks
disc-fortune history 0      # all picks
```

### Favorites

```sh
# Favorite the last pick
disc-fortune favorite

# Favorite a specific album (case-insensitive substring of "Artist - Title")
disc-fortune favorite "kind of blue"

# Narrow an ambiguous query with filters
disc-fortune favorite "miles" --year 1959
disc-fortune favorite "coltrane" --genre jazz

# Remove the last pick from favorites
disc-fortune unfavorite

# Remove a specific album from favorites
disc-fortune unfavorite "kind of blue"
```

### Exit codes

`disc-fortune` exits 0 when the command produced what you asked for, and 1 when
it could not — no collection synced yet, no albums matching your filters, an
ambiguous `favorite` or `unfavorite` query, or a usage error. Removing a
favorite that is not there exits 0, since the end state you asked for already
holds. `history` on an empty log also exits 0: a log report succeeds even when
the log has nothing in it.

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
