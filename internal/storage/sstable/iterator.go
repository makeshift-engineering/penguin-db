package sstable

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

// Default and maximum constraint limits for iterator buffers and key/value allocations.
const (
	DefaultIteratorBufferSize = 64 * 1024
	DefaultIteratorKeyCap     = 256
	DefaultIteratorValueCap   = 1024
	MaxIteratorBufferSize     = 16 * 1024 * 1024
	MaxIteratorKeyCap         = 65535
	MaxIteratorValueCap       = 32 * 1024 * 1024
)

// IteratorOptions holds configuration parameters for buffer size and initial key/value capacity sizing.
type IteratorOptions struct {
	BufferSize      int
	InitialKeyCap   int
	InitialValueCap int
}

// IteratorOption defines a functional option configuration function for IteratorOptions.
type IteratorOption func(*IteratorOptions)

// WithBufferSize configures a custom read buffer size for the underlying file reader.
func WithBufferSize(size int) IteratorOption {
	return func(option *IteratorOptions) {
		if size > MaxIteratorBufferSize {
			option.BufferSize = MaxIteratorBufferSize
		} else if size > 0 {
			option.BufferSize = size
		}
	}
}

// WithInitialCapacities sets custom pre-allocation capacities for key and value buffers to reduce allocations during iteration.
func WithInitialCapacities(keyCap, valueCap int) IteratorOption {
	return func(option *IteratorOptions) {
		if keyCap > 0 && keyCap <= MaxIteratorKeyCap {
			option.InitialKeyCap = keyCap
		}
		if valueCap > 0 && valueCap <= MaxIteratorValueCap {
			option.InitialValueCap = valueCap
		}
	}
}

// Iterator reads sequential data entries from an immutable SSTable file.
//
// Iterator uses a peek-based API: call Next() to advance, then check Valid()
// before accessing Key(), Value(), IsDeleted(), or Opcode(). This design allows
// Iterator to structurally satisfy the internalIterator interface in the storage
// package without requiring an adapter wrapper.
type Iterator struct {
	file        *os.File
	reader      *bufio.Reader
	limitOffset uint64
	currOffset  uint64
	key         []byte
	value       []byte
	opcode      uint8
	err         error
	closed      bool
	hasCurrent  bool
}

// NewIterator creates a new Iterator starting from the beginning of the SSTable file.
// It parses the footer metadata of the SSTable to establish the data boundaries
// and returns an error if the underlying file cannot be opened or is corrupted.
func NewIterator(filePath string, opts ...IteratorOption) (*Iterator, error) {
	config := &IteratorOptions{
		BufferSize:      DefaultIteratorBufferSize,
		InitialKeyCap:   DefaultIteratorKeyCap,
		InitialValueCap: DefaultIteratorValueCap,
	}

	for _, opt := range opts {
		opt(config)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file for iteration: %w", err)
	}

	success := false
	defer func() {
		if !success {
			file.Close()
		}
	}()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	fileSize := info.Size()

	if fileSize < int64(footerSize) {
		return nil, fmt.Errorf("%w: file too small for footer (%d bytes)", ErrCorrupted, fileSize)
	}

	var footer [footerSize]byte
	if _, err := file.ReadAt(footer[:], fileSize-int64(footerSize)); err != nil {
		return nil, fmt.Errorf("reading footer: %w", err)
	}

	magic := binary.LittleEndian.Uint32(footer[footerMagicOffset : footerMagicOffset+footerMagicSize])
	if magic != magicNumber {
		return nil, fmt.Errorf("%w: got 0x%08X, want 0x%08X", ErrInvalidMagic, magic, magicNumber)
	}

	indexOffset := binary.LittleEndian.Uint64(footer[footerIndexOffsetOffset:footerBloomOffsetOffset])
	bloomOffset := binary.LittleEndian.Uint64(footer[footerBloomOffsetOffset:footerBloomNumHashesOffset])
	entryCount := binary.LittleEndian.Uint32(footer[footerEntryCountOffset:footerMagicOffset])

	footerStart := uint64(fileSize) - uint64(footerSize)
	if indexOffset > footerStart || bloomOffset > footerStart || bloomOffset < indexOffset {
		return nil, fmt.Errorf("%w: invalid footer offsets in iterator initialization", ErrCorrupted)
	}

	if uint64(entryCount)*uint64(indexEntryHeaderSize) > bloomOffset-indexOffset {
		return nil, fmt.Errorf("%w: index block too small for %d entries", ErrCorrupted, entryCount)
	}

	if entryCount == 0 {
		if indexOffset != 0 {
			return nil, fmt.Errorf("%w: indexOffset must be 0 for empty SSTable", ErrCorrupted)
		}
	} else {
		// Detect downward-corrupted indexOffsets before trusting them as limitOffset.
		// A corrupted offset pointing to an earlier entry boundary in the data block
		// would be parsed here as an index entry, resulting in a non-zero data offset.
		var idxHeader [indexEntryHeaderSize]byte
		if _, err := file.ReadAt(idxHeader[:], int64(indexOffset)); err != nil {
			return nil, fmt.Errorf("reading first index entry: %w", err)
		}
		firstDataOffset := binary.LittleEndian.Uint64(idxHeader[indexOffsetOffset:indexKeyDataOffset])
		if firstDataOffset != 0 {
			return nil, fmt.Errorf("%w: structurally inconsistent index/data boundary", ErrCorrupted)
		}
	}

	success = true

	return &Iterator{
		file:        file,
		reader:      bufio.NewReaderSize(file, config.BufferSize),
		limitOffset: indexOffset,
		currOffset:  0,
		key:         make([]byte, 0, config.InitialKeyCap),
		value:       make([]byte, 0, config.InitialValueCap),
	}, nil
}

