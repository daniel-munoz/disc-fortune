# disc-fortune

A Unix `fortune`-style CLI that randomly picks a vinyl record from your Discogs collection.

## Setup

1. Get a Discogs personal access token from https://www.discogs.com/settings/developers
2. Export it:
   ```sh
   export DISCOGS_TOKEN=your_token_here
   ```
3. Install it:
   ```sh
   go install github.com/daniel-munoz/disc-fortune/v2@latest
   ```
   The `/v2` suffix is required. `go install github.com/daniel-munoz/disc-fortune@latest`,
   without it, installs the latest v1 release — that is how Go module major
   versions work, not a mistake on your part.

   Or build from a clone:
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

# --format also matches a pressing's colour
disc-fortune --format "blue translucent"

# Combine filters
disc-fortune --year 1970-1980 --genre jazz

# Two pressings of one title can be identical in every field -- two
# store-exclusive colours, say. --release-id names one exactly, and
# needs no query beside it.
disc-fortune favorite --release-id 1839278

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
| `migrate` | Move your data to the `XDG_CONFIG_HOME` location. |
| `version` | Print the version. |
| `help` | Show help for a command. |

Run `disc-fortune help <command>` for details on any of them.

### Global flags

Every command accepts `--color`:

```sh
disc-fortune list --color=always | less -R   # keep color through a pipe
disc-fortune list --color=never              # no escape sequences, even on a terminal
```

`--color=auto` is the default: color when stdout is a terminal, none when it is
redirected. Under `auto`, a non-empty [`NO_COLOR`](https://no-color.org)
environment variable disables color. An explicit `--color=always` overrides
`NO_COLOR`, since that is you asking directly.

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
- **Crash-safe writes** - Every data file is written atomically, so an interrupted write cannot corrupt your collection or history
- **Resilient syncing** - Rate limits and server hiccups are retried with backoff, and long syncs report progress

## Data

Your data is stored locally in `$XDG_CONFIG_HOME/disc-fortune/`, falling back to
`~/.config/disc-fortune/` when `XDG_CONFIG_HOME` is unset:

- `collection.json` - Your full collection with metadata (Discogs release ID, artist, title, year, label, catalog number, genres, formats)
- `history.json` - Timestamped history of all your picks
- `favorites.json` - Albums you've marked as favorites
- `meta.json` - When you last synced

Every one of these is written atomically: disc-fortune writes a temporary file
alongside the target, flushes it, then renames it into place. An interrupted
write leaves the previous file untouched rather than truncated.

### If you already have `XDG_CONFIG_HOME` set

disc-fortune will keep reading your existing `~/.config/disc-fortune/` rather
than silently starting fresh somewhere else — upgrading never makes your
collection appear to vanish. It says so once, and you can move it when ready:

```sh
disc-fortune migrate
```

That copies each file to the new location and removes the originals. It refuses
to run if the destination already contains files.
