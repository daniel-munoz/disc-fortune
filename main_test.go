package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunListOutput(t *testing.T) {
	albums := []Album{
		{Artist: "Slowdive", Title: "Souvlaki", Year: 1993, Label: "Creation Records", Genres: []string{"Shoegaze"}},
		{Artist: "Ride", Title: "Nowhere", Year: 1990, Label: "Creation Records", Genres: []string{"Shoegaze"}},
	}
	out := formatList(albums, false, false)
	if !strings.Contains(out, "Slowdive") {
		t.Errorf("output missing Slowdive: %q", out)
	}
	if !strings.Contains(out, "Ride") {
		t.Errorf("output missing Ride: %q", out)
	}
	if !strings.Contains(out, "2 albums") {
		t.Errorf("output missing count summary: %q", out)
	}
}

func TestRunListEmpty(t *testing.T) {
	out := formatList([]Album{}, false, false)
	if !strings.Contains(out, "No albums") {
		t.Errorf("expected empty message, got: %q", out)
	}
}

func TestRunListSeparator(t *testing.T) {
	albums := []Album{
		{Artist: "A", Title: "X"},
		{Artist: "B", Title: "Y"},
	}
	out := formatList(albums, false, false)
	// There should be a blank line between the two entries
	if !strings.Contains(out, "\n\n") {
		t.Errorf("expected blank line separator between entries: %q", out)
	}
}

func TestRunListSingular(t *testing.T) {
	out := formatList([]Album{{Artist: "A", Title: "X"}}, false, false)
	if !strings.Contains(out, "1 album") {
		t.Errorf("expected singular 'album', got: %q", out)
	}
	if strings.Contains(out, "1 albums") {
		t.Errorf("unexpected plural '1 albums': %q", out)
	}
}

// --- Exit-code coverage -----------------------------------------------
//
// None of the orchestration functions (runPick, runList, runFavorite,
// runUnfavorite, runHistory, dispatch, selectAlbums, loadCollectionOrExit,
// loadFavoritesOrExit) were exercised by any test: os.Exit inside those
// functions can't be observed by calling them in-process, since it would
// kill the test binary itself. The standard fix is the Go self-exec helper
// pattern (as used by package os/exec's own tests): re-run this same test
// binary as a subprocess restricted to TestHelperProcess, which actually
// calls dispatch and lets any os.Exit take down that subprocess instead of
// us, then inspect the subprocess's real exit code.

// TestHelperProcess is not a real test. It stays inert under a normal `go
// test` run because DISC_FORTUNE_HELPER is unset; runHelper (below) sets it
// when re-execing the binary.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("DISC_FORTUNE_HELPER") != "1" {
		return
	}
	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+1:]
			break
		}
	}
	dispatch(args)

	// A success falls off the end of dispatch rather than calling os.Exit, so
	// without this the test binary's own "PASS" would land on the
	// subprocess's stdout right after the command's real output -- harmless
	// for the Contains checks elsewhere, but fatal to a caller that parses
	// stdout as a single JSON value.
	os.Exit(0)
}

// helperEnv builds the subprocess environment: HOME pinned at home, and the
// variables disc-fortune reads from the environment stripped, so a developer
// who happens to have XDG_CONFIG_HOME or NO_COLOR set does not get different
// test results from CI.
func helperEnv(home string) []string {
	var env []string
	for _, e := range os.Environ() {
		switch {
		case strings.HasPrefix(e, "HOME="),
			strings.HasPrefix(e, "XDG_CONFIG_HOME="),
			strings.HasPrefix(e, "NO_COLOR="):
			continue
		}
		env = append(env, e)
	}
	return append(env, "HOME="+home)
}

// runHelper re-execs the test binary as `disc-fortune <args...>` with HOME
// pointed at home, and returns the subprocess's exit code and combined
// output.
func runHelper(t *testing.T, home string, args ...string) (exitCode int, output string) {
	t.Helper()

	helperArgs := append([]string{"-test.run=^TestHelperProcess$", "--"}, args...)
	cmd := exec.Command(os.Args[0], helperArgs...)

	cmd.Env = append(helperEnv(home), "DISC_FORTUNE_HELPER=1")

	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), string(out)
	}
	t.Fatalf("running helper process: %v\noutput:\n%s", err, out)
	return -1, string(out)
}

// runHelperSplit is runHelper but captures stdout and stderr separately,
// for tests that need to confirm a message landed on the right stream (not
// just that it appeared somewhere in the combined output).
func runHelperSplit(t *testing.T, home string, args ...string) (exitCode int, stdout, stderr string) {
	t.Helper()

	return runHelperEnv(t, home, nil, args...)
}

// runHelperEnv is runHelperSplit with extra environment variables layered on
// top, for the tests that exercise XDG_CONFIG_HOME and NO_COLOR.
func runHelperEnv(t *testing.T, home string, extraEnv []string, args ...string) (exitCode int, stdout, stderr string) {
	t.Helper()

	helperArgs := append([]string{"-test.run=^TestHelperProcess$", "--"}, args...)
	cmd := exec.Command(os.Args[0], helperArgs...)

	cmd.Env = append(append(helperEnv(home), "DISC_FORTUNE_HELPER=1"), extraEnv...)

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err == nil {
		return 0, outBuf.String(), errBuf.String()
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), outBuf.String(), errBuf.String()
	}
	t.Fatalf("running helper process: %v\nstdout:\n%s\nstderr:\n%s", err, outBuf.String(), errBuf.String())
	return -1, outBuf.String(), errBuf.String()
}

// fixturePaths returns where the collection/favorites/history files live
// under a HOME directory, matching configDir()'s layout.
func fixturePaths(home string) (collection, favorites, history string) {
	dir := filepath.Join(home, ".config", "disc-fortune")
	return filepath.Join(dir, "collection.json"), filepath.Join(dir, "favorites.json"), filepath.Join(dir, "history.json")
}

func mustSaveCollection(t *testing.T, path string, albums []Album) {
	t.Helper()
	if err := saveCollectionTo(path, albums); err != nil {
		t.Fatalf("saveCollectionTo: %v", err)
	}
}

