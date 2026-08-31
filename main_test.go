package main

import (
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
