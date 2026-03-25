//go:build !windows

package lockfile

import (
	"os"
	"syscall"
)

// tryLock attempts a non-blocking exclusive lock on the file.
func tryLock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

// unlock releases the file lock.
func unlock(f *os.File) {
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}

// isProcessAlive checks whether a process with the given PID is still running.
func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds. Send signal 0 to check.
	return proc.Signal(syscall.Signal(0)) == nil
}
