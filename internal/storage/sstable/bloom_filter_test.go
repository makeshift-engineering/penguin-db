package sstable

import (
	"crypto/rand"
	"fmt"
	"testing"
)

// bitsPerKey is the standard bits-per-key value used across all tests,
// matching the recommended 10 bits/key for the LSM-tree storage engine.
const bitsPerKey = 10

// TestBloomFilter_BasicMembership verifies the fundamental contract:
// added keys must always be found, and a key that was never added should
// (with very high probability) not be reported as present.
func TestBloomFilter_BasicMembership(t *testing.T) {
	bf := NewBloomFilter(100, bitsPerKey)

	keys := [][]byte{
		[]byte("key1"),
		[]byte("key2"),
		[]byte("key3"),
	}

	for _, k := range keys {
		bf.Add(k)
	}

	for _, k := range keys {
		if !bf.MayContain(k) {
			t.Errorf("expected %s to be in bloom filter", string(k))
		}
	}

	if bf.MayContain([]byte("key4")) {
		t.Logf("false positive for key4 (acceptable but noted)")
	}
}

// TestBloomFilter_NoFalseNegatives adds many keys and asserts that every
// single one is reported as present. A false negative is a correctness bug.
func TestBloomFilter_NoFalseNegatives(t *testing.T) {
	numKeys := 5000
	bf := NewBloomFilter(numKeys, bitsPerKey)

	keys := make([][]byte, numKeys)
	for i := 0; i < numKeys; i++ {
		keys[i] = []byte(fmt.Sprintf("present-key-%06d", i))
		bf.Add(keys[i])
	}

	for i, k := range keys {
		if !bf.MayContain(k) {
			t.Fatalf("false negative at index %d for key %s — bloom filters must never have false negatives", i, k)
		}
	}
}

// TestBloomFilter_EmptyFilter verifies that querying an empty filter
// returns false (no keys have been added, so nothing should match).
func TestBloomFilter_EmptyFilter(t *testing.T) {
	bf := NewBloomFilter(100, bitsPerKey)

	if bf.MayContain([]byte("any_key")) {
		t.Errorf("empty bloom filter should not contain any keys")
	}
}

// TestBloomFilter_EmptyKey verifies that empty byte slice keys can be
// added and queried without panics.
func TestBloomFilter_EmptyKey(t *testing.T) {
	bf := NewBloomFilter(100, bitsPerKey)

	emptyKey := []byte("")
	if bf.MayContain(emptyKey) {
		t.Errorf("empty bloom filter should not contain empty key")
	}

	bf.Add(emptyKey)

	if !bf.MayContain(emptyKey) {
		t.Errorf("expected empty key to be in bloom filter after adding")
	}
}

// TestBloomFilter_NilKey verifies that nil keys can be added and queried
// without panics, and behave consistently.
func TestBloomFilter_NilKey(t *testing.T) {
	bf := NewBloomFilter(100, bitsPerKey)

	bf.Add(nil)

	if !bf.MayContain(nil) {
		t.Errorf("expected nil key to be found after adding")
	}
}

// TestBloomFilter_DuplicateAdd verifies that adding the same key multiple
// times does not corrupt the filter and the key remains queryable.
func TestBloomFilter_DuplicateAdd(t *testing.T) {
	bf := NewBloomFilter(100, bitsPerKey)

	key := []byte("duplicate_key")

	bf.Add(key)
	bf.Add(key)
	bf.Add(key)

	if !bf.MayContain(key) {
		t.Errorf("expected duplicate_key to be in bloom filter after multiple adds")
	}
}

// TestBloomFilter_LargeKey verifies that very large keys (1 MB) can be
// added and queried without errors.
func TestBloomFilter_LargeKey(t *testing.T) {
	bf := NewBloomFilter(100, bitsPerKey)

	largeKey := make([]byte, 1024*1024) // 1MB
	if _, err := rand.Read(largeKey); err != nil {
		t.Fatalf("failed to generate random key: %v", err)
	}

	bf.Add(largeKey)

	if !bf.MayContain(largeKey) {
		t.Errorf("expected large key to be in bloom filter after adding")
	}
}

