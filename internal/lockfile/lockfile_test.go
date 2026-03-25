package lockfile

import (
	"testing"
	"time"
)

func TestLock_AcquireAndRelease(t *testing.T) {
	dir := t.TempDir()

	lock, existing, err := Acquire(dir, "test")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if existing != nil {
		t.Fatal("expected no existing lock")
	}
	if lock == nil {
		t.Fatal("expected lock to be acquired")
	}

	if err := lock.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
}

func TestLock_DoubleAcquire(t *testing.T) {
	dir := t.TempDir()

	lock1, _, err := Acquire(dir, "test")
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	defer lock1.Release()

	// Second acquire should detect the held lock.
	lock2, existing, err := Acquire(dir, "test")
	if lock2 != nil {
		lock2.Release()
		t.Fatal("expected second acquire to fail")
	}
	// Either we get existing info or an error — both are acceptable.
	if existing == nil && err == nil {
		t.Fatal("expected either existing info or error")
	}
	if existing != nil {
		if existing.PID <= 0 {
			t.Errorf("expected valid PID, got %d", existing.PID)
		}
	}
}

func TestLock_ReleaseAndReacquire(t *testing.T) {
	dir := t.TempDir()

	lock1, _, _ := Acquire(dir, "test")
	lock1.Release()

	lock2, existing, err := Acquire(dir, "test")
	if err != nil {
		t.Fatalf("reacquire: %v", err)
	}
	if existing != nil {
		t.Fatal("expected no existing lock after release")
	}
	defer lock2.Release()
}

func TestLockInfo_IsStale(t *testing.T) {
	// A lock from a non-existent PID should be stale.
	info := &LockInfo{
		PID:       999999999, // unlikely to exist
		Timestamp: time.Now(),
	}
	if !info.IsStale(30 * time.Minute) {
		t.Error("expected lock with dead PID to be stale")
	}

	// A very old lock should be stale.
	info = &LockInfo{
		PID:       1, // init, always exists
		Timestamp: time.Now().Add(-time.Hour),
	}
	if !info.IsStale(30 * time.Minute) {
		t.Error("expected old lock to be stale")
	}
}
