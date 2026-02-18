# disc-fortune

A Unix `fortune`-style CLI that randomly picks a vinyl record from your Discogs collection.

## Setup

1. Get a Discogs personal access token from https://www.discogs.com/settings/developers
2. Export it:
   ```sh
   export DISCOGS_TOKEN=your_token_here
   ```
3. Build:
   ```sh
   go build -o disc-fortune .
   ```

## Usage

```sh
# Sync your full Discogs collection
disc-fortune --sync

# List available folder names
disc-fortune --sync --list-folders

# Sync only specific folders
disc-fortune --sync --folder "Vinyl 12\"" --folder "Vinyl 7\""

# Print a random album
disc-fortune
```

## Data

Your collection is stored locally at `~/.config/disc-fortune/collection.json` as a JSON array of `{artist, title}` objects.
