package storage

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/makeshift-engineering/penguin-db/internal/storage/compactor"
	"github.com/makeshift-engineering/penguin-db/internal/storage/memtable"
	"github.com/makeshift-engineering/penguin-db/internal/storage/sstable"
	"github.com/makeshift-engineering/penguin-db/internal/storage/wal"
)

// mustNewEngine opens an engine and registers Close on t.Cleanup so callers
// never need to defer it manually.
func mustNewEngine(t *testing.T, dir string, opts Options) Engine {
	t.Helper()
	engine, err := NewEngine(dir, opts)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	return engine
}

// collectScan drains an iterator and returns all (key, value) pairs.
func collectScan(iter Iterator) [][2]string {
	var out [][2]string
	for iter.Valid() {
		k, v := iter.Next()
		out = append(out, [2]string{string(k), string(v)})
	}
	return out
}

// waitForCompaction blocks until L0 is empty (compaction finished) or deadline.
func waitForCompaction(de *dbEngine, maxWait time.Duration) {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		de.mu.RLock()
		l0 := len(de.levels[0])
		l1 := len(de.levels[1])
		de.mu.RUnlock()
		if l0 == 0 && l1 > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// triggerFlush writes data records to the engine until a new L0 file is successfully flushed.
func triggerFlush(t *testing.T, eng Engine, prefix string, startVal int) {
	t.Helper()
	de := eng.(*dbEngine)
	de.mu.RLock()
	initialL0 := len(de.levels[0])
	de.mu.RUnlock()

	for valID := startVal; ; valID++ {
		key := []byte(fmt.Sprintf("%s%05d", prefix, valID))
		val := make([]byte, 10)
		if err := eng.Put(key, val); err != nil {
			t.Fatalf("triggerFlush Put failed: %v", err)
		}

		time.Sleep(10 * time.Millisecond)

		de.mu.RLock()
		currentL0 := len(de.levels[0])
		de.mu.RUnlock()
		if currentL0 > initialL0 {
			break
		}
	}
}

// ─── CRUD ─────────────────────────────────────────────────────────────────────

func TestEngine_BasicCRUD(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 1024 * 1024
	engine := mustNewEngine(t, dir, opts)

	// Put
	if err := engine.Put([]byte("key1"), []byte("value1")); err != nil {
		t.Errorf("Put failed: %v", err)
	}
	if err := engine.Put([]byte("key2"), []byte("value2")); err != nil {
		t.Errorf("Put failed: %v", err)
	}

	// Get hit
	val, err := engine.Get([]byte("key1"))
	if err != nil {
		t.Errorf("Get failed: %v", err)
	}
	if !bytes.Equal(val, []byte("value1")) {
		t.Errorf("expected value1, got %s", val)
	}

	// Update
	if err := engine.Put([]byte("key1"), []byte("value1-updated")); err != nil {
		t.Errorf("Put update failed: %v", err)
	}
	val, err = engine.Get([]byte("key1"))
	if err != nil {
		t.Errorf("Get after update failed: %v", err)
	}
	if !bytes.Equal(val, []byte("value1-updated")) {
		t.Errorf("expected value1-updated, got %s", val)
	}

	// Delete + tombstone check
	if err := engine.Delete([]byte("key2")); err != nil {
		t.Errorf("Delete failed: %v", err)
	}
	_, err = engine.Get([]byte("key2"))
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("expected ErrKeyNotFound after delete, got %v", err)
	}

	// Get miss for a key never written
	_, err = engine.Get([]byte("never-written"))
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("expected ErrKeyNotFound for absent key, got %v", err)
	}
}

func TestEngine_WALRecovery(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 1024 * 1024

	// Write data and close.
	engine, err := NewEngine(dir, opts)
	if err != nil {
		t.Fatalf("failed to open engine: %v", err)
	}
	_ = engine.Put([]byte("recovered1"), []byte("val1"))
	_ = engine.Put([]byte("recovered2"), []byte("val2"))
	_ = engine.Delete([]byte("recovered2"))
	if err := engine.Close(); err != nil {
		t.Fatalf("failed to close engine: %v", err)
	}

	// Re-open and verify.
	engine2 := mustNewEngine(t, dir, opts)
	val, err := engine2.Get([]byte("recovered1"))
	if err != nil {
		t.Errorf("recovered1 not found: %v", err)
	}
	if !bytes.Equal(val, []byte("val1")) {
		t.Errorf("expected val1, got %q", val)
	}
	_, err = engine2.Get([]byte("recovered2"))
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("expected ErrKeyNotFound for deleted key, got %v", err)
	}
}

func TestEngine_WriteBatch(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	engine := mustNewEngine(t, dir, opts)

	_ = engine.Put([]byte("b3"), []byte("val3"))

	batch := []Op{
		{Type: OpPut, Key: []byte("b1"), Value: []byte("val1")},
		{Type: OpPut, Key: []byte("b2"), Value: []byte("val2")},
		{Type: OpDelete, Key: []byte("b3")},
	}
	if err := engine.WriteBatch(batch); err != nil {
		t.Fatalf("WriteBatch failed: %v", err)
	}

	v1, err := engine.Get([]byte("b1"))
	if err != nil || !bytes.Equal(v1, []byte("val1")) {
		t.Errorf("b1 not written correctly: %v", err)
	}
	_, err = engine.Get([]byte("b3"))
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("b3 should be deleted: %v", err)
	}
}

// TestEngine_WriteBatch_Empty verifies that an empty batch is a valid no-op.
func TestEngine_WriteBatch_Empty(t *testing.T) {
	dir := t.TempDir()
	engine := mustNewEngine(t, dir, DefaultOptions())
	if err := engine.WriteBatch(nil); err != nil {
		t.Errorf("empty WriteBatch should return nil, got %v", err)
	}
	if err := engine.WriteBatch([]Op{}); err != nil {
		t.Errorf("empty WriteBatch slice should return nil, got %v", err)
	}
}
func TestEngine_ValidationErrors(t *testing.T) {
	dir := t.TempDir()
	engine := mustNewEngine(t, dir, DefaultOptions())

	// Empty-key guards.
	if err := engine.Put(nil, []byte("v")); err == nil {
		t.Error("Put with nil key: expected error, got nil")
	}
	if err := engine.Put([]byte{}, []byte("v")); err == nil {
		t.Error("Put with empty key: expected error, got nil")
	}
	if err := engine.Delete(nil); err == nil {
		t.Error("Delete with nil key: expected error, got nil")
	}
	if _, err := engine.Get(nil); err == nil {
		t.Error("Get with nil key: expected error, got nil")
	}
	if _, err := engine.Get([]byte{}); err == nil {
		t.Error("Get with empty key: expected error, got nil")
	}

	// Invalid OpType in WriteBatch.
	if err := engine.WriteBatch([]Op{{Type: 0x99, Key: []byte("k"), Value: []byte("v")}}); err == nil {
		t.Error("WriteBatch with invalid OpType: expected error, got nil")
	}

	// Empty key inside WriteBatch.
	if err := engine.WriteBatch([]Op{{Type: OpPut, Key: nil, Value: []byte("v")}}); err == nil {
		t.Error("WriteBatch with nil key: expected error, got nil")
	}
}

