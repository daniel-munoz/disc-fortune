package main

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"os"
	"strings"
	"testing"
)

func TestParseInterspersedFlagsAfterPositional(t *testing.T) {
	fs := newFlagSet("favorite")
	year := fs.String("year", "", "")

	rest, err := parseInterspersed(fs, []string{"miles", "--year", "1959"})
	if err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if len(rest) != 1 || rest[0] != "miles" {
		t.Errorf("positional = %v, want [miles]", rest)
	}
	if *year != "1959" {
		t.Errorf("year = %q, want 1959 (flag after positional was dropped)", *year)
	}
}

func TestParseInterspersedFlagsBeforePositional(t *testing.T) {
	fs := newFlagSet("favorite")
	year := fs.String("year", "", "")

	rest, err := parseInterspersed(fs, []string{"--year", "1959", "miles"})
	if err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if len(rest) != 1 || rest[0] != "miles" {
		t.Errorf("positional = %v, want [miles]", rest)
	}
	if *year != "1959" {
		t.Errorf("year = %q, want 1959", *year)
	}
}

func TestParseInterspersedFlagsSurroundingPositional(t *testing.T) {
	fs := newFlagSet("favorite")
	year := fs.String("year", "", "")
	genre := fs.String("genre", "", "")

	rest, err := parseInterspersed(fs, []string{"--genre", "jazz", "miles", "--year", "1959"})
	if err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if len(rest) != 1 || rest[0] != "miles" {
		t.Errorf("positional = %v, want [miles]", rest)
	}
	if *genre != "jazz" || *year != "1959" {
		t.Errorf("genre = %q, year = %q, want jazz/1959", *genre, *year)
	}
}

func TestParseInterspersedMultiplePositionals(t *testing.T) {
	fs := newFlagSet("favorite")
	rest, err := parseInterspersed(fs, []string{"kind", "of", "blue"})
	if err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if len(rest) != 3 {
		t.Errorf("positional = %v, want 3 items", rest)
	}
}

func TestParseInterspersedDashTerminator(t *testing.T) {
	fs := newFlagSet("favorite")
	rest, err := parseInterspersed(fs, []string{"--", "-live-"})
	if err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if len(rest) != 1 || rest[0] != "-live-" {
		t.Errorf("positional = %v, want [-live-]", rest)
	}
}

