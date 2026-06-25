package sstable

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testDir creates a temporary directory for SSTable test files and returns its
// path along with a cleanup function. The directory is automatically removed
// after the test completes.
func testDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return dir
}

// TestWriter_EmptyTable verifies that closing a writer with zero entries produces
// a valid SSTable file containing only the Bloom Block and Footer (no data or
// index entries). The footer must still carry the correct magic number, zero
// entry count, and valid bloom / index offsets (both pointing to offset 0 since
// there is no data block).
func TestWriter_EmptyTable(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "empty.sst")

	w, err := NewWriter(path, 0)
	if err != nil {
		t.Fatalf("NewWriter failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	if len(data) < footerSize {
		t.Fatalf("file too small for footer: %d bytes", len(data))
	}

	footer := data[len(data)-footerSize:]

	// Magic number validation
	magic := binary.LittleEndian.Uint32(footer[footerMagicOffset : footerMagicOffset+footerMagicSize])
	if magic != magicNumber {
		t.Errorf("magic mismatch: got 0x%08X, want 0x%08X", magic, magicNumber)
	}

	// Entry count must be zero
	entryCount := binary.LittleEndian.Uint32(footer[footerEntryCountOffset:footerMagicOffset])
	if entryCount != 0 {
		t.Errorf("entry count: got %d, want 0", entryCount)
	}

	// Index offset should be 0 (no data block)
	indexOffset := binary.LittleEndian.Uint64(footer[footerIndexOffsetOffset:footerBloomOffsetOffset])
	if indexOffset != 0 {
		t.Errorf("index offset: got %d, want 0", indexOffset)
	}
}

// TestWriter_SingleEntry writes one Put entry and verifies the file layout
// byte-by-byte: data entry encoding, index entry encoding, bloom block, and
// footer fields.
func TestWriter_SingleEntry(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "single.sst")

	w, err := NewWriter(path, 1)
	if err != nil {
		t.Fatalf("NewWriter failed: %v", err)
	}

	key := []byte("hello")
	val := []byte("world")
	if err := w.Add(key, val, OpcodePut); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	offset := 0
	keyLen := binary.LittleEndian.Uint16(data[offset : offset+keyLenSize])
	offset += keyLenSize
	valLen := binary.LittleEndian.Uint32(data[offset : offset+valueLenSize])
	offset += valueLenSize
	opcode := data[offset]
	offset += opcodeSize

	if keyLen != uint16(len(key)) {
		t.Errorf("data keyLen: got %d, want %d", keyLen, len(key))
	}
	if valLen != uint32(len(val)) {
		t.Errorf("data valLen: got %d, want %d", valLen, len(val))
	}
	if opcode != OpcodePut {
		t.Errorf("data opcode: got 0x%02X, want 0x%02X", opcode, OpcodePut)
	}

	gotKey := data[offset : offset+int(keyLen)]
	offset += int(keyLen)
	gotVal := data[offset : offset+int(valLen)]

	if !bytes.Equal(gotKey, key) {
		t.Errorf("data key: got %q, want %q", gotKey, key)
	}
	if !bytes.Equal(gotVal, val) {
		t.Errorf("data value: got %q, want %q", gotVal, val)
	}

	footer := data[len(data)-footerSize:]
	entryCount := binary.LittleEndian.Uint32(footer[footerEntryCountOffset:footerMagicOffset])
	if entryCount != 1 {
		t.Errorf("entry count: got %d, want 1", entryCount)
	}

	magic := binary.LittleEndian.Uint32(footer[footerMagicOffset : footerMagicOffset+footerMagicSize])
	if magic != magicNumber {
		t.Errorf("magic: got 0x%08X, want 0x%08X", magic, magicNumber)
	}
}

