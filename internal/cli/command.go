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
