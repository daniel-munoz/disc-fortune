# Subcommand CLI Design

**Date:** 2026-08-24
**Status:** Approved

## Overview

Restructure the CLI from a flat flag namespace (`disc-fortune --list --favorites`) to
subcommands (`disc-fortune list --favorites`). Every mode the tool has today becomes a
verb; the modifiers that genuinely are modifiers stay flags.

This is a breaking change with no compatibility shims, released as **v2.0.0**. Running
`disc-fortune` with no arguments still picks a random album, so the tool's most common
invocation is unchanged.

## Goals

- One verb per mode, so the surface is discoverable via `disc-fortune help`
- Let the command structure enforce constraints that are currently hand-checked in `main()`
- Make argument parsing unit-testable, which it is not today
- Normalize exit codes onto a single rule
- No new dependencies; stdlib `flag` only

## Non-Goals

- Backwards-compatible flag aliases. v1 flags stop working; the migration table in the
  release notes is the upgrade path.
- Shell completion generation
- Interactive prompts of any kind (unchanged from v1)
- Restructuring the domain packages (`collection.go`, `discogs.go`, `filter.go`,
  `history.go`, `color.go`). `Filter` already carries `Query` and needs no change.

## CLI Surface

### Commands

| Command | Positional | Flags | Notes |
|-----------|-------------|--------------------------------|-----------------------------|
| `pick` | — | filters, `--favorites` | Implicit default |
| `list` | — | filters, `--favorites` | |
| `sync` | — | `--folder NAME` (repeatable) | Only syncs |
| `folders` | — | — | Was `--sync --list-folders` |
| `history` | `[N]` | — | Default 10, `0` = all |
| `favorite` | `[QUERY]` | filters | No query = last pick |
| `unfavorite` | `[QUERY]` | filters | No query = last pick |
| `version` | — | — | |
| `help` | `[COMMAND]` | — | Generated from the table |

Filters are `--year`, `--genre`, `--label`, `--format`, registered on `pick`, `list`,
`favorite`, and `unfavorite` by a single shared helper so they cannot drift apart.

### Migration from v1

| v1 | v2 |
|-----------------------------------|------------------------------------|
| `disc-fortune` | `disc-fortune` (or `pick`) |
| `--year 1975 --genre jazz` | `pick --year 1975 --genre jazz` |
| `--list` | `list` |
| `--list --favorites` | `list --favorites` |
| `--favorites` | `pick --favorites` |
| `--sync` | `sync` |
| `--sync --folder "Vinyl 12\""` | `sync --folder "Vinyl 12\""` |
| `--sync --list-folders` | `folders` |
| `--history 25` | `history 25` |
| `--favorite-last` | `favorite` |
| `--unfavorite-last` | `unfavorite` |
| `--favorite "kind of blue"` | `favorite "kind of blue"` |
| *(no equivalent)* | `unfavorite "kind of blue"` (new) |
| `--version` | `version` |

### Entry points and special cases

- Empty argv, or argv[1] beginning with `-`, dispatches to `pick`. This keeps
  `disc-fortune --year 1975` working and guarantees no flag can shadow a command name.
- An unrecognized first word is an error: `unknown command "foo"`, exit 1. It does *not*
  silently fall through to `pick`.
- `-h`, `--help`, `-help` dispatch to `help`. Too universal to drop.
- `--version` and `-version` produce a targeted error pointing at `disc-fortune version`,
  so v1 muscle memory gets a signpost rather than `flag provided but not defined`.

### Argument strictness

- More than one positional is an error, not a silent space-join:
  `favorite: too many arguments (quote the query: favorite "kind of blue")`
- An explicit empty query (`favorite ""`) keeps the v1 error: `favorite: requires a query`
- `history` requires an integer; `history abc` errors

### Guards

All four of v1's mutual-exclusion checks are deleted because the structure makes them
unrepresentable:

- `--list-folders requires --sync` — `folders` is its own command
- `--folder requires --sync` — `--folder` is only registered on `sync`
- `--favorite` with `--favorites` — `--favorites` is not registered on `favorite`, so
  `flag` rejects it unaided
- `--favorite` with `--favorite-last` — collapsed into one verb

One guard is added: filters with no query is meaningless, since "the last pick" cannot be
filtered. `favorite --year 1959` errors with `favorite: filters require a query`.

### Exit codes

Normalized onto one rule: **0 means the command produced what was asked for; 1 means it
could not.**

