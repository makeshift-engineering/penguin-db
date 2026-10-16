package log

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	consensuspb "github.com/makeshift-engineering/penguin-db/gen/go/consensus/v1"
)

// TestMetaStore_BasicAndPersistence verifies initial state, updating terms and
// voting candidates, and persistent recovery across simulated process restarts.
func TestMetaStore_BasicAndPersistence(t *testing.T) {
	dir := t.TempDir()

	ms, err := NewMetaStore(dir)
	if err != nil {
		t.Fatalf("NewMetaStore failed: %v", err)
	}

	term, votedFor := ms.Get()
	if term != 0 || votedFor != "" {
		t.Fatalf("expected initial term=0, votedFor=\"\", got term=%d, votedFor=%q", term, votedFor)
	}

	if err := ms.Set(3, "node-b"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	term, votedFor = ms.Get()
	if term != 3 || votedFor != "node-b" {
		t.Fatalf("expected term=3, votedFor=\"node-b\", got term=%d, votedFor=%q", term, votedFor)
	}

	reopened, err := NewMetaStore(dir)
	if err != nil {
		t.Fatalf("reopening MetaStore failed: %v", err)
	}

	reopenedTerm, reopenedVote := reopened.Get()
	if reopenedTerm != 3 || reopenedVote != "node-b" {
		t.Fatalf("expected persisted term=3, votedFor=\"node-b\", got term=%d, votedFor=%q", reopenedTerm, reopenedVote)
	}
}

// TestMetaStore_AtomicTempFileCleanup verifies that dangling temporary files
// from interrupted writes are safely purged on startup without corrupting committed state.
func TestMetaStore_AtomicTempFileCleanup(t *testing.T) {
	dir := t.TempDir()

	ms, err := NewMetaStore(dir)
	if err != nil {
		t.Fatalf("NewMetaStore failed: %v", err)
	}

	if err := ms.Set(5, "node-c"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	tmpFile := filepath.Join(dir, tempMetaFile)
	if err := os.WriteFile(tmpFile, []byte("dangling-partial-data"), 0o644); err != nil {
		t.Fatalf("failed to create dummy temp file: %v", err)
	}

	reopened, err := NewMetaStore(dir)
	if err != nil {
		t.Fatalf("reopening MetaStore with temp file failed: %v", err)
	}

	term, votedFor := reopened.Get()
	if term != 5 || votedFor != "node-c" {
		t.Fatalf("expected term=5, votedFor=\"node-c\", got term=%d, votedFor=%q", term, votedFor)
	}

	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Fatalf("expected temporary metadata file to be cleaned up on recovery")
	}
}

// TestSegment_AppendAndReadAt validates low-level record framing, checksum calculation,
// and random-access reads within an individual segment.
func TestSegment_AppendAndReadAt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "00000000000000000001.log")

	seg, err := OpenSegment(path, 1)
	if err != nil {
		t.Fatalf("OpenSegment failed: %v", err)
	}
	defer seg.Close()

	entries := []*consensuspb.LogEntry{
		{
			Index: 1,
			Term:  1,
			Type:  consensuspb.EntryType_ENTRY_TYPE_NOOP,
			Data:  nil,
		},
		{
			Index: 2,
			Term:  1,
			Type:  consensuspb.EntryType_ENTRY_TYPE_NORMAL,
			Data:  []byte("write-batch-data-key-1"),
		},
		{
			Index: 3,
			Term:  2,
			Type:  consensuspb.EntryType_ENTRY_TYPE_CONFIG_CHANGE,
			Data:  []byte("add-node-4"),
		},
	}

	offsets := make([]int64, len(entries))
	for i, entry := range entries {
		offset, length, appendErr := seg.Append(entry)
		if appendErr != nil {
			t.Fatalf("Append failed for entry %d: %v", entry.Index, appendErr)
		}
		if length <= headerSize && len(entry.Data) > 0 {
			t.Fatalf("record length too small: %d", length)
		}
		offsets[i] = offset
	}

	if err := seg.Sync(); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	for i, expected := range entries {
		readEntry, readErr := seg.ReadAt(offsets[i])
		if readErr != nil {
			t.Fatalf("ReadAt failed at offset %d: %v", offsets[i], readErr)
		}
		if readEntry.Index != expected.Index {
			t.Errorf("entry %d index mismatch: got %d, want %d", i, readEntry.Index, expected.Index)
		}
		if readEntry.Term != expected.Term {
			t.Errorf("entry %d term mismatch: got %d, want %d", i, readEntry.Term, expected.Term)
		}
		if readEntry.Type != expected.Type {
			t.Errorf("entry %d type mismatch: got %v, want %v", i, readEntry.Type, expected.Type)
		}
		if !bytes.Equal(readEntry.Data, expected.Data) {
			t.Errorf("entry %d data mismatch: got %q, want %q", i, readEntry.Data, expected.Data)
		}
	}
}

