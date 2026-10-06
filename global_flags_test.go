package main

import (
	"flag"
	"strings"
	"testing"

	"github.com/daniel-munoz/disc-fortune/v2/internal/cli"
	"github.com/daniel-munoz/disc-fortune/v2/internal/term"
)

func TestPickAcceptsColorFlag(t *testing.T) {
	cfg, err := parseSelection("pick", []string{"--color", "always"})
	if err != nil {
		t.Fatalf("parseSelection: %v", err)
	}
	if cfg.color != term.Always {
		t.Errorf("color = %v, want term.Always", cfg.color)
	}
}

func TestColorFlagRejectsUnknownValue(t *testing.T) {
	_, err := parseSelection("pick", []string{"--color", "sometimes"})
	if err == nil {
		t.Fatal("expected a usage error for --color=sometimes")
	}
	if !strings.Contains(err.Error(), "pick") {
		t.Errorf("error %q should name the command", err)
	}
}

func TestHistoryAcceptsColorFlag(t *testing.T) {
	cfg, err := parseHistory([]string{"--color", "never", "5"})
	if err != nil {
		t.Fatalf("parseHistory: %v", err)
	}
	if cfg.color != term.Never {
		t.Errorf("color = %v, want term.Never", cfg.color)
	}
	if cfg.limit != 5 {
		t.Errorf("limit = %d, want 5", cfg.limit)
	}
}

func TestFavoriteAcceptsColorFlag(t *testing.T) {
	cfg, err := parseFavorite("favorite", []string{"miles", "--color", "never"})
	if err != nil {
		t.Fatalf("parseFavorite: %v", err)
	}
	if cfg.color != term.Never {
		t.Errorf("color = %v, want term.Never", cfg.color)
	}
}

// The point of registering global flags in cli.NewFlagSet: no command can miss
// them. This is the test that keeps a future command honest.
func TestEveryCommandAcceptsColorFlag(t *testing.T) {
	parsers := map[string]func([]string) error{
		"pick":       func(a []string) error { _, err := parseSelection("pick", a); return err },
		"reroll":     func(a []string) error { _, err := parseSelection("reroll", a); return err },
		"list":       func(a []string) error { _, err := parseSelection("list", a); return err },
		"favorite":   func(a []string) error { _, err := parseFavorite("favorite", a); return err },
		"unfavorite": func(a []string) error { _, err := parseFavorite("unfavorite", a); return err },
		"history":    func(a []string) error { _, err := parseHistory(a); return err },
		"stats":      func(a []string) error { _, err := parseStats(a); return err },
		"open":       func(a []string) error { _, err := parseOpen(a); return err },
		"sync":       func(a []string) error { _, err := parseSync(a); return err },
		"folders":    func(a []string) error { return parseNoArgs("folders", a) },
		"version":    func(a []string) error { return parseNoArgs("version", a) },
		"migrate":    func(a []string) error { return parseNoArgs("migrate", a) },
		"completion": func(a []string) error {
			_, err := parseCompletion(append([]string{"bash"}, a...))
			return err
		},
	}
	for name, parse := range parsers {
		if err := parse([]string{"--color", "never"}); err != nil {
			t.Errorf("%s rejected --color: %v", name, err)
		}
	}
	if len(parsers) != len(program.Commands)-1 { // help takes a topic, not flags
		t.Errorf("this test covers %d commands but there are %d; add the new one",
			len(parsers), len(program.Commands)-1)
	}
}

// A flag that works but is undocumented may as well not exist.
func TestColorFlagIsDocumentedEverywhere(t *testing.T) {
	for _, c := range program.Commands {
		if c.Name == "help" {
			continue
		}
		if !strings.Contains(c.Usage, "--color") {
			t.Errorf("%s usage does not mention --color", c.Name)
		}
	}
}

func TestMigrateCommandExists(t *testing.T) {
	c := program.Lookup("migrate")
	if c == nil {
		t.Fatal("no migrate command registered")
	}
	if c.Summary == "" {
		t.Error("migrate has no summary, so `help` would list it blank")
	}
	if !strings.Contains(c.Usage, "XDG_CONFIG_HOME") {
		t.Error("migrate usage should explain what it migrates and why")
	}
}

