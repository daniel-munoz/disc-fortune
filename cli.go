package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/daniel-munoz/disc-fortune/v2/internal/cli"
	"github.com/daniel-munoz/disc-fortune/v2/internal/disc"
	"github.com/daniel-munoz/disc-fortune/v2/internal/term"
)

// command is one subcommand in the CLI. help is generated from the table, so a
// command cannot ship undocumented.
type command struct {
	name    string
	summary string // one line, listed by `help`
	usage   string // full block, shown by `help <cmd>` and on usage error
	run     func(a app, args []string) error
	// needsConfig marks the commands that read or write data files. Only
	// those fail when the config directory cannot be resolved; help,
	// version and folders must keep working on a machine with no usable
	// home directory.
	needsConfig bool
}

// commands is the full CLI surface. It is populated in init rather than as a
// package-level literal because help reads from it, which would otherwise be
// an initialization cycle.
var commands []command

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

// parseHelp validates help's arguments (an optional topic). Routing it
// through cli.NewFlagSet/cli.ParseInterspersed, like every other command, means
// -h/-help/--help on help itself hits the flag package's built-in ErrHelp
// case instead of being mistaken for a topic named "--help".
func parseHelp(args []string) (string, error) {
	fs, _ := cli.NewFlagSet("help")
	rest, err := cli.ParseInterspersed(fs, args)
	if err != nil {
		return "", fmt.Errorf("help: %w", err)
	}
	if len(rest) > 1 {
		return "", fmt.Errorf("help: too many arguments")
	}
	if len(rest) == 1 {
		return rest[0], nil
	}
	return "", nil
}

// handleParseErr reports a command's parse/validation failure and tells the
// caller whether it already handled it (in which case run should return
// immediately instead of proceeding with a zero-value config).
//
// The flag package treats -h/--help as flag.ErrHelp rather than a real
// failure, so `disc-fortune <command> --help` must not be treated the same
// as a bad flag: it prints the command's usage and exits 0, the same as top
// level -h. Any other error is a genuine usage error; command.usage's doc
// comment promises that text is "shown ... on usage error", so it is printed
// there too, alongside the message, before exiting 1.
func handleParseErr(name string, err error) bool {
	if err == nil {
		return false
	}
	c := lookup(name)
	if errors.Is(err, flag.ErrHelp) {
		if c != nil {
			fmt.Println(c.usage)
		}
		return true
	}
	fmt.Fprintln(os.Stderr, err)
	if c != nil {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, c.usage)
	}
	os.Exit(1)
	return true
}

// lookup returns the named command, or nil.
func lookup(name string) *command {
	for i := range commands {
		if commands[i].name == name {
			return &commands[i]
		}
	}
	return nil
}

// v1Signposts maps a v1 flag (leading dashes trimmed) to the v2 command that
// replaced it, for the migration signpost in resolve. Filter flags that still
// work verbatim under implicit pick (--favorites, --year, --genre, --label,
// --format) must NOT appear here: adding them would break working v2
// invocations by redirecting them to a command the user never asked for.
var v1Signposts = map[string]string{
	"sync":            "disc-fortune sync",
	"list":            "disc-fortune list",
	"list-folders":    "disc-fortune folders",
	"history":         "disc-fortune history",
	"favorite-last":   "disc-fortune favorite",
	"unfavorite-last": "disc-fortune unfavorite",
	"favorite":        "disc-fortune favorite QUERY",
}

// resolve maps raw argv (without the program name) to a command and its
// arguments. Empty argv, or a leading flag, means the implicit pick.
func resolve(args []string) (*command, []string, error) {
	if len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-h", "--help", "-help":
			return lookup("help"), nil, nil
		case "-v", "--version", "-version":
			return nil, nil, fmt.Errorf(
				"there is no %s flag; use `disc-fortune version`", args[0])
		}
		if target, ok := v1Signposts[strings.TrimLeft(args[0], "-")]; ok {
			return nil, nil, fmt.Errorf(
				"%s is now `%s` (see RELEASE_NOTES_v2.0.0.md)", args[0], target)
		}
	}
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		args = append([]string{"pick"}, args...)
	}
	cmd := lookup(args[0])
	if cmd == nil {
		return nil, nil, fmt.Errorf(
			"unknown command %q\nRun `disc-fortune help` for usage.", args[0])
	}
	return cmd, args[1:], nil
}

// helpText renders the general help, or one command's usage block.
func helpText(topic string) (string, error) {
	if topic != "" {
		c := lookup(topic)
		if c == nil {
			return "", fmt.Errorf("help: unknown command %q", topic)
		}
		return c.usage, nil
	}

	var sb strings.Builder
	sb.WriteString("disc-fortune - randomly picks a record from your Discogs collection\n\n")
	sb.WriteString("Usage:\n  disc-fortune [command] [flags]\n\n")
	sb.WriteString("Commands:\n")
	for _, c := range commands {
		sb.WriteString(fmt.Sprintf("  %-11s %s\n", c.name, c.summary))
	}
	sb.WriteString("\nRun `disc-fortune help <command>` for details on a command.\n")
	sb.WriteString("With no command, disc-fortune picks a random album.\n")
	return sb.String(), nil
}

func init() {
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
		{
			name:    "help",
			summary: "Show help for a command",
			usage:   "Usage: disc-fortune help [COMMAND]\n\nShows general help, or detailed help for one command.",
			run: func(a app, args []string) error {
				topic, err := parseHelp(args)
				if handleParseErr("help", err) {
					return nil
				}
				out, err := helpText(topic)
				if err != nil {
					return err
				}
				fmt.Fprintln(a.stdout, out)
				return nil
			},
		},
	}

	// Documented centrally, matching where they are registered.
	for i := range commands {
		if commands[i].name == "help" {
			continue
		}
		commands[i].usage += cli.GlobalFlagHelp
	}
}

// dispatch resolves argv and runs the chosen command.
func dispatch(args []string) {
	cmd, rest, err := resolve(args)
	if err != nil {
		fatal("disc-fortune: %v", err)
	}
	// Resolved once, here, so every path helper on app can be infallible.
	// A failure is only fatal for the commands that actually need it.
	a, cfgErr := newApp(os.Getenv, os.UserHomeDir, os.Stdout, os.Stderr)
	if cfgErr != nil {
		if cmd.needsConfig {
			fatal("disc-fortune: %v", cfgErr)
		}
	} else if cmd.needsConfig {
		fmt.Fprint(a.stderr, disc.MigrationNotice(a.loc, a.metaPath(), term.IsTTY(os.Stderr)))
	}
	// The one exit point for a command failure. The printer adds no prefix:
	// fatal never did either, and the two messages above carry their own
	// "disc-fortune: " in their text.
	if err := cmd.run(a, rest); err != nil {
		fmt.Fprintln(a.stderr, err)
		os.Exit(1)
	}
}
