# Package Structure — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Decouple command logic from process exit and ambient config, then extract four `internal/` packages — without changing a single byte of observable behaviour.

**Architecture:** An `app` value carries the three things commands take ambiently today (data location, output writers, failure mode); `run*` functions become methods on it returning `error`, with `os.Exit` surviving only in `dispatch`. That removes the config singleton, which is what currently pins every file to `package main`. Extraction then follows the dependency graph bottom-up: `term` (no dependencies), `disc` (domain + persistence), then `pick`, `discogs` and `stats`.

**Tech Stack:** Go 1.24.3, standard library only. No new dependencies.

**Spec:** [`docs/plans/2026-09-06-package-structure-design.md`](2026-09-06-package-structure-design.md)

## How this plan differs from a feature plan

**This is a refactor. There is no new behaviour to test-drive.** The normal
red-green cycle does not apply, and faking it would be dishonest. The
equivalent discipline here is:

- The existing suite is the specification. It stays green at every step.
- **No test assertion may be edited to make a task pass.** If a test fails, the
  refactor is wrong, not the test. The only permitted test edits are mechanical:
  moving a file between packages, and renaming an identifier that was renamed in
  production code.
- Task 1 builds a differential harness that compares the real binary's
  stdout/stderr/exit code before and after each task. That is the red-green
  substitute, and it runs at the end of every task.

Two tasks (5 and 11) *do* add new tests, because they introduce a genuinely new
capability — calling a command in process and capturing its output. Those follow
a real red-green cycle.

## Global Constraints

- Module is `github.com/daniel-munoz/disc-fortune/v2`, Go 1.24.3. **No third-party dependencies.** `go.mod` must stay dependency-free.
- **Every observable behaviour is byte-identical before and after every task:** stdout, stderr, exit codes, file formats, flag grammar.
- **Exit codes are 0 and 1 only.** A non-nil error from a command maps to exit 1. No new codes.
- **`flag.ErrHelp` is a success**: usage to **stdout**, exit **0**. It must never flow into the generic error path.
- **No test assertion is edited to accommodate a refactor.** Moving a test file and renaming a renamed identifier are the only permitted test changes, except in Tasks 5 and 11 which add new tests.
- `gofmt -l .` must be empty and `go vet ./...` clean before every commit.
- Run tests with `go test ./...` from the repository root.
- Test files stay **white-box** — `package disc`, never `package disc_test`.
- Work happens on the `refactor` branch. One commit per task.

---

## File Structure

**End state.** Root keeps `package main`; four packages under `internal/`.

| Path | Responsibility |
|---|---|
| `main.go` | `main()`, the `app` type, the `run*` methods, `formatAlbum` |
| `cli.go` | Flag registration, parsing, the `commands` table, `dispatch` |
| `completion.go` | Shell completion generation |
| `sync.go` | The sync orchestration (Discogs → disk) |
| `json.go` | Wire types for `--json` |
| `open.go` | Browser launch planning |
| `format.go` | *(new)* `formatAlbum`, split out of `color.go` |
| `internal/term/` | `colorMode`, `parseColorMode`, `useColor`, `isTTY`, ANSI constants. **No dependencies.** |
| `internal/disc/` | `Album`, `HistoryEntry`, `Meta`, `Filter`, `Location`, all persistence, locking, atomic writes, backfill, migration |
| `internal/pick/` | Draw strategies and history-aware candidate selection |
| `internal/stats/` | `Stats` computation and rendering |
| `internal/discogs/` | The Discogs HTTP client |
| `scripts/behaviour-diff.sh` | *(new)* Differential harness (Task 1) |
| `docs/releases/` | *(new)* Relocated release notes |

**Test files follow their subject.** Staying at root: `main_test.go`,
`cli_test.go`, `completion_test.go`, `sync_test.go`, `json_test.go`,
`open_test.go`, `global_flags_test.go`, `env_conventions_test.go`,
`version_test.go`, `progress_test.go`, and a new `format_test.go`.

---

### Task 1: Differential-behaviour harness

Everything after this task depends on being able to prove behaviour did not
change. Build that first.

**Files:**
- Create: `scripts/behaviour-diff.sh`

**Interfaces:**
- Produces: `scripts/behaviour-diff.sh <baseline-binary>` — exits 0 when the freshly built binary matches the baseline across every probe, non-zero with a diff otherwise.

- [ ] **Step 1: Build and stash the baseline binary**

```bash
cd /Users/danielm/personal-projects/disc-fortune/refactor
mkdir -p .refactor-baseline
go build -o .refactor-baseline/disc-fortune-base .
echo '.refactor-baseline/' >> .gitignore
```

- [ ] **Step 2: Write the harness**

Create `scripts/behaviour-diff.sh`:

