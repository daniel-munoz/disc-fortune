# v2.5.0 "Insight" — design

**Date:** 2026-09-04
**Status:** Approved in brainstorming; ready for an implementation plan.
**Covers:** T9 (`stats`), T10 (`open`), and the two items that close out
`docs/plans/2026-08-26-roadmap.md`.

This is the last phase of the post-v2.0.0 roadmap. Phase 5 was the tersest entry
on it — four words for T9 and one line for T10 — so everything below is decided
here rather than in the roadmap.

---

## Scope

1. **T9 `stats`** — a local, read-only summary of whatever set of albums the
   filter flags describe.
2. **T10 `open`** — open a record's Discogs page in a browser.
3. **Correct a stale roadmap note.** Phase 2 carries a "known limitation" that
   `sync` can lose a concurrent `pick`'s history entry. T5 closed it in v2.3.0.
4. **Add the module-major-version guard** left pending after the v2.1.1 `/v2`
   fix. Nothing currently fails when `go.mod`'s major and `main.go`'s `version`
   disagree.

Out of scope: anything that adds a write path, a network call, or a new data
file. Both commands read files that already exist.

---

## T9 — `stats`

### Command surface

```
Usage: disc-fortune stats [flags]

Flags:
  --favorites      Describe favorites only
  --json           Emit machine-readable JSON instead of text
<the shared filter flags>
```

`needsConfig: true`.

**Filters yes, `--unheard` no.** `stats` takes the same filter surface as
`list` — including `--query`, which is a `filterFields` entry and therefore
free. It does **not** take `--unheard`: that flag is defined *by* history, and
"share of the set ever picked" over an unheard-only set is 0% by construction.
A flag whose only effect is to make one of the headline figures meaningless
does not belong on the command.

It does not take `--draw` either. `--draw` is a draw strategy and `stats` draws
nothing. (Phase 3 settled that `--unheard` is a filter and `--draw` is a
strategy; `stats` declines one of each, for different reasons.)

**Filters may stand alone.** `favorite`, `unfavorite` and `open` refuse
narrowing filters with no query, because they act on exactly one record and a
filter alone does not say which. `stats` is set-oriented, like `list` and
`pick`: `disc-fortune stats --genre jazz` is a complete request. Do not reuse
the `anyNarrowing() && !identifies()` check here.

**Registration.** A new `addStatsFlags(fs *flag.FlagSet) *statsFlags`
registering `--favorites` and `--json` and delegating to `addFilterFlags`.
Per the v2.4.0 decision, flag registration lives in an `add*Flags` function so
`completion` enumerates the same `FlagSet` the command parses with — a flag
cannot be accepted without also being completable. `filterFlagHelp` is
appended to the usage block, which `TestFilterFlagsAreDocumented` enforces.

### Shape

New `stats.go`:

```go
// computeStats is pure: it takes everything it needs and touches no files.
func computeStats(pool, favorites []Album, entries []HistoryEntry,
                  total int, m Meta, now time.Time) Stats

func formatStats(s Stats, useColor bool) string
```

and in `json.go`:

```go
func newStatsPayload(s Stats) statsPayload
```

**Both views render from one `Stats` value.** This is the point of the split.
`formatHistory` and `newHistoryPayload` deliberately duplicate their clamp and
reverse, with a test asserting they agree; that was the right call there
because the two were written against `[]HistoryEntry` directly. Here there is a
computed intermediate, so text and JSON cannot disagree about a figure by
construction — no duplication, and no test needed to keep them honest.

`computeStats` taking `total` separately is what lets the header say "312 of
1247": the pool has already been filtered by the time it arrives.

### The figures

**Header.**

- Line 1: `<pool> albums`, becoming `<pool> of <total> albums` when filters
  narrowed the set, then `· <k> favorites` where *k* counts how many of the
  pool are favorited. Under `--favorites` the pool *is* favorites, so it reads
  `84 favorites` and the tail is dropped as redundant.
- Line 2: `last synced <relative>`, from `meta.json` through the existing
  `formatTimestamp`. Omitted entirely when nothing has ever been synced —
  `staleNotice` already takes that position, and nagging a user who has never
  synced is noise.

