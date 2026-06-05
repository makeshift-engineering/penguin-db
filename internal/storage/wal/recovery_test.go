package wal

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
)

type mockMemTable struct {
	puts    map[string][]byte
	deletes []string
	putErr  error
	delErr  error
}

func newMockMemTable() *mockMemTable {
	return &mockMemTable{puts: make(map[string][]byte)}
}

func (m *mockMemTable) Put(key, value []byte) error {
	if m.putErr != nil {
		return m.putErr
	}
	m.puts[string(key)] = value
	return nil
}

func (m *mockMemTable) Delete(key []byte) error {
	if m.delErr != nil {
		return m.delErr
	}
	m.deletes = append(m.deletes, string(key))
	return nil
}

func writeRecordsToFile(t *testing.T, path string, records []*Record) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		t.Fatalf("writeRecordsToFile open: %v", err)
	}
	defer f.Close()
	for _, r := range records {
		if _, err := f.Write(r.Marshal()); err != nil {
			t.Fatalf("writeRecordsToFile write: %v", err)
		}
	}
}

func segmentPath(dir string, id int) string {
	return filepath.Join(dir, fmt.Sprintf("%06d.wal", id))
}

func TestReplay_NonExistentDirectory_ReturnsFreshSegmentID(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "no-such-wal")
	mem := newMockMemTable()

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

func TestReplay_EmptyDirectory_ReturnsFreshSegmentID(t *testing.T) {
	dir := t.TempDir()
	mem := newMockMemTable()

	nextID, err := Replay(dir, mem)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if nextID != 1 {
		t.Errorf("nextID = %d, want 1", nextID)
	}
}

func TestReplay_NonWALFilesIgnored(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"data.sst", "manifest", "000001.log", "LOCK"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	mem := newMockMemTable()
	nextID, err := Replay(dir, mem)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if nextID != 1 {
		t.Errorf("nextID = %d, want 1", nextID)
	}
}

func TestReplay_SingleSegment_AllPuts(t *testing.T) {
	dir := t.TempDir()
	records := []*Record{
		{Opcode: OpcodePut, Key: []byte("a"), Value: []byte("1")},
		{Opcode: OpcodePut, Key: []byte("b"), Value: []byte("2")},
		{Opcode: OpcodePut, Key: []byte("c"), Value: []byte("3")},
	}
	writeRecordsToFile(t, segmentPath(dir, 1), records)

	mem := newMockMemTable()
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

func TestReplay_SingleSegment_Deletes(t *testing.T) {
	dir := t.TempDir()
	writeRecordsToFile(t, segmentPath(dir, 1), []*Record{
		{Opcode: OpcodePut, Key: []byte("x"), Value: []byte("val")},
		{Opcode: OpcodeDelete, Key: []byte("x")},
	})

	mem := newMockMemTable()
	if _, err := Replay(dir, mem); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mem.deletes) != 1 || mem.deletes[0] != "x" {
		t.Errorf("expected one delete for 'x', got %v", mem.deletes)
	}
}

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

	mem := newMockMemTable()
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