// TestBloomFilter_FalsePositiveRate inserts numKeys keys and then probes with
// a disjoint set of absent keys, asserting the observed false positive rate
// stays within a reasonable bound derived from the configured bits per key.
func TestBloomFilter_FalsePositiveRate(t *testing.T) {
	numKeys := 10000
	bf := NewBloomFilter(numKeys, bitsPerKey)

	for i := 0; i < numKeys; i++ {
		key := []byte(fmt.Sprintf("present-key-%d", i))
		bf.Add(key)
	}

	// Verify all added keys are present (no false negatives allowed)
	for i := 0; i < numKeys; i++ {
		key := []byte(fmt.Sprintf("present-key-%d", i))
		if !bf.MayContain(key) {
			t.Fatalf("false negative detected for key %s", key)
		}
	}

	// Check false positive rate with absent keys
	falsePositives := 0
	numQueries := 100000
	for i := 0; i < numQueries; i++ {
		key := []byte(fmt.Sprintf("absent-key-%d", i))
		if bf.MayContain(key) {
			falsePositives++
		}
	}

	// With 10 bits/key, theoretical FPR ≈ 0.82%. We allow up to 2% to
	// account for hash quality variance while still catching regressions.
	fpr := float64(falsePositives) / float64(numQueries)
	t.Logf("observed false positive rate: %.4f%% (%d / %d)", fpr*100, falsePositives, numQueries)

	if fpr > 0.02 {
		t.Errorf("false positive rate too high: got %.4f, want <= 0.02", fpr)
	}
}

// TestBloomFilter_FPRScalesWithBitsPerKey verifies that increasing bits per
// key monotonically decreases the false positive rate.
func TestBloomFilter_FPRScalesWithBitsPerKey(t *testing.T) {
	numKeys := 5000
	numQueries := 50000
	bpkValues := []int{5, 10, 15, 20}
	prevFPR := 1.0

	for _, bpk := range bpkValues {
		bf := NewBloomFilter(numKeys, bpk)

		for i := 0; i < numKeys; i++ {
			bf.Add([]byte(fmt.Sprintf("key-%d", i)))
		}

		falsePositives := 0
		for i := 0; i < numQueries; i++ {
			if bf.MayContain([]byte(fmt.Sprintf("miss-%d", i))) {
				falsePositives++
			}
		}

		fpr := float64(falsePositives) / float64(numQueries)
		t.Logf("bitsPerKey=%d  FPR=%.4f%%", bpk, fpr*100)

		if fpr >= prevFPR {
			t.Errorf("FPR did not decrease with more bits: bpk=%d fpr=%.4f >= prevFPR=%.4f", bpk, fpr, prevFPR)
		}
		prevFPR = fpr
	}
}

// TestBloomFilter_SmallCapacity verifies a filter created for a single key
// works correctly.
func TestBloomFilter_SmallCapacity(t *testing.T) {
	bf := NewBloomFilter(1, bitsPerKey)
	bf.Add([]byte("only_key"))
	if !bf.MayContain([]byte("only_key")) {
		t.Errorf("expected 'only_key' to be present")
	}
}

// TestBloomFilter_ZeroKeys verifies that a filter created with zero expected
// keys still functions (minimum bit vector is enforced).
func TestBloomFilter_ZeroKeys(t *testing.T) {
	bf := NewBloomFilter(0, bitsPerKey)

	if bf.MayContain([]byte("anything")) {
		t.Errorf("filter with 0 expected keys should not match arbitrary queries")
	}

	// Should still be able to add and query
	bf.Add([]byte("inserted"))
	if !bf.MayContain([]byte("inserted")) {
		t.Errorf("expected 'inserted' to be found even in zero-capacity filter")
	}
}

// TestBloomFilter_NegativeKeys verifies that negative numKeys is clamped to
// zero and the filter still behaves correctly.
func TestBloomFilter_NegativeKeys(t *testing.T) {
	bf := NewBloomFilter(-10, bitsPerKey)

	// Should not panic and should have a valid minimum-size bit vector
	if len(bf.Bytes()) == 0 {
		t.Errorf("expected non-empty bit vector even with negative numKeys")
	}

	bf.Add([]byte("key"))
	if !bf.MayContain([]byte("key")) {
		t.Errorf("expected key to be found after adding to negative-capacity filter")
	}
}

// TestBloomFilter_ZeroBitsPerKey verifies that zero bits-per-key produces a
// filter with minimum size and at least one hash function (k clamped to 1).
func TestBloomFilter_ZeroBitsPerKey(t *testing.T) {
	bf := NewBloomFilter(100, 0)

	// Should use minimum bit vector (64 bits = 8 bytes)
	if len(bf.Bytes()) < 8 {
		t.Errorf("expected at least 8 bytes for minimum bit vector, got %d", len(bf.Bytes()))
	}

	// numHashes should be clamped to at least 1
	if bf.NumHashes() < 1 {
		t.Errorf("expected at least 1 hash function, got %d", bf.NumHashes())
	}

	bf.Add([]byte("test"))
	if !bf.MayContain([]byte("test")) {
		t.Errorf("expected key to be found after adding with 0 bitsPerKey")
	}
}