Counting favorites within the pool goes through `containsAlbum`, i.e.
`sameAlbum`. Loading favorites must tolerate `errNoFavorites` as zero rather
than failing: `stats` is read-only and having no favorites is not an error.

**Decades.** Bucket on `Year/10*10`. `Year == 0` becomes an `unknown` row,
listed last and only when non-empty. Buckets run contiguously from the lowest
to the highest decade present, so a decade you own nothing from shows as a zero
row — in a histogram, a gap is information.

Bars are Unicode eighth-blocks (`U+2588` and friends), scaled so the largest
bucket fills a fixed 24 columns. Fixed rather than terminal-derived: the output
has to be deterministic to be golden-tested, and a width that changes with the
terminal is not.

**Top genres and top labels.** Five each. An album counts once per distinct
genre it lists; `Album.Label` is a single string, so an album contributes at
most one label. Sorted by count descending, then name ascending — the tiebreak
is not cosmetic, it is what makes a golden test possible at all. A section with
nothing in it is omitted from the text output but still emits `[]` in JSON.

**Share ever picked.**

```
312 of 1247 albums picked at least once (25%)
  last picked 2 hours ago
```

Resolved through `lastPlayedIndex`, per the Phase 3 constraint: **not** a map.
`sameAlbum` is not transitive when an entry has no release ID, and a map key
would silently assume it was. The cost is O(pool × history) `sameAlbum` calls;
at realistic collection and history sizes that is sub-millisecond, and keeping
the semantics identical to picking's is worth more than the constant factor.
If it ever does become a problem, the fix is to retire un-ID'd entries, not to
index them.

The second line reports the most recent history entry whose album is *in the
pool*, so `stats --genre jazz` reports when you last played jazz. Omitted when
nothing in the pool was ever picked.

The percentage is rounded to a whole number for display. The JSON emits the
unrounded fraction.

### Behavior on an empty match

Same as `list`: message on stderr, exit 1. `--json` changes the format, never
the semantics, so `stats --json` on an empty match also exits 1 with the
message on stderr and nothing on stdout — exactly as `list --json` does.

The two "nothing to work with" states keep their existing messages through
`loadCollectionOrExit` / `loadFavoritesOrExit`.

### Colour

Section headings bold, bars dim, nothing else. `--json` is never coloured: an
ANSI escape inside a JSON string is a parse hazard for no benefit, which is
already the rule `writeJSON` documents.

### JSON schema

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

- `count` is the described set, after filters. `total` is the source set
  before them — the collection, or favorites under `--favorites`. When no
  filter is set they are equal.
- `favorites` is how many of the described set are favorited. Under
  `--favorites` the described set *is* favorites, so it equals `count`; the
  text view drops that half of the header rather than repeat itself, but the
  key stays present in JSON because every key always is.
- `synced_at` is `null` when nothing has ever been synced.
- `"decade": null` means unknown year, matching `jsonAlbum`'s convention that
  null says "Discogs did not tell us" — something `0` cannot, since year 0
  sorts before 1959.
- `picked.count` and `share` are measured against `count`, the described set —
  not against `total`. `stats --genre jazz` reports the share of your *jazz*
  ever picked. `share` is the unrounded fraction; the text view rounds, and a
  consumer should not have to un-round.
- `last_picked` is `null` when nothing in the set was ever picked.
- `picked` is an object rather than three flat keys so a later figure about
  picking has somewhere to go.
- Every key is always present, and `decades`/`genres`/`labels` are `[]` rather
  than `null` when empty, so a consumer's loop needs no nil check. This is the
  existing `listOrEmpty` rule.

The payload is an object, not an array, for the reason `json.go` already
records: a key can be added to an object without breaking a consumer, and a
top-level array can never become an object.

---

## T10 — `open`

### Command surface

```
Usage: disc-fortune open [QUERY] [flags]

Flags:
  --print          Print the URL instead of opening a browser
<the shared filter flags>
```

`needsConfig: true`.

