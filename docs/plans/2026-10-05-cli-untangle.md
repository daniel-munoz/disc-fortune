# Untangling cli.go ↔ main.go Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract disc-fortune's command framework into `internal/cli`, give each
command one self-contained file in `package main`, and leave `main()` as the only
place the process exits. Observable behaviour stays byte-identical.

**Architecture:** `internal/cli` gets a generic `Program[E]` holding `Spec[E]`s.
Each spec builds a `Command[E]` with three phases: `Flags`, `Parse`, `Run`. `cli`
owns flag parsing, the `--help`/usage-error policy, help, and completion. It
never imports `main`; `E` is `main`'s `app`. `package main` becomes `cmd_*.go`
files, shared grammar in `grammar.go`, and a `run(args, stdout, stderr) int`
that `main()` passes to `os.Exit`.

**Tech Stack:** Go 1.24, standard library only (`flag`, `fmt`, `io`, `strings`,
`sort`, `errors`).

**Spec:** `docs/plans/2026-10-05-cli-untangle-design.md`, including its
"Amendments from planning" section, which overrides the body where they differ.

## Global Constraints

- Byte-identical stdout, stderr and exit codes for every invocation. Exit codes are 0 and 1 only.
- `--help` / `-h` / `-help` on any command prints that command's usage to **stdout** and exits **0**.
- A usage error prints `<error>`, a blank line, then the usage to **stderr**, and exits 1. A runtime error prints `<error>` alone to stderr, exits 1, and never shows usage.
- Runtime errors print with no prefix. Only resolve and config failures carry `disc-fortune: `.
- Order in `main`: v1 signposts, then resolve, then config resolution + migration notice, then parse, then run.
- Each command checks its own inputs in the order today's parse function does. `cli` never validates `--color` itself.
- `go.mod` gains no dependencies.
- No test assertion's expected value changes. Call sites may be renamed where a symbol moved or was exported.
- Tests stay white-box (`package main` / `package cli`).
- No version bump. `const version = "2.6.0"` is untouched.
- Dev tooling stays outside the repo: `../tools/behaviour-diff.sh` is edited in place and never copied into the worktree.
- Commit messages end with:
  ```
  Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01Cn2QECzyxmYVhLjvwZ7jts
  ```

## Review Focus

1. **`<command> --help` for every command, built-ins included** (`help --help`, `completion --help`): usage on stdout, empty stderr, exit 0. Pinned by `TestEveryCommandHelpFlagPrintsUsageToStdout` in Task 5, and by harness probes from Task 0.
2. **Two bad inputs at once** (`list extra --color=sometimes`): the same error is reported first as before (`unexpected argument`, not the color error). Pinned by `TestParseSelectionReportsPositionalBeforeColor` in Task 3, and by a Task 0 probe.
3. **No usable home directory plus a usage error on a data command** (`HOME= disc-fortune list --bogus-flag`): the config error wins, because config is resolved before parsing. Pinned by `TestBinaryConfigFailureBeatsUsageError` in Task 5.
4. **A runtime failure never prints usage** (`favorite` with no history, `help nope`). Pinned by `TestExecuteRunErrorPrintsNoUsage` and `TestExecuteHelpUnknownTopicIsARuntimeError` in Task 2.
5. **Built-in usage text with the program name substituted** (`help help`, `help completion`) matches the old literals exactly. Pinned by `TestBuiltinUsageMatchesTheOldText` in Task 6, and by Task 0 probes.

---

## File map (end state)

| File | Responsibility |
|---|---|
| `internal/cli/flags.go` | `Globals`, `NewFlagSet`, `ParseInterspersed`, `GlobalFlagHelp` |
| `internal/cli/command.go` | `Parser`, `Command[E]`, `Parse` |
| `internal/cli/program.go` | `Spec[E]`, `Program[E]`, `NewProgram`, `Lookup`, `Resolve`, `HelpText`, `Execute`, `Flags`, `FlagInfo` |
| `internal/cli/help.go` | built-in `help` |
| `internal/cli/completion.go` | built-in `completion`, bash/zsh/fish generation |
| `main.go` | `version`, `discogsUserAgent`, `main`, `run`, `program`, `resolve`, `v1Signposts` |
| `app.go` | `app`, `newApp`, path helpers, guidance errors, `collection`, `favorites`, `stdoutColor`, `selectAlbums`, `reportAmbiguous`, `describeSelection` |
| `grammar.go` | filter flags, `selection`, `selectionFlags`, `selectionCmd`, `queryCmd`, `parseQueryCommand`, `noArgsCmd`, `filterFlagHelp` |
| `cmd_pick.go` `cmd_reroll.go` `cmd_list.go` `cmd_favorite.go` `cmd_open.go` `cmd_history.go` `cmd_stats.go` `cmd_sync.go` `cmd_folders.go` `cmd_migrate.go` `cmd_version.go` | one command each (favorite and unfavorite share one file): spec, command type, `run*` method |
| `sync.go` | Discogs helpers: `syncProgress`, `arrayFlags`, folder resolution, `collectAlbums`, unmerge notice |
| `format.go` `json.go` `open.go` | output and browser helpers |
| `cli.go`, `completion.go` (root) | **deleted** |

---

### Task 0: Baseline and harness probes

No repo commit. This sets up the safety net every later task runs.

**Files:**
- Modify: `/Users/danielm/personal-projects/disc-fortune/tools/behaviour-diff.sh` (`PROBES` array). This file is outside the repo.
- Create: `.refactor-baseline/base` (already gitignored by `.gitignore:3`)

- [ ] **Step 1: Build the baseline from the pre-refactor code**

The branch tip (`b98ee0d` or later) has only doc commits since `4a15e6f`, so the code is identical.

```bash
git diff --stat 4a15e6f HEAD -- '*.go'   # expect: no output
mkdir -p .refactor-baseline
go build -o .refactor-baseline/base .
```

- [ ] **Step 2: Add the probes this refactor needs**

Append these lines inside `PROBES=( ... )` in `../tools/behaviour-diff.sh`, before the closing `)`:

```zsh
  "pick --help"
  "list --help"
  "sync --help"
  "migrate --help"
  "completion --help"
  "help --help"
  "help -h"
  "help help"
  "help completion"
  "help sync"
  "help bogus"
  "help a b"
  "help --color=bogus"
  "-h"
  "--help"
  "-v"
  "--version"
  "--sync"
  "--favorite-last"
  "list extra --color=sometimes"
  "list --color=sometimes"
  "list --draw any"
  "history -3"
  "history abc"
  "history 1 2"
  "favorite a b"
  "favorite --genre jazz"
  "open a b"
  "stats extra"
  "folders extra"
  "version extra"
  "completion"
  "completion tcsh"
  "completion bash zsh"
  "completion --color=bad bash"
```

None of these mutate data. Each one either prints help or fails before loading anything.

- [ ] **Step 3: Prove the harness can fail**

```bash
../tools/behaviour-diff.sh /bin/echo; echo "exit=$?"
```

Expected: `MISMATCH` lines and `exit=1`. If it reports all-match, stop: the harness is broken.

- [ ] **Step 4: Prove the harness passes on the unchanged tree**

```bash
../tools/behaviour-diff.sh .refactor-baseline/base; echo "exit=$?"
```

Expected: every probe matches, `exit=0`.

---

### Task 1: Flag plumbing and the parse contract in `internal/cli`

**Files:**
- Create: `internal/cli/flags.go`, `internal/cli/command.go`, `internal/cli/flags_test.go`, `internal/cli/command_test.go`
- Modify: `cli.go` (remove `globalFlags`, `newFlagSet`, `parseInterspersed`, `globalFlagHelp`; use the `cli` versions), `completion.go`, `cli_test.go`, `global_flags_test.go`

**Interfaces:**
- Produces:
  - `type Globals struct{ color *string }`, `func (g *Globals) Mode() (term.Mode, error)`
  - `func NewFlagSet(name string) (*flag.FlagSet, *Globals)`
  - `func ParseInterspersed(fs *flag.FlagSet, args []string) ([]string, error)`
  - `const GlobalFlagHelp`
  - `type Parser interface { Flags(fs *flag.FlagSet, g *Globals); Parse(args []string) error }`
  - `type Command[E any] interface { Parser; Run(env E) error }`
  - `func Parse(p Parser, name string, args []string) error`

- [ ] **Step 1: Write the failing `Parse` tests**

Create `internal/cli/command_test.go`:

```go
package cli

import (
	"errors"
	"flag"
	"reflect"
	"testing"

	"github.com/daniel-munoz/disc-fortune/v2/internal/term"
)

// fakeParser records what Parse handed it.
type fakeParser struct {
	verbose *bool
	g       *Globals
	got     []string
	err     error
}

func (f *fakeParser) Flags(fs *flag.FlagSet, g *Globals) {
	f.verbose = fs.Bool("verbose", false, "")
	f.g = g
}

func (f *fakeParser) Parse(args []string) error {
	f.got = args
	return f.err
}

func TestParsePassesPositionalsAfterFlags(t *testing.T) {
	p := &fakeParser{}
	if err := Parse(p, "fake", []string{"a", "--verbose", "b"}); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !*p.verbose {
		t.Error("--verbose after a positional was dropped")
	}
	if want := []string{"a", "b"}; !reflect.DeepEqual(p.got, want) {
		t.Errorf("positionals = %v, want %v", p.got, want)
	}
}

// Every parse function used to wrap its flag error as "<name>: %w". Owning
// that step here is only behaviour-preserving if the text is the same.
func TestParseWrapsFlagErrorsWithTheCommandName(t *testing.T) {
	err := Parse(&fakeParser{}, "fake", []string{"--nope"})
	want := "fake: flag provided but not defined: -nope"
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

func TestParseKeepsErrHelpRecognisable(t *testing.T) {
	for _, arg := range []string{"-h", "--help", "-help"} {
		if err := Parse(&fakeParser{}, "fake", []string{arg}); !errors.Is(err, flag.ErrHelp) {
			t.Errorf("Parse(%s) = %v, want errors.Is(_, flag.ErrHelp)", arg, err)
		}
	}
}

func TestParseRegistersTheGlobalColorFlag(t *testing.T) {
	p := &fakeParser{}
	if err := Parse(p, "fake", []string{"--color", "never"}); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if m, err := p.g.Mode(); err != nil || m != term.Never {
		t.Errorf("Mode() = %v, %v; want term.Never, nil", m, err)
	}
}

// Validation order belongs to each command: a command with two bad inputs
// must keep reporting the same one first. So Parse must not judge --color.
func TestParseLeavesColorValidationToTheCommand(t *testing.T) {
	if err := Parse(&fakeParser{}, "fake", []string{"--color", "bogus"}); err != nil {
		t.Fatalf("Parse rejected --color=bogus itself: %v", err)
	}
}

func TestParseReturnsTheCommandsOwnErrorUnwrapped(t *testing.T) {
	own := errors.New("fake: requires a name")
	if err := Parse(&fakeParser{err: own}, "fake", nil); err != own {
		t.Errorf("err = %v, want the command's own error unchanged", err)
	}
}
```