| Situation | v1 | v2 |
|-------------------------------------------|----|----|
| No collection file / collection empty | 0 | 1 |
| No favorites yet (`--favorites`) | 0 | 1 |
| `pick` — no albums match | 0 | 1 |
| `list` — no albums match | 0 | 1 |
| `favorite` — no match | 1 | 1 |
| `unfavorite` — no match | 1 | 0 |
| `favorite`/`unfavorite` — multiple matches | 1 | 1 |
| Already in favorites / not in favorites | 0 | 0 |
| Usage error | 1 | 1 |
| Operational failure | 1 | 1 |

The already-favorited and not-in-favorites cases stay 0 deliberately: the requested end
state holds, so they are idempotent successes rather than failures.

`unfavorite` with no match is 0 for the same reason, and this is a change from v1. Removal
is idempotent in a way addition is not: `favorite "query"` must resolve one specific album
to add, so no match means it cannot do its job, while `unfavorite`'s job is to ensure an
album is not favorited — a no-match means that already holds. This matches `rm -f`, HTTP
`DELETE`, and `kubectl delete --ignore-not-found`. It also makes the query form agree with
the bare form, which already exits 0 when the last pick was never favorited.

The tradeoff, accepted knowingly: because `unfavorite` searches only the favorites list, a
typo is indistinguishable from an already-removed album and now exits 0 silently.
Distinguishing them would require a fallback search of the collection, which adds a load on
the miss path and behaves incoherently when no collection file exists. The message carries
the signal instead: `No favorites match "QUERY" - nothing to remove.`

Multiple matches remain 1 for both verbs: the command cannot choose on the user's behalf.

`list` returning 1 on an empty result follows `grep`'s precedent. The tradeoff is that
`disc-fortune list --genre jazz` under `set -e` halts when nothing matches.

## Implementation

### File layout

`main.go` is currently 400 lines holding dispatch, every `run*`, `formatList`, and the
Discogs sync helpers. Since the dispatch half is being rewritten, it is split three ways:

- **`cli.go`** *(new)* — `command` struct and table, `dispatch`, `parseInterspersed`,
  `addFilterFlags`, `runHelp`, `runVersion`
- **`sync.go`** *(new)* — `runSync`, `runFolders`, `printFolders`, `resolveFolderIDs`,
  `collectAlbums`, `resolveFolderNames`, and `arrayFlags` (only `--folder` uses it)
- **`main.go`** — `main()`, `fatal`, `formatList`, and the `run*` orchestration for
  pick/list/history/favorite/unfavorite. Roughly 230 lines.

`runFortune` is renamed `runPick` to match its verb. No other `run*` signature changes
beyond the parse/execute split below.

### The command table

```go
type command struct {
    name    string
    summary string // one line, listed by `help`
    usage   string // full block, shown by `help <cmd>` and on usage error
    run     func(args []string)
}
```

`help` and `help <cmd>` are both generated from this table, so a command cannot ship
undocumented. Each command's `FlagSet` points its `Usage` at its own entry, so a bad flag
prints that command's usage rather than a global dump.

### `main()`

```
args := os.Args[1:]
handle -h/--help and --version special cases
if len(args) == 0 || strings.HasPrefix(args[0], "-") { prepend "pick" }
look up args[0]; unknown -> error, exit 1
cmd.run(args[1:])
```

### Interspersed flags

Go's `flag` package stops parsing at the first non-flag argument. Without handling,
`favorite "miles" --year 1959` — an example from the current README — would parse the
query, silently ignore `--year 1959`, and favorite the wrong album with no error.

`parseInterspersed` loops parse / peel one positional / parse the rest:

```go
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
    var positional []string
    for {
        if err := fs.Parse(args); err != nil {
            return nil, err
        }
        rest := fs.Args()
        if len(rest) == 0 {
            break
        }
        positional = append(positional, rest[0])
        args = rest[1:]
    }
    return positional, nil
}
```

Every command parses through it, so flag position never matters anywhere in the CLI.

A `--` terminator still works: `flag` stops at it and returns the remainder as positional
arguments, so a query beginning with a dash stays reachable as `favorite -- "-live-"`.

### Parse/execute split

Every `run*` today prints and calls `os.Exit`, which is why `main()`'s dispatch has never
had a test. Each command splits into a pure parse function and an executing one:

```go
func parsePick(args []string) (pickConfig, error) // pure: no I/O, no exit
func runPick(cfg pickConfig)                      // does the work
```

The table's `run` closure is glue: parse, `fatal` on error, execute. Every parsing rule in
this design becomes testable over pure functions with no filesystem or network.

