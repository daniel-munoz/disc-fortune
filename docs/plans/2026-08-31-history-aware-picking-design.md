# T5 — History-aware picking

**Date:** 2026-08-31
**Phase:** 3 — v2.3.0 "Discovery"
**Status:** Approved design. Implementation plan to follow.
**Roadmap:** [`2026-08-26-roadmap.md`](2026-08-26-roadmap.md), task T5.

---

## Problem

`randomAlbum` (`collection.go:126`) is a bare `rand.IntN` over the filtered slice.
`history.json` is written on every pick by `addToHistory` and never read back into
the decision. The tool records what you played and then ignores it completely, so a
collection of 400 records will hand you the same one twice in a week and call it
chance.

The roadmap splits the fix into three behaviors — anti-repeat, `--unheard`, and
recency weighting — and is explicit that they are *one feature*, not three ballot
lines. The original vote diluted them across three entries and pushed each down the
rankings; the maintainer and leverage reviewers both argued they should be presented
together, which is how they are treated here.

## Constraints inherited from earlier phases

These are not preferences. Each one is a rule an earlier phase paid for.

- **`sameAlbum`, not `Identity()`.** Phase 2 established that `Album.Identity()` is a
  map key valid only where every album is known to carry a release ID (sync dedup),
  and that `sameAlbum(a, b)` is the lenient pairwise comparison for "is this the same
  record as that one". History entries can predate release IDs, so every identity
  comparison in this task uses `sameAlbum`.
- **`sameAlbum` is not transitive**, and is safe only inside a *first-match* scan. Two
  defects on the Phase 2 branch came from ignoring this. Every history lookup here is
  therefore a backwards scan that stops at the first hit.
- **`--favorites` stays a hard filter.** The roadmap records the maintainer's dissent:
  silently converting it to a soft bias would break anyone scripting against today's
  behavior with no warning. `--favorites` remains a hard filter and non-favorites stay
  unreachable, while the pool it produces is subject to the default draw like any other
  filter's pool, and `--draw any` reproduces the v2.2.1 draw.
- **`Album.Key()` stays `"Artist - Title"`.** It is the `--query` search text, not an
  identity.
- **Global flags register in `newFlagSet`**; filter flags in `addFilterFlags`. A
  command must not be able to ship without a flag it should accept.

## Design

### 1. CLI surface

```
pick   --unheard              --draw any|fresh|stale   (default: fresh)
list   --unheard
```

The three behaviors are exposed along the two axes they actually occupy, rather than
as three booleans:

`--unheard` is a **filter on the candidate set**, exactly like `--favorites`. It
registers beside `--favorites` in `parseSelection`, which is what scopes it to `pick`
and `list` and keeps it away from `favorite`/`unfavorite` (those take their flags from
`addFilterFlags`). `list --unheard` — "what have I never played?" — is worth having on
its own, and would be unreachable if `unheard` were a draw mode.

`--draw` is a **draw strategy**, meaningful only where something is drawn. It is
registered in `parseSelection` too, so one parser serves both commands, but rejected
when the command name is `list`.

`Filter` is untouched. `--unheard` needs history, which `Filter` has no access to, and
adding it there would drag history loading into `favorite` and `unfavorite`.

`selection` gains two fields:

```go
type selection struct {
	favoritesOnly bool
	unheard       bool
	draw          drawMode
	filter        Filter
	color         colorMode
}
```

**Why an enum for `--draw` and a boolean for `--unheard`.** Three independent booleans
(`--unheard`, `--stale`, `--any`) would need validation code and error strings for
`--stale --any`, which is a contradiction. A single enum covering all four states
would make `list --unheard` impossible. Splitting them by what each *is* leaves no
invalid combination to reject: `--unheard --draw stale` is legal and simply degenerates
to uniform, because every candidate is never-played and therefore every weight is equal.

`--draw` parses exactly like `--color`, whose shape is already established in
`color.go`:

```go
type drawMode int

const (
	drawFresh drawMode = iota // default: exclude the recently played
	drawAny                   // uniform; history is not consulted
	drawStale                 // fresh, then bias toward the longest unplayed
)

func parseDrawMode(s string) (drawMode, error)
```

`drawFresh` is the zero value, so a `selection` built without an explicit mode gets the
default rather than a silent `any`. The error text follows `parseColorMode`'s:
`invalid --draw value %q (want any, fresh, or stale)`.

### 2. `picker.go` — the decision