func mustSaveFavorites(t *testing.T, path string, albums []Album) {
	t.Helper()
	if err := saveFavorites(path, albums); err != nil {
		t.Fatalf("saveFavorites: %v", err)
	}
}

func mustSaveHistory(t *testing.T, path string, entries []HistoryEntry) {
	t.Helper()
	if err := saveHistory(path, entries); err != nil {
		t.Fatalf("saveHistory: %v", err)
	}
}

// TestExitCodes pins the exit-code table that is this release's headline
// breaking change: 0 when a command produced what was asked for, 1 when it
// could not. Each row seeds a throwaway HOME with just enough fixture data
// to reach the situation under test, then checks the real binary's exit
// code via the subprocess helper above.
func TestExitCodes(t *testing.T) {
	miles := Album{Artist: "Miles Davis", Title: "Kind of Blue", Year: 1959}
	beatles := Album{Artist: "The Beatles", Title: "Revolver", Year: 1966}
	who := Album{Artist: "The Who", Title: "Tommy", Year: 1969}

	tests := []struct {
		name  string
		args  []string
		setup func(t *testing.T, home string)
		want  int
	}{
		{
			name: "no collection file",
			args: []string{"pick"},
			want: 1,
		},
		{
			name: "collection file present but empty",
			args: []string{"pick"},
			setup: func(t *testing.T, home string) {
				collection, _, _ := fixturePaths(home)
				mustSaveCollection(t, collection, []Album{})
			},
			want: 1,
		},
		{
			name: "no favorites yet (pick --favorites)",
			args: []string{"pick", "--favorites"},
			want: 1,
		},
		{
			name: "pick: no albums match",
			args: []string{"pick", "--year", "1899"},
			setup: func(t *testing.T, home string) {
				collection, _, _ := fixturePaths(home)
				mustSaveCollection(t, collection, []Album{miles})
			},
			want: 1,
		},
		{
			name: "list: no albums match",
			args: []string{"list", "--year", "1899"},
			setup: func(t *testing.T, home string) {
				collection, _, _ := fixturePaths(home)
				mustSaveCollection(t, collection, []Album{miles})
			},
			want: 1,
		},
		{
			name: "favorite: no match",
			args: []string{"favorite", "does not exist zzz"},
			setup: func(t *testing.T, home string) {
				collection, _, _ := fixturePaths(home)
				mustSaveCollection(t, collection, []Album{miles})
			},
			want: 1,
		},
		{
			name: "unfavorite: no match, favorites file absent",
			args: []string{"unfavorite", "does not exist zzz"},
			// No setup: favorites.json does not exist. Removal is
			// idempotent, so this must succeed quietly rather than fatal
			// with "No favorites yet" (that guidance is for the read paths,
			// not for removal).
			want: 0,
		},
		{
			name: "unfavorite: no match, favorites file empty",
			args: []string{"unfavorite", "does not exist zzz"},
			setup: func(t *testing.T, home string) {
				_, favorites, _ := fixturePaths(home)
				mustSaveFavorites(t, favorites, []Album{})
			},
			want: 0,
		},
		{
			name: "unfavorite: no match, favorites populated (exercises UnfavoriteNoMatch)",
			args: []string{"unfavorite", "does not exist zzz"},
			// Unlike the two subtests above, favorites.json here has a real,
			// non-matching entry. loadFavoritesChecked succeeds (favorites
			// isn't empty), so this reaches unfavoriteByQuery and its
			// case UnfavoriteNoMatch switch arm in runUnfavorite, rather than
			// the errNoFavorites shortcut the two subtests above take.
			setup: func(t *testing.T, home string) {
				_, favorites, _ := fixturePaths(home)
				mustSaveFavorites(t, favorites, []Album{miles})
			},
			want: 0,
		},
		{
			name: "favorite: multiple matches",
			args: []string{"favorite", "the"},
			setup: func(t *testing.T, home string) {
				collection, _, _ := fixturePaths(home)
				mustSaveCollection(t, collection, []Album{beatles, who})
			},
			want: 1,
		},
		{
			name: "unfavorite: multiple matches",
			args: []string{"unfavorite", "the"},
			setup: func(t *testing.T, home string) {
				_, favorites, _ := fixturePaths(home)
				mustSaveFavorites(t, favorites, []Album{beatles, who})
			},
			want: 1,
		},
		{
			name: "favorite: already in favorites (bare, last pick)",
			args: []string{"favorite"},
			setup: func(t *testing.T, home string) {
				_, favorites, history := fixturePaths(home)
				mustSaveFavorites(t, favorites, []Album{miles})
				mustSaveHistory(t, history, []HistoryEntry{{Album: miles, Timestamp: time.Now()}})
			},
			want: 0,
		},
		{
			name: "unfavorite: not in favorites (bare, last pick)",
			args: []string{"unfavorite"},
			setup: func(t *testing.T, home string) {
				_, _, history := fixturePaths(home)
				mustSaveHistory(t, history, []HistoryEntry{{Album: miles, Timestamp: time.Now()}})
			},
			want: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			if tc.setup != nil {
				tc.setup(t, home)
			}
			got, output := runHelper(t, home, tc.args...)
			if got != tc.want {
				t.Errorf("exit code = %d, want %d\nargs: %v\noutput:\n%s", got, tc.want, tc.args, output)
			}
		})
	}
}

