package sstable

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
)

// indexEntry records the key and byte offset of a single data entry
// for the in-memory index that is flushed to the Index Block on Close.
type indexEntry struct {
	key    []byte
	offset uint64
}

// Writer writes a sorted stream of key-value entries to an SSTable file.
// Entries must be added in sorted key order via Add. After all entries have
// been added, Close must be called to write the Index Block, Bloom Block,
// and Footer, then fsync and close the underlying file.
type Writer struct {
	file        *os.File
	writer      *bufio.Writer
	bloomFilter *BloomFilter
	offset      uint64       // running byte offset within the data block
	entryCount  uint32       // total number of entries written
	index       []indexEntry // in-memory index built during Add calls
	lastKey     []byte       // last key added, used to enforce sorted order
}

// MaxWriterExpectedKeys is the upper bound limit for the estimated keys parameter
// passed to a new SSTable writer. This prevents unbounded memory allocation for
// the in-memory bloom filter and index structures.
const MaxWriterExpectedKeys = 100000000

// NewWriter creates a new SSTable Writer for the specified file path.
// It initializes a bloom filter and an index based on expectedKeys to minimize
// memory reallocations. expectedKeys must be between 0 and MaxWriterExpectedKeys.
func NewWriter(filePath string, expectedKeys int) (*Writer, error) {
	if expectedKeys < 0 || expectedKeys > MaxWriterExpectedKeys {
		return nil, fmt.Errorf("%w: got %d", ErrInvalidExpectedKeys, expectedKeys)
	}

	file, err := os.OpenFile(
		filePath,
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		0o644,
	)
	if err != nil {
		return nil, err
	}

	return &Writer{
		file:        file,
		writer:      bufio.NewWriter(file),
		bloomFilter: NewBloomFilter(expectedKeys, 10),
		index:       make([]indexEntry, 0, expectedKeys),
	}, nil
}

// Add writes a single data entry to the SSTable. Entries must be added in
// sorted key order. Each call appends to the Data Block, records the offset
// in the in-memory index, and inserts the key into the Bloom Filter.
//
// Data Entry Layout:
// +------------+--------------+----------+-----------+-----------+
// | Key Length | Value Length | Opcode   | Key       | Value     |
// | (2 bytes)  | (4 bytes)    | (1 byte) | (n bytes) | (m bytes) |
// +------------+--------------+----------+-----------+-----------+
func (w *Writer) Add(key, value []byte, opcode uint8) error {
	if len(key) > math.MaxUint16 {
		return ErrKeyTooLarge
	}
	if uint64(len(value)) > math.MaxUint32 {
		return ErrValueTooLarge
	}
	if len(w.lastKey) > 0 && bytes.Compare(w.lastKey, key) >= 0 {
		return fmt.Errorf("%w: last %q >= new %q", ErrKeysOutOfOrder, w.lastKey, key)
	}

	// Record the offset for the index before writing.
	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)
	w.index = append(w.index, indexEntry{
		key:    keyCopy,
		offset: w.offset,
	})

	var header [entryHeaderSize]byte
	binary.LittleEndian.PutUint16(header[keyLenOffset:valueLenOffset], uint16(len(key)))
	binary.LittleEndian.PutUint32(header[valueLenOffset:opcodeOffset], uint32(len(value)))
	header[opcodeOffset] = opcode

	if _, err := w.writer.Write(header[:]); err != nil {
		return err
	}

	if _, err := w.writer.Write(key); err != nil {
		return err
	}

	if _, err := w.writer.Write(value); err != nil {
		return err
	}

	// Update running state.
	entrySize := uint64(entryHeaderSize) + uint64(len(key)) + uint64(len(value))
	w.offset += entrySize
	w.entryCount++

	// Add key to the Bloom Filter for later lookup acceleration.
	w.bloomFilter.Add(key)

	// Track last key for sorted-order enforcement.
	w.lastKey = make([]byte, len(key))
	copy(w.lastKey, key)

	return nil
}

// Close finalises the SSTable by writing the Index Block, Bloom Block, and
// Footer after the Data Block. It then flushes the bufio.Writer, fsyncs the
// file to durable storage, and closes the file descriptor.
//
// Index Entry Layout:
// +------------+--------------+-----------+
// | Key Length | Data Offset  | Key       |
// | (2 bytes)  | (8 bytes)    | (n bytes) |
// +------------+--------------+-----------+
//
// Footer Layout (25 bytes):
// +--------------+--------------+-----------------+-------------+-----------+
// | Index Offset | Bloom Offset | Bloom NumHashes | Entry Count | Magic     |
// | (8 bytes)    | (8 bytes)    | (1 byte)        | (4 bytes)   | (4 bytes) |
// +--------------+--------------+-----------------+-------------+-----------+
func (w *Writer) Close() (closeErr error) {
	defer func() {
		if err := w.file.Close(); closeErr == nil {
			closeErr = err
		}
	}()

	indexOffset := w.offset

	for _, entry := range w.index {
		var idxHeader [indexEntryHeaderSize]byte
		binary.LittleEndian.PutUint16(idxHeader[indexKeyLenOffset:indexOffsetOffset], uint16(len(entry.key)))
		binary.LittleEndian.PutUint64(idxHeader[indexOffsetOffset:indexKeyDataOffset], entry.offset)

		if _, err := w.writer.Write(idxHeader[:]); err != nil {
			return err
		}
		if _, err := w.writer.Write(entry.key); err != nil {
			return err
		}

		w.offset += uint64(indexEntryHeaderSize) + uint64(len(entry.key))
	}

	bloomOffset := w.offset
	bloomBytes := w.bloomFilter.Bytes()

	if _, err := w.writer.Write(bloomBytes); err != nil {
		return err
	}
	w.offset += uint64(len(bloomBytes))

	var footer [footerSize]byte
	binary.LittleEndian.PutUint64(footer[footerIndexOffsetOffset:footerBloomOffsetOffset], indexOffset)
	binary.LittleEndian.PutUint64(footer[footerBloomOffsetOffset:footerBloomNumHashesOffset], bloomOffset)
	footer[footerBloomNumHashesOffset] = w.bloomFilter.NumHashes()
	binary.LittleEndian.PutUint32(footer[footerEntryCountOffset:footerMagicOffset], w.entryCount)
	binary.LittleEndian.PutUint32(footer[footerMagicOffset:footerMagicOffset+footerMagicSize], magicNumber)

	if _, err := w.writer.Write(footer[:]); err != nil {
		return err
	}

	if err := w.writer.Flush(); err != nil {
		return err
	}

	return w.file.Sync()
}

// DataSize returns the total bytes written to the data block so far.
func (w *Writer) DataSize() uint64 {
	return w.offset
}

// EntryCount returns the number of entries written so far.
func (w *Writer) EntryCount() uint32 {
	return w.entryCount
}

// CurrentSize returns the current on-disk size of the SSTable file if it were
// closed and finalized immediately. It computes this by summing the sizes of the
// data block, index block, bloom filter block, and footer.
func (w *Writer) CurrentSize() uint64 {
	if w == nil {
		return 0
	}
	indexSize := uint64(len(w.index) * indexEntryHeaderSize)
	for _, entry := range w.index {
		indexSize += uint64(len(entry.key))
	}
	bloomSize := uint64(len(w.bloomFilter.Bytes()))
	return w.offset + indexSize + bloomSize + uint64(footerSize)
}
