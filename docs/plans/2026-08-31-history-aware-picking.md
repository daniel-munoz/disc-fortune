# History-Aware Picking (T5) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `pick` read the history it has always written, so the tool stops handing you a record you played three days ago, and give the collector two explicit ways to lean harder on that — `--unheard` and `--draw stale`.

**Architecture:** A new `picker.go` holds the whole decision as small pure functions over `([]Album, []HistoryEntry)`, drawn with an injected `*rand.Rand` so it is testable. `--unheard` is a filter on the candidate set (registered beside `--favorites`, so `pick` and `list` both get it); `--draw any|fresh|stale` is a draw strategy registered only on `pick`. A new `lock.go` closes the read-modify-write race between `sync`'s backfill and a concurrent `pick` or `favorite`, which matters now that a lost history entry changes future picks.

**Tech Stack:** Go 1.24.3, standard library only. Single `package main` at the repository root; tests live beside the code as `*_test.go`.

**Spec:** [`docs/plans/2026-08-31-history-aware-picking-design.md`](2026-08-31-history-aware-picking-design.md)

## Global Constraints

- Module is `github.com/daniel-munoz/disc-fortune/v2`, Go 1.24.3. **No third-party dependencies.** `go.mod` must stay dependency-free.
- Everything is `package main` in the repository root. There is no `src/` and no `tests/` directory.
- Run tests with `go test .` from the repository root. A single test is `go test . -run TestName -v`.
- All writes to data files go through the existing atomic savers (`saveCollection`, `saveFavorites`, `saveHistory`). **Never** call `os.WriteFile` on a live data path.
- **Every identity comparison against history uses `sameAlbum`, never `Identity()`, and only inside a backwards scan that stops at the first match.** An entry with no `ReleaseID` is a wildcard for its name; `sameAlbum` is not transitive. This rule is what the two Phase 2 defects cost.
- **`--favorites` must behave exactly as in v2.2.1.** Its existing tests pass unchanged, untouched.
- `Album.Key()` must keep returning exactly `Artist + " - " + Title`. It is the string `--query` substring-matches against.
- Advisory output goes to **stderr**; stdout is the data channel.
- Comments explain *why*, not *what* — match the density and voice of the surrounding code.
- Commit after every task, using the repo's `type: summary` message style (`feat:`, `fix:`, `test:`, `refactor:`).

---

### Task 1: `drawMode` and `parseDrawMode`

The `--draw` flag's value type, mirroring `colorMode`/`parseColorMode` in `color.go`. Nothing consumes it yet.

**Files:**
- Create: `picker.go`
- Test: `picker_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type drawMode int` with constants `drawFresh` (zero value), `drawAny`, `drawStale`
  - `func parseDrawMode(s string) (drawMode, error)`

- [ ] **Step 1: Write the failing test**

Create `picker_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func TestParseDrawMode(t *testing.T) {
	cases := []struct {
		in   string
		want drawMode
	}{
		{"fresh", drawFresh},
		{"any", drawAny},
		{"stale", drawStale},
	}
	for _, c := range cases {
		got, err := parseDrawMode(c.in)
		if err != nil {
			t.Errorf("parseDrawMode(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseDrawMode(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseDrawModeRejectsUnknown(t *testing.T) {
	_, err := parseDrawMode("weighted")
	if err == nil {
		t.Fatal("expected an error for an unknown --draw value")
	}
	if !strings.Contains(err.Error(), "weighted") {
		t.Errorf("error %q does not name the offending value", err)
	}
}

// drawFresh must be the zero value: a selection built without an explicit
// mode has to get the default, not an unfiltered draw.
func TestDrawFreshIsZeroValue(t *testing.T) {
	var m drawMode
	if m != drawFresh {
		t.Errorf("zero drawMode = %v, want drawFresh", m)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run 'TestParseDrawMode|TestDrawFreshIsZeroValue' -v`
Expected: FAIL — `undefined: parseDrawMode`, `undefined: drawFresh`.

- [ ] **Step 3: Write minimal implementation**

Create `picker.go`:

```go
package main

import "fmt"

// drawMode selects how pick draws from the candidate pool.
type drawMode int

const (
	// drawFresh excludes the recently played. It is the zero value so a
	// selection built without an explicit mode gets the default rather than
	// silently falling back to an unfiltered draw.
	drawFresh drawMode = iota
	// drawAny is a uniform draw; history is not consulted at all. This is
	// what restores pre-2.3 behavior for anyone scripting against it.
	drawAny
	// drawStale is drawFresh's exclusion followed by a bias toward the
	// records left unplayed longest.
	drawStale
)

// parseDrawMode converts the --draw flag value to a drawMode.
func parseDrawMode(s string) (drawMode, error) {
	switch s {
	case "fresh":
		return drawFresh, nil
	case "any":
		return drawAny, nil
	case "stale":
		return drawStale, nil
	default:
		return drawFresh, fmt.Errorf("invalid --draw value %q (want any, fresh, or stale)", s)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test . -run 'TestParseDrawMode|TestDrawFreshIsZeroValue' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add picker.go picker_test.go
git commit -m "feat: add drawMode for the --draw flag"
```

---

### Task 2: History lookup primitives

The three functions everything else is built from. All pure, all testable without a config directory.

**Files:**
- Modify: `picker.go`
- Test: `picker_test.go`

**Interfaces:**
- Consumes: `sameAlbum(a, b Album) bool` and `HistoryEntry{Album, Timestamp}` (existing).
- Produces:
  - `func lastPlayedIndex(entries []HistoryEntry, album Album) (int, bool)`
  - `func containsAlbum(list []Album, album Album) bool`
  - `func recentlyPlayed(entries []HistoryEntry, n int) []Album`
  - `func antiRepeatWindow(poolSize int) int`
  - `const maxAntiRepeatWindow = 10`

- [ ] **Step 1: Write the failing test**

Append to `picker_test.go`:

```go
// histOf builds a history whose entries are the given albums, oldest first.
// Timestamps are irrelevant to every function under test -- the window is
// counted in picks, not in time -- so they are left zero.
func histOf(albums ...Album) []HistoryEntry {
	entries := make([]HistoryEntry, len(albums))
	for i, a := range albums {
		entries[i] = HistoryEntry{Album: a}
	}
	return entries
}

func TestAntiRepeatWindowScalesToPool(t *testing.T) {
	cases := []struct{ pool, want int }{
		{0, 0},
		{1, 0},
		{2, 0},
		{3, 1},
		{9, 3},
		{30, 10},
		{100, 10},
	}
	for _, c := range cases {
		if got := antiRepeatWindow(c.pool); got != c.want {
			t.Errorf("antiRepeatWindow(%d) = %d, want %d", c.pool, got, c.want)
		}
	}
}

func TestLastPlayedIndexFindsMostRecent(t *testing.T) {
	a := Album{ReleaseID: 1, Artist: "Slowdive", Title: "Souvlaki"}
	b := Album{ReleaseID: 2, Artist: "Ride", Title: "Nowhere"}
	entries := histOf(a, b, a)

	idx, played := lastPlayedIndex(entries, a)
	if !played {
		t.Fatal("played = false, want true")
	}
	if idx != 2 {
		t.Errorf("idx = %d, want 2 (the most recent play, not the first)", idx)
	}
}

func TestLastPlayedIndexNeverPlayed(t *testing.T) {
	entries := histOf(Album{ReleaseID: 1, Artist: "Ride", Title: "Nowhere"})
	if _, played := lastPlayedIndex(entries, Album{ReleaseID: 2, Artist: "Lush", Title: "Spooky"}); played {
		t.Error("played = true for an album that is not in history")
	}
}

// A history entry written before release IDs existed carries only a name, and
// sameAlbum treats it as that name's wildcard. It must still match the
// ID-bearing album it refers to.
func TestLastPlayedIndexMatchesUnIDdEntry(t *testing.T) {
	stored := Album{Artist: "Slowdive", Title: "Souvlaki"}
	synced := Album{ReleaseID: 42, Artist: "Slowdive", Title: "Souvlaki"}
	if _, played := lastPlayedIndex(histOf(stored), synced); !played {
		t.Error("an un-ID'd history entry did not match its synced self")
	}
}

func TestRecentlyPlayedReturnsDistinctAlbums(t *testing.T) {
	a := Album{ReleaseID: 1, Artist: "A", Title: "1"}
	b := Album{ReleaseID: 2, Artist: "B", Title: "2"}
	// a played three times in a row must not consume the whole window.
	got := recentlyPlayed(histOf(b, a, a, a), 2)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2; got %+v", len(got), got)
	}
	if got[0].ReleaseID != 1 || got[1].ReleaseID != 2 {
		t.Errorf("got %+v, want a then b (most recent first)", got)
	}
}

func TestRecentlyPlayedShorterThanWindow(t *testing.T) {
	got := recentlyPlayed(histOf(Album{ReleaseID: 1, Artist: "A", Title: "1"}), 10)
	if len(got) != 1 {
		t.Errorf("len = %d, want 1", len(got))
	}
}

func TestRecentlyPlayedZeroWindow(t *testing.T) {
	if got := recentlyPlayed(histOf(Album{ReleaseID: 1, Artist: "A", Title: "1"}), 0); len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run 'TestAntiRepeatWindow|TestLastPlayedIndex|TestRecentlyPlayed' -v`
Expected: FAIL — `undefined: antiRepeatWindow`, `undefined: lastPlayedIndex`, `undefined: recentlyPlayed`.

- [ ] **Step 3: Write minimal implementation**

Append to `picker.go`:

```go
// maxAntiRepeatWindow caps how many recently played albums the default draw
// excludes, however large the collection is.
const maxAntiRepeatWindow = 10

// antiRepeatWindow returns how many recently played albums to exclude from a
// pool of poolSize candidates.
//
// Dividing by three is what makes the degradation automatic rather than a
// special case: a pool of one or two excludes nothing, so a heavily filtered
// query can never be narrowed into an empty set. Note that this bounds the
// number of excluded *names*, not of excluded albums -- see excludeRecent for
// why that distinction needs a guard.
func antiRepeatWindow(poolSize int) int {
	n := poolSize / 3
	if n > maxAntiRepeatWindow {
		return maxAntiRepeatWindow
	}
	return n
}

// lastPlayedIndex returns the index in entries of the most recent pick of
// album, and whether it was ever picked.
//
// This is the single point where picking decides what "the same record" means,
// and it scans backwards and stops at the first match. That is the only shape
// sameAlbum is safe in: an entry with no release ID is a wildcard for its
// name, so a comparison that kept scanning would conflate distinct pressings.
func lastPlayedIndex(entries []HistoryEntry, album Album) (int, bool) {
	for i := len(entries) - 1; i >= 0; i-- {
		if sameAlbum(album, entries[i].Album) {
			return i, true
		}
	}
	return 0, false
}

// containsAlbum reports whether album matches any entry of list.
func containsAlbum(list []Album, album Album) bool {
	for _, a := range list {
		if sameAlbum(a, album) {
			return true
		}
	}
	return false
}

// recentlyPlayed returns the last n distinct albums in entries, most recent
// first.
//
// Distinct albums rather than raw entries: playing one record ten times in a
// row should not spend the whole window on that one record.
func recentlyPlayed(entries []HistoryEntry, n int) []Album {
	if n <= 0 {
		return nil
	}
	var recent []Album
	for i := len(entries) - 1; i >= 0 && len(recent) < n; i-- {
		album := entries[i].Album
		if containsAlbum(recent, album) {
			continue
		}
		recent = append(recent, album)
	}
	return recent
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test . -run 'TestAntiRepeatWindow|TestLastPlayedIndex|TestRecentlyPlayed' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add picker.go picker_test.go
git commit -m "feat: add the history lookup primitives picking needs"
```

---

### Task 3: `unheardOnly`

The `--unheard` filter, as a pure function. Not yet reachable from the CLI.

**Files:**
- Modify: `picker.go`
- Test: `picker_test.go`

**Interfaces:**
- Consumes: `lastPlayedIndex` (Task 2).
- Produces: `func unheardOnly(pool []Album, entries []HistoryEntry) []Album`

- [ ] **Step 1: Write the failing test**

Append to `picker_test.go`:

```go
func TestUnheardOnlyKeepsNeverPlayed(t *testing.T) {
	played := Album{ReleaseID: 1, Artist: "A", Title: "1"}
	fresh := Album{ReleaseID: 2, Artist: "B", Title: "2"}

	got := unheardOnly([]Album{played, fresh}, histOf(played))
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1; got %+v", len(got), got)
	}
	if got[0].ReleaseID != 2 {
		t.Errorf("kept release %d, want 2", got[0].ReleaseID)
	}
}

func TestUnheardOnlyEmptyHistoryKeepsEverything(t *testing.T) {
	pool := []Album{{ReleaseID: 1, Artist: "A", Title: "1"}, {ReleaseID: 2, Artist: "B", Title: "2"}}
	if got := unheardOnly(pool, nil); len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
}

// A history entry with no release ID does not say which pressing was played,
// so --unheard must not claim any of them is unheard.
func TestUnheardOnlyIsConservativeAboutUnIDdEntries(t *testing.T) {
	pool := []Album{
		{ReleaseID: 1, Artist: "Slowdive", Title: "Souvlaki"},
		{ReleaseID: 2, Artist: "Slowdive", Title: "Souvlaki"},
	}
	entries := histOf(Album{Artist: "Slowdive", Title: "Souvlaki"})

	if got := unheardOnly(pool, entries); len(got) != 0 {
		t.Errorf("len = %d, want 0; an un-ID'd entry must hide every pressing of its title", len(got))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run TestUnheardOnly -v`
Expected: FAIL — `undefined: unheardOnly`.

- [ ] **Step 3: Write minimal implementation**

Append to `picker.go`:

```go
// unheardOnly returns the albums in pool that never appear in entries.
//
// Conservative by construction: a history entry with no release ID matches
// every pressing of its title, so none of them count as unheard. Nothing in
// the file says which pressing was actually played, and calling the others
// unheard would assert more than the data supports. The backfill retires
// these entries on the first sync after upgrade.
func unheardOnly(pool []Album, entries []HistoryEntry) []Album {
	var out []Album
	for _, album := range pool {
		if _, played := lastPlayedIndex(entries, album); !played {
			out = append(out, album)
		}
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test . -run TestUnheardOnly -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add picker.go picker_test.go
git commit -m "feat: add the unheard filter"
```

---

### Task 4: `pickAlbum` — the three draw modes

The decision itself. `randomAlbum` still exists after this task; Task 6 removes it.