// TestFailureDiagnosticsGoToStderr pins finding #2 from the final review:
// once a command exits 1, its explanation must be readable on stderr (and
// absent from stdout), so `album=$(disc-fortune)` never captures error text
// as if it were a real result and `2>/dev/null` actually silences it.
func TestFailureDiagnosticsGoToStderr(t *testing.T) {
	miles := Album{Artist: "Miles Davis", Title: "Kind of Blue", Year: 1959}
	beatles := Album{Artist: "The Beatles", Title: "Revolver", Year: 1966}
	who := Album{Artist: "The Who", Title: "Tommy", Year: 1969}

	t.Run("pick: no albums match", func(t *testing.T) {
		home := t.TempDir()
		collection, _, _ := fixturePaths(home)
		mustSaveCollection(t, collection, []Album{miles})

		got, stdout, stderr := runHelperSplit(t, home, "pick", "--year", "1899")
		if got != 1 {
			t.Fatalf("exit code = %d, want 1", got)
		}
		if stdout != "" {
			t.Errorf("stdout = %q, want empty", stdout)
		}
		if !strings.Contains(stderr, "No albums match the specified filters") {
			t.Errorf("stderr = %q, want the no-match message", stderr)
		}
	})

	t.Run("list: no albums match", func(t *testing.T) {
		home := t.TempDir()
		collection, _, _ := fixturePaths(home)
		mustSaveCollection(t, collection, []Album{miles})

		got, stdout, stderr := runHelperSplit(t, home, "list", "--year", "1899")
		if got != 1 {
			t.Fatalf("exit code = %d, want 1", got)
		}
		if stdout != "" {
			t.Errorf("stdout = %q, want empty", stdout)
		}
		if !strings.Contains(stderr, "No albums match the specified filters") {
			t.Errorf("stderr = %q, want the no-match message", stderr)
		}
	})

	t.Run("favorite: multiple matches keeps the list on stdout but moves the trailer to stderr", func(t *testing.T) {
		home := t.TempDir()
		collection, _, _ := fixturePaths(home)
		mustSaveCollection(t, collection, []Album{beatles, who})

		got, stdout, stderr := runHelperSplit(t, home, "favorite", "the")
		if got != 1 {
			t.Fatalf("exit code = %d, want 1", got)
		}
		if !strings.Contains(stdout, "The Beatles") || !strings.Contains(stdout, "The Who") {
			t.Errorf("stdout = %q, want the disambiguation list", stdout)
		}
		if strings.Contains(stdout, "Be more specific") {
			t.Errorf("stdout = %q, want the trailer moved off stdout", stdout)
		}
		if !strings.Contains(stderr, "Be more specific, add filters, or use --release-id.") {
			t.Errorf("stderr = %q, want the trailer", stderr)
		}
	})

	t.Run("unfavorite: multiple matches keeps the list on stdout but moves the trailer to stderr", func(t *testing.T) {
		home := t.TempDir()
		_, favorites, _ := fixturePaths(home)
		mustSaveFavorites(t, favorites, []Album{beatles, who})

		got, stdout, stderr := runHelperSplit(t, home, "unfavorite", "the")
		if got != 1 {
			t.Fatalf("exit code = %d, want 1", got)
		}
		if !strings.Contains(stdout, "The Beatles") || !strings.Contains(stdout, "The Who") {
			t.Errorf("stdout = %q, want the disambiguation list", stdout)
		}
		if strings.Contains(stdout, "Be more specific") {
			t.Errorf("stdout = %q, want the trailer moved off stdout", stdout)
		}
		if !strings.Contains(stderr, "Be more specific, add filters, or use --release-id.") {
			t.Errorf("stderr = %q, want the trailer", stderr)
		}
	})
}

// TestFormatListHidesIDsByDefault: everyday list output is unchanged from
// v2.2.0 -- the release ID stays out of it.
func TestFormatListHidesIDsByDefault(t *testing.T) {
	albums := []Album{{ReleaseID: 1839278, Artist: "Slowdive", Title: "Souvlaki", Year: 1993}}

	got := formatList(albums, false, false)
	if strings.Contains(got, "1839278") || strings.Contains(got, "release") {
		t.Errorf("formatList showed the ID:\n%s", got)
	}
}

// TestFormatListShowsIDsWhenAsked: at a multi-match the ID is the only thing
// that can tell two identical-looking pressings apart, and the only thing
// --release-id can act on.
func TestFormatListShowsIDsWhenAsked(t *testing.T) {
	albums := []Album{
		{ReleaseID: 1839278, Artist: "Slowdive", Title: "Souvlaki", Year: 1993},
		{ReleaseID: 9112233, Artist: "Slowdive", Title: "Souvlaki", Year: 1993},
	}

	got := formatList(albums, false, true)
	for _, want := range []string{"release 1839278", "release 9112233"} {
		if !strings.Contains(got, want) {
			t.Errorf("formatList missing %q:\n%s", want, got)
		}
	}
}

// An entry with no ID -- written before v2.2.0 -- has nothing to show, and
// must not render a bare "release 0".
func TestFormatListOmitsAbsentIDs(t *testing.T) {
	albums := []Album{{Artist: "Slowdive", Title: "Souvlaki"}}

	got := formatList(albums, false, true)
	if strings.Contains(got, "release") {
		t.Errorf("formatList showed a release line for an un-ID'd entry:\n%s", got)
	}
}

// TestReleaseIDSelectsWithoutAQuery covers the routing, not just the parsing:
// runFavorite sends an empty query to the last pick, so a --release-id with
// no query has to be recognised there too, or the flag is silently ignored.
func TestReleaseIDSelectsWithoutAQuery(t *testing.T) {
	blue := Album{ReleaseID: 111, Artist: "Slowdive", Title: "Souvlaki", Year: 1993}
	clear := Album{ReleaseID: 222, Artist: "Slowdive", Title: "Souvlaki", Year: 1993}

	t.Run("favorite", func(t *testing.T) {
		home := t.TempDir()
		collection, favorites, _ := fixturePaths(home)
		mustSaveCollection(t, collection, []Album{blue, clear})

		code, stdout, stderr := runHelperSplit(t, home, "favorite", "--release-id", "222")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stdout=%q stderr=%q)", code, stdout, stderr)
		}

		favs, err := loadFavorites(favorites)
		if err != nil {
			t.Fatalf("loadFavorites: %v", err)
		}
		if len(favs) != 1 || favs[0].ReleaseID != 222 {
			t.Errorf("favorites = %+v, want only release 222", favs)
		}
	})

	t.Run("unfavorite", func(t *testing.T) {
		home := t.TempDir()
		collection, favorites, _ := fixturePaths(home)
		mustSaveCollection(t, collection, []Album{blue, clear})
		if err := saveFavorites(favorites, []Album{blue, clear}); err != nil {
			t.Fatalf("saveFavorites: %v", err)
		}

		code, stdout, stderr := runHelperSplit(t, home, "unfavorite", "--release-id", "222")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stdout=%q stderr=%q)", code, stdout, stderr)
		}

		favs, err := loadFavorites(favorites)
		if err != nil {
			t.Fatalf("loadFavorites: %v", err)
		}
		if len(favs) != 1 || favs[0].ReleaseID != 111 {
			t.Errorf("favorites = %+v, want only release 111 left", favs)
		}
	})
}

