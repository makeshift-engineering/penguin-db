package memtable

import (
	"bytes"
	"testing"
)

// TestSkipList_Basic verifies the fundamental read and write contract of the skip
// list. It ensures that a Get on a missing key returns ErrKeyNotFound and that a
// subsequent Put followed by Get returns the correct stored value.
func TestSkipList_Basic(t *testing.T) {
	skipList := NewSkipList(1000)

	_, err := skipList.Get([]byte("key1"))
	if err != ErrKeyNotFound {
		t.Fatalf("expected ErrKeyNotFound, got %v", err)
	}

	err = skipList.Put([]byte("key1"), []byte("val1"))
	if err != nil {
		t.Fatalf("failed to Put: %v", err)
	}

	val, err := skipList.Get([]byte("key1"))
	if err != nil {
		t.Fatalf("failed to Get: %v", err)
	}
	if !bytes.Equal(val, []byte("val1")) {
		t.Fatalf("expected val1, got %s", val)
	}
}

// TestSkipList_Delete verifies LSM-tree tombstone semantics. It confirms that
// deleting a non-existent key inserts a visible tombstone node and that a
// subsequent Put on the same key clears the tombstone and restores visibility.
func TestSkipList_Delete(t *testing.T) {
	skipList := NewSkipList(1000)

	err := skipList.Delete([]byte("key1"))
	if err != nil {
		t.Fatalf("failed to Delete: %v", err)
	}

	iterator := skipList.NewIterator()
	if !iterator.Valid() {
		t.Fatalf("expected iterator to be valid since Delete should insert a tombstone")
	}
	k, v, deleted := iterator.Next()
	if !bytes.Equal(k, []byte("key1")) {
		t.Fatalf("expected key1, got %s", k)
	}
	if !deleted {
		t.Fatalf("expected tombstone to be marked as deleted")
	}
	if len(v) != 0 {
		t.Fatalf("expected value of deleted key to be empty/nil, got %s", v)
	}

	err = skipList.Put([]byte("key1"), []byte("alive"))
	if err != nil {
		t.Fatalf("failed to Put deleted key: %v", err)
	}
	val, err := skipList.Get([]byte("key1"))
	if err != nil {
		t.Fatalf("failed to Get key after re-put: %v", err)
	}
	if !bytes.Equal(val, []byte("alive")) {
		t.Fatalf("expected alive, got %s", val)
	}

	iterator = skipList.NewIterator()
	if !iterator.Valid() {
		t.Fatalf("expected iterator to be valid after putting back key1")
	}
	k, v, deleted = iterator.Next()
	if !bytes.Equal(k, []byte("key1")) {
		t.Fatalf("expected key1, got %s", k)
	}
	if deleted {
		t.Fatalf("expected deleted to be false after putting back key1")
	}
	if !bytes.Equal(v, []byte("alive")) {
		t.Fatalf("expected value to be alive, got %s", v)
	}
}

// TestSkipList_SizeTracking verifies that the internal byte counter is updated
// accurately for new inserts, in-place value updates, and failed writes that
// exceed capacity, ensuring no size leakage occurs on a rejected Put.
func TestSkipList_SizeTracking(t *testing.T) {
	maxSize := int64(20)
	skipList := NewSkipList(maxSize)

	err := skipList.Put([]byte("k"), []byte("v"))
	if err != nil {
		t.Fatalf("failed to Put: %v", err)
	}
	if skipList.currentSizeBytes != 2 {
		t.Fatalf("expected size 2, got %d", skipList.currentSizeBytes)
	}

	err = skipList.Put([]byte("k2"), []byte("v222222222"))
	if err != nil {
		t.Fatalf("failed to Put k2: %v", err)
	}
	if skipList.currentSizeBytes != 14 {
		t.Fatalf("expected size 14, got %d", skipList.currentSizeBytes)
	}

	err = skipList.Put([]byte("k"), []byte("v5555"))
	if err != nil {
		t.Errorf("failed to Put update close to capacity: %v", err)
	}

	if skipList.currentSizeBytes != 18 {
		t.Errorf("expected size 18, got %d", skipList.currentSizeBytes)
	}

	err = skipList.Put([]byte("k"), []byte("vOverTheLimit"))
	if err != ErrMemTableFull {
		t.Errorf("expected ErrMemTableFull on limit overrun, got %v", err)
	}
	if skipList.currentSizeBytes != 18 {
		t.Errorf("expected size to remain 18 after failed Put, but got %d", skipList.currentSizeBytes)
	}

	err = skipList.Delete([]byte("k"))
	if err != nil {
		t.Fatalf("failed to delete key: %v", err)
	}
	if skipList.currentSizeBytes != 13 {
		t.Fatalf("expected size 13 after delete, got %d", skipList.currentSizeBytes)
	}

	err = skipList.Put([]byte("k3"), []byte("v3"))
	if err != nil {
		t.Fatalf("failed to Put k3 after delete freed space: %v", err)
	}
	if skipList.currentSizeBytes != 17 {
		t.Fatalf("expected size 17 after putting new value, got %d", skipList.currentSizeBytes)
	}
}

