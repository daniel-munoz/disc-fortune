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
```

### Filtering

Filters follow one rule: **values within a field OR together, different fields
AND together, and any `--exclude-` match removes the record outright.**

```sh
# Filter by year, decade, or range
disc-fortune --year 1975
disc-fortune --year 1970-1980
disc-fortune --decade 70s

# Filter by genre, label, or format
disc-fortune --genre jazz
disc-fortune --label blue-note
disc-fortune --format 12\"

# --format also matches a pressing's colour
disc-fortune --format "blue translucent"

# Search the whole "Artist - Title" line, or one field of it
disc-fortune --query "kind of blue"
disc-fortune --artist "miles davis"
disc-fortune --title "kind of blue"

# Repeat a flag for "either" -- these are the same records, in one command
disc-fortune --genre jazz --genre funk

# Every narrowing filter has an --exclude- twin (--release-id has none)
disc-fortune --exclude-genre rock
disc-fortune list --decade 70s --exclude-label "blue note"

# Different fields narrow each other
disc-fortune --year 1970-1980 --genre jazz
```

`--year` and `--decade` are two spellings of one field, so they widen rather
than narrow each other: `--year 1959 --decade 70s` gives you 1959 *or* the
seventies.

An exclusion only removes records that actually match it. Discogs leaves the
year or label blank on plenty of releases, and those records survive
`--exclude-year 1975` and `--exclude-label x` rather than quietly disappearing.

Two-digit decades from `30s` to `90s` mean the twentieth century. `--decade
20s` is refused, because it could mean either the 1920s or the 2020s — write
whichever you meant in full.

`--decade` also accepts any year, not just one already aligned to a decade:
`--decade 1975` and `--decade 75s` both mean the 1970s, the same range as
`--decade 1970s`. If you meant one specific year, use `--year` instead —
`--decade` always widens to the full ten years.

```sh
# Two pressings of one title can be identical in every field -- two
# store-exclusive colours, say. --release-id names one exactly, and
# needs no query beside it.
disc-fortune favorite --release-id 1839278

# Pick randomly from favorites only
disc-fortune pick --favorites

# Pick something you have never picked before
disc-fortune pick --unheard

# Restore the old, history-blind random draw
disc-fortune pick --draw any
```

### JSON output

`pick`, `list`, `history` and `stats` accept `--json`, which replaces the
human output with a documented payload. Nothing else changes: exit codes, the
messages on stderr, and `pick` recording its pick are all identical either way.

```sh
disc-fortune pick --json
disc-fortune list --json --genre jazz
disc-fortune history --json 5
```

Each command emits a single JSON object:

```json
{
  "album": {
    "release_id": 1839278,
    "artist": "Miles Davis",
    "title": "Kind of Blue",
    "year": 1959,
    "label": "Columbia",
    "catno": "CL 1355",
    "genres": ["Jazz"],
    "formats": ["Vinyl", "LP", "Album"]
  }
}
```

`list` emits `{"albums": [...], "count": N}` and `history` emits
`{"entries": [{"album": {...}, "timestamp": "..."}], "count": N}`, most recent
first. `count` is how many records were emitted, so `history --json 5` reports
at most `5` — fewer if your history is shorter.

Every album key is always present. `release_id`, `year`, `label` and `catno`
are `null` when Discogs did not say — `release_id` is also `null` for anything
picked before v2.2.0 — while `genres` and `formats` are `[]` rather than null,
so a loop over them needs no guard. `artist` and `title` are always strings.

Timestamps are RFC 3339 with your local UTC offset (not necessarily `Z`), and
fractional seconds are variable-length, so parse them with a real RFC 3339
parser rather than a fixed format string.

Exit codes are unchanged, which means a script should check them: `list --json`
matching nothing writes its message to stderr and exits 1 with an empty stdout,
rather than emitting an empty result.

```sh
if out=$(disc-fortune list --json --genre jazz); then
  echo "$out" | jq -r '.albums[] | "\(.artist) - \(.title)"'