// TestNoMatchMessageNamesTheReleaseID: a query-less --release-id would
// otherwise be reported as an empty query -- 'No albums match query ""'.
func TestNoMatchMessageNamesTheReleaseID(t *testing.T) {
	album := Album{ReleaseID: 111, Artist: "Slowdive", Title: "Souvlaki"}

	t.Run("favorite", func(t *testing.T) {
		home := t.TempDir()
		collection, _, _ := fixturePaths(home)
		mustSaveCollection(t, collection, []Album{album})

		code, _, stderr := runHelperSplit(t, home, "favorite", "--release-id", "999")
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		if !strings.Contains(stderr, "release 999") {
			t.Errorf("stderr = %q, want it to name release 999", stderr)
		}
		if strings.Contains(stderr, `""`) {
			t.Errorf("stderr = %q, reports an empty query", stderr)
		}
	})

	t.Run("unfavorite", func(t *testing.T) {
		home := t.TempDir()
		collection, favorites, _ := fixturePaths(home)
		mustSaveCollection(t, collection, []Album{album})
		if err := saveFavorites(favorites, []Album{album}); err != nil {
			t.Fatalf("saveFavorites: %v", err)
		}

		code, stdout, _ := runHelperSplit(t, home, "unfavorite", "--release-id", "999")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (removal is idempotent)", code)
		}
		if !strings.Contains(stdout, "release 999") {
			t.Errorf("stdout = %q, want it to name release 999", stdout)
		}
	})
}

// mustSaveHistory and fixturePaths already exist in this file; runHelper
// re-execs the test binary as the real CLI so os.Exit is observable.

func TestPickUnheardExhaustedExitsOne(t *testing.T) {
	home := t.TempDir()
	collection, _, history := fixturePaths(home)

	albums := []Album{
		{ReleaseID: 1, Artist: "Slowdive", Title: "Souvlaki"},
		{ReleaseID: 2, Artist: "Ride", Title: "Nowhere"},
	}
	mustSaveCollection(t, collection, albums)
	mustSaveHistory(t, history, []HistoryEntry{
		{Album: albums[0], Timestamp: time.Now()},
		{Album: albums[1], Timestamp: time.Now()},
	})

	code, stdout, stderr := runHelperSplit(t, home, "pick", "--unheard")
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "already been played") {
		t.Errorf("stderr does not explain the exhaustion: %q", stderr)
	}
	if stdout != "" {
		t.Errorf("stdout should stay empty on failure, got %q", stdout)
	}
}

func TestPickUnheardReturnsTheUnplayedOne(t *testing.T) {
	home := t.TempDir()
	collection, _, history := fixturePaths(home)

	albums := []Album{
		{ReleaseID: 1, Artist: "Slowdive", Title: "Souvlaki"},
		{ReleaseID: 2, Artist: "Ride", Title: "Nowhere"},
	}
	mustSaveCollection(t, collection, albums)
	mustSaveHistory(t, history, []HistoryEntry{{Album: albums[0], Timestamp: time.Now()}})

	code, stdout, _ := runHelperSplit(t, home, "pick", "--unheard")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "Ride") {
		t.Errorf("stdout = %q, want the never-played album", stdout)
	}
}

func TestPickRejectsBadDrawValue(t *testing.T) {
	home := t.TempDir()
	collection, _, _ := fixturePaths(home)
	mustSaveCollection(t, collection, []Album{{ReleaseID: 1, Artist: "A", Title: "1"}})

	code, _, stderr := runHelperSplit(t, home, "pick", "--draw", "weighted")
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "--draw") {
		t.Errorf("stderr does not mention --draw: %q", stderr)
	}
}

func TestListUnheardFiltersPlayedAlbums(t *testing.T) {
	home := t.TempDir()
	collection, _, history := fixturePaths(home)

	albums := []Album{
		{ReleaseID: 1, Artist: "Slowdive", Title: "Souvlaki"},
		{ReleaseID: 2, Artist: "Ride", Title: "Nowhere"},
	}
	mustSaveCollection(t, collection, albums)
	mustSaveHistory(t, history, []HistoryEntry{{Album: albums[0], Timestamp: time.Now()}})

	code, stdout, _ := runHelperSplit(t, home, "list", "--unheard")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.Contains(stdout, "Slowdive") {
		t.Errorf("stdout lists a played album: %q", stdout)
	}
	if !strings.Contains(stdout, "Ride") {
		t.Errorf("stdout is missing the unplayed album: %q", stdout)
	}
	if !strings.Contains(stdout, "1 album") {
		t.Errorf("stdout is missing the count: %q", stdout)
	}
}

func TestListUnheardExhaustedExitsOne(t *testing.T) {
	home := t.TempDir()
	collection, _, history := fixturePaths(home)

	album := Album{ReleaseID: 1, Artist: "Slowdive", Title: "Souvlaki"}
	mustSaveCollection(t, collection, []Album{album})
	mustSaveHistory(t, history, []HistoryEntry{{Album: album, Timestamp: time.Now()}})

	code, stdout, stderr := runHelperSplit(t, home, "list", "--unheard")
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "already been played") {
		t.Errorf("stderr does not explain the exhaustion: %q", stderr)
	}
	if stdout != "" {
		t.Errorf("stdout should stay empty on failure, got %q", stdout)
	}
}

