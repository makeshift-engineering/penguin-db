// Package config provides structures and utilities for loading, parsing,
// and holding runtime configuration settings for the Penguin-DB storage node server.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/makeshift-engineering/penguin-db/internal/storage"
)

// ServerConfig stores the configurations related to the server network port
// and the underlying storage file directory.
type ServerConfig struct {
	Port int    `json:"port"`
	Dir  string `json:"dir"`
}

// Config represents the root configuration tree for the storage node daemon,
// containing separate sub-configurations for server and storage engine concerns.
type Config struct {
	Server  ServerConfig  `json:"server"`
	Storage StorageConfig `json:"storage"`
}

// DefaultConfig returns a new Config populated with sensible production-ready
// default values for all server and storage parameters.
func DefaultConfig() *Config {
	baseOpts := storage.DefaultOptions()
	return &Config{
		Server: ServerConfig{
			Port: 50051,
			Dir:  "./data",
		},
		Storage: StorageConfig{
			MemTable: MemTableConfig{
				MaxSizeBytes: baseOpts.MaxMemTableSize,
				MaxLevel:     baseOpts.MemTableMaxLevel,
				MaxImm:       baseOpts.MaxImmMemtables,
			},
			WAL: WALConfig{
				SegmentSizeBytes:      baseOpts.WALOptions.SegmentSizeBytes,
				BatchSizeBytes:        baseOpts.WALOptions.BatchSizeBytes,
				IngestChannelCapacity: baseOpts.WALOptions.IngestChannelCapacity,
			},
			Flush: FlushConfig{
				EstimatedKeys: baseOpts.FlushEstimatedKeys,
			},
			Compaction: CompactionConfig{
				Threshold:      baseOpts.CompactionThreshold,
				ReadBufferSize: baseOpts.CompactionReadBufferSize,
				EstimatedKeys:  baseOpts.CompactionEstimatedKeys,
				MaxSSTableSize: baseOpts.CompactionMaxSSTableSize,
			},
		},
	}
}

// LoadConfig opens a JSON file at the specified path, parses it, and returns
// a Config populated with its contents. Any unspecified fields fall back to
// DefaultConfig values.
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	dec := json.NewDecoder(file)
	if err := dec.Decode(cfg); err != nil {
		return nil, err
	}

	// Ensure no trailing garbage data exists after the config document
	var dummy json.RawMessage
	if err := dec.Decode(&dummy); err != io.EOF {
		if err == nil {
			return nil, errors.New("config file contains extra data after the configuration object")
		}
		return nil, fmt.Errorf("config file contains invalid trailing data: %w", err)
	}

	return cfg, nil
}
