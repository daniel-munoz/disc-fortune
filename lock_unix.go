//go:build unix

package main

import (
	"os"
	"syscall"
)

// lockFD takes an exclusive advisory lock, blocking until it is available.
//
// flock is preferred over an O_CREATE|O_EXCL sentinel file because the kernel
// releases it when the process exits: an interrupted run can never strand a
// lock that a later run would have to decide whether to break.
func lockFD(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

func unlockFD(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
