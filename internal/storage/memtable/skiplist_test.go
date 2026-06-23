package memtable

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"testing"
	"unicode/utf8"
)

// TestSkipList_Basic verifies the fundamental read and write contract of the skip
// list. It ensures that a Get on a missing key returns ErrKeyNotFound and that a
// subsequent Put followed by Get returns the correct stored value.
func TestSkipList_Basic(t *testing.T) {
	skipList := NewSkipList(1000, 12)

	_, err := skipList.Get([]byte("key1"))
	if !errors.Is(err, ErrKeyNotFound) {
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
	skipList := NewSkipList(1000, 12)

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
	skipList := NewSkipList(maxSize, 12)

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
	if !errors.Is(err, ErrMemTableFull) {
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
	skipList := NewSkipList(1000, 12)

	_, err := skipList.Get(nil)
	if !errors.Is(err, ErrEmptyKey) {
		t.Errorf("expected ErrEmptyKey for Get with nil key, got %v", err)
	}
	_, err = skipList.Get([]byte(""))
	if !errors.Is(err, ErrEmptyKey) {
		t.Errorf("expected ErrEmptyKey for Get with empty key, got %v", err)
	}

	err = skipList.Put(nil, []byte("value"))
	if !errors.Is(err, ErrEmptyKey) {
		t.Errorf("expected ErrEmptyKey for Put with nil key, got %v", err)
	}
	err = skipList.Put([]byte(""), []byte("value"))
	if !errors.Is(err, ErrEmptyKey) {
		t.Errorf("expected ErrEmptyKey for Put with empty key, got %v", err)
	}

	err = skipList.Delete(nil)
	if !errors.Is(err, ErrEmptyKey) {
		t.Errorf("expected ErrEmptyKey for Delete with nil key, got %v", err)
	}
	err = skipList.Delete([]byte(""))
	if !errors.Is(err, ErrEmptyKey) {
		t.Errorf("expected ErrEmptyKey for Delete with empty key, got %v", err)
	}
}

// TestSkipList_StrictConcurrency verifies that a single writer goroutine and
// multiple concurrent reader goroutines operating on the same key never observe
// a corrupted or malformed value, confirming read-write lock correctness.
func TestSkipList_StrictConcurrency(t *testing.T) {
	skipList := NewSkipList(100000, 12)
	var waitGroup sync.WaitGroup
	key := []byte("shared-key")

	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		for i := 0; i < 1000; i++ {
			val := []byte(fmt.Sprintf("val-%d", i))
			_ = skipList.Put(key, val)
		}
	}()

	for r := 0; r < 5; r++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for i := 0; i < 1000; i++ {
				val, err := skipList.Get(key)
				if err != nil && !errors.Is(err, ErrKeyNotFound) {
					t.Errorf("unexpected error on Get: %v", err)
				}
				if err == nil {
					if !bytes.HasPrefix(val, []byte("val-")) {
						t.Errorf("corrupted value: %s", val)
					}
				}
			}
		}()
	}

	waitGroup.Wait()
}

// TestSkipList_Concurrency is a broad stress test that runs concurrent Puts,
// Deletes, and Iterator traversals across disjoint key ranges simultaneously,
// verifying that no deadlock, panic, or data corruption occurs under contention.
func TestSkipList_Concurrency(t *testing.T) {
	skipList := NewSkipList(100000, 12)
	var waitGroup sync.WaitGroup

	for i := 0; i < 100; i++ {
		waitGroup.Add(1)
		go func(id int) {
			defer waitGroup.Done()
			key := []byte(fmt.Sprintf("key-%03d", id))
			val := []byte(fmt.Sprintf("val-%03d", id))
			if err := skipList.Put(key, val); err != nil {
				t.Errorf("put failed for %s: %v", key, err)
			}
		}(i)
	}

	for i := 100; i < 200; i++ {
		waitGroup.Add(1)
		go func(id int) {
			defer waitGroup.Done()
			key := []byte(fmt.Sprintf("key-%03d", id))
			if err := skipList.Delete(key); err != nil {
				t.Errorf("delete failed for %s: %v", key, err)
			}
		}(i)
	}

	for i := 0; i < 20; i++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			iterator := skipList.NewIterator()
			for iterator.Valid() {
				_, _, _ = iterator.Next()
			}
		}()
	}

	waitGroup.Wait()

	for i := 0; i < 100; i++ {
		key := []byte(fmt.Sprintf("key-%03d", i))
		expected := []byte(fmt.Sprintf("val-%03d", i))
		got, err := skipList.Get(key)
		if err != nil || !bytes.Equal(got, expected) {
			t.Fatalf("unexpected state for %s: got (%q, %v), want (%q, nil)", key, got, err, expected)
		}
	}

	for i := 100; i < 200; i++ {
		key := []byte(fmt.Sprintf("key-%03d", i))
		if _, err := skipList.Get(key); !errors.Is(err, ErrKeyNotFound) {
			t.Fatalf("expected ErrKeyNotFound for deleted/tombstoned key %s, got %v", key, err)
		}
	}
}

