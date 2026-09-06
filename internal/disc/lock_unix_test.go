//go:build unix

package disc

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Two exclusive locks on one path must not be held at once. flock is per
// open file description, so separate os.OpenFile calls contend even inside a
// single process -- which is what makes this assertable without spawning
// subprocesses.
func TestWithFileLockSerializes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")

	var mu sync.Mutex
	inside, maxInside := 0, 0

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := withFileLock(path, func() error {
				mu.Lock()
				inside++
				if inside > maxInside {
					maxInside = inside
				}
				mu.Unlock()

				time.Sleep(2 * time.Millisecond)

				mu.Lock()
				inside--
				mu.Unlock()
				return nil
			})
			if err != nil {
				t.Errorf("withFileLock: %v", err)
			}
		}()
	}
	wg.Wait()

	if maxInside != 1 {
		t.Errorf("max concurrent holders = %d, want 1", maxInside)
	}
}
