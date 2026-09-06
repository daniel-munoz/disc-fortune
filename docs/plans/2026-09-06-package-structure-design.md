# Package structure — design

**Date:** 2026-09-06
**Status:** Approved in brainstorming; ready for an implementation plan.
**Covers:** Decoupling command logic from process exit, retiring the config
singleton, and extracting four `internal/` packages.

This is the first work after `docs/plans/2026-08-26-roadmap.md` closed with
v2.5.0. It adds no user-visible capability. Every observable behaviour —
stdout, stderr, exit codes, file formats, flag grammar — is byte-identical
before and after.

---

## Why

The starting question was whether a flat repository root still serves a
codebase this size. Measured at `0e73bf0`: 22 production files (5,007 lines)
and 27 test files (8,933 lines), all `package main`, 49 Go files in one
directory listing.

**Flatness is not itself the problem.** The files are already well-bounded —
`filter.go` filters, `picker.go` draws, `atomic.go` is one helper. What exists
is a clean decomposition that the compiler does not enforce. Three specific
couplings are the real defects, and none of them is fixed by creating
directories:

1. **Command logic is fused to process exit.** Every `run*` function calls
   `fatal()` (`main.go:16`), which is `os.Exit(1)` — 33 call sites in
   `main.go`, 9 in `sync.go`, 3 in `cli.go`, 1 each in `completion.go` and
   `migrate.go`. Output is equally hardcoded: 28 direct `os.Stdout` /
   `os.Stderr` / `fmt.Print` writes in `main.go`, 7 in `sync.go`, 2 in
   `migrate.go`. The cost is already paid and visible — `main_test.go:60-93`
   re-execs the test binary as a subprocess because neither the exit code nor
   the output can be observed in process. 64 tests depend on that harness.

2. **Config is an ambient singleton.** `activeConfig` (`collection.go:70`)
   backs `configDir()`, which backs `collectionPath()`, `favoritesPath()`,
   `historyPath()` and `metaPath()`. Nothing can move to a package while six
   files read a package-level global.

3. **`cli.go` and `main.go` are mutually referential.** The `commands` table's
   closures (`cli.go:761+`) call `runPick`/`runList`/…; `main()` calls
   `dispatch` (`cli.go:1050`).

The layout question is downstream of these. Fix them first and extraction
becomes mechanical; do the directory move first and the singleton and
`os.Exit` get dragged across new package boundaries, which is where import
cycles come from.

## Goals

- **Testability.** A command's behaviour — including its failure paths and its
  output — is observable without spawning a subprocess.
- **Navigability.** The root listing becomes scannable.

## Non-goals

- **Not a library.** Nothing outside this module imports this code. That is why
  packages are `internal/` and why no API is designed for external consumers.
- **Not compiler-enforced boundaries for their own sake.** Enforcement is a
  side effect, not the motivation.
- **Not a test-suite rewrite.** Test files move with the code they test and stay
  white-box (`package foo`, not `foo_test`). No test is restructured, and no
  assertion is edited to accommodate a refactor.
- **Not untangling `cli.go` ↔ `main.go`.** Explicitly deferred; see
  "Deliberately unmoved".
- **No new dependencies.** `go.mod` stays dependency-free.

---

## Part 1 — The `app` value

`run*` functions need three things they currently take ambiently: where data
lives, where output goes, and how to fail. One value replaces all three.

```go
// app carries what every command needs but no command's flags describe:
// where the data lives, and where its output goes. Constructed once in
// dispatch; passed to each command instead of being read from globals.
type app struct {
	loc    disc.Location
	stdout io.Writer
	stderr io.Writer
}

func (a app) collectionPath() string { return filepath.Join(a.loc.Dir, "collection.json") }
func (a app) favoritesPath() string  { return filepath.Join(a.loc.Dir, "favorites.json") }
func (a app) historyPath() string    { return filepath.Join(a.loc.Dir, "history.json") }
func (a app) metaPath() string       { return filepath.Join(a.loc.Dir, "meta.json") }
```

Every `run*` becomes a method returning `error`:

```go
func (a app) runPick(cfg selection) error   // was: func runPick(cfg selection)
```

`command.run` changes from `func(args []string)` to `func(app, []string) error`.
The table already stores function values, so this is a signature change, not a
restructure.

`fatal` survives in exactly one place — `dispatch`:

```go
if err := cmd.run(a, rest); err != nil {
	fmt.Fprintf(a.stderr, "disc-fortune: %v\n", err)
	os.Exit(1)
}
```

