//go:build windows

package utils

// SyncDir durability syncs a directory to disk.
func SyncDir(dir string) error {
	// Directory sync is not needed/supported on Windows
	return nil
}
