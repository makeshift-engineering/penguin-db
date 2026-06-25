package sstable

import (
	"errors"
	"math"
	"strconv"
)

var (
	// ErrKeyTooLarge is returned when the key exceeds the maximum representable
	// length (math.MaxUint16 bytes) in the on-disk data entry format.
	ErrKeyTooLarge = errors.New("sstable entry rejected: key length exceeds maximum of " + strconv.FormatUint(math.MaxUint16, 10) + " bytes")

	// ErrValueTooLarge is returned when the value exceeds the maximum
	// representable length (math.MaxUint32 bytes) in the on-disk data entry format.
	ErrValueTooLarge = errors.New("sstable entry rejected: value length exceeds maximum of " + strconv.FormatUint(math.MaxUint32, 10) + " bytes")

	// ErrInvalidMagic is returned when the SSTable footer does not contain the
	// expected magic number (0x50454E47 / "PENG"), indicating the file is not a
	// valid SSTable or has been truncated.
	ErrInvalidMagic = errors.New("sstable: invalid magic number")

	// ErrCorrupted is returned when structural invariants of the SSTable are
	// violated (e.g., offsets exceed file size, entry counts mismatch).
	ErrCorrupted = errors.New("sstable: data corruption detected")

	// ErrInvalidExpectedKeys is returned when NewWriter receives a negative
	// expectedKeys argument.
	ErrInvalidExpectedKeys = errors.New("sstable: expectedKeys must be non-negative")

	// ErrKeysOutOfOrder is returned when Add is called with a key that is
	// less than or equal to the previously added key.
	ErrKeysOutOfOrder = errors.New("sstable: keys must be added in strictly ascending order")

	// ErrReaderClosed is returned when a read operation is attempted on a
	// Reader whose underlying file has already been closed.
	ErrReaderClosed = errors.New("sstable: reader is closed")
)
