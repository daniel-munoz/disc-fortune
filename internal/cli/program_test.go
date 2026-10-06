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
