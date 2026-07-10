package utils_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/storage/utils"
)

func TestSyncDir(t *testing.T) {
	dir := t.TempDir()

	// Sync an existing directory should succeed
	err := utils.SyncDir(dir)
	if err != nil {
		t.Fatalf("expected SyncDir to succeed, got error: %v", err)
	}
}

func TestSyncDir_NonExistentDir(t *testing.T) {
	dir := t.TempDir()
	nonExistentDir := filepath.Join(dir, "missing")

	err := utils.SyncDir(nonExistentDir)
	if runtime.GOOS == "windows" {
		// SyncDir is a no-op on Windows and returns nil.
		if err != nil {
			t.Fatalf("expected no error on Windows for SyncDir, got: %v", err)
		}
	} else {
		// SyncDir tries to open the directory on Unix, should fail if it doesn't exist.
		if err == nil {
			t.Fatalf("expected error on Unix for SyncDir on non-existent directory")
		}
	}
}