// TestPickFavoritesStaysAHardFilterUnderTheDefaultDraw guards the roadmap's
// maintainer dissent: --favorites must never become a soft bias. The new
// default anti-repeat draw runs over whatever pool a filter produces, and
// --favorites is a filter like any other, so its pool is subject to that
// draw too -- but the pool itself must never include a non-favorite. This
// pins that a run of picks confined to --favorites, exercised across enough
// draws to invoke the anti-repeat exclusion at least once, never returns the
// one album that was left out of favorites.
func TestPickFavoritesStaysAHardFilterUnderTheDefaultDraw(t *testing.T) {
	home := t.TempDir()
	collection, favorites, _ := fixturePaths(home)

	fav1 := Album{ReleaseID: 1, Artist: "Fav", Title: "One"}
	fav2 := Album{ReleaseID: 2, Artist: "Fav", Title: "Two"}
	fav3 := Album{ReleaseID: 3, Artist: "Fav", Title: "Three"}
	nonFav := Album{ReleaseID: 4, Artist: "NotAFavorite", Title: "Excluded"}

	mustSaveCollection(t, collection, []Album{fav1, fav2, fav3, nonFav})
	mustSaveFavorites(t, favorites, []Album{fav1, fav2, fav3})

	// history.json persists across these calls (each is a fresh process
	// reading and writing the same fixture directory), so this both
	// accumulates enough history to trigger the default exclusion window
	// and exercises the draw's randomness across several picks.
	for i := 0; i < 20; i++ {
		code, stdout, stderr := runHelperSplit(t, home, "pick", "--favorites")
		if code != 0 {
			t.Fatalf("run %d: exit code = %d, want 0 (stderr: %q)", i, code, stderr)
		}
		if strings.Contains(stdout, "NotAFavorite") {
			t.Fatalf("run %d: stdout picked the non-favorite: %q", i, stdout)
		}
		if !strings.Contains(stdout, "Fav") {
			t.Fatalf("run %d: stdout did not pick a favorite: %q", i, stdout)
		}
	}
}

// TestPickDefaultDrawAvoidsRepeatsAcrossSequentialRuns closes a gap the unit
// tests leave open: TestPickAlbumFreshExcludesRecent (picker_test.go) pins
// pickAlbum directly, and TestPickFavoritesStaysAHardFilterUnderTheDefaultDraw
// above never asserts anti-repeat happened at all. Nothing exercises the
// wiring at main.go's runPick -- pickAlbum(albums, entries, cfg.draw,
// newRNG()) -- so a future change that hardcoded drawAny there, or dropped
// draw from the selection parseSelection returns, would leave the whole
// suite green while plain `disc-fortune` silently reverted to pre-2.3
// behavior. This runs the real binary with no flags, the default this
// release exists to ship.
//
// A pool of 6 gives antiRepeatWindow(6) == 6/3 == 2: each pick excludes the
// two most recently played distinct albums. history.json persists across
// these sequential runs against the same home, the way it does for a real
// user, so by the third pick the window is in effect and stays in effect:
// at pick k (0-indexed, k >= 2) the two most recent distinct albums are
// excluded, so pick k must differ from both pick k-1 and pick k-2 -- i.e. no
// album repeats within any 3 consecutive picks.
func TestPickDefaultDrawAvoidsRepeatsAcrossSequentialRuns(t *testing.T) {
	home := t.TempDir()
	collection, _, _ := fixturePaths(home)

	albums := []Album{
		{ReleaseID: 1, Artist: "Aardvark", Title: "One"},
		{ReleaseID: 2, Artist: "Bobcat", Title: "Two"},
		{ReleaseID: 3, Artist: "Coyote", Title: "Three"},
		{ReleaseID: 4, Artist: "Dingo", Title: "Four"},
		{ReleaseID: 5, Artist: "Egret", Title: "Five"},
		{ReleaseID: 6, Artist: "Falcon", Title: "Six"},
	}
	mustSaveCollection(t, collection, albums)

	var picks []string
	for i := 0; i < 6; i++ {
		code, stdout, stderr := runHelperSplit(t, home, "pick")
		if code != 0 {
			t.Fatalf("run %d: exit code = %d, want 0 (stderr: %q)", i, code, stderr)
		}
		if stdout == "" {
			t.Fatalf("run %d: stdout is empty", i)
		}

		// pick prints "Artist - Title" on the first line and a metadata
		// line only when the album has metadata; these fixtures have none,
		// but split on the first line anyway so the test does not depend
		// on that.
		line := stdout
		if idx := strings.IndexByte(stdout, '\n'); idx >= 0 {
			line = stdout[:idx]
		}
		picks = append(picks, line)

		if i >= 2 {
			if picks[i] == picks[i-1] {
				t.Errorf("run %d: repeated the immediately preceding pick %q", i, picks[i])
			}
			if picks[i] == picks[i-2] {
				t.Errorf("run %d: repeated pick %d within the 3-pick anti-repeat window: %q", i, i-2, picks[i])
			}
		}
	}
}

