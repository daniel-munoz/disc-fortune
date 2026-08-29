package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationNoticeSilentWhenNothingToMigrate(t *testing.T) {
	meta := filepath.Join(t.TempDir(), "meta.json")
	loc := configLocation{Dir: "/somewhere/disc-fortune"}

	if got := migrationNotice(loc, meta, true); got != "" {
		t.Errorf("migrationNotice = %q, want empty when Dir is already correct", got)
	}
}

func TestMigrationNoticeSilentWhenDisabled(t *testing.T) {
	meta := filepath.Join(t.TempDir(), "meta.json")
	loc := configLocation{Dir: "/legacy/disc-fortune", Preferred: "/xdg/disc-fortune"}

	if got := migrationNotice(loc, meta, false); got != "" {
		t.Errorf("migrationNotice = %q, want empty when notices are off", got)
	}
}

func TestMigrationNoticeNamesBothPathsAndTheCommand(t *testing.T) {
	meta := filepath.Join(t.TempDir(), "meta.json")
	loc := configLocation{Dir: "/legacy/disc-fortune", Preferred: "/xdg/disc-fortune"}

	got := migrationNotice(loc, meta, true)
	for _, want := range []string{"/legacy/disc-fortune", "/xdg/disc-fortune", "disc-fortune migrate"} {
		if !strings.Contains(got, want) {
			t.Errorf("notice %q is missing %q", got, want)
		}
	}
}

// "One-time" is the whole point: a notice on every single pick would be
// noise, and the user would learn to ignore it.
func TestMigrationNoticeShowsOnlyOnce(t *testing.T) {
	meta := filepath.Join(t.TempDir(), "meta.json")
	loc := configLocation{Dir: "/legacy/disc-fortune", Preferred: "/xdg/disc-fortune"}

	if first := migrationNotice(loc, meta, true); first == "" {
		t.Fatal("first call produced no notice")
	}
	if second := migrationNotice(loc, meta, true); second != "" {
		t.Errorf("second call produced %q, want empty", second)
	}
}

// seedConfigDir creates a config directory holding the three data files.
func seedConfigDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, configDirPerms); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for name, body := range map[string]string{
		"collection.json": `[{"artist":"Miles Davis","title":"Kind of Blue"}]`,
		"favorites.json":  `[]`,
		"history.json":    `[]`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), collectionFilePerms); err != nil {
			t.Fatalf("seeding %s: %v", name, err)
		}
	}
}

func TestMigrateConfigMovesEveryDataFile(t *testing.T) {
	root := t.TempDir()
	from := filepath.Join(root, "legacy", "disc-fortune")
	to := filepath.Join(root, "xdg", "disc-fortune")
	seedConfigDir(t, from)

	moved, err := migrateConfig(from, to)
	if err != nil {
		t.Fatalf("migrateConfig: %v", err)
	}
	if moved != 3 {
		t.Errorf("moved = %d, want 3", moved)
	}

	got, err := os.ReadFile(filepath.Join(to, "collection.json"))
	if err != nil {
		t.Fatalf("collection did not arrive: %v", err)
	}
	if !strings.Contains(string(got), "Kind of Blue") {
		t.Errorf("collection content = %q", got)
	}
	if _, err := os.Stat(filepath.Join(from, "collection.json")); !os.IsNotExist(err) {
		t.Error("source file still present after migration")
	}
}

func TestMigrateConfigPreservesPermissions(t *testing.T) {
	root := t.TempDir()
	from := filepath.Join(root, "legacy", "disc-fortune")
	to := filepath.Join(root, "xdg", "disc-fortune")
	seedConfigDir(t, from)

	if _, err := migrateConfig(from, to); err != nil {
		t.Fatalf("migrateConfig: %v", err)
	}

	info, err := os.Stat(filepath.Join(to, "collection.json"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != collectionFilePerms {
		t.Errorf("perm = %o, want %o", got, collectionFilePerms)
	}
}

// Refusing rather than merging: silently overwriting a populated destination
// is how someone loses the collection they were trying to protect.
func TestMigrateConfigRefusesToOverwriteExistingData(t *testing.T) {
	root := t.TempDir()
	from := filepath.Join(root, "legacy", "disc-fortune")
	to := filepath.Join(root, "xdg", "disc-fortune")
	seedConfigDir(t, from)
	seedConfigDir(t, to)

	if _, err := migrateConfig(from, to); err == nil {
		t.Fatal("migrateConfig overwrote a populated destination")
	}
	if _, err := os.Stat(filepath.Join(from, "collection.json")); err != nil {
		t.Errorf("source was disturbed by a refused migration: %v", err)
	}
}

func TestMigrateConfigAcceptsAnEmptyDestination(t *testing.T) {
	root := t.TempDir()
	from := filepath.Join(root, "legacy", "disc-fortune")
	to := filepath.Join(root, "xdg", "disc-fortune")
	seedConfigDir(t, from)
	if err := os.MkdirAll(to, configDirPerms); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if _, err := migrateConfig(from, to); err != nil {
		t.Fatalf("an empty destination directory is not a conflict: %v", err)
	}
}

func TestMigrateConfigErrorsWhenSourceMissing(t *testing.T) {
	root := t.TempDir()
	from := filepath.Join(root, "nope")
	to := filepath.Join(root, "xdg", "disc-fortune")

	if _, err := migrateConfig(from, to); err == nil {
		t.Fatal("migrateConfig succeeded with no source directory")
	}
}

// migrate moves files that already exist; their modes belong to the user and
// must survive the move verbatim. Because writeFileAtomic honors the umask
// when creating a file, migrate has to restore the source mode explicitly --
// otherwise migrating under a strict umask silently re-permissions the data.
func TestMigrateConfigPreservesSourceModeUnderStrictUmask(t *testing.T) {
	root := t.TempDir()
	from := filepath.Join(root, "legacy", "disc-fortune")
	to := filepath.Join(root, "xdg", "disc-fortune")
	seedConfigDir(t, from)
	if err := os.Chmod(filepath.Join(from, "collection.json"), 0o644); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	withUmask(t, 0o077)

	if _, err := migrateConfig(from, to); err != nil {
		t.Fatalf("migrateConfig: %v", err)
	}

	info, err := os.Stat(filepath.Join(to, "collection.json"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if want := os.FileMode(0o644); info.Mode().Perm() != want {
		t.Errorf("perm = %04o, want %04o -- migrate re-permissioned the user's data",
			info.Mode().Perm(), want)
	}
}
