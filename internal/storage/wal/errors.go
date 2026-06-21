// Package wal implements a Write-Ahead Log (WAL) for penguin-db.
// It supports log appending, rotation, record serialization, and replay recovery.
package wal

import (
	"errors"
	"math"
	"strconv"
)

var (
	// ErrInvalidCRC is returned when a record's checksum does not match its payload.
	ErrInvalidCRC = errors.New("corrupt wal record: crc32 mismatch")

	// ErrTruncated is returned when the log record has an unexpected size or EOF is reached mid-record.
	ErrTruncated = errors.New("corrupt wal record: truncated payload")

	// ErrEmptyKey is returned when attempting to write a log record with a zero-length key.
	ErrEmptyKey = errors.New("wal record rejected: key must not be empty")

	// ErrInvalidOpcode is returned when a record carries an opcode that is not
	// recognized by the WAL format. Persisting such a record would succeed but
	// the entry would be silently skipped during recovery replay.
	ErrInvalidOpcode = errors.New("wal record rejected: unrecognized opcode")

	// ErrKeyTooLarge is returned when the key exceeds the maximum representable
	// length (math.MaxUint16 bytes) in the on-disk frame format.
	ErrKeyTooLarge = errors.New("wal record rejected: key length exceeds maximum of " + strconv.FormatUint(math.MaxUint16, 10) + " bytes")

	// ErrFrameTooLarge is returned when the total serialized frame size exceeds
	// maxFrameSizeBytes (32 MiB, equal to MaxSegmentSizeBytes).
	ErrFrameTooLarge = errors.New("wal record rejected: frame size exceeds maximum of " + strconv.FormatUint(uint64(maxFrameSizeBytes), 10) + " bytes")

	// ErrWriterClosed is returned by Append when the LogWriter has already been
	// closed. Callers can detect this condition programmatically with errors.Is.
	ErrWriterClosed = errors.New("wal writer is closed")
)
