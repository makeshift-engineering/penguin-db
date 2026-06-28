package sstable

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestIterator_OptionsCapping verifies that WithBufferSize and WithInitialCapacities
// correctly restrict option parameters to predefined maximums and ignore invalid/negative options.
func TestIterator_OptionsCapping(t *testing.T) {
	opts := &IteratorOptions{
		BufferSize:      DefaultIteratorBufferSize,
		InitialKeyCap:   DefaultIteratorKeyCap,
		InitialValueCap: DefaultIteratorValueCap,
	}

	WithBufferSize(-1)(opts)
	if opts.BufferSize != DefaultIteratorBufferSize {
		t.Errorf("expected buffer size %d, got %d", DefaultIteratorBufferSize, opts.BufferSize)
	}

	WithBufferSize(MaxIteratorBufferSize + 1)(opts)
	if opts.BufferSize != MaxIteratorBufferSize {
		t.Errorf("expected buffer size %d, got %d", MaxIteratorBufferSize, opts.BufferSize)
	}

	WithBufferSize(8192)(opts)
	if opts.BufferSize != 8192 {
		t.Errorf("expected buffer size 8192, got %d", opts.BufferSize)
	}

	WithInitialCapacities(-10, -20)(opts)
	if opts.InitialKeyCap != DefaultIteratorKeyCap || opts.InitialValueCap != DefaultIteratorValueCap {
		t.Errorf("caps mutated by negative values: key=%d, val=%d", opts.InitialKeyCap, opts.InitialValueCap)
	}

	WithInitialCapacities(MaxIteratorKeyCap+1, MaxIteratorValueCap+1)(opts)
	if opts.InitialKeyCap != DefaultIteratorKeyCap || opts.InitialValueCap != DefaultIteratorValueCap {
		t.Errorf("caps mutated by overflow values: key=%d, val=%d", opts.InitialKeyCap, opts.InitialValueCap)
	}

	WithInitialCapacities(100, 200)(opts)
	if opts.InitialKeyCap != 100 || opts.InitialValueCap != 200 {
		t.Errorf("expected caps key=100 val=200, got key=%d val=%d", opts.InitialKeyCap, opts.InitialValueCap)
	}
}

// TestIterator_NilGuards verifies that methods called on a nil Iterator receiver
// return safe zero-values and do not trigger nil-pointer dereference panics.
func TestIterator_NilGuards(t *testing.T) {
	var iter *Iterator
	if iter.Next() {
		t.Error("Next should return false on nil receiver")
	}
	if iter.Key() != nil {
		t.Error("Key should return nil on nil receiver")
	}
	if iter.Value() != nil {
		t.Error("Value should return nil on nil receiver")
	}
	if iter.Opcode() != 0 {
		t.Error("Opcode should return 0 on nil receiver")
	}
	if iter.Error() != nil {
		t.Error("Error should return nil on nil receiver")
	}
	if err := iter.Close(); err != nil {
		t.Errorf("Close should return nil on nil receiver, got %v", err)
	}
}

// TestIterator_LifecycleAndBounds performs a standard full-lifecycle iteration
// over a mock SSTable file containing updates and deletes, verifying correct
// extraction of keys, values, and opcodes.
func TestIterator_LifecycleAndBounds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "iterator_test.sst")

	w, err := NewWriter(path, 3)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	_ = w.Add([]byte("a"), []byte("value-a"), OpcodePut)
	_ = w.Add([]byte("b"), []byte("value-b"), OpcodePut)
	_ = w.Add([]byte("c"), nil, OpcodeDelete)
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	iter, err := NewIterator(path)
	if err != nil {
		t.Fatalf("NewIterator: %v", err)
	}

	if !iter.Next() {
		t.Fatal("expected first entry")
	}
	if !bytes.Equal(iter.Key(), []byte("a")) || !bytes.Equal(iter.Value(), []byte("value-a")) || iter.Opcode() != OpcodePut {
		t.Errorf("first entry mismatch: key=%s val=%s op=%d", iter.Key(), iter.Value(), iter.Opcode())
	}

	if !iter.Next() {
		t.Fatal("expected second entry")
	}
	if !bytes.Equal(iter.Key(), []byte("b")) || !bytes.Equal(iter.Value(), []byte("value-b")) {
		t.Errorf("second entry mismatch")
	}

	if !iter.Next() {
		t.Fatal("expected third entry")
	}
	if !bytes.Equal(iter.Key(), []byte("c")) || len(iter.Value()) != 0 || iter.Opcode() != OpcodeDelete {
		t.Errorf("third entry mismatch")
	}

	if iter.Next() {
		t.Fatal("expected no more entries")
	}
	if err := iter.Error(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if err := iter.Close(); err != nil {
		t.Errorf("expected clean close, got %v", err)
	}

	if iter.Next() {
		t.Fatal("Next should return false after close")
	}
}

// TestIterator_CorruptedBoundary checks that the iterator detects corrupted entry lengths
// that exceed the SSTable data block boundaries, returning a corruption error.
func TestIterator_CorruptedBoundary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt_boundary.sst")

	w, err := NewWriter(path, 1)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	_ = w.Add([]byte("a"), []byte("value-a"), OpcodePut)
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	binary.LittleEndian.PutUint32(data[2:6], 999999)

	if err := os.WriteFile(path, data, 0666); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	iter, err := NewIterator(path)
	if err != nil {
		t.Fatalf("NewIterator: %v", err)
	}
	defer iter.Close()

	if iter.Next() {
		t.Fatal("Next should fail on corrupted boundary")
	}
	if !errors.Is(iter.Error(), ErrCorrupted) {
		t.Errorf("expected ErrCorrupted, got %v", iter.Error())
	}
}

