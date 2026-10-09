package sstable

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"sync"
	"sync/atomic"
)

// Reader reads entries from an immutable SSTable file. On Open, the footer,
// index block, and bloom filter are loaded into memory. The data block remains
// on disk and is accessed via seek+read when Get finds a candidate in the
// in-memory index.
type Reader struct {
	file        *os.File
	bloomFilter *BloomFilter
	index       []indexEntry
	maxKey      []byte
	maxKeyOnce  sync.Once
	entryCount  uint32
	fileSize    int64
	indexOffset uint64
	closed      int32
}

// Open opens an SSTable file for reading. It reads the footer, validates the
// magic number, and loads the Index Block and Bloom Block into memory.
func Open(filePath string) (*Reader, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}

	// Ensure the file is closed if we bail out at any point below.
	// On success we set success=true and the defer becomes a no-op,
	// transferring ownership of the file handle to the returned Reader.
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

	// Read the footer (last 25 bytes).
	var footer [footerSize]byte
	if _, err := file.ReadAt(footer[:], fileSize-int64(footerSize)); err != nil {
		return nil, fmt.Errorf("reading footer: %w", err)
	}

	// Validate magic number.
	magic := binary.LittleEndian.Uint32(footer[footerMagicOffset : footerMagicOffset+footerMagicSize])
	if magic != magicNumber {
		return nil, fmt.Errorf("%w: got 0x%08X, want 0x%08X", ErrInvalidMagic, magic, magicNumber)
	}

	indexOffset := binary.LittleEndian.Uint64(footer[footerIndexOffsetOffset:footerBloomOffsetOffset])
	bloomOffset := binary.LittleEndian.Uint64(footer[footerBloomOffsetOffset:footerBloomNumHashesOffset])
	bloomNumHashes := footer[footerBloomNumHashesOffset]
	entryCount := binary.LittleEndian.Uint32(footer[footerEntryCountOffset:footerMagicOffset])

	footerStart := uint64(fileSize) - uint64(footerSize)

	// Validate offsets are within bounds and ordered correctly.
	if indexOffset > footerStart {
		return nil, fmt.Errorf("%w: index offset %d exceeds footer start %d", ErrCorrupted, indexOffset, footerStart)
	}
	if bloomOffset > footerStart {
		return nil, fmt.Errorf("%w: bloom offset %d exceeds footer start %d", ErrCorrupted, bloomOffset, footerStart)
	}
	if bloomOffset < indexOffset {
		return nil, fmt.Errorf("%w: bloom offset %d precedes index offset %d", ErrCorrupted, bloomOffset, indexOffset)
	}

	if entryCount > 0 && uint64(entryCount)*uint64(entryHeaderSize) > indexOffset {
		return nil, fmt.Errorf("%w: entry count %d is impossible for data block size %d bytes",
			ErrCorrupted, entryCount, indexOffset)
	}

	// Load the Index Block.
	indexSize := bloomOffset - indexOffset
	indexData := make([]byte, indexSize)
	if indexSize > 0 {
		if _, err := file.ReadAt(indexData, int64(indexOffset)); err != nil {
			return nil, fmt.Errorf("reading index block: %w", err)
		}
	}

	index, err := parseIndex(indexData, entryCount, indexOffset)
	if err != nil {
		return nil, err
	}

	// Load the Bloom Block.
	bloomSize := footerStart - bloomOffset
	bloomData := make([]byte, bloomSize)
	if bloomSize > 0 {
		if _, err := file.ReadAt(bloomData, int64(bloomOffset)); err != nil {
			return nil, fmt.Errorf("reading bloom block: %w", err)
		}
	}

	bloomFilter := NewBloomFilterFromBytes(bloomData, bloomNumHashes)

	success = true

	return &Reader{
		file:        file,
		bloomFilter: bloomFilter,
		index:       index,
		entryCount:  entryCount,
		fileSize:    fileSize,
		indexOffset: indexOffset,
	}, nil
}