fi
```

`stats --json` emits:

```json
{
  "count": 312,
  "total": 1247,
  "favorites": 28,
  "synced_at": "2026-09-01T10:00:00Z",
  "decades": [
    {"decade": 1970, "count": 486},
    {"decade": null, "count": 22}
  ],
  "genres": [{"name": "Jazz", "count": 412}],
  "labels": [{"name": "Blue Note", "count": 88}],
  "picked": {
    "count": 78,
    "share": 0.25,
    "last_picked": "2026-09-04T18:00:00Z"
  }
}
```

- `count` — albums described, after filters
- `total` — the source set before filters (the collection, or favorites under `--favorites`)
- `favorites` — how many of the described set are favorited
- `synced_at` — RFC 3339, or `null` if never synced
- `decades` — `{"decade": 1970, "count": 486}` rows; `"decade": null` is the unknown-year bucket
- `genres`, `labels` — `{"name": ..., "count": ...}` rows, at most five each
- `picked` — `count`, `share` (an unrounded fraction of `count`, not of `total`), and `last_picked` or `null`

Every key is always present, and the three arrays are `[]` rather than `null`
when empty.

### Shell completion

`completion` prints a script for bash, zsh or fish. It is generated from the
commands and flags this binary actually accepts, so it cannot drift from them.

```sh
# try it for the current shell
eval "$(disc-fortune completion bash)"
eval "$(disc-fortune completion zsh)"
disc-fortune completion fish | source
```

To keep it, add that line to your shell's startup file, or write the script
into the directory your shell reads completions from.

Command names, flag names, and the fixed values of `--draw` and `--color` are
completed. Flags are scoped to the command that takes them, so `list --<TAB>`
offers `--json` but not `--draw`, which `list` rejects.

Values that would have to be read from your collection — those of `--genre`
and `--label` — are deliberately not completed. A tab-press should never depend
on a file that a `sync` may be rewriting.

### Commands

| Command | What it does |
|---|---|
| `pick` | Print a random album. Runs by default when you give no command. |
| `list` | List every matching album, with a count. |
| `sync` | Fetch your collection from Discogs. |
| `folders` | List your Discogs folder names. |
| `history` | Show recent picks. |
| `stats` | Summarize your collection, or whatever a filter describes. |
| `favorite` | Add an album to favorites. |
| `unfavorite` | Remove an album from favorites. |
| `open` | Open a record's Discogs page in a browser. |
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

### Discovery

By default, `pick` avoids the records you played most recently, so the same
album does not come back around twice in a week:

```sh
disc-fortune
```

The exclusion window is your last `min(10, pool/3)` *distinct* picks,
measured against whatever pool your filters leave — so `--genre jazz`
matching four albums uses a window of one, not ten. A pool of one or two
excludes nothing. That is what keeps a narrow filter from ever being
narrowed into an empty set.

This applies to `--favorites` too: it is still a hard filter — a
non-favorite is never reachable, full stop — but the pool it produces is
subject to the same default draw as any other filter's pool. So
`disc-fortune pick --favorites` now avoids the favorites you played most
recently. Add `--draw any` to get the exact old behavior, a uniform draw
across your favorites with history ignored:

```sh
disc-fortune pick --favorites --draw any
```

`--draw` controls how a pick is drawn:

```sh
disc-fortune pick --draw fresh   # default: skip the recently played
disc-fortune pick --draw any     # uniform draw, history ignored entirely
disc-fortune pick --draw stale   # skip the recently played, then favor
                                 # whatever you have left unplayed longest
```

`--unheard` restricts `pick` and `list` to albums that have never appeared
in your history:

```sh
disc-fortune pick --unheard
disc-fortune list --unheard --genre jazz
```

If everything matching your other filters has already been played,
`pick --unheard` exits 1 and says so rather than picking a repeat.

### Statistics

```bash
# Summarize the whole collection
disc-fortune stats

# Summarize whatever a filter describes
disc-fortune stats --genre jazz
disc-fortune stats --decade 70s

