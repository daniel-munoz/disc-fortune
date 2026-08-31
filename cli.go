package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// globalFlags holds the flags every command accepts. They are registered in
// newFlagSet rather than per-command, so a command physically cannot ship
// without them and their help text cannot drift. Later global flags belong
// here too.
type globalFlags struct {
	color *string
}

// mode resolves the global flags into the values commands actually use.
func (g *globalFlags) mode() (colorMode, error) {
	return parseColorMode(*g.color)
}

// newFlagSet builds a FlagSet that never prints or exits on its own, so the
// caller controls the message and the exit code. Every command's flags start
// here, which is what makes the global flags universal.
func newFlagSet(name string) (*flag.FlagSet, *globalFlags) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	g := &globalFlags{
		color: fs.String("color", "auto", "When to colorize output: auto, always, or never"),
	}
	return fs, g
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
	year      *string
	genre     *string
	label     *string
	format    *string
	releaseID *int
}

func addFilterFlags(fs *flag.FlagSet) *filterFlags {
	return &filterFlags{
		year:      fs.String("year", "", "Filter by year or year range (e.g., 1975 or 1970-1980)"),
		genre:     fs.String("genre", "", "Filter by genre (case-insensitive substring match)"),
		label:     fs.String("label", "", "Filter by label (case-insensitive substring match)"),
		format:    fs.String("format", "", "Filter by format or colour (case-insensitive substring match)"),
		releaseID: fs.Int("release-id", 0, "Select one exact record by its Discogs release ID"),
	}
}

// Filter builds a Filter from the parsed flags, validating the year format.
func (ff *filterFlags) Filter() (Filter, error) {
	if err := ParseYearFilter(*ff.year); err != nil {
		return Filter{}, err
	}
	return Filter{
		Year:      *ff.year,
		Genre:     *ff.genre,
		Label:     *ff.label,
		Format:    *ff.format,
		ReleaseID: *ff.releaseID,
	}, nil
}

// anyNarrowing reports whether a filter that only *refines* a query was set.
// Those cannot stand alone: --year 1959 does not say which record is meant.
// --release-id is deliberately excluded, because it identifies one exact
// record and needs nothing beside it.
func (ff *filterFlags) anyNarrowing() bool {
	return *ff.year != "" || *ff.genre != "" || *ff.label != "" || *ff.format != ""
}