// parseIndex decodes the raw index block bytes into a slice of indexEntry.
func parseIndex(data []byte, expectedCount uint32, indexOffset uint64) ([]indexEntry, error) {
	maxPossibleEntries := len(data) / indexEntryHeaderSize
	entries := make([]indexEntry, 0, maxPossibleEntries)
	pos := 0

	for pos < len(data) {
		if pos+indexEntryHeaderSize > len(data) {
			return nil, fmt.Errorf("%w: truncated index entry at offset %d", ErrCorrupted, pos)
		}

		keyLen := binary.LittleEndian.Uint16(data[pos : pos+indexKeyLenSize])
		pos += indexKeyLenSize

		offset := binary.LittleEndian.Uint64(data[pos : pos+indexOffsetSize])
		pos += indexOffsetSize

		if offset > indexOffset || indexOffset-offset < uint64(entryHeaderSize) {
			return nil, fmt.Errorf("%w: offset %d exceeds data block boundary", ErrCorrupted, offset)
		}

		if pos+int(keyLen) > len(data) {
			return nil, fmt.Errorf("%w: truncated index key at offset %d (keyLen=%d)", ErrCorrupted, pos, keyLen)
		}

		key := make([]byte, keyLen)
		copy(key, data[pos:pos+int(keyLen)])
		pos += int(keyLen)

		entries = append(entries, indexEntry{
			key:    key,
			offset: offset,
		})
	}

	if expectedCount == 0 {
		if len(entries) > 0 {
			return nil, fmt.Errorf("%w: index has %d entries, footer says 0 entries", ErrCorrupted, len(entries))
		}
	} else {
		if len(entries) == 0 {
			return nil, fmt.Errorf("%w: index has 0 entries, footer says %d entries", ErrCorrupted, expectedCount)
		}
		if uint32(len(entries)) > expectedCount {
			return nil, fmt.Errorf("%w: index has %d entries, footer says %d entries", ErrCorrupted, len(entries), expectedCount)
		}
		if entries[0].offset != 0 {
			return nil, fmt.Errorf("%w: first index entry offset %d must be 0", ErrCorrupted, entries[0].offset)
		}
	}

	return entries, nil
}

// BloomMayContain returns true if the key might exist in this SSTable
// according to the Bloom Filter. A false return guarantees the key is absent.
func (r *Reader) BloomMayContain(key []byte) bool {
	if atomic.LoadInt32(&r.closed) != 0 {
		return false
	}
	return r.bloomFilter.MayContain(key)
}