// TestSkipList_EmptyAndNil verifies that the skip list actively rejects nil
// and empty-slice keys across all exported methods (Put, Get, Delete) by
// returning ErrEmptyKey, preventing hash ring corruption downstream.
func TestSkipList_EmptyAndNil(t *testing.T) {
	skipList := NewSkipList(1000)

	_, err := skipList.Get(nil)
	if err != ErrEmptyKey {
		t.Errorf("expected ErrEmptyKey for Get with nil key, got %v", err)
	}
	_, err = skipList.Get([]byte(""))
	if err != ErrEmptyKey {
		t.Errorf("expected ErrEmptyKey for Get with empty key, got %v", err)
	}

	err = skipList.Put(nil, []byte("value"))
	if err != ErrEmptyKey {
		t.Errorf("expected ErrEmptyKey for Put with nil key, got %v", err)
	}
	err = skipList.Put([]byte(""), []byte("value"))
	if err != ErrEmptyKey {
		t.Errorf("expected ErrEmptyKey for Put with empty key, got %v", err)
	}

	err = skipList.Delete(nil)
	if err != ErrEmptyKey {
		t.Errorf("expected ErrEmptyKey for Delete with nil key, got %v", err)
	}
	err = skipList.Delete([]byte(""))
	if err != ErrEmptyKey {
		t.Errorf("expected ErrEmptyKey for Delete with empty key, got %v", err)
	}
}

// TestSkipList_SortedOrder verifies that the iterator always returns keys in
// ascending lexicographic order, regardless of insertion order.
func TestSkipList_SortedOrder(t *testing.T) {
	skipList := NewSkipList(10000)

	keys := []string{"zebra", "apple", "mango", "banana", "cherry"}
	for _, key := range keys {
		err := skipList.Put([]byte(key), []byte("v"))
		if err != nil {
			t.Fatalf("failed to Put %s: %v", key, err)
		}
	}

	expectedOrder := []string{"apple", "banana", "cherry", "mango", "zebra"}
	iterator := skipList.NewIterator()
	for _, expectedKey := range expectedOrder {
		if !iterator.Valid() {
			t.Fatalf("iterator exhausted early, expected key %s", expectedKey)
		}
		key, _, _ := iterator.Next()
		if !bytes.Equal(key, []byte(expectedKey)) {
			t.Errorf("expected key %s, got %s", expectedKey, key)
		}
	}
	if iterator.Valid() {
		t.Errorf("iterator has extra elements after all expected keys were consumed")
	}
}

// TestSkipList_DeleteSizeAccounting verifies that deleting an existing key
// correctly subtracts the value length from the size counter while retaining
// the key length, since the tombstone node must be preserved.
func TestSkipList_DeleteSizeAccounting(t *testing.T) {
	skipList := NewSkipList(1000)

	err := skipList.Put([]byte("key"), []byte("value"))
	if err != nil {
		t.Fatalf("failed to Put: %v", err)
	}
	sizeAfterPut := skipList.currentSizeBytes

	err = skipList.Delete([]byte("key"))
	if err != nil {
		t.Fatalf("failed to Delete: %v", err)
	}

	expectedSizeAfterDelete := sizeAfterPut - int64(len("value"))
	if skipList.currentSizeBytes != expectedSizeAfterDelete {
		t.Errorf("expected size %d after Delete, got %d", expectedSizeAfterDelete, skipList.currentSizeBytes)
	}
}

// TestSkipList_DuplicateDelete verifies that deleting the same key twice is
// idempotent and does not corrupt the size counter or return an error.
func TestSkipList_DuplicateDelete(t *testing.T) {
	skipList := NewSkipList(1000)

	err := skipList.Put([]byte("key"), []byte("value"))
	if err != nil {
		t.Fatalf("failed to Put: %v", err)
	}

	err = skipList.Delete([]byte("key"))
	if err != nil {
		t.Fatalf("first Delete failed: %v", err)
	}
	sizeAfterFirstDelete := skipList.currentSizeBytes

	err = skipList.Delete([]byte("key"))
	if err != nil {
		t.Fatalf("second Delete failed: %v", err)
	}
	if skipList.currentSizeBytes != sizeAfterFirstDelete {
		t.Errorf("size changed after duplicate Delete: before %d, after %d",
			sizeAfterFirstDelete, skipList.currentSizeBytes)
	}

	_, err = skipList.Get([]byte("key"))
	if err != ErrKeyNotFound {
		t.Errorf("expected ErrKeyNotFound after duplicate Delete, got %v", err)
	}
}

// TestSkipList_IteratorExhaustion verifies that calling Next on an exhausted
// iterator returns nil sentinel values and does not panic.
func TestSkipList_IteratorExhaustion(t *testing.T) {
	skipList := NewSkipList(1000)

	err := skipList.Put([]byte("only-key"), []byte("val"))
	if err != nil {
		t.Fatalf("failed to Put: %v", err)
	}

	iterator := skipList.NewIterator()

	key, val, isDeleted := iterator.Next()
	if !bytes.Equal(key, []byte("only-key")) {
		t.Errorf("expected only-key, got %s", key)
	}
	if !bytes.Equal(val, []byte("val")) {
		t.Errorf("expected val, got %s", val)
	}
	if isDeleted {
		t.Errorf("expected isDeleted=false, got true")
	}

	if iterator.Valid() {
		t.Errorf("expected iterator to be invalid after consuming all nodes")
	}

	key, val, isDeleted = iterator.Next()
	if key != nil || val != nil || isDeleted {
		t.Errorf("exhausted Next() should return (nil, nil, false), got (%s, %s, %v)", key, val, isDeleted)
	}
}

// TestSkipList_TombstoneSizeLimit verifies that inserting a tombstone for a
// non-existent key is correctly rejected when the memtable is at capacity.
func TestSkipList_TombstoneSizeLimit(t *testing.T) {
	skipList := NewSkipList(4)

	err := skipList.Put([]byte("ab"), []byte("cd"))
	if err != nil {
		t.Fatalf("failed to Put: %v", err)
	}

	err = skipList.Delete([]byte("xyz"))
	if err != ErrMemTableFull {
		t.Errorf("expected ErrMemTableFull when inserting tombstone over capacity, got %v", err)
	}
}
