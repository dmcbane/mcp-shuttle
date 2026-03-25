//go:build windows

package lockfile

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	modkernel32      = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = modkernel32.NewProc("LockFileEx")
	procUnlockFileEx = modkernel32.NewProc("UnlockFileEx")
)

const (
	// LOCKFILE_EXCLUSIVE_LOCK requests an exclusive lock.
	lockfileExclusiveLock = 0x00000002
	// LOCKFILE_FAIL_IMMEDIATELY returns immediately if the lock cannot be acquired.
	lockfileFailImmediately = 0x00000001
)

// tryLock attempts a non-blocking exclusive lock on the file using LockFileEx.
func tryLock(f *os.File) error {
	// Lock the first byte of the file (sufficient for advisory locking).
	ol := new(syscall.Overlapped)
	r1, _, err := procLockFileEx.Call(
		f.Fd(),
		uintptr(lockfileExclusiveLock|lockfileFailImmediately),
		0,       // reserved
		1,       // nNumberOfBytesToLockLow
		0,       // nNumberOfBytesToLockHigh
		uintptr(unsafe.Pointer(ol)),
	)
	if r1 == 0 {
		return err
	}
	return nil
}

// unlock releases the file lock using UnlockFileEx.
func unlock(f *os.File) {
	ol := new(syscall.Overlapped)
	procUnlockFileEx.Call(
		f.Fd(),
		0, // reserved
		1, // nNumberOfBytesToUnlockLow
		0, // nNumberOfBytesToUnlockHigh
		uintptr(unsafe.Pointer(ol)),
	)
}

// isProcessAlive checks whether a process with the given PID is still running.
func isProcessAlive(pid int) bool {
	const processQueryLimitedInformation = 0x1000
	handle, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)

	var exitCode uint32
	err = syscall.GetExitCodeProcess(handle, &exitCode)
	if err != nil {
		return false
	}
	// STILL_ACTIVE (259) means the process is still running.
	return exitCode == 259
}
