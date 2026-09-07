# `reroll` — design

**Date:** 2026-09-06
**Status:** Approved in brainstorming; ready for an implementation plan.
**Ships in:** v2.6.0.
**Covers:** one new command, `reroll`. The first feature after
`docs/plans/2026-08-26-roadmap.md` closed with v2.5.0 "Insight" and the package
split landed.

---

## Problem

`pick` appends to `history.json` unconditionally, and since v2.3.0 that file is
no longer a log. It is the input to every decision the tool makes:

- `pick.Draw` in `Fresh` and `Stale` modes excludes or deprioritizes what was
  played recently.
- `--unheard` is defined entirely by it.
- `stats` reports the share of a set ever played from it.
- `favorite`, `unfavorite` and `open` with no query all mean "the last pick".

A record the user looked at and declined to play is recorded identically to one
they listened to end to end. Two consequences follow, and both are wrong in the
same direction:

1. The declined record is *avoided* for roughly the next ten picks, which is
   the opposite of what the user wants — they still have not heard it.
2. It drops off `--unheard` permanently, and no command can put it back.

The only recourse today is running `pick` again, which compounds the damage:
now two records are marked played and only one of them was.

---

## Scope

A `reroll` command: draw a replacement for the last pick and write it **over**
that pick's history entry rather than after it.

Out of scope, each considered in brainstorming and rejected under "Decisions"
below: recording *why* an entry was replaced, remembering what has already been
rejected, an `undo` that removes without replacing, and any guard against
rerolling an old entry.

No new data file, no new network call, no schema change to `history.json`.

---

## Command surface

```
Usage: disc-fortune reroll [flags]

Draws a replacement for your last pick, writing it over that pick's history
entry instead of adding a new one. Use it when the album you were handed is
not what you want to hear: the discarded pick leaves no trace, so it is
neither avoided by later picks nor counted as one you have heard.

Flags:
  --favorites      Pick from favorites only
  --unheard        Pick only from albums you have never picked
  --draw WHEN      How to draw: fresh (default), any, or stale.
  --json           Emit machine-readable JSON instead of text
<the shared filter flags>
```

`needsConfig: true`.

**The flag surface is `pick`'s, in full, via `parseSelection("reroll", args)`.**
Nothing is added and nothing is withheld. `--draw` in particular comes free:
`addSelectionFlags` gates it on `name != "list"`, so a command that draws gets
it without touching that function.

**Registration is three places, not one.** A `commands` table entry, a
`commandFlagSet` case in `completion.go`, and a `"reroll": true` row in
`TestEveryCommandHasACompletionDecision`'s `hasOwnFlags` map. Phase 5 recorded
this as the standing cost of a new command; the `FlagSet` automates flags, not
commands. The `hasOwnFlags` length check fails loudly if the row is forgotten,
and `TestFilterFlagsAreDocumented` fails if the usage block omits
`filterFlagHelp`.

---

## Behaviour

1. Load the collection (or favorites) and apply the filters, exactly as `pick`
   does.
2. Load history.
3. **If history is empty, fail:** `Nothing to reroll. Run \`disc-fortune pick\`
   first.` Exit 1, per the v2 exit-code rule.
4. Take the last entry as `dropped` and **remove it from the slice in memory.**
5. Apply `--unheard` and draw, against the shortened history.
6. Write history back with the new pick in `dropped`'s place.
7. Print the new album on stdout.
8. Print `Replaced: <artist> - <title> (<relative time>)` on stderr.
9. Print the sync-staleness notice, as `pick` does.

### Why step 4 comes before step 5

Removing the entry *before* the draw is the entire feature. Three behaviours
fall out of that ordering rather than needing code of their own:

- `--draw fresh` no longer counts the declined record as recently played, so it
  stops being spuriously avoided.
- `--unheard` treats it as unheard again — correct, because it was never heard.
- `--draw stale` stops treating it as freshly played.

The same ordering is why a legacy entry with no release ID behaves correctly:
such an entry is a wildcard for its "Artist - Title" (Phase 3), so dropping it
restores *every* pressing of that title to the unheard pool. That is right —
none of them were played.

### What needs no code at all

`favorite`, `unfavorite` and `open` with no query resolve "the last pick" from
history, so after a reroll they act on the new album automatically. Chained
rerolls need no handling either: the second drops what the first wrote. A
favorite added to a record that is then rerolled away survives, because
`favorites.json` is keyed by album and is not touched here.

### Ordering of failures