func TestReplay_ReturnsHighestSegmentID(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []int{1, 5, 10} {
		writeRecordsToFile(t, segmentPath(dir, id), []*Record{
			{Opcode: OpcodePut, Key: []byte(fmt.Sprintf("k%d", id)), Value: []byte("v")},
		})
	}
	mem := newMockMemTable()
	nextID, err := Replay(dir, mem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nextID != 10 {
		t.Errorf("nextID = %d, want 10", nextID)
	}
}

func TestReplay_UnknownOpcode_Ignored(t *testing.T) {
	dir := t.TempDir()
	writeRecordsToFile(t, segmentPath(dir, 1), []*Record{
		{Opcode: 99, Key: []byte("k"), Value: []byte("v")},
	})

	mem := newMockMemTable()
	if _, err := Replay(dir, mem); err != nil {
		t.Errorf("unexpected error for unknown opcode: %v", err)
	}
	if _, ok := mem.puts["k"]; ok {
		t.Error("unknown opcode record should not be applied to memtable")
	}
}

func TestReplay_CorruptedCRC_TruncatesFile(t *testing.T) {
	dir := t.TempDir()
	path := segmentPath(dir, 1)

	good := &Record{Opcode: OpcodePut, Key: []byte("good"), Value: []byte("val")}
	validBytes := good.Marshal()
	writeRecordsToFile(t, path, []*Record{good})

	badFrame := (&Record{Opcode: OpcodePut, Key: []byte("bad"), Value: []byte("x")}).Marshal()
	badFrame[12] ^= 0xFF
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	f.Write(badFrame)
	f.Close()

	mem := newMockMemTable()
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

func TestReplay_TruncatedHeader_TruncatesFile(t *testing.T) {
	dir := t.TempDir()
	path := segmentPath(dir, 1)

	writeRecordsToFile(t, path, []*Record{
		{Opcode: OpcodePut, Key: []byte("ok"), Value: []byte("v")},
	})

	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	f.Write([]byte{0xDE, 0xAD, 0xBE, 0xEF, 0xFF})
	f.Close()

	mem := newMockMemTable()
	if _, err := Replay(dir, mem); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := mem.puts["ok"]; !ok {
		t.Error("valid record was not applied")
	}
}

func TestReplay_TruncatedPayload_TruncatesFile(t *testing.T) {
	dir := t.TempDir()
	path := segmentPath(dir, 1)

	writeRecordsToFile(t, path, []*Record{
		{Opcode: OpcodePut, Key: []byte("safe"), Value: []byte("data")},
	})

	fakeKey := []byte("payload-cut")
	fakeSizeBytes := uint32(8 + 3 + len(fakeKey) + 100)
	hdr := make([]byte, 8)
	binary.LittleEndian.PutUint32(hdr[4:8], fakeSizeBytes)
	binary.LittleEndian.PutUint32(hdr[0:4], crc32.ChecksumIEEE(hdr[4:]))

	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	f.Write(hdr)
	f.Write(fakeKey[:3])
	f.Close()

	mem := newMockMemTable()
	if _, err := Replay(dir, mem); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := mem.puts["safe"]; !ok {
		t.Error("valid record before truncation point was not applied")
	}
}

func TestReplay_EmptySegmentFile_NoError(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(segmentPath(dir, 1), []byte{}, 0644)

	mem := newMockMemTable()
	nextID, err := Replay(dir, mem)
	if err != nil {
		t.Fatalf("unexpected error for empty WAL file: %v", err)
	}
	if nextID != 1 {
		t.Errorf("nextID = %d, want 1", nextID)
	}
}

func TestReplay_MemTablePutError_PropagatesError(t *testing.T) {
	dir := t.TempDir()
	writeRecordsToFile(t, segmentPath(dir, 1), []*Record{
		{Opcode: OpcodePut, Key: []byte("k"), Value: []byte("v")},
	})

	mem := newMockMemTable()
	mem.putErr = fmt.Errorf("memtable full")

	if _, err := Replay(dir, mem); err == nil {
		t.Fatal("expected error when memtable.Put fails, got nil")
	}
}

func TestReplay_MemTableDeleteError_PropagatesError(t *testing.T) {
	dir := t.TempDir()
	writeRecordsToFile(t, segmentPath(dir, 1), []*Record{
		{Opcode: OpcodeDelete, Key: []byte("k")},
	})

	mem := newMockMemTable()
	mem.delErr = fmt.Errorf("read-only memtable")

	if _, err := Replay(dir, mem); err == nil {
		t.Fatal("expected error when memtable.Delete fails, got nil")
	}
}

func TestReplay_MixedPutsAndDeletes_CorrectOrder(t *testing.T) {
	dir := t.TempDir()
	mem := newMockMemTable()

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

func TestReplay_SegmentsSortedNumerically(t *testing.T) {
	dir := t.TempDir()

	for i := 1; i <= 12; i++ {
		writeRecordsToFile(t, segmentPath(dir, i), []*Record{
			{Opcode: OpcodePut, Key: []byte("seq"), Value: []byte(fmt.Sprintf("%d", i))},
		})
	}

	mem := newMockMemTable()
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

func TestReplay_SubdirectoriesAreIgnored(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "archive.wal")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeRecordsToFile(t, segmentPath(dir, 1), []*Record{
		{Opcode: OpcodePut, Key: []byte("k"), Value: []byte("v")},
	})

	mem := newMockMemTable()
	nextID, err := Replay(dir, mem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nextID != 1 {
		t.Errorf("nextID = %d, want 1", nextID)
	}
}

func TestReplay_InvalidFrameSize_TooSmall_TruncatesFile(t *testing.T) {
	dir := t.TempDir()
	path := segmentPath(dir, 1)

	good := &Record{Opcode: OpcodePut, Key: []byte("good"), Value: []byte("val")}
	validBytes := good.Marshal()
	writeRecordsToFile(t, path, []*Record{good})

	hdr := make([]byte, 8)
	binary.LittleEndian.PutUint32(hdr[4:8], 7)
	binary.LittleEndian.PutUint32(hdr[0:4], crc32.ChecksumIEEE(hdr[4:]))

	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	f.Write(hdr)
	f.Close()

	mem := newMockMemTable()
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

func TestReplay_InvalidFrameSize_TooLarge_TruncatesFile(t *testing.T) {
	dir := t.TempDir()
	path := segmentPath(dir, 1)

	good := &Record{Opcode: OpcodePut, Key: []byte("good"), Value: []byte("val")}
	validBytes := good.Marshal()
	writeRecordsToFile(t, path, []*Record{good})

	hdr := make([]byte, 8)
	binary.LittleEndian.PutUint32(hdr[4:8], 129*1024*1024)
	binary.LittleEndian.PutUint32(hdr[0:4], crc32.ChecksumIEEE(hdr[4:]))

	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	f.Write(hdr)
	f.Close()

	mem := newMockMemTable()
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
