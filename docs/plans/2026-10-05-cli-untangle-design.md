# Untangling cli.go ↔ main.go — design

**Date:** 2026-10-05
**Status:** Approved in brainstorming; ready for an implementation plan.
**Issue:** #54
**Covers:** Extracting the command framework into `internal/cli`, giving each
command a self-contained file, and retiring the last direct `os.Exit` outside
`main`.

This follows `docs/plans/2026-09-06-package-structure-design.md`, which left
`cli.go`, `main.go`, `completion.go`, `sync.go`, `json.go` and `open.go` at the
root on purpose ("Deliberately unmoved"). Like that work, it adds no
user-visible capability. Every observable behaviour — stdout, stderr, exit
codes, file formats, flag grammar, help text, completion scripts — is
byte-identical before and after.

---

## Why

The goal is maintainability: improving code quality so future changes are
cheaper. The measure this design is judged by is **what the next person
adding a command or a flag has to touch.**

Measured at `4a15e6f`, adding a command means editing five places in three
files:

1. the `init()` literal in `cli.go` — usage text plus a run closure;
2. a `parseX` function in `cli.go`;
3. an `addXFlags` function in `cli.go`, if the command has flags;
4. a `runX` method in `main.go`;
5. the `switch` in `commandFlagSet` (`completion.go:40`) — the "second place to
   teach" that its own comment and `TestEveryCommandHasACompletionDecision`
   exist to police.

The specific defects:

- **`cli.go` ↔ `main.go` are mutually referential.** The `commands` table's
  closures (`cli.go:765–1112`) call `a.runX`; `main()` calls `dispatch`
  (`cli.go:1114`). Inside one package that compiles, so nothing stops it
  growing. `cli.go` is 1,136 lines.
- **`handleParseErr` (`cli.go:637`) still calls `os.Exit(1)`** from inside
  command closures, and writes to `os.Stdout`/`os.Stderr` directly instead of
  the `app` writers. Every other failure path returns an error to `dispatch`.
- **`completion.go` reaches into `cli.go`'s flag registration** through a
  hand-maintained `switch` over command names.
- **`commands` is filled in `init()`** only because `help` reads the table
  that contains it.

One item listed on #54 is already done: `sync.go` no longer calls `fatal`.
`fatal`'s only remaining callers are the two early exits in `dispatch`. What
`sync.go` still borrows from `cli.go` is `syncConfig` and `arrayFlags`.

## Goals

- **One place per command.** A command's usage, flags, parsing and run logic
  live in one file.
- **A compiler-enforced boundary.** The command framework cannot import the
  commands, so the cycle cannot come back.
- **One exit point.** Process exit happens in `main()` and nowhere else.

## Non-goals

- **Not a library, and no general-purpose CLI framework.** `internal/cli`
  serves exactly this program. It is generic over the env type only to avoid
  importing `main`. No features are added for hypothetical callers.
- **No new dependencies.** `go.mod` stays dependency-free.
- **No version bump.** Nothing user-visible changes, so this ships with the
  next user-visible release, as the September refactor did with v2.6.0.
- **No assertion edited to accommodate the refactor.** Call sites may change
  where a function moved package; expected strings and codes do not.
- **Not converting `runHelper` subprocess tests to in-process tests.** This
  design makes that possible (see `run`, below); doing it is a separate
  decision.
- **Not the `internal/disc` → `internal/term` rendering coupling.** Unrelated.
- **Not moving the commands out of `package main`.** See "Rejected
  alternatives".

---

## Part 1 — The command contract

Each command is a small type with three phases. `internal/cli` drives the
phases and never sees what's inside them.

```go
package cli

// Command is one subcommand. A fresh value is built per invocation, so its
// fields can hold flag pointers and the parsed config between phases.
type Command[E any] interface {
	// Flags registers the command's flags. Called for real parsing and by
	// completion on a throwaway FlagSet: the one place flags are taught.
	Flags(fs *flag.FlagSet, g *Globals)
	// Parse validates positional args and flag values after the flag
	// package has run. Any error here is a usage error.
	Parse(args []string) error
	// Run does the work. Any error here is a runtime failure.
	Run(env E) error
}

type Spec[E any] struct {
	Name, Summary, Usage string
	New                  func() Command[E]
	// NeedsConfig is opaque to cli. main reads it before Execute to decide
	// whether a config-resolution failure is fatal for this command.
	NeedsConfig bool
}
```

