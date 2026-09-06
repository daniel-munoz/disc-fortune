package disc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// lockFilePerms is what a lock sidecar is created with. It never holds data;
// only its existence and the kernel's lock on it matter.
const lockFilePerms = 0644

// withFileLock runs fn while holding an exclusive advisory lock on path, so a
// read-modify-write of a data file cannot interleave with another process
// doing the same thing. `sync`'s backfill rewrites history.json and
// favorites.json wholesale while a concurrent `pick` or `favorite` is
// appending to them; without this, one of the two writes is simply lost. That
// was cosmetic while history was only a log, and is not any more: a lost entry
// now changes which records the next pick will avoid.
//
// The lock is taken on a `<path>.lock` sidecar rather than on the data file,
// because every write replaces that file by rename: a lock held on the old
// inode would be invisible to the next process to open the path.
//
// Callers must not nest. Two exclusive locks on one path, through two file
// descriptors, deadlock even inside a single process, so withFileLock belongs
// at the outermost layer of a read-modify-write and never inside another one.
func withFileLock(path string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), DirPerms); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, lockFilePerms)
	if err != nil {
		return fmt.Errorf("opening lock file: %w", err)
	}
	defer f.Close()

	if err := lockFD(f); err != nil {
		return fmt.Errorf("locking %s: %w", filepath.Base(path), err)
	}
	defer func() { _ = unlockFD(f) }()

	return fn()
}

// isLockSidecar reports whether name is one of the lock files withFileLock
// creates. Anything enumerating the config directory has to skip them.
func isLockSidecar(name string) bool {
	return strings.HasSuffix(name, ".lock")
}
