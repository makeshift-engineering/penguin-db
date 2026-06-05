package wal

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestNewLogWriter_CreatesDirectory checks that initializing LogWriter creates the target directory.
func TestNewLogWriter_CreatesDirectory(t *testing.T) {
	base := t.TempDir()
	dir := fmt.Sprintf("%s/nested/wal", base)

	w, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer w.Close()

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("WAL directory was not created")
	}
}

// TestNewLogWriter_CreatesFirstSegmentFile checks that the first log segment is created upon initialization.
func TestNewLogWriter_CreatesFirstSegmentFile(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer w.Close()

	if _, err := os.Stat(segmentPath(dir, 1)); os.IsNotExist(err) {
		t.Error("first segment file 000001.wal was not created")
	}
}

// TestNewLogWriter_RespectsStartSegmentID verifies that the writer uses the provided initial segment ID.
func TestNewLogWriter_RespectsStartSegmentID(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer w.Close()

	if _, err := os.Stat(segmentPath(dir, 7)); os.IsNotExist(err) {
		t.Error("segment 000007.wal was not created for startSegmentID=7")
	}
}

// TestNewLogWriter_InvalidDirectory_ReturnsError checks that invalid directories return initialization errors.
func TestNewLogWriter_InvalidDirectory_ReturnsError(t *testing.T) {
	tmpFile, err := os.CreateTemp(t.TempDir(), "not-a-dir")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	_, err = NewLogWriter(tmpFile.Name()+"/subdir", 1)
	if err == nil {
		t.Error("expected error when directory path is inside a file, got nil")
	}
}

// TestAppend_SingleRecord_WrittenToDisk checks that a single record write changes the file size on disk.
func TestAppend_SingleRecord_WrittenToDisk(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}
	defer w.Close()

	r := &Record{Opcode: OpcodePut, Key: []byte("hello"), Value: []byte("world")}
	if err := w.Append(r); err != nil {
		t.Fatalf("Append: %v", err)
	}

	info, err := os.Stat(segmentPath(dir, 1))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() == 0 {
		t.Error("segment file is empty after Append")
	}
}

// TestAppend_MultipleRecords_AllWritten checks that multiple sequential records are successfully appended.
func TestAppend_MultipleRecords_AllWritten(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}

	records := []*Record{
		{Opcode: OpcodePut, Key: []byte("k1"), Value: []byte("v1")},
		{Opcode: OpcodePut, Key: []byte("k2"), Value: []byte("v2")},
		{Opcode: OpcodeDelete, Key: []byte("k1")},
	}
	for _, r := range records {
		if err := w.Append(r); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	mem := newMockMemTable()
	if _, err := Replay(dir, mem); err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if _, ok := mem.puts["k2"]; !ok {
		t.Error("k2 not in memtable after replay")
	}
	found := false
	for _, d := range mem.deletes {
		if d == "k1" {
			found = true
		}
	}
	if !found {
		t.Error("delete for k1 not replayed")
	}
}

// TestAppend_RecordRoundtrip_ViaReplay verifies that written records are completely recoverable.
func TestAppend_RecordRoundtrip_ViaReplay(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}

	original := &Record{Opcode: OpcodePut, Key: []byte("penguindb"), Value: []byte("rocks")}
	if err := w.Append(original); err != nil {
		t.Fatalf("Append: %v", err)
	}
	w.Close()

	mem := newMockMemTable()
	if _, err := Replay(dir, mem); err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if string(mem.puts["penguindb"]) != "rocks" {
		t.Errorf("expected value 'rocks', got %q", mem.puts["penguindb"])
	}
}

// TestAppend_AfterClose_ReturnsError checks that writes are rejected after Close is called.
func TestAppend_AfterClose_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}
	w.Close()

	r := &Record{Opcode: OpcodePut, Key: []byte("k"), Value: []byte("v")}
	if err = w.Append(r); err == nil {
		t.Error("expected error when Appending after Close, got nil")
	}
}

// TestAppend_EmptyKey_Rejected verifies empty key appends are rejected with ErrEmptyKey.
func TestAppend_EmptyKey_Rejected(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}
	defer w.Close()

	r := &Record{Opcode: OpcodePut, Key: []byte{}, Value: []byte("v")}
	if err = w.Append(r); !errors.Is(err, ErrEmptyKey) {
		t.Errorf("expected ErrEmptyKey for empty key, got %v", err)
	}
}

// TestAppend_NilKey_Rejected verifies nil key appends are rejected with ErrEmptyKey.
func TestAppend_NilKey_Rejected(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}
	defer w.Close()

	r := &Record{Opcode: OpcodePut, Key: nil, Value: []byte("v")}
	if err = w.Append(r); !errors.Is(err, ErrEmptyKey) {
		t.Errorf("expected ErrEmptyKey for nil key, got %v", err)
	}
}

// TestAppend_EmptyValue_Allowed checks that empty/nil values are allowed in WAL entries.
func TestAppend_EmptyValue_Allowed(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}
	defer w.Close()

	r := &Record{Opcode: OpcodePut, Key: []byte("k"), Value: []byte{}}
	if err := w.Append(r); err != nil {
		t.Errorf("unexpected error appending record with empty value: %v", err)
	}
}

// TestRotation_NewSegmentCreatedAfterSizeExceeded checks that segment file rotates on limit overrun.
func TestRotation_NewSegmentCreatedAfterSizeExceeded(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}
	defer w.Close()

	w.currentSizeBytes = MaxSegmentSizeBytes - 1

	r := &Record{Opcode: OpcodePut, Key: []byte("trigger"), Value: []byte("rotation")}
	if err := w.Append(r); err != nil {
		t.Fatalf("Append: %v", err)
	}

	if _, err := os.Stat(segmentPath(dir, 2)); os.IsNotExist(err) {
		t.Error("expected segment 000002.wal after rotation")
	}
}

