# disc-fortune v2.2.0 — "Identity"

**No breaking changes to any command.** Every v2.1.1 invocation behaves exactly as
before, and `pick`, `list` and `history` print byte-for-byte what they printed
before.

**But your collection count may go up after the first sync, and that is the
point.** Read the next section before you assume something broke.

## Records that share a name are no longer merged into one

disc-fortune identified a record by the string `"Artist - Title"`. Two different
pressings of the same album — an original and a reissue, a US and a UK press —
produced the same string, so syncing silently kept one and discarded the other.
If you own both, one of them has been missing from your collection this whole
time, and nothing told you.

Every album now carries its Discogs release ID, and that ID decides what counts
as the same record. Both pressings survive a sync, and appear separately in
`list`.

So on your first sync after upgrading, your collection is likely to get bigger.
`sync` says so once, and only once:

```
Synced 412 albums (398 with full metadata)
Note: 7 records share an artist and title with another record. Before v2.2.0
      these were merged into one entry; they are now listed separately.
Filled in release IDs for 12 favorites and 106 history entries.
```

The notice fires on the first sync after the upgrade and never again. It counts
the records that share a name with another record — not the change in your
collection's size, which may also include everything you have bought since your
last sync.

## Favorites survive a retitle on Discogs

A favorite was matched by name too, so if Discogs edited a release's title —
which happens, as the database is community-maintained — your favorite pointed
at nothing and quietly stopped matching.

Once both sides carry a release ID, the title no longer matters. The favorite
follows the record.

## Your existing files migrate themselves

After each sync, disc-fortune fills in release IDs on the favorites and history
entries that predate them, matching them against your freshly synced collection:

- **Exactly one match** — the ID is filled in.
- **No match** — the entry is left alone. The record is no longer in your
  collection; you sold it, or it left the folders you sync. There is nothing to
  do about it, so nothing is said.
- **More than one match** — the entry is left alone and listed for you. See
  below.

This is idempotent. Syncing twice changes nothing the second time, and files are
rewritten only when something actually changed — a user who has never favorited
anything does not get an empty `favorites.json` created for them. The backfill
runs only after your collection has been written successfully, and if it fails it
warns without failing the sync, because the next sync simply retries it.

**Downgrading is safe.** v2.1.x ignores the new `release_id` field, so if you go
back, nothing is lost but the ID itself.

## When a favorite matches more than one pressing

If a favorite you saved before this release now matches several pressings,
nothing on disk records which one you meant. disc-fortune will not guess. It
leaves the entry as it is — it still displays, and still works — and tells you:

```
These favorites matched more than one record and were left as-is:
  Miles Davis - Kind of Blue
```

To resolve it, favorite the pressing you actually want:

```sh
disc-fortune favorite "kind of blue" --label Columbia
```

That replaces the ambiguous entry with the specific one. You end up with one
favorite for that title either way, now pinned to the pressing you named, and
the notice stops.

Until you do, the notice repeats on every sync. That is deliberate: it is the
only signal you get, it is fixable, and silencing it would hide a condition you
can actually act on.

History entries that match more than one pressing are handled the same way but
are not listed. History is a log — there is no useful action to take on a pick
you made last year.

## What has not changed

- `--query`, `favorite QUERY` and `unfavorite QUERY` still match on artist and
  title, exactly as before. The release ID is an identity, not a search term.
- `pick`, `list` and `history` output is unchanged. The release ID lives in your
  data files; it is not printed.
- `--favorites` is still a hard filter.

## Known limitation

`sync` now rewrites `history.json` in order to fill in release IDs. If you run a
`pick` at the same moment a `sync` is rewriting that file, the pick's history
entry can be lost. The window is a few milliseconds on a tool that one person
runs at a time, and closing it properly means file locking, which is a larger
change than this release should carry. It is recorded here rather than left for
you to discover.

## Note for anyone reading the source

Identity is deliberately two things, not one:

- `Album.Identity()` is a map key, used only for sync deduplication, where every
  album comes straight from the API and always has an ID.
- `sameAlbum(a, b)` is a pairwise comparison, used by favorites. It compares IDs
  when both are present and falls back to `"Artist - Title"` when either is not,
  so an entry written before this release is never mistaken for a different
  record.

`sameAlbum` is therefore not transitive: an entry with no ID acts as a wildcard
for its name. That is bounded to first-match scans over the favorites list, and
it is exactly why deduplication uses `Identity()` instead — a non-transitive
comparison there would make the surviving set depend on the order pages arrive
in.

`Album.Key()` still returns `"Artist - Title"` and is untouched, because it is
what `--query` searches against.
