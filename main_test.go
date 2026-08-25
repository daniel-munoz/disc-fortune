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
	out := formatList(albums, false)
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
	out := formatList([]Album{}, false)
	if !strings.Contains(out, "No albums") {
		t.Errorf("expected empty message, got: %q", out)
	}
}

func TestRunListSeparator(t *testing.T) {
	albums := []Album{
		{Artist: "A", Title: "X"},
		{Artist: "B", Title: "Y"},
	}
	out := formatList(albums, false)
	// There should be a blank line between the two entries
	if !strings.Contains(out, "\n\n") {
		t.Errorf("expected blank line separator between entries: %q", out)
	}
}

func TestRunListSingular(t *testing.T) {
	out := formatList([]Album{{Artist: "A", Title: "X"}}, false)
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

// runHelper re-execs the test binary as `disc-fortune <args...>` with HOME
// pointed at home, and returns the subprocess's exit code and combined
// output.
func runHelper(t *testing.T, home string, args ...string) (exitCode int, output string) {
	t.Helper()

	helperArgs := append([]string{"-test.run=^TestHelperProcess$", "--"}, args...)
	cmd := exec.Command(os.Args[0], helperArgs...)

	var env []string
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "HOME=") {
			continue
		}
		env = append(env, e)
	}
	cmd.Env = append(env, "HOME="+home, "DISC_FORTUNE_HELPER=1")

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
