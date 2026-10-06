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