// TestWriter_SortedEntries writes 100 sorted entries and verifies every key is
// present in the data block at the correct offset recorded in the index block.
func TestWriter_SortedEntries(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "sorted.sst")

	n := 100
	w, err := NewWriter(path, n)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	type kv struct {
		key, val []byte
	}
	entries := make([]kv, n)
	for i := 0; i < n; i++ {
		entries[i] = kv{
			key: []byte(fmt.Sprintf("key-%04d", i)),
			val: []byte(fmt.Sprintf("val-%04d", i)),
		}
	}
	for _, e := range entries {
		if err := w.Add(e.key, e.val, OpcodePut); err != nil {
			t.Fatalf("Add %q: %v", e.key, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Parse footer to get index offset and entry count
	footer := data[len(data)-footerSize:]
	indexOff := binary.LittleEndian.Uint64(footer[footerIndexOffsetOffset:footerBloomOffsetOffset])
	entryCount := binary.LittleEndian.Uint32(footer[footerEntryCountOffset:footerMagicOffset])

	if entryCount != uint32(n) {
		t.Fatalf("entry count: got %d, want %d", entryCount, n)
	}

	// Walk the index block and verify each entry's data offset resolves correctly
	pos := int(indexOff)
	for i := range n {
		if pos+indexEntryHeaderSize > len(data)-footerSize {
			t.Fatalf("index entry %d: overflows file at pos %d", i, pos)
		}
		idxKeyLen := binary.LittleEndian.Uint16(data[pos : pos+indexKeyLenSize])
		pos += indexKeyLenSize
		dataOff := binary.LittleEndian.Uint64(data[pos : pos+indexOffsetSize])
		pos += indexOffsetSize
		idxKey := data[pos : pos+int(idxKeyLen)]
		pos += int(idxKeyLen)

		// Verify this index key matches the expected sorted key
		if !bytes.Equal(idxKey, entries[i].key) {
			t.Errorf("index[%d] key: got %q, want %q", i, idxKey, entries[i].key)
		}

		// Read the actual data entry at dataOff and verify key+value
		dPos := int(dataOff)
		dKeyLen := binary.LittleEndian.Uint16(data[dPos : dPos+keyLenSize])
		dPos += keyLenSize
		dValLen := binary.LittleEndian.Uint32(data[dPos : dPos+valueLenSize])
		dPos += valueLenSize
		dOpcode := data[dPos]
		dPos += opcodeSize
		dKey := data[dPos : dPos+int(dKeyLen)]
		dPos += int(dKeyLen)
		dVal := data[dPos : dPos+int(dValLen)]

		if !bytes.Equal(dKey, entries[i].key) {
			t.Errorf("data[%d] key: got %q, want %q", i, dKey, entries[i].key)
		}
		if !bytes.Equal(dVal, entries[i].val) {
			t.Errorf("data[%d] val: got %q, want %q", i, dVal, entries[i].val)
		}
		if dOpcode != OpcodePut {
			t.Errorf("data[%d] opcode: got 0x%02X, want 0x%02X", i, dOpcode, OpcodePut)
		}
	}
}

// TestWriter_TombstoneEntry verifies that a delete-opcode entry is written with
// OpcodeDelete and an empty value, and is correctly encoded in the data block.
func TestWriter_TombstoneEntry(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "tombstone.sst")

	w, err := NewWriter(path, 1)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	key := []byte("deleted-key")
	// Tombstones carry an empty (zero-length) value.
	if err := w.Add(key, nil, OpcodeDelete); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Parse the first (only) data entry
	keyLen := binary.LittleEndian.Uint16(data[0:keyLenSize])
	valLen := binary.LittleEndian.Uint32(data[keyLenSize : keyLenSize+valueLenSize])
	opcode := data[keyLenSize+valueLenSize]

	if opcode != OpcodeDelete {
		t.Errorf("opcode: got 0x%02X, want 0x%02X (OpcodeDelete)", opcode, OpcodeDelete)
	}
	if valLen != 0 {
		t.Errorf("tombstone value length: got %d, want 0", valLen)
	}
	if keyLen != uint16(len(key)) {
		t.Errorf("key length: got %d, want %d", keyLen, len(key))
	}

	gotKey := data[entryHeaderSize : entryHeaderSize+int(keyLen)]
	if !bytes.Equal(gotKey, key) {
		t.Errorf("key: got %q, want %q", gotKey, key)
	}
}

// TestWriter_MixedPutAndDelete writes interleaved Put and Delete entries and
// verifies each one's opcode is preserved in the data block.
func TestWriter_MixedPutAndDelete(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "mixed.sst")

	type entry struct {
		key    []byte
		value  []byte
		opcode uint8
	}

	entries := []entry{
		{[]byte("a"), []byte("val-a"), OpcodePut},
		{[]byte("b"), nil, OpcodeDelete},
		{[]byte("c"), []byte("val-c"), OpcodePut},
		{[]byte("d"), nil, OpcodeDelete},
		{[]byte("e"), []byte("val-e"), OpcodePut},
	}

	w, err := NewWriter(path, len(entries))
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	for _, e := range entries {
		if err := w.Add(e.key, e.value, e.opcode); err != nil {
			t.Fatalf("Add %q: %v", e.key, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Walk data block sequentially and verify each entry
	pos := 0
	for i, e := range entries {
		if pos+entryHeaderSize > len(data) {
			t.Fatalf("entry %d: data block ends prematurely at pos %d", i, pos)
		}
		kl := binary.LittleEndian.Uint16(data[pos : pos+keyLenSize])
		pos += keyLenSize
		vl := binary.LittleEndian.Uint32(data[pos : pos+valueLenSize])
		pos += valueLenSize
		op := data[pos]
		pos += opcodeSize

		if op != e.opcode {
			t.Errorf("entry %d opcode: got 0x%02X, want 0x%02X", i, op, e.opcode)
		}

		gotKey := data[pos : pos+int(kl)]
		pos += int(kl)
		if !bytes.Equal(gotKey, e.key) {
			t.Errorf("entry %d key: got %q, want %q", i, gotKey, e.key)
		}

		if int(vl) > 0 {
			gotVal := data[pos : pos+int(vl)]
			pos += int(vl)
			if !bytes.Equal(gotVal, e.value) {
				t.Errorf("entry %d value: got %q, want %q", i, gotVal, e.value)
			}
		} else {
			// Ensure expected value was also nil/empty
			if len(e.value) != 0 {
				t.Errorf("entry %d: expected non-empty value but data has zero-length", i)
			}
		}
	}
}

// TestWriter_FooterMagicValidation verifies the footer magic number is
// exactly 0x50454E47 ("PENG") at the expected position within the file.
func TestWriter_FooterMagicValidation(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "magic.sst")

	w, err := NewWriter(path, 0)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	footer := data[len(data)-footerSize:]
	magic := binary.LittleEndian.Uint32(footer[footerMagicOffset : footerMagicOffset+footerMagicSize])

	// Check the raw bytes spell "PENG" in little-endian
	if magic != 0x50454E47 {
		t.Errorf("magic: got 0x%08X, want 0x50454E47", magic)
	}

	// Also verify via raw byte comparison
	expectedMagicBytes := []byte{0x47, 0x4E, 0x45, 0x50} // "PENG" little-endian
	magicSlice := footer[footerMagicOffset : footerMagicOffset+footerMagicSize]
	if !bytes.Equal(magicSlice, expectedMagicBytes) {
		t.Errorf("magic bytes: got %x, want %x", magicSlice, expectedMagicBytes)
	}
}

// TestWriter_LargeValue writes an entry with a 1 MB value and verifies the
// data round-trips correctly through the binary encoding.
func TestWriter_LargeValue(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "large_value.sst")

	w, err := NewWriter(path, 1)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	key := []byte("big-key")
	val := bytes.Repeat([]byte("X"), 1024*1024) // 1 MB
	if err := w.Add(key, val, OpcodePut); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Verify the value length field
	valLen := binary.LittleEndian.Uint32(data[keyLenSize : keyLenSize+valueLenSize])
	if valLen != uint32(len(val)) {
		t.Errorf("value length: got %d, want %d", valLen, len(val))
	}

	// Read back the value
	valStart := entryHeaderSize + len(key)
	gotVal := data[valStart : valStart+int(valLen)]
	if !bytes.Equal(gotVal, val) {
		t.Errorf("value mismatch for 1MB entry (lengths: got %d, want %d)", len(gotVal), len(val))
	}
}

// TestWriter_LargeKey writes an entry with the maximum key length (65535 bytes,
// uint16 max) and verifies it encodes correctly.
func TestWriter_LargeKey(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "large_key.sst")

	w, err := NewWriter(path, 1)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	key := bytes.Repeat([]byte("K"), math.MaxUint16) // 65535 bytes
	val := []byte("v")
	if err := w.Add(key, val, OpcodePut); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	keyLen := binary.LittleEndian.Uint16(data[0:keyLenSize])
	if keyLen != math.MaxUint16 {
		t.Errorf("key length: got %d, want %d", keyLen, math.MaxUint16)
	}
}