**Files:**
- Modify: `picker.go`
- Test: `picker_test.go`

**Interfaces:**
- Consumes: `drawMode` (Task 1), `antiRepeatWindow` / `recentlyPlayed` / `containsAlbum` (Task 2), `lastPlayedIndex` (Task 2).
- Produces:
  - `func pickAlbum(pool []Album, entries []HistoryEntry, mode drawMode, rng *rand.Rand) Album`
  - `func excludeRecent(pool []Album, entries []HistoryEntry) []Album`
  - `func staleWeights(candidates []Album, entries []HistoryEntry) []int`
  - `func weightedIndex(weights []int, rng *rand.Rand) int`
  - `func newRNG() *rand.Rand`

- [ ] **Step 1: Write the failing test**

Append to `picker_test.go` (add `"math/rand/v2"` to the test imports):

```go
// seededRNG returns a generator pinned to a fixed sequence, which is what
// makes picking assertable at all.
func seededRNG() *rand.Rand {
	return rand.New(rand.NewPCG(1, 2))
}

func poolOf(n int) []Album {
	pool := make([]Album, n)
	for i := range pool {
		pool[i] = Album{ReleaseID: i + 1, Artist: "A", Title: strconv.Itoa(i + 1)}
	}
	return pool
}

func TestPickAlbumIsDeterministicUnderASeed(t *testing.T) {
	pool := poolOf(20)
	entries := histOf(pool[0], pool[1], pool[2])

	for _, mode := range []drawMode{drawAny, drawFresh, drawStale} {
		first := pickAlbum(pool, entries, mode, seededRNG())
		second := pickAlbum(pool, entries, mode, seededRNG())
		if first.ReleaseID != second.ReleaseID {
			t.Errorf("mode %v: got %d then %d from the same seed", mode, first.ReleaseID, second.ReleaseID)
		}
	}
}

func TestPickAlbumFreshExcludesRecent(t *testing.T) {
	pool := poolOf(9) // window = 3
	// The three most recent picks are releases 1, 2 and 3.
	entries := histOf(pool[2], pool[1], pool[0])

	for range 200 {
		got := pickAlbum(pool, entries, drawFresh, seededRNG())
		if got.ReleaseID <= 3 {
			t.Fatalf("fresh returned release %d, which is inside the anti-repeat window", got.ReleaseID)
		}
	}
}

func TestPickAlbumAnyIgnoresHistory(t *testing.T) {
	pool := poolOf(3)
	entries := histOf(pool[0], pool[1], pool[2])

	seen := make(map[int]bool)
	rng := seededRNG()
	for range 200 {
		seen[pickAlbum(pool, entries, drawAny, rng).ReleaseID] = true
	}
	if len(seen) != 3 {
		t.Errorf("saw %d distinct albums, want 3; --draw any must not consult history", len(seen))
	}
}

// The guard that matters. antiRepeatWindow bounds excluded *names*, and one
// un-ID'd history entry is a wildcard matching every pressing of its title,
// so exclusion really can empty a pool.
func TestPickAlbumFallsBackWhenExclusionEmptiesThePool(t *testing.T) {
	pool := []Album{
		{ReleaseID: 1, Artist: "Slowdive", Title: "Souvlaki"},
		{ReleaseID: 2, Artist: "Slowdive", Title: "Souvlaki"},
		{ReleaseID: 3, Artist: "Slowdive", Title: "Souvlaki"},
	}
	entries := histOf(Album{Artist: "Slowdive", Title: "Souvlaki"})

	got := pickAlbum(pool, entries, drawFresh, seededRNG())
	if got.ReleaseID == 0 {
		t.Fatal("pickAlbum returned the zero Album instead of falling back to the full pool")
	}
}

func TestPickAlbumSinglePool(t *testing.T) {
	pool := poolOf(1)
	got := pickAlbum(pool, histOf(pool[0]), drawFresh, seededRNG())
	if got.ReleaseID != 1 {
		t.Errorf("got release %d, want 1", got.ReleaseID)
	}
}

func TestStaleWeightsRankNeverPlayedHighest(t *testing.T) {
	old := Album{ReleaseID: 1, Artist: "A", Title: "1"}
	recent := Album{ReleaseID: 2, Artist: "B", Title: "2"}
	never := Album{ReleaseID: 3, Artist: "C", Title: "3"}
	entries := histOf(old, recent)

	w := staleWeights([]Album{old, recent, never}, entries)
	if !(w[2] > w[0] && w[0] > w[1]) {
		t.Errorf("weights = %v, want never-played > long-unplayed > recent", w)
	}
	for i, x := range w {
		if x < 1 {
			t.Errorf("weights[%d] = %d, want at least 1 so nothing is unreachable", i, x)
		}
	}
}

func TestStaleWeightsEmptyHistoryIsUniform(t *testing.T) {
	w := staleWeights(poolOf(3), nil)
	if w[0] != w[1] || w[1] != w[2] {
		t.Errorf("weights = %v, want all equal for an empty history", w)
	}
}

func TestWeightedIndexRespectsWeights(t *testing.T) {
	counts := make([]int, 2)
	rng := seededRNG()
	for range 2000 {
		counts[weightedIndex([]int{1, 9}, rng)]++
	}
	if counts[1] <= counts[0]*3 {
		t.Errorf("counts = %v, want index 1 drawn far more often", counts)
	}
}
```

Add `"strconv"` to the test file's imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run 'TestPickAlbum|TestStaleWeights|TestWeightedIndex' -v`
Expected: FAIL — `undefined: pickAlbum`, `undefined: staleWeights`, `undefined: weightedIndex`.

- [ ] **Step 3: Write minimal implementation**

Append to `picker.go`, and add `"math/rand/v2"` to its imports:

```go
// pickAlbum chooses one album from pool, consulting entries per mode.
//
// pool must not be empty; the caller reports that case with its own message
// and exit code, because what to say about it depends on which filters were
// responsible.
func pickAlbum(pool []Album, entries []HistoryEntry, mode drawMode, rng *rand.Rand) Album {
	if mode == drawAny {
		return pool[rng.IntN(len(pool))]
	}

	// drawStale is drawFresh plus a bias, not an alternative to it, so the
	// anti-repeat guarantee holds whatever --draw says.
	candidates := excludeRecent(pool, entries)

	if mode == drawStale {
		return candidates[weightedIndex(staleWeights(candidates, entries), rng)]
	}
	return candidates[rng.IntN(len(candidates))]
}

// excludeRecent drops the recently played from pool, falling back to the whole
// pool when that would leave nothing.
//
// The fallback is reachable, not padding. antiRepeatWindow bounds the number
// of excluded *names*, and a history entry with no release ID matches every
// pressing of its title: three identically-titled pressings and one un-ID'd
// entry empty a pool with a window of one.
func excludeRecent(pool []Album, entries []HistoryEntry) []Album {
	recent := recentlyPlayed(entries, antiRepeatWindow(len(pool)))
	if len(recent) == 0 {
		return pool
	}

	var kept []Album
	for _, album := range pool {
		if !containsAlbum(recent, album) {
			kept = append(kept, album)
		}
	}
	if len(kept) == 0 {
		return pool
	}
	return kept
}

// staleWeights scores each candidate by how long it has gone unplayed,
// measured in picks rather than in time. A never-played record outranks every
// played one; among played ones the least recent wins. The lowest weight is 1,
// so nothing is ever unreachable, and an empty history makes every weight
// equal, which degenerates to a uniform draw.
//
// Linear rather than exponential: the records that would justify a sharper
// curve are the recently played ones, and excludeRecent has already removed
// them.
func staleWeights(candidates []Album, entries []HistoryEntry) []int {
	weights := make([]int, len(candidates))
	for i, album := range candidates {
		idx, played := lastPlayedIndex(entries, album)
		if !played {
			weights[i] = len(entries) + 1
			continue
		}
		weights[i] = len(entries) - idx
	}
	return weights
}

// weightedIndex draws an index from weights with probability proportional to
// each weight. Every weight must be at least 1, which staleWeights guarantees.
func weightedIndex(weights []int, rng *rand.Rand) int {
	total := 0
	for _, w := range weights {
		total += w
	}
	target := rng.IntN(total)
	for i, w := range weights {
		target -= w
		if target < 0 {
			return i
		}
	}
	// Unreachable while every weight is positive; returning the last index
	// beats panicking if that ever stops being true.
	return len(weights) - 1
}

// newRNG seeds a generator from the global source. pickAlbum takes an explicit
// *rand.Rand rather than calling rand.IntN so that tests can pin the sequence;
// this is where production gets a real one.
func newRNG() *rand.Rand {
	return rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
}
```

