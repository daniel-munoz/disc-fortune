package main

import (
	"flag"
	"strings"
	"testing"
)

func TestPickAcceptsColorFlag(t *testing.T) {
	cfg, err := parseSelection("pick", []string{"--color", "always"})
	if err != nil {
		t.Fatalf("parseSelection: %v", err)
	}
	if cfg.color != colorAlways {
		t.Errorf("color = %v, want colorAlways", cfg.color)
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
	if cfg.color != colorNever {
		t.Errorf("color = %v, want colorNever", cfg.color)
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
	if cfg.color != colorNever {
		t.Errorf("color = %v, want colorNever", cfg.color)
	}
}

// The point of registering global flags in newFlagSet: no command can miss
// them. This is the test that keeps a future command honest.
func TestEveryCommandAcceptsColorFlag(t *testing.T) {
	parsers := map[string]func([]string) error{
		"pick":       func(a []string) error { _, err := parseSelection("pick", a); return err },
		"list":       func(a []string) error { _, err := parseSelection("list", a); return err },
		"favorite":   func(a []string) error { _, err := parseFavorite("favorite", a); return err },
		"unfavorite": func(a []string) error { _, err := parseFavorite("unfavorite", a); return err },
		"history":    func(a []string) error { _, err := parseHistory(a); return err },
		"sync":       func(a []string) error { _, err := parseSync(a); return err },
		"folders":    func(a []string) error { return parseNoArgs("folders", a) },
		"version":    func(a []string) error { return parseNoArgs("version", a) },
		"migrate":    func(a []string) error { return parseNoArgs("migrate", a) },
	}
	for name, parse := range parsers {
		if err := parse([]string{"--color", "never"}); err != nil {
			t.Errorf("%s rejected --color: %v", name, err)
		}
	}
	if len(parsers) != len(commands)-1 { // help takes a topic, not flags
		t.Errorf("this test covers %d commands but there are %d; add the new one",
			len(parsers), len(commands)-1)
	}
}

// A flag that works but is undocumented may as well not exist.
func TestColorFlagIsDocumentedEverywhere(t *testing.T) {
	for _, c := range commands {
		if c.name == "help" {
			continue
		}
		if !strings.Contains(c.usage, "--color") {
			t.Errorf("%s usage does not mention --color", c.name)
		}
	}
}

func TestMigrateCommandExists(t *testing.T) {
	c := lookup("migrate")
	if c == nil {
		t.Fatal("no migrate command registered")
	}
	if c.summary == "" {
		t.Error("migrate has no summary, so `help` would list it blank")
	}
	if !strings.Contains(c.usage, "XDG_CONFIG_HOME") {
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
	base, _ := newFlagSet("pick")
	global := map[string]bool{}
	base.VisitAll(func(f *flag.Flag) { global[f.Name] = true })

	fs, _ := newFlagSet("pick")
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
	for _, c := range commands {
		// A command takes the filter flags if it documents any of them.
		if !strings.Contains(c.usage, "--year") {
			continue
		}
		documented++

		if !strings.Contains(c.usage, "--exclude-NAME twin") {
			t.Errorf("%s usage does not explain the --exclude-NAME twins", c.name)
		}
		for _, name := range names {
			if strings.Contains(c.usage, "--"+name) {
				continue
			}
			twin, isTwin := strings.CutPrefix(name, "exclude-")
			if isTwin && strings.Contains(c.usage, "--"+twin) {
				continue
			}
			t.Errorf("%s usage does not mention --%s", c.name, name)
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
// property directly: each of the three flags outside filterFields is
// registered with, and documented with, the very same string.
func TestNonTableFilterFlagsHaveOneHelpSource(t *testing.T) {
	fs, _ := newFlagSet("pick")
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

// buildFilterFlagHelp generates filterFlagHelp from filterFields, and every
// usage block appends it straight after a line already ending in "\n". A
// trailing newline left on the generated block would double up with that
// "\n" (and with globalFlagHelp's own leading "\n\n"), adding a stray blank
// line to every help and usage-error screen. Three consecutive newlines
// anywhere in a usage block is that regression.
func TestUsageBlocksHaveNoDoubleBlankLines(t *testing.T) {
	for _, c := range commands {
		if strings.Contains(c.usage, "\n\n\n") {
			t.Errorf("%s usage contains a doubled blank line", c.name)
		}
	}
}

// The commands that accept --unheard must document it, and the ones that do
// not must not claim to. Same guard as TestFilterFlagsAreDocumented, for a
// flag that is registered per-command rather than centrally.
func TestUnheardFlagIsDocumentedWhereAccepted(t *testing.T) {
	for _, name := range []string{"pick", "list"} {
		c := lookup(name)
		if c == nil {
			t.Fatalf("command %q not found", name)
		}
		if !strings.Contains(c.usage, "--unheard") {
			t.Errorf("%s usage does not mention --unheard", name)
		}
	}
	for _, name := range []string{"favorite", "unfavorite"} {
		c := lookup(name)
		if c == nil {
			t.Fatalf("command %q not found", name)
		}
		if strings.Contains(c.usage, "--unheard") {
			t.Errorf("%s documents --unheard but does not accept it", name)
		}
	}
}

func TestDrawFlagIsDocumentedOnPickOnly(t *testing.T) {
	if c := lookup("pick"); !strings.Contains(c.usage, "--draw") {
		t.Error("pick usage does not mention --draw")
	}
	if c := lookup("list"); strings.Contains(c.usage, "--draw") {
		t.Error("list documents --draw but does not accept it")
	}
}

// The commands that accept --json must document it, and the ones that do not
// must not claim to. Same guard as TestUnheardFlagIsDocumentedWhereAccepted.
func TestJSONFlagIsDocumentedWhereAccepted(t *testing.T) {
	for _, name := range []string{"pick", "list", "history"} {
		c := lookup(name)
		if c == nil {
			t.Fatalf("command %q not found", name)
		}
		if !strings.Contains(c.usage, "--json") {
			t.Errorf("%s usage does not mention --json", name)
		}
	}
	for _, name := range []string{"favorite", "unfavorite", "sync", "folders", "migrate", "version", "help"} {
		c := lookup(name)
		if c == nil {
			continue
		}
		if strings.Contains(c.usage, "--json") {
			t.Errorf("%s documents --json but does not accept it", name)
		}
	}
}