### `unfavoriteByQuery`

`favorite "query"` searches the collection, because it is pulling an album in.
`unfavorite "query"` searches the **favorites list** instead: that is the set being removed
from, it is far smaller, and querying the collection would allow "unfavoriting" an album
that was never favorited, producing a confusing not-in-favorites error instead of a clean
no-match.

It mirrors the existing seam in `favorites.go`:

```go
func unfavoriteByQuery(favorites []Album, query string, filter Filter, favPath string) (UnfavoriteOutcome, error)
```

with `Status` one of `UnfavoriteRemoved`, `UnfavoriteNoMatch`, or `UnfavoriteMultiMatch`,
reusing `removeFavorite` and `ErrNotInFavorites` underneath. Multi-match reuses
`formatList` and the `Be more specific or add filters.` wording from `favoriteByQuery`.

### User-facing strings and the duplicated load blocks

Eight printed strings instruct the user to run v1 flags and are wrong the moment this ships:

| Location | v1 text | v2 text |
|-------------------------|--------------------------------------|-------------------------------------|
| 4 sites | ``Run `disc-fortune --sync` `` | ``Run `disc-fortune sync` `` |
| 2 sites | `Use --favorite-last after a pick` | ``Use `disc-fortune favorite` `` |
| `--list-folders` help | "use with --sync" | (flag deleted) |
| `--folder` help | "use with --sync" | (dropped; implied by `sync --folder`) |

They repeat because the load-collection-or-explain block is copy-pasted three times across
`runList`, `runFortune`, and `runFavorite`, and the load-favorites block twice. That
duplication is also exactly where the exit codes change, so leaving it in place would mean
making the same edit five times and risking drift.

Two helpers collapse it, and are the single place the new exit-code rule is applied:

```go
func loadCollectionOrExit() []Album // missing/empty -> message + exit 1
func loadFavoritesOrExit() []Album  // empty -> message + exit 1
```

This is a targeted cleanup of code the change already rewrites, not general refactoring.

### Error message conventions

- Usage and validation errors: `<command>: <message>` (e.g. `favorite: requires a query`)
- Operational failures: today's `Error: <message>` (e.g. `Error loading collection: ...`)

`FlagSet`s use `flag.ContinueOnError`, not `ExitOnError`, so usage errors exit 1 like every
other error rather than `flag`'s default 2.

## Testing

### `cli_test.go` (new)

- `parseInterspersed` with flags before, after, and surrounding the positional — the
  `favorite "miles" --year 1959` case gets an explicit regression test, since silently
  dropped filters is the failure mode this helper exists to prevent
- Dispatch: implicit `pick` on empty argv and on a leading `-`; unknown command; `-h`;
  the `--version` signpost
- Per-command parse functions: valid forms, `too many arguments`, `favorite --year` with
  no query, `history abc`, unknown flags
- Every table entry has a non-empty `summary` and `usage`, so `help` cannot go stale

### `favorites_test.go`

`unfavoriteByQuery` gets the four cases the existing `favoriteByQuery` tests cover: single
match, no match, multiple matches, and album not in favorites.

### `sync_test.go` (new)

`TestResolveFolderNames`, `TestResolveFolderNamesMultiple`, `TestResolveFolderNamesNotFound`,
and `TestCollectAlbumsDeduplicates` relocate here from `main_test.go` to mirror the source
split. Their bodies are unchanged.

### `main_test.go`

Retains the `formatList` tests unchanged.

No existing test changes behavior; the moves are relocations only.

## Documentation

- **`README.md`** — Usage section restructured by command; all 23 examples rewritten
- **`RELEASE_NOTES_v2.0.0.md`** *(new)* — the migration table above plus the exit-code
  changes, since both break existing scripts
- **`version`** const bumped to `2.0.0`
- **`docs/plans/`** and `RELEASE_NOTES_v1.*.md` left untouched — historical records

## Risk and reversibility

Moderate risk, entirely concentrated in the CLI layer:

- Every v1 invocation breaks by design. Mitigated by the migration table and the
  `--version` signpost, and bounded by the major version bump.
- The domain layer is untouched, so collection, favorites, and history file formats are
  unchanged. A user can downgrade to v1 without touching their data.
- The interspersed-flag helper is the highest-risk new code, because its failure mode is
  silent rather than loud. It has a dedicated regression test.
- Exit-code normalization can break scripts that check status. Called out in the release
  notes.
- Reversible by reverting the branch; no migrations, no persisted state changes.

## Open questions

None.
