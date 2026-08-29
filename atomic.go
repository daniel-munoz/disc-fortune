package main

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
)

// createTempFile creates a uniquely-named file in dir, opened with mode perm.
//
// os.CreateTemp would be the obvious choice, but it hardcodes 0600 and offers
// no way to ask for anything else. Passing perm to open(2) here is what lets
// the process umask filter it, exactly as it does for os.WriteFile.
func createTempFile(dir, prefix string, perm os.FileMode) (*os.File, error) {
	for attempt := 0; attempt < 1000; attempt++ {
		name := filepath.Join(dir, fmt.Sprintf("%s%d", prefix, rand.Uint32()))
		f, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, perm)
		if os.IsExist(err) {
			continue // name collision, try another
		}
		return f, err
	}
	return nil, fmt.Errorf("creating temp file in %s: no unused name found", dir)
}

// writeFileAtomic writes data to path so that a reader never observes a
// partially-written file. It writes to a temp file in the same directory --
// rename is only atomic within a single filesystem -- fsyncs it, then renames
// over the target.
//
// Every data file disc-fortune owns goes through here. history.json in
// particular is rewritten on every pick, so a crash mid-write is not a
// theoretical concern: a truncated history.json fails to parse, and every
// command that reads it (including the default pick) stays broken until
// someone deletes the file by hand.
//
// Permissions follow os.WriteFile, which this replaced, because anything else
// would be a silent regression for existing users:
//
//   - A file being created gets perm as filtered by the process umask. Someone
//     running umask 077 keeps their collection private.
//   - A file that already exists keeps the mode it already has. Someone who
//     tightened history.json does not get it widened again on the next pick.
//
// On any failure the temp file is removed and the pre-existing target is left
// byte-identical.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)

	// An existing target's mode wins over perm, and must be reproduced
	// exactly -- umask included, since the user may have widened it.
	mode, exact := perm, false
	if info, err := os.Stat(path); err == nil {
		mode, exact = info.Mode().Perm(), true
	}

	f, err := createTempFile(dir, filepath.Base(path)+".tmp-", mode)
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
	// open(2) applied the umask. That is what we want for a new file, but for
	// an existing one it may have narrowed a mode the user chose deliberately,
	// so restore it exactly. fchmod ignores the umask.
	if exact {
		if err := f.Chmod(mode); err != nil {
			return cleanup(fmt.Errorf("setting permissions on %s: %w", tmp, err))
		}
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
