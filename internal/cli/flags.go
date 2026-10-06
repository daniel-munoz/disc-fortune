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

// Globals holds the flags every command accepts. They are registered in
// NewFlagSet rather than per-command, so a command physically cannot ship
// without them and their help text cannot drift. Later global flags belong
// here too.
type Globals struct {
	color *string
}

// Mode resolves the global flags into the values commands actually use.
func (g *Globals) Mode() (term.Mode, error) {
	return term.ParseMode(*g.color)
}

// NewFlagSet builds a FlagSet that never prints or exits on its own, so the
// caller controls the message and the exit code. Every command's flags start
// here, which is what makes the global flags universal. It is exported for
// main's tests, which build ad-hoc flag sets to test shared registration.
func NewFlagSet(name string) (*flag.FlagSet, *Globals) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	g := &Globals{
		color: fs.String("color", "auto", "When to colorize output: auto, always, or never"),
	}
	return fs, g
}

// ParseInterspersed parses args allowing flags to appear before, after, or
// around positional arguments. Go's flag package stops at the first non-flag
// argument, which would silently drop trailing flags such as the --year in
// `favorite "miles" --year 1959`. Parse uses it; it is exported for the same
// tests NewFlagSet is.
func ParseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return positional, nil
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}

// GlobalFlagHelp documents the flags NewFlagSet registers on every command.
// It is appended to each usage block programmatically, for the same reason
// the flags themselves are registered centrally: a command must not be able
// to ship without them. NewProgram appends it; it is exported so main's
// tests can check a usage block ends with it.
const GlobalFlagHelp = `

Global flags (accepted by every command):
  --color WHEN     Colorize output: auto (default), always, or never.
                   auto colorizes only a terminal, and honors NO_COLOR.`
