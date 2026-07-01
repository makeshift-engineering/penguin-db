package wal

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
)

// mockRecordConsumer implements the RecordConsumer interface for tracking replayed actions
// and simulating write failures during recovery tests.
type mockRecordConsumer struct {
	puts    map[string][]byte
	deletes []string
	putErr  error
	delErr  error
}

// newMockRecordConsumer constructs an empty mockRecordConsumer.
func newMockRecordConsumer() *mockRecordConsumer {
	return &mockRecordConsumer{puts: make(map[string][]byte)}
}

// Put records a put operation or returns an injected failure error.
func (m *mockRecordConsumer) Put(key, value []byte) error {
	if m.putErr != nil {
		return m.putErr
	}
	m.puts[string(key)] = value
	return nil
}

// Delete records a delete operation or returns an injected failure error.
func (m *mockRecordConsumer) Delete(key []byte) error {
	if m.delErr != nil {
		return m.delErr
	}
	m.deletes = append(m.deletes, string(key))
	return nil
}

// writeRecordsToFile serializes and appends a list of Records to a segment file.
func writeRecordsToFile(t *testing.T, path string, records []*Record) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		t.Fatalf("writeRecordsToFile open: %v", err)
	}
	defer f.Close()
	for _, r := range records {
		if _, err := f.Write(mustMarshal(t, r)); err != nil {
			t.Fatalf("writeRecordsToFile write: %v", err)
		}
	}
}

// segmentPath constructs the absolute filepath for a given segment ID.
func segmentPath(dir string, id int) string {
	return filepath.Join(dir, fmt.Sprintf("%06d.wal", id))
}

// TestReplay_NonExistentDirectory_ReturnsFreshSegmentID verifies recovery behaves
// correctly if the WAL directory does not exist.
func TestReplay_NonExistentDirectory_ReturnsFreshSegmentID(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "no-such-wal")
	mem := newMockRecordConsumer()

	nextID, err := Replay(dir, mem)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if nextID != 1 {
		t.Errorf("nextID = %d, want 1", nextID)
	}
	if len(mem.puts) != 0 || len(mem.deletes) != 0 {
		t.Error("memtable should be empty for a fresh start")
	}
}

// TestReplay_EmptyDirectory_ReturnsFreshSegmentID checks recovery on an empty directory.
func TestReplay_EmptyDirectory_ReturnsFreshSegmentID(t *testing.T) {
	dir := t.TempDir()
	mem := newMockRecordConsumer()

	nextID, err := Replay(dir, mem)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if nextID != 1 {
		t.Errorf("nextID = %d, want 1", nextID)
	}
}

// TestReplay_NonWALFilesIgnored checks that recovery ignores files without .wal extensions.
func TestReplay_NonWALFilesIgnored(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"data.sst", "manifest", "000001.log", "LOCK"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	mem := newMockRecordConsumer()
	nextID, err := Replay(dir, mem)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if nextID != 1 {
		t.Errorf("nextID = %d, want 1", nextID)
	}
}

// TestReplay_SingleSegment_AllPuts checks that a simple segment with Put operations is fully replayed.
func TestReplay_SingleSegment_AllPuts(t *testing.T) {
	dir := t.TempDir()
	records := []*Record{
		{Opcode: OpcodePut, Key: []byte("a"), Value: []byte("1")},
		{Opcode: OpcodePut, Key: []byte("b"), Value: []byte("2")},
		{Opcode: OpcodePut, Key: []byte("c"), Value: []byte("3")},
	}
	writeRecordsToFile(t, segmentPath(dir, 1), records)

	mem := newMockRecordConsumer()
	nextID, err := Replay(dir, mem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nextID != 1 {
		t.Errorf("nextID = %d, want 1", nextID)
	}
	for _, r := range records {
		if string(mem.puts[string(r.Key)]) != string(r.Value) {
			t.Errorf("key %q: got value %q, want %q", r.Key, mem.puts[string(r.Key)], r.Value)
		}
	}
}

// TestReplay_SingleSegment_Deletes checks that Delete operations are replayed.
func TestReplay_SingleSegment_Deletes(t *testing.T) {
	dir := t.TempDir()
	writeRecordsToFile(t, segmentPath(dir, 1), []*Record{
		{Opcode: OpcodePut, Key: []byte("x"), Value: []byte("val")},
		{Opcode: OpcodeDelete, Key: []byte("x")},
	})

	mem := newMockRecordConsumer()
	if _, err := Replay(dir, mem); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mem.deletes) != 1 || mem.deletes[0] != "x" {
		t.Errorf("expected one delete for 'x', got %v", mem.deletes)
	}
}

