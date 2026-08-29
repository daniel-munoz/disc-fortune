package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// tmpResidue returns the names of any leftover temp files the atomic writer
// may have abandoned in dir. A successful or failed write must leave none.
func tmpResidue(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading dir: %v", err)
	}
	var residue []string
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			residue = append(residue, e.Name())
		}
	}
	return residue
}

func TestWriteFileAtomicCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "collection.json")

	if err := writeFileAtomic(path, []byte("hello"), collectionFilePerms); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("content = %q, want %q", got, "hello")
	}
}

// withUmask sets the process umask for the duration of a test. It is
// process-global, so these tests must not run in parallel.
func withUmask(t *testing.T, mask int) {
	t.Helper()
	old := syscall.Umask(mask)
	t.Cleanup(func() { syscall.Umask(old) })
}

func TestWriteFileAtomicAppliesPerms(t *testing.T) {
	withUmask(t, 0o022)
	dir := t.TempDir()
	path := filepath.Join(dir, "collection.json")

	if err := writeFileAtomic(path, []byte("hello"), collectionFilePerms); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != collectionFilePerms {
		t.Errorf("perm = %o, want %o", got, collectionFilePerms)
	}
}

// writeFileAtomic replaced os.WriteFile, so it must reproduce os.WriteFile's
// permission semantics. The first of those is that perm is a request filtered
// by the process umask, not a mode imposed on the user: someone running with
// umask 077 expects their collection to stay private.
func TestWriteFileAtomicRespectsUmaskOnNewFiles(t *testing.T) {
	withUmask(t, 0o077)
	dir := t.TempDir()
	path := filepath.Join(dir, "collection.json")

	if err := writeFileAtomic(path, []byte("hello"), collectionFilePerms); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if want := os.FileMode(0o600); info.Mode().Perm() != want {
		t.Errorf("perm = %04o, want %04o (0644 filtered through umask 077)",
			info.Mode().Perm(), want)
	}
}

// The second half of os.WriteFile's semantics: perm applies only at creation.
// An existing file keeps whatever mode it has, so a user who tightened
// history.json does not have it widened again on the next pick.
func TestWriteFileAtomicPreservesAnExistingFilesMode(t *testing.T) {
	withUmask(t, 0o022)
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")
	if err := os.WriteFile(path, []byte("old"), collectionFilePerms); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	if err := writeFileAtomic(path, []byte("new"), collectionFilePerms); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if want := os.FileMode(0o600); info.Mode().Perm() != want {
		t.Errorf("perm = %04o, want %04o -- the user's own mode was overwritten",
			info.Mode().Perm(), want)
	}
}

// A widened mode must survive too: the rule is "leave it alone", not
// "tighten it".
func TestWriteFileAtomicPreservesAWidenedMode(t *testing.T) {
	withUmask(t, 0o077)
	dir := t.TempDir()
	path := filepath.Join(dir, "collection.json")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	if err := os.Chmod(path, 0o664); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	if err := writeFileAtomic(path, []byte("new"), collectionFilePerms); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if want := os.FileMode(0o664); info.Mode().Perm() != want {
		t.Errorf("perm = %04o, want %04o -- umask must not narrow an existing file",
			info.Mode().Perm(), want)
	}
}

func TestWriteFileAtomicOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "collection.json")
	if err := os.WriteFile(path, []byte("old"), collectionFilePerms); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	if err := writeFileAtomic(path, []byte("new"), collectionFilePerms); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if string(got) != "new" {
		t.Errorf("content = %q, want %q", got, "new")
	}
}

func TestWriteFileAtomicLeavesNoResidueOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "collection.json")

	if err := writeFileAtomic(path, []byte("hello"), collectionFilePerms); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	if residue := tmpResidue(t, dir); len(residue) != 0 {
		t.Errorf("temp files left behind: %v", residue)
	}
}

// The whole point of T1: a failed write must not damage what was already there.
// Renaming over an existing *directory* fails, which exercises the rename error
// path without needing to fake a filesystem.
func TestWriteFileAtomicPreservesOriginalOnRenameFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "collection.json")
	if err := os.Mkdir(path, configDirPerms); err != nil {
		t.Fatalf("seeding directory: %v", err)
	}

	err := writeFileAtomic(path, []byte("new"), collectionFilePerms)
	if err == nil {
		t.Fatal("writeFileAtomic succeeded renaming over a directory, want error")
	}

	info, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("target vanished after failed write: %v", statErr)
	}
	if !info.IsDir() {
		t.Error("target was replaced despite the write failing")
	}
	if residue := tmpResidue(t, dir); len(residue) != 0 {
		t.Errorf("temp files left behind after failure: %v", residue)
	}
}