- [ ] **Step 4: Run the full suite**

Run: `go test ./...`
Expected: PASS — nothing else changed yet, so every existing test still holds.

- [ ] **Step 5: Commit**

```bash
git add picker.go picker_test.go
git commit -m "feat: add history-aware picking with fresh, any and stale draws"
```

---

### Task 5: `--unheard` and `--draw` flags

Wires the flags into `parseSelection` and the usage blocks. `runPick` and `runList` do not read them yet, so behavior is unchanged.

**Files:**
- Modify: `cli.go` (`selection` struct ~line 128, `parseSelection` ~line 152, the `pick` and `list` usage blocks in `init`)
- Test: `cli_test.go`, `global_flags_test.go`

**Interfaces:**
- Consumes: `parseDrawMode`, `drawFresh` (Task 1).
- Produces: `selection.unheard bool`, `selection.draw drawMode`.

- [ ] **Step 1: Write the failing test**

Append to `cli_test.go`:

```go
func TestParseSelectionUnheardFlag(t *testing.T) {
	for _, name := range []string{"pick", "list"} {
		cfg, err := parseSelection(name, []string{"--unheard"})
		if err != nil {
			t.Fatalf("parseSelection(%q): %v", name, err)
		}
		if !cfg.unheard {
			t.Errorf("%s: unheard = false, want true", name)
		}
	}
}

func TestParseSelectionDrawDefaultsToFresh(t *testing.T) {
	cfg, err := parseSelection("pick", nil)
	if err != nil {
		t.Fatalf("parseSelection: %v", err)
	}
	if cfg.draw != drawFresh {
		t.Errorf("draw = %v, want drawFresh", cfg.draw)
	}
}

func TestParseSelectionDrawFlag(t *testing.T) {
	cfg, err := parseSelection("pick", []string{"--draw", "stale"})
	if err != nil {
		t.Fatalf("parseSelection: %v", err)
	}
	if cfg.draw != drawStale {
		t.Errorf("draw = %v, want drawStale", cfg.draw)
	}
}

func TestParseSelectionRejectsBadDraw(t *testing.T) {
	if _, err := parseSelection("pick", []string{"--draw", "weighted"}); err == nil {
		t.Fatal("expected an error for an unknown --draw value")
	}
}

// Nothing is drawn by `list`, so the flag must not be quietly accepted there.
func TestParseSelectionRejectsDrawOnList(t *testing.T) {
	if _, err := parseSelection("list", []string{"--draw", "stale"}); err == nil {
		t.Fatal("expected list to reject --draw")
	}
}

// --unheard reads history, which favorite and unfavorite have no business
// loading. They take their flags from addFilterFlags, so this must stay true.
func TestParseFavoriteRejectsUnheardFlag(t *testing.T) {
	if _, err := parseFavorite("favorite", []string{"kind of blue", "--unheard"}); err == nil {
		t.Fatal("expected favorite to reject --unheard")
	}
}
```

Append to `global_flags_test.go`:

```go
// The commands that accept --unheard must document it, and the ones that do
// not must not claim to. Same guard as TestFilterFlagsAreDocumented, for a
// flag that is registered per-command rather than centrally.
func TestUnheardFlagIsDocumentedWhereAccepted(t *testing.T) {
	for _, name := range []string{"pick", "list"} {
		c := lookup(name)
		if c == nil {
			t.Fatalf("command %q not found", name)
		}
		if !strings.Contains(c.usage, "--unheard") {
			t.Errorf("%s usage does not mention --unheard", name)
		}
	}
	for _, name := range []string{"favorite", "unfavorite"} {
		c := lookup(name)
		if c == nil {
			t.Fatalf("command %q not found", name)
		}
		if strings.Contains(c.usage, "--unheard") {
			t.Errorf("%s documents --unheard but does not accept it", name)
		}
	}
}

func TestDrawFlagIsDocumentedOnPickOnly(t *testing.T) {
	if c := lookup("pick"); !strings.Contains(c.usage, "--draw") {
		t.Error("pick usage does not mention --draw")
	}
	if c := lookup("list"); strings.Contains(c.usage, "--draw") {
		t.Error("list documents --draw but does not accept it")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run 'TestParseSelectionUnheard|TestParseSelectionDraw|TestParseSelectionRejectsDraw|TestParseSelectionRejectsBadDraw|TestParseFavoriteRejectsUnheard|TestUnheardFlagIsDocumented|TestDrawFlagIsDocumented' -v`
Expected: FAIL — `cfg.unheard undefined`, `cfg.draw undefined`.

- [ ] **Step 3: Write minimal implementation**

In `cli.go`, extend `selection`:

```go
// selection is the parsed form of the flags shared by pick and list.
type selection struct {
	favoritesOnly bool
	// unheard restricts to albums that have never been picked. It is a
	// filter on the candidate set, like favoritesOnly, not a draw strategy
	// -- which is what lets `list` have it too.
	unheard bool
	// draw is how pick chooses from the candidates. It is meaningless for
	// list, which never sets it.
	draw   drawMode
	filter Filter
	color  colorMode
}
```

Replace `parseSelection` with:

```go
func parseSelection(name string, args []string) (selection, error) {
	fs, gf := newFlagSet(name)
	favoritesOnly := fs.Bool("favorites", false, "Restrict to favorites only")
	unheard := fs.Bool("unheard", false, "Restrict to albums never picked before")

	// --draw is registered only where something is actually drawn, so
	// `list --draw stale` fails as an unknown flag rather than being
	// accepted and silently ignored. Nothing else has to check for it.
	var draw *string
	if name != "list" {
		draw = fs.String("draw", "fresh", "How to draw a pick: any, fresh, or stale")
	}

	ff := addFilterFlags(fs)

	rest, err := parseInterspersed(fs, args)
	if err != nil {
		return selection{}, fmt.Errorf("%s: %w", name, err)
	}
	if len(rest) > 0 {
		return selection{}, fmt.Errorf("%s: unexpected argument %q", name, rest[0])
	}
	filter, err := ff.Filter()
	if err != nil {
		return selection{}, fmt.Errorf("%s: %v", name, err)
	}
	color, err := gf.mode()
	if err != nil {
		return selection{}, fmt.Errorf("%s: %v", name, err)
	}

	mode := drawFresh
	if draw != nil {
		m, err := parseDrawMode(*draw)
		if err != nil {
			return selection{}, fmt.Errorf("%s: %v", name, err)
		}
		mode = m
	}

	return selection{
		favoritesOnly: *favoritesOnly,
		unheard:       *unheard,
		draw:          mode,
		filter:        filter,
		color:         color,
	}, nil
}
```

