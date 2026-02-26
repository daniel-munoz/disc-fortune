# disc-fortune v1.1.0

Feature release adding metadata display, filtering, history, and favorites.

## New Features

- **Metadata display** - Albums now show year, label, catalog number, and genres
- **Colored output** - Artist, title, and metadata color-coded in terminal (auto-disabled when piped)
- **Year filtering** - `--year 1975` or `--year 1970-1980`
- **Genre filtering** - `--genre jazz` (case-insensitive substring match)
- **Label filtering** - `--label blue-note`
- **Format filtering** - `--format 12\"`
- **Pick history** - `--history 10` shows past picks with relative timestamps
- **Favorites** - `--favorite-last` to mark, `--favorites` to pick from favorites only
- **Unfavorite** - `--unfavorite-last` to remove last pick from favorites
- **Filter combinations** - All filters work together and with favorites

## Data Storage

Collection now includes metadata. After upgrading, run `disc-fortune --sync` to populate metadata for existing albums.

Three JSON files in `~/.config/disc-fortune/`:
- `collection.json` - enriched with metadata
- `history.json` - timestamped picks
- `favorites.json` - favorited albums

## Breaking Changes

None - v1.0.0 collection files are automatically compatible.

---