// TestPickFavoritesAntiRepeatSurvivesAnInterleavedUnfilteredPick guards the
// agreement between the two halves of the anti-repeat window: it is SIZED from
// the filtered pool, so it must also be FILLED from that pool.
//
// Neither test above catches this.
// TestPickFavoritesStaysAHardFilterUnderTheDefaultDraw only ever runs
// consecutive `pick --favorites`, and TestPickDefaultDrawAvoidsRepeatsAcrossSequentialRuns
// runs no filter at all -- so both stay green even if the window is filled
// from unfiltered global history. When it is, a plain `pick` between two
// favorites picks spends the favorites window on a record that is not a
// favorite, and the favorite played moments earlier becomes immediately
// re-pickable.
//
// The interleaved pick is deliberately `--genre filler`, not a bare `pick`.
// A bare pick could land on a favorite, which would legitimately advance the
// favorites window and make the assertion below wrong for the right reason.
// Restricting it to the non-favorite genre keeps the favorites pool's own
// history untouched, so with a 3-favorite pool (window 1) the two favorites
// picks must always differ.
//
// The round count is deliberate. With the window filled correctly this test
// never fails; with it filled from global history each round is an
// independent 1-in-3 chance of drawing the same favorite again, so one round
// would miss a regression two times in three. Fifteen rounds cut that to
// roughly (2/3)^15, about one in 430.
func TestPickFavoritesAntiRepeatSurvivesAnInterleavedUnfilteredPick(t *testing.T) {
	home := t.TempDir()
	collection, favoritesPath, _ := fixturePaths(home)

	favs := []Album{
		{ReleaseID: 1, Artist: "Aardvark", Title: "One", Genres: []string{"Jazz"}},
		{ReleaseID: 2, Artist: "Bobcat", Title: "Two", Genres: []string{"Jazz"}},
		{ReleaseID: 3, Artist: "Coyote", Title: "Three", Genres: []string{"Jazz"}},
	}
	others := []Album{
		{ReleaseID: 101, Artist: "Dingo", Title: "Four", Genres: []string{"Filler"}},
		{ReleaseID: 102, Artist: "Egret", Title: "Five", Genres: []string{"Filler"}},
		{ReleaseID: 103, Artist: "Falcon", Title: "Six", Genres: []string{"Filler"}},
		{ReleaseID: 104, Artist: "Gannet", Title: "Seven", Genres: []string{"Filler"}},
	}
	mustSaveCollection(t, collection, append(append([]Album{}, favs...), others...))
	mustSaveFavorites(t, favoritesPath, favs)

	firstLine := func(s string) string {
		return strings.SplitN(strings.TrimSpace(s), "\n", 2)[0]
	}

	for round := 0; round < 15; round++ {
		code, out, stderr := runHelperSplit(t, home, "pick", "--favorites")
		if code != 0 {
			t.Fatalf("round %d: first favorites pick exited %d (stderr: %q)", round, code, stderr)
		}
		before := firstLine(out)
		if before == "" {
			t.Fatalf("round %d: first favorites pick produced no stdout", round)
		}

		if code, _, stderr := runHelperSplit(t, home, "pick", "--genre", "filler"); code != 0 {
			t.Fatalf("round %d: interleaved non-favorite pick exited %d (stderr: %q)", round, code, stderr)
		}

		code, out, stderr = runHelperSplit(t, home, "pick", "--favorites")
		if code != 0 {
			t.Fatalf("round %d: second favorites pick exited %d (stderr: %q)", round, code, stderr)
		}
		after := firstLine(out)
		if after == "" {
			t.Fatalf("round %d: second favorites pick produced no stdout", round)
		}

		if before == after {
			t.Errorf("round %d: %q was re-picked immediately; the interleaved non-favorite pick consumed the favorites anti-repeat window", round, before)
		}
	}
}

// TestJSONOutput drives the real binary and parses what it emits. A payload
// that only looks right is not enough -- these decode it.
func TestJSONOutput(t *testing.T) {
	miles := Album{ReleaseID: 1839278, Artist: "Miles Davis", Title: "Kind of Blue", Year: 1959, Label: "Columbia", Genres: []string{"Jazz"}}
	bare := Album{Artist: "Some Artist", Title: "Untitled"}

	t.Run("pick emits one album and exits 0", func(t *testing.T) {
		home := t.TempDir()
		collection, _, _ := fixturePaths(home)
		mustSaveCollection(t, collection, []Album{miles})

		code, stdout, _ := runHelperSplit(t, home, "pick", "--json")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		var got pickPayload
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("stdout does not parse: %v\n%s", err, stdout)
		}
		if got.Album.Artist != "Miles Davis" {
			t.Errorf("artist = %q, want Miles Davis", got.Album.Artist)
		}
		if got.Album.ReleaseID == nil || *got.Album.ReleaseID != 1839278 {
			t.Errorf("release_id missing from the payload: %+v", got.Album)
		}
	})

	t.Run("pick still records history", func(t *testing.T) {
		home := t.TempDir()
		collection, _, historyFile := fixturePaths(home)
		mustSaveCollection(t, collection, []Album{miles})

		if code, _, stderr := runHelperSplit(t, home, "pick", "--json"); code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
		}
		entries, err := loadHistory(historyFile)
		if err != nil {
			t.Fatalf("loadHistory: %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("history has %d entries, want 1 -- --json is a format flag, not a dry run", len(entries))
		}
	})

	t.Run("list emits albums and a count", func(t *testing.T) {
		home := t.TempDir()
		collection, _, _ := fixturePaths(home)
		mustSaveCollection(t, collection, []Album{miles, bare})

		code, stdout, _ := runHelperSplit(t, home, "list", "--json")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		var got listPayload
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("stdout does not parse: %v\n%s", err, stdout)
		}
		if got.Count != 2 || len(got.Albums) != 2 {
			t.Errorf("count = %d, albums = %d, want 2 and 2", got.Count, len(got.Albums))
		}
	})

	t.Run("an album with nothing known still carries every key", func(t *testing.T) {
		home := t.TempDir()
		collection, _, _ := fixturePaths(home)
		mustSaveCollection(t, collection, []Album{bare})

		_, stdout, _ := runHelperSplit(t, home, "list", "--json")
		for _, key := range []string{`"release_id"`, `"artist"`, `"title"`, `"year"`, `"label"`, `"catno"`, `"genres"`, `"formats"`} {
			if !strings.Contains(stdout, key) {
				t.Errorf("payload is missing %s:\n%s", key, stdout)
			}
		}
		if !strings.Contains(stdout, `"genres": []`) {
			t.Errorf("absent genres should be [], not null:\n%s", stdout)
		}
	})

	t.Run("history is most recent first with a count", func(t *testing.T) {
		home := t.TempDir()
		collection, _, historyFile := fixturePaths(home)
		mustSaveCollection(t, collection, []Album{miles})
		base := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
		mustSaveHistory(t, historyFile, []HistoryEntry{
			{Album: Album{Artist: "oldest", Title: "1"}, Timestamp: base},
			{Album: Album{Artist: "middle", Title: "2"}, Timestamp: base.Add(time.Hour)},
			{Album: Album{Artist: "newest", Title: "3"}, Timestamp: base.Add(2 * time.Hour)},
		})

		code, stdout, _ := runHelperSplit(t, home, "history", "--json", "2")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		var got historyPayload
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("stdout does not parse: %v\n%s", err, stdout)
		}
		if got.Count != 2 {
			t.Errorf("count = %d, want 2 (what was emitted, not what the file holds)", got.Count)
		}
		if len(got.Entries) != 2 || got.Entries[0].Album.Artist != "newest" {
			t.Fatalf("entries not most-recent-first: %+v", got.Entries)
		}
	})
}

