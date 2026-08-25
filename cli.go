package main

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// newFlagSet builds a FlagSet that never prints or exits on its own, so the
// caller controls the message and the exit code.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	return fs
}

// parseInterspersed parses args allowing flags to appear before, after, or
// around positional arguments. Go's flag package stops at the first non-flag
// argument, which would silently drop trailing flags such as the --year in
// `favorite "miles" --year 1959`.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
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

// filterFlags holds the four filter flags shared by pick, list, favorite, and
// unfavorite. Registering them in one place keeps their names and help text
// from drifting apart between commands.
type filterFlags struct {
	year   *string
	genre  *string
	label  *string
	format *string
}

func addFilterFlags(fs *flag.FlagSet) *filterFlags {
	return &filterFlags{
		year:   fs.String("year", "", "Filter by year or year range (e.g., 1975 or 1970-1980)"),
		genre:  fs.String("genre", "", "Filter by genre (case-insensitive substring match)"),
		label:  fs.String("label", "", "Filter by label (case-insensitive substring match)"),
		format: fs.String("format", "", "Filter by format (case-insensitive substring match)"),
	}
}

// Filter builds a Filter from the parsed flags, validating the year format.
func (ff *filterFlags) Filter() (Filter, error) {
	if err := ParseYearFilter(*ff.year); err != nil {
		return Filter{}, err
	}
	return Filter{
		Year:   *ff.year,
		Genre:  *ff.genre,
		Label:  *ff.label,
		Format: *ff.format,
	}, nil
}

// any reports whether any filter flag was set.
func (ff *filterFlags) any() bool {
	return *ff.year != "" || *ff.genre != "" || *ff.label != "" || *ff.format != ""
}

// defaultHistoryLimit is how many past picks `history` shows with no argument.
const defaultHistoryLimit = 10

// command is one subcommand in the CLI. help is generated from the table, so a
// command cannot ship undocumented.
type command struct {
	name    string
	summary string // one line, listed by `help`
	usage   string // full block, shown by `help <cmd>` and on usage error
	run     func(args []string)
}

// commands is the full CLI surface. It is populated in init rather than as a
// package-level literal because help reads from it, which would otherwise be
// an initialization cycle.
var commands []command

// selection is the parsed form of the flags shared by pick and list.
type selection struct {
	favoritesOnly bool
	filter        Filter
}

// favoriteConfig is the parsed form of favorite and unfavorite. An empty query
// means "the last pick".
type favoriteConfig struct {
	query  string
	filter Filter
}

// historyConfig is the parsed form of history. A limit of 0 means "all".
type historyConfig struct {
	limit int
}

// syncConfig is the parsed form of sync.
type syncConfig struct {
	folders []string
}

func parseSelection(name string, args []string) (selection, error) {
	fs := newFlagSet(name)
	favoritesOnly := fs.Bool("favorites", false, "Restrict to favorites only")
	ff := addFilterFlags(fs)

	rest, err := parseInterspersed(fs, args)
	if err != nil {
		return selection{}, fmt.Errorf("%s: %w", name, err)
	}
	if len(rest) > 0 {
		return selection{}, fmt.Errorf("%s: unexpected argument %q", name, rest[0])
	}
	filter, err := ff.Filter()
	if err != nil {
		return selection{}, fmt.Errorf("%s: %v", name, err)
	}
	return selection{favoritesOnly: *favoritesOnly, filter: filter}, nil
}

func parseFavorite(name string, args []string) (favoriteConfig, error) {
	fs := newFlagSet(name)
	ff := addFilterFlags(fs)

	rest, err := parseInterspersed(fs, args)
	if err != nil {
		return favoriteConfig{}, fmt.Errorf("%s: %w", name, err)
	}
	if len(rest) > 1 {
		return favoriteConfig{}, fmt.Errorf(
			"%s: too many arguments (quote the query: %s %q)",
			name, name, strings.Join(rest, " "))
	}
	filter, err := ff.Filter()
	if err != nil {
		return favoriteConfig{}, fmt.Errorf("%s: %v", name, err)
	}

	if len(rest) == 0 {
		if ff.any() {
			return favoriteConfig{}, fmt.Errorf("%s: filters require a query", name)
		}
		return favoriteConfig{filter: filter}, nil
	}

	query := strings.TrimSpace(rest[0])
	if query == "" {
		return favoriteConfig{}, fmt.Errorf("%s: requires a query", name)
	}
	return favoriteConfig{query: query, filter: filter}, nil
}

func parseHistory(args []string) (historyConfig, error) {
	fs := newFlagSet("history")
	rest, err := parseInterspersed(fs, args)
	if err != nil {
		return historyConfig{}, fmt.Errorf("history: %w", err)
	}
	if len(rest) > 1 {
		return historyConfig{}, fmt.Errorf("history: too many arguments")
	}
	limit := defaultHistoryLimit
	if len(rest) == 1 {
		n, err := strconv.Atoi(strings.TrimSpace(rest[0]))
		if err != nil {
			return historyConfig{}, fmt.Errorf("history: requires a number (e.g., history 20)")
		}
		if n < 0 {
			return historyConfig{}, fmt.Errorf("history: count cannot be negative")
		}
		limit = n
	}
	return historyConfig{limit: limit}, nil
}

