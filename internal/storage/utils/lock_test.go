package utils_test

import (
	"path/filepath"
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/storage/utils"
)

func TestLockDirectory(t *testing.T) {
	dir := t.TempDir()

	// First lock should succeed
	lock1, err := utils.LockDirectory(dir)
	if err != nil {
		t.Fatalf("expected to acquire lock, got error: %v", err)
	}

	// Second lock should fail (already locked)
	lock2, err := utils.LockDirectory(dir)
	if err == nil {
		lock2.Close()
		t.Fatalf("expected failure when locking an already locked directory, got success")
	}

	// Release first lock
	err = lock1.Close()
	if err != nil {
		t.Fatalf("failed to close lock1: %v", err)
	}

	// Third lock should succeed after the first one is released
	lock3, err := utils.LockDirectory(dir)
	if err != nil {
		t.Fatalf("expected to acquire lock after previous was closed, got error: %v", err)
	}
	lock3.Close()
}

func TestLockDirectory_NonExistentDir(t *testing.T) {
	dir := t.TempDir()
	nonExistentDir := filepath.Join(dir, "missing")

	lock, err := utils.LockDirectory(nonExistentDir)
	if err == nil {
		lock.Close()
		t.Fatalf("expected error when locking in non-existent directory")
	}
}

func TestLockDirectory_NulByte(t *testing.T) {
	_, err := utils.LockDirectory("dir\x00")
	if err == nil {
		t.Fatalf("expected error with NUL byte in directory path")
	}
}
