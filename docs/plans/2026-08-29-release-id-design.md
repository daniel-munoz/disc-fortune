# T4 — Store the Discogs release ID

**Date:** 2026-08-29
**Phase:** 2 — v2.2.0 "Identity"
**Status:** Approved design. Implementation plan to follow.
**Roadmap:** [`2026-08-26-roadmap.md`](2026-08-26-roadmap.md), task T4.

---

## Problem

`Album.Key()` returns `Artist + " - " + Title`, and that one string is doing four
unrelated jobs:

1. the sync deduplication key (`sync.go`, `collectAlbums`),
2. the favorites identity (`favorites.go`, `addFavorite` / `removeFavorite`),
3. the text that `--query` substring-matches against (`filter.go:53`),
4. the implied identity of a history entry, for whenever something finally reads
   history back.

Because of (1), two distinct pressings of the same title collapse into one entry
during sync and one of them silently disappears from the collection. Because of
(2), an upstream retitle on Discogs orphans a favorite. The API returns a stable
release ID on every release and the tool currently throws it away.

## The landmine

The roadmap sketched this as "`Key()` prefers the release ID when non-zero and
falls back to the legacy string when it is absent." That cannot be done as
written. Job (3) above means `Key()` is also the *search* string: making it
return `id:12345` would silently break every `--query`, every `favorite QUERY`,
and every `unfavorite QUERY`, with no error and no test failure outside
`filter_test.go`.

Identity and search text have to become two different things.

## Design

### 1. Data model

`Album` gains one field, first in the struct so it leads each record in the
JSON files:

```go
type Album struct {
	ReleaseID int      `json:"release_id,omitempty"`
	Artist    string   `json:"artist"`
	Title     string   `json:"title"`
	Year      int      `json:"year,omitempty"`
	Label     string   `json:"label,omitempty"`
	CatNo     string   `json:"catno,omitempty"`
	Genres    []string `json:"genres,omitempty"`
	Formats   []string `json:"formats,omitempty"`
}
```

`omitempty` keeps pre-migration records byte-identical to what v2.1.0 wrote, and
Go's decoder ignores unknown fields, so a user who downgrades to v2.1.0 loses
nothing.

`Key()` is **unchanged**. It keeps returning `"Artist - Title"` and keeps being
the search text; only its doc comment changes, to say that it is the human label
and the legacy identity rather than "a deduplication key".

Identity is expressed two ways, because the two call sites need different things:

```go
// Identity is a map key. Sync dedup is its only caller, and there every
// album comes straight from the API, so the ID is always present.
func (a Album) Identity() string {
	if a.ReleaseID != 0 {
		return "id:" + strconv.Itoa(a.ReleaseID)
	}
	return "name:" + a.Key()
}

// sameAlbum is a pairwise comparison, deliberately lenient when either side
// predates the release ID.
func sameAlbum(a, b Album) bool {
	if a.ReleaseID != 0 && b.ReleaseID != 0 {
		return a.ReleaseID == b.ReleaseID
	}
	return a.Key() == b.Key()
}
```

**Why two.** A single ID-preferring key would make a pre-2.2 favorite (no ID) and
that same record freshly synced (with an ID) compare as different albums, so
`favorite` would happily append a duplicate of something already favorited.
`sameAlbum` avoids that by letting an un-ID'd entry act as a wildcard for its
name.

`sameAlbum` is therefore **not transitive**: given `A(id=1, "X")`,
`B(no id, "X")` and `C(id=2, "X")`, `A~B` and `B~C` but `A≁C`. That is acceptable
and bounded inside a linear "is this already in the list?" scan, and it is
precisely why `sameAlbum` must stay out of sync dedup — a non-transitive
comparison there would make the surviving set depend on fetch order. The
prefixes in `Identity()` (`id:` / `name:`) exist so a numeric-looking artist name
can never collide with an ID.

### 2. Sync

`releaseInfo` gains the ID:

```go
type releaseInfo struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	...
}
```

populated from `basic_information.id` into `Album.ReleaseID`.

Note that `instance_id` — a sibling field on the collection release, not inside
`basic_information` — is *not* this value. It identifies a physical copy in the
user's collection. Someone who owns two copies of one pressing has two instances
sharing one release ID, and merging those two into a single entry is the correct
outcome.

`collectAlbums` keys its `seen` map on `Identity()` rather than `Key()`. An album
whose ID is somehow absent falls back to the legacy name key, which is exactly
today's behavior.

Ordering inside `runSync`, which matters:

1. fetch the collection from Discogs
2. load the **previous** `collection.json` — before it is overwritten
3. `saveCollection` — the authoritative write
4. `recordSync`
5. backfill `favorites.json`, then `history.json`
6. print the summary

Backfill runs *after* the collection lands so it can never stamp IDs taken from
a collection that then failed to save. Step 2 has to precede step 3 for the
un-merge notice in §4; a failure to read the previous collection is not an error,
it just means no notice.

**A backfill failure warns on stderr and does not fail the sync.** The sync
itself succeeded, the pass is idempotent, and the next sync retries it. This is
deliberately inconsistent with `recordSync`, whose failure is currently fatal;
the difference is that a missing sync timestamp is invisible, while a failed
backfill announces itself and self-heals.

### 3. Backfill — new `backfill.go`

Build an index over the fresh collection: legacy key → set of distinct release
IDs. Then for each entry whose `ReleaseID` is zero:

- **exactly one** distinct ID for that key → stamp it,
- **zero** → leave it alone. The record is no longer in the collection — sold,
  or removed from the synced folders. Silent; there is nothing the user can do.