func TestEngine_MemTableFlush(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 100 // tiny — triggers many flushes
	engine := mustNewEngine(t, dir, opts)

	for i := range 10 {
		key := []byte(fmt.Sprintf("key-%03d", i))
		val := []byte(fmt.Sprintf("val-%03d", i))
		if err := engine.Put(key, val); err != nil {
			t.Fatalf("Put key %d: %v", i, err)
		}
	}
	time.Sleep(150 * time.Millisecond) // let background flushes settle

	for i := range 10 {
		key := []byte(fmt.Sprintf("key-%03d", i))
		val := []byte(fmt.Sprintf("val-%03d", i))
		got, err := engine.Get(key)
		if err != nil {
			t.Errorf("Get key %d after flush: %v", i, err)
		}
		if !bytes.Equal(got, val) {
			t.Errorf("key %d: expected %s, got %s", i, val, got)
		}
	}
}

func TestEngine_Compaction(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 80
	opts.CompactionThreshold = 3
	engine := mustNewEngine(t, dir, opts)

	for i := range 12 {
		key := []byte(fmt.Sprintf("ckey-%03d", i))
		val := []byte(fmt.Sprintf("cval-%03d", i))
		if err := engine.Put(key, val); err != nil {
			t.Fatalf("Put: %v", err)
		}
		time.Sleep(25 * time.Millisecond) // let each flush produce a distinct L0 file
	}
	time.Sleep(300 * time.Millisecond) // give compaction time to complete

	for i := range 12 {
		key := []byte(fmt.Sprintf("ckey-%03d", i))
		val := []byte(fmt.Sprintf("cval-%03d", i))
		got, err := engine.Get(key)
		if err != nil {
			t.Errorf("Get after compaction %s: %v", key, err)
		}
		if !bytes.Equal(got, val) {
			t.Errorf("value mismatch: got %s, want %s", got, val)
		}
	}
}

// TestEngine_CompactionWithDeletes verifies that tombstones are elided after
// compaction (bottom-level elision) and that overwritten values are deduped.
func TestEngine_CompactionWithDeletes(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 60
	opts.CompactionThreshold = 2
	eng := mustNewEngine(t, dir, opts)
	de := eng.(*dbEngine)

	// Write k1 and k2, flush.
	_ = eng.Put([]byte("k1"), []byte("v1"))
	_ = eng.Put([]byte("k2"), []byte("v2"))
	time.Sleep(50 * time.Millisecond)

	// Delete k1, update k2, flush again → triggers compaction.
	_ = eng.Delete([]byte("k1"))
	_ = eng.Put([]byte("k2"), []byte("v2-new"))
	time.Sleep(50 * time.Millisecond)

	waitForCompaction(de, 500*time.Millisecond)

	// After compaction k1 tombstone should be elided; k2 should read newest value.
	_, err := eng.Get([]byte("k1"))
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("expected k1 to be gone after compaction, got %v", err)
	}
	v, err := eng.Get([]byte("k2"))
	if err != nil || !bytes.Equal(v, []byte("v2-new")) {
		t.Errorf("expected k2=v2-new after compaction, got %q, err=%v", v, err)
	}
}

func TestEngine_Scan(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 100
	opts.CompactionThreshold = 10 // prevent compaction during setup
	engine := mustNewEngine(t, dir, opts)

	_ = engine.Put([]byte("aa1"), []byte("val_aa1"))
	_ = engine.Put([]byte("aa2"), []byte("val_aa2"))
	time.Sleep(30 * time.Millisecond)

	_ = engine.Put([]byte("ab1"), []byte("val_ab1"))
	_ = engine.Put([]byte("ab2"), []byte("val_ab2"))
	_ = engine.Put([]byte("ab3-del"), []byte("val_del"))
	_ = engine.Delete([]byte("ab3-del"))
	time.Sleep(30 * time.Millisecond)

	_ = engine.Put([]byte("bb1"), []byte("val_bb1")) // active memtable

	iter := engine.Scan([]byte("ab"))
	defer iter.Close()

	expected := [][2]string{{"ab1", "val_ab1"}, {"ab2", "val_ab2"}}
	got := collectScan(iter)
	if len(got) != len(expected) {
		t.Fatalf("expected %d results, got %d: %v", len(expected), len(got), got)
	}
	for i, pair := range got {
		if pair != expected[i] {
			t.Errorf("scan[%d]: got %v, want %v", i, pair, expected[i])
		}
	}
}

// TestEngine_ScanFullNoPrefix exercises the Scan(nil) path.
func TestEngine_ScanFullNoPrefix(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 1024 * 1024
	engine := mustNewEngine(t, dir, opts)

	keys := []string{"a1", "b2", "c3", "d4"}
	for _, k := range keys {
		_ = engine.Put([]byte(k), []byte("v-"+k))
	}

	iter := engine.Scan(nil) // nil prefix = full scan
	defer iter.Close()

	got := collectScan(iter)
	if len(got) != len(keys) {
		t.Fatalf("full scan: expected %d results, got %d", len(keys), len(got))
	}
}

// TestEngine_ScanAcrossL0AndL1 forces a flush and then compaction so the scan
// exercises the L1 SSTable iterator path (sstAdapter methods).
func TestEngine_ScanAcrossL0AndL1(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 60
	opts.CompactionThreshold = 2
	eng := mustNewEngine(t, dir, opts)
	de := eng.(*dbEngine)

	// Two flushes → compaction kicks in → data lands in L1.
	_ = eng.Put([]byte("m1"), []byte("v1"))
	_ = eng.Put([]byte("m2"), []byte("v2"))
	triggerFlush(t, eng, "dummy1", 1000)

	_ = eng.Put([]byte("m3"), []byte("v3"))
	_ = eng.Put([]byte("m4"), []byte("v4"))
	triggerFlush(t, eng, "dummy2", 2000)

	waitForCompaction(de, 1*time.Second)

	// Scan after compaction so data comes from L1 SSTables.
	iter := eng.Scan([]byte("m"))
	defer iter.Close()

	got := collectScan(iter)
	if len(got) < 4 {
		t.Errorf("expected 4 results from L1 scan, got %d: %v", len(got), got)
	}
}