```bash
#!/usr/bin/env zsh
# Compares the current tree's binary against a baseline build across a fixed
# set of invocations, asserting stdout, stderr and exit code are identical.
#
# Usage: scripts/behaviour-diff.sh .refactor-baseline/disc-fortune-base
set -uo pipefail

BASE="${1:?usage: behaviour-diff.sh <baseline-binary>}"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

go build -o "$WORK/new" . || { echo "BUILD FAILED"; exit 1; }

# A populated config dir, byte-identical for both binaries.
FIXTURE="$WORK/home"
mkdir -p "$FIXTURE/.config/disc-fortune"
cat > "$FIXTURE/.config/disc-fortune/collection.json" <<'JSON'
[
  {"release_id":1,"artist":"Miles Davis","title":"Kind of Blue","year":1959,"label":"Columbia","catno":"CL 1355","genres":["Jazz"],"formats":["Vinyl","LP"]},
  {"release_id":2,"artist":"Alice Coltrane","title":"Journey in Satchidananda","year":1971,"label":"Impulse!","catno":"AS-9203","genres":["Jazz"],"formats":["Vinyl","LP"]},
  {"release_id":3,"artist":"Parliament","title":"Mothership Connection","year":1975,"label":"Casablanca","catno":"NBLP 7022","genres":["Funk"],"formats":["Vinyl","LP"]}
]
JSON
cat > "$FIXTURE/.config/disc-fortune/favorites.json" <<'JSON'
[{"release_id":2,"artist":"Alice Coltrane","title":"Journey in Satchidananda","year":1971,"label":"Impulse!","catno":"AS-9203","genres":["Jazz"],"formats":["Vinyl","LP"]}]
JSON
cat > "$FIXTURE/.config/disc-fortune/history.json" <<'JSON'
[{"album":{"release_id":1,"artist":"Miles Davis","title":"Kind of Blue","year":1959,"label":"Columbia","catno":"CL 1355","genres":["Jazz"],"formats":["Vinyl","LP"]},"timestamp":"2026-09-01T10:00:00Z"}]
JSON
cat > "$FIXTURE/.config/disc-fortune/meta.json" <<'JSON'
{"synced_at":"2026-09-01T10:00:00Z"}
JSON

# Probes: each line is one argv. Deterministic commands only -- `pick` is
# excluded from the identical-output set because it draws at random; it is
# covered by exit-code-only probes below.
PROBES=(
  "list"
  "list --json"
  "list --genre jazz"
  "list --genre nope"
  "list --favorites"
  "history"
  "history --json"
  "history 1"
  "stats"
  "stats --json"
  "stats --favorites"
  "stats --genre nope"
  "help"
  "help pick"
  "help stats"
  "version"
  "--color=always list"
  "--color=never list"
  "open --print --release-id 1"
  "open --print --release-id 999"
  "completion bash"
  "completion zsh"
  "completion fish"
  "bogus-command"
  "list --bogus-flag"
  "favorite"
  "unfavorite --release-id 999"
)

FAIL=0
run_one() {  # $1=binary $2=argv-string $3=outfile
  local home_copy="$WORK/run-home"
  rm -rf "$home_copy"; cp -R "$FIXTURE" "$home_copy"
  ( cd "$home_copy" && env -i HOME="$home_copy" PATH="$PATH" \
      "$1" ${=2} ) >"$3.out" 2>"$3.err"
  echo "$?" > "$3.code"
}

for p in "${PROBES[@]}"; do
  run_one "$BASE"      "$p" "$WORK/base"
  run_one "$WORK/new"  "$p" "$WORK/new_"
  for ext in out err code; do
    if ! diff -q "$WORK/base.$ext" "$WORK/new_.$ext" >/dev/null; then
      echo "MISMATCH [$ext] for: disc-fortune $p"
      diff -u "$WORK/base.$ext" "$WORK/new_.$ext" | sed 's/^/    /'
      FAIL=1
    fi
  done
done

# `pick` is random: assert only that the exit code and stderr shape match.
for p in "pick" "pick --favorites" "pick --genre nope" "pick --json"; do
  run_one "$BASE"     "$p" "$WORK/base"
  run_one "$WORK/new" "$p" "$WORK/new_"
  if ! diff -q "$WORK/base.code" "$WORK/new_.code" >/dev/null; then
    echo "MISMATCH [exit code] for: disc-fortune $p"
    FAIL=1
  fi
done

if [ "$FAIL" -eq 0 ]; then echo "behaviour-diff: OK (${#PROBES[@]} probes + 4 pick probes)"; fi
exit "$FAIL"
```

The `${=2}` word-splitting is zsh-only, which is why the shebang is zsh. Make it
executable:

```bash
chmod +x scripts/behaviour-diff.sh
```

- [ ] **Step 3: Verify the harness passes against an unchanged tree**

Run: `scripts/behaviour-diff.sh .refactor-baseline/disc-fortune-base`
Expected: `behaviour-diff: OK (27 probes + 4 pick probes)` and exit 0.

If it reports a mismatch here, the harness is wrong (nothing has changed yet) —
fix it before proceeding. A likely cause is a probe that reads the wall clock;
remove that probe rather than loosening the comparison.

- [ ] **Step 4: Confirm the suite is green before any change**

Run: `go test ./... && gofmt -l . && go vet ./...`
Expected: tests PASS, `gofmt -l .` prints nothing, vet silent.

- [ ] **Step 5: Commit**

```bash
git add scripts/behaviour-diff.sh .gitignore
git commit -m "test: add differential behaviour harness for the refactor

Compares the built binary against a pre-refactor baseline across 31
invocations, asserting stdout, stderr and exit code are identical. This
is the safety net for the package-structure refactor."
```

---

### Task 2: Move the release notes out of the root

**Files:**
- Move: `RELEASE_NOTES_v*.md` (12 files) → `docs/releases/`

**Interfaces:**
- Consumes: nothing. Produces: nothing. No Go code is touched.

- [ ] **Step 1: Check for references before moving**

```bash
grep -rn "RELEASE_NOTES" --include='*.md' --include='*.go' --include='*.yml' --include='*.yaml' . | grep -v '^./RELEASE_NOTES'
```

Record what this prints. If `.github/workflows/` or `README.md` reference the
files by path, those references are updated in Step 3. If it prints nothing, skip
Step 3.

- [ ] **Step 2: Move them with git**

```bash
mkdir -p docs/releases
git mv RELEASE_NOTES_v1.0.0.md RELEASE_NOTES_v1.1.0.md RELEASE_NOTES_v1.2.0.md \
       RELEASE_NOTES_v1.3.0.md RELEASE_NOTES_v2.0.0.md RELEASE_NOTES_v2.1.0.md \
       RELEASE_NOTES_v2.1.1.md RELEASE_NOTES_v2.2.0.md RELEASE_NOTES_v2.2.1.md \
       RELEASE_NOTES_v2.3.0.md RELEASE_NOTES_v2.4.0.md RELEASE_NOTES_v2.5.0.md \
       docs/releases/
```

- [ ] **Step 3: Update any references found in Step 1**

Edit each file Step 1 listed, changing `RELEASE_NOTES_vX.Y.Z.md` to
`docs/releases/RELEASE_NOTES_vX.Y.Z.md`. If Step 1 printed nothing, do nothing.

- [ ] **Step 4: Verify nothing broke**

Run: `go test ./... && scripts/behaviour-diff.sh .refactor-baseline/disc-fortune-base`
Expected: tests PASS, harness OK.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "docs: move release notes to docs/releases/

Twelve RELEASE_NOTES_v*.md files were a fifth of the root listing. No
code change."
```

---

### Task 3: Replace the config singleton with an `app` value

The largest behavioural risk in the plan is Task 4; this task is deliberately
separated from it so the two can be reviewed apart.

**Files:**
- Create: `app.go`
- Modify: `collection.go` (delete `activeConfig`, `configDir`, `initConfig`, `collectionPath`, `loadCollection`, `saveCollection`)
- Modify: `favorites.go:18-20`, `history.go:18-20`, `meta.go:26-28` (delete the three `*Path()` functions)
- Modify: `cli.go:1050-1064` (`dispatch` builds the `app`), and the 13 `commands` table entries
- Modify: `main.go` (all `run*` become methods), `sync.go`, `migrate.go`

**Interfaces:**
- Produces:
  - `type app struct { loc configLocation }`
  - `func (a app) collectionPath() string`, `favoritesPath()`, `historyPath()`, `metaPath()`
  - `command.run` becomes `func(a app, args []string)` — still no error return; Task 4 adds that.
  - `func newApp(getenv func(string) string, homeDir func() (string, error)) (app, error)`

- [ ] **Step 1: Create `app.go`**

```go
package main