The checks run in the order the shared code path already imposes: collection
errors (`No collection found`, `Collection is empty`), then `No albums match
the specified filters`, then `Nothing to reroll`, then `--unheard` exhaustion.
So `reroll --genre nonesuch` on an empty history reports the filter problem
rather than the empty history. Accepted: it keeps one code path, and both
messages are true.

### The new entry's timestamp

`time.Now()`, not the dropped entry's. It is a new pick, made now, and
`history` should say "just now" about it. Because it replaces the newest entry,
the file stays ordered by time.

### Output routing

The `Replaced:` line goes to **stderr**, keeping stdout a clean data channel,
and is printed **unconditionally** — not gated on `term.IsTTY(os.Stderr)`. That
is a deliberate departure from `SyncNotice` and `sync`'s progress output, which
are both TTY-gated. Those are advisory nags a script does not want. This is a
receipt for a destructive action, and someone redirecting stderr to a log is
exactly the person who should still get it.

`--json` changes stdout's format and nothing else, per the v2.4.0 rule. The
payload is `pick`'s `pickPayload`, byte-identical in shape; there is no
`replaced` key and no `reroll`-specific payload type. A second payload shape
would have to be kept in step with `jsonAlbum` forever, and the information is
a stderr line away.

---

## Shape

### `internal/disc/history.go`

```go
// ErrHistoryChanged reports that the entry ReplaceLastHistory was asked to
// replace is no longer the most recent one.
var ErrHistoryChanged = errors.New("history changed while rerolling")

// ReplaceLastHistory swaps the most recent entry for album, provided the most
// recent entry is still the one the caller saw.
func ReplaceLastHistory(path string, expected HistoryEntry, album Album) error
```

Under `withFileLock`: reload, and if history is empty or its last entry's
timestamp is not `expected.Timestamp`, return `ErrHistoryChanged` having
written nothing. Otherwise overwrite the last entry with
`HistoryEntry{Album: album, Timestamp: time.Now()}` and save.

**Why a compare-and-swap rather than a lock held across the draw.** `runPick`
deliberately does not hold the lock while deciding, and this must not either.
But `reroll` cannot be as relaxed about staleness as `pick` is: if another
process appended between our unlocked read and our write, a blind "drop the
last entry" would delete *that* entry — the wrong one, and one the user was
never shown. The CAS costs one comparison and makes the failure loud instead of
silent. One lock, at the outermost layer of the read-modify-write, never
nested, as `lock.go` requires.

**Why the timestamp is the guard and `SameAlbum` is not.** An entry with no
release ID is a wildcard for its name, so `SameAlbum` would happily accept a
different pressing as "the entry we saw". `Timestamp` is the exact identity of
a history *entry*. Both sides of the comparison are decoded from the same file
by `LoadHistory`, so the RFC 3339 round-trip is not a hazard.

### `main.go`

`runPick` and a reroll differ in exactly two places — whether the last entry is
dropped before drawing, and which writer runs afterwards. Rather than duplicate
the function, both become one-liners over a shared path:

```go
type recordMode int

const (
    recordAppend  recordMode = iota // pick: add an entry
    recordReplace                   // reroll: write over the last one
)

func (a app) drawAndRecord(cfg selection, mode recordMode) error

func (a app) runPick(cfg selection) error   { return a.drawAndRecord(cfg, recordAppend) }
func (a app) runReroll(cfg selection) error { return a.drawAndRecord(cfg, recordReplace) }
```

Named constants rather than a bare boolean, so the call sites read as
`a.drawAndRecord(cfg, recordReplace)`. One path is the point: the alternative
is two functions that drift, which is why the filter-flag tables in `cli.go`
exist at all.

Two guidance errors join the existing set beside `errNoCollectionGuidance`,
following the same pattern of attaching the advice to the error rather than
printing at the failure site:

```go
errNothingToReroll  = errors.New("Nothing to reroll. Run `disc-fortune pick` first.")
errRerollRaced      = errors.New("The last pick changed while rerolling; nothing was replaced.\n" +
                                 "Run `disc-fortune history` to see what happened.")
```

**The write happens before the output**, as it already does in `runPick`. A CAS
failure must not print an album the tool did not record.

---

## Acceptance

- `disc-fortune reroll` replaces the last history entry: the number of
  entries is unchanged and every earlier entry is untouched.
- The dropped album is eligible for the replacement draw — in particular
  `reroll --unheard` on a one-album collection returns that album rather than
  failing, which is only possible if the drop precedes the filter.
- `reroll` on an empty history exits 1 with the `Nothing to reroll` message and
  writes nothing.
