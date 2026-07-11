package memtable

import "errors"

var (
	// ErrMemTableFull is returned by Put or Delete when the incoming write
	// would cause the memtable's byte usage to exceed its configured maximum
	// size. The caller must flush the memtable and retry the operation.
	ErrMemTableFull = errors.New("memtable size limit exceeded")

	// ErrEmptyKey is returned by Put, Get, and Delete when the provided key
	// is nil or has a length of zero.
	ErrEmptyKey = errors.New("key cannot be empty or nil")
)