import "path/filepath"

// app carries what every command needs but no command's flags describe:
// where the data lives. Constructed once in dispatch and passed to each
// command, replacing the package-level activeConfig that used to back the
// path helpers.
type app struct {
	loc configLocation
}

// newApp resolves the config location for this run. getenv and homeDir are
// injected so the decision can be tested without touching the real
// environment, matching resolveConfigDir's existing contract.
func newApp(getenv func(string) string, homeDir func() (string, error)) (app, error) {
	loc, err := resolveConfigDir(getenv, homeDir)
	if err != nil {
		return app{}, err
	}
	return app{loc: loc}, nil
}

func (a app) collectionPath() string { return filepath.Join(a.loc.Dir, "collection.json") }
func (a app) favoritesPath() string  { return filepath.Join(a.loc.Dir, "favorites.json") }
func (a app) historyPath() string    { return filepath.Join(a.loc.Dir, "history.json") }
func (a app) metaPath() string       { return filepath.Join(a.loc.Dir, "meta.json") }
```

- [ ] **Step 2: Delete the singleton and the dead no-arg helpers**

In `collection.go`, delete `activeConfig` (line 70), `configDir` (75-77),
`initConfig` (81-88), `collectionPath` (90-92), `loadCollection` (94-96) and
`saveCollection` (110-112).

`loadCollection()` has **no callers** — it is dead code, verified by
`grep -n 'loadCollection()' *.go`. `saveCollection` has exactly one caller,
`sync.go:63`, rewritten in Step 4.

In `favorites.go`, `history.go` and `meta.go`, delete `favoritesPath`,
`historyPath` and `metaPath` respectively. All four `*Path` names now live on
`app`.

- [ ] **Step 3: Turn the `run*` functions into methods**

In `main.go`, change each of these to a method on `app` and replace every
bare path call with the method:

```go
func (a app) runPick(cfg selection)              // was runPick(cfg selection)
func (a app) runList(cfg selection)
func (a app) runHistory(cfg historyConfig)
func (a app) runStats(cfg statsConfig)
func (a app) runFavorite(cfg favoriteConfig)
func (a app) runUnfavorite(cfg favoriteConfig)
func (a app) runOpen(cfg openConfig)
func (a app) favoriteLastPick()
func (a app) unfavoriteLastPick()
func (a app) loadCollectionOrExit() []Album
func (a app) loadFavoritesOrExit() []Album
func (a app) selectAlbums(cfg selection) []Album
```

Every `collectionPath()` becomes `a.collectionPath()`, and so on for the other
three. Internal calls gain the receiver: `loadCollectionOrExit()` becomes
`a.loadCollectionOrExit()`, `favoriteLastPick()` becomes `a.favoriteLastPick()`.

- [ ] **Step 4: Do the same in `sync.go` and `migrate.go`**

```go
func (a app) runSync(cfg syncConfig)
func (a app) runFolders()
func (a app) runMigrate()
```

`sync.go:61` `loadCollectionFrom(collectionPath())` → `loadCollectionFrom(a.collectionPath())`.
`sync.go:63` `saveCollection(albums)` → `saveCollectionTo(a.collectionPath(), albums)`.
`sync.go:69,79` likewise take `a.metaPath()`, `a.favoritesPath()`, `a.historyPath()`.
`migrate.go` reads `activeConfig` at line ~142 → `a.loc`.

`runFolders` and `runCompletion` need no paths, but `runFolders` becomes a
method anyway so every table entry has one shape. `runCompletion` stays a plain
function — it is in `completion.go`, needs no config, and its command entry does
not set `needsConfig`.

- [ ] **Step 5: Change the command table and `dispatch`**

In `cli.go`, `command.run` becomes `func(a app, args []string)`. Each of the 13
entries gains the parameter and threads it through, e.g.:

```go
run: func(a app, args []string) {
    cfg, err := parseSelection("pick", args)
    if handleParseErr("pick", err) {
        return
    }
    a.runPick(cfg)
},
```

For `completion`, whose handler calls a non-method, the entry becomes
`run: func(a app, args []string) { ... runCompletion(shell) }` — `a` is unused
there, which is fine and expected.

`dispatch` becomes:

```go
func dispatch(args []string) {
	cmd, rest, err := resolve(args)
	if err != nil {
		fatal("disc-fortune: %v", err)
	}
	// Resolved once, here, so every path helper on app can be infallible.
	// A failure is only fatal for the commands that actually need it.
	a, cfgErr := newApp(os.Getenv, os.UserHomeDir)
	if cfgErr != nil {
		if cmd.needsConfig {
			fatal("disc-fortune: %v", cfgErr)
		}
	} else if cmd.needsConfig {
		fmt.Fprint(os.Stderr, migrationNotice(a.loc, a.metaPath(), isTTY(os.Stderr)))
	}
	cmd.run(a, rest)
}
```

Note the ordering is unchanged: config resolution still happens **after**
command resolution and still only fails for `needsConfig` commands. Preserving
that is what keeps `help`, `version` and `folders` working on a machine with no
usable home directory.

- [ ] **Step 6: Verify**

Run: `go build ./... && go test ./... && gofmt -l . && go vet ./...`
Expected: builds, tests PASS, gofmt silent, vet silent.

Run: `scripts/behaviour-diff.sh .refactor-baseline/disc-fortune-base`
Expected: `behaviour-diff: OK`.

If `env_conventions_test.go` fails, the likely cause is config resolution
having moved relative to command resolution in `dispatch`. Re-read Step 5.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor: replace the config singleton with an app value

activeConfig and the four package-level *Path helpers are gone; an app
value built once in dispatch carries the resolved location, and every
run* function is a method on it. Also deletes loadCollection(), which
had no callers.

No behaviour change."
```

---

### Task 4: `run*` returns `error`; `fatal` survives only in `dispatch`

**Files:**
- Modify: `main.go` (all `run*` methods, `loadCollectionOrExit`, `loadFavoritesOrExit`), `sync.go`, `migrate.go`, `cli.go` (table + `dispatch`)

**Interfaces:**
- Consumes: `app` and the `run*` methods from Task 3.
- Produces:
  - `command.run` becomes `func(a app, args []string) error`
  - `func (a app) collection() ([]Album, error)` replacing `loadCollectionOrExit`
  - `func (a app) favorites() ([]Album, error)` replacing `loadFavoritesOrExit`
  - Package-level sentinels carrying the former `fatal` guidance text.

- [ ] **Step 1: Move the guidance text into error values**

The wording must be **byte-identical** — tests assert on it. In `main.go`,
replace `loadCollectionOrExit`/`loadFavoritesOrExit` with:

```go
// The guidance these carry used to be printed by loadCollectionOrExit and
// loadFavoritesOrExit immediately before os.Exit(1). It is now attached to
// the error so dispatch can print it at the one remaining exit point. The
// wording is asserted by tests and must not drift.
var (
	errNoCollectionGuidance    = errors.New("No collection found. Run `disc-fortune sync` to fetch your Discogs collection.")
	errEmptyCollectionGuidance = errors.New("Collection is empty. Run `disc-fortune sync` to fetch your Discogs collection.")
	errNoFavoritesGuidance     = errors.New("No favorites yet. Use `disc-fortune favorite` after a pick you like.")
)

func (a app) collection() ([]Album, error) {
	albums, err := loadCollectionChecked(a.collectionPath())
	switch {
	case errors.Is(err, errNoCollection):
		return nil, errNoCollectionGuidance
	case errors.Is(err, errEmptyCollection):
		return nil, errEmptyCollectionGuidance
	case err != nil:
		return nil, fmt.Errorf("Error loading collection: %v", err)
	}
	return albums, nil
}

func (a app) favorites() ([]Album, error) {
	favorites, err := loadFavoritesChecked(a.favoritesPath())
	switch {
	case errors.Is(err, errNoFavorites):
		return nil, errNoFavoritesGuidance
	case err != nil:
		return nil, fmt.Errorf("Error loading favorites: %v", err)
	}
	return favorites, nil
}
```

**Critical detail:** `fatal` prints `format+"\n"` with **no prefix**, whereas
`dispatch`'s existing `fatal("disc-fortune: %v", err)` adds one. To keep stderr
byte-identical, the error printer in Step 4 must **not** add a prefix, and the
two `dispatch`-level messages that already carry `disc-fortune: ` keep it in
their own text. Verify with the harness; the `bogus-command` and `favorite`
probes cover both shapes.

- [ ] **Step 2: Convert each `run*` to return `error`**

Every `fatal(...)` inside a `run*` becomes `return fmt.Errorf(...)` with the
same format string and arguments. Every `os.Exit(1)` preceded by a
`fmt.Fprintln(os.Stderr, msg)` becomes `return errors.New(msg)` with the same
message. Signatures:

```go
func (a app) runPick(cfg selection) error
func (a app) runList(cfg selection) error
func (a app) runHistory(cfg historyConfig) error
func (a app) runStats(cfg statsConfig) error
func (a app) runFavorite(cfg favoriteConfig) error
func (a app) runUnfavorite(cfg favoriteConfig) error
func (a app) runOpen(cfg openConfig) error
func (a app) favoriteLastPick() error
func (a app) unfavoriteLastPick() error
func (a app) runSync(cfg syncConfig) error
func (a app) runFolders() error
func (a app) runMigrate() error
func runCompletion(shell string) error
```

Example, `runList`'s empty-match path (`main.go:186-190`):

```go
	out := formatList(albums, stdoutColor(cfg.color), false)
	if len(albums) == 0 {
		return errors.New(strings.TrimSuffix(out, "\n"))
	}
	fmt.Print(out)
	return nil
```

The `TrimSuffix` matters: `formatList` returns a trailing newline and the error
printer adds one. Without it the empty-list case grows a blank line and the
harness will catch it.

- [ ] **Step 3: `reportAmbiguous` and other helpers that exit**

`reportAmbiguous` (`main.go:~280`) prints candidates then exits 1. It becomes:

```go
func reportAmbiguous(matches []Album, color colorMode) error
```

returning the assembled message rather than printing-then-exiting. Its callers
`return reportAmbiguous(...)`.

- [ ] **Step 4: Update the table and `dispatch`**

`command.run` becomes `func(a app, args []string) error`. Each entry returns
what it called:

```go
run: func(a app, args []string) error {
    cfg, err := parseSelection("pick", args)
    if handleParseErr("pick", err) {
        return nil   // help and usage errors are handled inside handleParseErr
    }
    return a.runPick(cfg)
},
```

**`handleParseErr` is not changed.** It keeps its bool return, keeps printing
usage to stdout for `flag.ErrHelp`, and keeps its own `os.Exit(1)` at
`cli.go:649` for a genuine usage error. Folding it into the `error` path would
turn `--help` into exit 1 on stderr. Returning `nil` after it is correct: it has
already decided the process's fate.

`dispatch` gains the single exit point:

```go
	if err := cmd.run(a, rest); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
```

- [ ] **Step 5: Verify — this is the task most likely to drift**

Run: `go test ./... && gofmt -l . && go vet ./...`
Expected: PASS / silent / silent.

Run: `scripts/behaviour-diff.sh .refactor-baseline/disc-fortune-base`
Expected: `behaviour-diff: OK`.

Run: `grep -n 'fatal(' *.go | grep -v _test`
Expected: hits **only** in `main.go` (the definition and, at most, `main()`'s
own use) and `cli.go`'s `dispatch`/`handleParseErr`. Any remaining `fatal` in
`runPick`…`runMigrate` means Step 2 is incomplete.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: run* returns error instead of exiting

Command logic no longer calls os.Exit. dispatch owns the single exit
point and maps a non-nil error to stderr + exit 1. flag.ErrHelp keeps
its own path: usage to stdout, exit 0.

Guidance text moves verbatim into error values. No behaviour change."
```

---

### Task 5: Inject the output writers

This task adds real new tests, because it is the one that delivers the
plan's stated goal: a command's output observable in process.

**Files:**
- Modify: `app.go` (add writers), `main.go`, `sync.go`, `migrate.go`, `cli.go`
- Create: `app_test.go`

**Interfaces:**
- Produces: `app` gains `stdout io.Writer` and `stderr io.Writer`; `newApp` takes them.

- [ ] **Step 1: Write the failing test**

Create `app_test.go`:

```go
package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunListWritesToInjectedStdout is the acceptance test for the whole
// refactor: a command's output is observable without spawning a subprocess.
func TestRunListWritesToInjectedStdout(t *testing.T) {
	dir := t.TempDir()
	writeTestCollection(t, filepath.Join(dir, "collection.json"))

	var out, errOut bytes.Buffer
	a := app{
		loc:    configLocation{Dir: dir},
		stdout: &out,
		stderr: &errOut,
	}

	if err := a.runList(selection{color: colorNever}); err != nil {
		t.Fatalf("runList: %v", err)
	}
	if !strings.Contains(out.String(), "Miles Davis - Kind of Blue") {
		t.Errorf("stdout missing the album, got:\n%s", out.String())
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr should be empty on success, got: %q", errOut.String())
	}
}

