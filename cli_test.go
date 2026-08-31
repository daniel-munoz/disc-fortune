package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
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
	if filter.Year != "1970-1980" || filter.Genre != "jazz" {
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
	if cfg.filter.ReleaseID != 1839278 || cfg.filter.Year != "1993" {
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
