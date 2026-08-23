# disc-fortune v1.3.0

Feature release adding favoriting by query.

## New Features

- **Favorite by query** - `--favorite "kind of blue"` marks a specific album as a favorite without having to pick it first (case-insensitive substring match against `Artist - Title`)
- **Filter narrowing** - `--favorite` accepts the existing filters (`--year`, `--genre`, `--label`, `--format`) to disambiguate when a query matches more than one album
- **Ambiguity handling** - When a query matches multiple albums, they're listed so you can refine the query or add filters
- **Query filtering** - `Filter` now supports an artist+title query term, shared by `--favorite` and available to future commands

## Examples

```
disc-fortune --favorite "kind of blue"
disc-fortune --favorite "miles" --year 1959
disc-fortune --favorite "coltrane" --genre jazz
```

## Behavior Notes

- Exactly one match: the album is added and confirmed with `Added to favorites: Artist - Title`
- Already favorited: prints `Already in favorites` and exits successfully
- No match or multiple matches: exits with status 1
- `--favorite` is mutually exclusive with `--favorite-last` and `--favorites`, and an empty query is rejected

## Breaking Changes

None - all existing flags and collection files remain fully compatible.

---
