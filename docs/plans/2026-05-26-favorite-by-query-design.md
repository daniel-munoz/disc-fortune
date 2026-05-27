# Favorite-by-Query Design

**Date:** 2026-05-26
**Status:** Approved

## Overview

Add a way to favorite a specific album from the collection without it being the most recent pick. A new `--favorite "query"` flag matches a case-insensitive substring against `Artist - Title` and adds the matched album to favorites. The existing `--favorite-last` shortcut is preserved.

## Goals

- Favorite any album in the collection by name, not only the last pick
- Reuse existing filter composition (`--year`, `--genre`, `--label`, `--format`) to disambiguate matches
- Keep the CLI non-interactive and consistent with the existing flag conventions
- No new dependencies; stdlib only

## Non-Goals

- Interactive disambiguation prompts
- Fuzzy matching (plain case-insensitive substring matches the rest of the project's filter style)
- Exposing the query mechanism to other modes (`--list`, `--fortune`, `--favorites`) — out of scope for this change

## CLI Surface

New flag:

```
--favorite QUERY    Add a specific album to favorites by query
                    (e.g., --favorite "kind of blue")
```

Examples:

```sh
disc-fortune --favorite "kind of blue"
disc-fortune --favorite "miles" --year 1959
disc-fortune --favorite "coltrane" --genre jazz
```

### Resolution rules

After applying the query alongside any other active filters:

| Result      | Behavior                                                                                                  |
|-------------|-----------------------------------------------------------------------------------------------------------|
| 1 match     | Add to favorites. Print `Added to favorites: Artist - Title` (same wording as `--favorite-last`). Exit 0. |
| Multiple    | Print the matches using `formatList` (which already includes a trailing `N albums` count), then a final line `Be more specific or add filters.` Exit 1. |
| 0 matches   | Print `No albums match query "QUERY"`. Exit 1.                                                            |
| Already fav | Print `Already in favorites` (reuses existing `ErrAlreadyInFavorites` path). Exit 0.                       |

### Error and conflict cases

- `--favorite ""` (explicit empty query) → `Error: --favorite requires a query`, exit 1
- `--favorite` set together with `--favorite-last` → `Error: --favorite and --favorite-last are mutually exclusive`, exit 1
- `--favorite` set together with `--favorites` (the pick-from-favorites flag) → `Error: --favorites is for picking from favorites, not adding`, exit 1
- Collection missing or empty → same messages as the existing `runFortune`/`runList` flows

`--favorite-last` and all existing flows remain unchanged.

## Implementation

### `filter.go`

Add a `Query` field to the existing `Filter` struct. The query matches case-insensitive substring against `Artist + " - " + Title` (equivalent to `Album.Key()`).

```go
type Filter struct {
    Query  string // new — matches against "Artist - Title"
    Year   string
    Genre  string
    Label  string
    Format string
}
```

A new `matchesQuery(album Album) bool` method wires into `matches()` alongside the existing field checks. An empty `Query` matches everything (consistent with how the other Filter fields behave).

The fast-path early-return in `Apply()` is extended to include `Query`:

```go
if f.Query == "" && f.Year == "" && f.Genre == "" && f.Label == "" && f.Format == "" {
    return albums
}
```

### `main.go`

- Add a flag: `favoriteFlag := flag.String("favorite", "", "Add a specific album to favorites by query (e.g., --favorite \"kind of blue\")")`
- Detect "was it set?" via `flag.Visit` (same pattern already used for `--history`) so `--favorite ""` is distinguishable from "not provided" and can be rejected explicitly.
- Dispatch order in `main()`:
  1. `--version`
  2. `--history`
  3. **`--favorite` (new)** — placed before `--favorite-last` so the conflict check against `--favorite-last` and `--favorites` fires before either of those dispatches and returns
  4. `--favorite-last`
  5. `--unfavorite-last`
  6. `--list`
  7. `--sync`
  8. Default: `runFortune`

- New function `runFavorite(query string, filter Filter)`:
  1. Load the collection using the same error handling as `runList`/`runFortune`
  2. Set `filter.Query = query`
  3. `matches := filter.Apply(albums)`
  4. Branch on `len(matches)`:
     - `0` → print no-match message, exit 1
     - `>1` → `fmt.Print(formatList(matches, useColor))` followed by `Be more specific or add filters.`, exit 1
     - `1` → `addFavorite(favoritesPath(), matches[0])`, handle `ErrAlreadyInFavorites`, print the success line

### `favorites.go`

A testable seam, `favoriteByQuery(collection, query, filter, favPath) (FavoriteOutcome, error)`, encapsulates the branch-on-match-count logic so the orchestration can be unit-tested without filesystem mocking. It returns a `FavoriteOutcome` whose `Status` is one of `FavoriteAdded`, `FavoriteAlreadyFav`, `FavoriteNoMatch`, or `FavoriteMultiMatch`. `runFavorite` becomes a thin I/O wrapper that loads the collection and dispatches on the returned status. Existing `addFavorite` and `ErrAlreadyInFavorites` are reused inside the seam.

### No changes required

- `collection.go` — `loadCollection` is reused
- `discogs.go`, `history.go`, `color.go` — untouched

### README

Document the new flag under the **Usage** section alongside the existing `--favorite-last` examples. No structural changes to the README.

## Testing

### `filter_test.go`

- `Query` matches artist substring
- `Query` matches title substring
- `Query` matches case-insensitively
- `Query` composes correctly with `Year`, `Genre`, `Label`, `Format`
- Empty `Query` matches every album (no filtering when only Query is unset)

### `favorites_test.go` or `main_test.go`

- Single match: favorites file gains the album; success message printed
- Multiple matches: favorites file unchanged; matches listed; exit non-zero
- Zero matches: favorites file unchanged; no-match message; exit non-zero
- Already favorited: favorites file unchanged; "Already in favorites" message; exit 0
- Conflict cases (`--favorite` + `--favorite-last`, `--favorite` + `--favorites`, empty query) produce the expected error messages

## Risk and reversibility

Low risk:

- Purely additive flag; no existing behavior changes
- `Filter.Query` defaults to `""`, which is a no-op in `Apply()` for all current callers
- New code path writes to `favorites.json` using the existing `addFavorite` helper (already covered by tests and used by `--favorite-last`)
- Easy to roll back by reverting the commit

## Open questions

None.
