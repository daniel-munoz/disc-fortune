package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/daniel-munoz/disc-fortune/v2/internal/cli"
	"github.com/daniel-munoz/disc-fortune/v2/internal/pick"
	"github.com/daniel-munoz/disc-fortune/v2/internal/term"
)

// The guard that makes "completion is generated, not hardcoded" true rather
// than aspirational. Enumeration calls each command's own Flags, the same
// call parsing makes, so they cannot diverge by construction -- but
// construction is not proof that the enumeration reaches the real parser.
// This drives every completed flag through the actual parse function and
// fails if any is rejected as unknown.
func TestCompletionOffersOnlyFlagsTheCommandAccepts(t *testing.T) {
	for _, c := range program.Commands {
		for _, f := range program.Flags(c.Name) {
			args := []string{"--" + f.Name}
			if !f.IsBool {
				args = append(args, sampleValue(f.Name))
			}

			var err error
			switch c.Name {
			case "pick", "reroll", "list":
				_, err = parseSelection(c.Name, args)
			case "history":
				_, err = parseHistory(args)
			case "stats":
				_, err = parseStats(args)
			case "favorite", "unfavorite":
				// These need a query beside a narrowing filter.
				_, err = parseFavorite(c.Name, append([]string{"miles"}, args...))
			case "open":
				// Shares favorite's query grammar, so it needs one too.
				_, err = parseOpen(append([]string{"miles"}, args...))
			case "sync":
				_, err = parseSync(args)
			default:
				err = parseNoArgs(c.Name, args)
			}
			if err != nil && strings.Contains(err.Error(), "not defined") {
				t.Errorf("%s completes --%s but the command rejects it: %v", c.Name, f.Name, err)
			}
		}
	}
}

// sampleValue returns something the named flag will accept, so the test above
// exercises parsing rather than tripping over a validation error it does not
// care about.
func sampleValue(name string) string {
	switch name {
	case "year", "exclude-year":
		return "1975"
	case "decade", "exclude-decade":
		return "70s"
	case "release-id":
		return "1839278"
	case "draw":
		return "fresh"
	case "color":
		return "auto"
	}
	return "x"
}

func TestCompletionKnowsEveryCommand(t *testing.T) {
	for _, shell := range cli.CompletionShells {
		script, err := program.CompletionScript(shell)
		if err != nil {
			t.Fatalf("program.CompletionScript(%q): %v", shell, err)
		}
		for _, c := range program.Commands {
			if !strings.Contains(script, c.Name) {
				t.Errorf("%s script does not mention the %q command", shell, c.Name)
			}
		}
	}
}

// pick draws, list does not. The scripts must reflect that, or completion
// would suggest a flag the command rejects.
func TestCompletionScopesFlagsPerCommand(t *testing.T) {
	if !hasFlag(program.Flags("pick"), "draw") {
		t.Error("pick should complete --draw")
	}
	if hasFlag(program.Flags("list"), "draw") {
		t.Error("list should not complete --draw: it draws nothing")
	}
	for _, name := range []string{"pick", "list", "history", "stats"} {
		if !hasFlag(program.Flags(name), "json") {
			t.Errorf("%s should complete --json", name)
		}
	}
	for _, name := range []string{"sync", "folders", "migrate", "open"} {
		if hasFlag(program.Flags(name), "json") {
			t.Errorf("%s should not complete --json: it does not accept it", name)
		}
	}
	if !hasFlag(program.Flags("sync"), "folder") {
		t.Error("sync should complete --folder")
	}
	// --color is global, so every command gets it.
	for _, c := range program.Commands {
		if !hasFlag(program.Flags(c.Name), "color") {
			t.Errorf("%s should complete the global --color", c.Name)
		}
	}
}

func hasFlag(flags []cli.FlagInfo, name string) bool {
	for _, f := range flags {
		if f.Name == name {
			return true
		}
	}
	return false
}

// The enum values are compiled in, so completing them costs nothing. This
// pins them to what the parsers actually accept rather than to a comment.
func TestCompletionEnumValuesAreAccepted(t *testing.T) {
	for _, v := range program.FlagValues["draw"] {
		if _, err := pick.ParseMode(v); err != nil {
			t.Errorf("completion offers --draw %q but pick.ParseMode rejects it: %v", v, err)
		}
	}
	for _, v := range program.FlagValues["color"] {
		if _, err := term.ParseMode(v); err != nil {
			t.Errorf("completion offers --color %q but term.ParseMode rejects it: %v", v, err)
		}
	}
	if _, err := pick.ParseMode("nonsense"); err == nil {
		t.Error("pick.ParseMode accepted nonsense; the test above proves nothing")
	}
	if _, err := term.ParseMode("nonsense"); err == nil {
		t.Error("term.ParseMode accepted nonsense; the test above proves nothing")
	}
}

