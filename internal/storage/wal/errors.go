// Package wal implements a Write-Ahead Log (WAL) for penguin-db.
// It supports log appending, rotation, record serialization, and replay recovery.
package wal

import "errors"

var (
	// ErrInvalidCRC is returned when a record's checksum does not match its payload.
	ErrInvalidCRC = errors.New("corrupt wal record: crc32 mismatch")

	// ErrTruncated is returned when the log record has an unexpected size or EOF is reached mid-record.
	ErrTruncated  = errors.New("corrupt wal record: truncated payload")

	// ErrEmptyKey is returned when attempting to write a log record with a zero-length key.
	ErrEmptyKey   = errors.New("wal record rejected: key must not be empty")
)