- [ ] **Step 2: Run them and confirm they fail**

Run: `go test ./internal/cli/`
Expected: build failure, `undefined: Parse` / `undefined: Globals`.

- [ ] **Step 3: Create `internal/cli/flags.go` by moving code out of `cli.go`**

Move these from `cli.go` **verbatim**, including their doc comments, then rename as shown:

| From `cli.go` | Becomes |
|---|---|
| `type globalFlags struct` (lines 17–24) | `type Globals struct` (field stays `color *string`) |
| `func (g *globalFlags) mode()` (25–29) | `func (g *Globals) Mode()` |
| `func newFlagSet` (30–45) | `func NewFlagSet`, returning `*Globals`, building `g := &Globals{...}` |
| `func parseInterspersed` (46–67) | `func ParseInterspersed` |
| `const globalFlagHelp` (731–739) | `const GlobalFlagHelp` |

The file header:

```go
// Package cli is disc-fortune's command framework: how argv becomes a
// command, how a command's flags are parsed, and what is printed when parsing
// fails. It knows nothing about any particular command -- those live in
// package main and are handed to it as values, which is what keeps the
// dependency one-way.
package cli

import (
	"flag"
	"io"

	"github.com/daniel-munoz/disc-fortune/v2/internal/term"
)
```

In the moved doc comments, update self-references (e.g. "registered in newFlagSet" becomes "registered in NewFlagSet").

- [ ] **Step 4: Create `internal/cli/command.go`**

```go
package cli

import (
	"flag"
	"fmt"
)

// Parser is the half of a command that turns argv into a config. It is split
// from Command so that grammar shared by several commands can be parsed, and
// tested, without having to run anything.
type Parser interface {
	// Flags registers the command's flags. It is called for real parsing and
	// by completion on a throwaway FlagSet, which makes it the one place a
	// command's flags are taught.
	Flags(fs *flag.FlagSet, g *Globals)
	// Parse validates positional arguments and flag values after the flag
	// package has run. Any error it returns is a usage error.
	Parse(args []string) error
}

// Command is one subcommand. A fresh value is built per invocation, so its
// fields can carry flag pointers and the parsed config from Parse to Run.
type Command[E any] interface {
	Parser
	// Run does the work. Any error it returns is a runtime failure, printed
	// without usage.
	Run(env E) error
}

// Parse runs p's parse phases against args: register flags, parse them
// wherever they appear, then hand the positionals to p.Parse.
//
// A flag-package error is wrapped as "<name>: %w" -- the exact form every
// command used before this owned the step -- so errors.Is still finds
// flag.ErrHelp through it. p.Parse's own errors are returned unchanged; each
// command already names itself in them.
func Parse(p Parser, name string, args []string) error {
	fs, g := NewFlagSet(name)
	p.Flags(fs, g)
	rest, err := ParseInterspersed(fs, args)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return p.Parse(rest)
}
```

- [ ] **Step 5: Move the six `TestParseInterspersed*` tests**

Cut `TestParseInterspersedFlagsAfterPositional` through `TestParseInterspersedUnknownFlag` (`cli_test.go:38–115`) into a new `internal/cli/flags_test.go` with `package cli`, importing `testing`. Rename `newFlagSet(` to `NewFlagSet(` and `parseInterspersed(` to `ParseInterspersed(`. Leave assertions and messages unchanged.

- [ ] **Step 6: Point `package main` at the new symbols**

```bash
perl -pi -e 's/\bnewFlagSet\(/cli.NewFlagSet(/g; s/\bparseInterspersed\(/cli.ParseInterspersed(/g; s/\*globalFlags\b/*cli.Globals/g; s/\bgf\.mode\(\)/gf.Mode()/g; s/\bglobalFlagHelp\b/cli.GlobalFlagHelp/g' cli.go completion.go cli_test.go global_flags_test.go
goimports -w cli.go completion.go cli_test.go global_flags_test.go 2>/dev/null || true
```

If `goimports` is not installed, add `"github.com/daniel-munoz/disc-fortune/v2/internal/cli"` to the import blocks by hand and remove `io` from `cli.go` if `go vet` reports it unused. Then fix any comment the perl touched so it reads naturally. For example, `global_flags_test.go:89` mentions `globalFlagHelp` in prose; make it `cli.GlobalFlagHelp`.

- [ ] **Step 7: Run everything**

```bash
go vet ./... && go test ./... && gofmt -l . && ../tools/behaviour-diff.sh .refactor-baseline/base
```

Expected: vet clean, all tests PASS (including the six moved ones and six new ones in `internal/cli`), `gofmt -l` prints nothing, harness all-match.

- [ ] **Step 8: Commit**

```bash
git add -A internal/cli cli.go completion.go cli_test.go global_flags_test.go
git commit -m "refactor: move flag plumbing into internal/cli

Adds the Parser/Command contract and cli.Parse, which owns the
'<name>: %w' wrapping every parse function used to repeat.
No behaviour change. Part of #54.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01Cn2QECzyxmYVhLjvwZ7jts"
```

---

### Task 2: `Program`, `Execute` and built-in `help` in `internal/cli`

This is purely additive: nothing in `package main` uses it yet. It is tested against a fake two-command program.

**Files:**
- Create: `internal/cli/program.go`, `internal/cli/help.go`, `internal/cli/program_test.go`

**Interfaces:**
- Consumes: `Parser`, `Command[E]`, `Parse`, `NewFlagSet`, `GlobalFlagHelp` (Task 1)
- Produces:
  - `type Spec[E any] struct { Name, Summary, Usage string; New func() Command[E]; NeedsConfig bool; usageFor func(program string) string }`
  - `type Program[E any] struct { Name, Tagline, Default, Footer string; Commands []Spec[E] }` (Task 6 adds `FlagValues`)
  - `func NewProgram[E any](p Program[E]) *Program[E]`
  - `func (p *Program[E]) Lookup(name string) *Spec[E]`
  - `func (p *Program[E]) Resolve(args []string) (*Spec[E], []string, error)`
  - `func (p *Program[E]) HelpText(topic string) (string, error)`
  - `func (p *Program[E]) Execute(s *Spec[E], args []string, env E, stdout, stderr io.Writer) int`
  - `func Help[E any]() Spec[E]`

- [ ] **Step 1: Write the failing tests**

Create `internal/cli/program_test.go`:

```go
package cli

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"testing"
)

// greetCmd is the fake command these tests drive. Its env is the buffer Run
// writes to, standing in for main's app.
type greetCmd struct {
	loud *bool
	g    *Globals
	who  string
}

func (c *greetCmd) Flags(fs *flag.FlagSet, g *Globals) {
	c.loud = fs.Bool("loud", false, "Shout")
	c.g = g
}

func (c *greetCmd) Parse(args []string) error {
	if len(args) != 1 {
		return errors.New("greet: requires a name")
	}
	if _, err := c.g.Mode(); err != nil {
		return fmt.Errorf("greet: %v", err)
	}
	c.who = args[0]
	return nil
}

func (c *greetCmd) Run(w *bytes.Buffer) error {
	if c.who == "fail" {
		return errors.New("greet failed")
	}
	fmt.Fprintf(w, "hello %s\n", c.who)
	return nil
}

const greetUsage = "Usage: tool greet NAME"

func testProgram() *Program[*bytes.Buffer] {
	return NewProgram(Program[*bytes.Buffer]{
		Name:    "tool",
		Tagline: "does things",
		Default: "greet",
		Footer:  "With no command, tool greets.",
		Commands: []Spec[*bytes.Buffer]{
			{Name: "greet", Summary: "Say hello", Usage: greetUsage,
				New: func() Command[*bytes.Buffer] { return &greetCmd{} }},
			Help[*bytes.Buffer](),
		},
	})
}

// execute runs one invocation the way main will: resolve, then Execute.
func execute(t *testing.T, p *Program[*bytes.Buffer], args ...string) (code int, env, stdout, stderr string) {
	t.Helper()
	s, rest, err := p.Resolve(args)
	if err != nil {
		t.Fatalf("Resolve(%v): %v", args, err)
	}
	var e, out, errOut bytes.Buffer
	code = p.Execute(s, rest, &e, &out, &errOut)
	return code, e.String(), out.String(), errOut.String()
}

func TestNewProgramAppendsGlobalHelpExceptToHelp(t *testing.T) {
	p := testProgram()
	if got := p.Lookup("greet").Usage; got != greetUsage+GlobalFlagHelp {
		t.Errorf("greet usage = %q, want it to end with GlobalFlagHelp", got)
	}
	want := "Usage: tool help [COMMAND]\n\nShows general help, or detailed help for one command."
	if got := p.Lookup("help").Usage; got != want {
		t.Errorf("help usage = %q, want %q", got, want)
	}
}

func TestLookupUnknownIsNil(t *testing.T) {
	if s := testProgram().Lookup("nope"); s != nil {
		t.Errorf("Lookup(nope) = %v, want nil", s)
	}
}

func TestResolveEmptyArgsMeansDefault(t *testing.T) {
	s, rest, err := testProgram().Resolve(nil)
	if err != nil || s.Name != "greet" || len(rest) != 0 {
		t.Errorf("Resolve(nil) = %v, %v, %v; want greet, [], nil", s, rest, err)
	}
}

func TestResolveLeadingFlagMeansDefault(t *testing.T) {
	s, rest, err := testProgram().Resolve([]string{"--loud", "bob"})
	if err != nil || s.Name != "greet" || len(rest) != 2 || rest[0] != "--loud" {
		t.Errorf("Resolve = %v, %v, %v; want greet, [--loud bob], nil", s, rest, err)
	}
}

func TestResolveHelpFlagsReachHelp(t *testing.T) {
	for _, arg := range []string{"-h", "--help", "-help"} {
		s, rest, err := testProgram().Resolve([]string{arg})
		if err != nil || s.Name != "help" || len(rest) != 0 {
			t.Errorf("Resolve(%s) = %v, %v, %v; want help, [], nil", arg, s, rest, err)
		}
	}
}

func TestResolveUnknownCommand(t *testing.T) {
	_, _, err := testProgram().Resolve([]string{"frob"})
	want := "unknown command \"frob\"\nRun `tool help` for usage."
	if err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
}

func TestHelpTextListsCommands(t *testing.T) {
	got, err := testProgram().HelpText("")
	if err != nil {
		t.Fatal(err)
	}
	want := "tool - does things\n\n" +
		"Usage:\n  tool [command] [flags]\n\n" +
		"Commands:\n" +
		"  greet       Say hello\n" +
		"  help        Show help for a command\n" +
		"\nRun `tool help <command>` for details on a command.\n" +
		"With no command, tool greets.\n"
	if got != want {
		t.Errorf("HelpText(\"\") =\n%q\nwant\n%q", got, want)
	}
}

func TestHelpTextForOneTopic(t *testing.T) {
	p := testProgram()
	got, err := p.HelpText("greet")
	if err != nil || got != p.Lookup("greet").Usage {
		t.Errorf("HelpText(greet) = %q, %v", got, err)
	}
	if _, err := p.HelpText("nope"); err == nil || err.Error() != `help: unknown command "nope"` {
		t.Errorf("HelpText(nope) err = %v", err)
	}
}

func TestExecuteRunsTheCommand(t *testing.T) {
	code, env, out, errOut := execute(t, testProgram(), "greet", "bob")
	if code != 0 || env != "hello bob\n" || out != "" || errOut != "" {
		t.Errorf("code=%d env=%q stdout=%q stderr=%q", code, env, out, errOut)
	}
}

func TestExecuteHelpFlagPrintsUsageToStdout(t *testing.T) {
	p := testProgram()
	code, env, out, errOut := execute(t, p, "greet", "--help")
	if code != 0 || env != "" || errOut != "" || out != p.Lookup("greet").Usage+"\n" {
		t.Errorf("code=%d env=%q stdout=%q stderr=%q", code, env, out, errOut)
	}
}

func TestExecuteUsageErrorPrintsErrorThenUsageToStderr(t *testing.T) {
	p := testProgram()
	code, env, out, errOut := execute(t, p, "greet")
	want := "greet: requires a name\n\n" + p.Lookup("greet").Usage + "\n"
	if code != 1 || env != "" || out != "" || errOut != want {
		t.Errorf("code=%d env=%q stdout=%q stderr=%q", code, env, out, errOut)
	}
}

func TestExecuteFlagErrorIsAUsageError(t *testing.T) {
	p := testProgram()
	code, _, _, errOut := execute(t, p, "greet", "--nope", "bob")
	want := "greet: flag provided but not defined: -nope\n\n" + p.Lookup("greet").Usage + "\n"
	if code != 1 || errOut != want {
		t.Errorf("code=%d stderr=%q", code, errOut)
	}
}

// Review Focus 4: a runtime failure is not a usage mistake.
func TestExecuteRunErrorPrintsNoUsage(t *testing.T) {
	code, _, out, errOut := execute(t, testProgram(), "greet", "fail")
	if code != 1 || out != "" || errOut != "greet failed\n" {
		t.Errorf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestExecuteHelpListsCommands(t *testing.T) {
	p := testProgram()
	want, _ := p.HelpText("")
	code, _, out, errOut := execute(t, p, "help")
	if code != 0 || out != want+"\n" || errOut != "" {
		t.Errorf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestExecuteHelpForOneCommand(t *testing.T) {
	p := testProgram()
	code, _, out, _ := execute(t, p, "help", "greet")
	if code != 0 || out != p.Lookup("greet").Usage+"\n" {
		t.Errorf("code=%d stdout=%q", code, out)
	}
}

// Ported from main's TestHelpHelpFlagExitsZero: `help --help` once fell
// through to `help: unknown command "--help"` and exit 1.
func TestExecuteHelpHelpFlagExitsZero(t *testing.T) {
	for _, arg := range []string{"-h", "--help", "-help"} {
		var e, out, errOut bytes.Buffer
		p := testProgram()
		code := p.Execute(p.Lookup("help"), []string{arg}, &e, &out, &errOut)
		if code != 0 || errOut.String() != "" {
			t.Errorf("help %s: code=%d stderr=%q", arg, code, errOut.String())
		}
		if out.String() != p.Lookup("help").Usage+"\n" {
			t.Errorf("help %s: stdout=%q", arg, out.String())
		}
	}
}

// Ported from main's TestParseHelpTooManyArguments.
func TestExecuteHelpTooManyArgumentsIsAUsageError(t *testing.T) {
	p := testProgram()
	code, _, _, errOut := execute(t, p, "help", "a", "b")
	want := "help: too many arguments\n\n" + p.Lookup("help").Usage + "\n"
	if code != 1 || errOut != want {
		t.Errorf("code=%d stderr=%q", code, errOut)
	}
}

// Review Focus 4: an unknown topic is a runtime error, as it was when
// helpText's error went straight to dispatch.
func TestExecuteHelpUnknownTopicIsARuntimeError(t *testing.T) {
	code, _, out, errOut := execute(t, testProgram(), "help", "nope")
	if code != 1 || out != "" || errOut != "help: unknown command \"nope\"\n" {
		t.Errorf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

// help never colorizes and never validated --color; that stays true.
func TestExecuteHelpIgnoresColor(t *testing.T) {
	if code, _, _, errOut := execute(t, testProgram(), "help", "--color=bogus"); code != 0 {
		t.Errorf("code=%d stderr=%q", code, errOut)
	}
}

// Ported from main's TestParseHelpTopic.
func TestHelpParsesTopic(t *testing.T) {
	c := &helpCmd[*bytes.Buffer]{}
	if err := Parse(c, "help", []string{"sync"}); err != nil {
		t.Fatal(err)
	}
	if c.topic != "sync" {
		t.Errorf("topic = %q, want sync", c.topic)
	}
}
```

- [ ] **Step 2: Run and confirm failure**

Run: `go test ./internal/cli/`
Expected: build failure, `undefined: NewProgram`, `undefined: Help`, and so on.

- [ ] **Step 3: Implement `internal/cli/program.go`**

```go
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// Spec describes one subcommand: what help says about it, and how to build
// it for an invocation.
type Spec[E any] struct {
	Name    string
	Summary string // one line, listed by `help`
	Usage   string // full block: `help <cmd>`, --help, and usage errors
	// New builds a fresh command for one invocation.
	New func() Command[E]
	// NeedsConfig is opaque to cli; main reads it before Execute. It marks
	// the commands that read or write data files, which are the only ones
	// that fail when the config directory cannot be resolved -- help,
	// version and folders must keep working on a machine with no usable
	// home directory.
	NeedsConfig bool

	// usageFor builds a built-in's usage from the program name. It is nil
	// for every command main defines.
	usageFor func(program string) string
}

// Program is the whole CLI surface: its commands, and the few strings help
// needs to describe it.
type Program[E any] struct {
	Name    string // the binary's name, as users type it
	Tagline string // follows "<Name> - " on the first line of general help
	Default string // the command that empty argv, or a leading flag, means
	Footer  string // the last line of general help
	// Commands is in help-listing order. The built-ins are listed wherever
	// Help() and Completion() are placed.
	Commands []Spec[E]
}

// NewProgram finishes a Program for use: built-ins get their usage from the
// program name, and every usage block except help's gains GlobalFlagHelp.
// The global flags are registered centrally, so they are documented
// centrally too -- a command must not be able to ship without them.
func NewProgram[E any](p Program[E]) *Program[E] {
	cmds := make([]Spec[E], len(p.Commands))
	copy(cmds, p.Commands)
	for i := range cmds {
		if cmds[i].usageFor != nil {
			cmds[i].Usage = cmds[i].usageFor(p.Name)
		}
		if cmds[i].Name != "help" {
			cmds[i].Usage += GlobalFlagHelp
		}
	}
	p.Commands = cmds
	return &p
}

// Lookup returns the named command, or nil.
func (p *Program[E]) Lookup(name string) *Spec[E] {
	for i := range p.Commands {
		if p.Commands[i].Name == name {
			return &p.Commands[i]
		}
	}
	return nil
}

// Resolve maps raw argv (without the program name) to a command and its
// arguments. Empty argv, or a leading flag, means the default command.
func (p *Program[E]) Resolve(args []string) (*Spec[E], []string, error) {
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help", "-help":
			return p.Lookup("help"), nil, nil
		}
	}
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		args = append([]string{p.Default}, args...)
	}
	s := p.Lookup(args[0])
	if s == nil {
		return nil, nil, fmt.Errorf(
			"unknown command %q\nRun `%s help` for usage.", args[0], p.Name)
	}
	return s, args[1:], nil
}

// HelpText renders the general help, or one command's usage block.
func (p *Program[E]) HelpText(topic string) (string, error) {
	if topic != "" {
		s := p.Lookup(topic)
		if s == nil {
			return "", fmt.Errorf("help: unknown command %q", topic)
		}
		return s.Usage, nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s - %s\n\n", p.Name, p.Tagline)
	fmt.Fprintf(&sb, "Usage:\n  %s [command] [flags]\n\n", p.Name)
	sb.WriteString("Commands:\n")
	for _, s := range p.Commands {
		fmt.Fprintf(&sb, "  %-11s %s\n", s.Name, s.Summary)
	}
	fmt.Fprintf(&sb, "\nRun `%s help <command>` for details on a command.\n", p.Name)
	sb.WriteString(p.Footer + "\n")
	return sb.String(), nil
}

// binder is implemented by the built-ins, which need the program itself and
// somewhere to print -- neither of which an env of type E can promise.
type binder[E any] interface {
	bind(p *Program[E], stdout io.Writer)
}

// Execute parses and runs one command, and returns the process exit code.
//
// The flag package reports -h/--help as flag.ErrHelp rather than a real
// failure, so `<command> --help` prints usage to stdout and succeeds. Any
// other parse error is a usage error: the message, a blank line and the usage
// go to stderr. A Run error is a runtime failure, printed bare -- no prefix
// and no usage, since the invocation itself was fine.
func (p *Program[E]) Execute(s *Spec[E], args []string, env E, stdout, stderr io.Writer) int {
	cmd := s.New()
	if b, ok := cmd.(binder[E]); ok {
		b.bind(p, stdout)
	}
	if err := Parse(cmd, s.Name, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(stdout, s.Usage)
			return 0
		}
		fmt.Fprintln(stderr, err)
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, s.Usage)
		return 1
	}
	if err := cmd.Run(env); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
```

