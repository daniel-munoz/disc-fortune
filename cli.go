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

// filterFlags holds the filter flags shared by pick, list, favorite and
// unfavorite. Registering them in one place keeps their names and help text
// from drifting apart between commands.
//
// Every narrowing filter is repeatable and has an --exclude-NAME twin;
// include and exclude are indexed by position in filterFields. --release-id
// is the exception, because it identifies one record rather than narrowing a
// query.
type filterFlags struct {
	include []*arrayFlags
	exclude []*arrayFlags
	// year and decade are two spellings of one constraint, kept apart
	// only long enough to parse them differently.
	year      arrayFlags
	noYear    arrayFlags
	decade    arrayFlags
	noDecade  arrayFlags
	releaseID *int
}

// nonSubstringFilterFlag describes one filter flag outside filterFields: it
// parses its value rather than substring-matching, which is why the matching
// engine's table has no room for it.
type nonSubstringFilterFlag struct {
	name, arg, help string
	twin            bool
}

// registeredHelp is the one text addFilterFlags registers a flag with and
// buildFilterFlagHelp displays for it. Before this existed, --year's help
// string was hand-copied into both places and drifted: the copy in
// addFilterFlags never picked up the "(repeatable)" suffix every table-driven
// flag's registration carries. Routing both call sites through this method
// makes that kind of drift impossible rather than merely unlikely.
func (f nonSubstringFilterFlag) registeredHelp() string {
	if f.twin {
		return f.help + " (repeatable)"
	}
	return f.help
}

// nonSubstringFilterFlags are the filter flags outside filterFields: they
// parse their values rather than substring-matching, which is why the
// matching engine's table has no room for them. Their help lives here so it
// has one source, and so shell completion can enumerate the whole surface.
var nonSubstringFilterFlags = []nonSubstringFilterFlag{
	{"year", "VALUE", "Filter by year or year range (e.g., 1975 or 1970-1980)", true},
	{"decade", "VALUE", "Filter by decade (e.g., 70s or 1970s); adds to --year", true},
	{"release-id", "N", "Select one exact record by its Discogs release ID (single-valued, no twin)", false},
}

func addFilterFlags(fs *flag.FlagSet) *filterFlags {
	ff := &filterFlags{
		include: make([]*arrayFlags, len(filterFields)),
		exclude: make([]*arrayFlags, len(filterFields)),
	}
	for i, field := range filterFields {
		inc, exc := new(arrayFlags), new(arrayFlags)
		fs.Var(inc, field.name, field.help+" (repeatable)")
		fs.Var(exc, "exclude-"+field.name, "Exclude matches of "+field.name+" (repeatable)")
		ff.include[i], ff.exclude[i] = inc, exc
	}
	for _, f := range nonSubstringFilterFlags {
		switch f.name {
		case "year":
			fs.Var(&ff.year, f.name, f.registeredHelp())
			fs.Var(&ff.noYear, "exclude-"+f.name, "Exclude a year or year range (repeatable)")
		case "decade":
			fs.Var(&ff.decade, f.name, f.registeredHelp())
			fs.Var(&ff.noDecade, "exclude-"+f.name, "Exclude a decade (repeatable)")
		case "release-id":
			ff.releaseID = fs.Int(f.name, 0, f.registeredHelp())
		}
	}
	return ff
}

// Filter builds a Filter from the parsed flags, validating year and decade
// values.
func (ff *filterFlags) Filter() (Filter, error) {
	f := Filter{ReleaseID: *ff.releaseID}
	for i, field := range filterFields {
		p := field.part(&f)
		p.Include = nonEmpty(*ff.include[i])
		p.Exclude = nonEmpty(*ff.exclude[i])
	}

	var err error
	if f.Year.Include, err = parseYearValues(ff.year, ff.decade); err != nil {
		return Filter{}, err
	}
	if f.Year.Exclude, err = parseYearValues(ff.noYear, ff.noDecade); err != nil {
		return Filter{}, err
	}
	return f, nil
}