A new file, five functions, each independently testable:

```go
func lastPlayedIndex(entries []HistoryEntry, album Album) (int, bool)
func recentlyPlayed(entries []HistoryEntry, n int) []Album
func antiRepeatWindow(poolSize int) int
func unheardOnly(pool []Album, entries []HistoryEntry) []Album
func pickAlbum(pool []Album, entries []HistoryEntry, mode drawMode, rng *rand.Rand) Album
```

`lastPlayedIndex` is the single point where identity is decided, and it is a backwards
scan stopping at the first `sameAlbum` hit — the only shape Phase 2 certified as safe
for a non-transitive comparison. No maps, no `Identity()`. Everything else calls it.

Cost is O(candidates × history) with no index. A collection is hundreds of records and
a history is thousands of entries, so this is a few hundred thousand comparisons on a
local file that was just read from disk. Correctness by construction is worth more here
than an index that would have to reproduce `sameAlbum`'s leniency to be right.

`randomAlbum` is deleted from `collection.go`, along with its now-unused `math/rand/v2`
import; the `drawAny` path in `pickAlbum` replaces it. Its existing test at
`collection_test.go:107` moves to `picker_test.go`.

**Determinism.** `pickAlbum` takes an explicit `*rand.Rand` rather than calling the
global `rand.IntN`. `runPick` constructs one from the global source
(`rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))`); tests pass a fixed PCG seed.
This is what makes the roadmap's "deterministic under a seeded RNG so it can be tested
at all" acceptance criterion reachable.

### 3. The wildcard hazard

A history entry with no `ReleaseID` is a wildcard for its name: `sameAlbum` matches it
against *every* pressing of that title. This is deliberate — it is what stops a pre-2.2
favorite from looking like a different record — but it has two consequences here that
must be designed for rather than discovered.

**Exclusion can empty the pool.** A pool of three identical-titled pressings with
`N = 1` and one name-only history entry excludes all three. `pickAlbum` therefore falls
back to the unexcluded pool whenever exclusion leaves nothing. This is a reachable case
with a concrete reproduction, not defensive padding, and the test suite pins it.

**`--unheard` is conservative.** A name-only history entry hides every pressing of its
title from `--unheard`. That is the right bias: nothing in the file says which pressing
was played, so claiming another one is unheard would assert something the data does not
support. Backfill retires these entries on the first sync after upgrade, which shrinks
the problem to nothing for anyone who syncs.

**Dedup inside the recent window** also uses `sameAlbum`, so two pressings can merge and
shrink the exclusion set by one. This errs in one direction only — it excludes less,
never more — so it cannot empty the pool.

### 4. `fresh`, `stale`, `any`

**`fresh`** — the default. The window is

```
N = min(10, len(pool)/3)
```

computed against the **filtered** pool, not the whole collection, so `--genre jazz`
yielding four albums gets `N = 1` rather than `N = 10`. Dividing by three means at most
a third of the pool is named for exclusion, so for any pool of at least one album the
exclusion *set* is smaller than the pool. A pool of 1 or 2 gets `N = 0` and no exclusion
at all, which is how the roadmap's "must degrade gracefully when the collection is
smaller than N" is satisfied — by arithmetic, not by a special case.

That bounds the count of excluded *names*, not of excluded albums: section 3's wildcard
case is exactly where one name removes several pool entries, and it is why the fallback
in `pickAlbum` is load-bearing rather than belt-and-braces.

`--unheard` is applied before the draw, and after `--favorites` and the filters, since
it operates on whatever `selectAlbums` produced. `pick --favorites --unheard` therefore
means "a favorite I have never played".

Walk history backwards collecting *distinct* albums until `N` are found, exclude them,
draw uniformly from the survivors. Distinct albums rather than raw entries: picking the
same record ten times in a row should not spend the whole window on one album.

**The window is filled from the same pool it is sized against.** History is global, so
before the window is filled the entries are narrowed to those whose album is still a
candidate. Skipping that step makes the two halves disagree: a plain `pick` between two
`pick --favorites` would spend the favorites window on a record that is not a favorite,
and the favorite played moments earlier would be immediately re-pickable — silently
defeating anti-repeat for every filtered pick interleaved with picks from outside that
filter. The same applied to records sold out of the collection, whose entries linger in
history forever and would quietly weaken every future pick. `staleWeights` needs no such
narrowing: it reads position in global history, and filtering changes the magnitudes but
not their order.