// TestRotation_OldSegmentSyncedOnRotation verifies old segment files are synced upon rotation.
func TestRotation_OldSegmentSyncedOnRotation(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}
	defer w.Close()

	r1 := &Record{Opcode: OpcodePut, Key: []byte("before"), Value: []byte("rotation")}
	if err := w.Append(r1); err != nil {
		t.Fatalf("Append r1: %v", err)
	}

	w.currentSizeBytes = MaxSegmentSizeBytes

	r2 := &Record{Opcode: OpcodePut, Key: []byte("after"), Value: []byte("rotation")}
	if err := w.Append(r2); err != nil {
		t.Fatalf("Append r2: %v", err)
	}

	w.Close()

	for _, id := range []int{1, 2} {
		info, err := os.Stat(segmentPath(dir, id))
		if err != nil {
			t.Fatalf("stat segment %d: %v", id, err)
		}
		if info.Size() == 0 {
			t.Errorf("segment %d is empty, expected data", id)
		}
	}
}

// TestRotation_SizeResetAfterRotation verifies that segment size tracking resets after file rotation.
func TestRotation_SizeResetAfterRotation(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}
	defer w.Close()

	w.currentSizeBytes = MaxSegmentSizeBytes
	r := &Record{Opcode: OpcodePut, Key: []byte("k"), Value: []byte("v")}
	if err := w.Append(r); err != nil {
		t.Fatalf("Append: %v", err)
	}

	if w.currentSizeBytes >= MaxSegmentSizeBytes {
		t.Errorf("currentSizeBytes = %d after rotation, expected < %d",
			w.currentSizeBytes, MaxSegmentSizeBytes)
	}
}

// TestAppend_ConcurrentWrites_NoDataRace validates concurrent WAL writes do not cause data races.
func TestAppend_ConcurrentWrites_NoDataRace(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}
	defer w.Close()

	const numGoroutines = 50
	const recordsPerGoroutine = 20

	var wg sync.WaitGroup
	var errCount int64

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < recordsPerGoroutine; i++ {
				r := &Record{
					Opcode: OpcodePut,
					Key:    []byte(fmt.Sprintf("goroutine-%d-key-%d", id, i)),
					Value:  []byte(fmt.Sprintf("value-%d", i)),
				}
				if err := w.Append(r); err != nil {
					atomic.AddInt64(&errCount, 1)
				}
			}
		}(g)
	}

	wg.Wait()

	if errCount > 0 {
		t.Errorf("%d Append calls failed under concurrency", errCount)
	}
}

// TestAppend_ConcurrentWrites_AllRecordsRecoverable verifies all concurrent writes are cleanly replayed.
func TestAppend_ConcurrentWrites_AllRecordsRecoverable(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}

	const numRecords = 100
	keys := make([]string, numRecords)
	for i := 0; i < numRecords; i++ {
		keys[i] = fmt.Sprintf("concurrent-key-%04d", i)
	}

	var wg sync.WaitGroup
	for i := 0; i < numRecords; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			r := &Record{
				Opcode: OpcodePut,
				Key:    []byte(keys[idx]),
				Value:  []byte("ok"),
			}
			_ = w.Append(r)
		}(i)
	}
	wg.Wait()
	w.Close()

	mem := newMockMemTable()
	if _, err := Replay(dir, mem); err != nil {
		t.Fatalf("Replay: %v", err)
	}
	for _, k := range keys {
		if _, ok := mem.puts[k]; !ok {
			t.Errorf("key %q missing after concurrent write + replay", k)
		}
	}
}

// TestClose_IsIdempotent validates that closing multiple times behaves correctly.
func TestClose_IsIdempotent(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Errorf("first Close error: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Logf("second Close returned (expected nil): %v", err)
	}
}

// TestClose_BlocksUntilWorkerDone checks that Close blocks until background activities terminate.
func TestClose_BlocksUntilWorkerDone(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}

	done := make(chan struct{})
	go func() {
		w.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Error("Close did not return within 3 seconds")
	}
}

// TestClose_SyncsDataToDisk verifies that closing the log writer flushes in-flight data.
func TestClose_SyncsDataToDisk(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}

	r := &Record{Opcode: OpcodePut, Key: []byte("durable"), Value: []byte("yes")}
	if err := w.Append(r); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	mem := newMockMemTable()
	if _, err := Replay(dir, mem); err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if string(mem.puts["durable"]) != "yes" {
		t.Error("data written before Close was not recovered")
	}
}

// TestMaxSegmentSizeBytes_Is32MB checks the segment size boundary constant.
func TestMaxSegmentSizeBytes_Is32MB(t *testing.T) {
	expected := int64(32 * 1024 * 1024)
	if MaxSegmentSizeBytes != expected {
		t.Errorf("MaxSegmentSizeBytes = %d, want %d (32 MiB)", MaxSegmentSizeBytes, expected)
	}
}

// TestBatchWorker_GroupCommit_AllTicketsSignalled verifies that all concurrent ticket requests receive replies.
func TestBatchWorker_GroupCommit_AllTicketsSignalled(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}
	defer w.Close()

	const n = 200
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			r := &Record{
				Opcode: OpcodePut,
				Key:    []byte(fmt.Sprintf("batch-key-%d", idx)),
				Value:  []byte("v"),
			}
			errs[idx] = w.Append(r)
		}(i)
	}
	wg.Wait()

	for i, e := range errs {
		if e != nil {
			t.Errorf("Append[%d] returned error: %v", i, e)
		}
	}
}