// Get searches the SSTable for the given key using binary search over the
// in-memory sparse index to locate the candidate data block, then scans
// the block sequentially from disk.
//
// Returns:
//   - (value, true, false, nil)  : key found with a live value
//   - (nil,   true, true,  nil)  : key found but it is a tombstone (deleted)
//   - (nil,   false, false, nil) : key not present in this SSTable
//   - (nil,   false, false, err) : an I/O or corruption error occurred
func (r *Reader) Get(key []byte) (value []byte, found, deleted bool, err error) {
	if atomic.LoadInt32(&r.closed) != 0 {
		return nil, false, false, ErrReaderClosed
	}

	if len(r.index) == 0 {
		return nil, false, false, nil
	}

	// Binary search over the sorted sparse index for the first entry with key > target key.
	idx := sort.Search(len(r.index), func(i int) bool {
		return bytes.Compare(r.index[i].key, key) > 0
	})

	if idx == 0 {
		return nil, false, false, nil
	}

	startOffset := r.index[idx-1].offset
	var endOffset uint64
	if idx < len(r.index) {
		endOffset = r.index[idx].offset
	} else {
		endOffset = r.indexOffset
	}

	if startOffset > uint64(math.MaxInt64) || endOffset > uint64(math.MaxInt64) {
		return nil, false, false, fmt.Errorf("%w: index entry offset exceeds max int64", ErrCorrupted)
	}

	if startOffset >= r.indexOffset || endOffset > r.indexOffset || startOffset >= endOffset {
		return nil, false, false, fmt.Errorf("%w: invalid block range [%d, %d)", ErrCorrupted, startOffset, endOffset)
	}

	blockSize := endOffset - startOffset
	blockBuf := make([]byte, blockSize)
	if _, err := r.file.ReadAt(blockBuf, int64(startOffset)); err != nil {
		return nil, false, false, fmt.Errorf("reading data block [%d, %d): %w", startOffset, endOffset, err)
	}

	pos := 0
	for pos < len(blockBuf) {
		if pos+entryHeaderSize > len(blockBuf) {
			return nil, false, false, fmt.Errorf("%w: truncated entry header in block at offset %d", ErrCorrupted, startOffset+uint64(pos))
		}

		keyLen := uint64(binary.LittleEndian.Uint16(blockBuf[pos+keyLenOffset : pos+valueLenOffset]))
		valLen := uint64(binary.LittleEndian.Uint32(blockBuf[pos+valueLenOffset : pos+opcodeOffset]))
		opcode := blockBuf[pos+opcodeOffset]

		if uint64(pos)+uint64(entryHeaderSize)+keyLen+valLen > uint64(len(blockBuf)) {
			return nil, false, false, fmt.Errorf("%w: entry sizes exceed block boundary at offset %d", ErrCorrupted, startOffset+uint64(pos))
		}

		entryKey := blockBuf[pos+entryHeaderSize : pos+entryHeaderSize+int(keyLen)]
		cmp := bytes.Compare(entryKey, key)
		if cmp == 0 {
			if opcode == OpcodeDelete {
				return nil, true, true, nil
			} else if opcode != OpcodePut {
				return nil, false, false, fmt.Errorf("%w: unknown opcode %d at offset %d", ErrCorrupted, opcode, startOffset+uint64(pos))
			}
			val := make([]byte, valLen)
			copy(val, blockBuf[pos+entryHeaderSize+int(keyLen):pos+entryHeaderSize+int(keyLen)+int(valLen)])
			return val, true, false, nil
		} else if cmp > 0 {
			return nil, false, false, nil
		}

		pos += entryHeaderSize + int(keyLen) + int(valLen)
	}

	return nil, false, false, nil
}

// EntryCount returns the total number of entries in this SSTable as recorded in the footer.
func (r *Reader) EntryCount() uint32 {
	return r.entryCount
}

// Close closes the underlying file handle. After Close, all read operations will return ErrReaderClosed.
func (r *Reader) Close() error {
	if !atomic.CompareAndSwapInt32(&r.closed, 0, 1) {
		return nil
	}
	return r.file.Close()
}

// FilePath returns the path of the underlying SSTable file.
func (r *Reader) FilePath() string {
	if r.file == nil {
		return ""
	}
	return r.file.Name()
}

// MinKey returns the smallest key in this SSTable.
func (r *Reader) MinKey() []byte {
	if len(r.index) == 0 {
		return nil
	}
	k := make([]byte, len(r.index[0].key))
	copy(k, r.index[0].key)
	return k
}

// MaxKey returns the largest key in this SSTable.
func (r *Reader) MaxKey() []byte {
	if atomic.LoadInt32(&r.closed) != 0 {
		return nil
	}
	r.maxKeyOnce.Do(func() {
		if r.entryCount == 0 || len(r.index) == 0 {
			return
		}
		lastBlockOffset := r.index[len(r.index)-1].offset
		curr := lastBlockOffset
		for curr < r.indexOffset {
			var header [entryHeaderSize]byte
			if _, err := r.file.ReadAt(header[:], int64(curr)); err != nil {
				return
			}
			keyLen := uint64(binary.LittleEndian.Uint16(header[keyLenOffset:valueLenOffset]))
			valLen := uint64(binary.LittleEndian.Uint32(header[valueLenOffset:opcodeOffset]))
			entryLen := uint64(entryHeaderSize) + keyLen + valLen

			if curr+entryLen > r.indexOffset {
				return
			}

			if curr+entryLen == r.indexOffset {
				buf := make([]byte, keyLen)
				if keyLen > 0 {
					if _, err := r.file.ReadAt(buf, int64(curr+uint64(entryHeaderSize))); err != nil {
						return
					}
				}
				r.maxKey = buf
				break
			}
			curr += entryLen
		}
	})

	if len(r.maxKey) == 0 && r.entryCount == 0 {
		return nil
	}
	k := make([]byte, len(r.maxKey))
	copy(k, r.maxKey)
	return k
}