`E` is `app` in practice. Making `cli` generic over it is what lets `cli`
avoid importing `main`'s types. The alternative, an `any` env with a type
assertion in every `Run`, moves a compile-time guarantee to runtime.

`Globals` is today's `globalFlags` (`--color`), exported, with `Mode()`. `cli`
registers it on every FlagSet, as `newFlagSet` does now, so a command still
cannot ship without it.

### The parse/run flow

`cli.Parse(cmd, name, args) error` covers steps 1–3. `Execute` adds 4–5:

1. Build the FlagSet: `ContinueOnError`, output discarded, empty `Usage`,
   `--color` registered. Then call `cmd.Flags(fs, globals)`.
2. Run interspersed parsing (today's `parseInterspersed`).
   - `flag.ErrHelp` → print the command's usage to **stdout**, return 0.
   - Any other error → wrap as `"<name>: %w"` and treat it as a usage error.
     Every `parseX` today wraps its flag error exactly this way, so the
     framework can own the step without changing a message.
3. Call `cmd.Parse(rest)`. Any error is a usage error.
4. Usage error → print the error, a blank line and the usage to **stderr**,
   return 1. This replaces `handleParseErr`.
5. `cmd.Run(env)`. An error is printed bare — no prefix, as today — and
   returns 1.

### Byte-identity constraints

- **Validation order stays inside each command's `Parse`.** Today each
  `parseX` checks positional args, filters, `--color` and `--draw` in its own
  sequence. Two simultaneous bad inputs must report the same one first, which
  is why `Globals` is handed to the command rather than validated by `cli`.
- **Resolve, then config, then parse.** Today `dispatch` resolves the command,
  resolves config and prints the migration notice, then parses arguments. A
  parse error therefore appears *after* the migration notice. `main` keeps
  that order.
- **Exit codes are 0 and 1 only.** `--help` on any command remains a success
  (stdout, 0).
- **Writers.** Usage output now goes to the injected `stdout`/`stderr` instead
  of `os.Stdout`/`os.Stderr`. In production these are the same streams.

---

## Part 2 — The program, help and completion

`cli.Program[E]` replaces the package-level `commands` slice and the `init()`
that fills it.

```go
type Program[E any] struct {
	Name       string              // "disc-fortune": help header, scripts
	Tagline    string              // "randomly picks a record from your Discogs collection"
	Default    string              // "pick": what empty argv or a leading flag means
	Footer     string              // "With no command, disc-fortune picks a random album."
	Commands   []Spec[E]
	FlagValues map[string][]string // compiled-in completion values: draw, color
}

func (p *Program[E]) Resolve(args []string) (*Spec[E], []string, error)
func (p *Program[E]) Execute(s *Spec[E], args []string, env E, stdout, stderr io.Writer) int
func (p *Program[E]) Flags(name string) []FlagInfo
```

- **Built-ins.** `help` and `completion` only read the registry, so `Program`
  provides them. That removes the self-reference that forced `init()`.
  `version` stays an ordinary command in `main`.
- **Global-flag help belongs to `cli`.** `cli` owns `--color`, so it owns
  `globalFlagHelp` and the rule that appends it to every usage block except
  `help`'s. `filterFlagHelp` stays in `main`, because filters are
  disc-fortune's domain.
- **v1 signposts stay in `main`.** The `-v`/`--version` message and
  `v1Signposts` are disc-fortune history, not framework behaviour. `main`
  keeps a `resolve(args)` that checks them and then calls `p.Resolve`,
  preserving today's precedence (`cli.go:684`). The 16 root `resolve` tests
  keep calling it unchanged. `-h`/`--help` routing to `help` is generic and
  moves into `p.Resolve`.
- **Sanctioned flag enumeration.** `p.Flags(name)` builds a throwaway
  `Command`, calls its `Flags` on a scratch FlagSet and returns name, isBool
  and values for each flag, sorted. That is the same call real parsing makes.
  `commandFlagSet`'s `switch` is deleted, and a command has nowhere else to
  register a flag.
- **Completion generation moves into `cli`.** The 17 `disc-fortune` literals in
  `completion.go` become `p.Name`. `flagValues` becomes `Program.FlagValues`,
  supplied by `main`. Turning `--draw` into a custom `flag.Value` that knows
  its own choices was considered and rejected: the bad-value error would then
  come from the flag package instead of `pick.ParseMode`, which changes the
  message.

### What `main.go` becomes

```go
func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// run is the whole process: argv in, exit code out. It is callable from tests.
func run(args []string, stdout, stderr io.Writer) int {
	// v1 signposts → program.Resolve → newApp + NeedsConfig gate
	// + migration notice → program.Execute
}
```

`fatal` and `dispatch` go away. Every exit code comes from one `return`.

---

## Part 3 — File layout

### `internal/cli` (new)

| File | Contents |
|---|---|
| `command.go` | `Command`, `Spec`, `Globals`, `Parse` |
| `program.go` | `Program`, `Resolve`, `Execute`, `Flags`, built-in `help` |
| `flags.go` | FlagSet construction, interspersed parsing, `globalFlagHelp` |
| `completion.go` | built-in `completion`, bash/zsh/fish generation |

Depends on `internal/term` (for `Globals.Mode`) and the standard library only.

### Root `package main`

| File | Contents |
|---|---|
| `main.go` | `main`, `run`, `program` (the `Program` value), v1 signposts, guidance errors |
| `app.go` | unchanged |
| `cmd_pick.go`, `cmd_reroll.go`, `cmd_list.go` | the selection commands; `drawAndRecord` lives with pick |
| `cmd_favorite.go` | `favorite` and `unfavorite`, which share one type parameterised by name |
| `cmd_open.go`, `cmd_history.go`, `cmd_stats.go`, `cmd_sync.go`, `cmd_folders.go`, `cmd_migrate.go`, `cmd_version.go` | one command each |
| `filters.go` | `filterFlags`, `selectionFlags`, `parseQueryCommand`, `filterFlagHelp`: the grammar several commands share |
| `sync.go` | Discogs-side helpers for `sync` and `folders`: progress, folder resolution, unmerge notice, `arrayFlags` |
| `format.go`, `json.go`, `open.go` | output and browser helpers used by the commands |

### Why the remaining root files stay

#54 asks for each remaining root file to be moved or justified. The
justification is a single rule: **`package main` is the command layer.** The
files above either are commands or exist only to serve them: text and JSON
rendering of command output, launching a browser for `open`, Discogs
orchestration for `sync`. None has a second consumer, and none is imported by
`internal/`. Moving them would create packages with one caller each.

---

## Part 4 — Tests

**Call sites stay; production shims do not.** The tests call `parseSelection`,
`parseFavorite`, `parseHistory`, `parseOpen`, `parseStats`, `parseSync`,
`parseNoArgs`, `parseHelp` and `parseCompletion` about 130 times. These
functions stop existing in production code. Each name is redefined as a test
helper in a `_test.go` file with the same signature, which drives the real
path (`cli.Parse` on the command's type) and returns the parsed config. No
production code exists only to keep tests compiling, and no assertion changes.

**Framework tests move with the framework.** Tests of interspersed parsing,
resolution, the `--help`/usage-error policy, help rendering and completion move
to `internal/cli`. They use a small fake `Program` there, so `cli`'s tests do
not depend on disc-fortune's commands. Assertions about disc-fortune's actual
help text and scripts stay at the root, against the real `program`.

**`handleParseErr` tests** become `Execute` tests with the same expected stdout,
stderr and exit codes.

**Deleted:** `TestEveryCommandHasACompletionDecision`. It guards the `switch`
in `commandFlagSet`, and that switch no longer exists. The structure itself now
prevents what the test checked for. This is the only test removed.

**Unchanged:** the 64 `runHelper` subprocess tests. They drive argv to
stdout/stderr/exit code and are the primary evidence that behaviour did not
move.

---

## Part 5 — Sequencing

Each commit is green and independently revertible.

| # | Commit | Risk |
|---|---|---|
| 0 | Build the baseline binary from `4a15e6f`. Confirm `behaviour-diff.sh` reports MISMATCH against a deliberately wrong baseline. | — |
| 1 | Move the flag plumbing (`newFlagSet`, `parseInterspersed`, `Globals`, `globalFlagHelp`) into `internal/cli`. | low |
| 2 | Turn each command into a `cmd_<name>.go` type, still dispatched inside `main` by a local executor. Delete `init()`, the closures and `handleParseErr`. Introduce the test-helper `parseX` functions. | **highest** — largest diff, but within one package |
| 3 | Move `Spec`/`Program`/`Resolve`/`Execute` and built-in `help` into `cli`. Replace `dispatch` and `fatal` with `run() int`. | medium |
| 4 | Move completion into `cli` on top of `p.Flags`. Delete the `switch` and its guard test. | medium — 17 name literals |
| 5 | Split `filters.go` out. Add a note to the September design's "Deliberately unmoved" section pointing here. (`README.md` does not describe the layout, so it needs no change.) | low |

Commit 2 is separated from 3 so the large reshaping happens inside one
package, where the compiler is most forgiving. The package boundary is then
crossed by a small diff.

## Verification

After every commit:

- `go test ./...` green.
- `gofmt -l .` empty, `go vet ./...` clean.
- `../tools/behaviour-diff.sh` against the commit-0 baseline. Every probe
  matches on stdout, stderr and exit code. Its probes already include `help`,
  `help pick`, `help stats`, `version` and `completion bash|zsh|fish`, which
  cover commits 3 and 4 byte for byte.
- No test assertion edited.

Done when:

- every acceptance criterion on #54 is met;
- `os.Exit` appears only in `main()`;
- `internal/cli` imports nothing from the module except `internal/term`;
- adding a command means one new `cmd_*.go` file plus one line in
  `program.Commands`.

## Rejected alternatives

- **Commands in `internal/command`, root holding only `main.go`.** This gives
  the shortest root listing. But it moves `app` and about 3,000 lines of root
  tests a second time, with no maintainability gain over this design. It
  remains a mechanical follow-up if the listing becomes a problem.
- **`cli` owns the table and calls an interface `app` implements.** Least
  code motion, but the interface lists every command, so adding one means
  touching the table, the interface and the method. That adds a place instead
  of removing one.
- **`cli` validates `--color` itself.** Simpler commands, but it changes which
  error is reported first when two inputs are bad.

## Amendments from planning

Found while writing `2026-10-05-cli-untangle.md`. Each one supersedes the
text above where they differ.

1. **`Parser` is split out of `Command`.** `cli.Parse` takes a
   `Parser` (`Flags` + `Parse`), and `Command[E]` embeds it and adds `Run`.
   This lets the shared grammar types (`selectionCmd`, `queryCmd`,
   `noArgsCmd`) be parsed, and tested, without a `Run`. It also makes
   `cli.Parse` non-generic.
2. **`run*` methods stay on `app`** and move into their command's file.
   `Run` delegates (`return a.runPick(c.cfg)`). `app_test.go` calls these
   methods directly 14 times. Keeping them means those tests keep testing
   the work without parsing, and `Run` is just the link between the two.
3. **The shared-grammar file is `grammar.go`, not `filters.go`.** It also
   holds `selectionCmd`, `queryCmd` and `noArgsCmd`, which are not filters.
4. **Seven commits, not five.** Commit 2 is split into "command types,
   bridged into the old table" and "move each `run*` method beside its
   command" (a pure move). The framework is built and tested on its own
   before anything uses it.
5. **Tests ported rather than kept.** Besides
   `TestEveryCommandHasACompletionDecision`, these root tests go away
   because what they test moves into `cli`. Each gets an equivalent
   `internal/cli` test with the same expectations:
   `TestParseHelpHelpFlagIsErrHelp`, `TestParseHelpTopic`,
   `TestParseHelpTooManyArguments`, `TestHelpHelpFlagExitsZero`,
   `TestParseCompletionRequiresAShell`, `TestCompletionRejectsInvalidColor`,
   and the six `TestParseInterspersed*`. The two `handleParseErr` help-flag
   tests become one root `Execute` test against the real program, keeping
   both tests' expectations. They differed only in whether `flag.ErrHelp`
   arrived wrapped, and `cli.Parse` now always wraps it.
   `TestHandleParseErrNilIsNotHandled` is dropped. The path it guarded ("no
   error means run the command") becomes `Execute`'s ordinary success
   path, which `TestExecuteRunsTheCommand` covers.

## Follow-ups (not this work)

- Convert `runHelper` subprocess tests to call `run` in-process where they
  only need stdout/stderr/exit code.
- Move rendering out of `internal/disc` so it no longer depends on
  `internal/term`.
- Cosmetic items from PR #18 not touched here: `LoadCollectionFrom`/
  `SaveCollectionTo` suffixes, and root `%v` vs internal `%w` in `fmt.Errorf`.
