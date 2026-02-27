# disc-fortune v1.0.0

Initial release.

## Features

- **Random album pick** — run `disc-fortune` to print a random "Artist - Title" from your saved Discogs collection
- **Collection sync** — `disc-fortune --sync` fetches your full Discogs collection via the API and stores it locally
- **Folder filtering** — `disc-fortune --sync --folder "Vinyl 12\""` syncs only specific folders; releases appearing in multiple folders are deduplicated
- **Folder listing** — `disc-fortune --sync --list-folders` shows available folder names so you know valid `--folder` values
- **Local storage** — collection saved as a minimal JSON file at `~/.config/disc-fortune/collection.json`
- **No external dependencies** — uses only the Go standard library

## Setup

1. Get a personal access token from https://www.discogs.com/settings/developers
2. `export DISCOGS_TOKEN=<your token>`
3. `go build -o disc-fortune .`
4. `disc-fortune --sync` to fetch your collection, then `disc-fortune` to get a random pick