// TestSkipList_SortedOrder verifies that the iterator always returns keys in
// ascending lexicographic order, regardless of insertion order.
func TestSkipList_SortedOrder(t *testing.T) {
	skipList := NewSkipList(10000, 12)

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
	skipList := NewSkipList(1000, 12)

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
	skipList := NewSkipList(1000, 12)

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
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("expected ErrKeyNotFound after duplicate Delete, got %v", err)
	}
}

// TestSkipList_IteratorExhaustion verifies that calling Next on an exhausted
// iterator returns nil sentinel values and does not panic.
func TestSkipList_IteratorExhaustion(t *testing.T) {
	skipList := NewSkipList(1000, 12)

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
	skipList := NewSkipList(4, 12)

	err := skipList.Put([]byte("ab"), []byte("cd"))
	if err != nil {
		t.Fatalf("failed to Put: %v", err)
	}

	err = skipList.Delete([]byte("xyz"))
	if !errors.Is(err, ErrMemTableFull) {
		t.Errorf("expected ErrMemTableFull when inserting tombstone over capacity, got %v", err)
	}
}

// TestSkipList_ConfigurableMaxLevel verifies that configuring a custom maxLevel
// works properly and that the skip list operates normally with different heights.
func TestSkipList_ConfigurableMaxLevel(t *testing.T) {
	skiplist := NewSkipList(1000, 0)
	if skiplist.maxLevel != 12 {
		t.Errorf("expected maxLevel to default to 12 for invalid value 0, got %d", skiplist.maxLevel)
	}

	skiplist = NewSkipList(1000, 33)
	if skiplist.maxLevel != 12 {
		t.Errorf("expected maxLevel to default to 12 for invalid value 33, got %d", skiplist.maxLevel)
	}

	skiplist = NewSkipList(1000, 1)
	if skiplist.maxLevel != 1 {
		t.Errorf("expected maxLevel 1, got %d", skiplist.maxLevel)
	}

	skiplist = NewSkipList(1000, 32)
	if skiplist.maxLevel != 32 {
		t.Errorf("expected maxLevel 32, got %d", skiplist.maxLevel)
	}
	skipList := NewSkipList(1000, 4)
	if skipList.maxLevel != 4 {
		t.Fatalf("expected maxLevel 4, got %d", skipList.maxLevel)
	}

	err := skipList.Put([]byte("key"), []byte("value"))
	if err != nil {
		t.Fatalf("failed to Put with custom maxLevel: %v", err)
	}

	val, err := skipList.Get([]byte("key"))
	if err != nil {
		t.Fatalf("failed to Get with custom maxLevel: %v", err)
	}
	if !bytes.Equal(val, []byte("value")) {
		t.Fatalf("expected value, got %s", val)
	}
}

// unicodeRuneRanges defines unicode blocks used to generate random multi-byte
// keys and values. Each range is a [lo, hi) pair of rune values.
var unicodeRuneRanges = []struct{ lo, hi rune }{
	{0x0041, 0x005B},   // Basic Latin uppercase A-Z
	{0x00C0, 0x0100},   // Latin Extended (accented characters)
	{0x0400, 0x0450},   // Cyrillic
	{0x0600, 0x0640},   // Arabic
	{0x0900, 0x0950},   // Devanagari
	{0x3040, 0x3097},   // Hiragana
	{0x4E00, 0x4F00},   // CJK Unified Ideographs (subset)
	{0xAC00, 0xAC80},   // Hangul Syllables (subset)
	{0x1F600, 0x1F650}, // Emoticons / Emoji
}