// TestBloomFilter_MinimumBitVectorSize verifies the 64-bit floor on the bit
// vector even when numKeys * bitsPerKey would produce something smaller.
func TestBloomFilter_MinimumBitVectorSize(t *testing.T) {
	bf := NewBloomFilter(1, 1) // 1 key * 1 bit = 1 bit, should be bumped to 64

	// 64 bits = 8 bytes minimum
	if len(bf.Bytes()) < 8 {
		t.Errorf("expected at least 8 bytes (64 bits minimum), got %d bytes", len(bf.Bytes()))
	}
}

// TestBloomFilter_API verifies the exported Bytes() and NumHashes() accessors
// return sensible values.
func TestBloomFilter_API(t *testing.T) {
	bf := NewBloomFilter(100, bitsPerKey)

	if len(bf.Bytes()) == 0 {
		t.Errorf("expected Bytes() to return a non-empty slice")
	}

	if bf.NumHashes() == 0 {
		t.Errorf("expected NumHashes() to be > 0, got %d", bf.NumHashes())
	}
}

// TestBloomFilter_BytesSizeMatchesBitsPerKey verifies the allocated byte
// slice size is consistent with the bits-per-key configuration.
func TestBloomFilter_BytesSizeMatchesBitsPerKey(t *testing.T) {
	numKeys := 1000
	bf := NewBloomFilter(numKeys, bitsPerKey)

	expectedBits := numKeys * bitsPerKey
	expectedBytes := (expectedBits + 7) / 8

	if len(bf.Bytes()) != expectedBytes {
		t.Errorf("expected %d bytes, got %d", expectedBytes, len(bf.Bytes()))
	}
}

// TestBloomFilter_NumHashesOptimal verifies the number of hash functions
// matches the formula k = bitsPerKey * ln(2), clamped to [1, 30].
func TestBloomFilter_NumHashesOptimal(t *testing.T) {
	tests := []struct {
		bitsPerKey      int
		expectedNumHash uint8
	}{
		{1, 1},   // 1 * 0.693 = 0.693 → clamped to 1
		{5, 3},   // 5 * 0.693 = 3.46  → 3
		{10, 6},  // 10 * 0.693 = 6.93 → 6
		{15, 10}, // 15 * 0.693 = 10.4 → 10
		{20, 13}, // 20 * 0.693 = 13.8 → 13
		{44, 30}, // 44 * 0.693 = 30.5 → capped at 30
		{50, 30}, // 50 * 0.693 = 34.6 → capped at 30
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("bpk=%d", tt.bitsPerKey), func(t *testing.T) {
			bf := NewBloomFilter(100, tt.bitsPerKey)
			if bf.NumHashes() != tt.expectedNumHash {
				t.Errorf("bitsPerKey=%d: expected %d hash functions, got %d",
					tt.bitsPerKey, tt.expectedNumHash, bf.NumHashes())
			}
		})
	}
}

// TestBloomFilter_Deterministic verifies that the same key always produces
// the same hash positions, so two independent filters with the same
// configuration and keys produce identical bit vectors.
func TestBloomFilter_Deterministic(t *testing.T) {
	keys := [][]byte{
		[]byte("alpha"),
		[]byte("bravo"),
		[]byte("charlie"),
		[]byte("delta"),
	}

	bf1 := NewBloomFilter(100, bitsPerKey)
	bf2 := NewBloomFilter(100, bitsPerKey)

	for _, k := range keys {
		bf1.Add(k)
		bf2.Add(k)
	}

	bits1 := bf1.Bytes()
	bits2 := bf2.Bytes()

	if len(bits1) != len(bits2) {
		t.Fatalf("byte lengths differ: %d vs %d", len(bits1), len(bits2))
	}
	for i := range bits1 {
		if bits1[i] != bits2[i] {
			t.Fatalf("bit vectors differ at byte %d: 0x%02x vs 0x%02x", i, bits1[i], bits2[i])
		}
	}
}

// TestBloomFilter_Saturation verifies that when far more keys are added than
// the filter was sized for, MayContain still never panics. The FPR will be
// very high (approaching 1.0) but the filter must remain usable.
func TestBloomFilter_Saturation(t *testing.T) {
	bf := NewBloomFilter(10, bitsPerKey) // sized for 10 keys

	// Insert 10000 keys — 1000x overload
	for i := 0; i < 10000; i++ {
		bf.Add([]byte(fmt.Sprintf("overload-%d", i)))
	}

	// Just verify no panic on query
	for i := 0; i < 100; i++ {
		bf.MayContain([]byte(fmt.Sprintf("query-%d", i)))
	}
}

