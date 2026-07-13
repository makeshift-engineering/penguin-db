package config

import "github.com/makeshift-engineering/penguin-db/internal/storage"

// MemTableConfig holds parameters for configuring the database active and immutable memtables.
type MemTableConfig struct {
	MaxSizeBytes int64 `json:"max_size_bytes"`
	MaxLevel     int   `json:"max_level"`
	MaxImm       int   `json:"max_imm"`
}

// WALConfig holds parameters for configuring the write-ahead log writer limits.
type WALConfig struct {
	SegmentSizeBytes      int64 `json:"segment_size_bytes"`
	BatchSizeBytes        int64 `json:"batch_size_bytes"`
	IngestChannelCapacity int   `json:"ingest_channel_capacity"`
}

// FlushConfig holds parameters for tuning skip-list to SSTable flush runs.
type FlushConfig struct {
	EstimatedKeys int `json:"estimated_keys"`
}

// CompactionConfig holds parameters for tuning L0 to L1 background merge comapctions.
type CompactionConfig struct {
	Threshold      int    `json:"threshold"`
	ReadBufferSize int    `json:"read_buffer_size"`
	EstimatedKeys  int    `json:"estimated_keys"`
	MaxSSTableSize uint64 `json:"max_sstable_size"`
}

// StorageConfig wraps all nested configuration structures for the LSM storage engine.
type StorageConfig struct {
	MemTable   MemTableConfig   `json:"memtable"`
	WAL        WALConfig        `json:"wal"`
	Flush      FlushConfig      `json:"flush"`
	Compaction CompactionConfig `json:"compaction"`
}

// ToStorageOptions converts this config to a storage.Options struct,
// falling back to storage.DefaultOptions() for any zero-value field.
func (c *StorageConfig) ToStorageOptions() storage.Options {
	base := storage.DefaultOptions()
	if c.MemTable.MaxSizeBytes > 0 {
		base.MaxMemTableSize = c.MemTable.MaxSizeBytes
	}
	if c.MemTable.MaxLevel > 0 {
		base.MemTableMaxLevel = c.MemTable.MaxLevel
	}
	if c.MemTable.MaxImm > 0 {
		base.MaxImmMemtables = c.MemTable.MaxImm
	}
	if c.WAL.SegmentSizeBytes > 0 {
		base.WALOptions.SegmentSizeBytes = c.WAL.SegmentSizeBytes
	}
	if c.WAL.BatchSizeBytes > 0 {
		base.WALOptions.BatchSizeBytes = c.WAL.BatchSizeBytes
	}
	if c.WAL.IngestChannelCapacity > 0 {
		base.WALOptions.IngestChannelCapacity = c.WAL.IngestChannelCapacity
	}
	if c.Flush.EstimatedKeys > 0 {
		base.FlushEstimatedKeys = c.Flush.EstimatedKeys
	}
	if c.Compaction.Threshold > 0 {
		base.CompactionThreshold = c.Compaction.Threshold
	}
	if c.Compaction.ReadBufferSize > 0 {
		base.CompactionReadBufferSize = c.Compaction.ReadBufferSize
	}
	if c.Compaction.EstimatedKeys > 0 {
		base.CompactionEstimatedKeys = c.Compaction.EstimatedKeys
	}
	if c.Compaction.MaxSSTableSize > 0 {
		base.CompactionMaxSSTableSize = c.Compaction.MaxSSTableSize
	}
	return base
}