// TestJSONDoesNotChangeSemantics is the load-bearing test of this task.
// --json is a formatting flag: every exit code and every stream stays as it
// was, so anyone scripting today keeps working.
func TestJSONDoesNotChangeSemantics(t *testing.T) {
	miles := Album{Artist: "Miles Davis", Title: "Kind of Blue", Year: 1959}

	t.Run("list matching nothing still exits 1 with an empty stdout", func(t *testing.T) {
		home := t.TempDir()
		collection, _, _ := fixturePaths(home)
		mustSaveCollection(t, collection, []Album{miles})

		code, stdout, stderr := runHelperSplit(t, home, "list", "--json", "--year", "1899")
		if code != 1 {
			t.Errorf("exit code = %d, want 1", code)
		}
		if stdout != "" {
			t.Errorf("stdout = %q, want empty -- no partial payload on a failing exit", stdout)
		}
		if !strings.Contains(stderr, "No albums match the specified filters") {
			t.Errorf("stderr = %q, want the no-match message", stderr)
		}
	})

	t.Run("pick matching nothing still exits 1 with an empty stdout", func(t *testing.T) {
		home := t.TempDir()
		collection, _, _ := fixturePaths(home)
		mustSaveCollection(t, collection, []Album{miles})

		code, stdout, _ := runHelperSplit(t, home, "pick", "--json", "--year", "1899")
		if code != 1 {
			t.Errorf("exit code = %d, want 1", code)
		}
		if stdout != "" {
			t.Errorf("stdout = %q, want empty", stdout)
		}
	})

	// history and list disagree about whether an empty result is a failure.
	// That predates this task; the JSON mirrors it rather than reconciling
	// it, because changing either would be a silent change to a scripted
	// exit code.
	t.Run("history on an empty history exits 0 with an empty payload", func(t *testing.T) {
		home := t.TempDir()
		collection, _, historyFile := fixturePaths(home)
		mustSaveCollection(t, collection, []Album{miles})
		mustSaveHistory(t, historyFile, nil)

		code, stdout, _ := runHelperSplit(t, home, "history", "--json")
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		var got historyPayload
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("stdout does not parse: %v\n%s", err, stdout)
		}
		if got.Count != 0 || len(got.Entries) != 0 {
			t.Errorf("want an empty payload, got %+v", got)
		}
	})

	// --unheard exhausts before the JSON branch is ever reached: runList
	// exits 1 on its plain-text stderr message the same way it does without
	// --json. Safe by construction, but worth pinning down.
	t.Run("list --json --unheard on a fully-heard collection exits 1 with empty stdout", func(t *testing.T) {
		home := t.TempDir()
		collection, _, history := fixturePaths(home)
		mustSaveCollection(t, collection, []Album{miles})
		mustSaveHistory(t, history, []HistoryEntry{{Album: miles, Timestamp: time.Now()}})

		code, stdout, stderr := runHelperSplit(t, home, "list", "--json", "--unheard")
		if code != 1 {
			t.Errorf("exit code = %d, want 1", code)
		}
		if stdout != "" {
			t.Errorf("stdout = %q, want empty -- no partial payload on a failing exit", stdout)
		}
		if !strings.Contains(stderr, "already been played") {
			t.Errorf("stderr = %q, want the exhaustion message", stderr)
		}
	})

	// An ANSI escape inside a JSON string would be a parse hazard, so the
	// colour mode has no effect on this path.
	t.Run("--color=always injects no escapes", func(t *testing.T) {
		home := t.TempDir()
		collection, _, _ := fixturePaths(home)
		mustSaveCollection(t, collection, []Album{miles})

		for _, cmd := range [][]string{
			{"pick", "--json", "--color", "always"},
			{"list", "--json", "--color", "always"},
			{"history", "--json", "--color", "always"},
		} {
			_, stdout, _ := runHelperSplit(t, home, cmd...)
			if strings.ContainsRune(stdout, 0x1b) {
				t.Errorf("%v: stdout contains an ANSI escape:\n%q", cmd, stdout)
			}
		}
	})
}

func TestStatsEndToEnd(t *testing.T) {
	home := t.TempDir()
	collection, favorites, history := fixturePaths(home)

	miles := Album{ReleaseID: 1, Artist: "Miles Davis", Title: "Kind of Blue", Year: 1959, Label: "Columbia", Genres: []string{"Jazz"}}
	ride := Album{ReleaseID: 2, Artist: "Ride", Title: "Nowhere", Year: 1990, Label: "Creation", Genres: []string{"Shoegaze"}}
	mustSaveCollection(t, collection, []Album{miles, ride})
	mustSaveFavorites(t, favorites, []Album{miles})
	mustSaveHistory(t, history, []HistoryEntry{{Album: miles, Timestamp: time.Now()}})

	code, stdout, _ := runHelperSplit(t, home, "stats")
	if code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, stdout)
	}
	for _, want := range []string{"2 albums · 1 favorite", "1950s", "1990s", "Top genres", "Top labels", "1 of 2 albums picked at least once (50%)"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stats output missing %q:\n%s", want, stdout)
		}
	}
}