// TestBloomFilter_DisjointKeySets inserts keys with a specific prefix and
// queries with a completely different prefix, verifying few false positives.
func TestBloomFilter_DisjointKeySets(t *testing.T) {
	numKeys := 1000
	bf := NewBloomFilter(numKeys, bitsPerKey)

	for i := 0; i < numKeys; i++ {
		bf.Add([]byte(fmt.Sprintf("SET_A/%08d", i)))
	}

	// All SET_A keys must be found
	for i := 0; i < numKeys; i++ {
		if !bf.MayContain([]byte(fmt.Sprintf("SET_A/%08d", i))) {
			t.Fatalf("false negative for SET_A key %d", i)
		}
	}

	// SET_B keys should have very few false positives
	falsePositives := 0
	for i := 0; i < numKeys; i++ {
		if bf.MayContain([]byte(fmt.Sprintf("SET_B/%08d", i))) {
			falsePositives++
		}
	}

	fpr := float64(falsePositives) / float64(numKeys)
	if fpr > 0.03 {
		t.Errorf("disjoint set FPR too high: %.4f", fpr)
	}
}

// TestBloomFilter_SingleByteKeys verifies that all 256 possible single-byte
// keys can be added and queried without collisions causing false negatives.
func TestBloomFilter_SingleByteKeys(t *testing.T) {
	bf := NewBloomFilter(256, bitsPerKey)

	for i := 0; i < 256; i++ {
		bf.Add([]byte{byte(i)})
	}

	for i := 0; i < 256; i++ {
		if !bf.MayContain([]byte{byte(i)}) {
			t.Errorf("false negative for single byte key 0x%02x", i)
		}
	}
}

// TestBloomFilter_BinaryKeys verifies the filter works correctly with keys
// containing null bytes and other non-printable characters.
func TestBloomFilter_BinaryKeys(t *testing.T) {
	bf := NewBloomFilter(100, bitsPerKey)

	binaryKeys := [][]byte{
		{0x00, 0x00, 0x00},
		{0xFF, 0xFF, 0xFF},
		{0x00, 0xFF, 0x00, 0xFF},
		{0x01, 0x02, 0x03, 0x04, 0x05},
	}

	for _, k := range binaryKeys {
		bf.Add(k)
	}

	for _, k := range binaryKeys {
		if !bf.MayContain(k) {
			t.Errorf("expected binary key %x to be found", k)
		}
	}
}

// TestBloomFilter_SimilarKeys verifies that keys differing by a single byte
// are distinguished properly (within probabilistic bounds).
func TestBloomFilter_SimilarKeys(t *testing.T) {
	bf := NewBloomFilter(100, bitsPerKey)

	bf.Add([]byte("user:1000"))

	if !bf.MayContain([]byte("user:1000")) {
		t.Errorf("expected user:1000 to be found")
	}

	// These similar keys should (with high probability) not match
	misses := 0
	similarKeys := []string{"user:1001", "user:1002", "user:0999", "user:1000x", "xuser:1000"}
	for _, sk := range similarKeys {
		if !bf.MayContain([]byte(sk)) {
			misses++
		}
	}

	// At least some of the similar keys should not be false positives
	if misses == 0 {
		t.Logf("warning: all similar keys were false positives — unusual but possible")
	}
}

// TestBloomFilter_LargeKeyCount verifies the filter works correctly with a
// large number of keys and maintains acceptable FPR.
func TestBloomFilter_LargeKeyCount(t *testing.T) {
	numKeys := 100000
	bf := NewBloomFilter(numKeys, bitsPerKey)

	for i := 0; i < numKeys; i++ {
		bf.Add([]byte(fmt.Sprintf("large-set-key-%d", i)))
	}

	// Verify no false negatives (spot check 1000 keys)
	for i := 0; i < numKeys; i += 100 {
		k := []byte(fmt.Sprintf("large-set-key-%d", i))
		if !bf.MayContain(k) {
			t.Fatalf("false negative at key index %d", i)
		}
	}

	// Check FPR with absent keys
	falsePositives := 0
	numQueries := 50000
	for i := 0; i < numQueries; i++ {
		if bf.MayContain([]byte(fmt.Sprintf("absent-large-%d", i))) {
			falsePositives++
		}
	}

	fpr := float64(falsePositives) / float64(numQueries)
	t.Logf("large key count FPR: %.4f%% (%d / %d)", fpr*100, falsePositives, numQueries)

	if fpr > 0.02 {
		t.Errorf("FPR too high for large key set: got %.4f, want <= 0.02", fpr)
	}
}

