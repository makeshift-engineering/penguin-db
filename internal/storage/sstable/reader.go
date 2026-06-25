package sstable

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"sort"
)

// Reader reads entries from an immutable SSTable file. On Open, the footer,
// index block, and bloom filter are loaded into memory. The data block remains
// on disk and is accessed via seek+read when Get finds a candidate in the
// in-memory index.
type Reader struct {
	file        *os.File
	bloomFilter *BloomFilter
	index       []indexEntry
	entryCount  uint32
	fileSize    int64
	indexOffset uint64
	closed      bool
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
	// Each entry requires at least indexEntryHeaderSize bytes.
	// If the declared count can't possibly fit in the index block, the
	// footer is corrupt — fail fast instead of silently capping.
	if uint64(expectedCount)*uint64(indexEntryHeaderSize) > uint64(len(data)) {
		return nil, fmt.Errorf("%w: expected %d entries but index block is only %d bytes",
			ErrCorrupted, expectedCount, len(data))
	}
	entries := make([]indexEntry, 0, expectedCount)
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

	if uint32(len(entries)) != expectedCount {
		return nil, fmt.Errorf("%w: index has %d entries, footer says %d", ErrCorrupted, len(entries), expectedCount)
	}

	return entries, nil
}

// BloomMayContain returns true if the key might exist in this SSTable
// according to the Bloom Filter. A false return guarantees the key is absent.
func (r *Reader) BloomMayContain(key []byte) bool {
	if r.closed {
		return false
	}
	return r.bloomFilter.MayContain(key)
}

// Get searches the SSTable for the given key using binary search over the
// in-memory index. If the key is found in the index, it seeks to the
// corresponding data offset and reads the full entry.
//
// Returns:
//   - (value, true, false, nil)  : key found with a live value
//   - (nil,   true, true,  nil)  : key found but it is a tombstone (deleted)
//   - (nil,   false, false, nil) : key not present in this SSTable
//   - (nil,   false, false, err) : an I/O or corruption error occurred
func (r *Reader) Get(key []byte) (value []byte, found, deleted bool, err error) {
	if r.closed {
		return nil, false, false, ErrReaderClosed
	}

	if len(r.index) == 0 {
		return nil, false, false, nil
	}

	// Binary search over the sorted index.
	i := sort.Search(len(r.index), func(i int) bool {
		return bytes.Compare(r.index[i].key, key) >= 0
	})

	if i >= len(r.index) || !bytes.Equal(r.index[i].key, key) {
		return nil, false, false, nil
	}

	// Found a candidate in the index, read the data entry from disk.
	if r.index[i].offset >= r.indexOffset {
		return nil, false, false, fmt.Errorf("%w: index entry offset %d exceeds index offset %d", ErrCorrupted, r.index[i].offset, r.indexOffset)
	}
	if r.index[i].offset > uint64(math.MaxInt64) {
		return nil, false, false, fmt.Errorf("%w: index entry offset %d exceeds max int64", ErrCorrupted, r.index[i].offset)
	}
	dataOffset := int64(r.index[i].offset)

	// Read the fixed-size entry header.
	var header [entryHeaderSize]byte
	if _, err := r.file.ReadAt(header[:], dataOffset); err != nil {
		return nil, false, false, fmt.Errorf("reading data entry header: %w", err)
	}

	keyLen := binary.LittleEndian.Uint16(header[keyLenOffset:valueLenOffset])
	valLen := binary.LittleEndian.Uint32(header[valueLenOffset:opcodeOffset])
	opcode := header[opcodeOffset]

	if uint64(dataOffset)+uint64(entryHeaderSize)+uint64(keyLen)+uint64(valLen) > r.indexOffset {
		return nil, false, false, fmt.Errorf("%w: entry sizes exceed data block boundary", ErrCorrupted)
	}

	// Read the key from disk and verify it matches.
	entryKey := make([]byte, keyLen)
	if keyLen > 0 {
		if _, err := r.file.ReadAt(entryKey, dataOffset+int64(entryHeaderSize)); err != nil {
			return nil, false, false, fmt.Errorf("reading data entry key: %w", err)
		}
	}

	if !bytes.Equal(entryKey, key) {
		// Index said this offset has our key but the on-disk key differs.
		return nil, false, false, fmt.Errorf("%w: index key mismatch at offset %d", ErrCorrupted, dataOffset)
	}

	if opcode == OpcodeDelete {
		return nil, true, true, nil
	}

	// Read the value.
	if uint64(valLen) > r.indexOffset-uint64(dataOffset)-uint64(entryHeaderSize)-uint64(keyLen) {
		return nil, false, false, fmt.Errorf("%w: value length %d exceeds available data block space", ErrCorrupted, valLen)
	}
	val := make([]byte, valLen)
	if valLen > 0 {
		if _, err := r.file.ReadAt(val, dataOffset+int64(entryHeaderSize)+int64(keyLen)); err != nil {
			return nil, false, false, fmt.Errorf("reading data entry value: %w", err)
		}
	}

	return val, true, false, nil
}

// EntryCount returns the total number of entries in this SSTable as recorded in the footer.
func (r *Reader) EntryCount() uint32 {
	return r.entryCount
}

// Close closes the underlying file handle. After Close, all read operations will return ErrReaderClosed.
func (r *Reader) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	return r.file.Close()
}

// FilePath returns the path of the underlying SSTable file.
func (r *Reader) FilePath() string {
	if r.file == nil {
		return ""
	}
	return r.file.Name()
}