- [ ] **Step 4: Implement `internal/cli/help.go`**

```go
package cli

import (
	"flag"
	"fmt"
	"io"
)

// Help is the built-in help command. Place it in Program.Commands where it
// should be listed.
func Help[E any]() Spec[E] {
	return Spec[E]{
		Name:    "help",
		Summary: "Show help for a command",
		usageFor: func(program string) string {
			return fmt.Sprintf("Usage: %s help [COMMAND]\n\n"+
				"Shows general help, or detailed help for one command.", program)
		},
		New: func() Command[E] { return &helpCmd[E]{} },
	}
}

type helpCmd[E any] struct {
	p      *Program[E]
	stdout io.Writer
	topic  string
}

func (c *helpCmd[E]) bind(p *Program[E], stdout io.Writer) { c.p, c.stdout = p, stdout }

// Flags registers nothing of help's own. Parsing still goes through the
// shared FlagSet, so -h/--help on help itself reaches the flag package's
// ErrHelp case instead of being mistaken for a topic named "--help". It also
// accepts --color without ever validating it, as it always has.
func (c *helpCmd[E]) Flags(*flag.FlagSet, *Globals) {}

func (c *helpCmd[E]) Parse(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("help: too many arguments")
	}
	if len(args) == 1 {
		c.topic = args[0]
	}
	return nil
}

func (c *helpCmd[E]) Run(E) error {
	out, err := c.p.HelpText(c.topic)
	if err != nil {
		return err
	}
	fmt.Fprintln(c.stdout, out)
	return nil
}
```

- [ ] **Step 5: Run tests**

Run: `go vet ./... && go test ./internal/cli/ -v -run 'Program|Resolve|HelpText|Execute|Help|Lookup'`
Expected: PASS for all tests in `program_test.go`.

- [ ] **Step 6: Full check and commit**

```bash
go test ./... && gofmt -l . && ../tools/behaviour-diff.sh .refactor-baseline/base
git add internal/cli
git commit -m "feat(cli): add Program, Execute and built-in help

Not wired into main yet; tested against a fake two-command program.
Part of #54.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01Cn2QECzyxmYVhLjvwZ7jts"
```

---

### Task 3: Every command becomes a type, bridged into the old table

`dispatch`, `handleParseErr`, `resolve`, the `commands` table and the `help` closure all stay. Every other table entry is now built from a `cli.Spec[app]` by a temporary adapter, `fromSpec`. The `run*` methods stay in `main.go` (Task 4 moves them).

**Files:**
- Create: `grammar.go`, `cmd_pick.go`, `cmd_reroll.go`, `cmd_list.go`, `cmd_favorite.go`, `cmd_open.go`, `cmd_history.go`, `cmd_stats.go`, `cmd_sync.go`, `cmd_folders.go`, `cmd_migrate.go`, `cmd_version.go`, `parsers_test.go`
- Modify: `cli.go` (shrinks to table + adapter + resolve/help/dispatch), `completion.go` (adds `completionSpec`, `completionCmd`; `parseCompletion` removed)
- Test: `cli_test.go`, `global_flags_test.go`, `completion_test.go` compile unchanged against the helpers in `parsers_test.go`

**Interfaces:**
- Consumes: `cli.Parser`, `cli.Command[app]`, `cli.Parse`, `cli.Spec[app]`, `cli.Globals` (Tasks 1–2)
- Produces (package main):
  - `type selectionCmd struct{ name string; sf *selectionFlags; g *cli.Globals; cfg selection }`, a `cli.Parser`
  - `type queryCmd struct{ name string; ff *filterFlags; g *cli.Globals; cfg favoriteConfig }`, a `cli.Parser`
  - `type noArgsCmd struct{ name string; g *cli.Globals }`, a `cli.Parser`
  - `func parseQueryCommand(name string, gf *cli.Globals, ff *filterFlags, rest []string) (favoriteConfig, error)`
  - Command types: `pickCmd`, `rerollCmd`, `listCmd` (each embeds `selectionCmd`), `favoriteCmd`, `unfavoriteCmd` (embed `queryCmd`), `openCmd`, `historyCmd`, `statsCmd`, `syncCmd`, `foldersCmd`, `migrateCmd`, `versionCmd` (last three embed `noArgsCmd`), `completionCmd`
  - Specs: `pickSpec`, `rerollSpec`, `listSpec`, `syncSpec`, `foldersSpec`, `historySpec`, `statsSpec`, `favoriteSpec`, `unfavoriteSpec`, `openSpec`, `migrateSpec`, `completionSpec`, `versionSpec`, all `cli.Spec[app]`
  - Test helpers (`parsers_test.go`), with exactly the old production signatures: `parseSelection`, `parseFavorite`, `parseOpen`, `parseHistory`, `parseStats`, `parseSync`, `parseNoArgs`, `parseCompletion`

- [ ] **Step 1: Write the Review Focus 2 test first**

Append to `cli_test.go`:

```go
// Review Focus 2 (#54): when a command gets two bad inputs, the one reported
// is part of its observable behaviour. Today the positional check runs
// before --color's; moving parsing into command types must keep that.
func TestParseSelectionReportsPositionalBeforeColor(t *testing.T) {
	_, err := parseSelection("list", []string{"extra", "--color", "sometimes"})
	if err == nil || err.Error() != `list: unexpected argument "extra"` {
		t.Errorf("err = %v, want the positional error first", err)
	}
}
```