**`stale`** — `fresh`'s exclusion, then a weighted draw over the survivors:

```
weight = len(entries) - lastPlayedIndex     (played)
weight = len(entries) + 1                   (never played)
```

The minimum weight is 1, so nothing is ever unreachable. Empty history makes every
weight equal, degenerating to uniform. Never-played outranks every played record by
construction. The weighting is linear rather than exponential because it is simple to
explain in one line of help text and simple to assert in a test; the records that would
justify a sharper curve are the recently played ones, and `fresh` has already removed
them.

Applying `fresh`'s exclusion under `stale` makes `stale` strictly stronger than the
default rather than a different thing, so the anti-repeat guarantee holds no matter
what `--draw` says.

**`any`** — uniform over the pool. History is neither read nor consulted for the
decision. This is the escape hatch that restores v2.2.1 behavior exactly, for anyone
scripting against it.

### 5. Exhaustion and exit codes

`runPick` loads history *before* deciding, then lets `addToHistory` do its own
load-append-save. The decision therefore reads a history that could be marginally stale,
which is harmless, and in exchange no lock is held across a decision. A corrupt
`history.json` becomes fatal to `pick` slightly earlier than before; it was already
fatal, via `addToHistory`, so this is not a regression.

| Situation | Message | Exit |
|---|---|---|
| Filters match nothing | `No albums match the specified filters` (unchanged) | 1 |
| `--unheard`, everything played | `Every album matching your filters has already been played.` plus how to proceed | 1 |
| `list --unheard`, everything played | same, on stderr | 1 |

The two cases are distinguished by *when* the pool empties: before the `--unheard`
filter is the existing message, after it is the new one. Both exit 1, per the v2 rule
that a command exits 1 when it could not produce what was asked for.

### 6. The race — new `lock.go`

The roadmap parked a defect for this phase: `sync`'s backfill rewrites `history.json`,
so a concurrent `pick` can lose its entry. History was a log, and a lost line in a log
is cosmetic. This task makes history drive the decision, so a lost entry now changes
future picks, which is why the roadmap said to close it here.

```go
func withFileLock(path string, fn func() error) error
```

`withFileLock` lives in `lock.go` and holds the platform-independent half: open (or
create) a `<path>.lock` sidecar, defer its close, acquire, run `fn`, release. Only the
two-line primitive is build-tagged — `lockFD` / `unlockFD` in `lock_unix.go`
(`//go:build unix`) calling `syscall.Flock(LOCK_EX)`, and a no-op pair in
`lock_other.go` (`//go:build !unix`) so `go build` stays green everywhere, documented as
offering no cross-process protection there.

flock is preferred over an `O_CREATE|O_EXCL` sentinel because the kernel releases it
when the process exits: a crash can never strand a lock file that a later run has to
decide whether to break.

Wrapped call sites: `addToHistory`, `runBackfill`'s history read-modify-write, and
`addFavorite` / `removeFavorite`.

**Consequence for `migrate`.** The sidecars are new files in the config directory, and
`migrateConfig` copies every regular file it finds there. It must skip them — they are
scaffolding, not the user's data, and copying them would inflate its "moved N files"
count — and then delete them from the source, because its closing best-effort
`os.Remove(from)` only succeeds on an empty directory. Skipping without deleting would
strand the legacy directory permanently. `hasData` is unaffected: it tests for the four
named data files, not for directory emptiness, so a stray sidecar cannot capture
directory resolution.

**No nested acquisition.** Two `LOCK_EX` calls on the same path through two different
file descriptors deadlock, even within one process. The call sites above are safe as
written because `runBackfill` uses `loadHistory`/`saveHistory` and
`loadFavorites`/`saveFavorites` directly, never `addToHistory` or `addFavorite`. Any
future caller must keep `withFileLock` at the outermost layer of a read-modify-write and
never call a locking helper from inside another one.

Favorites are included deliberately. `runBackfill` rewrites `favorites.json` by exactly
the same read-modify-write, and a concurrent `disc-fortune favorite` loses exactly the
same way. The helper exists either way; leaving a known-identical race open next to the
one being fixed would be a worse outcome than a three-line widening.

### 7. Release

One `release: v2.3.0 "Discovery"` commit, last in the branch, matching the shape of
`9ce0c90`:

- `main.go:11` — `const version` to `"2.3.0"`. `discogs.go:36` derives `userAgent` from
  it, so the bump is also what keeps the Discogs User-Agent accurate. No test hardcodes
  the string; `discogs_retry_test.go:40` compares against the constant, which is T2's
  anti-drift guarantee doing its job.