// TestWriter_EmptyValue writes a Put entry with a zero-length value (distinct
// from a tombstone, which uses OpcodeDelete). Verifies valueLenSize is 0 and
// the opcode is still OpcodePut.
func TestWriter_EmptyValue(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "empty_value.sst")

	w, err := NewWriter(path, 1)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	key := []byte("empty-val-key")
	if err := w.Add(key, []byte{}, OpcodePut); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	valLen := binary.LittleEndian.Uint32(data[keyLenSize : keyLenSize+valueLenSize])
	opcode := data[keyLenSize+valueLenSize]

	if valLen != 0 {
		t.Errorf("value length: got %d, want 0", valLen)
	}
	if opcode != OpcodePut {
		t.Errorf("opcode: got 0x%02X, want 0x%02X (OpcodePut)", opcode, OpcodePut)
	}
}

// TestWriter_NilValue verifies that passing a nil value slice produces the same
// encoding as an empty value (value length = 0).
func TestWriter_NilValue(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "nil_value.sst")

	w, err := NewWriter(path, 1)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	key := []byte("nil-val-key")
	if err := w.Add(key, nil, OpcodePut); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	valLen := binary.LittleEndian.Uint32(data[keyLenSize : keyLenSize+valueLenSize])
	if valLen != 0 {
		t.Errorf("nil value should encode as length 0, got %d", valLen)
	}
}

// TestWriter_OffsetTracking verifies that the index entries record the correct
// byte offsets into the data block for each entry. The expected offset for
// entry i is the sum of all preceding entry sizes.
func TestWriter_OffsetTracking(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "offsets.sst")

	type entry struct {
		key, val []byte
	}
	entries := []entry{
		{[]byte("a"), []byte("1")},       // size: 7 + 1 + 1 = 9
		{[]byte("bb"), []byte("22")},     // size: 7 + 2 + 2 = 11
		{[]byte("ccc"), []byte("333")},   // size: 7 + 3 + 3 = 13
		{[]byte("dddd"), []byte("4444")}, // size: 7 + 4 + 4 = 15
	}

	w, err := NewWriter(path, len(entries))
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	for _, e := range entries {
		if err := w.Add(e.key, e.val, OpcodePut); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	footer := data[len(data)-footerSize:]
	indexOff := binary.LittleEndian.Uint64(footer[footerIndexOffsetOffset:footerBloomOffsetOffset])

	// Calculate expected offsets
	expectedOffsets := make([]uint64, len(entries))
	running := uint64(0)
	for i, e := range entries {
		expectedOffsets[i] = running
		running += uint64(entryHeaderSize) + uint64(len(e.key)) + uint64(len(e.val))
	}

	// Read index entries and compare offsets
	pos := int(indexOff)
	for i := 0; i < len(entries); i++ {
		kl := binary.LittleEndian.Uint16(data[pos : pos+indexKeyLenSize])
		pos += indexKeyLenSize
		off := binary.LittleEndian.Uint64(data[pos : pos+indexOffsetSize])
		pos += indexOffsetSize
		pos += int(kl) // skip key data

		if off != expectedOffsets[i] {
			t.Errorf("index[%d] offset: got %d, want %d", i, off, expectedOffsets[i])
		}
	}
}

// TestWriter_BloomFilterIntegration verifies that the bloom filter stored in the
// SSTable file contains all keys that were added via the Writer.
func TestWriter_BloomFilterIntegration(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "bloom.sst")

	n := 500
	w, err := NewWriter(path, n)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	keys := make([][]byte, n)
	for i := 0; i < n; i++ {
		keys[i] = []byte(fmt.Sprintf("bloom-key-%05d", i))
		if err := w.Add(keys[i], []byte("v"), OpcodePut); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Manually reconstruct the bloom filter from the file
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	footer := data[len(data)-footerSize:]
	bloomOff := binary.LittleEndian.Uint64(footer[footerBloomOffsetOffset:footerBloomNumHashesOffset])
	bloomNumHashes := footer[footerBloomNumHashesOffset]

	// Bloom block ends where footer starts
	bloomEnd := uint64(len(data) - footerSize)
	bloomBytes := data[bloomOff:bloomEnd]

	// Reconstruct filter
	bf := &BloomFilter{
		bits:          bloomBytes,
		numHashes:     bloomNumHashes,
		totalBitCount: len(bloomBytes) * 8,
	}

	// Every added key must be found (no false negatives)
	for i, k := range keys {
		if !bf.MayContain(k) {
			t.Fatalf("bloom false negative at index %d for key %q", i, k)
		}
	}

	// Check false positives are within acceptable bounds
	falsePositives := 0
	probes := 10000
	for i := 0; i < probes; i++ {
		absentKey := []byte(fmt.Sprintf("absent-key-%05d", i))
		if bf.MayContain(absentKey) {
			falsePositives++
		}
	}
	fpr := float64(falsePositives) / float64(probes)
	t.Logf("bloom FPR from SSTable: %.4f%% (%d / %d)", fpr*100, falsePositives, probes)
	if fpr > 0.03 {
		t.Errorf("bloom FPR too high: %.4f, want <= 0.03", fpr)
	}
}