// TestRunListEmptyMatchReturnsErrorAndWritesNothing pins the contract that a
// failing command leaves stdout untouched -- the rule --json depends on.
func TestRunListEmptyMatchReturnsErrorAndWritesNothing(t *testing.T) {
	dir := t.TempDir()
	writeTestCollection(t, filepath.Join(dir, "collection.json"))

	var out, errOut bytes.Buffer
	a := app{loc: configLocation{Dir: dir}, stdout: &out, stderr: &errOut}

	filter := Filter{}
	filter.Fields = append(filter.Fields, FieldFilter{Field: queryField, Values: []string{"zzzz-no-such-album"}})

	err := a.runList(selection{color: colorNever, filter: filter})
	if err == nil {
		t.Fatal("expected an error for an empty match")
	}
	if out.Len() != 0 {
		t.Errorf("stdout must stay empty on failure, got: %q", out.String())
	}
}

func writeTestCollection(t *testing.T, path string) {
	t.Helper()
	const data = `[{"release_id":1,"artist":"Miles Davis","title":"Kind of Blue","year":1959,"label":"Columbia","catno":"CL 1355","genres":["Jazz"],"formats":["Vinyl","LP"]}]`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
}
```

Add `"os"` to the import block.

**Before writing it, check `Filter`'s actual field names** in `filter.go` and
adjust the `Filter{}` construction in the second test to match — the plan's
version is written from the type's shape and the exact field names must come
from the source.

- [ ] **Step 2: Run it and watch it fail**

Run: `go test . -run TestRunList -v`
Expected: FAIL — compile error, `unknown field stdout in struct literal of type app`.

- [ ] **Step 3: Add the writers**

In `app.go`:

```go
type app struct {
	loc    configLocation
	stdout io.Writer
	stderr io.Writer
}

func newApp(getenv func(string) string, homeDir func() (string, error), stdout, stderr io.Writer) (app, error) {
	loc, err := resolveConfigDir(getenv, homeDir)
	if err != nil {
		return app{stdout: stdout, stderr: stderr}, err
	}
	return app{loc: loc, stdout: stdout, stderr: stderr}, nil
}
```

Import `"io"`.

- [ ] **Step 4: Replace every direct write inside command code**

In `main.go` (28 sites), `sync.go` (7) and `migrate.go` (2):

- `fmt.Print(x)` / `fmt.Println(x)` → `fmt.Fprint(a.stdout, x)` / `fmt.Fprintln(a.stdout, x)`
- `fmt.Fprint(os.Stderr, x)` → `fmt.Fprint(a.stderr, x)`
- `fmt.Fprintln(os.Stderr, x)` → `fmt.Fprintln(a.stderr, x)`
- `writeJSON(os.Stdout, v)` → `writeJSON(a.stdout, v)`

`stdoutColor` becomes a method, since whether to colourise depends on whether
the *real* stdout is a terminal:

```go
// stdoutColor resolves whether stdout gets escape sequences. It deliberately
// still asks os.Stdout rather than a.stdout: colour depends on where output
// actually lands, and a test's bytes.Buffer is never a terminal anyway.
func (a app) stdoutColor(mode colorMode) bool {
	return useColor(mode, isTTY(os.Stdout), os.Getenv)
}
```

Leave `dispatch`'s own writes on `os.Stderr` — it has an `app` only after
`newApp` succeeds, and its failure path must still print.

- [ ] **Step 5: Wire the real writers in**

`dispatch`: `a, cfgErr := newApp(os.Getenv, os.UserHomeDir, os.Stdout, os.Stderr)`.

- [ ] **Step 6: Run the new tests and the whole suite**

Run: `go test . -run TestRunList -v`
Expected: PASS.

Run: `go test ./... && scripts/behaviour-diff.sh .refactor-baseline/disc-fortune-base`
Expected: PASS and `behaviour-diff: OK`.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor: inject stdout and stderr into app

Commands write to a.stdout/a.stderr instead of os.Stdout/os.Stderr, so
output is observable in process. Adds the first two tests that call a
command directly and read its output from a buffer.

No behaviour change."
```

---

### Task 6: Extract `internal/term`

**Files:**
- Create: `internal/term/term.go`, `internal/term/term_test.go`
- Create: `format.go`, `format_test.go` (the `formatAlbum` half of `color.go`)
- Delete: `color.go`, `color_test.go`

**Interfaces:**
- Produces:
  - `term.Mode` (was `colorMode`), `term.Auto`, `term.Always`, `term.Never`
  - `term.ParseMode(string) (term.Mode, error)`
  - `term.Use(mode Mode, tty bool, getenv func(string) string) bool`
  - `term.IsTTY(*os.File) bool`
  - `term.Reset`, `term.BoldCyan`, `term.BoldWhite`, `term.Dim`

- [ ] **Step 1: Create the package**

`internal/term/term.go` receives `color.go` lines 1-67 verbatim, with
`package term` and these renames (the package name already says "color", so
the prefixes go):

| Was | Becomes |
|---|---|
| `colorMode` | `Mode` |
| `colorAuto` / `colorAlways` / `colorNever` | `Auto` / `Always` / `Never` |
| `parseColorMode` | `ParseMode` |
| `useColor` | `Use` |
| `isTTY` | `IsTTY` |
| `colorReset` / `colorBoldCyan` / `colorBoldWhite` / `colorDim` | `Reset` / `BoldCyan` / `BoldWhite` / `Dim` |

The error text in `ParseMode` — `invalid --color value %q (want auto, always,
or never)` — is user-facing and must not change.

- [ ] **Step 2: Split the test file**

`internal/term/term_test.go` (`package term`) takes `TestIsTTY`,
`TestParseColorModeAcceptsTheThreeValues`, `TestParseColorModeRejectsAnythingElse`,
`TestAutoColorFollowsTheTerminal`, `TestAlwaysColorSurvivesAPipe`,
`TestNeverColorSuppressesOnATerminal`, `TestNoColorEnvDisablesColor`,
`TestEmptyNoColorIsIgnored`, `TestExplicitAlwaysOverridesNoColor` — renamed
identifiers only, no assertion changes.

`format_test.go` at the root keeps `TestFormatAlbum`.

- [ ] **Step 3: Create `format.go` at the root**

`color.go` lines 69-115 (`formatAlbum`) verbatim as `package main`, with
`colorBoldCyan` → `term.BoldCyan` etc., importing
`"github.com/daniel-munoz/disc-fortune/v2/internal/term"`.

- [ ] **Step 4: Update every consumer**

`cli.go`, `main.go`, `sync.go`, `history.go`, `stats.go` and `app_test.go`
reference the renamed symbols through the `term.` qualifier. The compiler enumerates them all; there
is nothing to guess. `git rm color.go color_test.go`.

