//go:build !windows

package utils

import "os"

// SyncDir durability syncs a directory to disk.
func SyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
