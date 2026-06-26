package sstable

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	DefaultIteratorBufferSize = 64 * 1024
	DefaultIteratorKeyCap     = 256
	DefaultIteratorValueCap   = 1024
	MaxIteratorBufferSize     = 16 * 1024 * 1024
	MaxIteratorKeyCap         = 65535
	MaxIteratorValueCap       = 32 * 1024 * 1024
)

type IteratorOptions struct {
	BufferSize      int
	InitialKeyCap   int
	InitialValueCap int
}

type IteratorOption func(*IteratorOptions)

func WithBufferSize(size int) IteratorOption {
	return func(option *IteratorOptions) {
		if size > 0 && size <= MaxIteratorBufferSize {
			option.BufferSize = size
		}
	}
}

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
			return false
		}
		iterator.err = fmt.Errorf("failed to read entry header at offset %d: %w", iterator.currOffset, err)
		return false
	}

	keyLen := binary.LittleEndian.Uint16(header[keyLenOffset:valueLenOffset])
	valLen := binary.LittleEndian.Uint32(header[valueLenOffset:opcodeOffset])
	iterator.opcode = header[opcodeOffset]

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

func (iterator *Iterator) Key() []byte {
	if iterator == nil {
		return nil
	}
	return iterator.key
}

func (iterator *Iterator) Value() []byte {
	if iterator == nil {
		return nil
	}
	return iterator.value
}

func (iterator *Iterator) Opcode() uint8 {
	if iterator == nil {
		return 0
	}
	return iterator.opcode
}

func (iterator *Iterator) Error() error {
	if iterator == nil {
		return nil
	}
	return iterator.err
}

func (iterator *Iterator) Close() error {
	if iterator == nil || iterator.closed || iterator.file == nil {
		return nil
	}
	iterator.closed = true
	return iterator.file.Close()
}
