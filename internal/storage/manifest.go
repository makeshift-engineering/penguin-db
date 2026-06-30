package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Manifest represents the persistent database state metadata.
type Manifest struct {
	NextSegmentID int              `json:"next_segment_id"`
	Levels        map[int][]string `json:"levels"`
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
			return newManifest(), nil
		}
		return nil, err
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
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
		_ = os.Remove(tmpPath)
		return writeErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return closeErr
	}

	path := filepath.Join(dir, "manifest.json")
	return os.Rename(tmpPath, path)
}