// TestWriter_BloomNumHashesInFooter verifies the footer stores the correct
// number of hash functions used by the bloom filter.
func TestWriter_BloomNumHashesInFooter(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "bloom_hashes.sst")

	w, err := NewWriter(path, 100)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	// Capture expected numHashes before close
	expectedHashes := w.bloomFilter.NumHashes()

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	footer := data[len(data)-footerSize:]
	gotHashes := footer[footerBloomNumHashesOffset]

	if gotHashes != expectedHashes {
		t.Errorf("bloom numHashes in footer: got %d, want %d", gotHashes, expectedHashes)
	}
}

// TestWriter_BinaryKeys verifies that keys containing null bytes, high bytes,
// and other non-printable characters are encoded and decoded faithfully.
func TestWriter_BinaryKeys(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "binary_keys.sst")

	binaryKeys := [][]byte{
		{0x00, 0x00, 0x00},
		{0x00, 0xFF, 0x00, 0xFF},
		{0x01, 0x02, 0x03, 0x04, 0x05},
		{0xFF, 0xFE, 0xFD},
	}

	w, err := NewWriter(path, len(binaryKeys))
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	for _, k := range binaryKeys {
		if err := w.Add(k, []byte("val"), OpcodePut); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Walk data entries and verify keys
	pos := 0
	for i, expectedKey := range binaryKeys {
		kl := binary.LittleEndian.Uint16(data[pos : pos+keyLenSize])
		pos += keyLenSize
		vl := binary.LittleEndian.Uint32(data[pos : pos+valueLenSize])
		pos += valueLenSize
		pos++ // opcode

		gotKey := data[pos : pos+int(kl)]
		pos += int(kl)
		pos += int(vl) // skip value

		if !bytes.Equal(gotKey, expectedKey) {
			t.Errorf("binary key[%d]: got %x, want %x", i, gotKey, expectedKey)
		}
	}
}

// TestWriter_UnicodeKeys verifies multi-byte UTF-8 keys are handled correctly.
func TestWriter_UnicodeKeys(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "unicode.sst")

	keys := [][]byte{
		[]byte("中文"),
		[]byte("日本語"),
		[]byte("한국어"),
		[]byte("🐧"),
	}

	w, err := NewWriter(path, len(keys))
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	for _, k := range keys {
		if err := w.Add(k, []byte("v"), OpcodePut); err != nil {
			t.Fatalf("Add %q: %v", k, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	pos := 0
	for i, expectedKey := range keys {
		kl := binary.LittleEndian.Uint16(data[pos : pos+keyLenSize])
		pos += keyLenSize
		vl := binary.LittleEndian.Uint32(data[pos : pos+valueLenSize])
		pos += valueLenSize
		pos++ // opcode

		gotKey := data[pos : pos+int(kl)]
		pos += int(kl)
		pos += int(vl)

		if !bytes.Equal(gotKey, expectedKey) {
			t.Errorf("unicode key[%d]: got %q, want %q", i, gotKey, expectedKey)
		}
	}
}

// TestWriter_FileSizeConsistency verifies the total file size equals the sum of
// all data entries + index entries + bloom block + footer.
func TestWriter_FileSizeConsistency(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "filesize.sst")

	type entry struct {
		key, val []byte
	}
	entries := []entry{
		{[]byte("alpha"), []byte("one")},
		{[]byte("bravo"), []byte("two")},
		{[]byte("charlie"), []byte("three")},
	}

	w, err := NewWriter(path, len(entries))
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	for _, e := range entries {
		if err := w.Add(e.key, e.val, OpcodePut); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	fileSize := info.Size()

	// Calculate expected size
	dataSize := int64(0)
	indexSize := int64(0)
	for _, e := range entries {
		dataSize += int64(entryHeaderSize + len(e.key) + len(e.val))
		indexSize += int64(indexEntryHeaderSize + len(e.key))
	}

	// Bloom size: NewBloomFilter(3, 10) → 3*10 = 30 bits < 64 → 64 bits → 8 bytes
	bloomBitsTotal := len(entries) * 10
	if bloomBitsTotal < 64 {
		bloomBitsTotal = 64
	}
	bloomSize := int64((bloomBitsTotal + 7) / 8)

	expectedSize := dataSize + indexSize + bloomSize + int64(footerSize)
	if fileSize != expectedSize {
		t.Errorf("file size: got %d, want %d (data=%d, index=%d, bloom=%d, footer=%d)",
			fileSize, expectedSize, dataSize, indexSize, bloomSize, footerSize)
	}
}

// TestWriter_FooterOffsets verifies that IndexOffset and BloomOffset in the
// footer correctly locate the start of their respective blocks.
func TestWriter_FooterOffsets(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "footer_offsets.sst")

	n := 10
	w, err := NewWriter(path, n)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	var totalDataSize uint64
	for i := 0; i < n; i++ {
		key := []byte(fmt.Sprintf("k%02d", i))
		val := []byte(fmt.Sprintf("v%02d", i))
		if err := w.Add(key, val, OpcodePut); err != nil {
			t.Fatalf("Add: %v", err)
		}
		totalDataSize += uint64(entryHeaderSize + len(key) + len(val))
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	footer := data[len(data)-footerSize:]
	indexOff := binary.LittleEndian.Uint64(footer[footerIndexOffsetOffset:footerBloomOffsetOffset])
	bloomOff := binary.LittleEndian.Uint64(footer[footerBloomOffsetOffset:footerBloomNumHashesOffset])

	// Index offset should equal total data size
	if indexOff != totalDataSize {
		t.Errorf("index offset: got %d, want %d (total data size)", indexOff, totalDataSize)
	}

	// Bloom offset should be after the index block
	if bloomOff <= indexOff {
		t.Errorf("bloom offset (%d) should be > index offset (%d)", bloomOff, indexOff)
	}

	// Bloom block should end right before the footer
	bloomEnd := uint64(len(data) - footerSize)
	if bloomOff >= bloomEnd {
		t.Errorf("bloom offset (%d) should be < footer start (%d)", bloomOff, bloomEnd)
	}
}

// TestWriter_EntryCountTracking verifies the footer's entry count field matches
// the actual number of Add calls, including tombstones.
func TestWriter_EntryCountTracking(t *testing.T) {
	counts := []int{0, 1, 5, 50, 500}

	for _, n := range counts {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			dir := testDir(t)
			path := filepath.Join(dir, "count.sst")

			w, err := NewWriter(path, n)
			if err != nil {
				t.Fatalf("NewWriter: %v", err)
			}
			for i := 0; i < n; i++ {
				key := []byte(fmt.Sprintf("key-%04d", i))
				opcode := OpcodePut
				if i%3 == 0 {
					opcode = OpcodeDelete
				}
				if err := w.Add(key, []byte("v"), opcode); err != nil {
					t.Fatalf("Add: %v", err)
				}
			}
			if err := w.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read: %v", err)
			}

			footer := data[len(data)-footerSize:]
			got := binary.LittleEndian.Uint32(footer[footerEntryCountOffset:footerMagicOffset])
			if got != uint32(n) {
				t.Errorf("entry count: got %d, want %d", got, n)
			}
		})
	}
}