- [ ] **Step 5: Verify**

Run: `go build ./... && go test ./... && gofmt -l . && go vet ./...`
Expected: builds, PASS, silent, silent.

Run: `scripts/behaviour-diff.sh .refactor-baseline/disc-fortune-base`
Expected: `behaviour-diff: OK`.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: extract internal/term

color.go splits: the mode/TTY/ANSI half becomes internal/term (which
depends on nothing), and formatAlbum stays in package main as format.go
because main.go is its only caller. This breaks the cycle that would
otherwise stop internal/disc from existing."
```

---

### Task 7: Extract `internal/disc`

The largest commit in the plan. It cannot be sliced smaller: the twelve files
reference each other, so they compile only once they all move together.

**Files:**
- Move: `collection.go` `favorites.go` `history.go` `meta.go` `filter.go` `backfill.go` `atomic.go` `lock.go` `lock_unix.go` `lock_other.go` `config.go` `migrate.go` → `internal/disc/`
- Move their 12 test files alongside
- Modify: `main.go` `cli.go` `sync.go` `json.go` `app.go` (qualify every reference)

**Interfaces:**
- Produces (the names root code depends on):
  - Types: `disc.Album`, `disc.HistoryEntry`, `disc.Meta`, `disc.Filter`, `disc.FieldFilter`, `disc.YearFilter`, `disc.Location` (was `configLocation`), `disc.FavoriteOutcome`, `disc.UnfavoriteOutcome`
  - Load/save: `disc.LoadCollectionFrom`, `disc.SaveCollectionTo`, `disc.LoadCollectionChecked`, `disc.LoadFavorites`, `disc.LoadFavoritesChecked`, `disc.SaveFavorites`, `disc.AddFavorite`, `disc.RemoveFavorite`, `disc.LoadHistory`, `disc.AddToHistory`, `disc.LoadMeta`, `disc.RecordSync`
  - Behaviour: `disc.FavoriteByQuery`, `disc.UnfavoriteByQuery`, `disc.MatchAlbums`, `disc.RunBackfill`, `disc.ResolveDir` (was `resolveConfigDir`), `disc.MigrationNotice`, `disc.SyncNotice`, `disc.Migrate` (was `migrateConfig`), `disc.FormatHistory`, `disc.FormatTimestamp`, `disc.ParseYearValue`, `disc.ParseDecadeValue`, `disc.Fields` (was `filterFields`), `disc.QueryField`, `disc.YearRange`, `disc.Plural`
  - Sentinels: `disc.ErrNoCollection`, `disc.ErrEmptyCollection`, `disc.ErrNoFavorites`, `disc.ErrAlreadyInFavorites`, `disc.ErrNotInFavorites`
  - Statuses: `disc.FavoriteAdded`, `disc.FavoriteAlreadyFav`, `disc.FavoriteNoMatch`, `disc.FavoriteMultiMatch`, `disc.UnfavoriteRemoved`, `disc.UnfavoriteNoMatch`, `disc.UnfavoriteMultiMatch`, `disc.MatchStatus` with `disc.MatchedNone`, `disc.MatchedMany`

- [ ] **Step 1: Move the command handler out of `migrate.go` first**

Task 3 made `runMigrate` a method on `app`, and `app` lives in `package main`.
It therefore cannot travel into `internal/disc` with the rest of the file.

Cut `runMigrate` from `migrate.go` and paste it into `main.go`, beside the other
`run*` methods. It keeps its `error` return from Task 4. What stays behind in
`migrate.go` — and moves to `disc` — is `migrationNotice` and `migrateConfig`,
which are pure functions over a `Location`.

After this step `runMigrate` calls `disc.Migrate(from, to)` (renamed in Step 3)
and reads `a.loc`. Move any test in `migrate_test.go` that exercises
`runMigrate` into `main_test.go`; tests for `migrationNotice` and
`migrateConfig` stay with the code and travel to `internal/disc`.

- [ ] **Step 2: Move the files**

```bash
mkdir -p internal/disc
git mv collection.go favorites.go history.go meta.go filter.go backfill.go \
       atomic.go lock.go lock_unix.go lock_other.go config.go migrate.go \
       internal/disc/
git mv collection_test.go favorites_test.go history_test.go history_unix_test.go \
       meta_test.go filter_test.go backfill_test.go atomic_test.go lock_test.go \
       lock_unix_test.go config_test.go migrate_test.go \
       internal/disc/
```

Change `package main` to `package disc` in all 24 files. Keep the `//go:build`
lines at the top of `lock_unix.go`, `lock_other.go` and `history_unix_test.go`
exactly where they are — a build tag must precede the package clause with a
blank line after it.

- [ ] **Step 3: Export what crosses the boundary**

Rename per the Interfaces list above. Everything **not** in that list stays
unexported — `sameAlbum`, `withFileLock`, `writeFileAtomic`, `hasData`,
`isLockSidecar`, `indexByLegacyKey` and friends are internal to the package and
the tests that use them now live inside it.

Rename `configLocation` → `Location` and `resolveConfigDir` → `ResolveDir`;
`Location`'s fields `Dir` and `Preferred` are already exported.

- [ ] **Step 4: Move the pluralisation helper**

`plural` is used by `stats.go` (a different package as of Task 10) and lives in
`backfill.go`, which is about data migration. Move it to the bottom of
`history.go`, beside `FormatTimestamp`, exported as `Plural`. Move its tests
from `backfill_test.go` to `history_test.go` unchanged.

- [ ] **Step 5: Qualify every reference at the root**

Add the import to `main.go`, `cli.go`, `sync.go`, `json.go`, `app.go`,
`app_test.go` — **and to `picker.go`, `stats.go` and `discogs.go`**:

```go
"github.com/daniel-munoz/disc-fortune/v2/internal/disc"
```

Those last three are easy to miss. They stay in `package main` until Tasks 8, 9
and 10, so the moment `internal/disc` exists they become cross-package consumers
of `Album`, `HistoryEntry`, `Meta`, `sameAlbum`, `formatTimestamp` and `plural`.
`internal/disc` therefore has to export **`SameAlbum`** in this task, not in
Task 8: rename `sameAlbum` → `SameAlbum` and update its callers inside `disc`
(`favorites.go` and the tests). Keep its doc comment verbatim — the roadmap's
Phase 2 notes depend on that ID-then-name fallback being understood.

`app.go`'s field becomes `loc disc.Location`, and `newApp` calls
`disc.ResolveDir`. The compiler enumerates the rest — build repeatedly and fix
what it names. There is nothing to guess here; a missed rename cannot compile.

- [ ] **Step 6: Verify, including the platforms you cannot see**