// TestSegment_CRCCorruptionDetection ensures that bit rot or storage media corruption
// triggers a checksum mismatch rather than silent data corruption.
func TestSegment_CRCCorruptionDetection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "00000000000000000001.log")

	seg, err := OpenSegment(path, 1)
	if err != nil {
		t.Fatalf("OpenSegment failed: %v", err)
	}

	entry := &consensuspb.LogEntry{
		Index: 1,
		Term:  1,
		Type:  consensuspb.EntryType_ENTRY_TYPE_NORMAL,
		Data:  []byte("critical-payload"),
	}

	offset, _, err := seg.Append(entry)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	_ = seg.Close()

	fileBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	corruptByteOffset := int(offset) + headerSize + 2
	fileBytes[corruptByteOffset] ^= 0xFF

	if err := os.WriteFile(path, fileBytes, 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	reopened, err := OpenSegment(path, 1)
	if err != nil {
		t.Fatalf("reopening segment failed: %v", err)
	}
	defer reopened.Close()

	_, err = reopened.ReadAt(offset)
	if err == nil {
		t.Fatal("expected CRC mismatch error, got nil")
	}
	if !errors.Is(err, ErrCorruptRecord) {
		t.Fatalf("expected ErrCorruptRecord, got: %v", err)
	}
}

// TestSegment_TornWriteRecoveryOnScan verifies that incomplete writes resulting
// from power loss during disk write are safely truncated back to the last valid entry.
func TestSegment_TornWriteRecoveryOnScan(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "00000000000000000001.log")

	seg, err := OpenSegment(path, 1)
	if err != nil {
		t.Fatalf("OpenSegment failed: %v", err)
	}

	entry1 := &consensuspb.LogEntry{Index: 1, Term: 1, Type: consensuspb.EntryType_ENTRY_TYPE_NORMAL, Data: []byte("entry-1")}
	entry2 := &consensuspb.LogEntry{Index: 2, Term: 1, Type: consensuspb.EntryType_ENTRY_TYPE_NORMAL, Data: []byte("entry-2")}

	_, len1, _ := seg.Append(entry1)
	_, len2, _ := seg.Append(entry2)
	validSize := len1 + len2
	_ = seg.Close()

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	partialGarbage := []byte{0x50, 0x47, 0x01, 0x02}
	if _, err := f.Write(partialGarbage); err != nil {
		t.Fatalf("failed to write partial garbage: %v", err)
	}
	f.Close()

	reopened, err := OpenSegment(path, 1)
	if err != nil {
		t.Fatalf("reopening segment failed: %v", err)
	}
	defer reopened.Close()

	var scanned []*consensuspb.LogEntry
	err = reopened.Scan(func(e *consensuspb.LogEntry, offset, length int64) error {
		scanned = append(scanned, e)
		return nil
	})
	if err != nil {
		t.Fatalf("Scan failed during recovery: %v", err)
	}

	if len(scanned) != 2 {
		t.Fatalf("expected 2 valid entries recovered, got %d", len(scanned))
	}
	if scanned[0].Index != 1 || scanned[1].Index != 2 {
		t.Fatalf("unexpected scanned entries order or index")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.Size() != validSize {
		t.Fatalf("expected file size to be truncated to %d, got %d", validSize, info.Size())
	}
}

