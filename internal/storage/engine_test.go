package storage

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestEngine_BasicCRUD(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 1024 * 1024

	engine, err := NewEngine(dir, opts)
	if err != nil {
		t.Fatalf("failed to open engine: %v", err)
	}
	defer engine.Close()

	// Put
	if err := engine.Put([]byte("key1"), []byte("value1")); err != nil {
		t.Errorf("Put failed: %v", err)
	}
	if err := engine.Put([]byte("key2"), []byte("value2")); err != nil {
		t.Errorf("Put failed: %v", err)
	}

	// Get
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

	// Delete
	if err := engine.Delete([]byte("key2")); err != nil {
		t.Errorf("Delete failed: %v", err)
	}
	_, err = engine.Get([]byte("key2"))
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("expected ErrKeyNotFound, got %v", err)
	}
}

func TestEngine_WALRecovery(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 1024 * 1024

	// Write data and close engine
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

	// Open a new engine on the same directory and check recovery
	engine2, err := NewEngine(dir, opts)
	if err != nil {
		t.Fatalf("failed to reopen engine: %v", err)
	}
	defer engine2.Close()

	val, err := engine2.Get([]byte("recovered1"))
	if err != nil {
		t.Errorf("recovered1 not found: %v", err)
	}
	if !bytes.Equal(val, []byte("val1")) {
		t.Errorf("expected val1, got %q", val)
	}

	_, err = engine2.Get([]byte("recovered2"))
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("expected deleted recovered2 to return ErrKeyNotFound, got %v", err)
	}
}

func TestEngine_WriteBatch(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()

	engine, err := NewEngine(dir, opts)
	if err != nil {
		t.Fatalf("failed to open engine: %v", err)
	}
	defer engine.Close()

	batch := []Op{
		{Type: OpPut, Key: []byte("b1"), Value: []byte("val1")},
		{Type: OpPut, Key: []byte("b2"), Value: []byte("val2")},
		{Type: OpDelete, Key: []byte("b3"), Value: nil},
	}

	// Insert b3 beforehand so we can check delete inside batch
	_ = engine.Put([]byte("b3"), []byte("val3"))

	if err := engine.WriteBatch(batch); err != nil {
		t.Fatalf("WriteBatch failed: %v", err)
	}

	// Verify Put
	v1, err := engine.Get([]byte("b1"))
	if err != nil || !bytes.Equal(v1, []byte("val1")) {
		t.Errorf("b1 not written: %v", err)
	}

	// Verify Delete
	_, err = engine.Get([]byte("b3"))
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("b3 not deleted in batch: %v", err)
	}
}

func TestEngine_MemTableFlush(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 100 // Very small to trigger multiple flushes

	engine, err := NewEngine(dir, opts)
	if err != nil {
		t.Fatalf("failed to open engine: %v", err)
	}
	defer engine.Close()

	// Write enough to trigger at least 3 flushes
	// (Each put takes key length + value length in memtable size tracking)
	for i := range 10 {
		key := []byte(fmt.Sprintf("key-%03d", i))
		val := []byte(fmt.Sprintf("val-%03d", i))
		if err := engine.Put(key, val); err != nil {
			t.Fatalf("failed to put key %d: %v", i, err)
		}
	}

	// Let background flushes complete
	time.Sleep(100 * time.Millisecond)

	// Verify we can read everything back
	for i := range 10 {
		key := []byte(fmt.Sprintf("key-%03d", i))
		val := []byte(fmt.Sprintf("val-%03d", i))
		got, err := engine.Get(key)
		if err != nil {
			t.Errorf("failed to get key %d: %v", i, err)
		}
		if !bytes.Equal(got, val) {
			t.Errorf("mismatch for key %d: expected %s, got %s", i, val, got)
		}
	}
}

func TestEngine_Compaction(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 80 // Small memtable size
	opts.CompactionThreshold = 3

	engine, err := NewEngine(dir, opts)
	if err != nil {
		t.Fatalf("failed to open engine: %v", err)
	}
	defer engine.Close()

	// Put items to force flushes
	// 4 flushes will create 4 Level 0 files, which exceeds CompactionThreshold=3.
	for i := range 12 {
		key := []byte(fmt.Sprintf("ckey-%03d", i))
		val := []byte(fmt.Sprintf("cval-%03d", i))
		if err := engine.Put(key, val); err != nil {
			t.Fatalf("failed to put: %v", err)
		}
		// Brief pause to allow flushes to execute and not combine
		time.Sleep(30 * time.Millisecond)
	}

	// Give compaction background worker time to finish
	time.Sleep(200 * time.Millisecond)

	// Verify all keys can still be fetched
	for i := range 12 {
		key := []byte(fmt.Sprintf("ckey-%03d", i))
		val := []byte(fmt.Sprintf("cval-%03d", i))
		got, err := engine.Get(key)
		if err != nil {
			t.Errorf("failed to get key after compaction: %s: %v", string(key), err)
		}
		if !bytes.Equal(got, val) {
			t.Errorf("value mismatch: got %s, want %s", got, val)
		}
	}
}