// randomUnicodeString generates a random string of n runes drawn uniformly from
// the unicode blocks defined in unicodeRuneRanges.
func randomUnicodeString(rng *rand.Rand, n int) string {
	buf := make([]byte, 0, n*4)
	for i := 0; i < n; i++ {
		block := unicodeRuneRanges[rng.Intn(len(unicodeRuneRanges))]
		r := block.lo + rng.Int31n(block.hi-block.lo)
		buf = utf8.AppendRune(buf, r)
	}
	return string(buf)
}

// TestSkipList_UnicodeKeys verifies that the skip list correctly handles
// arbitrary multi-byte UTF-8 encoded keys and values across Put, Get, Delete,
// and Iterator operations. It covers characters from Latin Extended, Cyrillic,
// Arabic, Devanagari, Hiragana, CJK, Hangul, and Emoji blocks.
func TestSkipList_UnicodeKeys(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	t.Run("PutGetRoundtrip", func(t *testing.T) {
		skipList := NewSkipList(100000, 12)

		// Fixed multi-script keys that exercise different byte widths.
		entries := map[string]string{
			"日本語テスト":   "値一",              // CJK + Hiragana (3-byte runes)
			"Привет":   "Мир",             // Cyrillic (2-byte runes)
			"مفتاح":    "قيمة",            // Arabic (2-byte runes)
			"हिंदी":    "मान",             // Devanagari (3-byte runes)
			"한국어":      "값",               // Hangul (3-byte runes)
			"🦊🐧🔥":      "🎉🚀",              // Emoji (4-byte runes)
			"café":     "résumé",          // Latin with accents (mixed 1-2 byte runes)
			"mix混合кл🎮": "val値ue🧪",         // Mixed scripts in single key
			"Z̤̈":      "combining-marks", // Latin + combining diacriticals
			"a\x00b":   "embedded-null",   // Embedded null byte in key
		}

		for k, v := range entries {
			if err := skipList.Put([]byte(k), []byte(v)); err != nil {
				t.Fatalf("Put(%q) failed: %v", k, err)
			}
		}

		for k, expectedVal := range entries {
			got, err := skipList.Get([]byte(k))
			if err != nil {
				t.Fatalf("Get(%q) failed: %v", k, err)
			}
			if !bytes.Equal(got, []byte(expectedVal)) {
				t.Errorf("Get(%q) = %q, want %q", k, got, expectedVal)
			}
		}
	})

	t.Run("UpdateUnicodeValue", func(t *testing.T) {
		skipList := NewSkipList(100000, 12)
		key := []byte("更新キー")
		original := []byte("元の値")
		updated := []byte("新しい値🆕")

		if err := skipList.Put(key, original); err != nil {
			t.Fatalf("Put original failed: %v", err)
		}
		if err := skipList.Put(key, updated); err != nil {
			t.Fatalf("Put update failed: %v", err)
		}

		got, err := skipList.Get(key)
		if err != nil {
			t.Fatalf("Get after update failed: %v", err)
		}
		if !bytes.Equal(got, updated) {
			t.Errorf("Get = %q, want %q", got, updated)
		}
	})

	t.Run("DeleteUnicodeKey", func(t *testing.T) {
		skipList := NewSkipList(100000, 12)
		key := []byte("удалить🗑️")

		if err := skipList.Put(key, []byte("存在する")); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
		if err := skipList.Delete(key); err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		_, err := skipList.Get(key)
		if !errors.Is(err, ErrKeyNotFound) {
			t.Errorf("expected ErrKeyNotFound after Delete, got %v", err)
		}

		// Verify tombstone is visible to iterator.
		iter := skipList.NewIterator()
		if !iter.Valid() {
			t.Fatal("expected iterator to be valid (tombstone should be present)")
		}
		k, v, deleted := iter.Next()
		if !bytes.Equal(k, key) {
			t.Errorf("iterator key = %q, want %q", k, key)
		}
		if !deleted {
			t.Error("expected tombstone marker on deleted unicode key")
		}
		if len(v) != 0 {
			t.Errorf("expected nil/empty value for tombstone, got %q", v)
		}
	})

	t.Run("SortedOrderByBytes", func(t *testing.T) {
		skipList := NewSkipList(100000, 12)
		keys := []string{
			"🦊emoji",      // starts with F0 9F A6 8A (4-byte)
			"Яcyrillic",   // starts with D0 AF (2-byte)
			"ascii-first", // starts with 61 (1-byte)
			"日cjk",        // starts with E6 97 A5 (3-byte)
			"àlatin-ext",  // starts with C3 A0 (2-byte)
		}

		for _, k := range keys {
			if err := skipList.Put([]byte(k), []byte("v")); err != nil {
				t.Fatalf("Put(%q) failed: %v", k, err)
			}
		}

		// Expected order: byte-level lexicographic sort.
		sorted := make([]string, len(keys))
		copy(sorted, keys)
		sort.Slice(sorted, func(i, j int) bool {
			return bytes.Compare([]byte(sorted[i]), []byte(sorted[j])) < 0
		})

		iter := skipList.NewIterator()
		for idx, expected := range sorted {
			if !iter.Valid() {
				t.Fatalf("iterator exhausted at index %d, expected key %q", idx, expected)
			}
			k, _, _ := iter.Next()
			if !bytes.Equal(k, []byte(expected)) {
				t.Errorf("position %d: got key %q, want %q", idx, k, expected)
			}
		}
		if iter.Valid() {
			t.Error("iterator has extra elements after consuming all expected keys")
		}
	})

	t.Run("SizeTrackingMultiByte", func(t *testing.T) {
		skipList := NewSkipList(100000, 12)

		key := []byte("π") // U+03C0 → 2 bytes in UTF-8
		val := []byte("🎲") // U+1F3B2 → 4 bytes in UTF-8

		if err := skipList.Put(key, val); err != nil {
			t.Fatalf("Put failed: %v", err)
		}

		expectedSize := int64(len(key) + len(val)) // 2 + 4 = 6
		if skipList.currentSizeBytes != expectedSize {
			t.Errorf("size = %d, want %d (key %d bytes + val %d bytes)",
				skipList.currentSizeBytes, expectedSize, len(key), len(val))
		}
	})

	t.Run("RandomUnicodeBulk", func(t *testing.T) {
		skipList := NewSkipList(500000, 12)
		const numEntries = 200
		entries := make(map[string]string, numEntries)

		// Generate unique random unicode keys and values.
		for len(entries) < numEntries {
			k := randomUnicodeString(rng, 3+rng.Intn(8))
			v := randomUnicodeString(rng, 1+rng.Intn(12))
			entries[k] = v
		}

		for k, v := range entries {
			if err := skipList.Put([]byte(k), []byte(v)); err != nil {
				t.Fatalf("Put(%q) failed: %v", k, err)
			}
		}

		for k, expectedVal := range entries {
			got, err := skipList.Get([]byte(k))
			if err != nil {
				t.Errorf("Get(%q) failed: %v", k, err)
				continue
			}
			if !bytes.Equal(got, []byte(expectedVal)) {
				t.Errorf("Get(%q) = %q, want %q", k, got, expectedVal)
			}
		}

		// Verify iterator produces keys in byte-sorted order.
		iter := skipList.NewIterator()
		var prev []byte
		count := 0
		for iter.Valid() {
			k, _, _ := iter.Next()
			if prev != nil && bytes.Compare(prev, k) >= 0 {
				t.Errorf("iterator order violation: %q >= %q", prev, k)
			}
			prev = k
			count++
		}
		if count != numEntries {
			t.Errorf("iterator yielded %d entries, want %d", count, numEntries)
		}
	})

	t.Run("ConcurrentUnicode", func(t *testing.T) {
		skipList := NewSkipList(500000, 12)
		const goroutines = 10
		const opsPerGoroutine = 100
		var wg sync.WaitGroup

		wg.Add(goroutines)
		for g := 0; g < goroutines; g++ {
			go func(id int) {
				defer wg.Done()
				localRng := rand.New(rand.NewSource(int64(id * 1000)))
				for i := 0; i < opsPerGoroutine; i++ {
					k := []byte(fmt.Sprintf("г%d-키%d-%s", id, i, randomUnicodeString(localRng, 2)))
					v := []byte(randomUnicodeString(localRng, 3))
					_ = skipList.Put(k, v)

					got, err := skipList.Get(k)
					if err != nil {
						t.Errorf("goroutine %d: Get(%q) failed: %v", id, k, err)
						continue
					}
					if !bytes.Equal(got, v) {
						t.Errorf("goroutine %d: Get(%q) = %q, want %q", id, k, got, v)
					}
				}
			}(g)
		}
		wg.Wait()
	})
}
