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
}

// NewIterator creates a new Iterator starting from the beginning of the SSTable file.
// It returns an error if the parent Reader is already closed or if the underlying file cannot be opened.
func (reader *Reader) NewIterator(opts ...IteratorOption) (*Iterator, error) {
	if reader.closed {
		return nil, ErrReaderClosed
	}

	config := &IteratorOptions{
		BufferSize:      DefaultIteratorBufferSize,
		InitialKeyCap:   DefaultIteratorKeyCap,
		InitialValueCap: DefaultIteratorValueCap,
	}

	for _, opt := range opts {
		opt(config)
	}

	file, err := os.Open(reader.FilePath())
	if err != nil {
		return nil, fmt.Errorf("failed to open file for iteration: %w", err)
	}

	return &Iterator{
		file:        file,
		reader:      bufio.NewReaderSize(file, config.BufferSize),
		limitOffset: reader.indexOffset,
		currOffset:  0,
		key:         make([]byte, 0, config.InitialKeyCap),
		value:       make([]byte, 0, config.InitialValueCap),
	}, nil
}

// Next advances the iterator to the next entry in the SSTable file.
// It returns true if an entry was successfully read, and false if either the end of the data block
// was reached or an I/O/corruption error occurred. Use Error() to distinguish between the two.
func (iterator *Iterator) Next() bool {
	if iterator == nil || iterator.closed || iterator.err != nil {
		return false
	}

	if iterator.currOffset >= iterator.limitOffset {
		return false
	}

	var header [entryHeaderSize]byte
	if _, err := io.ReadFull(iterator.reader, header[:]); err != nil {
		if errors.Is(err, io.EOF) {
			iterator.err = io.ErrUnexpectedEOF
			return false
		}
		iterator.err = fmt.Errorf("failed to read entry header at offset %d: %w", iterator.currOffset, err)
		return false
	}

	keyLen := binary.LittleEndian.Uint16(header[keyLenOffset:valueLenOffset])
	valLen := binary.LittleEndian.Uint32(header[valueLenOffset:opcodeOffset])
	iterator.opcode = header[opcodeOffset]

	switch iterator.opcode {
	case OpcodePut, OpcodeDelete:
	default:
		iterator.err = fmt.Errorf("%w: invalid opcode %d at offset %d", ErrCorrupted, iterator.opcode, iterator.currOffset)
		return false
	}

	if iterator.currOffset+uint64(entryHeaderSize)+uint64(keyLen)+uint64(valLen) > iterator.limitOffset {
		iterator.err = fmt.Errorf("%w: entry sizes exceed data block boundary", ErrCorrupted)
		return false
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
			return false
		}
	}

	if valLen > 0 {
		if _, err := io.ReadFull(iterator.reader, iterator.value); err != nil {
			iterator.err = fmt.Errorf("failed to read value at offset %d: %w", iterator.currOffset, err)
			return false
		}
	}

	iterator.currOffset += uint64(entryHeaderSize) + uint64(keyLen) + uint64(valLen)
	return true
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
func (iterator *Iterator) Close() error {
	if iterator == nil || iterator.closed || iterator.file == nil {
		return nil
	}
	iterator.closed = true
	return iterator.file.Close()
}