Run: `go test -run TestParseSelectionReportsPositionalBeforeColor .`
Expected: PASS (it pins today's behaviour before the change).

- [ ] **Step 2: Create `grammar.go`**

Move these from `cli.go` **verbatim**, with their doc comments: `filterFlags`, `nonSubstringFilterFlag` and `registeredHelp`, `nonSubstringFilterFlags`, `addFilterFlags`, `(*filterFlags).Filter`, `nonEmpty`, `parseYearValues`, `anyNarrowing`, `hasQuery`, `queryValues`, `identifies`, `selection`, `favoriteConfig`, `selectionFlags`, `addSelectionFlags`, `filterFlagHelp`, `buildFilterFlagHelp`. Then add the shared parsers:

```go
// selectionCmd is the grammar pick, reroll and list share. Each embeds it and
// adds only its own Run.
type selectionCmd struct {
	name string
	sf   *selectionFlags
	g    *cli.Globals
	cfg  selection
}

func (c *selectionCmd) Flags(fs *flag.FlagSet, g *cli.Globals) {
	c.sf, c.g = addSelectionFlags(c.name, fs), g
}

func (c *selectionCmd) Parse(rest []string) error {
	name, sf := c.name, c.sf
	if len(rest) > 0 {
		return fmt.Errorf("%s: unexpected argument %q", name, rest[0])
	}
	filter, err := sf.filters.Filter()
	if err != nil {
		return fmt.Errorf("%s: %v", name, err)
	}
	color, err := c.g.Mode()
	if err != nil {
		return fmt.Errorf("%s: %v", name, err)
	}

	mode := pick.Fresh
	if sf.draw != nil {
		m, err := pick.ParseMode(*sf.draw)
		if err != nil {
			return fmt.Errorf("%s: %v", name, err)
		}
		mode = m
	}

	c.cfg = selection{
		favoritesOnly: *sf.favoritesOnly,
		unheard:       *sf.unheard,
		draw:          mode,
		filter:        filter,
		color:         color,
		json:          *sf.asJSON,
	}
	return nil
}

// queryCmd is the grammar favorite and unfavorite share: filter flags plus
// parseQueryCommand.
type queryCmd struct {
	name string
	ff   *filterFlags
	g    *cli.Globals
	cfg  favoriteConfig
}

func (c *queryCmd) Flags(fs *flag.FlagSet, g *cli.Globals) {
	c.ff, c.g = addFilterFlags(fs), g
}

func (c *queryCmd) Parse(rest []string) error {
	cfg, err := parseQueryCommand(c.name, c.g, c.ff, rest)
	c.cfg = cfg
	return err
}

// noArgsCmd is the grammar of a command that takes no flags and no arguments
// beyond the global ones.
type noArgsCmd struct {
	name string
	g    *cli.Globals
}

func (c *noArgsCmd) Flags(_ *flag.FlagSet, g *cli.Globals) { c.g = g }

func (c *noArgsCmd) Parse(rest []string) error {
	if len(rest) > 0 {
		return fmt.Errorf("%s: unexpected argument %q", c.name, rest[0])
	}
	// These commands produce no colorized output, but they still accept the
	// flag, so they must still reject a bad value for it.
	if _, err := c.g.Mode(); err != nil {
		return fmt.Errorf("%s: %v", c.name, err)
	}
	return nil
}
```

Move `parseQueryCommand` here too, keeping its doc comment, and change its signature to `func parseQueryCommand(name string, gf *cli.Globals, ff *filterFlags, rest []string) (favoriteConfig, error)`. Delete its first five lines (the `parseInterspersed` call and its error return); `cli.Parse` now does that step with the identical wrapping. Replace "The flag set arrives already built so each command can register its own flags on it first." in its comment with "Flags have already been parsed; rest is the positionals."

Delete from `cli.go`: `parseSelection`, `parseFavorite`, `parseNoArgs`.

- [ ] **Step 3: Create the command files**

Each spec's `Usage` and `Summary` are moved **verbatim** from that command's entry in `init()` (`cli.go:765–1112`). Each `NeedsConfig` matches the old `needsConfig`. The per-command config types and `add*Flags` functions move from `cli.go` into the matching file unchanged.

`cmd_pick.go`:

```go
package main

import "github.com/daniel-munoz/disc-fortune/v2/internal/cli"

var pickSpec = cli.Spec[app]{
	Name:        "pick",
	NeedsConfig: true,
	Summary:     "Print a random album (default)",
	Usage:       `Usage: disc-fortune pick [flags]` /* ...rest of the block, verbatim from init(), ending + filterFlagHelp */,
	New:         func() cli.Command[app] { return &pickCmd{selectionCmd{name: "pick"}} },
}

type pickCmd struct{ selectionCmd }

func (c *pickCmd) Run(a app) error { return a.runPick(c.cfg) }
```

(The `/* ... */` marks the one thing that is copied rather than retyped: the existing raw-string usage block. Copy it exactly, backticks and `+ filterFlagHelp` included.)

`cmd_reroll.go`: the same shape with `Name: "reroll"`, the reroll usage block, `New` returning `&rerollCmd{selectionCmd{name: "reroll"}}`, and

```go
type rerollCmd struct{ selectionCmd }

func (c *rerollCmd) Run(a app) error { return a.runReroll(c.cfg) }
```

`cmd_list.go`: `Name: "list"`, `New` returning `&listCmd{selectionCmd{name: "list"}}`, and

```go
type listCmd struct{ selectionCmd }

func (c *listCmd) Run(a app) error { return a.runList(c.cfg) }
```

`cmd_favorite.go`: two specs, `favoriteSpec` (`New`: `&favoriteCmd{queryCmd{name: "favorite"}}`) and `unfavoriteSpec` (`New`: `&unfavoriteCmd{queryCmd{name: "unfavorite"}}`), and

```go
type favoriteCmd struct{ queryCmd }

func (c *favoriteCmd) Run(a app) error { return a.runFavorite(c.cfg) }

type unfavoriteCmd struct{ queryCmd }

func (c *unfavoriteCmd) Run(a app) error { return a.runUnfavorite(c.cfg) }
```

`cmd_open.go`: move `openConfig` and `addOpenFlags` here, delete `parseOpen` from `cli.go`, and add

```go
var openSpec = cli.Spec[app]{ /* Name "open", NeedsConfig true, summary/usage verbatim */
	New: func() cli.Command[app] { return &openCmd{} },
}

// openCmd shares favorite's query grammar and adds --print.
type openCmd struct {
	printOnly *bool
	ff        *filterFlags
	g         *cli.Globals
	cfg       openConfig
}

func (c *openCmd) Flags(fs *flag.FlagSet, g *cli.Globals) {
	c.printOnly, c.ff = addOpenFlags(fs)
	c.g = g
}

func (c *openCmd) Parse(rest []string) error {
	q, err := parseQueryCommand("open", c.g, c.ff, rest)
	if err != nil {
		return err
	}
	c.cfg = openConfig{
		query:     q.query,
		filter:    q.filter,
		color:     q.color,
		printOnly: *c.printOnly,
	}
	return nil
}

func (c *openCmd) Run(a app) error { return a.runOpen(c.cfg) }
```

`cmd_history.go`: move `historyConfig`, `defaultHistoryLimit` and `addHistoryFlags` here, delete `parseHistory`, and add

```go
var historySpec = cli.Spec[app]{ /* Name "history", NeedsConfig true, summary/usage verbatim */
	New: func() cli.Command[app] { return &historyCmd{} },
}

type historyCmd struct {
	asJSON *bool
	g      *cli.Globals
	cfg    historyConfig
}

func (c *historyCmd) Flags(fs *flag.FlagSet, g *cli.Globals) {
	c.asJSON, c.g = addHistoryFlags(fs), g
}

func (c *historyCmd) Parse(rest []string) error {
	if len(rest) > 1 {
		return fmt.Errorf("history: too many arguments")
	}
	limit := defaultHistoryLimit
	if len(rest) == 1 {
		n, err := strconv.Atoi(strings.TrimSpace(rest[0]))
		if err != nil {
			return fmt.Errorf("history: requires a number (e.g., history 20)")
		}
		if n < 0 {
			return fmt.Errorf("history: count cannot be negative")
		}
		limit = n
	}
	color, err := c.g.Mode()
	if err != nil {
		return fmt.Errorf("history: %v", err)
	}
	c.cfg = historyConfig{limit: limit, color: color, json: *c.asJSON}
	return nil
}

func (c *historyCmd) Run(a app) error { return a.runHistory(c.cfg) }
```

`cmd_stats.go`: move `statsConfig`, `statsFlags` and `addStatsFlags` (with comments) here. Move `parseStats`'s doc comment (the one explaining why stats does not apply "filters require a query") onto `Parse`, delete `parseStats`, and add

```go
var statsSpec = cli.Spec[app]{ /* Name "stats", NeedsConfig true, summary/usage verbatim */
	New: func() cli.Command[app] { return &statsCmd{} },
}

type statsCmd struct {
	sf  *statsFlags
	g   *cli.Globals
	cfg statsConfig
}

func (c *statsCmd) Flags(fs *flag.FlagSet, g *cli.Globals) {
	c.sf, c.g = addStatsFlags(fs), g
}

func (c *statsCmd) Parse(rest []string) error {
	if len(rest) > 0 {
		return fmt.Errorf("stats: unexpected argument %q", rest[0])
	}
	filter, err := c.sf.filters.Filter()
	if err != nil {
		return fmt.Errorf("stats: %v", err)
	}
	color, err := c.g.Mode()
	if err != nil {
		return fmt.Errorf("stats: %v", err)
	}
	c.cfg = statsConfig{
		favoritesOnly: *c.sf.favoritesOnly,
		filter:        filter,
		color:         color,
		json:          *c.sf.asJSON,
	}
	return nil
}

func (c *statsCmd) Run(a app) error { return a.runStats(c.cfg) }
```

`cmd_sync.go`: move `syncConfig` and `addSyncFlags` here, delete `parseSync`, and add

```go
var syncSpec = cli.Spec[app]{ /* Name "sync", NeedsConfig true, summary/usage verbatim */
	New: func() cli.Command[app] { return &syncCmd{} },
}

type syncCmd struct {
	folders *arrayFlags
	g       *cli.Globals
	cfg     syncConfig
}

func (c *syncCmd) Flags(fs *flag.FlagSet, g *cli.Globals) {
	c.folders, c.g = addSyncFlags(fs), g
}

func (c *syncCmd) Parse(rest []string) error {
	if len(rest) > 0 {
		return fmt.Errorf("sync: unexpected argument %q", rest[0])
	}
	// sync colorizes nothing, but a bad --color value is still a typo and
	// must be reported rather than ignored.
	if _, err := c.g.Mode(); err != nil {
		return fmt.Errorf("sync: %v", err)
	}
	c.cfg = syncConfig{folders: *c.folders}
	return nil
}

func (c *syncCmd) Run(a app) error { return a.runSync(c.cfg) }
```

Check `parseSync` in `cli.go` before deleting it: if its body differs from the above in any check or message, copy its version instead. The rule is the old function's order and text, not this sketch.

`cmd_folders.go`, `cmd_migrate.go`, `cmd_version.go` (each `NeedsConfig` as before: folders false, migrate true, version false):

```go
var foldersSpec = cli.Spec[app]{ /* Name "folders", summary/usage verbatim */
	New: func() cli.Command[app] { return &foldersCmd{noArgsCmd{name: "folders"}} },
}

type foldersCmd struct{ noArgsCmd }

func (c *foldersCmd) Run(a app) error { return a.runFolders() }
```

```go
var migrateSpec = cli.Spec[app]{ /* Name "migrate", NeedsConfig true, summary/usage verbatim */
	New: func() cli.Command[app] { return &migrateCmd{noArgsCmd{name: "migrate"}} },
}

type migrateCmd struct{ noArgsCmd }

func (c *migrateCmd) Run(a app) error { return a.runMigrate() }
```

```go
var versionSpec = cli.Spec[app]{
	Name:    "version",
	Summary: "Print the version",
	Usage:   "Usage: disc-fortune version\n\nPrints the disc-fortune version and exits.",
	New:     func() cli.Command[app] { return &versionCmd{noArgsCmd{name: "version"}} },
}

type versionCmd struct{ noArgsCmd }

func (c *versionCmd) Run(a app) error {
	fmt.Fprintf(a.stdout, "disc-fortune %s\n", version)
	return nil
}
```

`completion.go` (root): replace `parseCompletion` with

```go
// completionSpec stays NeedsConfig false: generating a script reads no data
// files, so completion must keep working on a machine with no usable home
// directory.
var completionSpec = cli.Spec[app]{ /* Name "completion", summary/usage verbatim */
	New: func() cli.Command[app] { return &completionCmd{} },
}

type completionCmd struct {
	g     *cli.Globals
	shell string
}

func (c *completionCmd) Flags(_ *flag.FlagSet, g *cli.Globals) { c.g = g }

// Parse validates completion's single argument, the shell name.
func (c *completionCmd) Parse(rest []string) error {
	if len(rest) == 0 {
		return fmt.Errorf("completion: requires a shell (%s)", strings.Join(completionShells, ", "))
	}
	if len(rest) > 1 {
		return fmt.Errorf("completion: too many arguments")
	}
	// completion colorizes nothing, but it still accepts --color, so it must
	// still reject a bad value for it -- as every other command does.
	if _, err := c.g.Mode(); err != nil {
		return fmt.Errorf("completion: %v", err)
	}
	c.shell = rest[0]
	return nil
}

func (c *completionCmd) Run(a app) error { return runCompletion(a.stdout, c.shell) }
```

- [ ] **Step 4: Rebuild the table from specs in `cli.go`**

Add the adapter:

```go
// fromSpec adapts a command type to the table dispatch still reads. It is
// scaffolding for #54 and is deleted when cli.Program replaces the table.
func fromSpec(s cli.Spec[app]) command {
	return command{
		name:        s.Name,
		summary:     s.Summary,
		usage:       s.Usage,
		needsConfig: s.NeedsConfig,
		run: func(a app, args []string) error {
			c := s.New()
			if handleParseErr(s.Name, cli.Parse(c, s.Name, args)) {
				return nil
			}
			return c.Run(a)
		},
	}
}
```

Replace the body of `init()`'s `commands = []command{ ... }` with the following, keeping the `help` entry exactly as it is and the global-help loop after it unchanged:

```go
	commands = []command{
		fromSpec(pickSpec),
		fromSpec(rerollSpec),
		fromSpec(listSpec),
		fromSpec(syncSpec),
		fromSpec(foldersSpec),
		fromSpec(historySpec),
		fromSpec(statsSpec),
		fromSpec(favoriteSpec),
		fromSpec(unfavoriteSpec),
		fromSpec(openSpec),
		fromSpec(migrateSpec),
		fromSpec(completionSpec),
		fromSpec(versionSpec),
		{ /* the existing help entry, unchanged */ },
	}
```

The order is the help-listing order and must not change.

- [ ] **Step 5: Add the test helpers**

Create `parsers_test.go`:

```go
package main

import "github.com/daniel-munoz/disc-fortune/v2/internal/cli"

// These were the production parse functions before each command became a
// type (#54). They survive as test helpers with the same signatures, so the
// parsing tests read as they always have, while driving the real path:
// cli.Parse on the command's own Parser.

func parseSelection(name string, args []string) (selection, error) {
	c := &selectionCmd{name: name}
	if err := cli.Parse(c, name, args); err != nil {
		return selection{}, err
	}
	return c.cfg, nil
}

func parseFavorite(name string, args []string) (favoriteConfig, error) {
	c := &queryCmd{name: name}
	if err := cli.Parse(c, name, args); err != nil {
		return favoriteConfig{}, err
	}
	return c.cfg, nil
}

func parseOpen(args []string) (openConfig, error) {
	c := &openCmd{}
	if err := cli.Parse(c, "open", args); err != nil {
		return openConfig{}, err
	}
	return c.cfg, nil
}

func parseHistory(args []string) (historyConfig, error) {
	c := &historyCmd{}
	if err := cli.Parse(c, "history", args); err != nil {
		return historyConfig{}, err
	}
	return c.cfg, nil
}

func parseStats(args []string) (statsConfig, error) {
	c := &statsCmd{}
	if err := cli.Parse(c, "stats", args); err != nil {
		return statsConfig{}, err
	}
	return c.cfg, nil
}

func parseSync(args []string) (syncConfig, error) {
	c := &syncCmd{}
	if err := cli.Parse(c, "sync", args); err != nil {
		return syncConfig{}, err
	}
	return c.cfg, nil
}

func parseNoArgs(name string, args []string) error {
	return cli.Parse(&noArgsCmd{name: name}, name, args)
}

func parseCompletion(args []string) (string, error) {
	c := &completionCmd{}
	if err := cli.Parse(c, "completion", args); err != nil {
		return "", err
	}
	return c.shell, nil
}
```

- [ ] **Step 6: Verify**

```bash
go vet ./... && go test ./... && gofmt -l . && ../tools/behaviour-diff.sh .refactor-baseline/base
git diff --stat HEAD -- '*_test.go'
```

Expected: everything passes, the harness is all-match, and the only test-file changes are the new `parsers_test.go` and the one test appended in Step 1. If any existing test fails, the moved parse logic differs from the old function: compare against `git show HEAD:cli.go`, and do not touch the test.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor: give every command its own type and file

Each command's usage, flag registration and parsing now live together
in cmd_<name>.go; shared grammar lives in grammar.go. The old table is
built from the new specs through a temporary adapter, so dispatch and
handleParseErr are untouched. Part of #54.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01Cn2QECzyxmYVhLjvwZ7jts"
```

---

### Task 4: Move each `run*` method beside its command

A pure move. No line changes except imports.

**Files:**
- Modify: `main.go` (shrinks), `app.go`, `format.go`, `sync.go`, and every `cmd_*.go` from Task 3

**Interfaces:** none change. Everything keeps its name and signature.

- [ ] **Step 1: Move, verbatim, with doc comments**

| Symbol(s) | From | To |
|---|---|---|
| `errNoCollectionGuidance`, `errEmptyCollectionGuidance`, `errNoFavoritesGuidance` and the comment above the `var` block | `main.go` | `app.go` |
| `(app).collection`, `(app).favorites`, `(app).stdoutColor`, `(app).selectAlbums`, `(app).reportAmbiguous`, `describeSelection` | `main.go` | `app.go` |
| `formatMatch`, `formatList` | `main.go` | `format.go` |
| `recordMode` and its consts, `errNothingToReroll`, `errRerollRaced`, `(app).runPick`, `(app).drawAndRecord`, `(app).record` | `main.go` | `cmd_pick.go` |
| `(app).runReroll` | `main.go` | `cmd_reroll.go` |
| `(app).runList` | `main.go` | `cmd_list.go` |
| `(app).runHistory` | `main.go` | `cmd_history.go` |
| `(app).runStats` | `main.go` | `cmd_stats.go` |
| `(favoriteConfig).describe`, `(app).runFavorite`, `(app).runUnfavorite`, `(app).favoriteLastPick`, `(app).unfavoriteLastPick` | `main.go` | `cmd_favorite.go` |
| `(openConfig).describe`, `resolveOpenTarget`, `(app).runOpen` | `main.go` | `cmd_open.go` |
| `(app).runMigrate` | `main.go` | `cmd_migrate.go` |
| `(app).runSync` | `sync.go` | `cmd_sync.go` |
| `(app).runFolders` | `sync.go` | `cmd_folders.go` |

The `errNothingToReroll`/`errRerollRaced` pair lives in the `var (...)` block with the guidance errors. Split them out into their own `var (...)` block in `cmd_pick.go`.

After this step `main.go` holds only `version`, `discogsUserAgent`, `fatal` and `main`.

- [ ] **Step 2: Fix imports and verify it is only a move**

```bash
goimports -w *.go 2>/dev/null || true   # else fix imports by hand from go vet output
go vet ./... && go test ./... && gofmt -l . && ../tools/behaviour-diff.sh .refactor-baseline/base
git diff HEAD --color-moved=zebra --stat
```

Then review `git diff HEAD --color-moved=zebra`. Every removed and added block outside import lists should show as moved. Any line in normal add/delete colour (other than imports and split `var (...)` lines) is an accidental edit and must be reverted.

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "refactor: move each run method beside its command

Pure move; main.go keeps only version, the user agent, fatal and main.
Part of #54.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01Cn2QECzyxmYVhLjvwZ7jts"
```

---

### Task 5: `cli.Program` replaces the table, `run` replaces `dispatch`

**Files:**
- Modify: `main.go`, `completion.go` (reads `program.Commands` instead of `commands`), `main_test.go` (`TestHelperProcess`), `cli_test.go`, `global_flags_test.go`, `completion_test.go`, `env_conventions_test.go`
- Delete: `cli.go`

**Interfaces:**
- Consumes: `cli.NewProgram`, `cli.Program[app]`, `cli.Help[app]`, `(*cli.Program).Resolve/Lookup/HelpText/Execute` (Task 2); the specs (Task 3)
- Produces: `var program *cli.Program[app]`; `func run(args []string, stdout, stderr io.Writer) int`; `func resolve(args []string) (*cli.Spec[app], []string, error)`

- [ ] **Step 1: Write the new failing tests**

Append to `main_test.go`:

```go
// Review Focus 1 (#54): --help on every command, built-ins included, is a
// success that prints usage to stdout and nothing to stderr.
func TestEveryCommandHelpFlagPrintsUsageToStdout(t *testing.T) {
	home := t.TempDir()
	for _, c := range program.Commands {
		code, stdout, stderr := runHelperSplit(t, home, c.Name, "--help")
		if code != 0 {
			t.Errorf("%s --help: exit %d, want 0", c.Name, code)
		}
		if stdout != c.Usage+"\n" {
			t.Errorf("%s --help: stdout = %q, want its usage", c.Name, stdout)
		}
		if stderr != "" {
			t.Errorf("%s --help: stderr = %q, want empty", c.Name, stderr)
		}
	}
}

// Ported from TestHandleParseErrPrintsUsageOnHelpAndDoesNotExit and
// TestHandleParseErrWrappedHelpFlag. Those differed only in whether
// flag.ErrHelp arrived bare or wrapped; cli.Parse always wraps it now, so
// the two collapse into one test with both of their expectations.
func TestExecutePrintsUsageOnHelpFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	a := app{stdout: &out, stderr: &errOut}
	if code := program.Execute(program.Lookup("sync"), []string{"--help"}, a, &out, &errOut); code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "--folder") {
		t.Errorf("output missing sync usage text: %q", out.String())
	}
	if !strings.Contains(out.String(), "Usage: disc-fortune sync") {
		t.Errorf("output missing sync usage text: %q", out.String())
	}
}
```

Append to `env_conventions_test.go`:

```go
// Review Focus 3 (#54): config is resolved before arguments are parsed, so
// with no usable home directory a data command reports that, not the flag
// it was given. run() must keep that order.
func TestBinaryConfigFailureBeatsUsageError(t *testing.T) {
	code, stdout, stderr := runHelperEnv(t, t.TempDir(), []string{"HOME="}, "list", "--bogus-flag")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.HasPrefix(stderr, "disc-fortune: ") || strings.Contains(stderr, "bogus-flag") {
		t.Errorf("stderr = %q, want the config error and not the flag error", stderr)
	}
}
```

Run `go test -run 'TestBinaryConfigFailureBeatsUsageError' .` now. It should **PASS**, pinning today's order. The other three don't compile yet (`program` undefined); that's expected.

- [ ] **Step 2: Write `main.go`'s new wiring**

Add to `main.go` (imports gain `io`, `strings`, `internal/cli`, `internal/disc`, `internal/term` as needed), and delete `fatal`:

```go
// program is disc-fortune's whole command surface, in help-listing order.
var program = cli.NewProgram(cli.Program[app]{
	Name:    "disc-fortune",
	Tagline: "randomly picks a record from your Discogs collection",
	Default: "pick",
	Footer:  "With no command, disc-fortune picks a random album.",
	Commands: []cli.Spec[app]{
		pickSpec,
		rerollSpec,
		listSpec,
		syncSpec,
		foldersSpec,
		historySpec,
		statsSpec,
		favoriteSpec,
		unfavoriteSpec,
		openSpec,
		migrateSpec,
		completionSpec,
		versionSpec,
		cli.Help[app](),
	},
})

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the whole process: argv in, exit code out. Every exit code comes
// from here, which is what lets a test call it without losing the test
// binary to an os.Exit.
func run(args []string, stdout, stderr io.Writer) int {
	spec, rest, err := resolve(args)
	if err != nil {
		fmt.Fprintf(stderr, "disc-fortune: %v\n", err)
		return 1
	}
	// Resolved once, here, so every path helper on app can be infallible.
	// A failure is only fatal for the commands that actually need it.
	a, cfgErr := newApp(os.Getenv, os.UserHomeDir, stdout, stderr)
	if cfgErr != nil {
		if spec.NeedsConfig {
			fmt.Fprintf(stderr, "disc-fortune: %v\n", cfgErr)
			return 1
		}
	} else if spec.NeedsConfig {
		fmt.Fprint(a.stderr, disc.MigrationNotice(a.loc, a.metaPath(), term.IsTTY(os.Stderr)))
	}
	return program.Execute(spec, rest, a, stdout, stderr)
}
```

Move `v1Signposts` (with its comment) from `cli.go` into `main.go`, and add:

```go
// resolve maps raw argv to a command. The -v and v1-flag signposts are
// disc-fortune's own history, so they are checked here before the generic
// resolution in cli.
func resolve(args []string) (*cli.Spec[app], []string, error) {
	if len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-v", "--version", "-version":
			return nil, nil, fmt.Errorf(
				"there is no %s flag; use `disc-fortune version`", args[0])
		}
		if target, ok := v1Signposts[strings.TrimLeft(args[0], "-")]; ok {
			return nil, nil, fmt.Errorf(
				"%s is now `%s` (see RELEASE_NOTES_v2.0.0.md)", args[0], target)
		}
	}
	return program.Resolve(args)
}
```

Copy both error strings from the old `resolve` in `cli.go`; do not retype them.

- [ ] **Step 3: Delete `cli.go`**

Everything left in it (`command`, `commands`, `fromSpec`, `handleParseErr`, `lookup`, the old `resolve`, `helpText`, `init`, `parseHelp`, `dispatch`) is now replaced. Delete the file.

In root `completion.go`, change `range commands` to `range program.Commands` and `c.name`/`c.summary` to `c.Name`/`c.Summary` (in `commandNames`, `bashCompletion`, `zshCompletion`, `fishCompletion`).

- [ ] **Step 4: Point the tests at the program**

```bash
perl -pi -e 's/\brange commands\b/range program.Commands/g; s/\blen\(commands\)/len(program.Commands)/g; s/\blookup\(/program.Lookup(/g; s/\bhelpText\(/program.HelpText(/g; s/\b(c|cmd)\.(name|summary|usage)\b/$1.\u$2/g; s/\bc\.run == nil\b/c.New == nil/g' cli_test.go global_flags_test.go completion_test.go
```

Then by hand:
- In `cli_test.go`, delete `TestParseHelpHelpFlagIsErrHelp`, `TestParseHelpTopic`, `TestParseHelpTooManyArguments`, `TestHelpHelpFlagExitsZero` (ported in Task 2), the three `TestHandleParseErr*` tests (ported in Step 1, and the nil case is dropped per spec amendment 5), `captureStdout`, and the comment block above `TestParseSelectionHelpFlagIsErrHelp` that mentions `handleParseErr`. Reword that comment to say the wrapping is what lets `Execute` recognise help. Remove the imports the deletions orphan (`bytes`, `io`, `os` if unused).
- In `cli_test.go`, `TestEveryCommandIsDocumented`: `c.New == nil` now reads "has no run function"; change only the message word "run function" to "constructor". The expectation itself is unchanged.
- In `main_test.go`, `TestHelperProcess`: replace `dispatch(args)` and the trailing `os.Exit(0)` (with its comment) by `os.Exit(run(args, os.Stdout, os.Stderr))`. Rewrite the "Exit-code coverage" comment block above it: commands return errors, `run` turns them into an exit code, and the subprocess pattern remains because many tests assert on a real process's streams. Add `bytes` to imports for the Step 1 tests.
- In `env_conventions_test.go:165`, change "in dispatch" to "in run".

- [ ] **Step 5: Verify**

```bash
go vet ./... && go test ./... && gofmt -l . && ../tools/behaviour-diff.sh .refactor-baseline/base
grep -n "os.Exit" *.go internal/cli/*.go | grep -v _test.go
```

Expected: everything passes, harness all-match, and `os.Exit` appears only in `main()` in `main.go`.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: replace the command table and dispatch with cli.Program

main() is now os.Exit(run(...)); run is the only place an exit code is
decided, and handleParseErr's direct os.Exit is gone. cli.go is
deleted. Part of #54.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01Cn2QECzyxmYVhLjvwZ7jts"
```

---

### Task 6: Completion moves into `cli`, enumerated through `Flags`

**Files:**
- Create: `internal/cli/completion.go`, `internal/cli/completion_test.go`
- Modify: `internal/cli/program.go` (adds `FlagValues`, `FlagInfo`, `Flags`), `main.go` (`FlagValues`, `cli.Completion[app]()`), `completion_test.go`, `global_flags_test.go`, `parsers_test.go`
- Delete: root `completion.go`

**Interfaces:**
- Produces:
  - `Program[E].FlagValues map[string][]string`
  - `type FlagInfo struct{ Name string; IsBool bool; Values []string }`
  - `func (p *Program[E]) Flags(name string) []FlagInfo`
  - `func Completion[E any]() Spec[E]`
  - `var CompletionShells = []string{"bash", "zsh", "fish"}`
  - `func (p *Program[E]) CompletionScript(shell string) (string, error)`

- [ ] **Step 1: Write the failing tests**

Create `internal/cli/completion_test.go`:

```go
package cli

import (
	"bytes"
	"testing"
)

func completionProgram() *Program[*bytes.Buffer] {
	return NewProgram(Program[*bytes.Buffer]{
		Name: "tool", Tagline: "does things", Default: "greet", Footer: "x",
		Commands: []Spec[*bytes.Buffer]{
			{Name: "greet", Summary: "Say hello", Usage: greetUsage,
				New: func() Command[*bytes.Buffer] { return &greetCmd{} }},
			Completion[*bytes.Buffer](),
			Help[*bytes.Buffer](),
		},
		FlagValues: map[string][]string{"color": {"auto", "always", "never"}},
	})
}

// Flags enumerates through the same Flags method parsing uses, so a flag a
// command registers is offered without anyone updating a list.
func TestFlagsComeFromTheCommandsOwnRegistration(t *testing.T) {
	got := completionProgram().Flags("greet")
	if len(got) != 2 || got[0].Name != "color" || got[1].Name != "loud" {
		t.Fatalf("Flags(greet) = %+v, want [color loud] sorted", got)
	}
	if got[0].IsBool || !got[1].IsBool {
		t.Errorf("IsBool wrong: %+v", got)
	}
	if want := []string{"auto", "always", "never"}; len(got[0].Values) != 3 || got[0].Values[0] != want[0] {
		t.Errorf("color values = %v, want %v", got[0].Values, want)
	}
}

func TestFlagsOfUnknownCommandAreTheGlobals(t *testing.T) {
	if got := completionProgram().Flags("\x00none"); len(got) != 1 || got[0].Name != "color" {
		t.Errorf("Flags(unknown) = %+v, want only --color", got)
	}
}

// Ported from main's TestParseCompletionRequiresAShell.
func TestCompletionRequiresOneShell(t *testing.T) {
	if err := Parse(&completionCmd[*bytes.Buffer]{}, "completion", nil); err == nil {
		t.Error("completion with no argument should fail")
	}
	if err := Parse(&completionCmd[*bytes.Buffer]{}, "completion", []string{"bash", "zsh"}); err == nil {
		t.Error("completion with two arguments should fail")
	}
	c := &completionCmd[*bytes.Buffer]{}
	if err := Parse(c, "completion", []string{"fish"}); err != nil {
		t.Fatalf("Parse([fish]): %v", err)
	}
	if c.shell != "fish" {
		t.Errorf("shell = %q, want fish", c.shell)
	}
}

// Ported from main's TestCompletionRejectsInvalidColor.
func TestCompletionRejectsInvalidColor(t *testing.T) {
	if err := Parse(&completionCmd[*bytes.Buffer]{}, "completion", []string{"--color", "sometimes", "bash"}); err == nil {
		t.Error("completion accepted --color=sometimes, want an error")
	}
	if err := Parse(&completionCmd[*bytes.Buffer]{}, "completion", []string{"--color", "never", "bash"}); err != nil {
		t.Errorf("completion rejected a valid --color: %v", err)
	}
}

func TestCompletionUnknownShellIsARuntimeError(t *testing.T) {
	code, _, out, errOut := execute(t, completionProgram(), "completion", "tcsh")
	if code != 1 || out != "" || errOut != "Error: unsupported shell \"tcsh\" (want bash, zsh, fish)\n" {
		t.Errorf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestCompletionScriptsUseTheProgramName(t *testing.T) {
	p := completionProgram()
	for _, shell := range CompletionShells {
		script, err := p.CompletionScript(shell)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains([]byte(script), []byte("disc-fortune")) {
			t.Errorf("%s script hard-codes disc-fortune", shell)
		}
		if !bytes.Contains([]byte(script), []byte("tool")) {
			t.Errorf("%s script never names the program", shell)
		}
	}
}
```

Append to root `completion_test.go`:

```go
// Review Focus 5 (#54): the built-ins build their usage from the program
// name. For disc-fortune it must equal what was written out by hand before.
func TestBuiltinUsageMatchesTheOldText(t *testing.T) {
	if got, want := program.Lookup("help").Usage,
		"Usage: disc-fortune help [COMMAND]\n\nShows general help, or detailed help for one command."; got != want {
		t.Errorf("help usage = %q, want %q", got, want)
	}
	if got := program.Lookup("completion").Usage; got != oldCompletionUsage+cli.GlobalFlagHelp {
		t.Errorf("completion usage drifted:\n%q", got)
	}
}
```

Define `oldCompletionUsage` in that test file as a raw string copied **verbatim** from `completionSpec.Usage` in root `completion.go`. Copy it before Step 3 deletes that file.

Run: `go test ./internal/cli/ .`
Expected: build failure (`undefined: Completion`, `FlagValues`, `CompletionShells`, ...).

- [ ] **Step 2: Add `Flags` to `internal/cli/program.go`**

Add the field to `Program` (after `Commands`):

```go
	// FlagValues holds the flags whose accepted values are compiled into the
	// binary, for completion to offer. Completing them costs nothing -- no
	// file is read and no process is forked -- which is why they are here and
	// collection-derived values such as --genre are not.
	FlagValues map[string][]string
```

and:

```go
// FlagInfo is one flag as completion needs to see it.
type FlagInfo struct {
	Name   string
	IsBool bool     // a bool flag takes no value, so nothing follows it
	Values []string // compiled-in values, if any
}

// Flags returns the flags the named command accepts, sorted by name so
// generated scripts are byte-stable. It calls the command's own Flags on a
// scratch FlagSet -- the call real parsing makes -- so a flag cannot be
// accepted without also being completable. An unknown name yields the
// global flags alone.
func (p *Program[E]) Flags(name string) []FlagInfo {
	fs, g := NewFlagSet(name)
	if s := p.Lookup(name); s != nil {
		s.New().Flags(fs, g)
	}
	var out []FlagInfo
	fs.VisitAll(func(f *flag.Flag) {
		out = append(out, FlagInfo{Name: f.Name, IsBool: isBoolFlag(f), Values: p.FlagValues[f.Name]})
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// isBoolFlag reports whether a flag is satisfied by its presence alone. The
// flag package marks these with an unexported interface that its own parser
// uses; asking the same question here keeps completion from suggesting a
// value where none is taken.
func isBoolFlag(f *flag.Flag) bool {
	bf, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && bf.IsBoolFlag()
}
```

(`sort` joins the imports.)

- [ ] **Step 3: Create `internal/cli/completion.go` from root `completion.go`**

Move into `package cli`, with doc comments, then adapt:

| Root symbol | Becomes |
|---|---|
| `completionShells` | `CompletionShells` |
| `completionFlag` | deleted, replaced by `FlagInfo` |
| `commandFlagSet`, `commandFlags`, `isBoolFlag`, `flagValues` | deleted (replaced by `Program.Flags` / `FlagValues`) |
| `completionScript(shell)` | `func (p *Program[E]) CompletionScript(shell string) (string, error)` |
| `bashCompletion()`, `zshCompletion()`, `fishCompletion()` | methods on `*Program[E]` |
| `flagNames(name)`, `commandNames()`, `freeValueFlags(command)`, `fishValueSpec(f)` | methods on `*Program[E]` (except `fishValueSpec`, which stays a function taking `FlagInfo`) |
| `shellQuote`, `sortedKeys` | unchanged functions |
| `runCompletion` | folded into `completionCmd.Run` |
| `completionSpec`, `completionCmd` | `Completion[E]()` and `completionCmd[E]` below |

Inside the moved generators: `commandFlags(x)` becomes `p.Flags(x)`, `flagValues` becomes `p.FlagValues`, `range program.Commands` becomes `range p.Commands`, `f.name`/`f.isBool`/`f.values` become `f.Name`/`f.IsBool`/`f.Values`, and the implicit-command fallback `"pick"` becomes `p.Default`.

**The program name.** Replace every `disc-fortune` with `p.Name`, and every `_disc_fortune` with `fn`, computed at the top of each generator as:

```go
	fn := "_" + strings.ReplaceAll(p.Name, "-", "_")
```

Raw-string blocks that contain the name become `fmt.Sprintf`/`fmt.Fprintf` calls. Use explicit argument indexes (`%[1]s` for name, `%[2]s` for fn) wherever one block mentions either more than once. Check each converted block for a literal `%` it already contained, and escape any as `%%`. Done when:

```bash
grep -c 'disc-fortune\|disc_fortune' internal/cli/completion.go   # expect 0
```

The built-in:

```go
// Completion is the built-in completion command. Place it in
// Program.Commands where it should be listed.
func Completion[E any]() Spec[E] {
	return Spec[E]{
		Name:     "completion",
		Summary:  "Print a shell completion script",
		usageFor: completionUsage,
		New:      func() Command[E] { return &completionCmd[E]{} },
	}
}

// completionUsage is completion's usage block. It is built from the program
// name because the load instructions quote the command users type.
func completionUsage(program string) string {
	return fmt.Sprintf(`Usage: %[1]s completion SHELL
...`, program)
}
```

Fill `completionUsage`'s body from the old `completionSpec.Usage`, with each `disc-fortune` replaced by `%[1]s`. `TestBuiltinUsageMatchesTheOldText` proves the result.

```go
type completionCmd[E any] struct {
	p      *Program[E]
	stdout io.Writer
	g      *Globals
	shell  string
}

func (c *completionCmd[E]) bind(p *Program[E], stdout io.Writer) { c.p, c.stdout = p, stdout }

func (c *completionCmd[E]) Flags(_ *flag.FlagSet, g *Globals) { c.g = g }
```

`Parse` is the root `completionCmd.Parse` body unchanged (with `completionShells` → `CompletionShells`). Then:

```go
func (c *completionCmd[E]) Run(E) error {
	script, err := c.p.CompletionScript(c.shell)
	if err != nil {
		return fmt.Errorf("Error: %v", err)
	}
	fmt.Fprint(c.stdout, script)
	return nil
}
```

Delete root `completion.go`.

- [ ] **Step 4: Wire `main.go`**

In `program`'s literal, replace `completionSpec` with `cli.Completion[app]()` (same position), and add, with the old `flagValues` doc comment adapted:

```go
	FlagValues: map[string][]string{
		"draw":  {"fresh", "any", "stale"},
		"color": {"auto", "always", "never"},
	},
```

- [ ] **Step 5: Point root tests at the new API**

```bash
perl -pi -e 's/\bcommandFlags\(/program.Flags(/g; s/\[\]completionFlag\b/[]cli.FlagInfo/g; s/\bf\.name\b/f.Name/g; s/\bf\.isBool\b/f.IsBool/g; s/\bflagValues\b/program.FlagValues/g; s/\bcompletionScript\(/program.CompletionScript(/g; s/\bcompletionShells\b/cli.CompletionShells/g' completion_test.go
```

Then by hand:
- Delete `TestEveryCommandHasACompletionDecision` (spec: its switch is gone), `TestParseCompletionRequiresAShell` and `TestCompletionRejectsInvalidColor` (ported in Step 1).
- Delete `parseCompletion` from `parsers_test.go`.
- In `global_flags_test.go`, the `"completion"` entry of `TestEveryCommandAcceptsColorFlag` becomes:
  ```go
		"completion": func(a []string) error {
			return cli.Parse(program.Lookup("completion").New(), "completion", append([]string{"bash"}, a...))
		},
  ```
- Fix the comment at the top of `TestCompletionOffersOnlyFlagsTheCommandAccepts` ("share one FlagSet builder") to say enumeration calls the command's own `Flags`.

- [ ] **Step 6: Verify**

```bash
go vet ./... && go test ./... && gofmt -l . && ../tools/behaviour-diff.sh .refactor-baseline/base
```

Expected: all pass, and the harness matches on `completion bash|zsh|fish` plus every completion error probe. If a script probe mismatches, run `diff <(.refactor-baseline/base completion bash) <(go run . completion bash)` to find the bad substitution.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor: move completion into internal/cli

Completion now enumerates flags by calling each command's own Flags,
so the hand-maintained per-command switch and its guard test are gone.
Part of #54.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01Cn2QECzyxmYVhLjvwZ7jts"
```

---

### Task 7: Tidy-up and the record

**Files:**
- Modify: `grammar.go`, `cmd_history.go`, `cmd_stats.go`, `cmd_open.go`, `cmd_sync.go`, `internal/cli/flags.go`, `internal/cli/program.go`, `docs/plans/2026-09-06-package-structure-design.md`, any file the grep below finds

- [ ] **Step 1: Inline the single-use flag registration helpers**

`addHistoryFlags`, `addStatsFlags`, `addOpenFlags` and `addSyncFlags` existed so completion's switch could call them. Each is now called only from its command's `Flags`. Inline each body into that `Flags` method and delete the function and its "See addSelectionFlags for why registration is factored out" comment. Keep `statsFlags` (a struct with a real doc comment). Keep `addSelectionFlags` and `addFilterFlags`: several commands share them. Rewrite `selectionFlags`'s doc comment: completion now enumerates through `Flags`, not through a shared registration function.

- [ ] **Step 2: Check `cli`'s exported surface**

```bash
go doc -all ./internal/cli | grep -E '^(func|type|var|const)'
```

Expected exports: `Parser`, `Command`, `Parse`, `Globals` (+`Mode`), `NewFlagSet`, `ParseInterspersed`, `GlobalFlagHelp`, `Spec`, `Program` (+`Lookup`, `Resolve`, `HelpText`, `Execute`, `Flags`, `CompletionScript`), `NewProgram`, `FlagInfo`, `Help`, `Completion`, `CompletionShells`. `NewFlagSet`, `ParseInterspersed` and `GlobalFlagHelp` stay exported even though `main`'s production code no longer calls them, because root tests do: filter-flag tests build ad-hoc flag sets, and `TestBuiltinUsageMatchesTheOldText` compares against the global help. Add a sentence to each of their doc comments saying so. Anything else exported that is not in this list should be unexported.

- [ ] **Step 3: Remove stale references**

```bash
grep -rn "handleParseErr\|dispatch\|commandFlagSet\|newFlagSet\|parseInterspersed\|globalFlags\|fromSpec\|init()" --include=*.go .
```

Expected matches: none in production code. Comments in tests that describe history ("before #54 ...") may stay if they say so; reword any that describe the current code wrongly.

- [ ] **Step 4: Point the September design at this one**

In `docs/plans/2026-09-06-package-structure-design.md`, under `### Deliberately unmoved`, append:

```markdown
**Update (2026-10-05):** done in #54. See
`2026-10-05-cli-untangle-design.md`. `cli.go` and the root `completion.go`
no longer exist; the framework is `internal/cli`.
```

- [ ] **Step 5: Final verification**

```bash
go vet ./... && go test ./... && gofmt -l .
GOOS=windows go build ./... && GOOS=linux go build ./...
../tools/behaviour-diff.sh .refactor-baseline/base
grep -n "os.Exit" $(git ls-files '*.go' | grep -v _test.go)        # only main.go's main()
go list -deps ./internal/cli | grep disc-fortune                     # only internal/cli and internal/term
ls *.go | grep -v _test.go                                            # matches the file map
```

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: tidy after the cli extraction

Inline single-use flag helpers, unexport cli's internals, drop stale
references, and record #54 against the package-structure design.
Closes #54.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01Cn2QECzyxmYVhLjvwZ7jts"
```