Run: `go build ./... && go test ./... && gofmt -l . && go vet ./...`
Expected: builds, PASS, silent, silent.

Run: `GOOS=windows go build ./... && GOOS=linux go build ./...`
Expected: both succeed. **This is the step that catches a broken
`lock_unix.go` / `lock_other.go` build-tag pair**, which is invisible when
building only for macOS.

Run: `scripts/behaviour-diff.sh .refactor-baseline/disc-fortune-base`
Expected: `behaviour-diff: OK`.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor: extract internal/disc

Domain types, persistence, locking, atomic writes, backfill and config
resolution move to internal/disc. filter.go moves with them: favorites.go
calls matchAlbums while filter.go needs Album, so a separate filter
package would be an import cycle.

Tests move with their subjects and stay white-box. plural() moves from
backfill.go to history.go, beside the other formatting helpers."
```

---

### Task 8: Extract `internal/pick`

**Files:**
- Move: `picker.go`, `picker_test.go` → `internal/pick/`
- Modify: `main.go`, `cli.go`

**Interfaces:**
- Consumes: `disc.Album`, `disc.HistoryEntry`, `disc.SameAlbum` (already exported in Task 7).
- Produces: `pick.Mode` (was `drawMode`), `pick.ParseMode`, `pick.Fresh`/`pick.Any`/`pick.Stale`, `pick.Draw(pool, entries, mode, rng) disc.Album` (was `pickAlbum`), `pick.UnheardOnly`, `pick.NewRNG`, `pick.ContainsAlbum`, `pick.LastPlayedIndex`

- [ ] **Step 1: Move and repackage**

```bash
mkdir -p internal/pick
git mv picker.go picker_test.go internal/pick/
```

`package main` → `package pick` in both. Import `internal/disc`.

- [ ] **Step 2: Rename to remove stutter**

`pickAlbum` → `pick.Album` would collide conceptually with `disc.Album`, so use
**`pick.Draw`**. `drawMode` → `Mode`, `drawFresh`/`drawAny`/`drawStale` →
`Fresh`/`Any`/`Stale`, `parseDrawMode` → `ParseMode`, `unheardOnly` →
`UnheardOnly`, `newRNG` → `NewRNG`, `containsAlbum` → `ContainsAlbum`,
`lastPlayedIndex` → `LastPlayedIndex`. `antiRepeatWindow`, `recentlyPlayed`,
`excludeRecent`, `staleWeights`, `weightedIndex` and `entriesInPool` stay
unexported.

The `--draw` flag's accepted values (`fresh`, `any`, `stale`) and its error text
are user-facing and unchanged.

- [ ] **Step 3: Verify**

Run: `go build ./... && go test ./... && gofmt -l . && go vet ./...`
Run: `scripts/behaviour-diff.sh .refactor-baseline/disc-fortune-base`
Expected: builds, PASS, silent, silent, `behaviour-diff: OK`.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor: extract internal/pick

picker.go becomes internal/pick. pickAlbum is now pick.Draw -- pick.Album
would have read as a type."
```

---

### Task 9: Extract `internal/discogs`

**Files:**
- Move: `discogs.go`, `discogs_test.go`, `discogs_retry_test.go` → `internal/discogs/`
- Modify: `sync.go`, `main.go`

**Interfaces:**
- Produces: `discogs.Client`, `discogs.New(userAgent string) (*Client, error)` (was `newDiscogsClient`), `discogs.Folder`, `discogs.ProgressFunc`, and methods `Username()`, `Folders(username)`, `CollectionReleases(username, folderID)`, plus `discogs.SetBaseURL` for tests.

- [ ] **Step 1: Move and repackage**

```bash
mkdir -p internal/discogs
git mv discogs.go discogs_test.go discogs_retry_test.go internal/discogs/
```

`package main` → `package discogs`. Import `internal/disc` for `disc.Album`.

- [ ] **Step 2: Invert the `version` dependency**

`discogs.go:36` currently reads the root's `version` constant:

```go
var userAgent = "disc-fortune/" + version
```

That is backwards once this is a package. Replace it with a constructor
parameter:

```go
// New returns a client identifying itself with the given User-Agent. Discogs'
// terms ask for accurate identification, so the caller passes the real version
// rather than this package carrying a copy that could drift.
func New(userAgent string) (*Client, error)
```

`Client` gains a `userAgent string` field, used where the global was.

- [ ] **Step 3: Keep the drift guard alive**

The test asserting the User-Agent contains the current version (roadmap T2)
must keep working. It moves to the root, since only the root knows `version`.
Add to `version_test.go`:

```go
// The Discogs API terms ask for accurate identification. This asserts the
// wiring at the seam: the root owns `version` and must hand it to the client.
func TestDiscogsUserAgentCarriesTheCurrentVersion(t *testing.T) {
	c, err := discogs.New(discogsUserAgent())
	if err != nil {
		t.Fatalf("discogs.New: %v", err)
	}
	if got := c.UserAgent(); !strings.Contains(got, version) {
		t.Errorf("User-Agent %q does not contain version %q", got, version)
	}
}
```

This needs `Client.UserAgent() string` (add it) and a root helper:

```go
// discogsUserAgent is the single place the version reaches the API client.
func discogsUserAgent() string { return "disc-fortune/" + version }
```

Delete the old in-package assertion of the same fact from
`internal/discogs/discogs_test.go`, since the constant it checked no longer
lives there. **This is the one permitted test deletion in the plan** — the fact
it guarded is now guarded by the test above, at the seam where drift could
actually occur.

- [ ] **Step 4: Update `sync.go`**

`newDiscogsClient()` → `discogs.New(discogsUserAgent())`; `discogsClient` →
`*discogs.Client`; `folder` → `discogs.Folder`; `progressFunc` →
`discogs.ProgressFunc`; the three methods take their new exported names.

- [ ] **Step 5: Verify**

Run: `go build ./... && go test ./... && gofmt -l . && go vet ./...`
Run: `scripts/behaviour-diff.sh .refactor-baseline/disc-fortune-base`
Expected: builds, PASS, silent, silent, `behaviour-diff: OK`.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: extract internal/discogs