// TestWriter_DuplicateKeys verifies the Writer rejects duplicate keys with
// ErrKeysOutOfOrder, since keys must be in strictly ascending order.
func TestWriter_DuplicateKeys(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "dupes.sst")

	w, err := NewWriter(path, 2)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	key := []byte("same-key")
	if err := w.Add(key, []byte("first"), OpcodePut); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	err = w.Add(key, []byte("second"), OpcodePut)
	if err == nil {
		w.Close()
		t.Fatal("expected error for duplicate key, got nil")
	}
	if !errors.Is(err, ErrKeysOutOfOrder) {
		w.Close()
		t.Fatalf("expected ErrKeysOutOfOrder, got: %v", err)
	}
	w.Close()
}

// TestWriter_CreatesFileIfNotExists verifies that NewWriter creates the file
// (and any missing parent directories are not its responsibility — only the file).
func TestWriter_CreatesFileIfNotExists(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "new_file.sst")

	// File should not exist yet
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file to not exist before NewWriter")
	}

	w, err := NewWriter(path, 0)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// File should now exist
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should exist after Close: %v", err)
	}
}

// TestWriter_OverwritesExistingFile verifies that creating a Writer for a path
// that already contains data truncates the old content (O_TRUNC flag).
func TestWriter_OverwritesExistingFile(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "overwrite.sst")

	// Write initial content directly
	if err := os.WriteFile(path, bytes.Repeat([]byte("X"), 10000), 0666); err != nil {
		t.Fatalf("pre-fill: %v", err)
	}

	// Now write an SSTable over it
	w, err := NewWriter(path, 1)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if err := w.Add([]byte("k"), []byte("v"), OpcodePut); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	// The file should be much smaller than the pre-filled 10000 bytes
	if info.Size() >= 10000 {
		t.Errorf("expected file to be truncated, got size %d", info.Size())
	}

	// Footer should be valid
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	footer := data[len(data)-footerSize:]
	magic := binary.LittleEndian.Uint32(footer[footerMagicOffset : footerMagicOffset+footerMagicSize])
	if magic != magicNumber {
		t.Errorf("magic after overwrite: got 0x%08X, want 0x%08X", magic, magicNumber)
	}
}

// TestWriter_InvalidPath verifies that NewWriter returns an error when given
// an invalid file path (e.g., non-existent directory).
func TestWriter_InvalidPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent", "dir", "test.sst")
	_, err := NewWriter(path, 0)
	if err == nil {
		t.Fatalf("expected error for invalid path, got nil")
	}
}

// TestWriter_IndexBlockStructure verifies the Index Block contains one entry
// per data entry, with correct key lengths and keys matching the data block.
func TestWriter_IndexBlockStructure(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "index_structure.sst")

	keys := [][]byte{
		[]byte("aaa"),
		[]byte("bbbbb"),
		[]byte("ccccccc"),
	}

	w, err := NewWriter(path, len(keys))
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	for _, k := range keys {
		if err := w.Add(k, []byte("v"), OpcodePut); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	footer := data[len(data)-footerSize:]
	indexOff := binary.LittleEndian.Uint64(footer[footerIndexOffsetOffset:footerBloomOffsetOffset])
	bloomOff := binary.LittleEndian.Uint64(footer[footerBloomOffsetOffset:footerBloomNumHashesOffset])

	// Walk the index block (between indexOff and bloomOff)
	pos := int(indexOff)
	for i := 0; i < len(keys); i++ {
		if pos >= int(bloomOff) {
			t.Fatalf("index entry %d: exceeded bloom boundary at pos %d", i, pos)
		}

		kl := binary.LittleEndian.Uint16(data[pos : pos+indexKeyLenSize])
		pos += indexKeyLenSize
		_ = binary.LittleEndian.Uint64(data[pos : pos+indexOffsetSize]) // data offset
		pos += indexOffsetSize

		gotKey := data[pos : pos+int(kl)]
		pos += int(kl)

		if !bytes.Equal(gotKey, keys[i]) {
			t.Errorf("index key[%d]: got %q, want %q", i, gotKey, keys[i])
		}
	}

	// After reading all index entries, pos should be exactly at bloomOff
	if uint64(pos) != bloomOff {
		t.Errorf("index block end: pos=%d, bloomOff=%d — index entries don't fill exactly to bloom start", pos, bloomOff)
	}
}

