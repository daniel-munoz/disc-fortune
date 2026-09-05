package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestParseInterspersedFlagsAfterPositional(t *testing.T) {
	fs, _ := newFlagSet("favorite")
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
	fs, _ := newFlagSet("favorite")
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
	fs, _ := newFlagSet("favorite")
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
	fs, _ := newFlagSet("favorite")
	rest, err := parseInterspersed(fs, []string{"kind", "of", "blue"})
	if err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if len(rest) != 3 {
		t.Errorf("positional = %v, want 3 items", rest)
	}
}

func TestParseInterspersedDashTerminator(t *testing.T) {
	fs, _ := newFlagSet("favorite")
	rest, err := parseInterspersed(fs, []string{"--", "-live-"})
	if err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if len(rest) != 1 || rest[0] != "-live-" {
		t.Errorf("positional = %v, want [-live-]", rest)
	}
}

func TestParseInterspersedUnknownFlag(t *testing.T) {
	fs, _ := newFlagSet("pick")
	if _, err := parseInterspersed(fs, []string{"--nope"}); err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestFilterFlagsBuildsFilter(t *testing.T) {
	fs, _ := newFlagSet("pick")
	ff := addFilterFlags(fs)
	if _, err := parseInterspersed(fs, []string{"--year", "1970-1980", "--genre", "jazz"}); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	filter, err := ff.Filter()
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if !reflect.DeepEqual(filter.Year, years(t, "1970-1980")) || !reflect.DeepEqual(filter.Genre, include("jazz")) {
		t.Errorf("filter = %+v, want Year=1970-1980 Genre=jazz", filter)
	}
	if !ff.anyNarrowing() {
		t.Error("anyNarrowing() = false, want true")
	}
}

func TestFilterFlagsRejectsBadYear(t *testing.T) {
	fs, _ := newFlagSet("pick")
	ff := addFilterFlags(fs)
	if _, err := parseInterspersed(fs, []string{"--year", "nineteen"}); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if _, err := ff.Filter(); err == nil {
		t.Fatal("expected error for non-numeric year")
	}
}

func TestFilterFlagsNoneSetWhenUnset(t *testing.T) {
	fs, _ := newFlagSet("pick")
	ff := addFilterFlags(fs)
	if _, err := parseInterspersed(fs, nil); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if ff.anyNarrowing() {
		t.Error("anyNarrowing() = true, want false when no filter flags set")
	}
	if ff.identifies() {
		t.Error("identifies() = true, want false when no filter flags set")
	}
}

// TestFilterFlagsReleaseIDIsNotNarrowing pins the distinction the query rule
// depends on: --release-id identifies, it does not narrow.
func TestFilterFlagsReleaseIDIsNotNarrowing(t *testing.T) {
	fs, _ := newFlagSet("pick")
	ff := addFilterFlags(fs)
	if _, err := parseInterspersed(fs, []string{"--release-id", "1839278"}); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if ff.anyNarrowing() {
		t.Error("anyNarrowing() = true, want false for --release-id alone")
	}
	if !ff.identifies() {
		t.Error("identifies() = false, want true for --release-id")
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
	if !reflect.DeepEqual(cfg.filter.Year, years(t, "1970-1980")) || !reflect.DeepEqual(cfg.filter.Genre, include("jazz")) {
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
	if !reflect.DeepEqual(cfg.filter.Year, years(t, "1959")) {
		t.Errorf("filter.Year = %+v, want 1959 (trailing filter was dropped)", cfg.filter.Year)
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
	// "-5" alone never reaches the n < 0 branch: the flag package tries to
	// parse it as a flag first and fails with "flag provided but not
	// defined: -5". Document that behavior explicitly rather than letting an
	// assertion on err != nil accidentally pass for the wrong reason.
	if _, err := parseHistory([]string{"-5"}); err == nil ||
		strings.Contains(err.Error(), "cannot be negative") {
		t.Fatalf("parseHistory(-5) = %v, want a flag-parsing error, not the negative-count branch", err)
	}

	// "--" forces everything after it to be treated as positional, which is
	// what actually reaches the n < 0 branch at cli.go's parseHistory.
	if _, err := parseHistory([]string{"--", "-5"}); err == nil ||
		!strings.Contains(err.Error(), "cannot be negative") {
		t.Fatalf("parseHistory(-- -5) = %v, want a negative-count error", err)
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

func TestResolveV1FlagSignposts(t *testing.T) {
	tests := []struct {
		arg    string
		target string
	}{
		{"--sync", "disc-fortune sync"},
		{"--list", "disc-fortune list"},
		{"--list-folders", "disc-fortune folders"},
		{"--history", "disc-fortune history"},
		{"--favorite-last", "disc-fortune favorite"},
		{"--unfavorite-last", "disc-fortune unfavorite"},
		{"--favorite", "disc-fortune favorite QUERY"},
	}
	for _, tc := range tests {
		_, _, err := resolve([]string{tc.arg})
		if err == nil {
			t.Fatalf("resolve(%q): expected a signpost error pointing at `%s`", tc.arg, tc.target)
		}
		want := fmt.Sprintf("%s is now `%s` (see RELEASE_NOTES_v2.0.0.md)", tc.arg, tc.target)
		if err.Error() != want {
			t.Errorf("resolve(%q) error = %q, want %q", tc.arg, err.Error(), want)
		}
	}
}

// TestResolveFilterFlagsStillReachPick guards the critical detail in the v1
// signpost map: filter flags that still work verbatim under implicit pick
// must NOT be signposted, or working v2 invocations would break.
func TestResolveFilterFlagsStillReachPick(t *testing.T) {
	for _, args := range [][]string{
		{"--favorites"},
		{"--year", "1975"},
		{"--genre", "jazz"},
		{"--label", "blue-note"},
		{"--format", "12\""},
	} {
		cmd, _, err := resolve(args)
		if err != nil {
			t.Fatalf("resolve(%v): unexpected error: %v", args, err)
		}
		if cmd.name != "pick" {
			t.Errorf("resolve(%v) = %q, want pick", args, cmd.name)
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

func TestParseHelpHelpFlagIsErrHelp(t *testing.T) {
	if _, err := parseHelp([]string{"--help"}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("parseHelp(--help) error = %v, want errors.Is(_, flag.ErrHelp)", err)
	}
	if _, err := parseHelp([]string{"-h"}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("parseHelp(-h) error = %v, want errors.Is(_, flag.ErrHelp)", err)
	}
}

func TestParseHelpTopic(t *testing.T) {
	topic, err := parseHelp([]string{"sync"})
	if err != nil {
		t.Fatalf("parseHelp(sync): %v", err)
	}
	if topic != "sync" {
		t.Errorf("topic = %q, want sync", topic)
	}
}

func TestParseHelpTooManyArguments(t *testing.T) {
	if _, err := parseHelp([]string{"sync", "list"}); err == nil {
		t.Fatal("expected error for too many arguments")
	}
}

// TestHelpHelpFlagExitsZero reproduces `disc-fortune help --help`, which
// previously fell through help's hand-rolled arg handling straight to
// `help: unknown command "--help"` and exit 1. It must instead be treated
// like -h/--help on any other command: print usage and hand back control
// without exiting.
func TestHelpHelpFlagExitsZero(t *testing.T) {
	for _, arg := range []string{"-h", "--help", "-help"} {
		topic, err := parseHelp([]string{arg})
		var handled bool
		out := captureStdout(t, func() {
			handled = handleParseErr("help", err)
		})
		if !handled {
			t.Errorf("handleParseErr(help, parseHelp(%q)) = false, want true (handled, exit 0)", arg)
		}
		if topic != "" {
			t.Errorf("parseHelp(%q) topic = %q, want empty", arg, topic)
		}
		if !strings.Contains(out, "Usage: disc-fortune help") {
			t.Errorf("output missing help usage text: %q", out)
		}
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

// TestFavoriteAcceptsReleaseIDWithoutQuery: --release-id identifies one exact
// record, so demanding a redundant query alongside it would be pointless.
func TestFavoriteAcceptsReleaseIDWithoutQuery(t *testing.T) {
	cfg, err := parseFavorite("favorite", []string{"--release-id", "1839278"})
	if err != nil {
		t.Fatalf("parseFavorite: %v", err)
	}
	if cfg.filter.ReleaseID != 1839278 {
		t.Errorf("filter.ReleaseID = %d, want 1839278", cfg.filter.ReleaseID)
	}
	if cfg.query != "" {
		t.Errorf("query = %q, want empty", cfg.query)
	}
}

func TestUnfavoriteAcceptsReleaseIDWithoutQuery(t *testing.T) {
	cfg, err := parseFavorite("unfavorite", []string{"--release-id", "1839278"})
	if err != nil {
		t.Fatalf("parseFavorite: %v", err)
	}
	if cfg.filter.ReleaseID != 1839278 {
		t.Errorf("filter.ReleaseID = %d, want 1839278", cfg.filter.ReleaseID)
	}
}

// TestFavoriteStillRequiresQueryForNarrowingFilters: the narrowing filters
// only refine a query, so on their own they still cannot say which record is
// meant. This behavior is unchanged from v2.2.0.
func TestFavoriteStillRequiresQueryForNarrowingFilters(t *testing.T) {
	for _, args := range [][]string{
		{"--year", "1959"},
		{"--genre", "jazz"},
		{"--label", "columbia"},
		{"--format", "blue"},
	} {
		_, err := parseFavorite("favorite", args)
		if err == nil {
			t.Errorf("parseFavorite(%v): expected an error, got none", args)
			continue
		}
		if !strings.Contains(err.Error(), "filters require a query") {
			t.Errorf("parseFavorite(%v) error = %q, want it to mention a query", args, err)
		}
	}
}

// A release ID alongside narrowing filters is still fine: the ID wins and the
// rest simply agree or exclude it.
func TestFavoriteAcceptsReleaseIDWithNarrowingFilters(t *testing.T) {
	cfg, err := parseFavorite("favorite", []string{"--release-id", "1839278", "--year", "1993"})
	if err != nil {
		t.Fatalf("parseFavorite: %v", err)
	}
	if cfg.filter.ReleaseID != 1839278 || !reflect.DeepEqual(cfg.filter.Year, years(t, "1993")) {
		t.Errorf("filter = %+v, want both the ID and the year", cfg.filter)
	}
}

func TestSelectionAcceptsReleaseID(t *testing.T) {
	for _, name := range []string{"pick", "list"} {
		cfg, err := parseSelection(name, []string{"--release-id", "1839278"})
		if err != nil {
			t.Fatalf("parseSelection(%s): %v", name, err)
		}
		if cfg.filter.ReleaseID != 1839278 {
			t.Errorf("%s: filter.ReleaseID = %d, want 1839278", name, cfg.filter.ReleaseID)
		}
	}
}

func TestParseSelectionUnheardFlag(t *testing.T) {
	for _, name := range []string{"pick", "list"} {
		cfg, err := parseSelection(name, []string{"--unheard"})
		if err != nil {
			t.Fatalf("parseSelection(%q): %v", name, err)
		}
		if !cfg.unheard {
			t.Errorf("%s: unheard = false, want true", name)
		}
	}
}

func TestParseSelectionDrawDefaultsToFresh(t *testing.T) {
	cfg, err := parseSelection("pick", nil)
	if err != nil {
		t.Fatalf("parseSelection: %v", err)
	}
	if cfg.draw != drawFresh {
		t.Errorf("draw = %v, want drawFresh", cfg.draw)
	}
}

func TestParseSelectionDrawFlag(t *testing.T) {
	cfg, err := parseSelection("pick", []string{"--draw", "stale"})
	if err != nil {
		t.Fatalf("parseSelection: %v", err)
	}
	if cfg.draw != drawStale {
		t.Errorf("draw = %v, want drawStale", cfg.draw)
	}
}

func TestParseSelectionRejectsBadDraw(t *testing.T) {
	if _, err := parseSelection("pick", []string{"--draw", "weighted"}); err == nil {
		t.Fatal("expected an error for an unknown --draw value")
	}
}

// Nothing is drawn by `list`, so the flag must not be quietly accepted there.
func TestParseSelectionRejectsDrawOnList(t *testing.T) {
	if _, err := parseSelection("list", []string{"--draw", "stale"}); err == nil {
		t.Fatal("expected list to reject --draw")
	}
}

// --unheard reads history, which favorite and unfavorite have no business
// loading. They take their flags from addFilterFlags, so this must stay true.
func TestParseFavoriteRejectsUnheardFlag(t *testing.T) {
	if _, err := parseFavorite("favorite", []string{"kind of blue", "--unheard"}); err == nil {
		t.Fatal("expected favorite to reject --unheard")
	}
}

func TestFilterFlagsRepeatAndOR(t *testing.T) {
	fs, _ := newFlagSet("pick")
	ff := addFilterFlags(fs)
	args := []string{"--genre", "jazz", "--genre", "funk"}
	if _, err := parseInterspersed(fs, args); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	filter, err := ff.Filter()
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if len(filter.Genre.Include) != 2 ||
		filter.Genre.Include[0] != "jazz" || filter.Genre.Include[1] != "funk" {
		t.Errorf("Genre.Include = %q, want [jazz funk]", filter.Genre.Include)
	}
}

func TestFilterFlagsExcludeTwins(t *testing.T) {
	fs, _ := newFlagSet("pick")
	ff := addFilterFlags(fs)
	args := []string{
		"--exclude-query", "bootleg",
		"--exclude-artist", "davis",
		"--exclude-title", "live",
		"--exclude-genre", "rock",
		"--exclude-label", "columbia",
		"--exclude-format", "cd",
	}
	if _, err := parseInterspersed(fs, args); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	filter, err := ff.Filter()
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	// Every value below is distinct, so a cross-wired index -- e.g.
	// --exclude-title landing in filter.Label.Exclude -- fails here even
	// though each field would still hold exactly one value.
	for name, got := range map[string][]string{
		"query":  filter.Query.Exclude,
		"artist": filter.Artist.Exclude,
		"title":  filter.Title.Exclude,
		"genre":  filter.Genre.Exclude,
		"label":  filter.Label.Exclude,
		"format": filter.Format.Exclude,
	} {
		want := map[string]string{
			"query":  "bootleg",
			"artist": "davis",
			"title":  "live",
			"genre":  "rock",
			"label":  "columbia",
			"format": "cd",
		}[name]
		if len(got) != 1 || got[0] != want {
			t.Errorf("%s: Exclude = %q, want [%s]", name, got, want)
		}
	}
}

// TestAllFilterTakingCommandsRegisterFilterFlags is the "Registration" test
// the design spec's Tests section calls for explicitly: every table-driven
// field and every field outside the table reaches pick, list, favorite and
// unfavorite alike. It is structurally guaranteed today, since all four
// route through addFilterFlags, but nothing exercised that guarantee through
// each command's actual parser until now.
func TestAllFilterTakingCommandsRegisterFilterFlags(t *testing.T) {
	args := []string{
		"--genre", "jazz", "--exclude-label", "x",
		"--year", "1975", "--decade", "70s", "--release-id", "42",
	}
	check := func(t *testing.T, filter Filter) {
		t.Helper()
		if len(filter.Genre.Include) != 1 || filter.Genre.Include[0] != "jazz" {
			t.Errorf("Genre.Include = %q, want [jazz]", filter.Genre.Include)
		}
		if len(filter.Label.Exclude) != 1 || filter.Label.Exclude[0] != "x" {
			t.Errorf("Label.Exclude = %q, want [x]", filter.Label.Exclude)
		}
		if len(filter.Year.Include) != 2 {
			t.Errorf("Year.Include = %v, want two ranges (1975 and the 70s)", filter.Year.Include)
		}
		if filter.ReleaseID != 42 {
			t.Errorf("ReleaseID = %d, want 42", filter.ReleaseID)
		}
	}

	for _, name := range []string{"pick", "list"} {
		t.Run(name, func(t *testing.T) {
			cfg, err := parseSelection(name, args)
			if err != nil {
				t.Fatalf("parseSelection(%s, %v): %v", name, args, err)
			}
			check(t, cfg.filter)
		})
	}
	for _, name := range []string{"favorite", "unfavorite"} {
		t.Run(name, func(t *testing.T) {
			cfg, err := parseFavorite(name, args)
			if err != nil {
				t.Fatalf("parseFavorite(%s, %v): %v", name, args, err)
			}
			check(t, cfg.filter)
		})
	}
}

func TestFilterFlagsNewNarrowingFields(t *testing.T) {
	fs, _ := newFlagSet("pick")
	ff := addFilterFlags(fs)
	args := []string{"--query", "kind of", "--artist", "miles", "--title", "blue"}
	if _, err := parseInterspersed(fs, args); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	filter, err := ff.Filter()
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if len(filter.Query.Include) != 1 || filter.Query.Include[0] != "kind of" {
		t.Errorf("Query.Include = %q, want [kind of]", filter.Query.Include)
	}
	if len(filter.Artist.Include) != 1 || filter.Artist.Include[0] != "miles" {
		t.Errorf("Artist.Include = %q, want [miles]", filter.Artist.Include)
	}
	if len(filter.Title.Include) != 1 || filter.Title.Include[0] != "blue" {
		t.Errorf("Title.Include = %q, want [blue]", filter.Title.Include)
	}
}

// --year and --decade are two spellings of one field, so they OR rather than
// AND. The naive reading -- two separate fields intersected -- would make
// this combination return nothing at all.
func TestYearAndDecadeFeedOneConstraint(t *testing.T) {
	fs, _ := newFlagSet("pick")
	ff := addFilterFlags(fs)
	if _, err := parseInterspersed(fs, []string{"--year", "1959", "--decade", "70s"}); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	filter, err := ff.Filter()
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if len(filter.Year.Include) != 2 {
		t.Fatalf("Year.Include = %v, want two ranges", filter.Year.Include)
	}
	albums := []Album{
		{Artist: "A", Year: 1959},
		{Artist: "B", Year: 1975},
		{Artist: "C", Year: 1985},
	}
	if got := filter.Apply(albums); len(got) != 2 {
		t.Errorf("matched %d albums, want 2 (1959 and 1975)", len(got))
	}
}

func TestExcludeYearAndDecadeFeedOneExclusion(t *testing.T) {
	fs, _ := newFlagSet("pick")
	ff := addFilterFlags(fs)
	args := []string{"--exclude-year", "1959", "--exclude-decade", "70s"}
	if _, err := parseInterspersed(fs, args); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	filter, err := ff.Filter()
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if len(filter.Year.Exclude) != 2 {
		t.Errorf("Year.Exclude = %v, want two ranges", filter.Year.Exclude)
	}
	if len(filter.Year.Include) != 0 {
		t.Errorf("Year.Include = %v, want none", filter.Year.Include)
	}
}

func TestFilterFlagsRejectsAmbiguousDecade(t *testing.T) {
	fs, _ := newFlagSet("pick")
	ff := addFilterFlags(fs)
	if _, err := parseInterspersed(fs, []string{"--decade", "20s"}); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	_, err := ff.Filter()
	if err == nil {
		t.Fatal("expected an error for --decade 20s")
	}
	if !strings.Contains(err.Error(), "1920s") || !strings.Contains(err.Error(), "2020s") {
		t.Errorf("error = %q, want it to name both spellings", err)
	}
}

// `--genre "$GENRE"` with an unset variable has always meant "no genre
// filter", and must keep meaning that. It also matters for exclusions: every
// string contains "", so an empty exclusion reaching the matcher would
// exclude the whole collection.
func TestEmptyFilterValuesAreDropped(t *testing.T) {
	fs, _ := newFlagSet("pick")
	ff := addFilterFlags(fs)
	args := []string{"--genre", "", "--exclude-genre", "", "--year", "", "--decade", ""}
	if _, err := parseInterspersed(fs, args); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	filter, err := ff.Filter()
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if filter.any() {
		t.Errorf("filter = %+v, want nothing set", filter)
	}

	albums := []Album{{Artist: "A", Genres: []string{"Jazz"}}, {Artist: "B"}}
	if got := filter.Apply(albums); len(got) != 2 {
		t.Errorf("matched %d albums, want all 2", len(got))
	}
}

func TestHasQueryIgnoresExclusions(t *testing.T) {
	fs, _ := newFlagSet("favorite")
	ff := addFilterFlags(fs)
	if _, err := parseInterspersed(fs, []string{"--exclude-query", "bootleg"}); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if ff.hasQuery() {
		t.Error("hasQuery() = true for --exclude-query; an exclusion says which record is NOT meant")
	}
	if !ff.anyNarrowing() {
		t.Error("anyNarrowing() = false for --exclude-query, want true")
	}
}

func TestHasQueryAndNarrowing(t *testing.T) {
	fs, _ := newFlagSet("favorite")
	ff := addFilterFlags(fs)
	if _, err := parseInterspersed(fs, []string{"--query", "miles"}); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if !ff.hasQuery() {
		t.Error("hasQuery() = false for --query, want true")
	}
	if ff.anyNarrowing() {
		t.Error("anyNarrowing() = true for --query alone; a query is not a narrowing filter")
	}
}

// pick and list have never had a free-text search: they reject positional
// arguments, and --query did not exist. This is the acceptance criterion for
// that gap.
func TestParseSelectionAcceptsQueryAndTheNewFilters(t *testing.T) {
	for _, name := range []string{"pick", "list"} {
		args := []string{
			"--query", "miles",
			"--artist", "davis",
			"--title", "blue",
			"--decade", "70s",
			"--exclude-genre", "rock",
		}
		cfg, err := parseSelection(name, args)
		if err != nil {
			t.Fatalf("parseSelection(%s): %v", name, err)
		}
		if len(cfg.filter.Query.Include) != 1 || cfg.filter.Query.Include[0] != "miles" {
			t.Errorf("%s: Query.Include = %q, want [miles]", name, cfg.filter.Query.Include)
		}
		if len(cfg.filter.Artist.Include) != 1 || len(cfg.filter.Title.Include) != 1 {
			t.Errorf("%s: artist/title not parsed: %+v", name, cfg.filter)
		}
		if len(cfg.filter.Year.Include) != 1 || cfg.filter.Year.Include[0] != (yearRange{1970, 1979}) {
			t.Errorf("%s: Year.Include = %v, want [{1970 1979}]", name, cfg.filter.Year.Include)
		}
		if len(cfg.filter.Genre.Exclude) != 1 || cfg.filter.Genre.Exclude[0] != "rock" {
			t.Errorf("%s: Genre.Exclude = %q, want [rock]", name, cfg.filter.Genre.Exclude)
		}
	}
}

func TestEveryNarrowingFlagCountsAsNarrowing(t *testing.T) {
	for _, args := range [][]string{
		{"--artist", "miles"},
		{"--title", "blue"},
		{"--genre", "jazz"},
		{"--label", "columbia"},
		{"--format", "vinyl"},
		{"--year", "1959"},
		{"--decade", "70s"},
		{"--exclude-artist", "miles"},
		{"--exclude-genre", "rock"},
		{"--exclude-year", "1959"},
		{"--exclude-decade", "70s"},
	} {
		fs, _ := newFlagSet("favorite")
		ff := addFilterFlags(fs)
		if _, err := parseInterspersed(fs, args); err != nil {
			t.Fatalf("parseInterspersed(%v): %v", args, err)
		}
		if !ff.anyNarrowing() {
			t.Errorf("anyNarrowing() = false for %v, want true", args)
		}
	}
}

// anyNarrowing must agree with Filter()/hasQuery() about what an empty value
// means: `--genre "$GENRE"` with an unset variable is "no genre filter", not
// a narrowing filter that then demands a query. Filter() already routed its
// slices through nonEmpty; this pins that anyNarrowing does too.
func TestAnyNarrowingIgnoresEmptyValues(t *testing.T) {
	fs, _ := newFlagSet("favorite")
	ff := addFilterFlags(fs)
	args := []string{
		"--exclude-genre", "",
		"--year", "",
		"--decade", "",
		"--exclude-year", "",
		"--exclude-decade", "",
		"--artist", "",
		"--exclude-artist", "",
	}
	if _, err := parseInterspersed(fs, args); err != nil {
		t.Fatalf("parseInterspersed: %v", err)
	}
	if ff.anyNarrowing() {
		t.Error("anyNarrowing() = true for all-empty filter values, want false")
	}
}

// The live regression this pins: `favorite --genre ""` used to report
// "filters require a query" instead of falling through to favorite the last
// pick, because anyNarrowing() counted the empty --genre value as narrowing.
func TestParseFavoriteEmptyFilterValueDoesNotRequireQuery(t *testing.T) {
	cfg, err := parseFavorite("favorite", []string{"--genre", ""})
	if err != nil {
		t.Fatalf("parseFavorite: %v", err)
	}
	if cfg.query != "" {
		t.Errorf("query = %q, want empty (last pick)", cfg.query)
	}
}

// The positional QUERY and --query are one thing said two ways, so the filter
// carries the value either way.
func TestFavoritePositionalQueryLandsInTheFilter(t *testing.T) {
	cfg, err := parseFavorite("favorite", []string{"miles"})
	if err != nil {
		t.Fatalf("parseFavorite: %v", err)
	}
	if len(cfg.filter.Query.Include) != 1 || cfg.filter.Query.Include[0] != "miles" {
		t.Errorf("filter.Query.Include = %q, want [miles]", cfg.filter.Query.Include)
	}
	if cfg.query != "miles" {
		t.Errorf("query = %q, want %q", cfg.query, "miles")
	}
}

func TestFavoriteQueryFlagIsEquivalentToPositional(t *testing.T) {
	positional, err := parseFavorite("favorite", []string{"miles"})
	if err != nil {
		t.Fatalf("parseFavorite positional: %v", err)
	}
	flagged, err := parseFavorite("favorite", []string{"--query", "miles"})
	if err != nil {
		t.Fatalf("parseFavorite --query: %v", err)
	}
	if flagged.query != positional.query {
		t.Errorf("query = %q, want %q", flagged.query, positional.query)
	}
	if len(flagged.filter.Query.Include) != 1 || flagged.filter.Query.Include[0] != "miles" {
		t.Errorf("filter.Query.Include = %q, want [miles]", flagged.filter.Query.Include)
	}
}

// Both spellings at once is refused rather than OR-ed. The rule in the design
// would make it an OR, but on a command that mutates favorites a surprise is
// worse than a refusal.
func TestFavoriteRejectsPositionalAndQueryFlagTogether(t *testing.T) {
	for _, name := range []string{"favorite", "unfavorite"} {
		_, err := parseFavorite(name, []string{"miles", "--query", "coltrane"})
		if err == nil {
			t.Errorf("%s: expected an error when both spellings are given", name)
			continue
		}
		if !strings.Contains(err.Error(), "give the query once") {
			t.Errorf("%s error = %q, want it to say the query is given once", name, err)
		}
	}
}

func TestFavoriteQueryFlagSatisfiesTheQueryRequirement(t *testing.T) {
	cfg, err := parseFavorite("favorite", []string{"--query", "miles", "--year", "1959"})
	if err != nil {
		t.Fatalf("parseFavorite: %v", err)
	}
	if cfg.query != "miles" {
		t.Errorf("query = %q, want miles", cfg.query)
	}
	if len(cfg.filter.Year.Include) != 1 {
		t.Errorf("Year.Include = %v, want one range", cfg.filter.Year.Include)
	}
}

// An exclusion says which record is NOT meant, so it cannot stand in for a
// query on a command that has to pick exactly one record.
func TestFavoriteExcludeQueryStillRequiresAQuery(t *testing.T) {
	_, err := parseFavorite("favorite", []string{"--exclude-query", "bootleg"})
	if err == nil {
		t.Fatal("expected an error for --exclude-query alone")
	}
	if !strings.Contains(err.Error(), "filters require a query") {
		t.Errorf("error = %q, want it to mention a query", err)
	}
}

func TestFavoriteSeveralQueryValuesDescribeThemselves(t *testing.T) {
	cfg, err := parseFavorite("favorite", []string{"--query", "miles", "--query", "coltrane"})
	if err != nil {
		t.Fatalf("parseFavorite: %v", err)
	}
	if cfg.query != "miles or coltrane" {
		t.Errorf("query = %q, want %q", cfg.query, "miles or coltrane")
	}
	if len(cfg.filter.Query.Include) != 2 {
		t.Errorf("filter.Query.Include = %q, want two values", cfg.filter.Query.Include)
	}
}

func TestParseSelectionJSONFlag(t *testing.T) {
	for _, name := range []string{"pick", "list"} {
		cfg, err := parseSelection(name, []string{"--json"})
		if err != nil {
			t.Fatalf("parseSelection(%s): %v", name, err)
		}
		if !cfg.json {
			t.Errorf("%s: cfg.json = false, want true", name)
		}
	}
}

func TestParseSelectionJSONDefaultsOff(t *testing.T) {
	for _, name := range []string{"pick", "list"} {
		cfg, err := parseSelection(name, nil)
		if err != nil {
			t.Fatalf("parseSelection(%s): %v", name, err)
		}
		if cfg.json {
			t.Errorf("%s: cfg.json = true, want false by default", name)
		}
	}
}

func TestParseHistoryJSONFlag(t *testing.T) {
	cfg, err := parseHistory([]string{"--json", "5"})
	if err != nil {
		t.Fatalf("parseHistory: %v", err)
	}
	if !cfg.json {
		t.Error("cfg.json = false, want true")
	}
	if cfg.limit != 5 {
		t.Errorf("limit = %d, want 5 (the positional must still work)", cfg.limit)
	}
}

// --json is registered where it is implemented, exactly as --draw is, so a
// command that cannot honour it says so rather than accepting and ignoring it.
func TestJSONFlagRejectedWhereNotImplemented(t *testing.T) {
	if _, err := parseSync([]string{"--json"}); err == nil {
		t.Error("sync accepted --json, want an unknown-flag error")
	}
	if _, err := parseFavorite("favorite", []string{"miles", "--json"}); err == nil {
		t.Error("favorite accepted --json, want an unknown-flag error")
	}
	if _, err := parseFavorite("unfavorite", []string{"miles", "--json"}); err == nil {
		t.Error("unfavorite accepted --json, want an unknown-flag error")
	}
	for _, name := range []string{"folders", "migrate", "version"} {
		if err := parseNoArgs(name, []string{"--json"}); err == nil {
			t.Errorf("%s accepted --json, want an unknown-flag error", name)
		}
	}
}

// stats is set-oriented like list and pick, so a filter stands on its own.
// The "filters require a query" rule belongs to favorite, unfavorite and
// open, which each act on exactly one record.
func TestParseStatsAcceptsFiltersWithoutAQuery(t *testing.T) {
	cfg, err := parseStats([]string{"--genre", "jazz"})
	if err != nil {
		t.Fatalf("parseStats: %v", err)
	}
	if len(cfg.filter.Genre.Include) != 1 || cfg.filter.Genre.Include[0] != "jazz" {
		t.Errorf("genre filter = %+v, want [jazz]", cfg.filter.Genre.Include)
	}
}

func TestParseStatsRejectsUnheardAndDraw(t *testing.T) {
	for _, arg := range []string{"--unheard", "--draw"} {
		if _, err := parseStats([]string{arg}); err == nil {
			t.Errorf("parseStats accepted %s; stats filters no history and draws nothing", arg)
		}
	}
}

func TestParseStatsRejectsPositionalArguments(t *testing.T) {
	if _, err := parseStats([]string{"jazz"}); err == nil {
		t.Error("parseStats accepted a positional argument")
	}
}

func TestParseStatsFlags(t *testing.T) {
	cfg, err := parseStats([]string{"--favorites", "--json"})
	if err != nil {
		t.Fatalf("parseStats: %v", err)
	}
	if !cfg.favoritesOnly || !cfg.json {
		t.Errorf("cfg = %+v, want favoritesOnly and json set", cfg)
	}
}