In `init()`, replace the `pick` usage `Flags:` block:

```go
			usage: `Usage: disc-fortune pick [flags]

Prints one random album from your collection and records it in history.
This is what runs when you give no command at all.

By default a pick avoids the records you played most recently, so the same
album does not come back around twice in a week. --draw any turns that off.

Flags:
  --favorites      Pick from favorites only
  --unheard        Pick only from albums you have never picked
  --draw WHEN      How to draw: fresh (default), any, or stale.
                   fresh skips your recent picks; any ignores history
                   entirely; stale favors what you have left longest.
` + filterFlagHelp,
```

And the `list` usage `Flags:` block:

```go
			usage: `Usage: disc-fortune list [flags]

Prints every album matching the filters, with a count.

Flags:
  --favorites      List favorites only
  --unheard        List only albums you have never picked
` + filterFlagHelp,
```

- [ ] **Step 4: Run the full suite**

Run: `go test ./...`
Expected: PASS. The flags parse but nothing consumes them, so `pick` and `list` behave exactly as before — including every existing `--favorites` test.

- [ ] **Step 5: Commit**

```bash
git add cli.go cli_test.go global_flags_test.go
git commit -m "feat: add the --unheard and --draw flags"
```

---

### Task 6: Wire `runPick`, retire `randomAlbum`

Where the feature becomes real. `randomAlbum` goes away in the same task that stops calling it, so the tree is never left uncompilable.

**Files:**
- Modify: `main.go` (`runPick` ~line 112), `collection.go` (delete `randomAlbum` ~line 126 and the `math/rand/v2` import)
- Test: `main_test.go`, `collection_test.go` (delete `TestRandomAlbum` ~line 97), `picker_test.go`

**Interfaces:**
- Consumes: `pickAlbum`, `unheardOnly`, `newRNG` (Tasks 3-4); `selection.unheard`, `selection.draw` (Task 5).
- Produces: nothing new.

- [ ] **Step 1: Write the failing test**

Append to `main_test.go`:

```go
// mustSaveHistory and fixturePaths already exist in this file; runHelper
// re-execs the test binary as the real CLI so os.Exit is observable.

func TestPickUnheardExhaustedExitsOne(t *testing.T) {
	home := t.TempDir()
	collection, _, history := fixturePaths(home)

	albums := []Album{
		{ReleaseID: 1, Artist: "Slowdive", Title: "Souvlaki"},
		{ReleaseID: 2, Artist: "Ride", Title: "Nowhere"},
	}
	mustSaveCollection(t, collection, albums)
	mustSaveHistory(t, history, []HistoryEntry{
		{Album: albums[0], Timestamp: time.Now()},
		{Album: albums[1], Timestamp: time.Now()},
	})

	code, stdout, stderr := runHelperSplit(t, home, "pick", "--unheard")
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "already been played") {
		t.Errorf("stderr does not explain the exhaustion: %q", stderr)
	}
	if stdout != "" {
		t.Errorf("stdout should stay empty on failure, got %q", stdout)
	}
}

func TestPickUnheardReturnsTheUnplayedOne(t *testing.T) {
	home := t.TempDir()
	collection, _, history := fixturePaths(home)

	albums := []Album{
		{ReleaseID: 1, Artist: "Slowdive", Title: "Souvlaki"},
		{ReleaseID: 2, Artist: "Ride", Title: "Nowhere"},
	}
	mustSaveCollection(t, collection, albums)
	mustSaveHistory(t, history, []HistoryEntry{{Album: albums[0], Timestamp: time.Now()}})

	code, stdout, _ := runHelperSplit(t, home, "pick", "--unheard")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "Ride") {
		t.Errorf("stdout = %q, want the never-played album", stdout)
	}
}

func TestPickRejectsBadDrawValue(t *testing.T) {
	home := t.TempDir()
	collection, _, _ := fixturePaths(home)
	mustSaveCollection(t, collection, []Album{{ReleaseID: 1, Artist: "A", Title: "1"}})

	code, _, stderr := runHelperSplit(t, home, "pick", "--draw", "weighted")
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "--draw") {
		t.Errorf("stderr does not mention --draw: %q", stderr)
	}
}
```

Move `TestRandomAlbum` from `collection_test.go` into `picker_test.go`, rewritten against `pickAlbum`:

```go
// The uniform draw, which is what --draw any restores. This replaces the old
// TestRandomAlbum from collection_test.go.
func TestPickAlbumAnyReturnsValidAlbums(t *testing.T) {
	albums := []Album{
		{Artist: "A", Title: "1"},
		{Artist: "B", Title: "2"},
		{Artist: "C", Title: "3"},
	}

	seen := make(map[string]bool)
	rng := seededRNG()
	for range 100 {
		seen[pickAlbum(albums, nil, drawAny, rng).Key()] = true
	}
	if len(seen) < 2 {
		t.Errorf("expected multiple different albums over 100 picks, got %d unique", len(seen))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run 'TestPickUnheard|TestPickRejectsBadDraw' -v`
Expected: FAIL — `pick --unheard` currently ignores the flag and exits 0 with an already-played album.

- [ ] **Step 3: Write minimal implementation**

In `collection.go`, delete `randomAlbum` entirely and remove `"math/rand/v2"` from the import block.

In `main.go`, replace `runPick`:

```go
func runPick(cfg selection) {
	albums := selectAlbums(cfg)
	if len(albums) == 0 {
		fatal("No albums match the specified filters")
	}

	// History is read for the decision and then read again by addToHistory,
	// which takes its own lock. Deciding from a marginally stale history is
	// harmless, and it means no lock is held across the decision.
	entries, err := loadHistory(historyPath())
	if err != nil {
		fatal("Error loading history: %v", err)
	}

	if cfg.unheard {
		albums = unheardOnly(albums, entries)
		if len(albums) == 0 {
			fatal("Every album matching your filters has already been played.\n" +
				"Drop --unheard, or try `disc-fortune pick --draw stale` for whatever you have left longest.")
		}
	}

	album := pickAlbum(albums, entries, cfg.draw, newRNG())

	if err := addToHistory(historyPath(), album); err != nil {
		fatal("Error saving history: %v", err)
	}

	fmt.Println(formatAlbum(album, stdoutColor(cfg.color)))

	// Advisory, and therefore on stderr and only for a human at a terminal:
	// stdout is the data channel and must stay parseable.
	fmt.Fprint(os.Stderr, syncNotice(metaPath(), time.Now(), isTTY(os.Stderr)))
}
```

- [ ] **Step 4: Run the full suite**

Run: `go test ./...`
Expected: PASS. `--favorites` tests are untouched and must still pass.

- [ ] **Step 5: Verify the default actually changed**

Run:
```bash
go build -o /tmp/disc-fortune . && echo built
```
Expected: builds clean with no unused-import error from `collection.go`.

- [ ] **Step 6: Commit**

