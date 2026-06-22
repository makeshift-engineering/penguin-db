package sstable

import (
	"bufio"
	"encoding/binary"
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
}

// NewWriter creates a new Writer that will write an SSTable to filePath.
// expectedKeys is used to size the Bloom Filter (10 bits per key).
func NewWriter(filePath string, expectedKeys int) (*Writer, error) {
	file, err := os.OpenFile(
		filePath,
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		0o666,
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
	// Record the offset for the index before writing.
	w.index = append(w.index, indexEntry{
		key:    key,
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
func (w *Writer) Close() error {
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
	if err := w.file.Sync(); err != nil {
		return err
	}

	return w.file.Close()
}
