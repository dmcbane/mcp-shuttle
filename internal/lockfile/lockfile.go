package lockfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// LockInfo describes the process that holds the lock.
type LockInfo struct {
	PID       int       `json:"pid"`
	Port      int       `json:"port"`
	Timestamp time.Time `json:"timestamp"`
}

// Lock represents an advisory file lock used to coordinate OAuth flows
// across multiple mcp-shuttle instances.
type Lock struct {
	path string
	file *os.File
}

// Acquire tries to take the lock. Returns the Lock if successful,
// or the existing LockInfo if another process holds it.
func Acquire(dir string, name string) (*Lock, *LockInfo, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, nil, fmt.Errorf("creating lock directory: %w", err)
	}

	path := filepath.Join(dir, name+".lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, nil, fmt.Errorf("opening lock file: %w", err)
	}

	// Try non-blocking exclusive lock (platform-specific).
	err = tryLock(f)
	if err != nil {
		// Lock is held by another process. Read the info.
		var info LockInfo
		if decErr := json.NewDecoder(f).Decode(&info); decErr == nil {
			f.Close()
			return nil, &info, nil
		}
		f.Close()
		return nil, nil, fmt.Errorf("lock held by another process")
	}

	// We got the lock. Write our info.
	info := LockInfo{
		PID:       os.Getpid(),
		Timestamp: time.Now(),
	}
	f.Truncate(0)
	f.Seek(0, 0)
	json.NewEncoder(f).Encode(&info)

	return &Lock{path: path, file: f}, nil, nil
}

// SetPort updates the lock info with the callback server port.
func (l *Lock) SetPort(port int) error {
	info := LockInfo{
		PID:       os.Getpid(),
		Port:      port,
		Timestamp: time.Now(),
	}
	l.file.Truncate(0)
	l.file.Seek(0, 0)
	return json.NewEncoder(l.file).Encode(&info)
}

// Release releases the lock and removes the lock file.
func (l *Lock) Release() error {
	if l.file == nil {
		return nil
	}
	unlock(l.file)
	l.file.Close()
	os.Remove(l.path)
	l.file = nil
	return nil
}

// IsStale returns true if the lock info is older than maxAge or the
// holding process is no longer running.
func (info *LockInfo) IsStale(maxAge time.Duration) bool {
	if time.Since(info.Timestamp) > maxAge {
		return true
	}
	return !isProcessAlive(info.PID)
}