`activeConfig`, `configDir()` and the four package-level `*Path()` functions
are deleted.

### Why one struct rather than threading paths alone

Passing `Location` alone fixes the singleton but leaves output hardcoded, so
tests would still need a subprocess to see what a command printed. The 64
`runHelper` call sites exist for two reasons — observing exit codes and
capturing output — and a paths-only change addresses neither.

### Constraints this part must honour

- **Exit codes are 0 and 1 only.** `os.Exit(1)` at `cli.go:649` and
  `main.go:18,169,186,224,284`; success falls off the end of `dispatch`. A
  non-nil error maps to exit 1. No new codes.
- **`flag.ErrHelp` is a success, not an error.** It prints usage to **stdout**
  and exits **0** (`cli.go:633-648`). It must not flow into the generic error
  path, which would flip it to stderr and exit 1. `handleParseErr`'s existing
  bool return already encodes this distinction and is preserved.
- **Guidance text moves verbatim.** `loadCollectionOrExit` and
  `loadFavoritesOrExit` (`main.go:27-51`) currently write messages like
  "No collection found. Run `disc-fortune sync` …". These move into the
  returned error values with byte-identical wording; several tests assert on
  them.
- **Advisory output stays advisory.** `syncNotice` and `migrationNotice` go to
  stderr and never affect the exit code.
- **Deliberately excluded:** no `context.Context`, no interface over the
  storage layer, no functional-options constructor. `app` is a plain struct
  with three fields, built in one place.

---

## Part 2 — Package boundaries

Four `internal/` packages; everything else stays at the root.

| Package | Files | Prod LOC | Depends on |
|---|---|---|---|
| `internal/term` | *(split from `color.go`)* | ~90 | — |
| `internal/disc` | `collection` `favorites` `history` `meta` `filter` `backfill` `atomic` `lock` `lock_unix` `lock_other` `config` `migrate` | 1,517 | `term` |
| `internal/pick` | `picker` | 232 | `disc` |
| `internal/stats` | `stats` | 360 | `disc`, `pick`, `term` |
| `internal/discogs` | `discogs` | 365 | `disc` |
| *(root)* `package main` | `main` `cli` `completion` `sync` `json` `open`, plus `formatAlbum` | 2,443 | all of the above |

Root Go files drop from 49 to 16 (6 production, 10 test). With the release
notes moved, the root listing goes from ~65 entries to ~22.

### Cycles found, and how each breaks

The boundaries above were derived from a symbol-level dependency graph of all
22 production files, not from intuition. Two cycles exist.

**1. `disc` ↔ `filter`.** `favorites.go` calls `matchAlbums`, `matchedNone` and
`matchedMany` from `filter.go`; `filter.go` needs `Album` and `Album.Key` from
`collection.go`. A separate `filter` package deadlocks.

**Resolution: `filter.go` lives inside `disc`.** A `Filter` selects `Album`s —
domain logic about the same type, not a separate concern. Costs nothing and
keeps the move file-level.

**2. `disc` ↔ `color`.** `history.go` (`formatHistory`) uses `colorBoldCyan`,
`colorBoldWhite` and `colorReset`; `stats.go` uses `colorBoldWhite`,
`colorDim` and `colorReset` — all defined in `color.go`, whose `formatAlbum`
needs `Album`. So `color.go` can sit neither above nor below `disc`.

**Resolution: split `color.go`** — the only file split in this design.

- `internal/term` receives `colorMode`, `parseColorMode`, `useColor`, `isTTY`
  and the ANSI constants. It depends on nothing, so anything may import it.
- `formatAlbum` stays in `package main`; `main.go` is its only caller
  (`main.go:76,104,148`).

`color_test.go` splits along the same line.

*Rejected alternative:* fold all of `color.go` into `disc`. It breaks the cycle
and keeps every file whole, but puts terminal escape sequences inside the
domain package.

### Couplings the move forces

- **`discogs.go` reads `version`** (`main.go:13`) to build its User-Agent
  (`discogs.go:36`). This inverts: `newDiscogsClient` takes the user-agent
  string and `main` supplies it. The roadmap's T2 requires the UA never drift
  from `version` again — the existing assertion stays, retargeted at the new
  seam.
- **`stats.go` calls `plural()`**, which lives in `backfill.go`. Both land in
  `disc` so it compiles unchanged, but a pluralisation helper does not belong
  in a data-migration file. Move it beside the other formatting helpers.
- **`configDirPerms` and `collectionFilePerms`** are declared in
  `collection.go` and used by `lock.go`, `history.go`, `favorites.go`,
  `meta.go` and `migrate.go`. All land in `disc`; no action needed.

