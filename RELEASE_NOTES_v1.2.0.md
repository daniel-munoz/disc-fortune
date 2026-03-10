# disc-fortune v1.2.0

Feature release adding album listing.

## New Features

- **List mode** - `--list` displays all albums in your collection instead of picking one at random
- **Filter + list** - `--list` respects all existing filters (`--year`, `--genre`, `--label`, `--format`) and `--favorites`
- **Album count** - List output ends with a summary count (e.g., `12 albums`)

## Examples

```
disc-fortune --list
disc-fortune --list --genre jazz
disc-fortune --list --year 1970-1979 --label "blue note"
disc-fortune --list --favorites
```

## Breaking Changes

None - all existing flags and collection files remain fully compatible.

---