# Summarize your favorites
disc-fortune stats --favorites
```

`stats` reads only files already on disk — it never contacts Discogs. It
reports a decade histogram, your five most common genres and labels, and how
much of the set you have ever played.

The share-ever-played figure is measured against the set being described, so
`stats --genre jazz` reports the share of your *jazz*, not of your collection.

`stats` takes the filter flags but not `--unheard`: that flag is defined by
history, and the share of an unheard-only set is always zero.

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

# The query can also be given as --query -- the two spellings are
# equivalent, and giving both at once is refused rather than guessed at
disc-fortune favorite --query "kind of blue"
disc-fortune favorite "kind of blue" --query coltrane   # error: give the query once

# Narrow an ambiguous query with filters
disc-fortune favorite "miles" --year 1959
disc-fortune favorite "coltrane" --genre jazz

# Remove the last pick from favorites
disc-fortune unfavorite

# Remove a specific album from favorites
disc-fortune unfavorite "kind of blue"
```

`disc-fortune pick --favorites` stays a hard filter — see
[Discovery](#discovery) for how the default anti-repeat draw applies to the
pool it produces, and how to turn that off with `--draw any`.

### Opening a release on Discogs

```bash
# Open the last pick
disc-fortune open

# Open a specific record
disc-fortune open "kind of blue"
disc-fortune open --release-id 1839278

# Print the URL instead of opening anything
disc-fortune open --print
```

`open` uses the same grammar as `favorite`: no query means the last pick, and
an ambiguous query lists the candidates with their release IDs rather than
guessing.

With nothing to launch into — no `xdg-open` on `PATH`, or no display — the URL
is printed instead and the command still succeeds, so it stays useful over SSH
and in scripts.

A record with no release ID cannot be opened. That only happens for a pick
recorded before v2.2.0 that `sync` could not identify; run `disc-fortune sync`,
or name the record with `--release-id`.

### Exit codes

`disc-fortune` exits 0 when the command produced what you asked for, and 1 when
it could not — no collection synced yet, no albums matching your filters, an
ambiguous `favorite`, `unfavorite` or `open` query, a record with no release ID
for `open` to point at, or a usage error. Removing a favorite that is not there
exits 0, since the end state you asked for already holds. `history` on an empty
log also exits 0: a log report succeeds even when the log has nothing in it.

`open` exits 0 whether it launched a browser or fell back to printing the URL.
Printing is a degradation, not a failure: you still got the address you asked
for. A launcher that exists but fails to start is a real failure, though: `open`
prints the URL anyway, so you are not left with nothing, but exits 1.

## Features

- **Metadata-rich display** - Shows year, label, catalog number, and genres with color-coded output
- **Flexible filtering** - Filter by query, artist, title, year, decade, genre, label, or format; repeat any flag to mean "either", and exclude with `--exclude-genre` and friends
- **Pick history** - Track all your picks with timestamps
- **Favorites** - Mark albums you love (by last pick or by query) and pick randomly from that subset
- **List mode** - Browse all albums (or filtered subsets) without picking one
- **Offline operation** - All data stored locally after initial sync
- **Crash-safe writes** - Every data file is written atomically, so an interrupted write cannot corrupt your collection or history
- **Resilient syncing** - Rate limits and server hiccups are retried with backoff, and long syncs report progress
- **Scriptable** - `--json` on `pick`, `list`, `history` and `stats` emits a documented payload with a fixed key set
- **Shell completion** - `completion bash|zsh|fish` generates a script from the commands and flags the binary accepts, so it cannot drift from them
- **Collection statistics** - `stats` reports a decade histogram, your most common genres and labels, and how much of any filtered set you have ever played
- **Straight to Discogs** - `open` takes the last pick, or any record you can name, to its Discogs page

## Data

Your data is stored locally in `$XDG_CONFIG_HOME/disc-fortune/`, falling back to
`~/.config/disc-fortune/` when `XDG_CONFIG_HOME` is unset:

- `collection.json` - Your full collection with metadata (Discogs release ID, artist, title, year, label, catalog number, genres, formats)
- `history.json` - Timestamped history of all your picks
- `favorites.json` - Albums you've marked as favorites
- `meta.json` - When you last synced

You may also see `history.json.lock` and `favorites.json.lock` alongside
them. These are advisory-lock sidecars that keep a `sync` and a concurrent
`pick` or `favorite` from clobbering each other's write; they hold no data of
yours, are safe to leave alone, and `migrate` neither copies them to the new
location nor leaves them behind in the old one.

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