// Accepting the flag is only half of it. A typo must be rejected by every
// command, or `disc-fortune folders --color=sometimes` silently does the
// wrong thing while `disc-fortune list --color=sometimes` errors -- exactly
// the drift that registering the flag centrally was meant to prevent.
func TestEveryCommandRejectsInvalidColor(t *testing.T) {
	parsers := map[string]func([]string) error{
		"pick":       func(a []string) error { _, err := parseSelection("pick", a); return err },
		"list":       func(a []string) error { _, err := parseSelection("list", a); return err },
		"favorite":   func(a []string) error { _, err := parseFavorite("favorite", a); return err },
		"unfavorite": func(a []string) error { _, err := parseFavorite("unfavorite", a); return err },
		"history":    func(a []string) error { _, err := parseHistory(a); return err },
		"stats":      func(a []string) error { _, err := parseStats(a); return err },
		"open":       func(a []string) error { _, err := parseOpen(a); return err },
		"sync":       func(a []string) error { _, err := parseSync(a); return err },
		"folders":    func(a []string) error { return parseNoArgs("folders", a) },
		"version":    func(a []string) error { return parseNoArgs("version", a) },
		"migrate":    func(a []string) error { return parseNoArgs("migrate", a) },
	}
	for name, parse := range parsers {
		err := parse([]string{"--color", "sometimes"})
		if err == nil {
			t.Errorf("%s accepted --color=sometimes", name)
			continue
		}
		if !strings.Contains(err.Error(), name) {
			t.Errorf("%s error %q should name the command", name, err)
		}
	}
}

// TestFilterFlagsAreDocumented is the drift guard the filter flags lacked.
// Their help text is assembled separately from the FlagSet, so a flag added
// without help would otherwise leave every usage block quietly stale -- which
// is exactly what happened when --release-id was added.
//
// It walks what addFilterFlags actually registers rather than a hand-written
// list, so a filter added in future fails here without anyone remembering to
// update the test. A flag counts as documented when it is named literally, or
// when it is the --exclude- twin of a flag that is; the block names that
// convention once instead of listing sixteen near-identical lines, so the
// sentence introducing it is required too.
func TestFilterFlagsAreDocumented(t *testing.T) {
	base, _ := cli.NewFlagSet("pick")
	global := map[string]bool{}
	base.VisitAll(func(f *flag.Flag) { global[f.Name] = true })

	fs, _ := cli.NewFlagSet("pick")
	addFilterFlags(fs)
	var names []string
	fs.VisitAll(func(f *flag.Flag) {
		if !global[f.Name] {
			names = append(names, f.Name)
		}
	})
	if len(names) == 0 {
		t.Fatal("addFilterFlags registered nothing; the guard is not testing anything")
	}

	documented := 0
	for _, c := range program.Commands {
		// A command takes the filter flags if it documents any of them.
		if !strings.Contains(c.Usage, "--year") {
			continue
		}
		documented++

		if !strings.Contains(c.Usage, "--exclude-NAME twin") {
			t.Errorf("%s usage does not explain the --exclude-NAME twins", c.Name)
		}
		for _, name := range names {
			if strings.Contains(c.Usage, "--"+name) {
				continue
			}
			twin, isTwin := strings.CutPrefix(name, "exclude-")
			if isTwin && strings.Contains(c.Usage, "--"+twin) {
				continue
			}
			t.Errorf("%s usage does not mention --%s", c.Name, name)
		}
	}
	if documented == 0 {
		t.Fatal("no command documents the filter flags; the guard is not testing anything")
	}
}

// TestNonTableFilterFlagsHaveOneHelpSource closes the gap TestFilterFlagsAreDocumented
// leaves: that test only checks that a flag *name* appears somewhere in the
// usage text, never that the help *text* registered with the flag matches
// what the shared help block shows for it. That gap is exactly how --year's
// registered help fell out of sync with its documented help before
// nonSubstringFilterFlags existed. This asserts the stronger, single-source
// property directly: each of the three flags outside disc.Fields is
// registered with, and documented with, the very same string.
func TestNonTableFilterFlagsHaveOneHelpSource(t *testing.T) {
	fs, _ := cli.NewFlagSet("pick")
	addFilterFlags(fs)
	for _, f := range nonSubstringFilterFlags {
		flg := fs.Lookup(f.name)
		if flg == nil {
			t.Fatalf("addFilterFlags did not register --%s", f.name)
		}
		if flg.Usage != f.registeredHelp() {
			t.Errorf("--%s registered Usage %q, want %q", f.name, flg.Usage, f.registeredHelp())
		}
		if !strings.Contains(filterFlagHelp, flg.Usage) {
			t.Errorf("--%s registered Usage %q does not appear verbatim in filterFlagHelp", f.name, flg.Usage)
		}
		twinRegistered := fs.Lookup("exclude-"+f.name) != nil
		if twinRegistered != f.twin {
			t.Errorf("--%s has a registered --exclude- twin = %v, want %v", f.name, twinRegistered, f.twin)
		}
	}
}

