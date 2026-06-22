package wal

import (
	"errors"
	"fmt"
	"os"
	"strings"
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

	mem := newMockRecordConsumer()
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

	mem := newMockRecordConsumer()
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

// TestAppend_InvalidOpcode_Rejected verifies that records with unrecognized opcodes
// are rejected with ErrInvalidOpcode instead of being silently persisted.
func TestAppend_InvalidOpcode_Rejected(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}
	defer w.Close()

	for _, opcode := range []uint8{2, 99, 255} {
		r := &Record{Opcode: opcode, Key: []byte("k"), Value: []byte("v")}
		if err := w.Append(r); !errors.Is(err, ErrInvalidOpcode) {
			t.Errorf("opcode %d: expected ErrInvalidOpcode, got %v", opcode, err)
		}
	}
}

// TestRotation_NewSegmentCreatedAfterSizeExceeded checks that segment file rotates on limit overrun.
func TestRotation_NewSegmentCreatedAfterSizeExceeded(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 1, WithSegmentSizeBytes(1))
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}
	defer w.Close()

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

	w.options.SegmentSizeBytes = 1 // force rotation on next write

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

	// 1. Write the first record. This goes into segment 1.
	r1 := &Record{Opcode: OpcodePut, Key: []byte("k1"), Value: []byte("v1")}
	if err := w.Append(r1); err != nil {
		t.Fatalf("Append r1: %v", err)
	}

	sizeBeforeRotation := w.currentSizeBytes
	if sizeBeforeRotation == 0 {
		t.Fatal("expected currentSizeBytes to be non-zero after first write")
	}

	// 2. Force rotation by setting a tiny segment limit.
	w.options.SegmentSizeBytes = 1

	// 3. Write a second record. This must trigger rotation to segment 2,
	// and the size counter must be reset to only include r2's size.
	r2 := &Record{Opcode: OpcodePut, Key: []byte("k2"), Value: []byte("v2")}
	if err := w.Append(r2); err != nil {
		t.Fatalf("Append r2: %v", err)
	}

	r2Frame, err := r2.Marshal()
	if err != nil {
		t.Fatalf("Marshal r2: %v", err)
	}
	expectedSize := int64(len(r2Frame))

	if w.currentSizeBytes != expectedSize {
		t.Errorf("currentSizeBytes = %d after rotation, expected %d (size of r2 frame only); size was not reset (first segment size was %d)",
			w.currentSizeBytes, expectedSize, sizeBeforeRotation)
	}
}

