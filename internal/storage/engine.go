package storage

// Engine defines the top-level interface for a storage engine that supports
// basic key-value operations.
type Engine interface {
	Put(key, value []byte) error
	Get(key []byte) ([]byte, error)
	Delete(key []byte) error
	Close() error
}
