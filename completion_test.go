package main

import (
	"os/exec"
	"strings"
	"testing"
)

// The guard that makes "completion is generated, not hardcoded" true rather
// than aspirational. Enumeration and parsing share one FlagSet builder, so
// they cannot diverge by construction -- but construction is not proof that
// the enumeration reaches the real parser. This drives every completed flag
// through the actual parse function and fails if any is rejected as unknown.
func TestCompletionOffersOnlyFlagsTheCommandAccepts(t *testing.T) {
	for _, c := range commands {
		for _, f := range commandFlags(c.name) {
			args := []string{"--" + f.name}
			if !f.isBool {
				args = append(args, sampleValue(f.name))
			}

			var err error
			switch c.name {
			case "pick", "list":
				_, err = parseSelection(c.name, args)
			case "history":
				_, err = parseHistory(args)
			case "favorite", "unfavorite":
				// These need a query beside a narrowing filter.
				_, err = parseFavorite(c.name, append([]string{"miles"}, args...))
			case "sync":
				_, err = parseSync(args)
			default:
				err = parseNoArgs(c.name, args)
			}
			if err != nil && strings.Contains(err.Error(), "not defined") {
				t.Errorf("%s completes --%s but the command rejects it: %v", c.name, f.name, err)
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
	for _, shell := range completionShells {
		script, err := completionScript(shell)
		if err != nil {
			t.Fatalf("completionScript(%q): %v", shell, err)
		}
		for _, c := range commands {
			if !strings.Contains(script, c.name) {
				t.Errorf("%s script does not mention the %q command", shell, c.name)
			}
		}
	}
}

// pick draws, list does not. The scripts must reflect that, or completion
// would suggest a flag the command rejects.
func TestCompletionScopesFlagsPerCommand(t *testing.T) {
	if !hasFlag(commandFlags("pick"), "draw") {
		t.Error("pick should complete --draw")
	}
	if hasFlag(commandFlags("list"), "draw") {
		t.Error("list should not complete --draw: it draws nothing")
	}
	for _, name := range []string{"pick", "list", "history"} {
		if !hasFlag(commandFlags(name), "json") {
			t.Errorf("%s should complete --json", name)
		}
	}
	for _, name := range []string{"sync", "folders", "migrate"} {
		if hasFlag(commandFlags(name), "json") {
			t.Errorf("%s should not complete --json: it does not accept it", name)
		}
	}
	if !hasFlag(commandFlags("sync"), "folder") {
		t.Error("sync should complete --folder")
	}
	// --color is global, so every command gets it.
	for _, c := range commands {
		if !hasFlag(commandFlags(c.name), "color") {
			t.Errorf("%s should complete the global --color", c.name)
		}
	}
}

func hasFlag(flags []completionFlag, name string) bool {
	for _, f := range flags {
		if f.name == name {
			return true
		}
	}
	return false
}

// The enum values are compiled in, so completing them costs nothing. This
// pins them to what the parsers actually accept rather than to a comment.
func TestCompletionEnumValuesAreAccepted(t *testing.T) {
	for _, v := range flagValues["draw"] {
		if _, err := parseDrawMode(v); err != nil {
			t.Errorf("completion offers --draw %q but parseDrawMode rejects it: %v", v, err)
		}
	}
	for _, v := range flagValues["color"] {
		if _, err := parseColorMode(v); err != nil {
			t.Errorf("completion offers --color %q but parseColorMode rejects it: %v", v, err)
		}
	}
	if _, err := parseDrawMode("nonsense"); err == nil {
		t.Error("parseDrawMode accepted nonsense; the test above proves nothing")
	}
	if _, err := parseColorMode("nonsense"); err == nil {
		t.Error("parseColorMode accepted nonsense; the test above proves nothing")
	}
}

func TestCompletionEnumValuesReachTheScripts(t *testing.T) {
	for _, shell := range completionShells {
		script, err := completionScript(shell)
		if err != nil {
			t.Fatalf("completionScript(%q): %v", shell, err)
		}
		for _, v := range append(flagValues["draw"], flagValues["color"]...) {
			if !strings.Contains(script, v) {
				t.Errorf("%s script is missing the enum value %q", shell, v)
			}
		}
	}
}

func TestCompletionRejectsUnknownShell(t *testing.T) {
	for _, shell := range []string{"", "tcsh", "powershell", "BASH"} {
		if _, err := completionScript(shell); err == nil {
			t.Errorf("completionScript(%q) succeeded, want an error", shell)
		}
	}
}

func TestParseCompletionRequiresAShell(t *testing.T) {
	if _, err := parseCompletion(nil); err == nil {
		t.Error("completion with no argument should fail")
	}
	if _, err := parseCompletion([]string{"bash", "zsh"}); err == nil {
		t.Error("completion with two arguments should fail")
	}
	shell, err := parseCompletion([]string{"fish"})
	if err != nil {
		t.Fatalf("parseCompletion([fish]): %v", err)
	}
	if shell != "fish" {
		t.Errorf("shell = %q, want fish", shell)
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

	if len(checks) != len(completionShells) {
		t.Fatalf("%d shells are generated but %d are syntax-checked; add the new one",
			len(completionShells), len(checks))
	}

	for shell, check := range checks {
		t.Run(shell, func(t *testing.T) {
			bin, err := exec.LookPath(check[0])
			if err != nil {
				t.Skipf("%s not installed", check[0])
			}
			script, err := completionScript(shell)
			if err != nil {
				t.Fatalf("completionScript(%q): %v", shell, err)
			}

			path := t.TempDir() + "/completion." + shell
			if err := writeFileAtomic(path, []byte(script), 0644); err != nil {
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

// commandFlagSet's switch is a second place that has to learn about a new
// command. Nothing else forces that: a command registering flags without a
// case there would offer only the global flags, and every other test would
// still pass. This map is the forcing function -- adding a command fails it
// until someone decides what completion should offer for it.
func TestEveryCommandHasACompletionDecision(t *testing.T) {
	// true when the command registers flags of its own beyond the globals.
	hasOwnFlags := map[string]bool{
		"pick":       true,
		"list":       true,
		"history":    true,
		"favorite":   true,
		"unfavorite": true,
		"sync":       true,
		"folders":    false,
		"migrate":    false,
		"version":    false,
		"help":       false,
		"completion": false,
	}
	if len(hasOwnFlags) != len(commands) {
		t.Fatalf("this test covers %d commands but there are %d; decide what "+
			"completion offers for the new one, then add it here",
			len(hasOwnFlags), len(commands))
	}

	// A name no command has, so commandFlagSet's switch adds nothing.
	globals := len(commandFlags("\x00none"))
	for name, own := range hasOwnFlags {
		got := len(commandFlags(name))
		switch {
		case own && got <= globals:
			t.Errorf("%s registers flags of its own, but completion offers only "+
				"the %d global ones -- missing from commandFlagSet's switch?", name, globals)
		case !own && got != globals:
			t.Errorf("%s should offer only the %d global flags, got %d", name, globals, got)
		}
	}
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

// completion colorizes nothing, but it accepts --color like every command, so
// it must reject a bad value like every command. TestEveryCommandAcceptsColorFlag
// covers the accepting half; this covers the rejecting half.
func TestCompletionRejectsInvalidColor(t *testing.T) {
	if _, err := parseCompletion([]string{"--color", "sometimes", "bash"}); err == nil {
		t.Error("completion accepted --color=sometimes, want an error")
	}
	if _, err := parseCompletion([]string{"--color", "never", "bash"}); err != nil {
		t.Errorf("completion rejected a valid --color: %v", err)
	}
}