```bash
git add main.go collection.go collection_test.go main_test.go picker_test.go
git commit -m "feat: pick from history, avoiding what you played recently"
```

---

### Task 7: Wire `list --unheard`

**Files:**
- Modify: `main.go` (`runList` ~line 132)
- Test: `main_test.go`

**Interfaces:**
- Consumes: `unheardOnly` (Task 3), `selection.unheard` (Task 5).
- Produces: nothing new.

- [ ] **Step 1: Write the failing test**

Append to `main_test.go`:

```go
func TestListUnheardFiltersPlayedAlbums(t *testing.T) {
	home := t.TempDir()
	collection, _, history := fixturePaths(home)

	albums := []Album{
		{ReleaseID: 1, Artist: "Slowdive", Title: "Souvlaki"},
		{ReleaseID: 2, Artist: "Ride", Title: "Nowhere"},
	}
	mustSaveCollection(t, collection, albums)
	mustSaveHistory(t, history, []HistoryEntry{{Album: albums[0], Timestamp: time.Now()}})

	code, stdout, _ := runHelperSplit(t, home, "list", "--unheard")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.Contains(stdout, "Slowdive") {
		t.Errorf("stdout lists a played album: %q", stdout)
	}
	if !strings.Contains(stdout, "Ride") {
		t.Errorf("stdout is missing the unplayed album: %q", stdout)
	}
	if !strings.Contains(stdout, "1 album") {
		t.Errorf("stdout is missing the count: %q", stdout)
	}
}

func TestListUnheardExhaustedExitsOne(t *testing.T) {
	home := t.TempDir()
	collection, _, history := fixturePaths(home)

	album := Album{ReleaseID: 1, Artist: "Slowdive", Title: "Souvlaki"}
	mustSaveCollection(t, collection, []Album{album})
	mustSaveHistory(t, history, []HistoryEntry{{Album: album, Timestamp: time.Now()}})

	code, stdout, stderr := runHelperSplit(t, home, "list", "--unheard")
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "already been played") {
		t.Errorf("stderr does not explain the exhaustion: %q", stderr)
	}
	if stdout != "" {
		t.Errorf("stdout should stay empty on failure, got %q", stdout)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run TestListUnheard -v`
Expected: FAIL — `list --unheard` currently lists everything and exits 0.

- [ ] **Step 3: Write minimal implementation**

In `main.go`, replace `runList`:

```go
func runList(cfg selection) {
	albums := selectAlbums(cfg)

	// Only load history when it is actually needed: `list` has never
	// required a readable history.json and must not start now.
	if cfg.unheard && len(albums) > 0 {
		entries, err := loadHistory(historyPath())
		if err != nil {
			fatal("Error loading history: %v", err)
		}
		albums = unheardOnly(albums, entries)
		if len(albums) == 0 {
			fmt.Fprintln(os.Stderr, "Every album matching your filters has already been played.")
			os.Exit(1)
		}
	}

	out := formatList(albums, stdoutColor(cfg.color), false)
	if len(albums) == 0 {
		fmt.Fprint(os.Stderr, out)
		os.Exit(1)
	}
	fmt.Print(out)
}
```

- [ ] **Step 4: Run the full suite**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add main.go main_test.go
git commit -m "feat: let list show only what you have never played"
```

---

### Task 8: `withFileLock`

The lock primitive, standalone and unused. Closes nothing yet.

**Files:**
- Create: `lock.go`, `lock_unix.go`, `lock_other.go`
- Test: `lock_test.go`, `lock_unix_test.go`

**Interfaces:**
- Consumes: `configDirPerms` (existing, `collection.go`).
- Produces:
  - `func withFileLock(path string, fn func() error) error`
  - `func lockFD(f *os.File) error` / `func unlockFD(f *os.File) error` (build-tagged)

- [ ] **Step 1: Write the failing test**

Create `lock_test.go`:

```go
package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWithFileLockRunsFn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	ran := false
	if err := withFileLock(path, func() error {
		ran = true
		return nil
	}); err != nil {
		t.Fatalf("withFileLock: %v", err)
	}
	if !ran {
		t.Error("fn was not run")
	}
}

func TestWithFileLockPropagatesError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	sentinel := errors.New("boom")
	err := withFileLock(path, func() error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want the error fn returned", err)
	}
}

// The sidecar sits beside the data file, so the config directory has to exist
// before the lock can be taken -- including on the very first run, when
// nothing has been written yet.
func TestWithFileLockCreatesConfigDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh", "history.json")
	if err := withFileLock(path, func() error { return nil }); err != nil {
		t.Fatalf("withFileLock: %v", err)
	}
	if _, err := os.Stat(path + ".lock"); err != nil {
		t.Errorf("lock sidecar was not created: %v", err)
	}
}
```

Create `lock_unix_test.go`:

```go
//go:build unix