// TestBloomFilter_HighBitsPerKey verifies that very high bits-per-key values
// produce a near-zero false positive rate and k is capped at 30.
func TestBloomFilter_HighBitsPerKey(t *testing.T) {
	bf := NewBloomFilter(100, 50) // Very generous

	if bf.NumHashes() != 30 {
		t.Errorf("expected numHashes capped at 30, got %d", bf.NumHashes())
	}

	for i := 0; i < 100; i++ {
		bf.Add([]byte(fmt.Sprintf("key-%d", i)))
	}

	falsePositives := 0
	for i := 0; i < 10000; i++ {
		if bf.MayContain([]byte(fmt.Sprintf("miss-%d", i))) {
			falsePositives++
		}
	}

	fpr := float64(falsePositives) / 10000.0
	t.Logf("high bpk=50 FPR: %.6f%%", fpr*100)

	if fpr > 0.001 {
		t.Errorf("FPR too high for 50 bits/key: got %.6f, want <= 0.001", fpr)
	}
}

// TestBloomFilter_NilReceiver verifies that calling Add and MayContain on a
// nil *BloomFilter does not panic. MayContain should return false (fail closed).
func TestBloomFilter_NilReceiver(t *testing.T) {
	var bf *BloomFilter // nil

	// Must not panic
	bf.Add([]byte("key"))

	if bf.MayContain([]byte("key")) {
		t.Errorf("MayContain on nil receiver should return false")
	}
}

// TestBloomFilter_ZeroValueStruct verifies that a zero-value BloomFilter
// (not created via NewBloomFilter) fails closed: MayContain returns false
// for any key, and Add is a safe no-op.
func TestBloomFilter_ZeroValueStruct(t *testing.T) {
	var bf BloomFilter // zero value: bits=nil, numHashes=0, totalBitCount=0

	// Add must not panic on a zero-value filter
	bf.Add([]byte("key"))

	// MayContain must return false, not true (the pre-guard bug)
	if bf.MayContain([]byte("key")) {
		t.Errorf("MayContain on zero-value struct should return false, got true")
	}
	if bf.MayContain([]byte("other")) {
		t.Errorf("MayContain on zero-value struct should return false for any key")
	}
}

// TestBloomFilter_ZeroNumHashes verifies that a filter with allocated bits
// but zero hash functions fails closed rather than vacuously returning true.
func TestBloomFilter_ZeroNumHashes(t *testing.T) {
	bf := &BloomFilter{
		bits:          make([]byte, 8),
		numHashes:     0,
		totalBitCount: 64,
	}

	// Add should be a no-op (no hashes to compute)
	bf.Add([]byte("key"))

	// MayContain must return false, not fall through to true
	if bf.MayContain([]byte("key")) {
		t.Errorf("MayContain with zero numHashes should return false")
	}
}

// TestBloomFilter_EmptyBitsSlice verifies that a filter with numHashes > 0
// but an empty bits slice does not panic on Add or MayContain.
func TestBloomFilter_EmptyBitsSlice(t *testing.T) {
	bf := &BloomFilter{
		bits:          []byte{},
		numHashes:     7,
		totalBitCount: 64,
	}

	// Must not panic
	bf.Add([]byte("key"))

	if bf.MayContain([]byte("key")) {
		t.Errorf("MayContain with empty bits should return false")
	}
}

// TestBloomFilter_NewFromBytes_ClampHashes verifies that reconstructing a
// BloomFilter from bytes clamps the numHashes parameter to a safe range [1, 30].
func TestBloomFilter_NewFromBytes_ClampHashes(t *testing.T) {
	data := []byte{0x00, 0x01, 0x02} // Arbitrary data

	tests := []struct {
		name      string
		numHashes uint8
		expected  uint8
	}{
		{"Zero is clamped to 1", 0, 1},
		{"Normal value is unchanged", 10, 10},
		{"Max valid is unchanged", 30, 30},
		{"Over max is clamped to 30", 31, 30},
		{"Huge value is clamped to 30", 255, 30},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bf := NewBloomFilterFromBytes(data, tc.numHashes)
			if bf.numHashes != tc.expected {
				t.Errorf("expected numHashes %d, got %d", tc.expected, bf.numHashes)
			}
		})
	}
}