// TestRotation_ReopenedSegment_SizeAccountedFor verifies that when a writer
// reopens an existing segment (e.g. after recovery), it seeds currentSizeBytes
// from the file's actual size so the rotation threshold isn't silently bypassed.
func TestRotation_ReopenedSegment_SizeAccountedFor(t *testing.T) {
	dir := t.TempDir()

	// Phase 1: Write data to segment 1, then close.
	w1, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("NewLogWriter (phase 1): %v", err)
	}
	for i := 0; i < 10; i++ {
		r := &Record{Opcode: OpcodePut, Key: []byte(fmt.Sprintf("k%d", i)), Value: []byte("v")}
		if err := w1.Append(r); err != nil {
			t.Fatalf("Append (phase 1): %v", err)
		}
	}
	if err := w1.Close(); err != nil {
		t.Fatalf("Close (phase 1): %v", err)
	}

	// Get actual file size on disk.
	info, err := os.Stat(segmentPath(dir, 1))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	fileSize := info.Size()
	if fileSize == 0 {
		t.Fatal("segment file is unexpectedly empty")
	}

	// Phase 2: Reopen the same segment (simulating post-recovery).
	w2, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("NewLogWriter (phase 2): %v", err)
	}
	defer w2.Close()

	// The writer must account for the preexisting data.
	if w2.currentSizeBytes != fileSize {
		t.Errorf("currentSizeBytes = %d, want %d (preexisting file size)",
			w2.currentSizeBytes, fileSize)
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

	allDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(allDone)
	}()

	select {
	case <-allDone:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent writers hung for more than 5 seconds")
	}

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
	errs := make([]error, numRecords)
	for i := 0; i < numRecords; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			r := &Record{
				Opcode: OpcodePut,
				Key:    []byte(keys[idx]),
				Value:  []byte("ok"),
			}
			errs[idx] = w.Append(r)
		}(i)
	}

	allDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(allDone)
	}()

	select {
	case <-allDone:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent writers hung for more than 5 seconds")
	}
	w.Close()

	for i, e := range errs {
		if e != nil {
			t.Errorf("Append(%q) failed: %v", keys[i], e)
		}
	}

	mem := newMockRecordConsumer()
	if _, err := Replay(dir, mem); err != nil {
		t.Fatalf("Replay: %v", err)
	}
	for i, k := range keys {
		if errs[i] == nil {
			if _, ok := mem.puts[k]; !ok {
				t.Errorf("key %q was accepted by Append but not recovered by Replay", k)
			}
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
		t.Errorf("second Close returned (expected nil): %v", err)
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

	mem := newMockRecordConsumer()
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
	allDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(allDone)
	}()

	select {
	case <-allDone:
	case <-time.After(5 * time.Second):
		t.Fatal("batch writers hung for more than 5 seconds")
	}

	for i, e := range errs {
		if e != nil {
			t.Errorf("Append[%d] returned error: %v", i, e)
		}
	}
}

// TestMaxBatchSizeBytes_Is4MB checks the batch size limit constant.
func TestMaxBatchSizeBytes_Is4MB(t *testing.T) {
	expected := int64(4 * 1024 * 1024)
	if MaxBatchSizeBytes != expected {
		t.Errorf("MaxBatchSizeBytes = %d, want %d (4 MiB)", MaxBatchSizeBytes, expected)
	}
}

// TestAppend_ConcurrentWithClose_NoHang verifies that goroutines calling Append
// while Close is invoked concurrently do not deadlock or panic.
func TestAppend_ConcurrentWithClose_NoHang(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}

	const numWriters = 50
	var wg sync.WaitGroup

	// Launch many concurrent writers.
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				r := &Record{
					Opcode: OpcodePut,
					Key:    []byte(fmt.Sprintf("race-key-%d-%d", id, j)),
					Value:  []byte("v"),
				}
				_ = w.Append(r) // may return shutdown error, that's fine
			}
		}(i)
	}

	// Close concurrently with writers.
	done := make(chan struct{})
	go func() {
		time.Sleep(1 * time.Millisecond)
		w.Close()
		close(done)
	}()

	wg.Wait()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close or writers hung for more than 5 seconds")
	}
}

// TestAppend_AfterClose_ReturnsShutdownError checks the specific rejection path
// where Append observes isClosed=true under the read lock.
func TestAppend_AfterClose_ReturnsShutdownError(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}

	// Write one record to prove the writer works.
	r := &Record{Opcode: OpcodePut, Key: []byte("before"), Value: []byte("close")}
	if err := w.Append(r); err != nil {
		t.Fatalf("Append before Close: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Multiple post-close appends should all return errors, never panic.
	for i := 0; i < 10; i++ {
		r := &Record{Opcode: OpcodePut, Key: []byte(fmt.Sprintf("post-%d", i)), Value: []byte("v")}
		if err := w.Append(r); !errors.Is(err, ErrWriterClosed) {
			t.Errorf("Append[%d] after Close = %v, want ErrWriterClosed", i, err)
		}
	}
}

// TestClose_ConcurrentCalls_NoPanic verifies that calling Close from multiple
// goroutines simultaneously does not panic or return inconsistent errors.
func TestClose_ConcurrentCalls_NoPanic(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}

	// Write some data first.
	for i := 0; i < 10; i++ {
		r := &Record{Opcode: OpcodePut, Key: []byte(fmt.Sprintf("k%d", i)), Value: []byte("v")}
		if err := w.Append(r); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	const numClosers = 10
	var wg sync.WaitGroup
	for i := 0; i < numClosers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = w.Close()
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent Close calls hung for more than 5 seconds")
	}
}

