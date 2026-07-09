package storage

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/makeshift-engineering/penguin-db/internal/storage/utils"
)

// Manifest represents the persistent database state metadata.
type Manifest struct {
	NextSegmentID    int              `json:"next_segment_id"`
	Levels           map[int][]string `json:"levels"`
	FlushedSegmentID int              `json:"flushed_segment_id,omitempty"`
}

// newManifest creates a default initial manifest structure.
func newManifest() *Manifest {
	return &Manifest{
		NextSegmentID: 1,
		Levels:        make(map[int][]string),
	}
}

// loadManifest reads the manifest file from the specified base directory.
// If the file does not exist, it returns a new empty manifest.
func loadManifest(dir string) (*Manifest, error) {
	path := filepath.Join(dir, "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Fallback: try loading manifest from backup if main is missing but backup exists
			backupPath := filepath.Join(dir, "manifest.backup.json")
			backupData, backupErr := os.ReadFile(backupPath)
			if backupErr == nil {
				var m Manifest
				if unmarshalErr := json.Unmarshal(backupData, &m); unmarshalErr != nil {
					return nil, fmt.Errorf("main manifest missing and backup manifest corrupt: %w", unmarshalErr)
				}
				if m.Levels == nil {
					m.Levels = make(map[int][]string)
				}
				return &m, nil
			}
			if !os.IsNotExist(backupErr) {
				return nil, fmt.Errorf("main manifest missing and backup manifest unavailable: %w", backupErr)
			}
			return newManifest(), nil
		}
		return nil, err
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		// Main manifest file is corrupt! Fall back to backup manifest.
		backupPath := filepath.Join(dir, "manifest.backup.json")
		backupData, backupErr := os.ReadFile(backupPath)
		if backupErr != nil {
			return nil, fmt.Errorf("main manifest corrupt and backup unavailable: %w", err)
		}
		var backupM Manifest
		if backupUnmarshalErr := json.Unmarshal(backupData, &backupM); backupUnmarshalErr != nil {
			return nil, fmt.Errorf("both main and backup manifests corrupt: %w", backupUnmarshalErr)
		}
		if backupM.Levels == nil {
			backupM.Levels = make(map[int][]string)
		}
		return &backupM, nil
	}
	if m.Levels == nil {
		m.Levels = make(map[int][]string)
	}
	return &m, nil
}

// writeManifest marshals and atomically writes the manifest to disk in base dir.
func writeManifest(dir string, m *Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := filepath.Join(dir, "manifest.tmp")
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	var writeErr error
	if _, err = file.Write(data); err != nil {
		writeErr = err
	} else if err = file.Sync(); err != nil {
		writeErr = err
	}

	closeErr := file.Close()
	if writeErr != nil {
		if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
			slog.Warn("failed to clean up temp manifest file after write error", "path", tmpPath, "error", err)
		}
		return writeErr
	}
	if closeErr != nil {
		if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
			slog.Warn("failed to clean up temp manifest file after close error", "path", tmpPath, "error", err)
		}
		return closeErr
	}

	path := filepath.Join(dir, "manifest.json")
	backupPath := filepath.Join(dir, "manifest.backup.json")

	// Backup existing manifest before overwriting
	if _, statErr := os.Stat(path); statErr == nil {
		if err := copyFile(path, backupPath); err != nil {
			slog.Warn("failed to backup manifest file", "error", err)
		}
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}

	return utils.SyncDir(dir)
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