// findOffsetAtOrAfter scans forward from startOffset in the data block to locate
// the exact byte offset of the first entry with a key greater than or equal to startKey.
func (r *Reader) findOffsetAtOrAfter(startOffset uint64, startKey []byte) (uint64, error) {
	curr := startOffset
	for curr < r.indexOffset {
		var header [entryHeaderSize]byte
		if _, err := r.file.ReadAt(header[:], int64(curr)); err != nil {
			return 0, fmt.Errorf("%w: failed to read entry header at offset %d: %w", ErrCorrupted, curr, err)
		}

		keyLen := uint64(binary.LittleEndian.Uint16(header[keyLenOffset:valueLenOffset]))
		valLen := uint64(binary.LittleEndian.Uint32(header[valueLenOffset:opcodeOffset]))
		entryLen := uint64(entryHeaderSize) + keyLen + valLen

		if curr+entryLen > r.indexOffset {
			return 0, fmt.Errorf("%w: entry at offset %d exceeds data block boundary", ErrCorrupted, curr)
		}

		keyBuf := make([]byte, keyLen)
		if keyLen > 0 {
			if _, err := r.file.ReadAt(keyBuf, int64(curr+uint64(entryHeaderSize))); err != nil {
				return 0, fmt.Errorf("%w: failed to read key at offset %d: %w", ErrCorrupted, curr, err)
			}
		}

		if bytes.Compare(keyBuf, startKey) >= 0 {
			return curr, nil
		}

		curr += entryLen
	}
	return r.indexOffset, nil
}

// NewIteratorAt creates a new Iterator positioned at the first key greater than or equal to startKey.
// It uses binary search on the reader's sparse index to locate the candidate data block interval,
// then scans forward to the target key position.
func (r *Reader) NewIteratorAt(startKey []byte, opts ...IteratorOption) (*Iterator, error) {
	if atomic.LoadInt32(&r.closed) != 0 {
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

	var startOffset uint64 = 0
	if len(startKey) > 0 && len(r.index) > 0 {
		idx := sort.Search(len(r.index), func(i int) bool {
			return bytes.Compare(r.index[i].key, startKey) > 0
		})
		if idx == 0 {
			startOffset = 0
		} else {
			startOffset = r.index[idx-1].offset
		}

		if startOffset < r.indexOffset {
			targetOffset, err := r.findOffsetAtOrAfter(startOffset, startKey)
			if err != nil {
				return nil, err
			}
			startOffset = targetOffset
		}
	}

	file, err := os.Open(r.FilePath())
	if err != nil {
		return nil, fmt.Errorf("failed to open file for iteration: %w", err)
	}

	success := false
	defer func() {
		if !success {
			file.Close()
		}
	}()

	if startOffset > 0 {
		if _, err := file.Seek(int64(startOffset), io.SeekStart); err != nil {
			return nil, fmt.Errorf("failed to seek to startOffset %d: %w", startOffset, err)
		}
	}

	success = true
	return &Iterator{
		file:        file,
		reader:      bufio.NewReaderSize(file, config.BufferSize),
		limitOffset: r.indexOffset,
		currOffset:  startOffset,
		key:         make([]byte, 0, config.InitialKeyCap),
		value:       make([]byte, 0, config.InitialValueCap),
	}, nil
}