// TestClose_DrainsInFlightTickets ensures that records already in the ingestion
// channel at the time of Close are still flushed and recoverable.
func TestClose_DrainsInFlightTickets(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}

	const numRecords = 100
	var wg sync.WaitGroup
	errs := make([]error, numRecords)

	// Spawn multiple concurrent appends to saturate the worker and ingestion channel.
	for i := 0; i < numRecords; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			r := &Record{
				Opcode: OpcodePut,
				Key:    []byte(fmt.Sprintf("drain-%04d", idx)),
				Value:  []byte("v"),
			}
			errs[idx] = w.Append(r)
		}(i)
	}

	closeErrChan := make(chan error, 1)
	go func() {
		closeErrChan <- w.Close()
	}()

	allAppendsDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(allAppendsDone)
	}()

	select {
	case <-allAppendsDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Append goroutines hung for more than 5 seconds")
	}

	select {
	case closeErr := <-closeErrChan:
		if closeErr != nil {
			t.Fatalf("Close: %v", closeErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close hung for more than 5 seconds")
	}

	// Count how many succeeded.
	var successCount int
	for _, e := range errs {
		if e == nil {
			successCount++
		}
	}
	if successCount == 0 {
		t.Fatal("no records were successfully appended")
	}

	// Verify that all successfully appended records are recoverable.
	mem := newMockRecordConsumer()
	if _, err := Replay(dir, mem); err != nil {
		t.Fatalf("Replay: %v", err)
	}

	for i, e := range errs {
		key := fmt.Sprintf("drain-%04d", i)
		if e == nil {
			if _, ok := mem.puts[key]; !ok {
				t.Errorf("record %q was accepted by Append but not recovered by Replay", key)
			}
		} else if !errors.Is(e, ErrWriterClosed) {
			t.Errorf("Append[%d] returned unexpected error: %v (expected nil or ErrWriterClosed)", i, e)
		}
	}
}

// TestBatchWorker_ExitsCleanly_WhenChannelClosed verifies the batchWorker goroutine
// terminates cleanly when the ingestion channel is closed (the new for-range pattern).
func TestBatchWorker_ExitsCleanly_WhenChannelClosed(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}

	// Append a record to prove the worker is running.
	r := &Record{Opcode: OpcodePut, Key: []byte("alive"), Value: []byte("yes")}
	if err := w.Append(r); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// Close should cause the channel to close and the worker to exit.
	done := make(chan struct{})
	go func() {
		w.Close()
		close(done)
	}()

	select {
	case <-done:
		// Worker exited cleanly.
	case <-time.After(3 * time.Second):
		t.Fatal("batchWorker did not exit within 3 seconds after channel close")
	}

	// Confirm the data is durable.
	mem := newMockRecordConsumer()
	if _, err := Replay(dir, mem); err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if string(mem.puts["alive"]) != "yes" {
		t.Error("record written before Close was not recovered")
	}
}