// TestIterator_FileOpenFailure verifies that NewIterator fails with an appropriate OS error
// if the underlying SSTable file is removed or renamed before iterator initialization.
func TestIterator_FileOpenFailure(t *testing.T) {
	if _, err := NewIterator("nonexistent.sst"); err == nil {
		t.Error("expected error when opening missing file")
	}
}

// TestIterator_NextErrorPaths constructs mock iterator structures and asserts that Next()
// returns false and flags ErrUnexpectedEOF on truncated entry headers, keys, or values.
func TestIterator_NextErrorPaths(t *testing.T) {
	dir := t.TempDir()

	path1 := filepath.Join(dir, "err_header.dat")
	if err := os.WriteFile(path1, []byte{1, 2, 3}, 0666); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	f1, err := os.Open(path1)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f1.Close()
	iter1 := &Iterator{
		file:        f1,
		reader:      bufio.NewReaderSize(f1, 100),
		limitOffset: 7,
	}
	if iter1.Next() {
		t.Error("expected Next to return false")
	}
	if iter1.Error() == nil || !errors.Is(iter1.Error(), io.ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", iter1.Error())
	}

	path2 := filepath.Join(dir, "err_key.dat")
	var header2 [7]byte
	binary.LittleEndian.PutUint16(header2[0:2], 10)
	binary.LittleEndian.PutUint32(header2[2:6], 0)
	header2[6] = OpcodePut
	if err := os.WriteFile(path2, append(header2[:], []byte("abc")...), 0666); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	f2, err := os.Open(path2)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f2.Close()
	iter2 := &Iterator{
		file:        f2,
		reader:      bufio.NewReaderSize(f2, 100),
		limitOffset: 17,
	}
	if iter2.Next() {
		t.Error("expected Next to return false")
	}
	if iter2.Error() == nil || !errors.Is(iter2.Error(), io.ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", iter2.Error())
	}

	path3 := filepath.Join(dir, "err_val.dat")
	var header3 [7]byte
	binary.LittleEndian.PutUint16(header3[0:2], 0)
	binary.LittleEndian.PutUint32(header3[2:6], 10)
	header3[6] = OpcodePut
	if err := os.WriteFile(path3, append(header3[:], []byte("abc")...), 0666); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	f3, err := os.Open(path3)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f3.Close()
	iter3 := &Iterator{
		file:        f3,
		reader:      bufio.NewReaderSize(f3, 100),
		limitOffset: 17,
	}
	if iter3.Next() {
		t.Error("expected Next to return false")
	}
	if iter3.Error() == nil || !errors.Is(iter3.Error(), io.ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", iter3.Error())
	}

	path4 := filepath.Join(dir, "err_opcode.dat")
	var header4 [7]byte
	binary.LittleEndian.PutUint16(header4[0:2], 0)
	binary.LittleEndian.PutUint32(header4[2:6], 0)
	header4[6] = 99
	if err := os.WriteFile(path4, header4[:], 0666); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	f4, err := os.Open(path4)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f4.Close()
	iter4 := &Iterator{
		file:        f4,
		reader:      bufio.NewReaderSize(f4, 100),
		limitOffset: 7,
	}
	if iter4.Next() {
		t.Error("expected Next to return false")
	}
	if iter4.Error() == nil || !errors.Is(iter4.Error(), ErrCorrupted) {
		t.Errorf("expected ErrCorrupted, got %v", iter4.Error())
	}
}

// TestIterator_HeaderEOF asserts that Next() sets an ErrUnexpectedEOF if the file stream
// is completely empty (0 bytes read) while the iterator is below limitOffset.
func TestIterator_HeaderEOF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eof_header.dat")
	if err := os.WriteFile(path, []byte{}, 0666); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	iter := &Iterator{
		file:        f,
		reader:      bufio.NewReader(f),
		limitOffset: 7,
	}
	if iter.Next() {
		t.Error("expected Next to return false")
	}
	if !errors.Is(iter.Error(), io.ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", iter.Error())
	}
}

// TestIterator_NewIteratorErrors verifies that NewIterator returns appropriate errors
// when opening corrupted files or files with invalid sizes and footers.
func TestIterator_NewIteratorErrors(t *testing.T) {
	dir := t.TempDir()

	pathSmall := filepath.Join(dir, "small.sst")
	if err := os.WriteFile(pathSmall, []byte{1, 2, 3}, 0666); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := NewIterator(pathSmall); err == nil {
		t.Error("expected error for file too small for footer")
	}

	pathBadMagic := filepath.Join(dir, "bad_magic.sst")
	badMagicBytes := make([]byte, footerSize)
	binary.LittleEndian.PutUint32(badMagicBytes[footerMagicOffset:footerMagicOffset+footerMagicSize], 0x12345678)
	if err := os.WriteFile(pathBadMagic, badMagicBytes, 0666); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := NewIterator(pathBadMagic); !errors.Is(err, ErrInvalidMagic) {
		t.Errorf("expected ErrInvalidMagic, got %v", err)
	}

	pathBadOffsets := filepath.Join(dir, "bad_offsets.sst")
	badOffsetsBytes := make([]byte, footerSize)
	binary.LittleEndian.PutUint32(badOffsetsBytes[footerMagicOffset:footerMagicOffset+footerMagicSize], magicNumber)
	binary.LittleEndian.PutUint64(badOffsetsBytes[footerIndexOffsetOffset:footerBloomOffsetOffset], 100)
	binary.LittleEndian.PutUint64(badOffsetsBytes[footerBloomOffsetOffset:footerBloomNumHashesOffset], 50)
	if err := os.WriteFile(pathBadOffsets, badOffsetsBytes, 0666); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := NewIterator(pathBadOffsets); !errors.Is(err, ErrCorrupted) {
		t.Errorf("expected ErrCorrupted, got %v", err)
	}
}