func TestWriteFileAtomicPreservesOriginalWhenTempCannotBeCreated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "collection.json")
	if err := os.WriteFile(path, []byte("original"), collectionFilePerms); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	// A read-only directory blocks the temp file, standing in for a full disk.
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })

	if err := writeFileAtomic(path, []byte("new"), collectionFilePerms); err == nil {
		t.Fatal("writeFileAtomic succeeded in a read-only directory, want error")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if string(got) != "original" {
		t.Errorf("original was damaged: content = %q, want %q", got, "original")
	}
}

// inodeOf identifies the file occupying path. An atomic save replaces the
// target by rename, which yields a *new* inode; os.WriteFile truncates and
// rewrites the existing one, keeping it. That difference is exactly what makes
// the write crash-safe, so it is what these tests assert on.
func inodeOf(t *testing.T, path string) uint64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Skip("inode identity unavailable on this platform")
	}
	return uint64(st.Ino)
}

func TestSaveCollectionReplacesRatherThanTruncates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "collection.json")
	albums := []Album{{Artist: "Miles Davis", Title: "Kind of Blue"}}

	if err := saveCollectionTo(path, albums); err != nil {
		t.Fatalf("first save: %v", err)
	}
	before := inodeOf(t, path)

	if err := saveCollectionTo(path, append(albums, Album{Artist: "Sun Ra", Title: "Lanquidity"})); err != nil {
		t.Fatalf("second save: %v", err)
	}
	if after := inodeOf(t, path); after == before {
		t.Error("saveCollectionTo rewrote the file in place; it must rename a temp file over it")
	}
	if residue := tmpResidue(t, dir); len(residue) != 0 {
		t.Errorf("temp files left behind: %v", residue)
	}
}

func TestSaveHistoryReplacesRatherThanTruncates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")
	entries := []HistoryEntry{{Album: Album{Artist: "Alice Coltrane", Title: "Journey in Satchidananda"}, Timestamp: time.Now()}}

	if err := saveHistory(path, entries); err != nil {
		t.Fatalf("first save: %v", err)
	}
	before := inodeOf(t, path)

	if err := saveHistory(path, append(entries, entries[0])); err != nil {
		t.Fatalf("second save: %v", err)
	}
	if after := inodeOf(t, path); after == before {
		t.Error("saveHistory rewrote the file in place; it must rename a temp file over it")
	}
	if residue := tmpResidue(t, dir); len(residue) != 0 {
		t.Errorf("temp files left behind: %v", residue)
	}
}

func TestSaveFavoritesReplacesRatherThanTruncates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "favorites.json")
	albums := []Album{{Artist: "Pharoah Sanders", Title: "Karma"}}

	if err := saveFavorites(path, albums); err != nil {
		t.Fatalf("first save: %v", err)
	}
	before := inodeOf(t, path)

	if err := saveFavorites(path, append(albums, Album{Artist: "Don Cherry", Title: "Brown Rice"})); err != nil {
		t.Fatalf("second save: %v", err)
	}
	if after := inodeOf(t, path); after == before {
		t.Error("saveFavorites rewrote the file in place; it must rename a temp file over it")
	}
	if residue := tmpResidue(t, dir); len(residue) != 0 {
		t.Errorf("temp files left behind: %v", residue)
	}
}

// The roadmap's acceptance criterion is stated structurally — "no bare
// os.WriteFile to a live data path remains" — so it is guarded structurally.
// A new saver that forgets writeFileAtomic fails here even if it never gets
// its own round-trip test.
func TestNoDirectWriteFileInProductionCode(t *testing.T) {
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("globbing sources: %v", err)
	}
	for _, src := range sources {
		if strings.HasSuffix(src, "_test.go") {
			continue
		}
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("reading %s: %v", src, err)
		}
		if strings.Contains(string(data), "os.WriteFile(") {
			t.Errorf("%s calls os.WriteFile directly; data files must go through writeFileAtomic", src)
		}
	}
}
