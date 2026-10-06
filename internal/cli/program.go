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
