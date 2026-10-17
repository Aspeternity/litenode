package state

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// ErrLocked means another LiteNode instance holds the lock.
var ErrLocked = errors.New("another litenode instance is running")

// Lock is an exclusive advisory lock (flock) held for the whole menu
// session. The kernel releases it when the process exits or is killed, so a
// stale lock file never blocks later runs.
type Lock struct {
	f *os.File
}

// AcquireLock takes the lock at path without waiting. It returns ErrLocked
// if another process holds it.
func AcquireLock(path string) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	return &Lock{f: f}, nil
}

// Release drops the lock. The lock file is left in place on purpose:
// removing it would let two processes lock different files.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}