- The `Replaced:` line appears on stderr and never on stdout, with or without
  `--json`, TTY or not.
- `reroll --json` stdout is the same payload shape as `pick --json`, with no
  extra keys.
- `ReplaceLastHistory` leaves the file byte-identical when the last entry has
  moved on, and returns `ErrHistoryChanged`.
- No `.tmp` residue in the config directory afterwards. The `.lock` sidecar
  persists, as it does for every other locked write — that is what
  `isLockSidecar` exists for. `writeFileAtomic`'s existing tests cover the
  temp-file half, since `SaveHistory` goes through it.
- `disc-fortune help` lists `reroll`; `disc-fortune help reroll` prints its
  usage with the filter flags and the global flags appended.
- Completion offers `reroll` and its flags in all three shells.

---

## Tests

**`internal/disc/history_test.go`** — `ReplaceLastHistory` replaces the last
entry and preserves earlier ones; refuses and writes nothing when the last
entry's timestamp differs; refuses on an empty file; stamps the new entry with
the current time; leaves no temp file behind.

**Application level** (`main_test.go` / `app_test.go`) — history length and
contents across a `pick`-then-`reroll` sequence; the empty-history failure and
its exit code; `Replaced:` on stderr and absent from stdout; `--json` stdout
shape.

The ordering proof deserves naming, because it is the one behaviour a reader
would not guess: **a one-album collection, `pick --unheard`, then `reroll
--unheard`.** If the drop precedes the `--unheard` filter, the reroll succeeds
and returns that album. If it does not, the command fails with "Every album
matching your filters has already been played". Deterministic with no seeded
RNG, which matters because `runPick` calls `pick.NewRNG()` directly and the
application layer therefore cannot seed a draw. Draw-eligibility questions that
*do* need a seed stay in `internal/pick`, where the RNG is injectable.

**`completion_test.go`** — the `hasOwnFlags` row. The length check fails
without it, so this is forced rather than remembered.

---

## Files

| File | Change |
|---|---|
| `internal/disc/history.go` | `ReplaceLastHistory`, `ErrHistoryChanged` |
| `internal/disc/history_test.go` | tests for both |
| `main.go` | `recordMode`, `drawAndRecord`, `runReroll`, two guidance errors, `version` → `2.6.0` |
| `cli.go` | `commands` entry for `reroll` |
| `completion.go` | `commandFlagSet` case |
| `completion_test.go` | `hasOwnFlags` row |
| `main_test.go` / `app_test.go` | behaviour tests |
| `README.md` | commands table row, and prose under "Discovery" |
| `docs/releases/RELEASE_NOTES_v2.6.0.md` | new |

---

## Decisions

Recorded because the alternatives are all reasonable and a later reader will
wonder why they were not taken.

**Plain undo, with no memory of what was rejected.** The declined album is
fully eligible for the replacement draw, so a reroll can hand back the record
just turned down. With a large pool this is negligible; with `--favorites` or a
narrow genre it is not. Accepted anyway, on the grounds that the remedy —
rerolling again — costs nothing, and that plain undo is a strict *subset* of
the alternatives: excluding the declined album, or persisting a rejection set
cleared by the next ordinary pick, can both be added later without changing any
grammar committed to here.

**No "skipped" flag on `HistoryEntry`.** Keeping the entry and marking it not
played would preserve the fuller record, but it changes the history schema and
every consumer of it — `--json`, `stats`, `FormatHistory`, the backfill — for a
distinction the user does not want to see.

**No staleness guard.** A reroll days after the pick erases a listen that
really happened. A time threshold would prevent it, but the threshold is a
guess about one person's listening habits and a wrong guess blocks a legitimate
reroll, which then wants an escape-hatch flag, which is more permanent grammar.
The `Replaced:` line is the chosen mitigation: it does not prevent the mistake,
but it makes it visible immediately and names the one thing needed to undo it
by hand.

**`reroll`, not `redo`, `again` or `repick`.** A rerolled die's previous value
never counted, which is exactly the semantics, and it carries none of the
undo/redo baggage the other short names bring from editors and version control.

**A subcommand, not `pick --redo`.** A flag would compose with every present
and future `pick` flag for free and add nothing to the commands table. But it
would hide a history-rewriting action inside the everyday command, and this
tool's whole reason for the change is that history is load-bearing. Keeping the
append-only command and the rewriting one visibly distinct is worth one table
entry. A `reroll` alias for `pick --redo` was rejected in turn: two spellings
of one behaviour is two things to document, test and keep in step forever.