// buildFilterFlagHelp generates filterFlagHelp from disc.Fields, and every
// usage block appends it straight after a line already ending in "\n". A
// trailing newline left on the generated block would double up with that
// "\n" (and with cli.GlobalFlagHelp's own leading "\n\n"), adding a stray blank
// line to every help and usage-error screen. Three consecutive newlines
// anywhere in a usage block is that regression.
func TestUsageBlocksHaveNoDoubleBlankLines(t *testing.T) {
	for _, c := range program.Commands {
		if strings.Contains(c.Usage, "\n\n\n") {
			t.Errorf("%s usage contains a doubled blank line", c.Name)
		}
	}
}

// The commands that accept --unheard must document it, and the ones that do
// not must not claim to. Same guard as TestFilterFlagsAreDocumented, for a
// flag that is registered per-command rather than centrally.
func TestUnheardFlagIsDocumentedWhereAccepted(t *testing.T) {
	accepts := []string{"pick", "reroll", "list"}
	rejects := []string{
		"favorite", "unfavorite", "stats", "open", "history", "sync",
		"folders", "migrate", "version", "help", "completion",
	}
	for _, name := range accepts {
		c := program.Lookup(name)
		if c == nil {
			t.Fatalf("command %q not found", name)
		}
		if !strings.Contains(c.Usage, "--unheard") {
			t.Errorf("%s usage does not mention --unheard", name)
		}
	}
	for _, name := range rejects {
		c := program.Lookup(name)
		if c == nil {
			t.Fatalf("command %q not found", name)
		}
		if strings.Contains(c.Usage, "--unheard") {
			t.Errorf("%s documents --unheard but does not accept it", name)
		}
	}

	// The two lists above are a manual enumeration of every command's stance
	// on --unheard. Nothing forces them to grow with commands, so a new
	// command could accept or reject it silently. This is the same guard as
	// TestEveryCommandHasACompletionDecision in completion_test.go: decide
	// what the new command does with --unheard, then add it to one of the
	// two lists above.
	if got := len(accepts) + len(rejects); got != len(program.Commands) {
		t.Fatalf("this test covers %d commands but there are %d; decide whether "+
			"the new command accepts --unheard, then add it here", got, len(program.Commands))
	}
}

func TestDrawFlagIsDocumentedWhereAccepted(t *testing.T) {
	// addSelectionFlags registers --draw for every selection command except
	// list, so the commands that draw document it and list must not.
	for _, name := range []string{"pick", "reroll"} {
		if c := program.Lookup(name); !strings.Contains(c.Usage, "--draw") {
			t.Errorf("%s usage does not mention --draw", name)
		}
	}
	if c := program.Lookup("list"); strings.Contains(c.Usage, "--draw") {
		t.Error("list documents --draw but does not accept it")
	}
}

// The commands that accept --json must document it, and the ones that do not
// must not claim to. Same guard as TestUnheardFlagIsDocumentedWhereAccepted.
func TestJSONFlagIsDocumentedWhereAccepted(t *testing.T) {
	accepts := []string{"pick", "reroll", "list", "history", "stats"}
	rejects := []string{
		"favorite", "unfavorite", "sync", "folders", "migrate", "version",
		"help", "open", "completion",
	}
	for _, name := range accepts {
		c := program.Lookup(name)
		if c == nil {
			t.Fatalf("command %q not found", name)
		}
		if !strings.Contains(c.Usage, "--json") {
			t.Errorf("%s usage does not mention --json", name)
		}
	}
	for _, name := range rejects {
		c := program.Lookup(name)
		if c == nil {
			continue
		}
		if strings.Contains(c.Usage, "--json") {
			t.Errorf("%s documents --json but does not accept it", name)
		}
	}

	// Same guard as TestUnheardFlagIsDocumentedWhereAccepted, for --json:
	// the two lists above must together cover every command, or a new one
	// could slip past with no stance on --json recorded here at all.
	if got := len(accepts) + len(rejects); got != len(program.Commands) {
		t.Fatalf("this test covers %d commands but there are %d; decide whether "+
			"the new command accepts --json, then add it here", got, len(program.Commands))
	}
}
