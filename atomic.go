package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// writeFileAtomic writes data to path so that a reader never observes a
// partially-written file. It writes to a temp file in the same directory —
// rename is only atomic within a single filesystem — fsyncs it, then renames
// over the target.
//
// Every data file disc-fortune owns goes through here. history.json in
// particular is rewritten on every pick, so a crash mid-write is not a
// theoretical concern: a truncated history.json fails to parse, and every
// command that reads it (including the default pick) stays broken until
// someone deletes the file by hand.
//
// On any failure the temp file is removed and the pre-existing target is left
// byte-identical.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)

	f, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file in %s: %w", dir, err)
	}
	tmp := f.Name()

	// From here on every error path must remove the temp file.
	cleanup := func(cause error) error {
		f.Close()
		os.Remove(tmp)
		return cause
	}

	if _, err := f.Write(data); err != nil {
		return cleanup(fmt.Errorf("writing %s: %w", tmp, err))
	}
	// Durability: rename only orders the directory entry, it does not flush
	// the file's contents. Without this a crash can leave an empty file
	// under the real name.
	if err := f.Sync(); err != nil {
		return cleanup(fmt.Errorf("syncing %s: %w", tmp, err))
	}
	// CreateTemp always uses 0600; restore the caller's intended mode before
	// the file becomes visible under its real name.
	if err := f.Chmod(perm); err != nil {
		return cleanup(fmt.Errorf("setting permissions on %s: %w", tmp, err))
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("closing %s: %w", tmp, err)
	}

	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	return nil
}