// nonEmpty drops empty values, so `--genre "$GENRE"` with an unset variable
// keeps meaning "no genre filter" as it always has. It matters more for
// exclusions: every string contains "", so an empty --exclude-genre reaching
// the matcher would exclude the entire collection.
func nonEmpty(vals []string) []string {
	var out []string
	for _, v := range vals {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// parseYearValues turns --year and --decade values into one list of ranges.
// They feed a single constraint on purpose: --year 1959 --decade 70s means
// "1959 or the 70s", not the empty intersection two AND-ed fields would give.
func parseYearValues(years, decades []string) ([]yearRange, error) {
	var out []yearRange
	for _, v := range years {
		if v == "" {
			continue
		}
		r, err := parseYearValue(v)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	for _, v := range decades {
		if v == "" {
			continue
		}
		r, err := parseDecadeValue(v)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// anyNarrowing reports whether a filter that only *refines* a query was set.
// Those cannot stand alone: --year 1959 does not say which record is meant.
//
// Two deliberate exclusions from the count. --release-id identifies one exact
// record and needs nothing beside it. A --query *inclusion* is itself a
// query, so it is reported by hasQuery instead -- but an --exclude-query only
// says which record is not meant, so it narrows like any other exclusion.
func (ff *filterFlags) anyNarrowing() bool {
	for i := range filterFields {
		if i != queryField && len(nonEmpty(*ff.include[i])) > 0 {
			return true
		}
		if len(nonEmpty(*ff.exclude[i])) > 0 {
			return true
		}
	}
	return len(nonEmpty(ff.year)) > 0 || len(nonEmpty(ff.noYear)) > 0 ||
		len(nonEmpty(ff.decade)) > 0 || len(nonEmpty(ff.noDecade)) > 0
}

// hasQuery reports whether --query named something to look for, which is what
// lets it satisfy favorite's "requires a query" rule.
func (ff *filterFlags) hasQuery() bool {
	return len(ff.queryValues()) > 0
}

// queryValues returns the --query values, empty ones dropped.
func (ff *filterFlags) queryValues() []string {
	return nonEmpty(*ff.include[queryField])
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
	// json switches the data channel to the documented machine-readable
	// payload. It changes the format only: exit codes, stderr advice and
	// history side effects are identical either way.
	json bool
}

// favoriteConfig is the parsed form of favorite and unfavorite. query is the
// human-readable description of what was asked for -- the constraint itself
// lives in filter.Query. An empty query means "the last pick".
type favoriteConfig struct {
	query  string
	filter Filter
	color  colorMode
}

// historyConfig is the parsed form of history. A limit of 0 means "all".
type historyConfig struct {
	limit int
	color colorMode
	json  bool
}

// statsConfig is the parsed form of stats.
type statsConfig struct {
	favoritesOnly bool
	filter        Filter
	color         colorMode
	json          bool
}

// syncConfig is the parsed form of sync.
type syncConfig struct {
	folders []string
}

// selectionFlags holds the flags pick and list register. Registration lives in
// a function rather than inline so `completion` can enumerate a command's flags
// from the same FlagSet the command parses with -- a flag cannot be accepted
// without also being completable.
type selectionFlags struct {
	favoritesOnly *bool
	unheard       *bool
	asJSON        *bool
	// draw is nil for list, which draws nothing.
	draw    *string
	filters *filterFlags
}

func addSelectionFlags(name string, fs *flag.FlagSet) *selectionFlags {
	sf := &selectionFlags{
		favoritesOnly: fs.Bool("favorites", false, "Restrict to favorites only"),
		unheard:       fs.Bool("unheard", false, "Restrict to albums never picked before"),
		asJSON:        fs.Bool("json", false, "Emit machine-readable JSON instead of text"),
	}

	// --draw is registered only where something is actually drawn, so
	// `list --draw stale` fails as an unknown flag rather than being
	// accepted and silently ignored. Nothing else has to check for it.
	if name != "list" {
		sf.draw = fs.String("draw", "fresh", "How to draw a pick: any, fresh, or stale")
	}

	sf.filters = addFilterFlags(fs)
	return sf
}

func parseSelection(name string, args []string) (selection, error) {
	fs, gf := newFlagSet(name)
	sf := addSelectionFlags(name, fs)
	favoritesOnly, unheard, asJSON, draw, ff := sf.favoritesOnly, sf.unheard, sf.asJSON, sf.draw, sf.filters

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
		json:          *asJSON,
	}, nil
}

// parseQueryCommand is the grammar favorite, unfavorite and open share: an
// optional positional QUERY that sets the same constraint --query does, with
// a release ID excusing its absence. The flag set arrives already built so
// each command can register its own flags on it first.
//
// It exists so a third command with this grammar cannot drift from the first
// two. Copying forty lines to get one is how a CLI ends up with three
// slightly different answers to "what does a bare filter mean?".
func parseQueryCommand(name string, fs *flag.FlagSet, gf *globalFlags, ff *filterFlags, args []string) (favoriteConfig, error) {
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

	// The positional QUERY and --query are one thing said two ways. Giving
	// both would be an OR by the grammar's own rule, but on a command that
	// acts on one record a surprise is worse than a refusal.
	if len(rest) == 1 && ff.hasQuery() {
		return favoriteConfig{}, fmt.Errorf(
			"%s: give the query once, as an argument or --query", name)
	}

	if len(rest) == 0 {
		// A release ID is a complete answer by itself, so it excuses the
		// missing query -- and carries any narrowing filters along with it.
		if ff.anyNarrowing() && !ff.identifies() && !ff.hasQuery() {
			return favoriteConfig{}, fmt.Errorf("%s: filters require a query", name)
		}
		return favoriteConfig{
			query:  strings.Join(ff.queryValues(), " or "),
			filter: filter,
			color:  color,
		}, nil
	}

	query := strings.TrimSpace(rest[0])
	if query == "" {
		return favoriteConfig{}, fmt.Errorf("%s: requires a query", name)
	}
	// The positional query is the same constraint --query would have set, so
	// it goes to the same place. cfg.query keeps only the description: an
	// empty one still means "the last pick".
	filter.Query.Include = append(filter.Query.Include, query)
	return favoriteConfig{query: query, filter: filter, color: color}, nil
}

func parseFavorite(name string, args []string) (favoriteConfig, error) {
	fs, gf := newFlagSet(name)
	ff := addFilterFlags(fs)
	return parseQueryCommand(name, fs, gf, ff, args)
}

// openConfig is the parsed form of open. As with favoriteConfig, an empty
// query means "the last pick".
type openConfig struct {
	query     string
	filter    Filter
	color     colorMode
	printOnly bool
}

// addOpenFlags registers open's flags. See addSelectionFlags for why
// registration is factored out of the parse function.
func addOpenFlags(fs *flag.FlagSet) (*bool, *filterFlags) {
	printOnly := fs.Bool("print", false, "Print the URL instead of opening a browser")
	return printOnly, addFilterFlags(fs)
}

func parseOpen(args []string) (openConfig, error) {
	fs, gf := newFlagSet("open")
	printOnly, ff := addOpenFlags(fs)

	cfg, err := parseQueryCommand("open", fs, gf, ff, args)
	if err != nil {
		return openConfig{}, err
	}
	return openConfig{
		query:     cfg.query,
		filter:    cfg.filter,
		color:     cfg.color,
		printOnly: *printOnly,
	}, nil
}

// addHistoryFlags registers history's flags. See addSelectionFlags for why
// registration is factored out of the parse function.
func addHistoryFlags(fs *flag.FlagSet) *bool {
	return fs.Bool("json", false, "Emit machine-readable JSON instead of text")
}

func parseHistory(args []string) (historyConfig, error) {
	fs, gf := newFlagSet("history")
	asJSON := addHistoryFlags(fs)
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
	return historyConfig{limit: limit, color: color, json: *asJSON}, nil
}

// statsFlags holds the flags stats registers. See addSelectionFlags for why
// registration is factored out of the parse function.
//
// No --unheard: that flag is defined by history, and "share ever picked" over
// an unheard-only set is 0% by construction, so its only effect would be to
// make one of the headline figures meaningless. No --draw either: that is a
// draw strategy and stats draws nothing.
type statsFlags struct {
	favoritesOnly *bool
	asJSON        *bool
	filters       *filterFlags
}

func addStatsFlags(fs *flag.FlagSet) *statsFlags {
	return &statsFlags{
		favoritesOnly: fs.Bool("favorites", false, "Describe favorites only"),
		asJSON:        fs.Bool("json", false, "Emit machine-readable JSON instead of text"),
		filters:       addFilterFlags(fs),
	}
}

// parseStats parses stats's arguments.
//
// Unlike favorite, unfavorite and open, it does not apply the
// "filters require a query" rule. Those commands act on exactly one record,
// and a filter alone does not say which; stats is set-oriented like list and
// pick, so `stats --genre jazz` is a complete request.
func parseStats(args []string) (statsConfig, error) {
	fs, gf := newFlagSet("stats")
	sf := addStatsFlags(fs)

	rest, err := parseInterspersed(fs, args)
	if err != nil {
		return statsConfig{}, fmt.Errorf("stats: %w", err)
	}
	if len(rest) > 0 {
		return statsConfig{}, fmt.Errorf("stats: unexpected argument %q", rest[0])
	}
	filter, err := sf.filters.Filter()
	if err != nil {
		return statsConfig{}, fmt.Errorf("stats: %v", err)
	}
	color, err := gf.mode()
	if err != nil {
		return statsConfig{}, fmt.Errorf("stats: %v", err)
	}

	return statsConfig{
		favoritesOnly: *sf.favoritesOnly,
		filter:        filter,
		color:         color,
		json:          *sf.asJSON,
	}, nil
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

// addSyncFlags registers sync's flags. See addSelectionFlags for why
// registration is factored out of the parse function.
func addSyncFlags(fs *flag.FlagSet) *arrayFlags {
	folders := new(arrayFlags)
	fs.Var(folders, "folder", "Sync only specific folder(s) by name (repeatable)")
	return folders
}

func parseSync(args []string) (syncConfig, error) {
	fs, gf := newFlagSet("sync")
	folders := addSyncFlags(fs)

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
	return syncConfig{folders: *folders}, nil
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

// filterFlagHelp is the shared help block for the filter flags, generated
// from filterFields and nonSubstringFilterFlags so a new filter cannot ship
// undocumented. The --exclude-NAME twins are named once by the heading
// rather than listed: sixteen near-identical lines would bury the eight that
// matter -- --release-id, which has no twin, says so on its own line instead.
// TestFilterFlagsAreDocumented enforces both halves of that bargain.
var filterFlagHelp = buildFilterFlagHelp()

func buildFilterFlagHelp() string {
	var sb strings.Builder
	sb.WriteString("\nFilters (all repeatable; each has an --exclude-NAME twin that removes matches):\n")
	for _, field := range filterFields {
		fmt.Fprintf(&sb, "  --%-12s VALUE  %s\n", field.name, field.help)
	}
	for _, f := range nonSubstringFilterFlags {
		fmt.Fprintf(&sb, "  --%-12s %-7s%s\n", f.name, f.arg, f.registeredHelp())
	}
	// The old hand-written constant ended without a trailing newline, and
	// every usage block appends filterFlagHelp straight after its own line
	// ending in "\n" -- so a trailing newline here would double up with
	// globalFlagHelp's leading "\n\n" and add a stray blank line.
	return strings.TrimRight(sb.String(), "\n")
}

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
  --json           Emit machine-readable JSON instead of text
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
  --json           Emit machine-readable JSON instead of text
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
			usage: `Usage: disc-fortune history [N] [flags]

Shows the last N picks. N defaults to 10; 0 shows all of them.

Flags:
  --json           Emit machine-readable JSON instead of text`,
			run: func(args []string) {
				cfg, err := parseHistory(args)
				if handleParseErr("history", err) {
					return
				}
				runHistory(cfg)
			},
		},
		{
			name:        "stats",
			needsConfig: true,
			summary:     "Summarize your collection",
			usage: `Usage: disc-fortune stats [flags]

Summarizes whatever the filters describe: a decade histogram, your most
common genres and labels, and how much of the set you have ever played.
Reads only files already on disk.

Flags:
  --favorites      Describe favorites only
  --json           Emit machine-readable JSON instead of text
` + filterFlagHelp,
			run: func(args []string) {
				cfg, err := parseStats(args)
				if handleParseErr("stats", err) {
					return
				}
				runStats(cfg)
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

The QUERY can also be given as --query, which is the only difference between
the two spellings. --release-id needs neither.
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

The QUERY can also be given as --query, which is the only difference between
the two spellings. --release-id needs neither.
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
			name:        "open",
			needsConfig: true,
			summary:     "Open a record's Discogs page",
			usage: `Usage: disc-fortune open [QUERY] [flags]

With no QUERY, opens the last pick. With a QUERY, opens the one album in your
collection whose "Artist - Title" contains it, case-insensitively. If the
query matches several albums, they are listed with their release IDs and
nothing is opened; narrow it with filters, or name one with --release-id.

With no browser to launch into -- no launcher on PATH, or no display -- the
URL is printed instead and the command still succeeds.

Flags:
  --print          Print the URL instead of opening a browser
` + filterFlagHelp,
			run: func(args []string) {
				cfg, err := parseOpen(args)
				if handleParseErr("open", err) {
					return
				}
				runOpen(cfg)
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
			// needsConfig stays false: generating a script reads no
			// data files, so completion must keep working on a machine
			// with no usable home directory.
			name:    "completion",
			summary: "Print a shell completion script",
			usage: `Usage: disc-fortune completion SHELL

Prints a completion script for bash, zsh or fish on stdout. The script is
generated from the commands and flags this binary actually accepts, so it
cannot drift from them.

Load it for the current shell:

  bash    eval "$(disc-fortune completion bash)"
  zsh     eval "$(disc-fortune completion zsh)"
  fish    disc-fortune completion fish | source

To make it permanent, add that line to your shell's startup file, or write the
script into the directory your shell reads completions from.

Command and flag names are completed, as are the fixed values of --draw and
--color. Values that would have to be read from your collection, such as those
of --genre and --label, are not: a completion should never depend on a file
that a sync may be rewriting.`,
			run: func(args []string) {
				shell, err := parseCompletion(args)
				if handleParseErr("completion", err) {
					return
				}
				runCompletion(shell)
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