func parseSync(args []string) (syncConfig, error) {
	fs := newFlagSet("sync")
	var folders arrayFlags
	fs.Var(&folders, "folder", "Sync only specific folder(s) by name (repeatable)")

	rest, err := parseInterspersed(fs, args)
	if err != nil {
		return syncConfig{}, fmt.Errorf("sync: %w", err)
	}
	if len(rest) > 0 {
		return syncConfig{}, fmt.Errorf("sync: unexpected argument %q", rest[0])
	}
	return syncConfig{folders: folders}, nil
}

// parseNoArgs validates that a command was invoked with no flags and no arguments.
func parseNoArgs(name string, args []string) error {
	fs := newFlagSet(name)
	rest, err := parseInterspersed(fs, args)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if len(rest) > 0 {
		return fmt.Errorf("%s: unexpected argument %q", name, rest[0])
	}
	return nil
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

const filterFlagHelp = `  --year VALUE     Filter by year or year range (e.g., 1975 or 1970-1980)
  --genre VALUE    Filter by genre (case-insensitive substring match)
  --label VALUE    Filter by label (case-insensitive substring match)
  --format VALUE   Filter by format (case-insensitive substring match)`

func init() {
	commands = []command{
		{
			name:    "pick",
			summary: "Print a random album (default)",
			usage: `Usage: disc-fortune pick [flags]

Prints one random album from your collection and records it in history.
This is what runs when you give no command at all.

Flags:
  --favorites      Pick from favorites only
` + filterFlagHelp,
			run: func(args []string) {
				cfg, err := parseSelection("pick", args)
				if err != nil {
					fatal("%v", err)
				}
				runPick(cfg)
			},
		},
		{
			name:    "list",
			summary: "List every matching album",
			usage: `Usage: disc-fortune list [flags]

Prints every album matching the filters, with a count.

Flags:
  --favorites      List favorites only
` + filterFlagHelp,
			run: func(args []string) {
				cfg, err := parseSelection("list", args)
				if err != nil {
					fatal("%v", err)
				}
				runList(cfg)
			},
		},
		{
			name:    "sync",
			summary: "Fetch your collection from Discogs",
			usage: `Usage: disc-fortune sync [--folder NAME ...]

Fetches your Discogs collection and caches it locally. Requires DISCOGS_TOKEN
to be set. With no --folder, syncs everything.

Flags:
  --folder NAME    Sync only this folder (repeatable)

Run ` + "`disc-fortune folders`" + ` to see available folder names.`,
			run: func(args []string) {
				cfg, err := parseSync(args)
				if err != nil {
					fatal("%v", err)
				}
				runSync(cfg)
			},
		},
		{
			name:    "folders",
			summary: "List your Discogs folder names",
			usage: `Usage: disc-fortune folders

Lists the folder names in your Discogs collection, for use with
` + "`disc-fortune sync --folder`" + `. Requires DISCOGS_TOKEN to be set.`,
			run: func(args []string) {
				if err := parseNoArgs("folders", args); err != nil {
					fatal("%v", err)
				}
				runFolders()
			},
		},
		{
			name:    "history",
			summary: "Show recent picks",
			usage: `Usage: disc-fortune history [N]

Shows the last N picks. N defaults to 10; 0 shows all of them.`,
			run: func(args []string) {
				cfg, err := parseHistory(args)
				if err != nil {
					fatal("%v", err)
				}
				runHistory(cfg)
			},
		},
		{
			name:    "favorite",
			summary: "Add an album to favorites",
			usage: `Usage: disc-fortune favorite [QUERY] [flags]

With no QUERY, favorites the last pick. With a QUERY, favorites the one
album in your collection whose "Artist - Title" contains it, case-insensitively.
If the query matches several albums, they are listed and nothing is added;
narrow it with filters.

Flags (only valid alongside a QUERY):
` + filterFlagHelp,
			run: func(args []string) {
				cfg, err := parseFavorite("favorite", args)
				if err != nil {
					fatal("%v", err)
				}
				runFavorite(cfg)
			},
		},
		{
			name:    "unfavorite",
			summary: "Remove an album from favorites",
			usage: `Usage: disc-fortune unfavorite [QUERY] [flags]

With no QUERY, unfavorites the last pick. With a QUERY, removes the one
favorite whose "Artist - Title" contains it, case-insensitively. Removing
something that is not favorited succeeds quietly.

Flags (only valid alongside a QUERY):
` + filterFlagHelp,
			run: func(args []string) {
				cfg, err := parseFavorite("unfavorite", args)
				if err != nil {
					fatal("%v", err)
				}
				runUnfavorite(cfg)
			},
		},
		{
			name:    "version",
			summary: "Print the version",
			usage:   "Usage: disc-fortune version\n\nPrints the disc-fortune version and exits.",
			run: func(args []string) {
				if err := parseNoArgs("version", args); err != nil {
					fatal("%v", err)
				}
				fmt.Printf("disc-fortune %s\n", version)
			},
		},
		{
			name:    "help",
			summary: "Show help for a command",
			usage:   "Usage: disc-fortune help [COMMAND]\n\nShows general help, or detailed help for one command.",
			run: func(args []string) {
				topic := ""
				if len(args) > 1 {
					fatal("help: too many arguments")
				}
				if len(args) == 1 {
					topic = args[0]
				}
				out, err := helpText(topic)
				if err != nil {
					fatal("%v", err)
				}
				fmt.Println(out)
			},
		},
	}
}

// dispatch resolves argv and runs the chosen command.
func dispatch(args []string) {
	cmd, rest, err := resolve(args)
	if err != nil {
		fatal("disc-fortune: %v", err)
	}
	cmd.run(rest)
}
