package disc

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWithFileLockRunsFn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	ran := false
	if err := withFileLock(path, func() error {
		ran = true
		return nil
	}); err != nil {
		t.Fatalf("withFileLock: %v", err)
	}
	if !ran {
		t.Error("fn was not run")
	}
}

func TestWithFileLockPropagatesError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	sentinel := errors.New("boom")
	err := withFileLock(path, func() error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want the error fn returned", err)
	}
}

// The sidecar sits beside the data file, so the config directory has to exist
// before the lock can be taken -- including on the very first run, when
// nothing has been written yet.
func TestWithFileLockCreatesConfigDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh", "history.json")
	if err := withFileLock(path, func() error { return nil }); err != nil {
		t.Fatalf("withFileLock: %v", err)
	}
	if _, err := os.Stat(path + ".lock"); err != nil {
		t.Errorf("lock sidecar was not created: %v", err)
	}
}
