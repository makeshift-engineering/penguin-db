// Package wal implements a Write-Ahead Log (WAL) for penguin-db.
// It supports log appending, rotation, record serialization, and replay recovery.
package wal

import (
	"errors"
	"math"
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
	ErrKeyTooLarge = errors.New("wal record rejected: key length exceeds maximum of " + uitoa(math.MaxUint16) + " bytes")

	// ErrFrameTooLarge is returned when the total serialized frame size exceeds
	// the maximum representable size (math.MaxUint32 bytes) in the on-disk format.
	ErrFrameTooLarge = errors.New("wal record rejected: frame size exceeds maximum of " + uitoa(math.MaxUint32) + " bytes")
)

// uitoa converts an unsigned integer to its decimal string representation.
// Used to embed numeric limits in error sentinel messages at init time without
// depending on strconv or fmt.
func uitoa(val uint64) string {
	if val == 0 {
		return "0"
	}
	var buf [20]byte // max uint64 is 20 digits
	i := len(buf)
	for val > 0 {
		i--
		buf[i] = byte(val%10) + '0'
		val /= 10
	}
	return string(buf[i:])
}