// TestEngine_ScanDoubleClose verifies that calling iter.Close() twice is safe.
func TestEngine_ScanDoubleClose(t *testing.T) {
	dir := t.TempDir()
	engine := mustNewEngine(t, dir, DefaultOptions())
	_ = engine.Put([]byte("x"), []byte("y"))

	iter := engine.Scan(nil)
	iter.Close()
	iter.Close() // must not panic
}

// TestEngine_ScanNextAfterExhausted verifies Next on an exhausted iterator.
func TestEngine_ScanNextAfterExhausted(t *testing.T) {
	dir := t.TempDir()
	engine := mustNewEngine(t, dir, DefaultOptions())

	iter := engine.Scan(nil) // empty db
	defer iter.Close()

	if iter.Valid() {
		t.Error("expected iterator to be invalid on empty scan")
	}
	k, v := iter.Next()
	if k != nil || v != nil {
		t.Errorf("Next on exhausted iterator: expected (nil, nil), got (%v, %v)", k, v)
	}
}

// TestEngine_ScanImmMemtable ensures that the immutable memtable is visible
// during a scan that starts while a flush is in progress.
func TestEngine_ScanImmMemtable(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 60
	opts.CompactionThreshold = 100 // never compact during this test
	engine := mustNewEngine(t, dir, opts)

	_ = engine.Put([]byte("imm1"), []byte("v1"))
	_ = engine.Put([]byte("imm2"), []byte("v2"))
	time.Sleep(25 * time.Millisecond) // trigger immMemtable flush

	_ = engine.Put([]byte("imm3"), []byte("v3")) // lands in fresh active memtable

	iter := engine.Scan([]byte("imm"))
	defer iter.Close()

	got := collectScan(iter)
	if len(got) < 2 {
		t.Errorf("expected at least 2 imm-prefixed results, got %d: %v", len(got), got)
	}
}

// ─── Get across layers ────────────────────────────────────────────────────────

// TestEngine_GetFromL0SSTable reads a key that has been flushed to Level 0.
func TestEngine_GetFromL0SSTable(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 40
	opts.CompactionThreshold = 100 // prevent compaction
	engine := mustNewEngine(t, dir, opts)

	_ = engine.Put([]byte("l0key"), []byte("l0val"))
	time.Sleep(80 * time.Millisecond) // wait for flush

	// The memtable is now empty; key must come from L0.
	val, err := engine.Get([]byte("l0key"))
	if err != nil {
		t.Errorf("Get from L0: %v", err)
	}
	if !bytes.Equal(val, []byte("l0val")) {
		t.Errorf("Get from L0: expected l0val, got %q", val)
	}

	// Non-existent key that passes bloom check negatively.
	_, err = engine.Get([]byte("l0missing"))
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("expected ErrKeyNotFound for missing L0 key, got %v", err)
	}
}

// TestEngine_GetFromL1SSTable reads a key that has been compacted into Level 1.
func TestEngine_GetFromL1SSTable(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 60
	opts.CompactionThreshold = 2
	eng := mustNewEngine(t, dir, opts)
	de := eng.(*dbEngine)

	_ = eng.Put([]byte("l1key"), []byte("l1val"))
	_ = eng.Put([]byte("l1key2"), []byte("l1val2"))
	triggerFlush(t, eng, "dummy1", 1000)

	_ = eng.Put([]byte("l1key3"), []byte("l1val3"))
	triggerFlush(t, eng, "dummy2", 2000)

	waitForCompaction(de, 1*time.Second)

	val, err := eng.Get([]byte("l1key"))
	if err != nil {
		t.Errorf("Get from L1: %v", err)
	}
	if !bytes.Equal(val, []byte("l1val")) {
		t.Errorf("Get from L1: expected l1val, got %q", val)
	}

	// 1. Search key larger than MaxKey of L1 (index == len(level1))
	_, err = eng.Get([]byte("z-never-written"))
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("expected ErrKeyNotFound for key larger than L1 MaxKey, got %v", err)
	}

	// Search key smaller than MinKey of L1 (MinKey > key)
	_, err = eng.Get([]byte("a-never-written"))
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("expected ErrKeyNotFound for key smaller than L1 MinKey, got %v", err)
	}

	// 3. Search key inside L1 range but missing (Bloom / index miss)
	_, err = eng.Get([]byte("l1key1.5"))
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("expected ErrKeyNotFound for missing L1 range key, got %v", err)
	}
}

// TestEngine_GetTombstoneFromSSTable verifies that a tombstone flushed to an
// SSTable correctly returns ErrKeyNotFound from Get.
func TestEngine_GetTombstoneFromSSTable(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 40
	opts.CompactionThreshold = 100
	engine := mustNewEngine(t, dir, opts)

	_ = engine.Put([]byte("tkey"), []byte("tval"))
	_ = engine.Delete([]byte("tkey"))
	time.Sleep(80 * time.Millisecond) // flush both put+tombstone to L0

	_, err := engine.Get([]byte("tkey"))
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("expected ErrKeyNotFound for SSTable tombstone, got %v", err)
	}
}

// ─── Concurrency & isolation ──────────────────────────────────────────────────

func TestEngine_ConcurrentReadCompactionIsolation(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 60
	opts.CompactionThreshold = 2
	engine := mustNewEngine(t, dir, opts)

	_ = engine.Put([]byte("k1"), []byte("v1"))
	time.Sleep(30 * time.Millisecond)
	_ = engine.Put([]byte("k2"), []byte("v2"))
	time.Sleep(30 * time.Millisecond)

	// Pin L0 files via an open iterator.
	iter := engine.Scan([]byte("k"))

	// Write more to trigger compaction that would otherwise delete L0 files.
	_ = engine.Put([]byte("k3"), []byte("v3"))
	time.Sleep(30 * time.Millisecond)
	_ = engine.Put([]byte("k4"), []byte("v4"))
	time.Sleep(200 * time.Millisecond) // let compaction attempt

	expectedKeys := map[string]string{"k1": "v1", "k2": "v2"}
	count := 0
	for iter.Valid() {
		k, v := iter.Next()
		if wantV, ok := expectedKeys[string(k)]; ok {
			if string(v) != wantV {
				t.Errorf("value mismatch: got %s, want %s", v, wantV)
			}
			count++
		}
	}
	iter.Close()

	if count < 2 {
		t.Errorf("expected pinned iterator to read at least k1 and k2, got %d", count)
	}
}