// identifies reports whether the flags name one exact record on their own.
func (ff *filterFlags) identifies() bool {
	return *ff.releaseID != 0
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

// selection is the parsed form of the flags shared by pick and list.
type selection struct {
	favoritesOnly bool
	// unheard restricts to albums that have never been picked. It is a
	// filter on the candidate set, like favoritesOnly, not a draw strategy
	// -- which is what lets `list` have it too.
	unheard bool
	// draw is how pick chooses from the candidates. It is meaningless for
	// list, which never sets it.
	draw   drawMode
	filter Filter
	color  colorMode
}

// favoriteConfig is the parsed form of favorite and unfavorite. An empty query
// means "the last pick".
type favoriteConfig struct {
	query  string
	filter Filter
	color  colorMode
}

// historyConfig is the parsed form of history. A limit of 0 means "all".
type historyConfig struct {
	limit int
	color colorMode
}

// syncConfig is the parsed form of sync.
type syncConfig struct {
	folders []string
}

func parseSelection(name string, args []string) (selection, error) {
	fs, gf := newFlagSet(name)
	favoritesOnly := fs.Bool("favorites", false, "Restrict to favorites only")
	unheard := fs.Bool("unheard", false, "Restrict to albums never picked before")

	// --draw is registered only where something is actually drawn, so
	// `list --draw stale` fails as an unknown flag rather than being
	// accepted and silently ignored. Nothing else has to check for it.
	var draw *string
	if name != "list" {
		draw = fs.String("draw", "fresh", "How to draw a pick: any, fresh, or stale")
	}

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
	color, err := gf.mode()
	if err != nil {
		return selection{}, fmt.Errorf("%s: %v", name, err)
	}

	mode := drawFresh
	if draw != nil {
		m, err := parseDrawMode(*draw)
		if err != nil {
			return selection{}, fmt.Errorf("%s: %v", name, err)
		}
		mode = m
	}

	return selection{
		favoritesOnly: *favoritesOnly,
		unheard:       *unheard,
		draw:          mode,
		filter:        filter,
		color:         color,
	}, nil
}

func parseFavorite(name string, args []string) (favoriteConfig, error) {
	fs, gf := newFlagSet(name)
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
	color, err := gf.mode()
	if err != nil {
		return favoriteConfig{}, fmt.Errorf("%s: %v", name, err)
	}

	if len(rest) == 0 {
		// A release ID is a complete answer by itself, so it excuses the
		// missing query -- and carries any narrowing filters along with it.
		if ff.anyNarrowing() && !ff.identifies() {
			return favoriteConfig{}, fmt.Errorf("%s: filters require a query", name)
		}
		return favoriteConfig{filter: filter, color: color}, nil
	}

	query := strings.TrimSpace(rest[0])
	if query == "" {
		return favoriteConfig{}, fmt.Errorf("%s: requires a query", name)
	}
	return favoriteConfig{query: query, filter: filter, color: color}, nil
}

func parseHistory(args []string) (historyConfig, error) {
	fs, gf := newFlagSet("history")
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
	color, err := gf.mode()
	if err != nil {
		return historyConfig{}, fmt.Errorf("history: %v", err)
	}
	return historyConfig{limit: limit, color: color}, nil
}

// parseHelp validates help's arguments (an optional topic). Routing it
// through newFlagSet/parseInterspersed, like every other command, means
// -h/-help/--help on help itself hits the flag package's built-in ErrHelp
// case instead of being mistaken for a topic named "--help".
func parseHelp(args []string) (string, error) {
	fs, _ := newFlagSet("help")
	rest, err := parseInterspersed(fs, args)
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

func parseSync(args []string) (syncConfig, error) {
	fs, gf := newFlagSet("sync")
	var folders arrayFlags
	fs.Var(&folders, "folder", "Sync only specific folder(s) by name (repeatable)")

	rest, err := parseInterspersed(fs, args)
	if err != nil {
		return syncConfig{}, fmt.Errorf("sync: %w", err)
	}
	if len(rest) > 0 {
		return syncConfig{}, fmt.Errorf("sync: unexpected argument %q", rest[0])
	}
	// sync colorizes nothing, but a bad --color value is still a typo and
	// must be reported rather than ignored.
	if _, err := gf.mode(); err != nil {
		return syncConfig{}, fmt.Errorf("sync: %v", err)
	}
	return syncConfig{folders: folders}, nil
}

// parseNoArgs validates that a command was invoked with no flags and no arguments.
func parseNoArgs(name string, args []string) error {
	fs, gf := newFlagSet(name)
	rest, err := parseInterspersed(fs, args)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if len(rest) > 0 {
		return fmt.Errorf("%s: unexpected argument %q", name, rest[0])
	}
	// These commands produce no colorized output, but they still accept the
	// flag, so they must still reject a bad value for it.
	if _, err := gf.mode(); err != nil {
		return fmt.Errorf("%s: %v", name, err)
	}
	return nil
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

// globalFlagHelp documents the flags newFlagSet registers on every command.
// It is appended to each usage block programmatically, for the same reason
// the flags themselves are registered centrally: a command must not be able
// to ship without them.
const globalFlagHelp = `

Global flags (accepted by every command):
  --color WHEN     Colorize output: auto (default), always, or never.
                   auto colorizes only a terminal, and honors NO_COLOR.`

const filterFlagHelp = `  --year VALUE     Filter by year or year range (e.g., 1975 or 1970-1980)
  --genre VALUE    Filter by genre (case-insensitive substring match)
  --label VALUE    Filter by label (case-insensitive substring match)
  --format VALUE   Filter by format or colour (case-insensitive substring match)
  --release-id N   Select one exact record by its Discogs release ID`

func init() {
	commands = []command{
		{
			name:        "pick",
			needsConfig: true,
			summary:     "Print a random album (default)",
			usage: `Usage: disc-fortune pick [flags]

Prints one random album from your collection and records it in history.
This is what runs when you give no command at all.

By default a pick avoids the records you played most recently, so the same
album does not come back around twice in a week. --draw any turns that off.

Flags:
  --favorites      Pick from favorites only
  --unheard        Pick only from albums you have never picked
  --draw WHEN      How to draw: fresh (default), any, or stale.
                   fresh skips your recent picks; any ignores history
                   entirely; stale favors what you have left longest.
` + filterFlagHelp,
			run: func(args []string) {
				cfg, err := parseSelection("pick", args)
				if handleParseErr("pick", err) {
					return
				}
				runPick(cfg)
			},
		},
		{
			name:        "list",
			needsConfig: true,
			summary:     "List every matching album",
			usage: `Usage: disc-fortune list [flags]

Prints every album matching the filters, with a count.

Flags:
  --favorites      List favorites only
  --unheard        List only albums you have never picked
` + filterFlagHelp,
			run: func(args []string) {
				cfg, err := parseSelection("list", args)
				if handleParseErr("list", err) {
					return
				}
				runList(cfg)
			},
		},
		{
			name:        "sync",
			needsConfig: true,
			summary:     "Fetch your collection from Discogs",
			usage: `Usage: disc-fortune sync [--folder NAME ...]

Fetches your Discogs collection and caches it locally. Requires DISCOGS_TOKEN
to be set. With no --folder, syncs everything.

Flags:
  --folder NAME    Sync only this folder (repeatable)

Run ` + "`disc-fortune folders`" + ` to see available folder names.`,
			run: func(args []string) {
				cfg, err := parseSync(args)
				if handleParseErr("sync", err) {
					return
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
				if handleParseErr("folders", parseNoArgs("folders", args)) {
					return
				}
				runFolders()
			},
		},
		{
			name:        "history",
			needsConfig: true,
			summary:     "Show recent picks",
			usage: `Usage: disc-fortune history [N]

Shows the last N picks. N defaults to 10; 0 shows all of them.`,
			run: func(args []string) {
				cfg, err := parseHistory(args)
				if handleParseErr("history", err) {
					return
				}
				runHistory(cfg)
			},
		},
		{
			name:        "favorite",
			needsConfig: true,
			summary:     "Add an album to favorites",
			usage: `Usage: disc-fortune favorite [QUERY] [flags]

With no QUERY, favorites the last pick. With a QUERY, favorites the one
album in your collection whose "Artist - Title" contains it, case-insensitively.
If the query matches several albums, they are listed with their release IDs
and nothing is added; narrow it with filters, or name one with --release-id.

Two pressings of a title can be identical in every other field -- two
store-exclusive colours, say -- so --release-id is the one that always works.

Flags (the filters narrow a QUERY; --release-id needs none):
` + filterFlagHelp,
			run: func(args []string) {
				cfg, err := parseFavorite("favorite", args)
				if handleParseErr("favorite", err) {
					return
				}
				runFavorite(cfg)
			},
		},
		{
			name:        "unfavorite",
			needsConfig: true,
			summary:     "Remove an album from favorites",
			usage: `Usage: disc-fortune unfavorite [QUERY] [flags]

With no QUERY, unfavorites the last pick. With a QUERY, removes the one
favorite whose "Artist - Title" contains it, case-insensitively. Removing
something that is not favorited succeeds quietly.

Flags (the filters narrow a QUERY; --release-id needs none):
` + filterFlagHelp,
			run: func(args []string) {
				cfg, err := parseFavorite("unfavorite", args)
				if handleParseErr("unfavorite", err) {
					return
				}
				runUnfavorite(cfg)
			},
		},
		{
			name:        "migrate",
			needsConfig: true,
			summary:     "Move config to the XDG location",
			usage: `Usage: disc-fortune migrate

Moves disc-fortune's data files from the legacy ~/.config/disc-fortune to the
directory XDG_CONFIG_HOME points at.

disc-fortune keeps using the legacy directory when it already holds your data,
even with XDG_CONFIG_HOME set, so that an upgrade never appears to lose your
collection. This command performs the move once you are ready. It refuses to
run if the destination already contains files.`,
			run: func(args []string) {
				if handleParseErr("migrate", parseNoArgs("migrate", args)) {
					return
				}
				runMigrate()
			},
		},
		{
			name:    "version",
			summary: "Print the version",
			usage:   "Usage: disc-fortune version\n\nPrints the disc-fortune version and exits.",
			run: func(args []string) {
				if handleParseErr("version", parseNoArgs("version", args)) {
					return
				}
				fmt.Printf("disc-fortune %s\n", version)
			},
		},
		{
			name:    "help",
			summary: "Show help for a command",
			usage:   "Usage: disc-fortune help [COMMAND]\n\nShows general help, or detailed help for one command.",
			run: func(args []string) {
				topic, err := parseHelp(args)
				if handleParseErr("help", err) {
					return
				}
				out, err := helpText(topic)
				if err != nil {
					fatal("%v", err)
				}
				fmt.Println(out)
			},
		},
	}

	// Documented centrally, matching where they are registered.
	for i := range commands {
		if commands[i].name == "help" {
			continue
		}
		commands[i].usage += globalFlagHelp
	}
}

// dispatch resolves argv and runs the chosen command.
func dispatch(args []string) {
	cmd, rest, err := resolve(args)
	if err != nil {
		fatal("disc-fortune: %v", err)
	}
	// Resolved once, here, so every path helper below can be infallible.
	// A failure is only fatal for the commands that actually need it.
	if err := initConfig(os.Getenv, os.UserHomeDir); err != nil {
		if cmd.needsConfig {
			fatal("disc-fortune: %v", err)
		}
	} else if cmd.needsConfig {
		fmt.Fprint(os.Stderr, migrationNotice(activeConfig, metaPath(), isTTY(os.Stderr)))
	}
	cmd.run(rest)
}