func TestCompletionEnumValuesReachTheScripts(t *testing.T) {
	for _, shell := range cli.CompletionShells {
		script, err := program.CompletionScript(shell)
		if err != nil {
			t.Fatalf("program.CompletionScript(%q): %v", shell, err)
		}
		for _, v := range append(program.FlagValues["draw"], program.FlagValues["color"]...) {
			if !strings.Contains(script, v) {
				t.Errorf("%s script is missing the enum value %q", shell, v)
			}
		}
	}
}

func TestCompletionRejectsUnknownShell(t *testing.T) {
	for _, shell := range []string{"", "tcsh", "powershell", "BASH"} {
		if _, err := program.CompletionScript(shell); err == nil {
			t.Errorf("program.CompletionScript(%q) succeeded, want an error", shell)
		}
	}
}

// A script that is syntactically broken is the failure mode most likely to
// escape review, because it only shows up in a shell nobody ran. Each shell
// is skipped when it is not installed rather than failing the suite.
func TestGeneratedScriptsAreSyntacticallyValid(t *testing.T) {
	checks := map[string][]string{
		"bash": {"bash", "-n"},
		"zsh":  {"zsh", "-n"},
		"fish": {"fish", "--no-execute"},
	}

	if len(checks) != len(cli.CompletionShells) {
		t.Fatalf("%d shells are generated but %d are syntax-checked; add the new one",
			len(cli.CompletionShells), len(checks))
	}

	for shell, check := range checks {
		t.Run(shell, func(t *testing.T) {
			bin, err := exec.LookPath(check[0])
			if err != nil {
				t.Skipf("%s not installed", check[0])
			}
			script, err := program.CompletionScript(shell)
			if err != nil {
				t.Fatalf("program.CompletionScript(%q): %v", shell, err)
			}

			path := t.TempDir() + "/completion." + shell
			if err := os.WriteFile(path, []byte(script), 0644); err != nil {
				t.Fatalf("writing script: %v", err)
			}
			out, err := exec.Command(bin, append(check[1:], path)...).CombinedOutput()
			if err != nil {
				t.Errorf("%s rejected the generated script: %v\n%s\n--- script ---\n%s",
					shell, err, out, script)
			}
		})
	}
}

