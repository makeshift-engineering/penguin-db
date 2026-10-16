package log

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	metaFileName = "meta.json"
	tempMetaFile = "meta.json.tmp"
)

type MetaState struct {
	CurrentTerm uint64 `json:"current_term"`
	VotedFor    string `json:"voted_for"`
}

type MetaStore struct {
	mu      sync.RWMutex
	dir     string
	state   MetaState
	fileDir string
}

func NewMetaStore(dir string) (*MetaStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create meta directory: %w", err)
	}

	ms := &MetaStore{
		dir: dir,
		state: MetaState{
			CurrentTerm: 0,
			VotedFor:    "",
		},
	}

	if err := ms.recover(); err != nil {
		return nil, err
	}

	return ms, nil
}

func (m *MetaStore) recover() error {
	metaPath := filepath.Join(m.dir, metaFileName)
	tempPath := filepath.Join(m.dir, tempMetaFile)

	_ = os.Remove(tempPath)

	data, err := os.ReadFile(metaPath)
	if os.IsNotExist(err) {
		return m.saveAtomic(m.state)
	}
	if err != nil {
		return fmt.Errorf("failed to read metadata file: %w", err)
	}

	var state MetaState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("corrupt metadata JSON: %w", err)
	}

	m.state = state
	return nil
}

func (m *MetaStore) Get() (uint64, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.CurrentTerm, m.state.VotedFor
}

func (m *MetaStore) Set(term uint64, votedFor string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	newState := MetaState{
		CurrentTerm: term,
		VotedFor:    votedFor,
	}

	if err := m.saveAtomic(newState); err != nil {
		return err
	}

	m.state = newState
	return nil
}

func (m *MetaStore) saveAtomic(state MetaState) error {
	data, err := json.MarshalIndent(state, "", " ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	tempPath := filepath.Join(m.dir, tempMetaFile)
	metaPath := filepath.Join(m.dir, metaFileName)

	file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("failed to create temp meta file: %w", err)
	}

	if _, err := file.Write(data); err != nil {
		file.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to write temp meta file: %w", err)
	}

	if err := file.Sync(); err != nil {
		file.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to fsync temp meta file: %w", err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close temp meta file: %w", err)
	}

	if err := os.Rename(tempPath, metaPath); err != nil {
		return fmt.Errorf("failed to rename temp meta file to %s: %w", metaPath, err)
	}
	return nil
}
