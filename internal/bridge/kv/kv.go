// Package kv defines the abstract key-value interface that decouples the
// upper layers (catalog, row store) from the concrete storage engine.
//
// In a single-node deployment, [LocalKV] wraps [storage.Engine] directly.
// In the eventual distributed deployment, a Raft-backed implementation
// will satisfy the same interface, allowing the catalog and row store to
// work in both modes without changes.
package kv

import (
	"context"
	"errors"
)

// ErrKeyNotFound is returned by [KV.Get] when the requested key does not
// exist in the store. Implementations must map their engine-specific
// "not found" error to this sentinel so that callers have a stable error
// to compare against.
var ErrKeyNotFound = errors.New("kv: key not found")

// OpType distinguishes the two kinds of operations within a [WriteBatch].
type OpType byte

const (
	// OpPut writes a key-value pair to the store, creating or overwriting
	// any existing value for that key.
	OpPut OpType = 0x01

	// OpDelete removes a key from the store. The Value field is ignored.
	OpDelete OpType = 0x02
)

// Op represents a single operation within an atomic [WriteBatch].
type Op struct {
	Type  OpType
	Key   []byte
	Value []byte // nil for OpDelete
}

// Iterator provides a forward-only cursor over key-value pairs returned
// by a prefix scan. Callers must call [Iterator.Close] when finished to
// release any resources (e.g. read locks or snapshots) held by the
// iterator.
type Iterator interface {
	// Valid reports whether the iterator is positioned on a valid entry.
	// It returns false once the prefix range is exhausted or after the
	// iterator is closed.
	Valid() bool

	// Next advances the iterator and returns the current key-value pair.
	// Returns (nil, nil) when the iterator is no longer valid.
	Next() (key, value []byte)

	// Close releases all resources held by the iterator. After Close
	// returns, Valid() must return false and Next() must return (nil, nil).
	Close()
}

// KV is the storage interface consumed by the catalog and row store layers.
// Every method accepts a [context.Context] so that request cancellation,
// deadlines, and distributed tracing work end-to-end without signature
// changes when the distributed transport layer is added.
type KV interface {
	// Get retrieves the value associated with key. Returns
	// [ErrKeyNotFound] if the key does not exist.
	Get(ctx context.Context, key []byte) ([]byte, error)

	// Put writes a key-value pair, creating or overwriting any existing
	// value for the key.
	Put(ctx context.Context, key, value []byte) error

	// Delete removes a key from the store. Returns [ErrKeyNotFound] if
	// the key does not exist.
	Delete(ctx context.Context, key []byte) error

	// Scan returns a forward-only [Iterator] over all keys that share the
	// given prefix, yielding entries in lexicographic key order.
	Scan(ctx context.Context, prefix []byte) (Iterator, error)

	// WriteBatch atomically applies a slice of [Op] values. Either all
	// operations succeed or none are visible. This is the primitive used
	// by DDL operations that must commit multiple keys atomically.
	WriteBatch(ctx context.Context, ops []Op) error
}
