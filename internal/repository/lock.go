package repository

import (
	"errors"
	"fmt"

	"github.com/gofrs/flock"
)

var ErrLocked = errors.New("another pxp execution holds the repository lock")

// Lock is a process-scoped advisory repository lock.
type Lock struct {
	file *flock.Flock
}

// AcquireLock attempts to acquire path without waiting.
func AcquireLock(path string) (*Lock, error) {
	file := flock.New(path, flock.SetPermissions(0o600))
	locked, err := file.TryLock()
	if err != nil {
		return nil, fmt.Errorf("acquire repository lock: %w", err)
	}
	if !locked {
		_ = file.Close()
		return nil, ErrLocked
	}
	return &Lock{file: file}, nil
}

// Release releases the advisory lock. The lock file intentionally remains in
// Git metadata; lock ownership is represented by the kernel lock, not by file
// existence.
func (l *Lock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	if err != nil {
		return fmt.Errorf("release repository lock: %w", err)
	}
	return nil
}
