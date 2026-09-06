# disc-fortune v2.2.1 — "Identity, finished"

A patch release, because it closes a gap that v2.2.0 opened rather than adding
a capability.

v2.2.0 stopped merging two pressings of the same title, which was the right
fix — but it left you unable to act on the result. Two store-exclusive colour
variants of one record can be identical in artist, title, year, label,
catalogue number and genre, so `favorite` would list two indistinguishable
blocks and no filter could tell them apart:

```
Slowdive - Souvlaki
1993 · Creation · CRELP 148 · Rock

Slowdive - Souvlaki
1993 · Creation · CRELP 148 · Rock

2 albums
Be more specific or add filters.
```

There was nothing to be more specific *with*. This release fixes that two ways.

## `--format` now matches the colour

Discogs records a pressing's colour in the format's free text — `"Blue
Translucent"`, `"Coke Bottle Clear"` — and disc-fortune was discarding that
field at parse time. It is now kept, so the existing `--format` filter finds it:

```sh
disc-fortune favorite souvlaki --format "coke bottle"
```

**This needs a re-sync to take effect**, since the colour has to be fetched
into `collection.json` before it can be filtered on. It also depends on whoever
entered the release on Discogs having filled the field in — which is why the
next section exists.

## `--release-id` names one record exactly

```sh
disc-fortune favorite --release-id 1839278
```

Accepted by `pick`, `list`, `favorite` and `unfavorite`. It matches the whole
ID rather than a substring — `183` will not match `1839278` — because an ID is
an identity, not a search term.

**It needs no query beside it.** The other filters narrow a query and still
require one, so `disc-fortune favorite --year 1959` is still an error. A
release ID is already a complete answer, so demanding a redundant query
alongside it would be pointless.

## You can now see the IDs

A flag you cannot discover a value for is useless, so an ambiguous match now
shows each candidate's release ID, and says what to do with it:

```
Slowdive - Souvlaki
1993 · Creation · CRELP 148 · Rock
release 1839278

Slowdive - Souvlaki
1993 · Creation · CRELP 148 · Rock
release 9112233

2 albums
Be more specific, add filters, or use --release-id.
```

IDs appear only here, where you have to choose. `list`, `pick` and `history`
print exactly what they printed in v2.2.0 — verified by diffing this build's
output against v2.1.1's across both pre- and post-migration data files.

An entry saved before v2.2.0 has no ID yet and simply shows no `release` line;
it gets one on your next sync.

## Also

- A message about finding nothing now names what you actually asked for.
  `favorite --release-id 999` says `No albums match release 999` rather than
  reporting an empty query.
- The filter flags' help text is hand-written into each command's usage block,
  so it could drift out of step with the flags themselves — and did.
  There is now a test that fails if a command accepting the filters does not
  document every one of them.