### Deliberately unmoved

`cli.go`, `main.go`, `completion.go`, `sync.go`, `json.go`, `open.go`.

`cli.go` ↔ `main.go` is mutually referential; `completion.go` reaches into ten
of `cli.go`'s unexported flag-registration functions; `sync.go` depends on both
`cli.go` (`syncConfig`) and `main.go` (`fatal`). Untangling that is a separate
job with its own risks, and Part 1 makes it easier rather than harder.

---

## Part 3 — Sequencing

Five commits, each green and independently shippable.

| # | Phase | Touches | Risk |
|---|---|---|---|
| 0 | `RELEASE_NOTES_v*.md` → `docs/releases/` | 12 files, no code | none |
| 1 | The `app` refactor (Part 1) | `main` `cli` `sync` `migrate` `completion` | highest |
| 2 | Extract `internal/term` | split `color.go`, `color_test.go` | low |
| 3 | Extract `internal/disc` | 12 files + 12 test files | medium; highest volume |
| 4 | Extract `pick`, `discogs`, then `stats` | 3 files + 4 test files | low |

Order is forced by the graph: `term` has no dependencies, `disc` depends only
on `term`, `pick` and `discogs` depend on `disc`, and `stats` needs `pick`.
Phase 1 precedes every extraction because the config singleton is what makes
`disc` unmovable today.

Test files move with their subject: `favorites_test.go` follows `favorites.go`
into `disc`, and so on. `main_test.go`, `cli_test.go`, `completion_test.go`,
`sync_test.go`, `json_test.go`, `open_test.go`, `global_flags_test.go`,
`env_conventions_test.go`, `version_test.go` and `progress_test.go` stay at the
root.

## Verification

**The subprocess harness is the safety net.** The 64 `runHelper` tests (49 in
`main_test.go`, 15 in `env_conventions_test.go`) drive the real binary — argv
in, stdout/stderr/exit code out. For a refactor promising byte-identical
behaviour, that is exactly the right guard, and it is already written. It stays
green through all five phases and is the primary evidence of correctness.

Retiring or converting any of those tests is **out of scope**. It becomes
possible after Part 1, and it is a separate decision made later, on its own
merits.

Every phase:

- `go test ./...` green.
- `gofmt -l .` empty and `go vet ./...` clean.
- No test assertion edited to make a phase pass.

After phase 3 additionally:

- `GOOS=windows go build ./...` and `GOOS=linux go build ./...`. The
  `lock_unix.go` / `lock_other.go` build-tag pair moves in that phase and its
  breakage is invisible on macOS.

**Differential check.** Build the binary at `HEAD` before starting and keep it.
After each phase, diff old against new across a fixed invocation set —
`pick --json` under a seeded RNG, `list`, `history`, `stats`, `stats --json`,
`help`, and each error path — comparing stdout, stderr and exit code. This is
what catches the error-message drift phase 1 is most likely to introduce.

## Costs, stated plainly

- **~60 identifiers need exporting.** `loadFavorites` → `disc.LoadFavorites`,
  `sameAlbum` → `disc.SameAlbum`, `withFileLock` → `disc.WithFileLock`,
  `errNoCollection` → `disc.ErrNoCollection`, and so on. Tests moving *into* a
  package keep using unexported names and are unaffected; the churn lands in
  `main.go`, `sync.go`, `json.go` and `cli.go`. It is mechanical and
  compiler-checked, but it is the bulk of the diff, and phase 3's commit will
  be large however it is sliced.
- **Naming needs a pass per package.** `stats.Stats` and `pick.PickAlbum`
  stutter; `disc.Album` and `disc.Filter` read well. Each package's exported
  names are settled during its own phase rather than mechanically prefixed.
- **The existing export convention is mixed.** `ErrAlreadyInFavorites` and
  `FavoriteAdded` are exported today with no boundary to justify it.
  Regularising this is an improvement, and also more diff.
- **The layout is inconsistent between phases 2 and 4** — some code under
  `internal/`, some flat. Each phase ships green, so this is tolerable, but it
  is real.

## Documentation to update

- `README.md` — the project-layout description, if it names the flat root.
- Future implementation plans inherit a global constraint reading "Everything
  is `package main` in the repository root. There is no `src/`" (e.g.
  `docs/plans/2026-09-03-json-output.md`). That sentence becomes false after
  phase 3. Existing plan documents are historical records and are **not**
  edited; new ones must not copy it.
- `RELEASE_NOTES` for the next release: no user-visible change, and say so.
