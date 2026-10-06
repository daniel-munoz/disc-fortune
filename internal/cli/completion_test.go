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
