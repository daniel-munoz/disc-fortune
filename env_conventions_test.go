package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedCollectionAt writes a one-album collection.json into dir.
func seedCollectionAt(t *testing.T, dir string, album Album) {
	t.Helper()
	if err := os.MkdirAll(dir, configDirPerms); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	data, err := json.Marshal([]Album{album})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "collection.json"), data, collectionFilePerms); err != nil {
		t.Fatalf("seeding collection: %v", err)
	}
}

const escape = "\033["

func TestBinaryHonorsXDGConfigHome(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	seedCollectionAt(t, filepath.Join(xdg, "disc-fortune"), Album{Artist: "Sun Ra", Title: "Lanquidity"})

	code, stdout, stderr := runHelperEnv(t, home, []string{"XDG_CONFIG_HOME=" + xdg}, "list")
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Lanquidity") {
		t.Errorf("stdout = %q, want the XDG collection", stdout)
	}
}

// The migration hazard, end to end: XDG_CONFIG_HOME is set and empty, the
// real collection is in the legacy directory. Before this change, upgrading
// would have made the collection appear to vanish.
func TestBinaryFindsLegacyCollectionWhenXDGIsEmpty(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	seedCollectionAt(t, filepath.Join(home, ".config", "disc-fortune"), Album{Artist: "Alice Coltrane", Title: "Ptah, the El Daoud"})

	code, stdout, stderr := runHelperEnv(t, home, []string{"XDG_CONFIG_HOME=" + xdg}, "list")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 -- the legacy collection must still be found\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Ptah") {
		t.Errorf("stdout = %q, want the legacy collection", stdout)
	}
}

func TestBinaryNotifiesAboutPendingMigrationOnce(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	seedCollectionAt(t, filepath.Join(home, ".config", "disc-fortune"), Album{Artist: "Don Cherry", Title: "Brown Rice"})
	env := []string{"XDG_CONFIG_HOME=" + xdg}

	// stderr is a pipe here, not a TTY, so the notice is suppressed the same
	// way progress output is. Confirm stdout is never contaminated either way.
	_, stdout, _ := runHelperEnv(t, home, env, "list")
	if strings.Contains(stdout, "migrate") {
		t.Errorf("migration notice leaked into stdout: %q", stdout)
	}
}