- **more than one** → leave it alone and record the key as ambiguous.

The ambiguous case is genuinely unknowable: if `"Miles Davis - Kind of Blue"` now
resolves to three pressings, no stored data says which one the user favorited.
Guessing writes an assertion the user never made and cannot later distinguish
from a real choice. So the entry stays on the name fallback — where it still
displays and still matches — and the user is told, so they can re-favorite the
specific pressing if they care.

```go
type backfillResult struct {
	Updated   int
	Ambiguous []string // legacy keys that matched more than one release
}

func backfillAlbums(entries, collection []Album) ([]Album, backfillResult)
func backfillHistory(entries []HistoryEntry, collection []Album) ([]HistoryEntry, backfillResult)
```

Both are pure functions over their inputs; `sync.go` owns the load, the decision
to write, and the reporting. A file is rewritten only when `Updated > 0`, through
the existing atomic savers from T1. Entries that already carry an ID are never
examined, which is what makes the second sync a no-op.

### 4. Sync output

Everything goes to stdout, next to the existing summary line. `sync`'s stdout has
always been a human report rather than a data channel, so the "stdout is data"
rule that governs `pick` and `list` does not apply here.

```
Synced 412 albums (398 with full metadata)
Note: 7 records share an artist and title with another record. Before v2.2.0
      these were merged into one entry; they are now listed separately.
Filled in release IDs for 12 favorites and 106 history entries.
These favorites matched more than one record and were left as-is:
  Miles Davis - Kind of Blue
```

The "Filled in release IDs" line is omitted entirely when both counts are zero,
and names only the non-zero half when just one file changed.

Only **favorites** report their ambiguous keys. History does the same backfill and
leaves the same entries un-ID'd, but reports nothing: history is a log rather than
a curated list, there is no action for the user to take on a past pick, and a
long-lived history could produce dozens of lines nobody can act on.

The un-merge notice is gated on three conditions together:

- a previous `collection.json` existed, **and**
- at least one of its entries had no release ID, **and**
- the freshly synced collection contains at least one legacy-key collision.

That fires exactly once — on the first sync after upgrading — and suppresses
itself forever afterwards, because every entry has an ID from then on. No flag in
`meta.json` is needed to make it one-time.

The wording deliberately states the collision count as a fact rather than
attributing the whole change in collection size to it. A user who also bought
records since their last sync would otherwise be told something false.

### 5. Favorites

`addFavorite` and `removeFavorite` swap their `Key()` comparison for
`sameAlbum`. Nothing else in `favorites.go` changes: `favoriteByQuery` and
`unfavoriteByQuery` go through `Filter.Apply`, which is untouched.

`history.go` is untouched. History stores whole albums and reads no identity
today; T5 is what will consume the IDs this task puts there.

### 6. Explicitly out of scope

- **Refreshing favorite metadata from the collection.** Once a favorite has an
  ID it is correctly *identified* even after an upstream retitle; its stored
  title simply stays as saved. Rewriting user data that the user did not ask to
  touch widens the blast radius of the riskiest change on the roadmap for a
  cosmetic gain.
- **Showing `release_id` in human output.** `pick`, `list` and `history` render
  exactly as they do in v2.1.1. T7 (`--json`) puts the ID in the machine schema;
  T10 (`open`) consumes it.
- **Retiring the name fallback.** It is permanent for entries the backfill could
  not resolve. This was the maintainer reviewer's stated objection to T4 and is
  accepted: the fallback applies only to entries with no ID, and the backfill
  retires nearly all of them on the first sync after upgrade.

## Acceptance

- Two releases with identical artist and title but different IDs both survive a sync.
- A `favorites.json` written by v2.1.0 still resolves after upgrade, for both
  `favorite` and `unfavorite`, with no data loss and no duplicate entries.
- Backfill is idempotent: running sync twice leaves the data files byte-identical.
- JSON carrying `release_id` decodes into a v2.1-shaped struct without error.
- `--query`, `favorite QUERY` and `unfavorite QUERY` behave exactly as in v2.1.1.
- The un-merge notice appears on the first sync after upgrade and not on the second.

## Tests

- `Identity()` prefers the ID and falls back to the prefixed name.
- `sameAlbum` truth table, including the un-ID'd wildcard case and two different
  IDs sharing a name.
- `collectAlbums`: same artist and title with different IDs → both survive; the
  same ID across two folders → one entry.
- `discogs`: `basic_information.id` is parsed, from an `httptest` fixture.
- `backfillAlbums` / `backfillHistory`: unique match stamps the ID; no match
  leaves the entry untouched; multiple matches leave it and report the key.
  Running the pass twice produces identical output.
- **Regression guard:** `--query` still substring-matches artist and title. This
  is the landmine described above; the test exists so that a future change to
  `Key()` fails loudly.
- Favorites round-trip against a v2.1.0-shaped fixture with no `release_id`.
- Sync summary: the un-merge notice fires on the first sync, is absent on the second.

## Files

| File | Change |
|---|---|
| `collection.go` | `ReleaseID` field, `Identity()`, `sameAlbum`, `Key()` doc |
| `discogs.go` | `releaseInfo.ID`, populated into `Album` |
| `sync.go` | dedup on `Identity()`, backfill orchestration, summary lines |
| `favorites.go` | `sameAlbum` in `addFavorite` / `removeFavorite` |
| `backfill.go` | new — `backfillAlbums`, `backfillHistory`, `backfillResult` |

`main.go`'s `version` and `RELEASE_NOTES_v2.2.0.md` are release-time work, not
part of this task. The release notes must call out the collection-count change
prominently, per the roadmap.
