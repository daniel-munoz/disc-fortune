//go:build !unix

package disc

import "os"

// lockFD does nothing where flock is unavailable. disc-fortune still works
// there; it simply has no protection against two copies writing the same data
// file at the same moment. Every development and release target is unix, so
// this exists to keep `go build` honest elsewhere rather than to be relied on.
func lockFD(f *os.File) error { return nil }

func unlockFD(f *os.File) error { return nil }