// Next advances the iterator to the next entry in the SSTable file.
// After calling Next, check Valid() before accessing Key/Value/IsDeleted/Opcode.
// If Valid() returns false after Next(), use Error() to distinguish between normal
// exhaustion (nil) and an I/O/corruption error.
func (iterator *Iterator) Next() {
	if iterator == nil || iterator.closed || iterator.err != nil {
		iterator.setInvalid()
		return
	}

	if iterator.currOffset >= iterator.limitOffset {
		iterator.setInvalid()
		return
	}

	var header [entryHeaderSize]byte
	if _, err := io.ReadFull(iterator.reader, header[:]); err != nil {
		if errors.Is(err, io.EOF) {
			iterator.err = io.ErrUnexpectedEOF
			iterator.hasCurrent = false
			return
		}
		iterator.err = fmt.Errorf("failed to read entry header at offset %d: %w", iterator.currOffset, err)
		iterator.hasCurrent = false
		return
	}

	keyLen := binary.LittleEndian.Uint16(header[keyLenOffset:valueLenOffset])
	valLen := binary.LittleEndian.Uint32(header[valueLenOffset:opcodeOffset])
	iterator.opcode = header[opcodeOffset]

	switch iterator.opcode {
	case OpcodePut, OpcodeDelete:
	default:
		iterator.err = fmt.Errorf("%w: invalid opcode %d at offset %d", ErrCorrupted, iterator.opcode, iterator.currOffset)
		iterator.hasCurrent = false
		return
	}

	if iterator.currOffset+uint64(entryHeaderSize)+uint64(keyLen)+uint64(valLen) > iterator.limitOffset {
		iterator.err = fmt.Errorf("%w: entry sizes exceed data block boundary", ErrCorrupted)
		iterator.hasCurrent = false
		return
	}

	if cap(iterator.key) < int(keyLen) {
		iterator.key = make([]byte, keyLen)
	}
	iterator.key = iterator.key[:keyLen]

	if cap(iterator.value) < int(valLen) {
		iterator.value = make([]byte, valLen)
	}
	iterator.value = iterator.value[:valLen]

	if keyLen > 0 {
		if _, err := io.ReadFull(iterator.reader, iterator.key); err != nil {
			iterator.err = fmt.Errorf("failed to read key at offset %d: %w", iterator.currOffset, err)
			iterator.hasCurrent = false
			return
		}
	}

	if valLen > 0 {
		if _, err := io.ReadFull(iterator.reader, iterator.value); err != nil {
			iterator.err = fmt.Errorf("failed to read value at offset %d: %w", iterator.currOffset, err)
			iterator.hasCurrent = false
			return
		}
	}

	iterator.currOffset += uint64(entryHeaderSize) + uint64(keyLen) + uint64(valLen)
	iterator.hasCurrent = true
}

// setInvalid marks the iterator as having no current entry.
// Safe to call on nil receivers.
func (iterator *Iterator) setInvalid() {
	if iterator != nil {
		iterator.hasCurrent = false
	}
}

// Valid reports whether the iterator is positioned on a valid entry.
// Returns false when the iterator has been exhausted, encountered an error, or is nil.
func (iterator *Iterator) Valid() bool {
	return iterator != nil && iterator.hasCurrent && iterator.err == nil
}

// Key returns the key of the current entry. The returned slice is valid until the next call to Next() or Close().
func (iterator *Iterator) Key() []byte {
	if iterator == nil {
		return nil
	}
	return iterator.key
}

// Value returns the value of the current entry. The returned slice is valid until the next call to Next() or Close().
func (iterator *Iterator) Value() []byte {
	if iterator == nil {
		return nil
	}
	return iterator.value
}

// IsDeleted reports whether the current entry is a tombstone (logically deleted).
func (iterator *Iterator) IsDeleted() bool {
	if iterator == nil {
		return false
	}
	return iterator.opcode == OpcodeDelete
}

// Opcode returns the operation code (Put/Delete) of the current entry.
func (iterator *Iterator) Opcode() uint8 {
	if iterator == nil {
		return 0
	}
	return iterator.opcode
}

// Error returns the first non-EOF error encountered by the iterator.
func (iterator *Iterator) Error() error {
	if iterator == nil {
		return nil
	}
	return iterator.err
}

// Close releases any system resources associated with the iterator.
func (iterator *Iterator) Close() {
	if iterator == nil || iterator.closed || iterator.file == nil {
		return
	}
	iterator.closed = true
	_ = iterator.file.Close()
}