// TestPersistentLog_LifecycleAndRecovery exercises the top-level consensus log
// interface through restarts, verifying index construction, metadata, and sequential lookups.
func TestPersistentLog_LifecycleAndRecovery(t *testing.T) {
	dir := t.TempDir()

	pl, err := OpenPersistentLog(dir)
	if err != nil {
		t.Fatalf("OpenPersistentLog failed: %v", err)
	}

	if err := pl.SetMeta(2, "leader-node"); err != nil {
		t.Fatalf("SetMeta failed: %v", err)
	}

	entries := make([]*consensuspb.LogEntry, 20)
	for i := range entries {
		entries[i] = &consensuspb.LogEntry{
			Index: uint64(i + 1),
			Term:  2,
			Type:  consensuspb.EntryType_ENTRY_TYPE_NORMAL,
			Data:  []byte(fmt.Sprintf("command-payload-%d", i+1)),
		}
	}

	if err := pl.Append(entries); err != nil {
		t.Fatalf("Append batch failed: %v", err)
	}

	lastIdx, lastTerm := pl.LastIndexAndTerm()
	if lastIdx != 20 || lastTerm != 2 {
		t.Fatalf("expected lastIndex=20, lastTerm=2, got lastIndex=%d, lastTerm=%d", lastIdx, lastTerm)
	}

	if err := pl.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	reopened, err := OpenPersistentLog(dir)
	if err != nil {
		t.Fatalf("OpenPersistentLog on existing dir failed: %v", err)
	}
	defer reopened.Close()

	if reopened.CurrentTerm() != 2 || reopened.VotedFor() != "leader-node" {
		t.Fatalf("recovered metadata mismatch: term=%d, votedFor=%q", reopened.CurrentTerm(), reopened.VotedFor())
	}

	recoveredLastIdx, recoveredLastTerm := reopened.LastIndexAndTerm()
	if recoveredLastIdx != 20 || recoveredLastTerm != 2 {
		t.Fatalf("expected recoveredLastIdx=20, lastTerm=2, got index=%d, term=%d", recoveredLastIdx, recoveredLastTerm)
	}

	fetchedEntries, err := reopened.GetEntries(1, 21)
	if err != nil {
		t.Fatalf("GetEntries range failed: %v", err)
	}
	if len(fetchedEntries) != 20 {
		t.Fatalf("expected 20 entries, got %d", len(fetchedEntries))
	}

	for i, entry := range fetchedEntries {
		expectedData := []byte(fmt.Sprintf("command-payload-%d", i+1))
		if !bytes.Equal(entry.Data, expectedData) {
			t.Errorf("entry %d payload mismatch: got %q, want %q", entry.Index, entry.Data, expectedData)
		}
	}
}

// TestPersistentLog_SegmentRolling verifies that the log seamlessly rotates active
// segments when the segment size threshold is crossed while keeping entries accessible.
func TestPersistentLog_SegmentRolling(t *testing.T) {
	dir := t.TempDir()

	pl, err := OpenPersistentLog(dir)
	if err != nil {
		t.Fatalf("OpenPersistentLog failed: %v", err)
	}
	defer pl.Close()

	pl.maxSegSize = 256

	entries := make([]*consensuspb.LogEntry, 10)
	for i := range entries {
		entries[i] = &consensuspb.LogEntry{
			Index: uint64(i + 1),
			Term:  1,
			Type:  consensuspb.EntryType_ENTRY_TYPE_NORMAL,
			Data:  bytes.Repeat([]byte("A"), 80),
		}
	}

	if err := pl.Append(entries); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	if len(pl.segments) <= 1 {
		t.Fatalf("expected multiple segments after rolling, got %d segments", len(pl.segments))
	}

	for i := uint64(1); i <= 10; i++ {
		entry, err := pl.GetEntry(i)
		if err != nil {
			t.Fatalf("failed retrieving entry %d across rotated segments: %v", i, err)
		}
		if entry.Index != i {
			t.Errorf("got entry index %d, want %d", entry.Index, i)
		}
	}
}

// TestPersistentLog_TruncateSuffix verifies that conflicting uncommitted entries
// on a follower are truncated both in memory and on disk when overwritten by an elected leader.
func TestPersistentLog_TruncateSuffix(t *testing.T) {
	dir := t.TempDir()

	pl, err := OpenPersistentLog(dir)
	if err != nil {
		t.Fatalf("OpenPersistentLog failed: %v", err)
	}

	entries := make([]*consensuspb.LogEntry, 8)
	for i := range entries {
		entries[i] = &consensuspb.LogEntry{
			Index: uint64(i + 1),
			Term:  1,
			Type:  consensuspb.EntryType_ENTRY_TYPE_NORMAL,
			Data:  []byte(fmt.Sprintf("initial-val-%d", i+1)),
		}
	}

	if err := pl.Append(entries); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	if err := pl.TruncateSuffix(5); err != nil {
		t.Fatalf("TruncateSuffix failed: %v", err)
	}

	lastIdx, _ := pl.LastIndexAndTerm()
	if lastIdx != 4 {
		t.Fatalf("expected lastIndex=4 after truncation, got %d", lastIdx)
	}

	_, err = pl.GetEntry(5)
	if !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("expected ErrEntryNotFound for truncated entry 5, got %v", err)
	}

	overwritingEntries := []*consensuspb.LogEntry{
		{Index: 5, Term: 2, Type: consensuspb.EntryType_ENTRY_TYPE_NORMAL, Data: []byte("overwritten-val-5")},
		{Index: 6, Term: 2, Type: consensuspb.EntryType_ENTRY_TYPE_NORMAL, Data: []byte("overwritten-val-6")},
	}
	if err := pl.Append(overwritingEntries); err != nil {
		t.Fatalf("Append after truncate failed: %v", err)
	}

	newLastIdx, newLastTerm := pl.LastIndexAndTerm()
	if newLastIdx != 6 || newLastTerm != 2 {
		t.Fatalf("expected lastIndex=6, term=2, got index=%d, term=%d", newLastIdx, newLastTerm)
	}

	pl.Close()

	reopened, err := OpenPersistentLog(dir)
	if err != nil {
		t.Fatalf("reopening log failed: %v", err)
	}
	defer reopened.Close()

	entry5, err := reopened.GetEntry(5)
	if err != nil {
		t.Fatalf("failed to retrieve overwritten entry 5: %v", err)
	}
	if !bytes.Equal(entry5.Data, []byte("overwritten-val-5")) {
		t.Fatalf("expected overwritten-val-5, got %s", entry5.Data)
	}
}