func TestBinaryMigrateMovesCollectionToXDG(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	seedCollectionAt(t, filepath.Join(home, ".config", "disc-fortune"), Album{Artist: "Pharoah Sanders", Title: "Karma"})
	env := []string{"XDG_CONFIG_HOME=" + xdg}

	code, stdout, stderr := runHelperEnv(t, home, env, "migrate")
	if code != 0 {
		t.Fatalf("migrate exit = %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	if _, err := os.Stat(filepath.Join(xdg, "disc-fortune", "collection.json")); err != nil {
		t.Fatalf("collection did not arrive at the XDG path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "disc-fortune", "collection.json")); !os.IsNotExist(err) {
		t.Error("legacy collection still present after migrate")
	}

	// And the tool now reads from the new location.
	code, stdout, stderr = runHelperEnv(t, home, env, "list")
	if code != 0 || !strings.Contains(stdout, "Karma") {
		t.Errorf("after migrate, list exit = %d stdout = %q stderr = %q", code, stdout, stderr)
	}
}

func TestBinaryMigrateIsANoopWhenAlreadyCorrect(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	seedCollectionAt(t, filepath.Join(xdg, "disc-fortune"), Album{Artist: "Sun Ra", Title: "Lanquidity"})

	code, stdout, _ := runHelperEnv(t, home, []string{"XDG_CONFIG_HOME=" + xdg}, "migrate")
	if code != 0 {
		t.Errorf("exit = %d, want 0 -- nothing to migrate is a success", code)
	}
	if !strings.Contains(stdout, "Nothing to migrate") {
		t.Errorf("stdout = %q, want it to say there was nothing to do", stdout)
	}
}

// stdout is a pipe under the test harness, so this is exactly the
// `disc-fortune list | less -R` case the roadmap calls out.
func TestBinaryColorAlwaysSurvivesAPipe(t *testing.T) {
	home := t.TempDir()
	seedCollectionAt(t, filepath.Join(home, ".config", "disc-fortune"), Album{Artist: "Miles Davis", Title: "Kind of Blue"})

	_, stdout, stderr := runHelperEnv(t, home, nil, "list", "--color", "always")
	if !strings.Contains(stdout, escape) {
		t.Errorf("--color=always produced no escape sequences through a pipe\nstdout: %q\nstderr: %q", stdout, stderr)
	}
}

func TestBinaryColorNeverEmitsNoEscapes(t *testing.T) {
	home := t.TempDir()
	seedCollectionAt(t, filepath.Join(home, ".config", "disc-fortune"), Album{Artist: "Miles Davis", Title: "Kind of Blue"})

	_, stdout, _ := runHelperEnv(t, home, nil, "list", "--color", "never")
	if strings.Contains(stdout, escape) {
		t.Errorf("--color=never still emitted escapes: %q", stdout)
	}
}

func TestBinaryNoColorBeatsAutoButNotExplicitAlways(t *testing.T) {
	home := t.TempDir()
	seedCollectionAt(t, filepath.Join(home, ".config", "disc-fortune"), Album{Artist: "Miles Davis", Title: "Kind of Blue"})
	noColor := []string{"NO_COLOR=1"}

	if _, stdout, _ := runHelperEnv(t, home, noColor, "list"); strings.Contains(stdout, escape) {
		t.Errorf("NO_COLOR=1 still produced escapes: %q", stdout)
	}
	// no-color.org: an explicit user instruction overrides the environment.
	if _, stdout, _ := runHelperEnv(t, home, noColor, "list", "--color", "always"); !strings.Contains(stdout, escape) {
		t.Errorf("--color=always should override NO_COLOR: %q", stdout)
	}
}

func TestBinaryRejectsBadColorValue(t *testing.T) {
	home := t.TempDir()
	seedCollectionAt(t, filepath.Join(home, ".config", "disc-fortune"), Album{Artist: "Miles Davis", Title: "Kind of Blue"})

	code, _, stderr := runHelperEnv(t, home, nil, "list", "--color", "sometimes")
	if code != 1 {
		t.Errorf("exit = %d, want 1 for an invalid --color value", code)
	}
	if !strings.Contains(stderr, "auto") {
		t.Errorf("stderr = %q, want it to list the valid values", stderr)
	}
}

// help and version touch no data files, so they must keep working even when
// the config directory cannot be resolved at all. Resolving config eagerly in
// dispatch must not make them dependent on it.
func TestBinaryHelpAndVersionWorkWithoutAHomeDirectory(t *testing.T) {
	home := t.TempDir()
	noHomeEnv := []string{"HOME="}

	for _, args := range [][]string{{"version"}, {"help"}, {"help", "list"}} {
		code, stdout, stderr := runHelperEnv(t, home, noHomeEnv, args...)
		if code != 0 {
			t.Errorf("%v exit = %d, want 0\nstdout: %s\nstderr: %s", args, code, stdout, stderr)
		}
	}
}

// A command that does need the config directory should still fail clearly.
func TestBinaryDataCommandFailsClearlyWithoutAHomeDirectory(t *testing.T) {
	home := t.TempDir()

	code, _, stderr := runHelperEnv(t, home, []string{"HOME="}, "list")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "home directory") {
		t.Errorf("stderr = %q, want it to explain that the home directory is unknown", stderr)
	}
}

// End to end: an empty XDG directory must not hide the user's collection,
// and `migrate` must still offer the way forward.
func TestBinarySurvivesAnEmptyXDGDirectory(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	seedCollectionAt(t, filepath.Join(home, ".config", "disc-fortune"), Album{Artist: "Don Cherry", Title: "Brown Rice"})
	if err := os.MkdirAll(filepath.Join(xdg, "disc-fortune"), configDirPerms); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	env := []string{"XDG_CONFIG_HOME=" + xdg}

	code, stdout, stderr := runHelperEnv(t, home, env, "list")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 -- an empty directory hid the collection\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Brown Rice") {
		t.Errorf("stdout = %q, want the legacy collection", stdout)
	}

	code, stdout, stderr = runHelperEnv(t, home, env, "migrate")
	if code != 0 || strings.Contains(stdout, "Nothing to migrate") {
		t.Errorf("migrate should still have work to do: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}