func TestEngine_Scan(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 100 // force flushes to create overlapping files
	opts.CompactionThreshold = 10 // disable compaction during setup

	engine, err := NewEngine(dir, opts)
	if err != nil {
		t.Fatalf("failed to open engine: %v", err)
	}
	defer engine.Close()

	// Insert sorted records across memtable and flushed SSTables
	_ = engine.Put([]byte("aa1"), []byte("val_aa1"))
	_ = engine.Put([]byte("aa2"), []byte("val_aa2"))
	time.Sleep(30 * time.Millisecond) // flush L0

	_ = engine.Put([]byte("ab1"), []byte("val_ab1"))
	_ = engine.Put([]byte("ab2"), []byte("val_ab2"))
	_ = engine.Put([]byte("ab3-deleted"), []byte("val_deleted"))
	_ = engine.Delete([]byte("ab3-deleted")) // tombstone
	time.Sleep(30 * time.Millisecond) // flush L0

	_ = engine.Put([]byte("bb1"), []byte("val_bb1")) // active memtable

	// Scan prefix "ab"
	iter := engine.Scan([]byte("ab"))
	defer iter.Close()

	expected := []struct {
		key string
		val string
	}{
		{"ab1", "val_ab1"},
		{"ab2", "val_ab2"},
	}

	idx := 0
	for iter.Valid() {
		k, v := iter.Next()
		if idx >= len(expected) {
			t.Fatalf("extra scan result: key=%s, val=%s", k, v)
		}
		if string(k) != expected[idx].key || string(v) != expected[idx].val {
			t.Errorf("scan mismatch at %d: got (%s, %s), want (%s, %s)",
				idx, k, v, expected[idx].key, expected[idx].val)
		}
		idx++
	}

	if idx != len(expected) {
		t.Errorf("expected %d scan results, got %d", len(expected), idx)
	}
}

func TestEngine_ConcurrentReadCompactionIsolation(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	opts.MaxMemTableSize = 60
	opts.CompactionThreshold = 2

	engine, err := NewEngine(dir, opts)
	if err != nil {
		t.Fatalf("failed to open engine: %v", err)
	}
	defer engine.Close()

	// Insert keys to generate SSTables
	_ = engine.Put([]byte("k1"), []byte("v1"))
	time.Sleep(30 * time.Millisecond)
	_ = engine.Put([]byte("k2"), []byte("v2"))
	time.Sleep(30 * time.Millisecond)

	// Now we have Level 0 files. Let's open a scan iterator.
	// This will pin L0 files in memory via reference counting.
	iter := engine.Scan([]byte("k"))
	
	// Insert more keys to trigger compaction that deletes obsolete L0 files
	_ = engine.Put([]byte("k3"), []byte("v3"))
	time.Sleep(30 * time.Millisecond)
	_ = engine.Put([]byte("k4"), []byte("v4"))
	time.Sleep(30 * time.Millisecond)
	
	// Wait for compaction to complete and attempt to delete L0 files
	time.Sleep(100 * time.Millisecond)

	// Verify that the pinned scanner can still read from the obsoleted files
	// because they are pinned via reference counts!
	expectedKeys := map[string]string{
		"k1": "v1",
		"k2": "v2",
	}

	count := 0
	for iter.Valid() {
		k, v := iter.Next()
		wantV, ok := expectedKeys[string(k)]
		if ok {
			if string(v) != wantV {
				t.Errorf("value mismatch: got %s, want %s", v, wantV)
			}
			count++
		}
	}
	iter.Close()

	if count < 2 {
		t.Errorf("expected to read at least k1 and k2 from the pinned files, read %d keys", count)
	}
}

func TestEngine_ExclusiveDirectoryLock(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()

	engine, err := NewEngine(dir, opts)
	if err != nil {
		t.Fatalf("failed to open first engine: %v", err)
	}
	defer engine.Close()

	// Try to open second engine on same directory
	_, err2 := NewEngine(dir, opts)
	if err2 == nil {
		t.Error("expected error opening second engine concurrently on same directory, got nil")
	} else {
		t.Logf("second engine correctly rejected: %v", err2)
	}
}
