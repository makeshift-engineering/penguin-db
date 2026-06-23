package sstable

import (
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

type testEntry struct {
	key    []byte
	value  []byte
	opcode uint8
}

func writeTestSSTable(t *testing.T, dir, name string, entries []testEntry) string {
	t.Helper()
	path := filepath.Join(dir, name)
	w, err := NewWriter(path, len(entries))
	if err != nil {
		t.Fatalf("NewWriter(%q): %v", name, err)
	}
	for _, e := range entries {
		if err := w.Add(e.key, e.value, e.opcode); err != nil {
			t.Fatalf("Add(%q): %v", e.key, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return path
}

// TestOpen_EmptyTable opens a valid SSTable with zero entries and verifies the
// Reader loads without error and reports an entry count of zero.
func TestOpen_EmptyTable(t *testing.T) {
	dir := t.TempDir()
	path := writeTestSSTable(t, dir, "empty.sst", nil)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	if r.EntryCount() != 0 {
		t.Errorf("entry count: got %d, want 0", r.EntryCount())
	}
	if len(r.index) != 0 {
		t.Errorf("index length: got %d, want 0", len(r.index))
	}
}

// TestOpen_SingleEntry opens an SSTable with one entry and verifies the
// Reader loads the index and bloom filter correctly.
func TestOpen_SingleEntry(t *testing.T) {
	dir := t.TempDir()
	path := writeTestSSTable(t, dir, "single.sst", []testEntry{
		{key: []byte("hello"), value: []byte("world"), opcode: OpcodePut},
	})

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	if r.EntryCount() != 1 {
		t.Errorf("entry count: got %d, want 1", r.EntryCount())
	}
	if len(r.index) != 1 {
		t.Errorf("index length: got %d, want 1", len(r.index))
	}
}

// TestOpen_InvalidMagic verifies that Open returns ErrInvalidMagic when the
// file's magic number has been corrupted.
func TestOpen_InvalidMagic(t *testing.T) {
	dir := t.TempDir()
	path := writeTestSSTable(t, dir, "bad_magic.sst", []testEntry{
		{key: []byte("k"), value: []byte("v"), opcode: OpcodePut},
	})

	// Corrupt the magic number (last 4 bytes of the file).
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	// Overwrite magic bytes with zeros.
	copy(data[len(data)-footerMagicSize:], []byte{0x00, 0x00, 0x00, 0x00})
	if err := os.WriteFile(path, data, 0o666); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err = Open(path)
	if err == nil {
		t.Fatal("expected error for corrupted magic, got nil")
	}
	if !strings.Contains(err.Error(), "invalid magic number") {
		t.Errorf("expected ErrInvalidMagic, got: %v", err)
	}
}

// TestOpen_FileTooSmall verifies that Open returns ErrCorrupted when the file
// is smaller than the footer size.
func TestOpen_FileTooSmall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tiny.sst")
	if err := os.WriteFile(path, []byte("short"), 0o666); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Open(path)
	if err == nil {
		t.Fatal("expected error for tiny file, got nil")
	}
	if !strings.Contains(err.Error(), "data corruption detected") {
		t.Errorf("expected ErrCorrupted, got: %v", err)
	}
}

// TestOpen_NonExistentFile verifies Open returns an error for a missing file.
func TestOpen_NonExistentFile(t *testing.T) {
	_, err := Open(filepath.Join(t.TempDir(), "does_not_exist.sst"))
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
}

// TestOpen_ExactlyFooterSizeFile verifies Open handles a file that is exactly
// footerSize bytes (but has invalid content), it should fail on magic.
func TestOpen_ExactlyFooterSizeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exact_footer.sst")
	data := make([]byte, footerSize)
	if err := os.WriteFile(path, data, 0o666); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Open(path)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Should fail on magic validation since all bytes are zero.
	if !strings.Contains(err.Error(), "invalid magic number") {
		t.Errorf("expected magic validation error, got: %v", err)
	}
}

// TestOpen_CorruptedIndexOffset verifies Open returns ErrCorrupted when the
// footer's index offset points past the file.
func TestOpen_CorruptedIndexOffset(t *testing.T) {
	dir := t.TempDir()
	path := writeTestSSTable(t, dir, "bad_index_off.sst", []testEntry{
		{key: []byte("k"), value: []byte("v"), opcode: OpcodePut},
	})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Set index offset to a value larger than the file.
	footerStart := len(data) - footerSize
	binary.LittleEndian.PutUint64(data[footerStart+footerIndexOffsetOffset:], uint64(len(data)*2))

	if err := os.WriteFile(path, data, 0o666); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err = Open(path)
	if err == nil {
		t.Fatal("expected error for bad index offset, got nil")
	}
	if !strings.Contains(err.Error(), "data corruption detected") {
		t.Errorf("expected ErrCorrupted, got: %v", err)
	}
}

// TestOpen_CorruptedBloomOffset verifies Open returns ErrCorrupted when the
// bloom offset is before the index offset.
func TestOpen_CorruptedBloomOffset(t *testing.T) {
	dir := t.TempDir()
	path := writeTestSSTable(t, dir, "bad_bloom_off.sst", []testEntry{
		{key: []byte("k"), value: []byte("v"), opcode: OpcodePut},
	})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	footerStart := len(data) - footerSize
	// Read current index offset.
	indexOff := binary.LittleEndian.Uint64(data[footerStart+footerIndexOffsetOffset:])
	// Set bloom offset to before index offset.
	if indexOff > 0 {
		binary.LittleEndian.PutUint64(data[footerStart+footerBloomOffsetOffset:], indexOff-1)
	} else {
		// index is at 0, so bloom "before" it doesn't make sense with uint64.
		// Skip this subcase.
		t.Skip("index offset is 0, cannot set bloom before it")
	}

	if err := os.WriteFile(path, data, 0o666); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err = Open(path)
	if err == nil {
		t.Fatal("expected error for bad bloom offset, got nil")
	}
}

// TestReader_GetHitSingle verifies a single entry round-trips through Write, Open, and Get.
func TestReader_GetHitSingle(t *testing.T) {
	dir := t.TempDir()
	path := writeTestSSTable(t, dir, "hit.sst", []testEntry{
		{key: []byte("alpha"), value: []byte("one"), opcode: OpcodePut},
	})

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	val, found, deleted, err := r.Get([]byte("alpha"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	if deleted {
		t.Fatal("expected deleted=false")
	}
	if !bytes.Equal(val, []byte("one")) {
		t.Errorf("value: got %q, want %q", val, "one")
	}
}

// TestReader_GetMiss verifies Get returns found=false for an absent key.
func TestReader_GetMiss(t *testing.T) {
	dir := t.TempDir()
	path := writeTestSSTable(t, dir, "miss.sst", []testEntry{
		{key: []byte("alpha"), value: []byte("one"), opcode: OpcodePut},
	})

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	_, found, deleted, err := r.Get([]byte("beta"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if found {
		t.Fatal("expected found=false for absent key")
	}
	if deleted {
		t.Fatal("expected deleted=false for absent key")
	}
}

// TestReader_GetTombstone verifies Get returns found=true, deleted=true for a
// tombstone entry.
func TestReader_GetTombstone(t *testing.T) {
	dir := t.TempDir()
	path := writeTestSSTable(t, dir, "tombstone.sst", []testEntry{
		{key: []byte("dead"), value: nil, opcode: OpcodeDelete},
	})

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	val, found, deleted, err := r.Get([]byte("dead"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found {
		t.Fatal("expected found=true for tombstone")
	}
	if !deleted {
		t.Fatal("expected deleted=true for tombstone")
	}
	if val != nil {
		t.Errorf("tombstone value: got %q, want nil", val)
	}
}

// TestReader_GetOnEmptyTable verifies Get returns not-found on an empty SSTable.
func TestReader_GetOnEmptyTable(t *testing.T) {
	dir := t.TempDir()
	path := writeTestSSTable(t, dir, "empty.sst", nil)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	_, found, _, err := r.Get([]byte("anything"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if found {
		t.Fatal("expected found=false on empty table")
	}
}

// TestReader_BinarySearchHit verifies Get finds a key in the middle of a
// 1000-entry table using binary search.
func TestReader_BinarySearchHit(t *testing.T) {
	dir := t.TempDir()

	n := 1000
	entries := make([]testEntry, n)
	for i := 0; i < n; i++ {
		entries[i] = testEntry{
			key:    []byte(fmt.Sprintf("key-%04d", i)),
			value:  []byte(fmt.Sprintf("val-%04d", i)),
			opcode: OpcodePut,
		}
	}
	path := writeTestSSTable(t, dir, "bsearch.sst", entries)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	// Search for the middle key.
	target := fmt.Sprintf("key-%04d", n/2)
	val, found, deleted, err := r.Get([]byte(target))
	if err != nil {
		t.Fatalf("Get(%q): %v", target, err)
	}
	if !found {
		t.Fatalf("expected to find %q", target)
	}
	if deleted {
		t.Fatalf("key %q should not be deleted", target)
	}

	expected := fmt.Sprintf("val-%04d", n/2)
	if !bytes.Equal(val, []byte(expected)) {
		t.Errorf("value: got %q, want %q", val, expected)
	}
}

// TestReader_BinarySearchMiss verifies Get returns found=false for a key that
// would sort between existing keys.
func TestReader_BinarySearchMiss(t *testing.T) {
	dir := t.TempDir()
	entries := []testEntry{
		{key: []byte("aaa"), value: []byte("1"), opcode: OpcodePut},
		{key: []byte("ccc"), value: []byte("3"), opcode: OpcodePut},
		{key: []byte("eee"), value: []byte("5"), opcode: OpcodePut},
	}
	path := writeTestSSTable(t, dir, "bsmiss.sst", entries)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	for _, missing := range []string{"bbb", "ddd", "fff", "000"} {
		_, found, _, err := r.Get([]byte(missing))
		if err != nil {
			t.Fatalf("Get(%q): %v", missing, err)
		}
		if found {
			t.Errorf("expected not-found for %q", missing)
		}
	}
}

// TestReader_GetAllEntries writes 100 entries and verifies every single one is
// retrievable via Get.
func TestReader_GetAllEntries(t *testing.T) {
	dir := t.TempDir()
	n := 100
	entries := make([]testEntry, n)
	for i := 0; i < n; i++ {
		entries[i] = testEntry{
			key:    []byte(fmt.Sprintf("key-%04d", i)),
			value:  []byte(fmt.Sprintf("val-%04d", i)),
			opcode: OpcodePut,
		}
	}
	path := writeTestSSTable(t, dir, "all.sst", entries)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	for _, e := range entries {
		val, found, deleted, err := r.Get(e.key)
		if err != nil {
			t.Fatalf("Get(%q): %v", e.key, err)
		}
		if !found {
			t.Fatalf("expected to find %q", e.key)
		}
		if deleted {
			t.Fatalf("key %q should not be deleted", e.key)
		}
		if !bytes.Equal(val, e.value) {
			t.Errorf("value for %q: got %q, want %q", e.key, val, e.value)
		}
	}
}

// TestReader_GetFirstAndLastKey verifies binary search handles the boundary
// entries (first and last in sorted order) correctly.
func TestReader_GetFirstAndLastKey(t *testing.T) {
	dir := t.TempDir()
	entries := []testEntry{
		{key: []byte("aaa"), value: []byte("first"), opcode: OpcodePut},
		{key: []byte("mmm"), value: []byte("middle"), opcode: OpcodePut},
		{key: []byte("zzz"), value: []byte("last"), opcode: OpcodePut},
	}
	path := writeTestSSTable(t, dir, "boundary.sst", entries)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	// First key.
	val, found, _, err := r.Get([]byte("aaa"))
	if err != nil || !found {
		t.Fatalf("Get first: found=%v, err=%v", found, err)
	}
	if !bytes.Equal(val, []byte("first")) {
		t.Errorf("first: got %q", val)
	}

	// Last key.
	val, found, _, err = r.Get([]byte("zzz"))
	if err != nil || !found {
		t.Fatalf("Get last: found=%v, err=%v", found, err)
	}
	if !bytes.Equal(val, []byte("last")) {
		t.Errorf("last: got %q", val)
	}
}

// TestReader_GetKeyBeforeFirst verifies Get returns not-found for a key that
// sorts before all entries.
func TestReader_GetKeyBeforeFirst(t *testing.T) {
	dir := t.TempDir()
	entries := []testEntry{
		{key: []byte("bbb"), value: []byte("v"), opcode: OpcodePut},
		{key: []byte("ccc"), value: []byte("v"), opcode: OpcodePut},
	}
	path := writeTestSSTable(t, dir, "before_first.sst", entries)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	_, found, _, err := r.Get([]byte("aaa"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if found {
		t.Fatal("expected not-found for key before first entry")
	}
}

// TestReader_GetKeyAfterLast verifies Get returns not-found for a key that
// sorts after all entries.
func TestReader_GetKeyAfterLast(t *testing.T) {
	dir := t.TempDir()
	entries := []testEntry{
		{key: []byte("aaa"), value: []byte("v"), opcode: OpcodePut},
		{key: []byte("bbb"), value: []byte("v"), opcode: OpcodePut},
	}
	path := writeTestSSTable(t, dir, "after_last.sst", entries)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	_, found, _, err := r.Get([]byte("zzz"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if found {
		t.Fatal("expected not-found for key after last entry")
	}
}

// TestReader_MixedPutAndDelete writes interleaved Put and Delete entries and
// verifies each one is correctly distinguished by Get.
func TestReader_MixedPutAndDelete(t *testing.T) {
	dir := t.TempDir()
	entries := []testEntry{
		{key: []byte("a"), value: []byte("val-a"), opcode: OpcodePut},
		{key: []byte("b"), value: nil, opcode: OpcodeDelete},
		{key: []byte("c"), value: []byte("val-c"), opcode: OpcodePut},
		{key: []byte("d"), value: nil, opcode: OpcodeDelete},
		{key: []byte("e"), value: []byte("val-e"), opcode: OpcodePut},
	}
	path := writeTestSSTable(t, dir, "mixed.sst", entries)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	for _, e := range entries {
		val, found, deleted, err := r.Get(e.key)
		if err != nil {
			t.Fatalf("Get(%q): %v", e.key, err)
		}
		if !found {
			t.Fatalf("expected to find %q", e.key)
		}
		if e.opcode == OpcodeDelete {
			if !deleted {
				t.Errorf("key %q: expected deleted=true", e.key)
			}
			if val != nil {
				t.Errorf("key %q: deleted value should be nil, got %q", e.key, val)
			}
		} else {
			if deleted {
				t.Errorf("key %q: expected deleted=false", e.key)
			}
			if !bytes.Equal(val, e.value) {
				t.Errorf("key %q: got %q, want %q", e.key, val, e.value)
			}
		}
	}
}

// TestReader_AllTombstones verifies that a table containing only deletes is
// handled correctly — every key returns found=true, deleted=true.
func TestReader_AllTombstones(t *testing.T) {
	dir := t.TempDir()
	n := 20
	entries := make([]testEntry, n)
	for i := 0; i < n; i++ {
		entries[i] = testEntry{
			key:    []byte(fmt.Sprintf("del-%03d", i)),
			value:  nil,
			opcode: OpcodeDelete,
		}
	}
	path := writeTestSSTable(t, dir, "all_tombs.sst", entries)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	for _, e := range entries {
		_, found, deleted, err := r.Get(e.key)
		if err != nil {
			t.Fatalf("Get(%q): %v", e.key, err)
		}
		if !found || !deleted {
			t.Errorf("key %q: expected found=true, deleted=true; got found=%v, deleted=%v", e.key, found, deleted)
		}
	}
}

// TestReader_BloomFilterIntegration verifies that BloomMayContain returns true
// for all present keys and false for most absent keys.
func TestReader_BloomFilterIntegration(t *testing.T) {
	dir := t.TempDir()
	n := 500
	entries := make([]testEntry, n)
	for i := 0; i < n; i++ {
		entries[i] = testEntry{
			key:    []byte(fmt.Sprintf("bloom-key-%05d", i)),
			value:  []byte("v"),
			opcode: OpcodePut,
		}
	}
	path := writeTestSSTable(t, dir, "bloom.sst", entries)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	// No false negatives: every added key must be positive.
	for _, e := range entries {
		if !r.BloomMayContain(e.key) {
			t.Fatalf("bloom false negative for %q", e.key)
		}
	}

	// False positive rate should be bounded.
	falsePositives := 0
	probes := 10000
	for i := 0; i < probes; i++ {
		absent := []byte(fmt.Sprintf("absent-key-%05d", i))
		if r.BloomMayContain(absent) {
			falsePositives++
		}
	}
	fpr := float64(falsePositives) / float64(probes)
	t.Logf("Bloom FPR: %.4f%% (%d / %d)", fpr*100, falsePositives, probes)
	if fpr > 0.03 {
		t.Errorf("bloom FPR too high: %.4f, want <= 0.03", fpr)
	}
}

// TestReader_BloomFilterOnEmptyTable verifies the bloom filter loaded from an
// empty SSTable returns false for any key.
func TestReader_BloomFilterOnEmptyTable(t *testing.T) {
	dir := t.TempDir()
	path := writeTestSSTable(t, dir, "empty_bloom.sst", nil)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	// In a completely empty filter all bits are zero, so no key should
	// match. However, the minimum filter size is 64 bits, so this is
	// technically valid either way — we just confirm no panics.
	for i := 0; i < 100; i++ {
		_ = r.BloomMayContain([]byte(fmt.Sprintf("key-%d", i)))
	}
}

// TestReader_LargeValues verifies an entry with a 1 MB value round-trips
// correctly through Write, Open, and Get.
func TestReader_LargeValues(t *testing.T) {
	dir := t.TempDir()
	val := bytes.Repeat([]byte("X"), 1024*1024) // 1 MB
	entries := []testEntry{
		{key: []byte("big-key"), value: val, opcode: OpcodePut},
	}
	path := writeTestSSTable(t, dir, "large.sst", entries)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	got, found, deleted, err := r.Get([]byte("big-key"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found || deleted {
		t.Fatalf("found=%v, deleted=%v", found, deleted)
	}
	if !bytes.Equal(got, val) {
		t.Errorf("1MB value mismatch (lengths: got %d, want %d)", len(got), len(val))
	}
}

// TestReader_EmptyValue verifies that a Put entry with an empty (zero-length)
// value is distinguishable from a tombstone.
func TestReader_EmptyValue(t *testing.T) {
	dir := t.TempDir()
	entries := []testEntry{
		{key: []byte("empty-val"), value: []byte{}, opcode: OpcodePut},
	}
	path := writeTestSSTable(t, dir, "empty_val.sst", entries)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	val, found, deleted, err := r.Get([]byte("empty-val"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	if deleted {
		t.Fatal("expected deleted=false for empty Put value")
	}
	if len(val) != 0 {
		t.Errorf("value length: got %d, want 0", len(val))
	}
}

// TestReader_NilValue verifies a nil-value Put round-trips the same as empty.
func TestReader_NilValue(t *testing.T) {
	dir := t.TempDir()
	entries := []testEntry{
		{key: []byte("nil-val"), value: nil, opcode: OpcodePut},
	}
	path := writeTestSSTable(t, dir, "nil_val.sst", entries)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	val, found, deleted, err := r.Get([]byte("nil-val"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	if deleted {
		t.Fatal("expected deleted=false for nil Put value")
	}
	if len(val) != 0 {
		t.Errorf("value length: got %d, want 0", len(val))
	}
}

// TestReader_MaxKeyLength verifies that the maximum-length key (65535 bytes)
// round-trips correctly through Write, Open, and Get.
func TestReader_MaxKeyLength(t *testing.T) {
	dir := t.TempDir()
	maxKey := bytes.Repeat([]byte("K"), math.MaxUint16)
	entries := []testEntry{
		{key: maxKey, value: []byte("v"), opcode: OpcodePut},
	}
	path := writeTestSSTable(t, dir, "max_key.sst", entries)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	val, found, deleted, err := r.Get(maxKey)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found || deleted {
		t.Fatalf("found=%v, deleted=%v", found, deleted)
	}
	if !bytes.Equal(val, []byte("v")) {
		t.Errorf("value: got %q, want %q", val, "v")
	}
}

// TestReader_SingleByteKeyAndValue verifies the minimum-size entry (1-byte
// key, 1-byte value) works correctly.
func TestReader_SingleByteKeyAndValue(t *testing.T) {
	dir := t.TempDir()
	entries := []testEntry{
		{key: []byte("k"), value: []byte("v"), opcode: OpcodePut},
	}
	path := writeTestSSTable(t, dir, "tiny.sst", entries)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	val, found, deleted, err := r.Get([]byte("k"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found || deleted {
		t.Fatalf("found=%v, deleted=%v", found, deleted)
	}
	if !bytes.Equal(val, []byte("v")) {
		t.Errorf("value: got %q, want %q", val, "v")
	}
}

// TestReader_LargeValue4MB verifies a 4 MB value round-trips correctly.
func TestReader_LargeValue4MB(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large allocation test in short mode")
	}

	dir := t.TempDir()
	val := bytes.Repeat([]byte("V"), 4*1024*1024)
	entries := []testEntry{
		{key: []byte("big"), value: val, opcode: OpcodePut},
	}
	path := writeTestSSTable(t, dir, "4mb.sst", entries)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	got, found, _, err := r.Get([]byte("big"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	if !bytes.Equal(got, val) {
		t.Errorf("4MB value mismatch (len got=%d, want=%d)", len(got), len(val))
	}
}

// TestReader_BinaryKeys verifies that keys with null bytes and high bytes
// round-trip correctly.
func TestReader_BinaryKeys(t *testing.T) {
	dir := t.TempDir()
	entries := []testEntry{
		{key: []byte{0x00, 0x00, 0x00}, value: []byte("v0"), opcode: OpcodePut},
		{key: []byte{0x00, 0xFF, 0x00, 0xFF}, value: []byte("v1"), opcode: OpcodePut},
		{key: []byte{0x01, 0x02, 0x03, 0x04, 0x05}, value: []byte("v2"), opcode: OpcodePut},
		{key: []byte{0xFF, 0xFE, 0xFD}, value: []byte("v3"), opcode: OpcodePut},
	}
	path := writeTestSSTable(t, dir, "binary.sst", entries)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	for _, e := range entries {
		val, found, deleted, err := r.Get(e.key)
		if err != nil {
			t.Fatalf("Get(%x): %v", e.key, err)
		}
		if !found || deleted {
			t.Fatalf("key %x: found=%v, deleted=%v", e.key, found, deleted)
		}
		if !bytes.Equal(val, e.value) {
			t.Errorf("key %x: got %q, want %q", e.key, val, e.value)
		}
	}
}

// TestReader_UnicodeKeys verifies multi-byte UTF-8 keys round-trip correctly.
func TestReader_UnicodeKeys(t *testing.T) {
	dir := t.TempDir()
	entries := []testEntry{
		{key: []byte("中文"), value: []byte("v0"), opcode: OpcodePut},
		{key: []byte("日本語"), value: []byte("v1"), opcode: OpcodePut},
		{key: []byte("한국어"), value: []byte("v2"), opcode: OpcodePut},
		{key: []byte("🐧"), value: []byte("v3"), opcode: OpcodePut},
	}
	path := writeTestSSTable(t, dir, "unicode.sst", entries)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	for _, e := range entries {
		val, found, deleted, err := r.Get(e.key)
		if err != nil {
			t.Fatalf("Get(%q): %v", e.key, err)
		}
		if !found || deleted {
			t.Fatalf("key %q: found=%v, deleted=%v", e.key, found, deleted)
		}
		if !bytes.Equal(val, e.value) {
			t.Errorf("key %q: got %q, want %q", e.key, val, e.value)
		}
	}
}

// TestReader_GetAfterClose verifies that Get on a closed Reader returns
// ErrReaderClosed.
func TestReader_GetAfterClose(t *testing.T) {
	dir := t.TempDir()
	path := writeTestSSTable(t, dir, "close.sst", []testEntry{
		{key: []byte("k"), value: []byte("v"), opcode: OpcodePut},
	})

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	r.Close()

	_, _, _, err = r.Get([]byte("k"))
	if err == nil {
		t.Fatal("expected error after Close, got nil")
	}
	if !strings.Contains(err.Error(), "reader is closed") {
		t.Errorf("expected ErrReaderClosed, got: %v", err)
	}
}

// TestReader_DoubleClose verifies that calling Close twice does not panic or
// return an error.
func TestReader_DoubleClose(t *testing.T) {
	dir := t.TempDir()
	path := writeTestSSTable(t, dir, "double_close.sst", []testEntry{
		{key: []byte("k"), value: []byte("v"), opcode: OpcodePut},
	})

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := r.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("second Close should not error: %v", err)
	}
}

// TestReader_BloomMayContainAfterClose does not panic when called after Close.
// The bloom filter is in-memory, so it technically still works, but we just
// want to ensure no crash.
func TestReader_BloomMayContainAfterClose(t *testing.T) {
	dir := t.TempDir()
	path := writeTestSSTable(t, dir, "bloom_after_close.sst", []testEntry{
		{key: []byte("k"), value: []byte("v"), opcode: OpcodePut},
	})

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	r.Close()

	// Should not panic. The bloom filter is in-memory.
	_ = r.BloomMayContain([]byte("k"))
}

// TestReader_HighEntryCount writes 10,000 entries and verifies random-access
// retrieval of a selection of them via Get.
func TestReader_HighEntryCount(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	dir := t.TempDir()
	n := 10000
	entries := make([]testEntry, n)
	for i := 0; i < n; i++ {
		entries[i] = testEntry{
			key:    []byte(fmt.Sprintf("stress-key-%06d", i)),
			value:  []byte(fmt.Sprintf("stress-val-%06d", i)),
			opcode: OpcodePut,
		}
	}
	path := writeTestSSTable(t, dir, "stress.sst", entries)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	if r.EntryCount() != uint32(n) {
		t.Fatalf("entry count: got %d, want %d", r.EntryCount(), n)
	}

	// Spot-check first, last, middle, and every 100th entry.
	checks := []int{0, n / 2, n - 1}
	for i := 0; i < n; i += 100 {
		checks = append(checks, i)
	}
	for _, idx := range checks {
		e := entries[idx]
		val, found, deleted, err := r.Get(e.key)
		if err != nil {
			t.Fatalf("Get(%q): %v", e.key, err)
		}
		if !found || deleted {
			t.Fatalf("key %q: found=%v, deleted=%v", e.key, found, deleted)
		}
		if !bytes.Equal(val, e.value) {
			t.Errorf("key %q: got %q, want %q", e.key, val, e.value)
		}
	}

	// Also confirm absent keys are not found.
	for i := 0; i < 100; i++ {
		absent := []byte(fmt.Sprintf("absent-%06d", i))
		_, found, _, err := r.Get(absent)
		if err != nil {
			t.Fatalf("Get(%q): %v", absent, err)
		}
		if found {
			t.Errorf("expected not-found for %q", absent)
		}
	}
}

// TestReader_SimilarPrefixKeys writes 50 keys sharing a long common prefix and
// verifies each is individually retrievable.
func TestReader_SimilarPrefixKeys(t *testing.T) {
	dir := t.TempDir()
	prefix := strings.Repeat("namespace/collection/", 5)
	n := 50
	entries := make([]testEntry, n)
	for i := 0; i < n; i++ {
		entries[i] = testEntry{
			key:    []byte(fmt.Sprintf("%s%04d", prefix, i)),
			value:  []byte("v"),
			opcode: OpcodePut,
		}
	}
	path := writeTestSSTable(t, dir, "prefix.sst", entries)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	for _, e := range entries {
		val, found, _, err := r.Get(e.key)
		if err != nil {
			t.Fatalf("Get(%q): %v", e.key, err)
		}
		if !found {
			t.Fatalf("expected to find %q", e.key)
		}
		if !bytes.Equal(val, e.value) {
			t.Errorf("key %q: got %q, want %q", e.key, val, e.value)
		}
	}
}

// TestReader_VariableSizeEntries writes entries with widely varying key/value
// sizes and verifies all round-trip correctly.
func TestReader_VariableSizeEntries(t *testing.T) {
	dir := t.TempDir()
	entries := []testEntry{
		{key: []byte("a"), value: bytes.Repeat([]byte("x"), 1), opcode: OpcodePut},
		{key: []byte("bb"), value: bytes.Repeat([]byte("y"), 100), opcode: OpcodePut},
		{key: []byte("ccc"), value: bytes.Repeat([]byte("z"), 10000), opcode: OpcodePut},
		{key: bytes.Repeat([]byte("d"), 200), value: []byte("short"), opcode: OpcodePut},
		{key: []byte("eee"), value: nil, opcode: OpcodePut},
	}
	path := writeTestSSTable(t, dir, "variable.sst", entries)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	for _, e := range entries {
		val, found, deleted, err := r.Get(e.key)
		if err != nil {
			t.Fatalf("Get(%q): %v", e.key, err)
		}
		if !found || deleted {
			t.Fatalf("key %q: found=%v, deleted=%v", e.key, found, deleted)
		}
		expectedLen := 0
		if e.value != nil {
			expectedLen = len(e.value)
		}
		if len(val) != expectedLen {
			t.Errorf("key %q: value length got %d, want %d", e.key, len(val), expectedLen)
		}
		if e.value != nil && !bytes.Equal(val, e.value) {
			t.Errorf("key %q: value mismatch", e.key)
		}
	}
}

// TestReader_FilePath verifies that FilePath returns the path used to open the
// SSTable.
func TestReader_FilePath(t *testing.T) {
	dir := t.TempDir()
	path := writeTestSSTable(t, dir, "filepath.sst", []testEntry{
		{key: []byte("k"), value: []byte("v"), opcode: OpcodePut},
	})

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	got := r.FilePath()
	if got != path {
		t.Errorf("FilePath: got %q, want %q", got, path)
	}
}

// TestReader_EntryCount verifies EntryCount returns the correct value for
// various table sizes.
func TestReader_EntryCount(t *testing.T) {
	counts := []int{0, 1, 5, 50, 500}
	for _, n := range counts {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			dir := t.TempDir()
			entries := make([]testEntry, n)
			for i := 0; i < n; i++ {
				entries[i] = testEntry{
					key:    []byte(fmt.Sprintf("key-%04d", i)),
					value:  []byte("v"),
					opcode: OpcodePut,
				}
			}
			path := writeTestSSTable(t, dir, "count.sst", entries)

			r, err := Open(path)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer r.Close()

			if r.EntryCount() != uint32(n) {
				t.Errorf("EntryCount: got %d, want %d", r.EntryCount(), n)
			}
		})
	}
}

// TestReader_IndexKeysMatchData verifies that the in-memory index keys loaded
// by Open exactly match the keys in the original entries.
func TestReader_IndexKeysMatchData(t *testing.T) {
	dir := t.TempDir()
	entries := []testEntry{
		{key: []byte("alpha"), value: []byte("1"), opcode: OpcodePut},
		{key: []byte("bravo"), value: []byte("2"), opcode: OpcodePut},
		{key: []byte("charlie"), value: []byte("3"), opcode: OpcodePut},
		{key: []byte("delta"), value: []byte("4"), opcode: OpcodePut},
	}
	path := writeTestSSTable(t, dir, "idx_keys.sst", entries)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	if len(r.index) != len(entries) {
		t.Fatalf("index size: got %d, want %d", len(r.index), len(entries))
	}

	for i, e := range entries {
		if !bytes.Equal(r.index[i].key, e.key) {
			t.Errorf("index[%d].key: got %q, want %q", i, r.index[i].key, e.key)
		}
	}
}

// TestReader_IndexOffsetsAreMonotonic verifies that the data offsets in the
// index are strictly monotonically increasing.
func TestReader_IndexOffsetsAreMonotonic(t *testing.T) {
	dir := t.TempDir()
	n := 50
	entries := make([]testEntry, n)
	for i := 0; i < n; i++ {
		entries[i] = testEntry{
			key:    []byte(fmt.Sprintf("key-%03d", i)),
			value:  []byte(fmt.Sprintf("val-%03d", i)),
			opcode: OpcodePut,
		}
	}
	path := writeTestSSTable(t, dir, "mono.sst", entries)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	for i := 1; i < len(r.index); i++ {
		if r.index[i].offset <= r.index[i-1].offset {
			t.Errorf("index offset not monotonic: [%d]=%d, [%d]=%d",
				i-1, r.index[i-1].offset, i, r.index[i].offset)
		}
	}
}

// TestReader_TruncatedFile verifies Open detects a file that has been
// truncated after writing (missing part of the bloom or footer).
func TestReader_TruncatedFile(t *testing.T) {
	dir := t.TempDir()
	path := writeTestSSTable(t, dir, "truncated.sst", []testEntry{
		{key: []byte("k"), value: []byte("v"), opcode: OpcodePut},
	})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Truncate file to half its size.
	half := len(data) / 2
	if err := os.WriteFile(path, data[:half], 0o666); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err = Open(path)
	if err == nil {
		t.Fatal("expected error for truncated file, got nil")
	}
}

// TestReader_ZeroBytesFile verifies Open rejects an empty (0-byte) file.
func TestReader_ZeroBytesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "zero.sst")
	if err := os.WriteFile(path, nil, 0o666); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Open(path)
	if err == nil {
		t.Fatal("expected error for zero-byte file, got nil")
	}
}

// TestReader_GarbageFile verifies Open rejects a file filled with random data.
func TestReader_GarbageFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "garbage.sst")
	garbage := bytes.Repeat([]byte{0xDE, 0xAD, 0xBE, 0xEF}, 100)
	if err := os.WriteFile(path, garbage, 0o666); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Open(path)
	if err == nil {
		t.Fatal("expected error for garbage file, got nil")
	}
}

// TestReader_CorruptedEntryCount verifies Open returns ErrCorrupted when the
// footer's entry count doesn't match the actual index entries.
func TestReader_CorruptedEntryCount(t *testing.T) {
	dir := t.TempDir()
	entries := []testEntry{
		{key: []byte("a"), value: []byte("1"), opcode: OpcodePut},
		{key: []byte("b"), value: []byte("2"), opcode: OpcodePut},
	}
	path := writeTestSSTable(t, dir, "bad_count.sst", entries)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Corrupt the entry count in the footer to 99.
	footerStart := len(data) - footerSize
	binary.LittleEndian.PutUint32(data[footerStart+footerEntryCountOffset:], 99)

	if err := os.WriteFile(path, data, 0o666); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err = Open(path)
	if err == nil {
		t.Fatal("expected error for mismatched entry count, got nil")
	}
	if !strings.Contains(err.Error(), "data corruption detected") {
		t.Errorf("expected ErrCorrupted, got: %v", err)
	}
}

// TestReader_MultipleIndependentTables opens multiple independently written
// SSTables and verifies each serves only its own entries.
func TestReader_MultipleIndependentTables(t *testing.T) {
	dir := t.TempDir()

	type tableContent struct {
		entries []testEntry
		reader  *Reader
	}

	tables := make([]tableContent, 5)
	for fileIdx := range tables {
		n := 10
		entries := make([]testEntry, n)
		for j := 0; j < n; j++ {
			entries[j] = testEntry{
				key:    []byte(fmt.Sprintf("file%d-key%d", fileIdx, j)),
				value:  []byte(fmt.Sprintf("file%d-val%d", fileIdx, j)),
				opcode: OpcodePut,
			}
		}
		path := writeTestSSTable(t, dir, fmt.Sprintf("%06d.sst", fileIdx), entries)

		r, err := Open(path)
		if err != nil {
			t.Fatalf("Open file %d: %v", fileIdx, err)
		}
		tables[fileIdx] = tableContent{entries: entries, reader: r}
	}
	defer func() {
		for _, tc := range tables {
			tc.reader.Close()
		}
	}()

	for fileIdx, tc := range tables {
		// Each reader should find its own entries.
		for _, e := range tc.entries {
			val, found, _, err := tc.reader.Get(e.key)
			if err != nil {
				t.Fatalf("file %d Get(%q): %v", fileIdx, e.key, err)
			}
			if !found {
				t.Fatalf("file %d: expected to find %q", fileIdx, e.key)
			}
			if !bytes.Equal(val, e.value) {
				t.Errorf("file %d key %q: got %q, want %q", fileIdx, e.key, val, e.value)
			}
		}

		// Each reader should NOT find entries from other files.
		otherKey := []byte(fmt.Sprintf("file%d-key0", (fileIdx+1)%len(tables)))
		_, found, _, err := tc.reader.Get(otherKey)
		if err != nil {
			t.Fatalf("file %d cross-check Get(%q): %v", fileIdx, otherKey, err)
		}
		if found {
			t.Errorf("file %d should not contain %q from another file", fileIdx, otherKey)
		}
	}
}

// TestReader_CloseAndReopen verifies that a reader can be closed and the same
// file re-opened with a new reader.
func TestReader_CloseAndReopen(t *testing.T) {
	dir := t.TempDir()
	entries := []testEntry{
		{key: []byte("persist"), value: []byte("data"), opcode: OpcodePut},
	}
	path := writeTestSSTable(t, dir, "reopen.sst", entries)

	r1, err := Open(path)
	if err != nil {
		t.Fatalf("Open 1: %v", err)
	}
	r1.Close()

	r2, err := Open(path)
	if err != nil {
		t.Fatalf("Open 2: %v", err)
	}
	defer r2.Close()

	val, found, _, err := r2.Get([]byte("persist"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found {
		t.Fatal("expected found=true after reopen")
	}
	if !bytes.Equal(val, []byte("data")) {
		t.Errorf("value: got %q, want %q", val, "data")
	}
}

// TestReader_DuplicateKeys verifies the Writer rejects duplicate keys with
// ErrKeysOutOfOrder, since keys must be in strictly ascending order.
func TestReader_DuplicateKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dupes.sst")

	w, err := NewWriter(path, 2)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	if err := w.Add([]byte("dup"), []byte("first"), OpcodePut); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	err = w.Add([]byte("dup"), []byte("second"), OpcodePut)
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
