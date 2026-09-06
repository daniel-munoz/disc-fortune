# disc-fortune v2.0.0

**Breaking release.** disc-fortune now uses subcommands instead of a flat set of
flags. Running `disc-fortune` with no arguments still prints a random album, so
the most common invocation is unchanged, but every other v1 flag has moved.

## Migration

| v1 | v2 |
|---|---|
| `disc-fortune` | `disc-fortune` (or `disc-fortune pick`) |
| `disc-fortune --year 1975 --genre jazz` | `disc-fortune --year 1975 --genre jazz` (unchanged) |
| `disc-fortune --list` | `disc-fortune list` |
| `disc-fortune --list --favorites` | `disc-fortune list --favorites` |
| `disc-fortune --favorites` | `disc-fortune --favorites` (unchanged) |
| `disc-fortune --sync` | `disc-fortune sync` |
| `disc-fortune --sync --folder "Vinyl 12\""` | `disc-fortune sync --folder "Vinyl 12\""` |
| `disc-fortune --sync --list-folders` | `disc-fortune folders` |
| `disc-fortune --history 25` | `disc-fortune history 25` |
| `disc-fortune --favorite-last` | `disc-fortune favorite` |
| `disc-fortune --unfavorite-last` | `disc-fortune unfavorite` |
| `disc-fortune --favorite "kind of blue"` | `disc-fortune favorite "kind of blue"` |
| `disc-fortune --version` | `disc-fortune version` |

Filter flags (`--year`, `--genre`, `--label`, `--format`) keep their names and
now work on `pick`, `list`, `favorite`, and `unfavorite`.

## New

- `disc-fortune help` and `disc-fortune help <command>`, generated from the
  command table
- `disc-fortune folders`, promoted from `--sync --list-folders` — it never
  synced anything anyway
- `disc-fortune unfavorite "query"`, to remove a specific album from favorites
  without it being the last pick
- Filters can now appear before or after a query, so both
  `favorite "miles" --year 1959` and `favorite --year 1959 "miles"` work

## Changed exit codes

v1 exited 0 in several cases where nothing had happened. v2 uses one rule:
**0 means the command produced what you asked for, 1 means it could not.**

| Situation | v1 | v2 |
|---|---|---|
| No collection file, or collection empty | 0 | 1 |
| No favorites yet | 0 | 1 |
| `pick` — no albums match | 0 | 1 |
| `list` — no albums match | 0 | 1 |
| `unfavorite` — no match | 1 | 0 |

Removing a favorite that is not there now exits 0: the end state you asked for
already holds, the same way `rm -f` succeeds on a missing file. Scripts that
check `$?` may need updating.

## Unchanged

Your `collection.json`, `favorites.json`, and `history.json` are untouched. You
can downgrade to v1 without touching your data.