// Syntax checking cannot catch a script that loads cleanly and then completes
// the wrong thing. These drive a real shell's completion engine and assert on
// what it offers. fish in particular defaults to filename completion, so
// before `complete -c disc-fortune -f` was emitted, tabbing a bare
// `disc-fortune ` listed the current directory instead of the subcommands --
// a bug every syntax check passed.
func TestCompletionOffersCommandsNotFilenames(t *testing.T) {
	bin := buildForCompletion(t)

	t.Run("fish", func(t *testing.T) {
		fish, err := exec.LookPath("fish")
		if err != nil {
			t.Skip("fish not installed")
		}
		out, err := exec.Command(fish, "-c",
			bin+" completion fish | source; complete -C 'disc-fortune '").CombinedOutput()
		if err != nil {
			t.Fatalf("fish: %v\n%s", err, out)
		}
		got := string(out)
		if !strings.Contains(got, "pick") {
			t.Errorf("fish did not offer the pick command:\n%s", got)
		}
		if strings.Contains(got, ".go") {
			t.Errorf("fish fell back to filename completion:\n%s", got)
		}
	})

	// The subtler half of the same bug: -f stops fish offering files for the
	// command, but not for an option's argument. Only -x does that, and
	// --genre is exactly where we promise to offer nothing rather than
	// something wrong.
	t.Run("fish offers nothing for a collection-derived value", func(t *testing.T) {
		fish, err := exec.LookPath("fish")
		if err != nil {
			t.Skip("fish not installed")
		}
		for _, flag := range []string{"--genre", "--label"} {
			out, err := exec.Command(fish, "-c",
				bin+" completion fish | source; complete -C 'disc-fortune list "+flag+" '").CombinedOutput()
			if err != nil {
				t.Fatalf("fish: %v\n%s", err, out)
			}
			if got := strings.TrimSpace(string(out)); got != "" {
				t.Errorf("%s should complete nothing, got:\n%s", flag, got)
			}
		}
	})

	// A leading flag means the implicit pick, so the no-subcommand position
	// must offer pick's flags rather than only the globals.
	t.Run("bash completes the implicit pick", func(t *testing.T) {
		bash, err := exec.LookPath("bash")
		if err != nil {
			t.Skip("bash not installed")
		}
		script := `eval "$(` + bin + ` completion bash)"
COMP_WORDS=(disc-fortune --); COMP_CWORD=1; _disc_fortune; echo "${COMPREPLY[@]}"`
		out, err := exec.Command(bash, "-c", script).CombinedOutput()
		if err != nil {
			t.Fatalf("bash: %v\n%s", err, out)
		}
		for _, want := range []string{"--draw", "--favorites", "--genre"} {
			if !strings.Contains(string(out), want) {
				t.Errorf("a leading flag is a pick, so %s should be offered:\n%s", want, out)
			}
		}
	})

	t.Run("bash", func(t *testing.T) {
		bash, err := exec.LookPath("bash")
		if err != nil {
			t.Skip("bash not installed")
		}
		script := `eval "$(` + bin + ` completion bash)"
COMP_WORDS=(disc-fortune ""); COMP_CWORD=1; _disc_fortune; echo "${COMPREPLY[@]}"`
		out, err := exec.Command(bash, "-c", script).CombinedOutput()
		if err != nil {
			t.Fatalf("bash: %v\n%s", err, out)
		}
		if !strings.Contains(string(out), "pick") {
			t.Errorf("bash did not offer the pick command:\n%s", out)
		}
	})

	t.Run("bash scopes flags to the command", func(t *testing.T) {
		bash, err := exec.LookPath("bash")
		if err != nil {
			t.Skip("bash not installed")
		}
		script := `eval "$(` + bin + ` completion bash)"
COMP_WORDS=(disc-fortune list --); COMP_CWORD=2; _disc_fortune; echo "${COMPREPLY[@]}"`
		out, err := exec.Command(bash, "-c", script).CombinedOutput()
		if err != nil {
			t.Fatalf("bash: %v\n%s", err, out)
		}
		got := " " + string(out) + " "
		if !strings.Contains(got, "--json") {
			t.Errorf("list should offer --json:\n%s", out)
		}
		if strings.Contains(got, "--draw") {
			t.Errorf("list should not offer --draw, which it rejects:\n%s", out)
		}
	})
}

// buildForCompletion builds the binary once so the shell tests drive the real
// command rather than a stub.
func buildForCompletion(t *testing.T) string {
	t.Helper()
	bin := t.TempDir() + "/disc-fortune"
	out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("building the binary: %v\n%s", err, out)
	}
	return bin
}

// bash and zsh fall through to the subcommand list when a flag's value cannot
// be completed, so `disc-fortune --genre <TAB>` offered command names as the
// value of --genre. fish gets this right for free via -x; the other two need
// the flags named explicitly.
func TestCompletionOffersNothingForAFreeFormValue(t *testing.T) {
	bin := buildForCompletion(t)
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not installed")
	}

	for _, words := range []string{
		`COMP_WORDS=(disc-fortune --genre ""); COMP_CWORD=2`,
		`COMP_WORDS=(disc-fortune --label ""); COMP_CWORD=2`,
		`COMP_WORDS=(disc-fortune list --genre ""); COMP_CWORD=3`,
	} {
		script := `eval "$(` + bin + ` completion bash)"
` + words + `; _disc_fortune; echo "${COMPREPLY[@]}"`
		out, err := exec.Command(bash, "-c", script).CombinedOutput()
		if err != nil {
			t.Fatalf("bash: %v\n%s", err, out)
		}
		if got := strings.TrimSpace(string(out)); got != "" {
			t.Errorf("%s should complete nothing, got: %s", words, got)
		}
	}
}

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

// oldCompletionUsage is completion's usage exactly as main defined it before
// completion became a cli built-in (#54).
const oldCompletionUsage = `Usage: disc-fortune completion SHELL

Prints a completion script for bash, zsh or fish on stdout. The script is
generated from the commands and flags this binary actually accepts, so it
cannot drift from them.

Load it for the current shell:

  bash    eval "$(disc-fortune completion bash)"
  zsh     eval "$(disc-fortune completion zsh)"
  fish    disc-fortune completion fish | source

To make it permanent, add that line to your shell's startup file, or write the
script into the directory your shell reads completions from.

Command and flag names are completed, as are the fixed values of --draw and
--color. Values that would have to be read from your collection, such as those
of --genre and --label, are not: a completion should never depend on a file
that a sync may be rewriting.`