The client takes its User-Agent as a constructor argument instead of
reading the root's version constant. The drift guard moves to the root,
where the version lives and where the wiring can actually break."
```

---

### Task 10: Extract `internal/stats`

**Files:**
- Move: `stats.go`, `stats_test.go` → `internal/stats/`
- Modify: `main.go`, `json.go`, `cli.go`

**Interfaces:**
- Consumes: `disc.Album`, `disc.HistoryEntry`, `disc.Meta`, `disc.FormatTimestamp`, `disc.Plural`, `pick.ContainsAlbum`, `pick.LastPlayedIndex`, `term.*`
- Produces: `stats.Stats`, `stats.DecadeBucket`, `stats.NameCount`, `stats.PickedStats`, `stats.Compute(...)` (was `computeStats`), `stats.Format(...)` (was `formatStats`), and `Stats.Share()`

- [ ] **Step 1: Move and repackage**

```bash
mkdir -p internal/stats
git mv stats.go stats_test.go internal/stats/
```

`package main` → `package stats`. Import `internal/disc`, `internal/pick`,
`internal/term`.

- [ ] **Step 2: Rename to remove stutter**

`computeStats` → `Compute`, `formatStats` → `Format`. The type stays `Stats`,
giving `stats.Stats` — acceptable for the package's central type, and the
alternative (`stats.Summary`) would make `json.go`'s `newStatsPayload` read
worse. `decadeBuckets`, `countGenres`, `countLabels`, `topNames`, `bar`, `pad`,
`heading`, `dim`, `decadeLabel`, `statsHeader`, `writeDecades` and
`writeNameTable` stay unexported.

Keep `topN`, `maxBarWidth` and every piece of output formatting byte-identical —
`stats` and `stats --json` are both harness probes.

- [ ] **Step 3: Update `json.go`**

`newStatsPayload(s Stats)` → `newStatsPayload(s stats.Stats)`; `NameCount` →
`stats.NameCount`; `s.Share()` unchanged.

- [ ] **Step 4: Verify**

Run: `go build ./... && go test ./... && gofmt -l . && go vet ./...`
Run: `GOOS=windows go build ./... && GOOS=linux go build ./...`
Run: `scripts/behaviour-diff.sh .refactor-baseline/disc-fortune-base`
Expected: all green, `behaviour-diff: OK`.

- [ ] **Step 5: Confirm the end state matches the design**

```bash
ls *.go | wc -l          # expect 20
ls internal              # expect: disc discogs pick stats term
go list ./...            # expect 6 packages
```

If the root count is not 20, something did not move. The expected root files
are: `main.go` `cli.go` `completion.go` `sync.go` `json.go` `open.go`
`format.go` `app.go` plus `main_test.go` `cli_test.go` `completion_test.go`
`sync_test.go` `json_test.go` `open_test.go` `global_flags_test.go`
`env_conventions_test.go` `version_test.go` `progress_test.go` `format_test.go`
`app_test.go` — 8 production and 12 test files, 20 total. (The design's
"16" predated `app.go`, `app_test.go`, `format.go` and `format_test.go`; 20 is
the correct end state.)

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: extract internal/stats

Completes the extraction. computeStats/formatStats become
stats.Compute/stats.Format; the root now holds only the CLI surface,
sync orchestration, JSON wire types and main."
```

---

### Task 11: Prove the goal, and update the docs

The refactor's purpose was testability and navigability. This task shows the
first is real, rather than asserting it.

**Files:**
- Modify: `app_test.go` (add coverage the subprocess harness used to own)
- Modify: `README.md`
- Create: `docs/releases/RELEASE_NOTES_v2.5.1.md` *(only if a release is cut; see Step 4)*

**Interfaces:**
- Consumes: everything above.

- [ ] **Step 1: Write in-process tests for two paths that previously needed a subprocess**

Add to `app_test.go`. These are **new** tests, not replacements — every
`runHelper` test stays exactly where it is.

```go
// TestRunHistoryEmptyIsNotAnError pins that an empty history prints its notice
// and succeeds. Before the refactor this fact could only be checked by
// re-execing the test binary.
func TestRunHistoryEmptyIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "history.json"), []byte(`[]`), 0644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	a := app{loc: disc.Location{Dir: dir}, stdout: &out, stderr: &errOut}

	if err := a.runHistory(historyConfig{color: term.Never}); err != nil {
		t.Fatalf("empty history should succeed, got: %v", err)
	}
	if !strings.Contains(out.String(), "No history yet") {
		t.Errorf("expected the empty-history notice, got: %q", out.String())
	}
}

// TestMissingCollectionCarriesItsGuidance pins the wording a user sees when
// they have not synced yet -- the text Task 4 moved from fatal() into an
// error value.
func TestMissingCollectionCarriesItsGuidance(t *testing.T) {
	var out, errOut bytes.Buffer
	a := app{loc: disc.Location{Dir: t.TempDir()}, stdout: &out, stderr: &errOut}

	err := a.runList(selection{color: term.Never})
	if err == nil {
		t.Fatal("expected an error when there is no collection")
	}
	if !strings.Contains(err.Error(), "Run `disc-fortune sync`") {
		t.Errorf("error should tell the user how to fix it, got: %q", err.Error())
	}
	if out.Len() != 0 {
		t.Errorf("stdout must stay empty, got: %q", out.String())
	}
}
```

- [ ] **Step 2: Run them**

Run: `go test . -run 'TestRunHistoryEmpty|TestMissingCollection' -v`
Expected: PASS. If the second fails on wording, Task 4 Step 1 drifted — fix the
error value, not the test.

- [ ] **Step 3: Update `README.md`**

Search for any description of the layout:

```bash
grep -n 'package main\|repository root\|\.go' README.md
```

Update anything claiming the code is flat. If the README says nothing about
layout, add nothing — this is not an invitation to document the structure.

- [ ] **Step 4: Decide on release notes**

This release has **no user-visible change**. If a version is cut, its notes say
exactly that. If no release is planned, create no file and skip this step.

- [ ] **Step 5: Full verification**

```bash
go build ./... && go test ./... && gofmt -l . && go vet ./...
GOOS=windows go build ./... && GOOS=linux go build ./...
scripts/behaviour-diff.sh .refactor-baseline/disc-fortune-base
```

Expected: everything green, `behaviour-diff: OK`.

- [ ] **Step 6: Commit and clean up the baseline**

```bash
git add -A
git commit -m "test: cover two command paths in process

Adds direct tests for the empty-history and missing-collection paths,
which previously required re-execing the test binary. The subprocess
harness is untouched -- retiring any of it is a separate decision.

Also updates README's layout description."
rm -rf .refactor-baseline
```

Leave `scripts/behaviour-diff.sh` in the tree; it is reusable for the
`cli.go`/`main.go` untangling that this plan deliberately deferred.

---

## Deferred, deliberately

Per the design's non-goals, this plan does **not** touch:

- The `cli.go` ↔ `main.go` mutual dependency, or `completion.go`'s reach into
  `cli.go`'s unexported flag registration.
- Any of the 64 `runHelper` subprocess tests. Converting them is now possible
  and is its own decision, made on its own merits.
- `cli.go`'s size (1,065 lines). It is the obvious next target.