// TestAppend_ConcurrentWritesDuringClose_AllResolve verifies that every goroutine
// that calls Append gets a definitive result (success or error), even when Close
// is called concurrently. No goroutine should hang.
func TestAppend_ConcurrentWritesDuringClose_AllResolve(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}

	const numWriters = 100
	results := make(chan error, numWriters)

	var wg sync.WaitGroup
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			r := &Record{
				Opcode: OpcodePut,
				Key:    []byte(fmt.Sprintf("resolve-%d", id)),
				Value:  []byte("v"),
			}
			results <- w.Append(r)
		}(i)
	}

	// Close while writers are still racing.
	closeDone := make(chan error, 1)
	go func() {
		time.Sleep(500 * time.Microsecond)
		closeDone <- w.Close()
	}()

	// All writers must eventually return.
	allDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(allDone)
	}()

	select {
	case <-allDone:
	case <-time.After(5 * time.Second):
		t.Fatal("not all Append goroutines resolved within 5 seconds")
	}

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not resolve within 5 seconds")
	}

	close(results)
	var succeeded, failed int
	for err := range results {
		if err == nil {
			succeeded++
		} else {
			failed++
		}
	}
	t.Logf("concurrent close test: %d succeeded, %d rejected", succeeded, failed)

	if succeeded+failed != numWriters {
		t.Errorf("expected %d total results, got %d", numWriters, succeeded+failed)
	}
}

// TestNewLogWriter_WithOptions verifies that functional options correctly configure the LogWriter.
func TestNewLogWriter_WithOptions(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 1,
		WithSegmentSizeBytes(1024),
		WithBatchSizeBytes(512),
		WithIngestChannelCapacity(100),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer w.Close()

	if w.options.SegmentSizeBytes != 1024 {
		t.Errorf("SegmentSizeBytes = %d, want 1024", w.options.SegmentSizeBytes)
	}
	if w.options.BatchSizeBytes != 512 {
		t.Errorf("BatchSizeBytes = %d, want 512", w.options.BatchSizeBytes)
	}
	if w.options.IngestChannelCapacity != 100 {
		t.Errorf("IngestChannelCapacity = %d, want 100", w.options.IngestChannelCapacity)
	}

	// Verify validation rules
	_, err = NewLogWriter(dir, 2, WithSegmentSizeBytes(0))
	if err == nil {
		t.Error("expected error with 0 segment size")
	}
	_, err = NewLogWriter(dir, 2, WithBatchSizeBytes(0))
	if err == nil {
		t.Error("expected error with 0 batch size")
	}
	_, err = NewLogWriter(dir, 2, WithIngestChannelCapacity(-1))
	if err == nil {
		t.Error("expected error with negative channel capacity")
	}
}

// TestLogWriter_TerminalError verifies that I/O write failures transition the
// writer to a terminal error state, making all subsequent appends fail immediately.
func TestLogWriter_TerminalError(t *testing.T) {
	dir := t.TempDir()
	w, err := NewLogWriter(dir, 1)
	if err != nil {
		t.Fatalf("NewLogWriter: %v", err)
	}
	defer w.Close()

	// Append a good record to ensure things are working.
	r1 := &Record{Opcode: OpcodePut, Key: []byte("good"), Value: []byte("val")}
	if err := w.Append(r1); err != nil {
		t.Fatalf("first Append: %v", err)
	}

	// Close the file descriptor from under the writer to force a write error on next append.
	if err := w.activeFile.Close(); err != nil {
		t.Fatalf("force-closing active file: %v", err)
	}

	// The next append will attempt to write/sync to the closed file, which must fail.
	r2 := &Record{Opcode: OpcodePut, Key: []byte("fail"), Value: []byte("val")}
	err2 := w.Append(r2)
	if err2 == nil {
		t.Fatal("expected second Append to fail due to closed file descriptor, but it succeeded")
	}

	// This failed append must transition the writer to a terminal error state.
	// Any subsequent append should immediately return a terminal error.
	r3 := &Record{Opcode: OpcodePut, Key: []byte("subsequent"), Value: []byte("val")}
	err3 := w.Append(r3)
	if err3 == nil {
		t.Fatal("expected subsequent Append to fail in terminal error state, but it succeeded")
	}

	// Verify that the error wraps or matches our terminal I/O error pattern.
	if !strings.Contains(err3.Error(), "terminal WAL I/O error") {
		t.Errorf("expected error containing 'terminal WAL I/O error', got: %v", err3)
	}
}
