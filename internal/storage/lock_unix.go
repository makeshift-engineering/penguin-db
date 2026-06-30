//go:build !windows

package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type unixLock struct {
	file *os.File
}

func (l *unixLock) Close() error {
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	return l.file.Close()
}

func lockDirectory(dir string) (interface{ Close() error }, error) {
	lockPath := filepath.Join(dir, "LOCK")
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}

	// Apply exclusive non-blocking lock
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		closeErr := file.Close()
		if closeErr != nil {
			return nil, fmt.Errorf("failed to acquire exclusive LOCK on directory %s (already in use?): %w; additionally failed to close lock file: %v", dir, err, closeErr)
		}
		return nil, fmt.Errorf("failed to acquire exclusive LOCK on directory %s (already in use?): %w", dir, err)
	}

	return &unixLock{file: file}, nil
}
