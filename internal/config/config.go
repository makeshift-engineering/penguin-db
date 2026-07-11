// Package config provides structures and utilities for loading, parsing,
// and holding runtime configuration settings for the Penguin-DB storage node server.
package config

import (
	"encoding/json"
	"os"
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
	return &Config{
		Server: ServerConfig{
			Port: 50051,
			Dir:  "./data",
		},
		Storage: StorageConfig{
			MemTable: MemTableConfig{
				MaxSizeBytes: 4 * 1024 * 1024,
				MaxLevel:     12,
				MaxImm:       2,
			},
			WAL: WALConfig{
				SegmentSizeBytes:      32 * 1024 * 1024,
				BatchSizeBytes:        4 * 1024 * 1024,
				IngestChannelCapacity: 10000,
			},
			Flush: FlushConfig{
				EstimatedKeys: 10000,
			},
			Compaction: CompactionConfig{
				Threshold:      4,
				ReadBufferSize:  1024 * 1024,
				EstimatedKeys:  100000,
				MaxSSTableSize: 2 * 1024 * 1024,
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
	return cfg, nil
}
