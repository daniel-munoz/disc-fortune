# disc-fortune v2.5.0 — "Insight"

This closes the post-v2.0.0 roadmap. `stats` and `open` were the last two
items on it, and both ship here.

## `stats`

Summarizes whatever your filters describe — the whole collection by default:

```bash
# Summarize the whole collection
disc-fortune stats

# Summarize whatever a filter describes
disc-fortune stats --genre jazz
disc-fortune stats --decade 70s

# Summarize your favorites
disc-fortune stats --favorites
```

It reports a decade histogram, your five most common genres and labels, and
how much of the set you have ever played. It reads only files already on
disk — it never contacts Discogs.

**The share-ever-played figure is measured against the set being described,
not your whole collection.** `stats --genre jazz` reports the share of your
*jazz* ever picked, so it is not comparable across two different filters.

`stats` takes the same filter flags as `pick` and `list`, but not
`--unheard`: that flag is defined by history, and the share of an
unheard-only set is always zero by construction.

## `open`

Opens a record's Discogs page:

```bash
# Open the last pick
disc-fortune open

# Open a specific record
disc-fortune open "kind of blue"
disc-fortune open --release-id 1839278

# Print the URL instead of opening anything
disc-fortune open --print
```

`open` inherits `favorite`'s grammar exactly: no query means the last pick, a
query or `--release-id` names one record, and an ambiguous query lists the
candidates with their release IDs rather than guessing.

With nothing to launch into — no launcher on `PATH`, or no display — `open`
prints the URL instead and still exits 0. That is a degradation, not a
failure: over SSH or in a script, the URL is what you actually wanted.

A record with no release ID cannot be opened. That only happens for a pick
recorded before v2.2.0 that `sync` could not identify; run `disc-fortune
sync`, or name the record with `--release-id`.

## `stats --json`

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

Every key is always present, and `decades`, `genres` and `labels` are `[]`
rather than `null` when empty. `picked.share` is an **unrounded** fraction of
`count`, not of `total` — the text view rounds it for display, so a consumer
should not have to un-round it back.

## Nothing existing changed

No flag, output, exit code or file format moved. `matchAlbums` and a shared
query-grammar helper were extracted so `favorite`, `open` and `unfavorite`
share one grammar instead of three copies of it — internal only, with no
user-visible effect.

## Upgrading

Nothing to do. No migration, no new file, no new network call.
