package disc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationNoticeSilentWhenNothingToMigrate(t *testing.T) {
	meta := filepath.Join(t.TempDir(), "meta.json")
	loc := Location{Dir: "/somewhere/disc-fortune"}

	if got := MigrationNotice(loc, meta, true); got != "" {
		t.Errorf("MigrationNotice = %q, want empty when Dir is already correct", got)
	}
}

func TestMigrationNoticeSilentWhenDisabled(t *testing.T) {
	meta := filepath.Join(t.TempDir(), "meta.json")
	loc := Location{Dir: "/legacy/disc-fortune", Preferred: "/xdg/disc-fortune"}

	if got := MigrationNotice(loc, meta, false); got != "" {
		t.Errorf("MigrationNotice = %q, want empty when notices are off", got)
	}
}

func TestMigrationNoticeNamesBothPathsAndTheCommand(t *testing.T) {
	meta := filepath.Join(t.TempDir(), "meta.json")
	loc := Location{Dir: "/legacy/disc-fortune", Preferred: "/xdg/disc-fortune"}

	got := MigrationNotice(loc, meta, true)
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
	loc := Location{Dir: "/legacy/disc-fortune", Preferred: "/xdg/disc-fortune"}

	if first := MigrationNotice(loc, meta, true); first == "" {
		t.Fatal("first call produced no notice")
	}
	if second := MigrationNotice(loc, meta, true); second != "" {
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

	moved, err := Migrate(from, to)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
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

	if _, err := Migrate(from, to); err != nil {
		t.Fatalf("Migrate: %v", err)
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

	if _, err := Migrate(from, to); err == nil {
		t.Fatal("Migrate overwrote a populated destination")
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

	if _, err := Migrate(from, to); err != nil {
		t.Fatalf("an empty destination directory is not a conflict: %v", err)
	}
}

func TestMigrateConfigErrorsWhenSourceMissing(t *testing.T) {
	root := t.TempDir()
	from := filepath.Join(root, "nope")
	to := filepath.Join(root, "xdg", "disc-fortune")

	if _, err := Migrate(from, to); err == nil {
		t.Fatal("Migrate succeeded with no source directory")
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

	if _, err := Migrate(from, to); err != nil {
		t.Fatalf("Migrate: %v", err)
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

// A migrate that fails part-way must leave the filesystem as it found it.
// Otherwise it strands a partial destination directory, and every later run
// has to decide which of two directories is the real one.
func TestMigrateConfigCleansUpAfterAPartialFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the permission bits this test relies on")
	}
	root := t.TempDir()
	from := filepath.Join(root, "legacy", "disc-fortune")
	to := filepath.Join(root, "xdg", "disc-fortune")
	seedConfigDir(t, from)
	// os.ReadDir returns sorted names, so collection.json and favorites.json
	// copy successfully before history.json fails: a genuine partial failure.
	unreadable := filepath.Join(from, "history.json")
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, collectionFilePerms) })

	if _, err := Migrate(from, to); err == nil {
		t.Fatal("Migrate succeeded despite an unreadable source file")
	}

	if _, err := os.Stat(to); !os.IsNotExist(err) {
		entries, _ := os.ReadDir(to)
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("destination directory survived a failed migration, holding %v", names)
	}
	// And the originals are all still there.
	for _, name := range []string{"collection.json", "favorites.json", "history.json"} {
		if _, err := os.Stat(filepath.Join(from, name)); err != nil {
			t.Errorf("source %s was lost: %v", name, err)
		}
	}
}

// A destination the user already had must not be deleted by a failed migrate;
// only what this call created gets rolled back.
func TestMigrateConfigKeepsAPreexistingEmptyDestination(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the permission bits this test relies on")
	}
	root := t.TempDir()
	from := filepath.Join(root, "legacy", "disc-fortune")
	to := filepath.Join(root, "xdg", "disc-fortune")
	seedConfigDir(t, from)
	if err := os.MkdirAll(to, configDirPerms); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	unreadable := filepath.Join(from, "history.json")
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, collectionFilePerms) })

	if _, err := Migrate(from, to); err == nil {
		t.Fatal("Migrate succeeded despite an unreadable source file")
	}

	if _, err := os.Stat(to); err != nil {
		t.Errorf("a destination directory the user already had was removed: %v", err)
	}
	entries, _ := os.ReadDir(to)
	if len(entries) != 0 {
		t.Errorf("partial copies left behind in the destination: %d entries", len(entries))
	}
}

// Lock sidecars are runtime scaffolding, not the user's data. Copying them
// would inflate the "moved N files" count and litter the new directory.
func TestMigrateSkipsLockSidecars(t *testing.T) {
	from, to := t.TempDir(), filepath.Join(t.TempDir(), "xdg")

	if err := os.WriteFile(filepath.Join(from, "collection.json"), []byte("[]"), 0644); err != nil {
		t.Fatalf("writing collection: %v", err)
	}
	if err := os.WriteFile(filepath.Join(from, "history.json.lock"), nil, 0644); err != nil {
		t.Fatalf("writing lock sidecar: %v", err)
	}

	n, err := Migrate(from, to)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if n != 1 {
		t.Errorf("moved %d files, want 1 (the sidecar must not count)", n)
	}
	if _, err := os.Stat(filepath.Join(to, "history.json.lock")); !os.IsNotExist(err) {
		t.Error("the lock sidecar was copied to the destination")
	}
	// A skipped sidecar must not strand the legacy directory: it is ours, it
	// holds nothing, and leaving it keeps the old directory alive forever.
	if _, err := os.Stat(from); !os.IsNotExist(err) {
		t.Error("the legacy directory survived because a sidecar was left in it")
	}
}