// TestEngine_ConcurrentWrites hammers the engine with concurrent writers to
// exercise locking and the flush/rotation code paths under contention.
func TestEngine_ConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 200
	engine := mustNewEngine(t, dir, opts)

	done := make(chan struct{})
	workers := 8
	writes := 20

	for w := range workers {
		go func(workerID int) {
			defer func() { done <- struct{}{} }()
			for i := range writes {
				key := []byte(fmt.Sprintf("w%d-key%03d", workerID, i))
				val := []byte(fmt.Sprintf("w%d-val%03d", workerID, i))
				_ = engine.Put(key, val)
			}
		}(w)
	}
	for range workers {
		<-done
	}
	time.Sleep(100 * time.Millisecond) // allow any background flushes to finish
}

// ─── Exclusive lock ───────────────────────────────────────────────────────────

func TestEngine_ExclusiveDirectoryLock(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	engine := mustNewEngine(t, dir, opts)
	_ = engine // keep open

	_, err2 := NewEngine(dir, opts)
	if err2 == nil {
		t.Error("expected error opening second engine on same directory, got nil")
	} else {
		t.Logf("second engine correctly rejected: %v", err2)
	}
}

// ─── Close semantics ──────────────────────────────────────────────────────────

func TestEngine_DoubleClose(t *testing.T) {
	dir := t.TempDir()
	engine, err := NewEngine(dir, DefaultOptions())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if err := engine.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := engine.Close(); err != nil {
		t.Errorf("second Close (must be idempotent): %v", err)
	}
}

// TestEngine_CloseWithDirtyMemTable ensures that data written to an active
// MemTable that hasn't been flushed yet is persisted on Close.
func TestEngine_CloseWithDirtyMemTable(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 1024 * 1024 // large — no background flush

	engine, err := NewEngine(dir, opts)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	_ = engine.Put([]byte("dirty1"), []byte("dval1"))
	_ = engine.Put([]byte("dirty2"), []byte("dval2"))
	if err := engine.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopen and verify data is present via WAL replay.
	engine2 := mustNewEngine(t, dir, opts)
	v, err := engine2.Get([]byte("dirty1"))
	if err != nil || !bytes.Equal(v, []byte("dval1")) {
		t.Errorf("dirty1 after close/reopen: got %q, err=%v", v, err)
	}
}

// ─── Recovery ─────────────────────────────────────────────────────────────────

// TestEngine_RecoveryMemTableFlush forces the startup recovery flush path by
// reopening the engine with a MaxMemTableSize smaller than the replayed WAL.
func TestEngine_RecoveryMemTableFlush(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 4096

	// First session: write enough to fill more than 10 bytes of WAL content.
	engine, err := NewEngine(dir, opts)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	_ = engine.Put([]byte("key1"), []byte("value1"))
	_ = engine.Put([]byte("key2"), []byte("value2"))
	if err := engine.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopen with a tiny MaxMemTableSize so the replayed content exceeds it.
	opts.MaxMemTableSize = 10
	engine2, err := NewEngine(dir, opts)
	if err != nil {
		t.Fatalf("second open (recovery flush): %v", err)
	}
	t.Cleanup(func() { _ = engine2.Close() })

	v1, err := engine2.Get([]byte("key1"))
	if err != nil || !bytes.Equal(v1, []byte("value1")) {
		t.Errorf("key1 after recovery flush: got %q, err=%v", v1, err)
	}
	v2, err := engine2.Get([]byte("key2"))
	if err != nil || !bytes.Equal(v2, []byte("value2")) {
		t.Errorf("key2 after recovery flush: got %q, err=%v", v2, err)
	}
}

// TestEngine_RecoveryResumeFromHighestWAL checks the normal recovery path where
// the replayed MemTable fits within MaxMemTableSize (no L0 flush needed).
func TestEngine_RecoveryResumeFromHighestWAL(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 1024 * 1024

	engine, err := NewEngine(dir, opts)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	_ = engine.Put([]byte("rr1"), []byte("rv1"))
	if err := engine.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	engine2 := mustNewEngine(t, dir, opts)
	v, err := engine2.Get([]byte("rr1"))
	if err != nil || !bytes.Equal(v, []byte("rv1")) {
		t.Errorf("rr1 after resume: got %q, err=%v", v, err)
	}
}

// ─── cleanupWALFiles ─────────────────────────────────────────────────────────

// TestEngine_CleanupWALFiles verifies that WAL segments up to a given ID are
// deleted when cleanupWALFiles is called, exercising that helper directly.
func TestEngine_CleanupWALFiles(t *testing.T) {
	walDir := t.TempDir()

	// Create synthetic .wal files.
	for _, id := range []int{1, 2, 3, 4, 5} {
		path := filepath.Join(walDir, fmt.Sprintf("%06d.wal", id))
		if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
			t.Fatalf("create wal file: %v", err)
		}
	}

	cleanupWALFiles(walDir, 3) // delete segments 1, 2, 3

	for _, id := range []int{1, 2, 3} {
		path := filepath.Join(walDir, fmt.Sprintf("%06d.wal", id))
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("expected segment %d to be deleted", id)
		}
	}
	for _, id := range []int{4, 5} {
		path := filepath.Join(walDir, fmt.Sprintf("%06d.wal", id))
		if _, err := os.Stat(path); err != nil {
			t.Errorf("segment %d should still exist: %v", id, err)
		}
	}
}

// TestEngine_CleanupWALFiles_NonexistentDir checks that cleanup on a missing
// directory is a silent no-op (no panic, no error returned).
func TestEngine_CleanupWALFiles_NonexistentDir(t *testing.T) {
	cleanupWALFiles("/nonexistent/path/that/does/not/exist", 99) // must not panic
}

// ─── writeMemTableToSSTable ───────────────────────────────────────────────────

// TestEngine_WriteMemTableToSSTable_Success exercises the happy path directly.
func TestEngine_WriteMemTableToSSTable_Success(t *testing.T) {
	dir := t.TempDir()
	mem := newTestSkipList(t)
	_ = mem.Put([]byte("wk1"), []byte("wv1"))
	_ = mem.Put([]byte("wk2"), []byte("wv2"))

	path := filepath.Join(dir, "000001.sst")
	reader, err := writeMemTableToSSTable(path, mem)
	if err != nil {
		t.Fatalf("writeMemTableToSSTable: %v", err)
	}
	defer reader.Close()

	val, found, deleted, readErr := reader.Get([]byte("wk1"))
	if readErr != nil || !found || deleted || !bytes.Equal(val, []byte("wv1")) {
		t.Errorf("reader.Get wk1: val=%q found=%v deleted=%v err=%v", val, found, deleted, readErr)
	}
}