### Selection

Identical grammar to `favorite`, deliberately:

- No query and no `--release-id` → the last pick from history.
- A query (positional or `--query`) or `--release-id` → resolve against the
  **collection**, not favorites.
- Exactly one match → open it.
- No match → exit 1, `No albums match <describe()>`.
- Several matches → list them with their release IDs, `Be more specific, add
  filters, or use --release-id.` on stderr, exit 1.
- Narrowing filters with no query and no `--release-id` → `filters require a
  query`, as on `favorite`.
- Giving both a positional query and `--query` → refused, as on `favorite`.

`open` takes the filter flags but not `--favorites`, matching `favorite` and
`unfavorite`. `--release-id` remains the one identifying filter and the one
that excuses a missing query.

**Shared parsing.** `parseFavorite`'s body — the too-many-arguments check, the
double-query refusal, the filters-require-a-query rule — is extracted into a
helper that takes an already-built `FlagSet` and `filterFlags`. `parseFavorite`
and a new `parseOpen` each build their own flag set (so `open` can register
`--print` through an `addOpenFlags`) and then call it. Copying forty lines to
get a third command with the same grammar is how the grammar drifts.

**Shared matching.** `favoriteByQuery` and `unfavoriteByQuery` both apply the
filter and switch on `len(matches)`. `open` is the third site of the same
three-way outcome, so that classification moves into `filter.go`:

```go
type matchStatus int
const (matchedOne matchStatus = iota; matchedNone; matchedMany)
func matchAlbums(albums []Album, filter Filter) (Album, []Album, matchStatus)
```

All three commands adopt it. The write in each `case 1` branch stays where it
is; only the classification is shared. This is a mechanical extraction covered
by the existing `favorites_test.go`.

### URL

`https://www.discogs.com/release/%d`, from the resolved album's `ReleaseID`.

**No release ID → exit 1**, with:

```
This pick predates release IDs and sync could not identify it.
Run `disc-fortune sync`, or name the record with --release-id.
```

Only reachable for a pre-v2.2 entry that the backfill deliberately left alone —
ambiguous, or sold out of the collection. Backfill refuses to guess which
pressing an ambiguous entry meant; opening a release page would be guessing the
same thing, and a wrong record silently opened is worse than an error. This
also follows the v2 exit-code rule.

A search-URL fallback was considered and rejected: it gives `open --print` an
output shape a script cannot assume is a release URL.

### Launching

New `open.go`. The platform decision is isolated in a pure function:

```go
func browserCommand(goos string) (name string, args []string, ok bool)
```

- `darwin` → `open <url>`
- `linux`, `freebsd`, `openbsd`, `netbsd` → `xdg-open <url>`
- `windows` → `rundll32 url.dll,FileProtocolHandler <url>`
- anything else → `ok == false`

Taking `goos` as a parameter rather than reading `runtime.GOOS` is what makes
all three branches testable from any host. A `runtime.GOOS` switch would only
ever exercise one of them in CI.

**`cmd.Start()`, and no wait.** `xdg-open` blocks until the browser exits on
some desktops, and hanging the user's terminal is a worse failure than missing
a launcher's exit code. The consequence, accepted: a launcher that starts and
*then* fails is invisible to us. That is tolerable because the browser window
is the user's own feedback. Only `Start()` errors are reported.

On success, `open` prints nothing.

### Falling back to printing

`open` prints the URL to stdout and a one-line note to stderr, **exiting 0**,
when either:

- the launcher is not on `PATH` (`exec.LookPath`), or
- on the X11-ish unixes only, neither `DISPLAY` nor `WAYLAND_DISPLAY` is set.

Neither check applies on darwin or windows, which have no `DISPLAY`.

```
$ disc-fortune open
https://www.discogs.com/release/1234567
disc-fortune: no browser launcher found; printed the URL instead.
```

Exit 0 because this is a graceful degradation, not a failure: a script on a
headless box gets a usable URL on the data channel, which is what it wanted.
The note is on stderr precisely so stdout stays a clean URL.

