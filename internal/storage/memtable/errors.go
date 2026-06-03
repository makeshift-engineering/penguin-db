package memtable

import "errors"

var (
	ErrKeyNotFound  = errors.New("Key not found")
	ErrMemTableFull = errors.New("Memtable size limit exceeded")
)