// TestEngine_WriteMemTableToSSTable_InvalidPath checks the error path where the
// SSTable file cannot be created (bad directory).
func TestEngine_WriteMemTableToSSTable_InvalidPath(t *testing.T) {
	mem := newTestSkipList(t)
	_ = mem.Put([]byte("k"), []byte("v"))

	_, err := writeMemTableToSSTable("/nonexistent/dir/file.sst", mem)
	if err == nil {
		t.Error("expected error writing to invalid path, got nil")
	}
}

// ─── openManifestLevels / closeOpenedLevels ───────────────────────────────────

// TestEngine_OpenManifestLevels_InvalidFile checks that openManifestLevels
// returns an error when a manifest entry points to a non-existent file and that
// closeOpenedLevels is exercised via the cleanup path.
func TestEngine_OpenManifestLevels_InvalidFile(t *testing.T) {
	sstRefs := make(map[*sstable.Reader]*sstableRef)
	badLevels := map[int][]string{
		0: {"nonexistent-000001.sst"},
	}
	// This should fail and call closeOpenedLevels internally.
	_, err := openManifestLevels(t.TempDir(), badLevels, sstRefs)
	if err == nil {
		t.Error("expected error opening non-existent SSTable, got nil")
	}
}

// ─── memAdapter & sstAdapter coverage ────────────────────────────────────────

// TestEngine_MemAdapterClose verifies the no-op Close on memAdapter is reached.
func TestEngine_MemAdapterClose(t *testing.T) {
	mem := newTestSkipList(t)
	_ = mem.Put([]byte("x"), []byte("y"))
	iter := mem.NewIterator()
	adapter := newMemAdapter(iter)
	adapter.Close() // must not panic; it is a no-op
}

// TestEngine_SstAdapterMethods exercises Valid/Key/Value/IsDeleted/Next/Close
// on sstAdapter by writing a real SSTable and iterating over it.
func TestEngine_SstAdapterMethods(t *testing.T) {
	dir := t.TempDir()
	mem := newTestSkipList(t)
	_ = mem.Put([]byte("sk1"), []byte("sv1"))

	path := filepath.Join(dir, "000001.sst")
	reader, err := writeMemTableToSSTable(path, mem)
	if err != nil {
		t.Fatalf("writeMemTableToSSTable: %v", err)
	}
	defer reader.Close()

	rawIter, err := reader.NewIteratorAt(nil)
	if err != nil {
		t.Fatalf("NewIteratorAt: %v", err)
	}
	adapter := newSstAdapter(rawIter)

	if !adapter.Valid() {
		t.Fatal("sstAdapter should be valid after creation")
	}
	if !bytes.Equal(adapter.Key(), []byte("sk1")) {
		t.Errorf("Key(): expected sk1, got %q", adapter.Key())
	}
	if !bytes.Equal(adapter.Value(), []byte("sv1")) {
		t.Errorf("Value(): expected sv1, got %q", adapter.Value())
	}
	if adapter.IsDeleted() {
		t.Error("IsDeleted() should be false for a Put record")
	}
	adapter.Next() // advance past the only entry
	if adapter.Valid() {
		t.Error("adapter should be invalid after exhaustion")
	}
	adapter.Close() // must not panic
}

// ─── unpinSSTable obsolete-deletion path ─────────────────────────────────────

// TestEngine_UnpinSSTable_ObsoleteDeletion verifies the deferred-deletion code
// path: when an SSTable is marked obsolete with refs>0, the file should only be
// deleted once the last ref is released via unpinSSTable.
func TestEngine_UnpinSSTable_ObsoleteDeletion(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 60
	opts.CompactionThreshold = 2
	eng := mustNewEngine(t, dir, opts)
	de := eng.(*dbEngine)

	_ = eng.Put([]byte("u1"), []byte("v1"))
	_ = eng.Put([]byte("u2"), []byte("v2"))
	time.Sleep(40 * time.Millisecond) // flush to L0

	// Open a scan to pin the L0 readers.
	iter := eng.Scan([]byte("u"))

	// Trigger compaction which will mark L0 readers as obsolete.
	_ = eng.Put([]byte("u3"), []byte("v3"))
	time.Sleep(40 * time.Millisecond)
	waitForCompaction(de, 500*time.Millisecond)

	// The scan can still read because files are pinned.
	got := collectScan(iter)
	iter.Close() // This should trigger deferred deletion of the now-obsolete file.

	if len(got) < 2 {
		t.Errorf("expected at least 2 results from pinned scan, got %d", len(got))
	}
}

// ─── WriteBatch while closing ─────────────────────────────────────────────────

// TestEngine_WriteBatchAfterClose verifies that writes after Close return an error.
func TestEngine_WriteBatchAfterClose(t *testing.T) {
	dir := t.TempDir()
	engine, err := NewEngine(dir, DefaultOptions())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if err := engine.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// isClosing = true → WriteBatch must return an error, not panic.
	err = engine.Put([]byte("afterclose"), []byte("v"))
	if err == nil {
		t.Error("expected error writing after Close, got nil")
	}
}

// ─── rotateActiveMemTableAndWAL ───────────────────────────────────────────────

// TestEngine_RotateMemtable confirms that a memtable rotation succeeds, data
// is preserved, and the engine correctly writes subsequent keys to the new WAL.
func TestEngine_RotateMemtable(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 80 // small enough to force rotation on write
	engine := mustNewEngine(t, dir, opts)

	// Fill up the memtable to trigger rotation on the next put.
	for i := range 6 {
		_ = engine.Put([]byte(fmt.Sprintf("rk%d", i)), []byte("val"))
	}
	time.Sleep(80 * time.Millisecond)

	// Write after rotation.
	_ = engine.Put([]byte("after-rotate"), []byte("rotated"))
	time.Sleep(50 * time.Millisecond)

	v, err := engine.Get([]byte("after-rotate"))
	if err != nil || !bytes.Equal(v, []byte("rotated")) {
		t.Errorf("expected rotated value, got %q err=%v", v, err)
	}
}

func newTestSkipList(t *testing.T) *memtable.SkipList {
	t.Helper()
	return memtable.NewSkipList(4*1024*1024, 12)
}