- `RELEASE_NOTES_v2.3.0.md` — new.
- `README.md` — `--unheard` and `--draw` in the flags table, plus a Discovery section.
- `docs/plans/2026-08-26-roadmap.md` — Phase 3 marked shipped, with inherited decisions
  recorded the way Phases 1 and 2 record theirs.

Minor rather than patch: new user-visible capability, and plain `disc-fortune` changes
behavior. **The default change is the thing to call out prominently in the notes**, the
way v2.2.0 called out the collection-count jump, together with `--draw any` as the
one-flag restoration of the old behavior.

### 8. Explicitly out of scope

- **Soft weighting of favorites.** The roadmap forbids it without its own flag. Not in
  this task.
- **An index over history.** The backwards scan is correct and fast enough. An index
  would have to reproduce `sameAlbum`'s leniency to be right, which is the trade the
  Phase 2 defects came from.
- **Time-based anti-repeat.** Considered and rejected: 30 picks in one evening would
  exhaust a day-based window, and every test would need an injected clock. The window is
  counted in picks.
- **`--draw` on `list`.** Nothing is drawn; accepting it would be a flag that does
  nothing.
- **A `--seed` flag.** The RNG is injected for tests, not exposed.

## Acceptance

- Picking is deterministic under a seeded RNG.
- `N` degrades by arithmetic: pool of 1 or 2 → `N = 0`; pool of 3 → `N = 1`; pool of
  100 → `N = 10`.
- Anti-repeat never returns an empty set, including when a name-only history entry
  wildcard-matches every pressing in the pool.
- `--unheard` on a fully-heard collection fails loudly with an actionable message and
  exits 1.
- `--draw any` reproduces v2.2.1 picking exactly.
- `--favorites` behavior is byte-identical to v2.2.1, and its existing tests pass
  unchanged.
- `--unheard` is accepted by `pick` and `list`, and rejected by `favorite` and
  `unfavorite`.
- `--draw` is accepted by `pick` and rejected by `list`.
- A concurrent `sync` backfill and `pick` do not lose a history entry.

## Tests

- `antiRepeatWindow` at pool sizes 0, 1, 2, 3, 30, 100.
- `lastPlayedIndex`: most recent hit wins over an earlier one; never-played reports
  false; a name-only entry matches an ID-bearing album.
- `recentlyPlayed`: distinct albums, not raw entries; a history shorter than `N`.
- **Pool-emptying guard:** three identical-titled pressings plus one name-only history
  entry, `N = 1` — `pickAlbum` still returns an album.
- `unheardOnly`: never-played survive; a name-only entry hides every pressing of its
  title.
- `stale` weights: never-played outranks every played record; the longest-unplayed
  outranks a recent one; empty history is uniform; no weight is zero.
- `pickAlbum` under a fixed PCG seed returns the same album across runs, for each of
  the three modes.
- `parseDrawMode`: three valid values plus an unknown one, mirroring the `--color`
  tests.
- A `--unheard` documentation guard mirroring `TestFilterFlagsAreDocumented`, so the
  flag cannot ship undocumented on a command that accepts it.
- Exit codes for both empty-pool cases, and the distinct messages.
- `withFileLock` serializes two goroutines contending for the same path.
- **Regression guard:** the existing `--favorites` tests pass unchanged.

## Files

| File | Change |
|---|---|
| `picker.go` | new — `drawMode`, `parseDrawMode`, the five decision functions |
| `lock.go` | new — `withFileLock`, the platform-independent half |
| `lock_unix.go` / `lock_other.go` | new — build-tagged `lockFD` / `unlockFD` |
| `collection.go` | delete `randomAlbum` and the `math/rand/v2` import |
| `cli.go` | `--unheard` and `--draw` in `parseSelection`; `selection` fields; usage blocks |
| `main.go` | `runPick` history load, RNG, `pickAlbum`; `runList` unheard; exhaustion messages |
| `history.go` | `addToHistory` under `withFileLock` |
| `favorites.go` | `addFavorite` / `removeFavorite` under `withFileLock` |
| `backfill.go` | `runBackfill`'s history and favorites writes under `withFileLock` |

`main.go`'s `version`, `RELEASE_NOTES_v2.3.0.md`, `README.md` and the roadmap update are
release-time work per section 7, not part of the implementation tasks.
