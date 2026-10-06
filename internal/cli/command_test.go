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