package main

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Two exclusive locks on one path must not be held at once. flock is per
// open file description, so separate os.OpenFile calls contend even inside a
// single process -- which is what makes this assertable without spawning
// subprocesses.
func TestWithFileLockSerializes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")

	var mu sync.Mutex
	inside, maxInside := 0, 0

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := withFileLock(path, func() error {
				mu.Lock()
				inside++
				if inside > maxInside {
					maxInside = inside
				}
				mu.Unlock()

				time.Sleep(2 * time.Millisecond)

				mu.Lock()
				inside--
				mu.Unlock()
				return nil
			})
			if err != nil {
				t.Errorf("withFileLock: %v", err)
			}
		}()
	}
	wg.Wait()

	if maxInside != 1 {
		t.Errorf("max concurrent holders = %d, want 1", maxInside)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run TestWithFileLock -v`
Expected: FAIL — `undefined: withFileLock`.

- [ ] **Step 3: Write minimal implementation**

Create `lock.go`:

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// lockFilePerms is what a lock sidecar is created with. It never holds data;
// only its existence and the kernel's lock on it matter.
const lockFilePerms = 0644

// withFileLock runs fn while holding an exclusive advisory lock on path, so a
// read-modify-write of a data file cannot interleave with another process
// doing the same thing. `sync`'s backfill rewrites history.json and
// favorites.json wholesale while a concurrent `pick` or `favorite` is
// appending to them; without this, one of the two writes is simply lost. That
// was cosmetic while history was only a log, and is not any more: a lost entry
// now changes which records the next pick will avoid.
//
// The lock is taken on a `<path>.lock` sidecar rather than on the data file,
// because every write replaces that file by rename: a lock held on the old
// inode would be invisible to the next process to open the path.
//
// Callers must not nest. Two exclusive locks on one path, through two file
// descriptors, deadlock even inside a single process, so withFileLock belongs
// at the outermost layer of a read-modify-write and never inside another one.
func withFileLock(path string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), configDirPerms); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, lockFilePerms)
	if err != nil {
		return fmt.Errorf("opening lock file: %w", err)
	}
	defer f.Close()

	if err := lockFD(f); err != nil {
		return fmt.Errorf("locking %s: %w", filepath.Base(path), err)
	}
	defer func() { _ = unlockFD(f) }()

	return fn()
}
```

Create `lock_unix.go`:

```go
//go:build unix

package main

import (
	"os"
	"syscall"
)

// lockFD takes an exclusive advisory lock, blocking until it is available.
//
// flock is preferred over an O_CREATE|O_EXCL sentinel file because the kernel
// releases it when the process exits: an interrupted run can never strand a
// lock that a later run would have to decide whether to break.
func lockFD(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

func unlockFD(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
```

Create `lock_other.go`:

```go
//go:build !unix

package main

import "os"

// lockFD does nothing where flock is unavailable. disc-fortune still works
// there; it simply has no protection against two copies writing the same data
// file at the same moment. Every development and release target is unix, so
// this exists to keep `go build` honest elsewhere rather than to be relied on.
func lockFD(f *os.File) error { return nil }

func unlockFD(f *os.File) error { return nil }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test . -run TestWithFileLock -v`
Expected: PASS, including `TestWithFileLockSerializes` with `max concurrent holders = 1`.

- [ ] **Step 5: Commit**

```bash
git add lock.go lock_unix.go lock_other.go lock_test.go lock_unix_test.go
git commit -m "feat: add an advisory file lock for data-file rewrites"
```

---

### Task 9: Apply the lock, and keep sidecars out of `migrate`

Wraps the four read-modify-writes, and stops `migrate` from carrying `.lock` sidecars to the new config directory.

**Files:**
- Modify: `history.go` (`addToHistory` ~line 51), `favorites.go` (`addFavorite` ~line 49, `removeFavorite` ~line 89), `backfill.go` (`runBackfill` ~line 137), `migrate.go` (the copy loop ~line 86)
- Test: `history_test.go`, `migrate_test.go`

**Interfaces:**
- Consumes: `withFileLock` (Task 8).
- Produces: `func isLockSidecar(name string) bool` in `lock.go`. `addFavorite` still returns `ErrAlreadyInFavorites` and `removeFavorite` still returns `ErrNotInFavorites`, unwrapped, so every `errors.Is` call site keeps working.

- [ ] **Step 1: Write the failing test**

Append to `history_test.go`:

```go
// Concurrent appends must not lose entries. This is the race the roadmap
// parked for this phase: it stopped being cosmetic when history started
// deciding what pick avoids.
func TestAddToHistoryConcurrentAppendsDoNotLose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")

	const writers = 8
	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			album := Album{ReleaseID: i + 1, Artist: "A", Title: strconv.Itoa(i + 1)}
			if err := addToHistory(path, album); err != nil {
				t.Errorf("addToHistory: %v", err)
			}
		}()
	}
	wg.Wait()

	entries, err := loadHistory(path)
	if err != nil {
		t.Fatalf("loadHistory: %v", err)
	}
	if len(entries) != writers {
		t.Errorf("history has %d entries, want %d", len(entries), writers)
	}
}
```

Add `"path/filepath"`, `"strconv"` and `"sync"` to `history_test.go`'s imports as needed.

Append to `migrate_test.go`:

```go
// Lock sidecars are runtime scaffolding, not the user's data. Copying them
// would inflate the "moved N files" count and litter the new directory.
func TestMigrateSkipsLockSidecars(t *testing.T) {
	from, to := t.TempDir(), filepath.Join(t.TempDir(), "xdg")

	if err := os.WriteFile(filepath.Join(from, "collection.json"), []byte("[]"), 0644); err != nil {
		t.Fatalf("writing collection: %v", err)
	}
	if err := os.WriteFile(filepath.Join(from, "history.json.lock"), nil, 0644); err != nil {
		t.Fatalf("writing lock sidecar: %v", err)
	}

	n, err := migrateConfig(from, to)
	if err != nil {
		t.Fatalf("migrateConfig: %v", err)
	}
	if n != 1 {
		t.Errorf("moved %d files, want 1 (the sidecar must not count)", n)
	}
	if _, err := os.Stat(filepath.Join(to, "history.json.lock")); !os.IsNotExist(err) {
		t.Error("the lock sidecar was copied to the destination")
	}
	// A skipped sidecar must not strand the legacy directory: it is ours, it
	// holds nothing, and leaving it keeps the old directory alive forever.
	if _, err := os.Stat(from); !os.IsNotExist(err) {
		t.Error("the legacy directory survived because a sidecar was left in it")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run 'TestAddToHistoryConcurrent|TestMigrateSkipsLock' -v`
Expected: FAIL — the history test loses entries (fewer than 8), and `migrateConfig` reports 2 files.

- [ ] **Step 3: Write minimal implementation**

In `history.go`, replace `addToHistory`:

```go
// addToHistory appends an album to history.
//
// The whole load-append-save runs under the file lock: `sync`'s backfill
// rewrites this same file, and without the lock one of the two writes is lost.
func addToHistory(path string, album Album) error {
	return withFileLock(path, func() error {
		entries, err := loadHistory(path)
		if err != nil {
			return err
		}
		entries = append(entries, HistoryEntry{
			Album:     album,
			Timestamp: time.Now(),
		})
		return saveHistory(path, entries)
	})
}
```

In `favorites.go`, wrap `addFavorite` and `removeFavorite`. This is a pure re-indent — **every existing comment moves across verbatim**; they record why `sameAlbum` is used, why the whole record is replaced rather than just the ID, and why removal stops at the first match. Losing them loses the Phase 2 reasoning. The full replacements:

```go
// addFavorite adds an album to favorites if not already present.
//
// Locked for the same reason as addToHistory: `sync`'s backfill rewrites
// favorites.json wholesale, and without the lock one of the two writes is lost.
func addFavorite(path string, album Album) error {
	return withFileLock(path, func() error {
		favorites, err := loadFavorites(path)
		if err != nil {
			return err
		}

		// sameAlbum rather than Key: two pressings of one title are two
		// favorites, but an entry written before release IDs existed is still
		// the same record as its freshly synced self.
		for i, fav := range favorites {
			if !sameAlbum(fav, album) {
				continue
			}

			// Replace a stored entry that predates release IDs with the one the
			// user just named, so naming a specific pressing actually resolves
			// an ambiguous favorite instead of reporting it forever. This is
			// safe rather than a guess: an un-ID'd favorite that can still be
			// re-favorited from the collection is necessarily an ambiguous one,
			// because a unique match would already have been stamped by the
			// backfill. The stored entry is therefore exactly the one the user
			// is now disambiguating, and either way they end up with one
			// favorite for that name.
			//
			// The whole record is replaced, not just the ID. Stamping the ID
			// alone would leave the entry asserting one pressing while carrying
			// another's year, label and catalogue number -- and permanently, as
			// backfillAlbums skips every entry that already has an ID.
			if album.ReleaseID != 0 && fav.ReleaseID == 0 {
				favorites[i] = album
				if err := saveFavorites(path, favorites); err != nil {
					return err
				}
			}
			return ErrAlreadyInFavorites
		}

		favorites = append(favorites, album)
		return saveFavorites(path, favorites)
	})
}

// removeFavorite removes the first favorite matching album.
//
// First match only, never every match: sameAlbum is not transitive, so an
// entry with no release ID matches every stored pressing sharing its name.
// Filtering all matches out would silently delete distinct pressings the
// user never named -- and `unfavorite` would still report removing one.
func removeFavorite(path string, album Album) error {
	return withFileLock(path, func() error {
		favorites, err := loadFavorites(path)
		if err != nil {
			return err
		}

		for i, fav := range favorites {
			if sameAlbum(fav, album) {
				// The three-index slice forces append to allocate rather than
				// alias favorites' backing array and clobber it in place.
				return saveFavorites(path, append(favorites[:i:i], favorites[i+1:]...))
			}
		}

		return ErrNotInFavorites
	})
}
```

`withFileLock` returns `fn`'s error unwrapped, so `errors.Is(err, ErrAlreadyInFavorites)` in `favoriteByQuery` and `favoriteLastPick` keeps working untouched.

In `backfill.go`, replace `runBackfill`. The error-reporting contract is unchanged — a favorites failure reports nothing because nothing landed, a history failure still reports what favorites did:

```go
func runBackfill(favPath, histPath string, collection []Album) (string, error) {
	var favRes backfillResult
	err := withFileLock(favPath, func() error {
		favorites, err := loadFavorites(favPath)
		if err != nil {
			return fmt.Errorf("loading favorites: %w", err)
		}
		filled, res := backfillAlbums(favorites, collection)
		favRes = res
		if res.Updated == 0 {
			return nil
		}
		if err := saveFavorites(favPath, filled); err != nil {
			return fmt.Errorf("saving favorites: %w", err)
		}
		return nil
	})
	if err != nil {
		// Nothing landed, so there is nothing to report.
		return "", err
	}

	var histRes backfillResult
	err = withFileLock(histPath, func() error {
		history, err := loadHistory(histPath)
		if err != nil {
			return fmt.Errorf("loading history: %w", err)
		}
		filled, res := backfillHistory(history, collection)
		histRes = res
		if res.Updated == 0 {
			return nil
		}
		if err := saveHistory(histPath, filled); err != nil {
			return fmt.Errorf("saving history: %w", err)
		}
		return nil
	})
	if err != nil {
		return backfillSummary(favRes, backfillResult{}), err
	}

	return backfillSummary(favRes, histRes), nil
}
```

The two locks are taken one after the other, never nested, and `runBackfill` uses `loadFavorites`/`saveFavorites` and `loadHistory`/`saveHistory` directly rather than `addFavorite` or `addToHistory` — which is what keeps it clear of the no-nesting rule.

In `migrate.go`, skip sidecars in the copy loop:

```go
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		// Lock sidecars are runtime scaffolding, not the user's data. Copying
		// them would inflate the moved-file count and litter the destination.
		if isLockSidecar(e.Name()) {
			continue
		}
		src := filepath.Join(from, e.Name())
```

Skipping alone is not enough: the removal phase ends with a best-effort
`os.Remove(from)` that succeeds only when the directory is empty, so a skipped
sidecar would strand the legacy directory forever. Unlike a stray user file, a
sidecar *is* ours to delete. Immediately before `_ = os.Remove(from)`, add:

```go
	// Sidecars were not copied, so they would keep the legacy directory alive
	// after everything real has moved out of it. They are ours, and they hold
	// nothing, so drop them rather than leave the directory stranded.
	for _, e := range entries {
		if e.Type().IsRegular() && isLockSidecar(e.Name()) {
			_ = os.Remove(filepath.Join(from, e.Name()))
		}
	}
	// Only succeeds if nothing else was in there; anything left is not ours
	// to delete.
	_ = os.Remove(from)
```

Add the predicate to `lock.go`, beside the thing that creates these files:

```go
// isLockSidecar reports whether name is one of the lock files withFileLock
// creates. Anything enumerating the config directory has to skip them.
func isLockSidecar(name string) bool {
	return strings.HasSuffix(name, ".lock")
}
```

Add `"strings"` to `lock.go`'s imports.

- [ ] **Step 4: Run the full suite, including the race detector**

Run: `go test ./...`
Expected: PASS

Run: `go test -race ./...`
Expected: PASS — the concurrency tests in Tasks 8 and 9 are the reason this matters.

- [ ] **Step 5: Commit**

```bash
git add history.go favorites.go backfill.go migrate.go history_test.go migrate_test.go
git commit -m "fix: stop a concurrent sync and pick losing each other's writes"
```

---

### Task 10: Release v2.3.0 "Discovery"

One commit, matching the shape of `9ce0c90`.

**Files:**
- Modify: `main.go:11` (`const version`), `README.md`, `docs/plans/2026-08-26-roadmap.md`
- Create: `RELEASE_NOTES_v2.3.0.md`

**Interfaces:**
- Consumes: everything above.
- Produces: nothing code-facing.

- [ ] **Step 1: Bump the version**

In `main.go`:

```go
const version = "2.3.0"
```

Minor, not patch: new user-visible capability, and plain `disc-fortune` changes behavior. `discogs.go:36` derives `userAgent` from this constant, so the bump also keeps the Discogs User-Agent accurate.

- [ ] **Step 2: Confirm nothing hardcoded the old version**

Run: `go test . -run TestRetry -v`
Expected: PASS. `discogs_retry_test.go:40` compares the header against the `version` constant rather than a literal, which is T2's anti-drift guarantee doing its job.

- [ ] **Step 3: Write `RELEASE_NOTES_v2.3.0.md`**

Follow the voice and structure of `RELEASE_NOTES_v2.2.1.md`. It must cover, in this order:

1. **The default changed** — prominently, the way v2.2.0 called out the collection-count jump. Plain `disc-fortune` now avoids the records you played most recently. `--draw any` restores the old behavior exactly, in one flag, for anyone scripting against it.
2. **`--unheard`** on `pick` and `list`, and that it exits 1 with an explanation when everything matching has been played.
3. **`--draw stale`** for whatever you have left longest.
4. **The window** — the last `min(10, pool/3)` distinct picks, measured against the *filtered* pool, so a narrow filter is never narrowed into nothing.
5. **`--favorites` is unchanged**, deliberately.
6. **The race fix**, and the new `*.lock` sidecars users will see in their config directory. Say plainly that these are scaffolding, safe to leave alone, and that `migrate` neither copies them nor leaves them behind.
7. **Known limitation** — a history entry with no release ID still acts as a wildcard for its name, so `--unheard` is conservative about pressings that share a title until a sync backfills them.

- [ ] **Step 4: Update `README.md`**

- Add `--unheard` and `--draw` to the usage examples near the existing filter examples.
- Add a "Discovery" subsection after "Listing" explaining the default anti-repeat, `--unheard`, and `--draw`.
- In the Data section, note the `*.lock` sidecars alongside the four data files.

- [ ] **Step 5: Update the roadmap**

In `docs/plans/2026-08-26-roadmap.md`, change the Phase 3 heading to `## Phase 3 — v2.3.0 "Discovery" ✅ SHIPPED (2026-08-31)` and add an in-session-decisions block in the style Phases 1 and 2 use, recording:

- **`--unheard` is a filter, `--draw` is a strategy.** T7's `--json` schema and T9's `stats` both need to know which is which.
- **Every history identity comparison is a backwards `sameAlbum` scan** through `lastPlayedIndex`. T9's "share of the collection ever picked" must use it rather than building a map.
- **An un-ID'd history entry is a name wildcard**, which is why `excludeRecent` has a fallback and `--unheard` is conservative.
- **`withFileLock` must never nest**, and new read-modify-writes on a data file belong inside it.
- **New `*.lock` sidecars** live in the config directory; anything enumerating that directory (as `migrate` does) has to skip them.

- [ ] **Step 6: Run the full suite one last time**

Run: `go test ./...`
Expected: PASS

Run: `go vet ./...`
Expected: no output.

- [ ] **Step 7: Commit**

```bash
git add main.go README.md RELEASE_NOTES_v2.3.0.md docs/plans/2026-08-26-roadmap.md
git commit -m "release: v2.3.0 \"Discovery\""
```