// TestReplay_MultipleSegments_ReplayedInOrder verifies segment logs are recovered in sequential order.
func TestReplay_MultipleSegments_ReplayedInOrder(t *testing.T) {
	dir := t.TempDir()

	writeRecordsToFile(t, segmentPath(dir, 3), []*Record{
		{Opcode: OpcodePut, Key: []byte("k"), Value: []byte("seg3")},
	})
	writeRecordsToFile(t, segmentPath(dir, 1), []*Record{
		{Opcode: OpcodePut, Key: []byte("k"), Value: []byte("seg1")},
	})
	writeRecordsToFile(t, segmentPath(dir, 2), []*Record{
		{Opcode: OpcodePut, Key: []byte("k"), Value: []byte("seg2")},
	})

	mem := newMockRecordConsumer()
	nextID, err := Replay(dir, mem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(mem.puts["k"]) != "seg3" {
		t.Errorf("key 'k' value = %q, want 'seg3'", mem.puts["k"])
	}
	if nextID != 3 {
		t.Errorf("nextID = %d, want 3", nextID)
	}
}

// TestReplay_ReturnsHighestSegmentID verifies recovery identifies and returns the highest active segment ID.
func TestReplay_ReturnsHighestSegmentID(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []int{1, 5, 10} {
		writeRecordsToFile(t, segmentPath(dir, id), []*Record{
			{Opcode: OpcodePut, Key: []byte(fmt.Sprintf("k%d", id)), Value: []byte("v")},
		})
	}
	mem := newMockRecordConsumer()
	nextID, err := Replay(dir, mem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nextID != 10 {
		t.Errorf("nextID = %d, want 10", nextID)
	}
}

// TestReplay_UnknownOpcode_Ignored verifies that records with invalid opcodes are ignored during replay.
func TestReplay_UnknownOpcode_Ignored(t *testing.T) {
	dir := t.TempDir()
	writeRecordsToFile(t, segmentPath(dir, 1), []*Record{
		{Opcode: 99, Key: []byte("k"), Value: []byte("v")},
	})

	mem := newMockRecordConsumer()
	if _, err := Replay(dir, mem); err != nil {
		t.Errorf("unexpected error for unknown opcode: %v", err)
	}
	if _, ok := mem.puts["k"]; ok {
		t.Error("unknown opcode record should not be applied to memtable")
	}
}

// TestReplay_CorruptedCRC_TruncatesFile checks that recovery detects CRC mismatches and truncates.
func TestReplay_CorruptedCRC_TruncatesFile(t *testing.T) {
	dir := t.TempDir()
	path := segmentPath(dir, 1)

	good := &Record{Opcode: OpcodePut, Key: []byte("good"), Value: []byte("val")}
	validBytes := mustMarshal(t, good)
	writeRecordsToFile(t, path, []*Record{good})

	badFrame := mustMarshal(t, &Record{Opcode: OpcodePut, Key: []byte("bad"), Value: []byte("x")})
	badFrame[12] ^= 0xFF
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(badFrame); err != nil {
		t.Fatalf("write bad frame: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	mem := newMockRecordConsumer()
	if _, err := Replay(dir, mem); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := mem.puts["good"]; !ok {
		t.Error("good record was not applied to memtable")
	}
	if _, ok := mem.puts["bad"]; ok {
		t.Error("corrupt record was applied to memtable")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() != int64(len(validBytes)) {
		t.Errorf("file size after truncate = %d, want %d", info.Size(), len(validBytes))
	}
}

// TestReplay_TruncatedHeader_TruncatesFile checks truncation when the frame header is truncated.
func TestReplay_TruncatedHeader_TruncatesFile(t *testing.T) {
	dir := t.TempDir()
	path := segmentPath(dir, 1)

	good := &Record{Opcode: OpcodePut, Key: []byte("ok"), Value: []byte("v")}
	validBytes := mustMarshal(t, good)
	writeRecordsToFile(t, path, []*Record{good})

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open for header append: %v", err)
	}
	if _, err := f.Write([]byte{0xDE, 0xAD, 0xBE, 0xEF, 0xFF}); err != nil {
		t.Fatalf("write truncated header: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	mem := newMockRecordConsumer()
	if _, err := Replay(dir, mem); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := mem.puts["ok"]; !ok {
		t.Error("valid record was not applied")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() != int64(len(validBytes)) {
		t.Errorf("file size after truncate = %d, want %d", info.Size(), len(validBytes))
	}
}

// TestReplay_TruncatedPayload_TruncatesFile checks truncation when the frame payload is truncated.
func TestReplay_TruncatedPayload_TruncatesFile(t *testing.T) {
	dir := t.TempDir()
	path := segmentPath(dir, 1)

	good := &Record{Opcode: OpcodePut, Key: []byte("safe"), Value: []byte("data")}
	validBytes := mustMarshal(t, good)
	writeRecordsToFile(t, path, []*Record{good})

	fakeKey := []byte("payload-cut")
	fakeSizeBytes := uint32(fixedHeaderSize + len(fakeKey) + 100)
	hdr := make([]byte, checksumSize+frameSizeSize)
	binary.LittleEndian.PutUint32(hdr[frameSizeOffset:frameSizeOffset+frameSizeSize], fakeSizeBytes)
	binary.LittleEndian.PutUint32(hdr[checksumOffset:checksumOffset+checksumSize], crc32.ChecksumIEEE(hdr[frameSizeOffset:]))

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open for payload append: %v", err)
	}
	if _, err := f.Write(hdr); err != nil {
		t.Fatalf("write hdr: %v", err)
	}
	if _, err := f.Write(fakeKey[:3]); err != nil {
		t.Fatalf("write partial payload: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	mem := newMockRecordConsumer()
	if _, err := Replay(dir, mem); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := mem.puts["safe"]; !ok {
		t.Error("valid record before truncation point was not applied")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() != int64(len(validBytes)) {
		t.Errorf("file size after truncate = %d, want %d", info.Size(), len(validBytes))
	}
}

// TestReplay_EmptySegmentFile_NoError checks that an empty WAL segment is handled gracefully.
func TestReplay_EmptySegmentFile_NoError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(segmentPath(dir, 1), []byte{}, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	mem := newMockRecordConsumer()
	nextID, err := Replay(dir, mem)
	if err != nil {
		t.Fatalf("unexpected error for empty WAL file: %v", err)
	}
	if nextID != 1 {
		t.Errorf("nextID = %d, want 1", nextID)
	}
}

// TestReplay_MemTablePutError_PropagatesError checks failure propagation on MemTable Put errors.
func TestReplay_MemTablePutError_PropagatesError(t *testing.T) {
	dir := t.TempDir()
	writeRecordsToFile(t, segmentPath(dir, 1), []*Record{
		{Opcode: OpcodePut, Key: []byte("k"), Value: []byte("v")},
	})

	mem := newMockRecordConsumer()
	mem.putErr = fmt.Errorf("memtable full")

	if _, err := Replay(dir, mem); err == nil {
		t.Fatal("expected error when memtable.Put fails, got nil")
	}
}

// TestReplay_MemTableDeleteError_PropagatesError checks failure propagation on MemTable Delete errors.
func TestReplay_MemTableDeleteError_PropagatesError(t *testing.T) {
	dir := t.TempDir()
	writeRecordsToFile(t, segmentPath(dir, 1), []*Record{
		{Opcode: OpcodeDelete, Key: []byte("k")},
	})

	mem := newMockRecordConsumer()
	mem.delErr = fmt.Errorf("read-only memtable")

	if _, err := Replay(dir, mem); err == nil {
		t.Fatal("expected error when memtable.Delete fails, got nil")
	}
}

// TestReplay_MixedPutsAndDeletes_CorrectOrder verifies interleaved Puts/Deletes replay in correct order.
func TestReplay_MixedPutsAndDeletes_CorrectOrder(t *testing.T) {
	dir := t.TempDir()
	mem := newMockRecordConsumer()

	writeRecordsToFile(t, segmentPath(dir, 1), []*Record{
		{Opcode: OpcodePut, Key: []byte("a"), Value: []byte("1")},
		{Opcode: OpcodePut, Key: []byte("b"), Value: []byte("2")},
		{Opcode: OpcodeDelete, Key: []byte("a")},
		{Opcode: OpcodePut, Key: []byte("a"), Value: []byte("3")},
	})

	if _, err := Replay(dir, mem); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(mem.puts["a"]) != "3" {
		t.Errorf("key 'a' value = %q, want '3'", mem.puts["a"])
	}
	if string(mem.puts["b"]) != "2" {
		t.Errorf("key 'b' value = %q, want '2'", mem.puts["b"])
	}

	found := false
	for _, d := range mem.deletes {
		if d == "a" {
			found = true
		}
	}
	if !found {
		t.Error("expected a Delete('a') call to memtable")
	}
}

// TestReplay_SegmentsSortedNumerically checks sorting logic on WAL segments with double digits.
func TestReplay_SegmentsSortedNumerically(t *testing.T) {
	dir := t.TempDir()

	for i := 1; i <= 12; i++ {
		writeRecordsToFile(t, segmentPath(dir, i), []*Record{
			{Opcode: OpcodePut, Key: []byte("seq"), Value: []byte(fmt.Sprintf("%d", i))},
		})
	}

	mem := newMockRecordConsumer()
	nextID, err := Replay(dir, mem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nextID != 12 {
		t.Errorf("nextID = %d, want 12", nextID)
	}
	if string(mem.puts["seq"]) != "12" {
		t.Errorf("value for 'seq' = %q, want '12'", mem.puts["seq"])
	}
}

// TestReplay_SubdirectoriesAreIgnored verifies subdirectories inside the WAL directory are skipped.
func TestReplay_SubdirectoriesAreIgnored(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "archive.wal")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeRecordsToFile(t, segmentPath(dir, 1), []*Record{
		{Opcode: OpcodePut, Key: []byte("k"), Value: []byte("v")},
	})

	mem := newMockRecordConsumer()
	nextID, err := Replay(dir, mem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nextID != 1 {
		t.Errorf("nextID = %d, want 1", nextID)
	}
}

// TestReplay_InvalidFrameSize_TooSmall_TruncatesFile verifies truncation on underflowing frame sizes.
func TestReplay_InvalidFrameSize_TooSmall_TruncatesFile(t *testing.T) {
	dir := t.TempDir()
	path := segmentPath(dir, 1)

	good := &Record{Opcode: OpcodePut, Key: []byte("good"), Value: []byte("val")}
	validBytes := mustMarshal(t, good)
	writeRecordsToFile(t, path, []*Record{good})

	hdr := make([]byte, checksumSize+frameSizeSize)
	binary.LittleEndian.PutUint32(hdr[frameSizeOffset:frameSizeOffset+frameSizeSize], 7)
	binary.LittleEndian.PutUint32(hdr[checksumOffset:checksumOffset+checksumSize], crc32.ChecksumIEEE(hdr[frameSizeOffset:]))

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(hdr); err != nil {
		t.Fatalf("write hdr (TooSmall): %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	mem := newMockRecordConsumer()
	if _, err := Replay(dir, mem); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := mem.puts["good"]; !ok {
		t.Error("good record was not applied")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() != int64(len(validBytes)) {
		t.Errorf("file size after truncate = %d, want %d", info.Size(), len(validBytes))
	}
}

// TestReplay_InvalidFrameSize_TooLarge_TruncatesFile verifies truncation on excessively large frame sizes.
func TestReplay_InvalidFrameSize_TooLarge_TruncatesFile(t *testing.T) {
	dir := t.TempDir()
	path := segmentPath(dir, 1)

	good := &Record{Opcode: OpcodePut, Key: []byte("good"), Value: []byte("val")}
	validBytes := mustMarshal(t, good)
	writeRecordsToFile(t, path, []*Record{good})

	hdr := make([]byte, checksumSize+frameSizeSize)
	// Frame size must exceed maxFrameSizeBytes (32 MiB); use 33 MiB.
	binary.LittleEndian.PutUint32(hdr[frameSizeOffset:frameSizeOffset+frameSizeSize], 33*1024*1024)
	binary.LittleEndian.PutUint32(hdr[checksumOffset:checksumOffset+checksumSize], crc32.ChecksumIEEE(hdr[frameSizeOffset:]))

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open for TooLarge append: %v", err)
	}
	if _, err := f.Write(hdr); err != nil {
		t.Fatalf("write hdr (TooLarge): %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	mem := newMockRecordConsumer()
	if _, err := Replay(dir, mem); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := mem.puts["good"]; !ok {
		t.Error("good record was not applied")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() != int64(len(validBytes)) {
		t.Errorf("file size after truncate = %d, want %d", info.Size(), len(validBytes))
	}
}

// TestReplay_MalformedWALFilename_Skipped verifies that files ending in .wal but not starting with a number are skipped during replay.
func TestReplay_MalformedWALFilename_Skipped(t *testing.T) {
	dir := t.TempDir()

	goodPath := segmentPath(dir, 1)
	good := &Record{Opcode: OpcodePut, Key: []byte("good"), Value: []byte("val")}
	writeRecordsToFile(t, goodPath, []*Record{good})

	badPath := filepath.Join(dir, "malformed.wal")
	bad := &Record{Opcode: OpcodePut, Key: []byte("bad"), Value: []byte("val")}
	f, err := os.Create(badPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(mustMarshal(t, bad)); err != nil {
		t.Fatalf("write malformed wal frame: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close malformed wal file: %v", err)
	}

	mem := newMockRecordConsumer()
	nextID, err := Replay(dir, mem)
	if err != nil {
		t.Fatalf("unexpected error replaying: %v", err)
	}
	if nextID != 1 {
		t.Errorf("nextID = %d, want 1", nextID)
	}
	if _, ok := mem.puts["good"]; !ok {
		t.Error("good record was not applied")
	}
	if _, ok := mem.puts["bad"]; ok {
		t.Error("bad record in malformed file was replayed, but should have been skipped")
	}
}

// TestTruncateSegment_Failure verifies that truncateSegment returns an error
// when called on an invalid path (like a directory).
func TestTruncateSegment_Failure(t *testing.T) {
	dir := t.TempDir()
	invalidPath := filepath.Join(dir, "some-dir")
	if err := os.Mkdir(invalidPath, 0755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	err := truncateSegment(invalidPath, 0)
	if err == nil {
		t.Error("expected error from truncateSegment on directory path, got nil")
	}
}