// TestWriter_MultipleFiles verifies that multiple independent Writers writing
// to separate files produce valid, independent SSTables.
func TestWriter_MultipleFiles(t *testing.T) {
	dir := testDir(t)

	for fileIdx := range 5 {
		path := filepath.Join(dir, fmt.Sprintf("%06d.sst", fileIdx))
		w, err := NewWriter(path, 10)
		if err != nil {
			t.Fatalf("file %d NewWriter: %v", fileIdx, err)
		}

		for j := 0; j < 10; j++ {
			key := []byte(fmt.Sprintf("file%d-key%d", fileIdx, j))
			val := []byte(fmt.Sprintf("file%d-val%d", fileIdx, j))
			if err := w.Add(key, val, OpcodePut); err != nil {
				t.Fatalf("file %d Add: %v", fileIdx, err)
			}
		}
		if err := w.Close(); err != nil {
			t.Fatalf("file %d Close: %v", fileIdx, err)
		}

		// Validate footer of each file
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("file %d read: %v", fileIdx, err)
		}
		footer := data[len(data)-footerSize:]
		magic := binary.LittleEndian.Uint32(footer[footerMagicOffset : footerMagicOffset+footerMagicSize])
		if magic != magicNumber {
			t.Errorf("file %d magic: got 0x%08X, want 0x%08X", fileIdx, magic, magicNumber)
		}
		count := binary.LittleEndian.Uint32(footer[footerEntryCountOffset:footerMagicOffset])
		if count != 10 {
			t.Errorf("file %d entry count: got %d, want 10", fileIdx, count)
		}
	}
}

// TestWriter_MaxValueLength verifies that a value approaching the uint32 max
// limit (we use a 4 MB value as a practical test) round-trips correctly.
func TestWriter_MaxValueLength(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large allocation test in short mode")
	}

	dir := testDir(t)
	path := filepath.Join(dir, "max_val.sst")

	w, err := NewWriter(path, 1)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	key := []byte("big")
	val := bytes.Repeat([]byte("V"), 4*1024*1024) // 4 MB
	if err := w.Add(key, val, OpcodePut); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	valLen := binary.LittleEndian.Uint32(data[keyLenSize : keyLenSize+valueLenSize])
	if valLen != uint32(len(val)) {
		t.Errorf("value length: got %d, want %d", valLen, len(val))
	}
}

// TestWriter_PreservesInsertionOrder verifies the data block stores entries in
// the exact order they were added. The Writer trusts the caller to provide sorted
// keys; this test confirms insertion order is faithfully preserved.
func TestWriter_PreservesInsertionOrder(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "order.sst")

	// Use keys that are sorted to mimic a real MemTable flush
	keys := []string{"aardvark", "banana", "cherry", "durian", "elderberry"}

	w, err := NewWriter(path, len(keys))
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	for _, k := range keys {
		if err := w.Add([]byte(k), []byte("v"), OpcodePut); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	pos := 0
	for i, expectedKey := range keys {
		kl := binary.LittleEndian.Uint16(data[pos : pos+keyLenSize])
		pos += keyLenSize
		vl := binary.LittleEndian.Uint32(data[pos : pos+valueLenSize])
		pos += valueLenSize
		pos++ // opcode

		gotKey := string(data[pos : pos+int(kl)])
		pos += int(kl)
		pos += int(vl) // skip value

		if gotKey != expectedKey {
			t.Errorf("order[%d]: got %q, want %q", i, gotKey, expectedKey)
		}
	}
}

// TestWriter_SingleByteKeyAndValue verifies the minimum-size key-value pair
// (1 byte each) is encoded correctly.
func TestWriter_SingleByteKeyAndValue(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "single_byte.sst")

	w, err := NewWriter(path, 1)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if err := w.Add([]byte("k"), []byte("v"), OpcodePut); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	kl := binary.LittleEndian.Uint16(data[0:keyLenSize])
	vl := binary.LittleEndian.Uint32(data[keyLenSize : keyLenSize+valueLenSize])

	if kl != 1 {
		t.Errorf("key length: got %d, want 1", kl)
	}
	if vl != 1 {
		t.Errorf("value length: got %d, want 1", vl)
	}

	if data[entryHeaderSize] != 'k' {
		t.Errorf("key byte: got 0x%02X, want 'k'", data[entryHeaderSize])
	}
	if data[entryHeaderSize+1] != 'v' {
		t.Errorf("value byte: got 0x%02X, want 'v'", data[entryHeaderSize+1])
	}
}

