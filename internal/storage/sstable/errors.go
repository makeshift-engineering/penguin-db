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
)
