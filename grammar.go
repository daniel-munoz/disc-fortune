package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/daniel-munoz/disc-fortune/v2/internal/cli"
	"github.com/daniel-munoz/disc-fortune/v2/internal/disc"
	"github.com/daniel-munoz/disc-fortune/v2/internal/pick"
	"github.com/daniel-munoz/disc-fortune/v2/internal/term"
)

// filterFlags holds the filter flags shared by pick, list, favorite and
// unfavorite. Registering them in one place keeps their names and help text
// from drifting apart between commands.
//
// Every narrowing filter is repeatable and has an --exclude-NAME twin;
// include and exclude are indexed by position in disc.Fields. --release-id
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

// nonSubstringFilterFlag describes one filter flag outside disc.Fields: it
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

// nonSubstringFilterFlags are the filter flags outside disc.Fields: they
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
		include: make([]*arrayFlags, len(disc.Fields)),
		exclude: make([]*arrayFlags, len(disc.Fields)),
	}
	for i, field := range disc.Fields {
		inc, exc := new(arrayFlags), new(arrayFlags)
		fs.Var(inc, field.Name, field.Help+" (repeatable)")
		fs.Var(exc, "exclude-"+field.Name, "Exclude matches of "+field.Name+" (repeatable)")
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
func (ff *filterFlags) Filter() (disc.Filter, error) {
	f := disc.Filter{ReleaseID: *ff.releaseID}
	for i, field := range disc.Fields {
		p := field.Part(&f)
		p.Include = nonEmpty(*ff.include[i])
		p.Exclude = nonEmpty(*ff.exclude[i])
	}

	var err error
	if f.Year.Include, err = parseYearValues(ff.year, ff.decade); err != nil {
		return disc.Filter{}, err
	}
	if f.Year.Exclude, err = parseYearValues(ff.noYear, ff.noDecade); err != nil {
		return disc.Filter{}, err
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
func parseYearValues(years, decades []string) ([]disc.YearRange, error) {
	var out []disc.YearRange
	for _, v := range years {
		if v == "" {
			continue
		}
		r, err := disc.ParseYearValue(v)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	for _, v := range decades {
		if v == "" {
			continue
		}
		r, err := disc.ParseDecadeValue(v)
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
	for i := range disc.Fields {
		if i != disc.QueryField && len(nonEmpty(*ff.include[i])) > 0 {
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
	return nonEmpty(*ff.include[disc.QueryField])
}

// identifies reports whether the flags name one exact record on their own.
func (ff *filterFlags) identifies() bool {
	return *ff.releaseID != 0
}

// filterFlagHelp is the shared help block for the filter flags, generated
// from disc.Fields and nonSubstringFilterFlags so a new filter cannot ship
// undocumented. The --exclude-NAME twins are named once by the heading
// rather than listed: sixteen near-identical lines would bury the eight that
// matter -- --release-id, which has no twin, says so on its own line instead.
// TestFilterFlagsAreDocumented enforces both halves of that bargain.
var filterFlagHelp = buildFilterFlagHelp()

func buildFilterFlagHelp() string {
	var sb strings.Builder
	sb.WriteString("\nFilters (all repeatable; each has an --exclude-NAME twin that removes matches):\n")
	for _, field := range disc.Fields {
		fmt.Fprintf(&sb, "  --%-12s VALUE  %s\n", field.Name, field.Help)
	}
	for _, f := range nonSubstringFilterFlags {
		fmt.Fprintf(&sb, "  --%-12s %-7s%s\n", f.name, f.arg, f.registeredHelp())
	}
	// The old hand-written constant ended without a trailing newline, and
	// every usage block appends filterFlagHelp straight after its own line
	// ending in "\n" -- so a trailing newline here would double up with
	// cli.GlobalFlagHelp's leading "\n\n" and add a stray blank line.
	return strings.TrimRight(sb.String(), "\n")
}

// selection is the parsed form of the flags shared by pick and list.
type selection struct {
	favoritesOnly bool
	// unheard restricts to albums that have never been picked. It is a
	// filter on the candidate set, like favoritesOnly, not a draw strategy
	// -- which is what lets `list` have it too.
	unheard bool
	// draw is how pick chooses from the candidates. It is meaningless for
	// list, which never sets it.
	draw   pick.Mode
	filter disc.Filter
	color  term.Mode
	// json switches the data channel to the documented machine-readable
	// payload. It changes the format only: exit codes, stderr advice and
	// history side effects are identical either way.
	json bool
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

// selectionCmd is the grammar pick, reroll and list share. Each embeds it and
// adds only its own Run.
type selectionCmd struct {
	name string
	sf   *selectionFlags
	g    *cli.Globals
	cfg  selection
}

func (c *selectionCmd) Flags(fs *flag.FlagSet, g *cli.Globals) {
	c.sf, c.g = addSelectionFlags(c.name, fs), g
}

func (c *selectionCmd) Parse(rest []string) error {
	name, sf := c.name, c.sf
	if len(rest) > 0 {
		return fmt.Errorf("%s: unexpected argument %q", name, rest[0])
	}
	filter, err := sf.filters.Filter()
	if err != nil {
		return fmt.Errorf("%s: %v", name, err)
	}
	color, err := c.g.Mode()
	if err != nil {
		return fmt.Errorf("%s: %v", name, err)
	}

	mode := pick.Fresh
	if sf.draw != nil {
		m, err := pick.ParseMode(*sf.draw)
		if err != nil {
			return fmt.Errorf("%s: %v", name, err)
		}
		mode = m
	}

	c.cfg = selection{
		favoritesOnly: *sf.favoritesOnly,
		unheard:       *sf.unheard,
		draw:          mode,
		filter:        filter,
		color:         color,
		json:          *sf.asJSON,
	}
	return nil
}

// favoriteConfig is the parsed form of favorite and unfavorite. query is the
// human-readable description of what was asked for -- the constraint itself
// lives in filter.Query. An empty query means "the last pick".
type favoriteConfig struct {
	query  string
	filter disc.Filter
	color  term.Mode
}

// parseQueryCommand is the grammar favorite, unfavorite and open share: an
// optional positional QUERY that sets the same constraint --query does, with
// a release ID excusing its absence. Flags have already been parsed; rest
// is the positionals.
//
// It exists so a third command with this grammar cannot drift from the first
// two. Copying forty lines to get one is how a CLI ends up with three
// slightly different answers to "what does a bare filter mean?".
func parseQueryCommand(name string, gf *cli.Globals, ff *filterFlags, rest []string) (favoriteConfig, error) {
	if len(rest) > 1 {
		return favoriteConfig{}, fmt.Errorf(
			"%s: too many arguments (quote the query: %s %q)",
			name, name, strings.Join(rest, " "))
	}
	filter, err := ff.Filter()
	if err != nil {
		return favoriteConfig{}, fmt.Errorf("%s: %v", name, err)
	}
	color, err := gf.Mode()
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

// queryCmd is the grammar favorite and unfavorite share: filter flags plus
// parseQueryCommand.
type queryCmd struct {
	name string
	ff   *filterFlags
	g    *cli.Globals
	cfg  favoriteConfig
}

func (c *queryCmd) Flags(fs *flag.FlagSet, g *cli.Globals) {
	c.ff, c.g = addFilterFlags(fs), g
}

func (c *queryCmd) Parse(rest []string) error {
	cfg, err := parseQueryCommand(c.name, c.g, c.ff, rest)
	c.cfg = cfg
	return err
}

// noArgsCmd is the grammar of a command that takes no flags and no arguments
// beyond the global ones.
type noArgsCmd struct {
	name string
	g    *cli.Globals
}

func (c *noArgsCmd) Flags(_ *flag.FlagSet, g *cli.Globals) { c.g = g }

func (c *noArgsCmd) Parse(rest []string) error {
	if len(rest) > 0 {
		return fmt.Errorf("%s: unexpected argument %q", c.name, rest[0])
	}
	// These commands produce no colorized output, but they still accept the
	// flag, so they must still reject a bad value for it.
	if _, err := c.g.Mode(); err != nil {
		return fmt.Errorf("%s: %v", c.name, err)
	}
	return nil
}