`--print` takes the same stdout path but never consults the launcher or the
environment at all, so it behaves identically everywhere.

### No `--json` on `open`

`--print` already covers every scripting case. `open --json` would have to both
emit a payload *and* launch a browser to honour the "format never semantics"
rule — true to the letter and useless in practice.

---

## Closing the roadmap

### The stale Phase 2 note

`docs/plans/2026-08-26-roadmap.md`, Phase 2, currently reads:

> **Known limitation carried forward:** `sync` now rewrites `history.json`, so
> a concurrent `pick` can lose its entry. T5 makes history load-bearing for
> picking, so this is worth closing then.

T5 closed it. `lock.go:14` names that exact scenario in its doc comment, and
`runBackfill` wraps both rewrites in `withFileLock` (`backfill.go:144,165`).
Update the note to record that, and mark Phase 5 shipped once this lands.

### The module-major-version guard

`go.mod` declares `github.com/daniel-munoz/disc-fortune/v2`; `main.go:11`
declares `version = "2.4.0"`. Nothing fails when those disagree, which is the
guard left pending after the v2.1.1 `/v2` fix.

New `version_test.go`: read `go.mod`, extract the `/vN` major suffix (its
absence means v0/v1), parse the major out of `version`, fail when they differ.

A test rather than a CI step, following this repo's own forcing-function
pattern (`TestFilterFlagsAreDocumented`,
`TestEveryAlbumFieldHasAWireDecision`). It runs inside the existing
`go test ./...`, so `.github/workflows/go.yml` needs no edit, and it fails
locally before a release rather than only on a pull request.

---

## Files

| File | Change |
|---|---|
| `stats.go` | new — `Stats`, `computeStats`, `formatStats` |
| `stats_test.go` | new |
| `open.go` | new — `browserCommand`, URL building, launch and fallback |
| `open_test.go` | new |
| `version_test.go` | new — the major-version guard |
| `json.go` | `statsPayload`, `newStatsPayload` |
| `json_test.go` | golden test for the stats payload |
| `cli.go` | `stats` and `open` commands; `addStatsFlags`, `addOpenFlags`, `parseStats`, `parseOpen`; `parseFavorite` body extracted |
| `filter.go` | `matchAlbums` |
| `favorites.go` | `favoriteByQuery` / `unfavoriteByQuery` adopt `matchAlbums` |
| `main.go` | `runStats`, `runOpen`; `version` to `2.5.0` |
| `README.md` | both commands, the stats schema, exit-code notes |
| `RELEASE_NOTES_v2.5.0.md` | new |
| `docs/plans/2026-08-26-roadmap.md` | correct the Phase 2 note; mark Phase 5 shipped |

---

## Testing

**`stats`**

- `computeStats` unit tests: decade bucketing including `Year == 0` and a
  contiguous gap decade; genre counting when one album lists several; label
  counting; the favorites-within-pool count; share-ever-picked with an un-ID'd
  history entry (which is a name wildcard and must count every same-named
  pressing as picked).
- `formatStats` golden tests, coloured and plain, including the `--favorites`
  header variant and the filtered `<n> of <total>` variant.
- Bar scaling: largest bucket fills the full width; a single-bucket histogram;
  a bucket of zero renders an empty bar without panicking.
- Tiebreak determinism: two genres with equal counts always sort the same way.
- `stats --json` golden test.
- Empty match exits 1 with the message on stderr, in both formats.

**`open`**

- `browserCommand` for each GOOS, including an unknown one.
- URL building from a release ID.
- Selection: last pick, one match, no match, several matches, a `--release-id`
  with no query, narrowing filters with no query, both spellings of the query
  at once. These mirror the existing `favorite` cases.
- No release ID exits 1 with the actionable message.
- `--print` emits the URL on stdout and launches nothing.
- The fallback path prints the URL on stdout, the note on stderr, and exits 0.

**Shared**

- `matchAlbums` extraction: the existing `favorites_test.go` cases must pass
  unchanged.
- `completion` already asserts every command's flags are enumerable; both new
  commands must appear without a special case.
- The major-version guard test itself.