// TestWriter_HighEntryCount writes 10,000 entries and verifies the footer
// entry count and that the data block is parseable without corruption.
func TestWriter_HighEntryCount(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	dir := testDir(t)
	path := filepath.Join(dir, "stress.sst")

	n := 10000
	w, err := NewWriter(path, n)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	for i := 0; i < n; i++ {
		key := []byte(fmt.Sprintf("stress-key-%06d", i))
		val := []byte(fmt.Sprintf("stress-val-%06d", i))
		if err := w.Add(key, val, OpcodePut); err != nil {
			t.Fatalf("Add[%d]: %v", i, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	footer := data[len(data)-footerSize:]
	count := binary.LittleEndian.Uint32(footer[footerEntryCountOffset:footerMagicOffset])
	if count != uint32(n) {
		t.Errorf("entry count: got %d, want %d", count, n)
	}

	// Walk every data entry to confirm parsability
	indexOff := binary.LittleEndian.Uint64(footer[footerIndexOffsetOffset:footerBloomOffsetOffset])
	pos := 0
	parsed := 0
	for pos < int(indexOff) {
		if pos+entryHeaderSize > int(indexOff) {
			t.Fatalf("truncated entry header at pos %d, indexOff %d", pos, indexOff)
		}
		kl := binary.LittleEndian.Uint16(data[pos : pos+keyLenSize])
		pos += keyLenSize
		vl := binary.LittleEndian.Uint32(data[pos : pos+valueLenSize])
		pos += valueLenSize
		pos++ // opcode
		pos += int(kl) + int(vl)
		parsed++
	}

	if parsed != n {
		t.Errorf("parsed %d entries from data block, want %d", parsed, n)
	}
}

// TestWriter_VariableSizeEntries writes entries with widely varying key and
// value sizes and verifies all are correctly encoded and can be parsed back.
func TestWriter_VariableSizeEntries(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "variable.sst")

	type entry struct {
		key, val []byte
	}
	entries := []entry{
		{[]byte("a"), bytes.Repeat([]byte("x"), 1)},
		{[]byte("bb"), bytes.Repeat([]byte("y"), 100)},
		{[]byte("ccc"), bytes.Repeat([]byte("z"), 10000)},
		{bytes.Repeat([]byte("d"), 200), []byte("short")},
		{[]byte("e"), nil},
	}

	w, err := NewWriter(path, len(entries))
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	for _, e := range entries {
		if err := w.Add(e.key, e.val, OpcodePut); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Parse back and verify each entry
	pos := 0
	for i, e := range entries {
		kl := binary.LittleEndian.Uint16(data[pos : pos+keyLenSize])
		pos += keyLenSize
		vl := binary.LittleEndian.Uint32(data[pos : pos+valueLenSize])
		pos += valueLenSize
		pos++ // opcode

		gotKey := data[pos : pos+int(kl)]
		pos += int(kl)

		if !bytes.Equal(gotKey, e.key) {
			t.Errorf("entry[%d] key: got %q, want %q", i, gotKey, e.key)
		}

		expectedValLen := 0
		if e.val != nil {
			expectedValLen = len(e.val)
		}
		if int(vl) != expectedValLen {
			t.Errorf("entry[%d] val length: got %d, want %d", i, vl, expectedValLen)
		}

		if vl > 0 {
			gotVal := data[pos : pos+int(vl)]
			if !bytes.Equal(gotVal, e.val) {
				t.Errorf("entry[%d] val mismatch (len got=%d, want=%d)", i, len(gotVal), len(e.val))
			}
		}
		pos += int(vl)
	}
}

// TestWriter_AllTombstones verifies that a table containing only delete entries
// (no Put entries at all) is valid and all opcodes are OpcodeDelete.
func TestWriter_AllTombstones(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "all_tombstones.sst")

	n := 20
	w, err := NewWriter(path, n)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	for i := 0; i < n; i++ {
		key := []byte(fmt.Sprintf("del-%03d", i))
		if err := w.Add(key, nil, OpcodeDelete); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	footer := data[len(data)-footerSize:]
	indexOff := binary.LittleEndian.Uint64(footer[footerIndexOffsetOffset:footerBloomOffsetOffset])

	pos := 0
	for i := 0; pos < int(indexOff); i++ {
		kl := binary.LittleEndian.Uint16(data[pos : pos+keyLenSize])
		pos += keyLenSize
		vl := binary.LittleEndian.Uint32(data[pos : pos+valueLenSize])
		pos += valueLenSize
		opcode := data[pos]
		pos++

		if opcode != OpcodeDelete {
			t.Errorf("entry[%d] opcode: got 0x%02X, want 0x%02X", i, opcode, OpcodeDelete)
		}
		if vl != 0 {
			t.Errorf("entry[%d] tombstone value length: got %d, want 0", i, vl)
		}

		pos += int(kl) + int(vl)
	}
}

// TestWriter_SimilarPrefixKeys writes keys that share a long common prefix
// (common in real-world usage, e.g., "user:1000", "user:1001") and verifies
// each is independently distinguishable in the data and index blocks.
func TestWriter_SimilarPrefixKeys(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "prefix.sst")

	prefix := strings.Repeat("namespace/collection/", 5) // long shared prefix
	n := 50
	keys := make([][]byte, n)
	for i := range n {
		keys[i] = []byte(fmt.Sprintf("%s%04d", prefix, i))
	}

	w, err := NewWriter(path, n)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	for _, k := range keys {
		if err := w.Add(k, []byte("v"), OpcodePut); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Verify all keys are distinct and correct in the data block
	footer := data[len(data)-footerSize:]
	indexOff := binary.LittleEndian.Uint64(footer[footerIndexOffsetOffset:footerBloomOffsetOffset])

	pos := 0
	for i := 0; pos < int(indexOff); i++ {
		kl := binary.LittleEndian.Uint16(data[pos : pos+keyLenSize])
		pos += keyLenSize
		vl := binary.LittleEndian.Uint32(data[pos : pos+valueLenSize])
		pos += valueLenSize
		pos++ // opcode

		gotKey := data[pos : pos+int(kl)]
		pos += int(kl)
		pos += int(vl)

		if i < n && !bytes.Equal(gotKey, keys[i]) {
			t.Errorf("key[%d]: got %q, want %q", i, gotKey, keys[i])
		}
	}
}

// TestWriter_FooterSizeConstant sanity-checks that the footerSize constant
// equals the sum of its component sizes (regression guard).
func TestWriter_FooterSizeConstant(t *testing.T) {
	expected := 8 + 8 + 1 + 4 + 4 // indexOffset + bloomOffset + numHashes + entryCount + magic
	if footerSize != expected {
		t.Errorf("footerSize: got %d, want %d", footerSize, expected)
	}
}

// TestWriter_EntryHeaderSizeConstant sanity-checks entryHeaderSize.
func TestWriter_EntryHeaderSizeConstant(t *testing.T) {
	expected := 2 + 4 + 1 // keyLen + valueLen + opcode
	if entryHeaderSize != expected {
		t.Errorf("entryHeaderSize: got %d, want %d", entryHeaderSize, expected)
	}
}

// TestWriter_IndexEntryHeaderSizeConstant sanity-checks indexEntryHeaderSize.
func TestWriter_IndexEntryHeaderSizeConstant(t *testing.T) {
	expected := 2 + 8 // keyLen + dataOffset
	if indexEntryHeaderSize != expected {
		t.Errorf("indexEntryHeaderSize: got %d, want %d", indexEntryHeaderSize, expected)
	}
}

// TestWriter_KeyTooLarge verifies that Add returns ErrKeyTooLarge when the key
// exceeds math.MaxUint16 bytes (the maximum representable in the 2-byte key
// length header field).
func TestWriter_KeyTooLarge(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "key_too_large.sst")

	w, err := NewWriter(path, 1)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer w.Close()

	oversizedKey := bytes.Repeat([]byte("K"), math.MaxUint16+1)
	err = w.Add(oversizedKey, []byte("v"), OpcodePut)
	if !errors.Is(err, ErrKeyTooLarge) {
		t.Errorf("expected ErrKeyTooLarge, got: %v", err)
	}
}

// TestWriter_ValueTooLarge verifies that Add returns ErrValueTooLarge when the
// value exceeds math.MaxUint32 bytes (the maximum representable in the 4-byte
// value length header field).
//
// This test is skipped by default because it allocates >4 GiB of memory.
func TestWriter_ValueTooLarge(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: allocates >4 GiB")
	}

	dir := testDir(t)
	path := filepath.Join(dir, "value_too_large.sst")

	w, err := NewWriter(path, 1)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer w.Close()

	oversizedValue := make([]byte, math.MaxUint32+1)
	err = w.Add([]byte("k"), oversizedValue, OpcodePut)
	if !errors.Is(err, ErrValueTooLarge) {
		t.Errorf("expected ErrValueTooLarge, got: %v", err)
	}
}

// TestWriter_MaxKeyLengthAccepted verifies that a key of exactly
// math.MaxUint16 bytes is accepted (boundary test).
func TestWriter_MaxKeyLengthAccepted(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "max_key.sst")

	w, err := NewWriter(path, 1)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	maxKey := bytes.Repeat([]byte("K"), math.MaxUint16)
	if err := w.Add(maxKey, []byte("v"), OpcodePut); err != nil {
		t.Fatalf("Add with max-length key should succeed, got: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// mockFailWriter is a writer that always returns an error, used to simulate
// write failures during Close.
type mockFailWriter struct{}

func (*mockFailWriter) Write([]byte) (int, error) {
	return 0, errors.New("injected write failure")
}

// TestWriter_CloseReleasesFileOnWriteError verifies that Close always releases
// the underlying file descriptor even when writing the index/bloom/footer
// fails. Before the fix this was a file descriptor leak.
func TestWriter_CloseReleasesFileOnWriteError(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "leak.sst")

	w, err := NewWriter(path, 1)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	// Add an entry so there is index data to write during Close.
	if err := w.Add([]byte("k"), []byte("v"), OpcodePut); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Sabotage the buffered writer so all writes inside Close fail.
	w.writer = bufio.NewWriter(&mockFailWriter{})

	// Close must return an error from the failing writer...
	if err := w.Close(); err == nil {
		t.Fatal("expected Close to return an error from the failing writer")
	}

	// ...but the underlying file must still be closed (defer guarantee).
	// Attempting Sync on a closed file must fail.
	if err := w.file.Sync(); err == nil {
		t.Error("file.Sync() succeeded after Close; file descriptor was leaked")
	}
}

// TestWriter_CloseReturnsWriteErrorOverCloseError verifies that when a write
// error occurs AND the file close also errors, the original write error is the
// one surfaced to the caller.
func TestWriter_CloseReturnsWriteErrorOverCloseError(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "err_priority.sst")

	w, err := NewWriter(path, 1)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	if err := w.Add([]byte("k"), []byte("v"), OpcodePut); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Close the file early so that the deferred file.Close() also errors.
	w.file.Close()

	// Sabotage the writer so Close hits a write error first.
	w.writer = bufio.NewWriter(&mockFailWriter{})

	err = w.Close()
	if err == nil {
		t.Fatal("expected Close to return an error")
	}

	// The returned error must be the write error, not the double-close error.
	if !strings.Contains(err.Error(), "injected write failure") {
		t.Errorf("expected write error to take precedence, got: %v", err)
	}
}

// TestWriter_CloseReleasesFileOnSuccess verifies that after a successful Close
// the file descriptor is no longer open (sanity check for the defer path).
func TestWriter_CloseReleasesFileOnSuccess(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "closed.sst")

	w, err := NewWriter(path, 1)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	if err := w.Add([]byte("k"), []byte("v"), OpcodePut); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// The file must be closed; Sync on a closed file must fail.
	if err := w.file.Sync(); err == nil {
		t.Error("file.Sync() succeeded after successful Close; fd not released")
	}
}