func TestStatsJSONParses(t *testing.T) {
	home := t.TempDir()
	collection, _, _ := fixturePaths(home)
	mustSaveCollection(t, collection, []Album{
		{ReleaseID: 1, Artist: "Miles Davis", Title: "Kind of Blue", Year: 1959, Genres: []string{"Jazz"}},
	})

	code, stdout, _ := runHelperSplit(t, home, "stats", "--json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, stdout)
	}

	var payload struct {
		Count  int `json:"count"`
		Picked struct {
			Share float64 `json:"share"`
		} `json:"picked"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("stdout is not one JSON value: %v\n%s", err, stdout)
	}
	if payload.Count != 1 {
		t.Errorf("count = %d, want 1", payload.Count)
	}
}

// --json changes the format, never the semantics: an empty match is still a
// failure, on stderr, with exit 1 and nothing on stdout.
func TestStatsEmptyMatchExitsOneInBothFormats(t *testing.T) {
	home := t.TempDir()
	collection, _, _ := fixturePaths(home)
	mustSaveCollection(t, collection, []Album{{ReleaseID: 1, Artist: "Ride", Title: "Nowhere", Year: 1990}})

	for _, args := range [][]string{{"stats", "--genre", "polka"}, {"stats", "--genre", "polka", "--json"}} {
		code, stdout, stderr := runHelperSplit(t, home, args...)
		if code != 1 {
			t.Errorf("%v: exit = %d, want 1", args, code)
		}
		if stdout != "" {
			t.Errorf("%v: stdout = %q, want empty", args, stdout)
		}
		if !strings.Contains(stderr, "No albums match") {
			t.Errorf("%v: stderr = %q, want a no-match message", args, stderr)
		}
	}
}

func TestStatsFavoritesOnlyHeader(t *testing.T) {
	home := t.TempDir()
	collection, favorites, _ := fixturePaths(home)
	miles := Album{ReleaseID: 1, Artist: "Miles Davis", Title: "Kind of Blue", Year: 1959}
	mustSaveCollection(t, collection, []Album{miles, {ReleaseID: 2, Artist: "Ride", Title: "Nowhere", Year: 1990}})
	mustSaveFavorites(t, favorites, []Album{miles})

	code, stdout, _ := runHelperSplit(t, home, "stats", "--favorites")
	if code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, stdout)
	}
	if !strings.Contains(stdout, "1 favorite\n") {
		t.Errorf("favorites header wrong:\n%s", stdout)
	}
}

func TestOpenPrintsTheLastPicksURL(t *testing.T) {
	home := t.TempDir()
	collection, _, history := fixturePaths(home)
	miles := Album{ReleaseID: 1839278, Artist: "Miles Davis", Title: "Kind of Blue"}
	mustSaveCollection(t, collection, []Album{miles})
	mustSaveHistory(t, history, []HistoryEntry{{Album: miles, Timestamp: time.Now()}})

	code, stdout, stderr := runHelperSplit(t, home, "open", "--print")
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstderr: %s", code, stderr)
	}
	if strings.TrimSpace(stdout) != "https://www.discogs.com/release/1839278" {
		t.Errorf("stdout = %q", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty under --print", stderr)
	}
}

func TestOpenPrintsAQueriedReleasesURL(t *testing.T) {
	home := t.TempDir()
	collection, _, _ := fixturePaths(home)
	mustSaveCollection(t, collection, []Album{
		{ReleaseID: 1839278, Artist: "Miles Davis", Title: "Kind of Blue"},
		{ReleaseID: 2, Artist: "Ride", Title: "Nowhere"},
	})

	code, stdout, _ := runHelperSplit(t, home, "open", "kind of blue", "--print")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(stdout, "release/1839278") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestOpenReleaseIDNeedsNoQuery(t *testing.T) {
	home := t.TempDir()
	collection, _, _ := fixturePaths(home)
	mustSaveCollection(t, collection, []Album{{ReleaseID: 42, Artist: "A", Title: "B"}})

	code, stdout, _ := runHelperSplit(t, home, "open", "--release-id", "42", "--print")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(stdout, "release/42") {
		t.Errorf("stdout = %q", stdout)
	}
}

// Ambiguity lists the candidates with their IDs and exits 1, exactly as
// favorite does. Nothing is launched, so --print is not needed here.
func TestOpenAmbiguousQueryListsCandidates(t *testing.T) {
	home := t.TempDir()
	collection, _, _ := fixturePaths(home)
	mustSaveCollection(t, collection, []Album{
		{ReleaseID: 1, Artist: "Miles Davis", Title: "Kind of Blue"},
		{ReleaseID: 2, Artist: "Miles Davis", Title: "Kind of Blue"},
	})

	code, stdout, stderr := runHelperSplit(t, home, "open", "kind of blue")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stdout, "release 1") || !strings.Contains(stdout, "release 2") {
		t.Errorf("candidates not listed with their IDs:\n%s", stdout)
	}
	if !strings.Contains(stderr, "--release-id") {
		t.Errorf("stderr = %q, want the disambiguation advice", stderr)
	}
}

func TestOpenNoMatchExitsOne(t *testing.T) {
	home := t.TempDir()
	collection, _, _ := fixturePaths(home)
	mustSaveCollection(t, collection, []Album{{ReleaseID: 1, Artist: "Ride", Title: "Nowhere"}})

	code, _, stderr := runHelperSplit(t, home, "open", "kind of blue")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "No albums match") {
		t.Errorf("stderr = %q", stderr)
	}
}

// A pre-v2.2 history entry the backfill could not identify has no release ID.
// Opening a release page would be guessing which pressing was meant, which is
// exactly what backfill refuses to do.
func TestOpenWithoutAReleaseIDExitsOne(t *testing.T) {
	home := t.TempDir()
	collection, _, history := fixturePaths(home)
	legacy := Album{Artist: "Miles Davis", Title: "Kind of Blue"}
	mustSaveCollection(t, collection, []Album{legacy})
	mustSaveHistory(t, history, []HistoryEntry{{Album: legacy, Timestamp: time.Now()}})

	code, stdout, stderr := runHelperSplit(t, home, "open", "--print")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "sync") || !strings.Contains(stderr, "--release-id") {
		t.Errorf("stderr = %q, want it to name both remedies", stderr)
	}
}

// --print covers every scripting case, so open has no --json. open --json
// would have to both emit a payload and launch a browser to honour the
// "format never semantics" rule -- true to the letter and useless in
// practice. This pins that decision.
func TestOpenHasNoJSONFlag(t *testing.T) {
	if _, err := parseOpen([]string{"--json"}); err == nil {
		t.Error("open accepted --json; --print is the scripting path")
	}
}

func TestOpenWithNoHistoryExitsOne(t *testing.T) {
	home := t.TempDir()
	collection, _, _ := fixturePaths(home)
	mustSaveCollection(t, collection, []Album{{ReleaseID: 1, Artist: "A", Title: "B"}})

	code, _, stderr := runHelperSplit(t, home, "open", "--print")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "No history") {
		t.Errorf("stderr = %q", stderr)
	}
}
