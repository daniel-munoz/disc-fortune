package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// migrationNotice returns the one-time notice telling a user that their data
// is still in the legacy directory and how to move it, or "" when there is
// nothing to migrate, the notice has already been shown, or notices are off.
//
// It is deliberately one-time. Repeating it on every pick would train the
// user to ignore it, and `disc-fortune migrate` stays discoverable in help
// either way. The "shown" flag is recorded best-effort: failing to record it
// means the notice appears again, which is harmless.
func migrationNotice(loc configLocation, metaFile string, enabled bool) string {
	if !enabled || loc.Preferred == "" {
		return ""
	}
	m, err := loadMeta(metaFile)
	if err != nil {
		m = Meta{}
	}
	if m.LegacyNoticeShown {
		return ""
	}
	m.LegacyNoticeShown = true
	_ = saveMeta(metaFile, m)

	return fmt.Sprintf(
		"Note: XDG_CONFIG_HOME is set, but disc-fortune is still reading %s.\n"+
			"      Run `disc-fortune migrate` to move it to %s.\n"+
			"      (This notice is shown once.)\n",
		loc.Dir, loc.Preferred)
}

// migrateConfig moves disc-fortune's data files from one config directory to
// another, returning how many files it moved.
//
// It copies-then-removes rather than renaming: a rename cannot cross
// filesystems, and XDG_CONFIG_HOME pointing at a different mount is exactly
// the situation this exists to serve. Each file is written atomically, so an
// interrupted migration leaves whole files at the destination and the
// originals still in place.
//
// A destination that already holds files is refused outright. Merging two
// collections is not something to guess at, and silently overwriting is how
// someone loses the data they were trying to protect.
func migrateConfig(from, to string) (int, error) {
	entries, err := os.ReadDir(from)
	if err != nil {
		return 0, fmt.Errorf("reading %s: %w", from, err)
	}

	// Whether this call creates the destination decides how much of it may be
	// rolled back later: a directory the user already had is not ours to
	// remove.
	createdDir := false
	if existing, err := os.ReadDir(to); err == nil {
		if len(existing) > 0 {
			return 0, fmt.Errorf("%s already contains data; move or remove it first", to)
		}
	} else if os.IsNotExist(err) {
		createdDir = true
	}
	if err := os.MkdirAll(to, configDirPerms); err != nil {
		return 0, fmt.Errorf("creating %s: %w", to, err)
	}

	var copied, written []string

	// rollback undoes everything this call created. Without it a failure
	// part-way leaves a partial destination behind, and every later run has
	// to guess which of two directories holds the real collection.
	rollback := func(cause error) (int, error) {
		for _, p := range written {
			_ = os.Remove(p)
		}
		if createdDir {
			_ = os.Remove(to)
		}
		return 0, cause
	}

	// Copy everything first, so a failure part-way never leaves the user with
	// neither copy.
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		src := filepath.Join(from, e.Name())
		data, err := os.ReadFile(src)
		if err != nil {
			return rollback(fmt.Errorf("reading %s: %w", src, err))
		}
		perm := os.FileMode(collectionFilePerms)
		if info, err := e.Info(); err == nil {
			perm = info.Mode().Perm()
		}
		dst := filepath.Join(to, e.Name())
		if err := writeFileAtomic(dst, data, perm); err != nil {
			return rollback(err)
		}
		written = append(written, dst)
		// writeFileAtomic honors the umask when creating a file, which is
		// right for a fresh write but wrong for a move: these files already
		// exist and their modes are the user's, so restore them exactly.
		if err := os.Chmod(dst, perm); err != nil {
			return rollback(fmt.Errorf("setting permissions on %s: %w", dst, err))
		}
		copied = append(copied, src)
	}

	for _, src := range copied {
		if err := os.Remove(src); err != nil {
			return len(copied), fmt.Errorf("removing %s after copying it: %w", src, err)
		}
	}
	// Only succeeds if nothing else was in there; anything left is not ours
	// to delete.
	_ = os.Remove(from)

	return len(copied), nil
}

// runMigrate moves the data directory to its XDG-preferred location.
func runMigrate() {
	if activeConfig.Preferred == "" {
		fmt.Printf("Nothing to migrate: disc-fortune is already using %s\n", activeConfig.Dir)
		return
	}

	from, to := activeConfig.Dir, activeConfig.Preferred
	moved, err := migrateConfig(from, to)
	if err != nil {
		fatal("Error migrating: %v", err)
	}

	noun := "files"
	if moved == 1 {
		noun = "file"
	}
	fmt.Printf("Moved %d %s from %s to %s\n", moved, noun, from, to)
}