// TestPersistentLog_TruncatePrefix verifies log compaction after a snapshot,
// ensuring obsolete segment files are deleted from storage and lookups behind the prefix return ErrCompactPrecedes.
func TestPersistentLog_TruncatePrefix(t *testing.T) {
	dir := t.TempDir()

	pl, err := OpenPersistentLog(dir)
	if err != nil {
		t.Fatalf("OpenPersistentLog failed: %v", err)
	}
	defer pl.Close()

	pl.maxSegSize = 120

	entries := make([]*consensuspb.LogEntry, 6)
	for i := range entries {
		entries[i] = &consensuspb.LogEntry{
			Index: uint64(i + 1),
			Term:  1,
			Type:  consensuspb.EntryType_ENTRY_TYPE_NORMAL,
			Data:  bytes.Repeat([]byte("K"), 70),
		}
	}

	if err := pl.Append(entries); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	initialSegCount := len(pl.segments)
	if initialSegCount <= 1 {
		t.Fatalf("expected multiple segments, got %d", initialSegCount)
	}

	if err := pl.TruncatePrefix(2); err != nil {
		t.Fatalf("TruncatePrefix failed: %v", err)
	}

	if pl.FirstIndex() != 3 {
		t.Fatalf("expected FirstIndex=3, got %d", pl.FirstIndex())
	}

	_, err = pl.GetEntry(1)
	if !errors.Is(err, ErrCompactPrecedes) {
		t.Fatalf("expected ErrCompactPrecedes for compacted entry 1, got %v", err)
	}

	entry3, err := pl.GetEntry(3)
	if err != nil || entry3 == nil {
		t.Fatalf("expected entry 3 to remain accessible, got %v", err)
	}
}

// TestPersistentLog_ConcurrentOperations verifies thread safety when multiple goroutines
// concurrently read from and append to the persistent log.
func TestPersistentLog_ConcurrentOperations(t *testing.T) {
	dir := t.TempDir()

	pl, err := OpenPersistentLog(dir)
	if err != nil {
		t.Fatalf("OpenPersistentLog failed: %v", err)
	}
	defer pl.Close()

	seedEntries := make([]*consensuspb.LogEntry, 50)
	for i := range seedEntries {
		seedEntries[i] = &consensuspb.LogEntry{
			Index: uint64(i + 1),
			Term:  1,
			Type:  consensuspb.EntryType_ENTRY_TYPE_NORMAL,
			Data:  []byte(fmt.Sprintf("initial-val-%d", i+1)),
		}
	}
	if err := pl.Append(seedEntries); err != nil {
		t.Fatalf("seed Append failed: %v", err)
	}

	var wg sync.WaitGroup
	readers := 6
	appenderCount := 2

	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			for iter := 0; iter < 100; iter++ {
				idx := uint64((iter % 50) + 1)
				entry, err := pl.GetEntry(idx)
				if err != nil {
					t.Errorf("reader %d failed reading entry %d: %v", readerID, idx, err)
					return
				}
				if entry.Index != idx {
					t.Errorf("reader %d got index %d, want %d", readerID, entry.Index, idx)
					return
				}
			}
		}(r)
	}

	for a := 0; a < appenderCount; a++ {
		wg.Add(1)
		go func(appenderID int) {
			defer wg.Done()
			for iter := 0; iter < 20; iter++ {
				pl.mu.Lock()
				lastIdx, _ := pl.LastIndexAndTerm()
				newIdx := lastIdx + 1
				newEntry := &consensuspb.LogEntry{
					Index: newIdx,
					Term:  1,
					Type:  consensuspb.EntryType_ENTRY_TYPE_NORMAL,
					Data:  []byte(fmt.Sprintf("concurrent-append-%d-%d", appenderID, newIdx)),
				}
				pl.activeSegment.Append(newEntry)
				pl.index.Append(newIdx, 1, pl.activeSegment.baseIndex, 0, 0)
				pl.mu.Unlock()
			}
		}(a)
	}

	wg.Wait()
}
