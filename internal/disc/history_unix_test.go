//go:build unix

package disc

import (
	"path/filepath"
	"strconv"
	"sync"
	"testing"
)

// Concurrent appends must not lose entries. This is the race the roadmap
// parked for this phase: it stopped being cosmetic when history started
// deciding what pick avoids.
//
// This relies on real file locking to pass, and lock_other.go's lockFD is a
// deliberate no-op on non-unix targets, so it is tagged unix like its
// sibling in lock_unix_test.go.
func TestAddToHistoryConcurrentAppendsDoNotLose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")

	const writers = 8
	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			album := Album{ReleaseID: i + 1, Artist: "A", Title: strconv.Itoa(i + 1)}
			if err := AddToHistory(path, album); err != nil {
				t.Errorf("AddToHistory: %v", err)
			}
		}()
	}
	wg.Wait()

	entries, err := LoadHistory(path)
	if err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}
	if len(entries) != writers {
		t.Errorf("history has %d entries, want %d", len(entries), writers)
	}
}