func TestParseInterspersedUnknownFlag(t *testing.T) {
	fs := newFlagSet("pick")
	if _, err := parseInterspersed(fs, []string{"--nope"}); err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestFilterFlagsBuildsFilter(t *testing.T) {
	fs := newFlagSet("pick")
	ff := addFilterFlags(fs)
	if _, err := parseInterspersed(fs, []string{"--year", "1970-1980", "--genre", "jazz"}); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	filter, err := ff.Filter()
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if filter.Year != "1970-1980" || filter.Genre != "jazz" {
		t.Errorf("filter = %+v, want Year=1970-1980 Genre=jazz", filter)
	}
	if !ff.any() {
		t.Error("any() = false, want true")
	}
}

func TestFilterFlagsRejectsBadYear(t *testing.T) {
	fs := newFlagSet("pick")
	ff := addFilterFlags(fs)
	if _, err := parseInterspersed(fs, []string{"--year", "nineteen"}); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if _, err := ff.Filter(); err == nil {
		t.Fatal("expected error for non-numeric year")
	}
}

func TestFilterFlagsAnyFalseWhenUnset(t *testing.T) {
	fs := newFlagSet("pick")
	ff := addFilterFlags(fs)
	if _, err := parseInterspersed(fs, nil); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if ff.any() {
		t.Error("any() = true, want false when no filter flags set")
	}
}

func TestParseSelectionFavoritesFlag(t *testing.T) {
	cfg, err := parseSelection("list", []string{"--favorites"})
	if err != nil {
		t.Fatalf("parseSelection: %v", err)
	}
	if !cfg.favoritesOnly {
		t.Error("favoritesOnly = false, want true")
	}
}

func TestParseSelectionFilters(t *testing.T) {
	cfg, err := parseSelection("pick", []string{"--year", "1970-1980", "--genre", "jazz"})
	if err != nil {
		t.Fatalf("parseSelection: %v", err)
	}
	if cfg.filter.Year != "1970-1980" || cfg.filter.Genre != "jazz" {
		t.Errorf("filter = %+v", cfg.filter)
	}
}

func TestParseSelectionRejectsPositional(t *testing.T) {
	if _, err := parseSelection("pick", []string{"1975"}); err == nil {
		t.Fatal("expected error for unexpected positional argument")
	}
}

func TestParseSelectionRejectsBadYear(t *testing.T) {
	if _, err := parseSelection("pick", []string{"--year", "nineteen"}); err == nil {
		t.Fatal("expected error for invalid year")
	}
}

func TestParseFavoriteBareMeansLastPick(t *testing.T) {
	cfg, err := parseFavorite("favorite", nil)
	if err != nil {
		t.Fatalf("parseFavorite: %v", err)
	}
	if cfg.query != "" {
		t.Errorf("query = %q, want empty (last pick)", cfg.query)
	}
}

func TestParseFavoriteWithQuery(t *testing.T) {
	cfg, err := parseFavorite("favorite", []string{"kind of blue"})
	if err != nil {
		t.Fatalf("parseFavorite: %v", err)
	}
	if cfg.query != "kind of blue" {
		t.Errorf("query = %q, want 'kind of blue'", cfg.query)
	}
}

func TestParseFavoriteQueryWithTrailingFilter(t *testing.T) {
	cfg, err := parseFavorite("favorite", []string{"miles", "--year", "1959"})
	if err != nil {
		t.Fatalf("parseFavorite: %v", err)
	}
	if cfg.query != "miles" {
		t.Errorf("query = %q, want miles", cfg.query)
	}
	if cfg.filter.Year != "1959" {
		t.Errorf("filter.Year = %q, want 1959 (trailing filter was dropped)", cfg.filter.Year)
	}
}

func TestParseFavoriteEmptyQueryRejected(t *testing.T) {
	if _, err := parseFavorite("favorite", []string{""}); err == nil {
		t.Fatal("expected error for explicit empty query")
	}
}

func TestParseFavoriteTooManyArguments(t *testing.T) {
	if _, err := parseFavorite("favorite", []string{"kind", "of", "blue"}); err == nil {
		t.Fatal("expected error for unquoted multi-word query")
	}
}

func TestParseFavoriteFiltersRequireQuery(t *testing.T) {
	if _, err := parseFavorite("favorite", []string{"--year", "1959"}); err == nil {
		t.Fatal("expected error for filters with no query")
	}
}

func TestParseFavoriteRejectsFavoritesFlag(t *testing.T) {
	if _, err := parseFavorite("favorite", []string{"--favorites", "miles"}); err == nil {
		t.Fatal("expected error: --favorites is not registered on favorite")
	}
}

func TestParseHistoryDefault(t *testing.T) {
	cfg, err := parseHistory(nil)
	if err != nil {
		t.Fatalf("parseHistory: %v", err)
	}
	if cfg.limit != defaultHistoryLimit {
		t.Errorf("limit = %d, want %d", cfg.limit, defaultHistoryLimit)
	}
}

func TestParseHistoryExplicit(t *testing.T) {
	cfg, err := parseHistory([]string{"25"})
	if err != nil {
		t.Fatalf("parseHistory: %v", err)
	}
	if cfg.limit != 25 {
		t.Errorf("limit = %d, want 25", cfg.limit)
	}
}

func TestParseHistoryZeroMeansAll(t *testing.T) {
	cfg, err := parseHistory([]string{"0"})
	if err != nil {
		t.Fatalf("parseHistory: %v", err)
	}
	if cfg.limit != 0 {
		t.Errorf("limit = %d, want 0 (all)", cfg.limit)
	}
}

func TestParseHistoryNonNumeric(t *testing.T) {
	if _, err := parseHistory([]string{"abc"}); err == nil {
		t.Fatal("expected error for non-numeric count")
	}
}

func TestParseHistoryNegative(t *testing.T) {
	if _, err := parseHistory([]string{"-5"}); err == nil {
		t.Fatal("expected error for negative count")
	}
}

func TestParseSyncFolders(t *testing.T) {
	cfg, err := parseSync([]string{"--folder", "Vinyl 12\"", "--folder", "Vinyl 7\""})
	if err != nil {
		t.Fatalf("parseSync: %v", err)
	}
	if len(cfg.folders) != 2 || cfg.folders[0] != "Vinyl 12\"" {
		t.Errorf("folders = %v, want two entries", cfg.folders)
	}
}

func TestParseSyncRejectsListFolders(t *testing.T) {
	if _, err := parseSync([]string{"--list-folders"}); err == nil {
		t.Fatal("expected error: --list-folders is now the `folders` command")
	}
}

func TestParseNoArgsRejectsArgument(t *testing.T) {
	if err := parseNoArgs("folders", []string{"extra"}); err == nil {
		t.Fatal("expected error for unexpected argument")
	}
}

func TestResolveEmptyArgsIsPick(t *testing.T) {
	cmd, rest, err := resolve(nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if cmd.name != "pick" {
		t.Errorf("command = %q, want pick", cmd.name)
	}
	if len(rest) != 0 {
		t.Errorf("rest = %v, want empty", rest)
	}
}

func TestResolveLeadingFlagIsPick(t *testing.T) {
	cmd, rest, err := resolve([]string{"--year", "1975"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if cmd.name != "pick" {
		t.Errorf("command = %q, want pick", cmd.name)
	}
	if len(rest) != 2 || rest[0] != "--year" {
		t.Errorf("rest = %v, want [--year 1975]", rest)
	}
}

func TestResolveNamedCommand(t *testing.T) {
	cmd, rest, err := resolve([]string{"list", "--favorites"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if cmd.name != "list" {
		t.Errorf("command = %q, want list", cmd.name)
	}
	if len(rest) != 1 || rest[0] != "--favorites" {
		t.Errorf("rest = %v, want [--favorites]", rest)
	}
}

func TestResolveHelpFlags(t *testing.T) {
	for _, arg := range []string{"-h", "--help", "-help"} {
		cmd, _, err := resolve([]string{arg})
		if err != nil {
			t.Fatalf("resolve(%q): %v", arg, err)
		}
		if cmd.name != "help" {
			t.Errorf("resolve(%q) = %q, want help", arg, cmd.name)
		}
	}
}

func TestResolveVersionFlagIsSignpost(t *testing.T) {
	for _, arg := range []string{"-v", "--version", "-version"} {
		_, _, err := resolve([]string{arg})
		if err == nil {
			t.Fatalf("resolve(%q): expected an error pointing at `disc-fortune version`", arg)
		}
		if !strings.Contains(err.Error(), "disc-fortune version") {
			t.Errorf("resolve(%q) error = %q, want it to name `disc-fortune version`", arg, err)
		}
	}
}

func TestResolveUnknownCommand(t *testing.T) {
	_, _, err := resolve([]string{"frobnicate"})
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
	if !strings.Contains(err.Error(), "frobnicate") {
		t.Errorf("error = %q, want it to name the unknown command", err)
	}
}

func TestEveryCommandIsDocumented(t *testing.T) {
	if len(commands) == 0 {
		t.Fatal("commands table is empty")
	}
	for _, c := range commands {
		if c.name == "" {
			t.Error("a command has an empty name")
		}
		if c.summary == "" {
			t.Errorf("command %q has no summary", c.name)
		}
		if c.usage == "" {
			t.Errorf("command %q has no usage text", c.name)
		}
		if c.run == nil {
			t.Errorf("command %q has no run function", c.name)
		}
	}
}

func TestHelpListsEveryCommand(t *testing.T) {
	out, err := helpText("")
	if err != nil {
		t.Fatalf("helpText: %v", err)
	}
	for _, c := range commands {
		if !strings.Contains(out, c.name) {
			t.Errorf("help output missing command %q", c.name)
		}
	}
}

func TestHelpForOneCommand(t *testing.T) {
	out, err := helpText("sync")
	if err != nil {
		t.Fatalf("helpText: %v", err)
	}
	if !strings.Contains(out, "--folder") {
		t.Errorf("help sync output missing --folder: %q", out)
	}
}

func TestHelpUnknownTopic(t *testing.T) {
	if _, err := helpText("frobnicate"); err == nil {
		t.Fatal("expected error for unknown help topic")
	}
}

// The flag package treats -h/--help as flag.ErrHelp rather than an ordinary
// parse failure. These confirm each parser's %w wrapping preserves that so
// errors.Is(err, flag.ErrHelp) still works through the "<command>: " prefix,
// which is what lets handleParseErr tell a help request apart from a usage
// error.

func TestParseSelectionHelpFlagIsErrHelp(t *testing.T) {
	if _, err := parseSelection("pick", []string{"--help"}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("parseSelection(--help) error = %v, want errors.Is(_, flag.ErrHelp)", err)
	}
}

func TestParseFavoriteHelpFlagIsErrHelp(t *testing.T) {
	if _, err := parseFavorite("favorite", []string{"--help"}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("parseFavorite(--help) error = %v, want errors.Is(_, flag.ErrHelp)", err)
	}
}

func TestParseHistoryHelpFlagIsErrHelp(t *testing.T) {
	if _, err := parseHistory([]string{"--help"}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("parseHistory(--help) error = %v, want errors.Is(_, flag.ErrHelp)", err)
	}
}

func TestParseSyncHelpFlagIsErrHelp(t *testing.T) {
	if _, err := parseSync([]string{"--help"}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("parseSync(--help) error = %v, want errors.Is(_, flag.ErrHelp)", err)
	}
}

func TestParseNoArgsHelpFlagIsErrHelp(t *testing.T) {
	if err := parseNoArgs("folders", []string{"--help"}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("parseNoArgs(--help) error = %v, want errors.Is(_, flag.ErrHelp)", err)
	}
}

// captureStdout runs fn with os.Stdout redirected and returns what it wrote.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = orig

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("io.Copy: %v", err)
	}
	return buf.String()
}

func TestHandleParseErrPrintsUsageOnHelpAndDoesNotExit(t *testing.T) {
	var handled bool
	out := captureStdout(t, func() {
		handled = handleParseErr("sync", flag.ErrHelp)
	})
	if !handled {
		t.Error("handleParseErr(flag.ErrHelp) = false, want true")
	}
	if !strings.Contains(out, "--folder") {
		t.Errorf("output missing sync usage text: %q", out)
	}
}

func TestHandleParseErrWrappedHelpFlag(t *testing.T) {
	// Reproduces disc-fortune sync --help: parseSync wraps flag.ErrHelp with
	// "sync: %w", and handleParseErr must still recognize it via errors.Is
	// rather than a direct == comparison, and must not fall through to fatal.
	_, err := parseSync([]string{"--help"})
	var handled bool
	out := captureStdout(t, func() {
		handled = handleParseErr("sync", err)
	})
	if !handled {
		t.Error("handleParseErr(wrapped flag.ErrHelp) = false, want true")
	}
	if !strings.Contains(out, "Usage: disc-fortune sync") {
		t.Errorf("output missing sync usage text: %q", out)
	}
}

func TestHandleParseErrNilIsNotHandled(t *testing.T) {
	if handleParseErr("pick", nil) {
		t.Error("handleParseErr(nil) = true, want false")
	}
}
