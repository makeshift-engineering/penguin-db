//go:build windows

package utils

import (
	"fmt"
	"path/filepath"
	"syscall"
)

type windowsLock struct {
	handle syscall.Handle
	path   string
}

func (l *windowsLock) Close() error {
	return syscall.CloseHandle(l.handle)
}

func LockDirectory(dir string) (interface{ Close() error }, error) {
	lockPath := filepath.Join(dir, "LOCK")
	pathPtr, err := syscall.UTF16PtrFromString(lockPath)
	if err != nil {
		return nil, err
	}

	handle, err := syscall.CreateFile(
		pathPtr,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0,
		nil,
		syscall.OPEN_ALWAYS,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire exclusive LOCK on directory %s (already in use?): %w", dir, err)
	}

	return &windowsLock{handle: handle, path: lockPath}, nil
}