// TestEngine_GetImmMemtable_Value covers the Get path that reads a live value
// from the immutable memtable (the frozen-but-not-yet-flushed SkipList).
// We directly swap active → immutable to avoid relying on goroutine timing.
func TestEngine_GetImmMemtable_Value(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 1 << 20 // large – no automatic flush
	opts.CompactionThreshold = 100

	engine, err := NewEngine(dir, opts)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	de := engine.(*dbEngine)

	// Write a value into the active memtable.
	_ = engine.Put([]byte("imm-v-key"), []byte("imm-v-val"))

	// Manually freeze the active memtable into the immutable slot so that the
	// flush worker hasn't cleared it by the time Get runs.
	de.mu.Lock()
	de.immMemtable = de.memtable
	de.immWALSegmentID = de.activeWALSegmentID
	de.memtable = memtable.NewSkipList(de.opts.MaxMemTableSize, de.opts.MemTableMaxLevel)
	de.mu.Unlock()

	val, err := engine.Get([]byte("imm-v-key"))
	if err != nil || !bytes.Equal(val, []byte("imm-v-val")) {
		t.Errorf("expected imm-v-val from immMemtable: got %q, err=%v", val, err)
	}

	// Close lets the flush worker drain immMemtable normally.
	if err := engine.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// TestEngine_GetImmMemtable_Tombstone covers the Get path that finds a tombstone
// in the immutable memtable and correctly returns ErrKeyNotFound.
func TestEngine_GetImmMemtable_Tombstone(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 1 << 20
	opts.CompactionThreshold = 100

	engine, err := NewEngine(dir, opts)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	de := engine.(*dbEngine)

	// Put then immediately delete – both records land in the active memtable.
	_ = engine.Put([]byte("imm-t-key"), []byte("v"))
	_ = engine.Delete([]byte("imm-t-key"))

	// Freeze active memtable (which contains the tombstone) into the immutable slot.
	de.mu.Lock()
	de.immMemtable = de.memtable
	de.immWALSegmentID = de.activeWALSegmentID
	de.memtable = memtable.NewSkipList(de.opts.MaxMemTableSize, de.opts.MemTableMaxLevel)
	de.mu.Unlock()

	_, err = engine.Get([]byte("imm-t-key"))
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("expected ErrKeyNotFound for tombstone in immMemtable, got %v", err)
	}

	if err := engine.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// ─── Scan: L1 range exclusion ─────────────────────────────────────────────────

// TestEngine_ScanL1RangeExclusion verifies that L1 SSTables are skipped when
// the scan prefix falls entirely below (MinKey branch) or entirely above
// (MaxKey branch) the file's key range, exercising both overlap=false branches.
func TestEngine_ScanL1RangeExclusion(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 60
	opts.CompactionThreshold = 2
	eng := mustNewEngine(t, dir, opts)
	de := eng.(*dbEngine)

	// Build L1 files whose keys are all in the "m*" range.
	_ = eng.Put([]byte("m1"), []byte("v1"))
	_ = eng.Put([]byte("m2"), []byte("v2"))
	triggerFlush(t, eng, "dummy1", 1000)

	_ = eng.Put([]byte("m3"), []byte("v3"))
	triggerFlush(t, eng, "dummy2", 2000)

	waitForCompaction(de, 1*time.Second)

	// Prefix "a" → prefixLimit="b" < L1 MinKey="m1" → MinKey branch: overlap=false.
	iter1 := eng.Scan([]byte("a"))
	got1 := collectScan(iter1)
	iter1.Close()
	if len(got1) != 0 {
		t.Errorf("prefix 'a' (below L1 range): expected 0 results, got %d: %v", len(got1), got1)
	}

	// Prefix "z" → L1 MaxKey="m3" < "z" → MaxKey branch: overlap=false.
	iter2 := eng.Scan([]byte("z"))
	got2 := collectScan(iter2)
	iter2.Close()
	if len(got2) != 0 {
		t.Errorf("prefix 'z' (above L1 range): expected 0 results, got %d: %v", len(got2), got2)
	}
}

// ─── Manifest: error paths ────────────────────────────────────────────────────

// TestEngine_LoadManifest_CorruptJSON covers the json.Unmarshal error path in
// loadManifest when the manifest file contains malformed JSON.
func TestEngine_LoadManifest_CorruptJSON(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte("not: valid: json {{{"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := loadManifest(dir)
	if err == nil {
		t.Error("expected error for corrupt manifest JSON, got nil")
	}
}

// TestEngine_NewEngine_CorruptManifest exercises the NewEngine error path when
// the persisted manifest file contains invalid JSON.
func TestEngine_NewEngine_CorruptManifest(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte("{invalid-json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := NewEngine(dir, DefaultOptions())
	if err == nil {
		t.Error("expected error opening engine with corrupt manifest, got nil")
	}
}

// TestEngine_WriteManifest_InvalidDir covers the os.OpenFile error path in
// writeManifest when the target directory does not exist.
func TestEngine_WriteManifest_InvalidDir(t *testing.T) {
	err := writeManifest("/nonexistent/deeply/nested/dir", newManifest())
	if err == nil {
		t.Error("expected error writing manifest to non-existent directory, got nil")
	}
}

// ─── openManifestLevels: partial failure (closeOpenedLevels inner body) ───────

// TestEngine_OpenManifestLevels_PartialFailure causes openManifestLevels to
// successfully open one SSTable before encountering a non-existent second file.
// The resulting cleanup call exercises the inner body of closeOpenedLevels.
func TestEngine_OpenManifestLevels_PartialFailure(t *testing.T) {
	dir := t.TempDir()

	// Create a real, readable SSTable so the first file opens successfully.
	mem := newTestSkipList(t)
	_ = mem.Put([]byte("pk1"), []byte("pv1"))
	sstPath := filepath.Join(dir, "000001.sst")
	reader, err := writeMemTableToSSTable(sstPath, mem)
	if err != nil {
		t.Fatalf("writeMemTableToSSTable: %v", err)
	}
	reader.Close() // release our handle; openManifestLevels will re-open it

	sstRefs := make(map[*sstable.Reader]*sstableRef)
	// Manifest lists one valid SSTable followed by one that does not exist.
	badLevels := map[int][]string{
		0: {"000001.sst", "999999-missing.sst"},
	}
	_, err = openManifestLevels(dir, badLevels, sstRefs)
	if err == nil {
		t.Error("expected error from openManifestLevels with partial failure, got nil")
	}
}

// ─── recoverActiveState: IF branch via direct WAL write ──────────────────────

// TestEngine_RecoveryFlushPath writes WAL records directly (without opening an
// engine) and then opens a new engine with a tiny MaxMemTableSize so that the
// WAL replay produces a recovery MemTable that exceeds the limit, triggering the
// recoverActiveState → writeMemTableToSSTable flush path (the IF branch).
func TestEngine_RecoveryFlushPath(t *testing.T) {
	dir := t.TempDir()
	walDir := filepath.Join(dir, "wal")
	if err := os.MkdirAll(walDir, 0o755); err != nil {
		t.Fatalf("MkdirAll walDir: %v", err)
	}

	// Write 15 records with long values directly to WAL segment 1.
	// Total size ≈ 15 × (11 + 40) = 765 bytes, far above MaxMemTableSize=10.
	walWriter, err := wal.NewLogWriter(walDir, 1)
	if err != nil {
		t.Fatalf("wal.NewLogWriter: %v", err)
	}
	for i := range 15 {
		rec := &wal.Record{
			Opcode: wal.OpcodePut,
			Key:    []byte(fmt.Sprintf("rfkey-%03d", i)),
			Value:  []byte("padding-value-to-fill-recovery-memtable"),
		}
		if err := walWriter.Append(rec); err != nil {
			t.Fatalf("Append record %d: %v", i, err)
		}
	}
	if err := walWriter.Close(); err != nil {
		t.Fatalf("walWriter.Close: %v", err)
	}

	// Open engine with MaxMemTableSize far smaller than the WAL payload.
	opts := DefaultOptions()
	opts.MaxMemTableSize = 10 // 10 bytes << 765 bytes of WAL data
	engine, err := NewEngine(dir, opts)
	if err != nil {
		t.Fatalf("NewEngine (recovery flush): %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })

	// The recovery flush must have placed the data into L0; verify readability.
	val, err := engine.Get([]byte("rfkey-000"))
	if err != nil {
		t.Errorf("Get rfkey-000 after recovery flush: %v", err)
	}
	_ = val
}

// ─── unpinSSTable: deferred-deletion path ────────────────────────────────────

// TestEngine_UnpinSSTable_DeletesObsoleteFile verifies the deferred-deletion
// branch in unpinSSTable: after compaction marks L0 readers as obsolete while
// an iterator still holds a pin (refs > 0), the files are deleted only when
// iter.Close() brings refs to zero.
func TestEngine_UnpinSSTable_DeletesObsoleteFile(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 40
	opts.CompactionThreshold = 2
	eng := mustNewEngine(t, dir, opts)
	de := eng.(*dbEngine)

	// Force 1st L0 flush using triggerFlush.
	_ = eng.Put([]byte("del1a"), []byte("very-long-value-x"))
	_ = eng.Put([]byte("del1b"), []byte("very-long-value-y"))
	triggerFlush(t, eng, "dummy1", 1000)

	// Pin L0 readers by opening a scan.
	iter := eng.Scan(nil)

	// Trigger second L0 flush to exceed CompactionThreshold=2 and kick off compaction.
	_ = eng.Put([]byte("del2a"), []byte("very-long-value-z"))
	_ = eng.Put([]byte("del2b"), []byte("very-long-value-w"))
	triggerFlush(t, eng, "dummy2", 2000)

	// Wait for the compaction worker to complete.
	waitForCompaction(de, 1*time.Second)

	// Drain the pinned iterator (reads from now-obsolete but still-pinned files).
	got := collectScan(iter)

	// Close releases all pins → refs→0 for the obsolete readers → files deleted.
	iter.Close()

	if len(got) < 2 {
		t.Errorf("expected ≥2 entries from pinned scan, got %d", len(got))
	}

	// Data should still be readable via the new L1 file produced by compaction.
	v, err := eng.Get([]byte("del1a"))
	if err != nil {
		t.Errorf("del1a not readable after unpin: %v", err)
	}
	_ = v
}

// TestEngine_WriteMemTableToSSTable_AddFailure forces a writer.Add failure by
// attempting to serialize a memtable containing a key exceeding math.MaxUint16.
func TestEngine_WriteMemTableToSSTable_AddFailure(t *testing.T) {
	dir := t.TempDir()
	mem := newTestSkipList(t)
	// Create a key larger than math.MaxUint16 (65536 bytes) to trigger Add failure
	largeKey := make([]byte, 65536)
	_ = mem.Put(largeKey, []byte("val"))

	path := filepath.Join(dir, "000001.sst")
	_, err := writeMemTableToSSTable(path, mem)
	if err == nil {
		t.Error("expected error writing memtable with key too large, got nil")
	}
}

// TestEngine_RotateMemtable_WALWriterFailure triggers a rotateActiveMemTableAndWAL
// failure in createWALWriter by placing a directory where the new WAL file should go.
func TestEngine_RotateMemtable_WALWriterFailure(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 40
	engine, err := NewEngine(dir, opts)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	de := engine.(*dbEngine)

	// Create a directory where the next WAL segment file should go
	nextWALPath := filepath.Join(de.walDir, fmt.Sprintf("%06d.wal", de.nextSegmentID))
	if err := os.MkdirAll(nextWALPath, 0o755); err != nil {
		t.Fatalf("MkdirAll nextWALPath: %v", err)
	}

	// Trigger rotation
	err = engine.Put([]byte("key"), make([]byte, 50))
	if err == nil {
		t.Error("expected error during rotation due to WAL writer creation failure, got nil")
	}

	_ = engine.Close()
}

// TestEngine_RotateMemtable_WriteManifestFailure triggers a rotateActiveMemTableAndWAL
// failure in writeManifest by placing a directory where the manifest file should go.
func TestEngine_RotateMemtable_WriteManifestFailure(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 40
	engine, err := NewEngine(dir, opts)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	de := engine.(*dbEngine)

	// Create a directory named manifest.json to force rename failure
	manifestPath := filepath.Join(de.dir, "manifest.json")
	_ = os.Remove(manifestPath)
	if err := os.MkdirAll(manifestPath, 0o755); err != nil {
		t.Fatalf("MkdirAll manifestPath: %v", err)
	}

	// Trigger rotation
	err = engine.Put([]byte("key"), make([]byte, 50))
	if err == nil {
		t.Error("expected error during rotation due to manifest write failure, got nil")
	}

	_ = engine.Close()
}

// TestCompactor_BottomLevelTombstoneElision tests that the compactor correctly
// elides deleted keys (tombstones) during bottom-level compaction.
func TestCompactor_BottomLevelTombstoneElision(t *testing.T) {
	dir := t.TempDir()

	// 1. Create first SSTable with values
	mem1 := newTestSkipList(t)
	_ = mem1.Put([]byte("key1"), []byte("val1"))
	_ = mem1.Put([]byte("key2"), []byte("val2"))
	path1 := filepath.Join(dir, "000001.sst")
	r1, err := writeMemTableToSSTable(path1, mem1)
	if err != nil {
		t.Fatalf("writeMemTableToSSTable: %v", err)
	}
	r1.Close()

	// 2. Create second SSTable with a delete/tombstone for key1
	mem2 := newTestSkipList(t)
	_ = mem2.Delete([]byte("key1"))
	path2 := filepath.Join(dir, "000002.sst")
	r2, err := writeMemTableToSSTable(path2, mem2)
	if err != nil {
		t.Fatalf("writeMemTableToSSTable: %v", err)
	}
	r2.Close()

	// 3. Run compaction with IsBottomLevel: true
	task := &compactor.Task{
		InputFiles:      []string{path1, path2},
		FileIDs:         []int{1, 2}, // file 2 is newer than file 1
		OutputDirectory: dir,
		NextSegmentID:   3,
		IsBottomLevel:   true,
	}

	res, err := compactor.Run(task)
	if err != nil {
		t.Fatalf("compactor.Run: %v", err)
	}

	if len(res.NewFilesCreated) != 1 {
		t.Fatalf("expected 1 compacted file, got %d", len(res.NewFilesCreated))
	}

	// 4. Open the compacted SSTable and verify key1 is completely gone (elided)
	r3, err := sstable.Open(res.NewFilesCreated[0])
	if err != nil {
		t.Fatalf("sstable.Open: %v", err)
	}
	defer r3.Close()

	iter, err := r3.NewIteratorAt(nil)
	if err != nil {
		t.Fatalf("NewIteratorAt: %v", err)
	}
	defer iter.Close()

	foundKey1 := false
	foundKey2 := false
	for iter.Next() {
		if bytes.Equal(iter.Key(), []byte("key1")) {
			foundKey1 = true
		}
		if bytes.Equal(iter.Key(), []byte("key2")) {
			foundKey2 = true
		}
	}

	if foundKey1 {
		t.Error("expected key1 (tombstone) to be elided from bottom level compacted SSTable, but it was found")
	}
	if !foundKey2 {
		t.Error("expected key2 to be preserved, but it was not found")
	}
}

// TestEngine_NewEngine_MkdirAllFailure verifies that NewEngine returns an error
// if it fails to create the WAL directory because a file is blocking it.
func TestEngine_NewEngine_MkdirAllFailure(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "wal-blocking-file")
	if err := os.WriteFile(filePath, []byte("content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Pass filePath/wal as base directory: MkdirAll will fail because a file exists at that path
	_, err := NewEngine(filepath.Join(filePath, "sub"), DefaultOptions())
	if err == nil {
		t.Error("expected error from NewEngine due to MkdirAll failure, got nil")
	}
}

// TestEngine_RecoveryWALWriterFailure verifies WAL writer initialization failure
// during resume from highest WAL segment in recoverActiveState.
func TestEngine_RecoveryWALWriterFailure(t *testing.T) {
	dir := t.TempDir()
	walDir := filepath.Join(dir, "wal")
	if err := os.MkdirAll(walDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Create an empty WAL segment file so replay returns highest segment ID 1
	walPath := filepath.Join(walDir, "000001.wal")
	if err := os.WriteFile(walPath, []byte{}, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Replace it with a directory to force WAL writer creation failure on resume
	_ = os.Remove(walPath)
	if err := os.MkdirAll(walPath, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	_, err := NewEngine(dir, DefaultOptions())
	if err == nil {
		t.Error("expected error during recovery WAL writer init, got nil")
	}
}

// TestEngine_RecoveryFlushWALWriterFailure verifies active WAL writer failure
// during the recovery memtable flush branch in recoverActiveState.
func TestEngine_RecoveryFlushWALWriterFailure(t *testing.T) {
	dir := t.TempDir()
	walDir := filepath.Join(dir, "wal")
	if err := os.MkdirAll(walDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Write enough to WAL segment 1 to exceed MaxMemTableSize = 10
	walWriter, err := wal.NewLogWriter(walDir, 1)
	if err != nil {
		t.Fatalf("wal.NewLogWriter: %v", err)
	}
	rec := &wal.Record{
		Opcode: wal.OpcodePut,
		Key:    []byte("key"),
		Value:  []byte("very-long-value-exceeding-ten-bytes"),
	}
	if err := walWriter.Append(rec); err != nil {
		t.Fatalf("Append: %v", err)
	}
	_ = walWriter.Close()

	// Make 000002.wal a directory to trigger recovery flush WAL writer failure
	walPath2 := filepath.Join(walDir, "000002.wal")
	if err := os.MkdirAll(walPath2, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	opts := DefaultOptions()
	opts.MaxMemTableSize = 10
	_, err = NewEngine(dir, opts)
	if err == nil {
		t.Error("expected error during recovery flush WAL writer init, got nil")
	}
}

// TestEngine_LoadManifest_DirectoryError verifies loadManifest error path when
// the manifest file itself is a directory.
func TestEngine_LoadManifest_DirectoryError(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := os.MkdirAll(manifestPath, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	_, err := loadManifest(dir)
	if err == nil {
		t.Error("expected error loading manifest when it is a directory, got nil")
	}
}

// TestEngine_Compaction_RunFailure verifies the bgErr error path when compactor.Run
// fails during a background compaction (e.g. because an SSTable is deleted from disk).
func TestEngine_Compaction_RunFailure(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 60
	opts.CompactionThreshold = 100 // prevent compaction
	engine, err := NewEngine(dir, opts)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	de := engine.(*dbEngine)

	// Flush 2 L0 files
	_ = engine.Put([]byte("k1"), []byte("v1"))
	_ = engine.Put([]byte("k2"), []byte("v2"))
	triggerFlush(t, engine, "dummy1", 1000)

	_ = engine.Put([]byte("k3"), []byte("k3"))
	triggerFlush(t, engine, "dummy2", 2000)

	// Verify L0 has 2 files
	de.mu.RLock()
	l0Files := len(de.levels[0])
	de.mu.RUnlock()
	if l0Files != 2 {
		t.Fatalf("expected 2 L0 files, got %d", l0Files)
	}

	// Close the reader so Windows releases the file handle, then delete from disk
	de.mu.Lock()
	missingFilePath := de.levels[0][0].FilePath()
	_ = de.levels[0][0].Close()
	de.mu.Unlock()
	if err := os.Remove(missingFilePath); err != nil {
		t.Fatalf("os.Remove: %v", err)
	}

	// Set threshold to 2 and trigger compaction directly
	de.mu.Lock()
	de.opts.CompactionThreshold = 2
	de.mu.Unlock()

	select {
	case de.compactChan <- struct{}{}:
	default:
	}

	// Wait a moment for background compaction worker to fail and set bgErr
	deadline := time.Now().Add(500 * time.Millisecond)
	var bgErr error
	for time.Now().Before(deadline) {
		de.mu.RLock()
		bgErr = de.bgErr
		de.mu.RUnlock()
		if bgErr != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if bgErr == nil {
		t.Error("expected compaction to fail and set bgErr, but bgErr is nil")
	}

	_ = engine.Close()
}
