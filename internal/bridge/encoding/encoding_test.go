package encoding

import (
	"bytes"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

// TestEncodeDecodeInt32 validates the Int32 sortable encoding by verifying
// roundtrip correctness across boundary values (MinInt32, -1, 0, 1, MaxInt32)
// and ensuring the encoded byte order matches the natural numeric order.
func TestEncodeDecodeInt32(t *testing.T) {
	tests := []int32{math.MinInt32, -1, 0, 1, math.MaxInt32}
	for _, v := range tests {
		encoded := EncodeInt32(v)
		decoded := DecodeInt32(encoded)
		if v != decoded {
			t.Errorf("Int32 roundtrip failed: got %d, want %d", decoded, v)
		}
	}

	// Test order
	for i := 0; i < len(tests)-1; i++ {
		enc1 := EncodeInt32(tests[i])
		enc2 := EncodeInt32(tests[i+1])
		if bytes.Compare(enc1, enc2) >= 0 {
			t.Errorf("Int32 ordering failed: %d >= %d", tests[i], tests[i+1])
		}
	}
}

// TestEncodeDecodeInt64 validates the Int64 sortable encoding by verifying
// roundtrip correctness across boundary values (MinInt64, -1, 0, 1, MaxInt64)
// and ensuring the encoded byte order matches the natural numeric order.
func TestEncodeDecodeInt64(t *testing.T) {
	tests := []int64{math.MinInt64, -1, 0, 1, math.MaxInt64}
	for _, v := range tests {
		encoded := EncodeInt64(v)
		decoded := DecodeInt64(encoded)
		if v != decoded {
			t.Errorf("Int64 roundtrip failed: got %d, want %d", decoded, v)
		}
	}

	// Test order
	for i := 0; i < len(tests)-1; i++ {
		enc1 := EncodeInt64(tests[i])
		enc2 := EncodeInt64(tests[i+1])
		if bytes.Compare(enc1, enc2) >= 0 {
			t.Errorf("Int64 ordering failed: %d >= %d", tests[i], tests[i+1])
		}
	}
}

// TestEncodeDecodeFloat64 validates the Float64 sortable encoding by:
//  1. Verifying roundtrip correctness across the full float64 domain (infinities,
//     max/min magnitudes, smallest nonzero, positive/negative zero).
//  2. Asserting that NaN values are rejected with ErrNaNNotAllowed.
//  3. Ensuring encoded byte order matches the natural IEEE 754 numeric order,
//     including the critical negative-to-positive cross-sign boundary.
func TestEncodeDecodeFloat64(t *testing.T) {
	tests := []float64{
		math.Inf(-1),
		-math.MaxFloat64,
		-1.0,
		-math.SmallestNonzeroFloat64,
		math.Copysign(0, -1),
		0.0,
		math.SmallestNonzeroFloat64,
		1.0,
		math.MaxFloat64,
		math.Inf(1),
	}

	for _, v := range tests {
		encoded, err := EncodeFloat64(v)
		if err != nil {
			t.Errorf("EncodeFloat64 failed for %v: %v", v, err)
		}
		decoded := DecodeFloat64(encoded)
		if v != decoded {
			t.Errorf("Float64 roundtrip failed: got %v, want %v", decoded, v)
		}
	}

	// Test NaN
	_, err := EncodeFloat64(math.NaN())
	if !errors.Is(err, ErrNaNNotAllowed) {
		t.Errorf("Expected ErrNaNNotAllowed, got %v", err)
	}

	// Test order
	for i := 0; i < len(tests)-1; i++ {
		enc1, _ := EncodeFloat64(tests[i])
		enc2, _ := EncodeFloat64(tests[i+1])
		if bytes.Compare(enc1, enc2) >= 0 {
			t.Errorf("Float64 ordering failed: %v >= %v", tests[i], tests[i+1])
		}
	}
}

// TestEncodeDecodeString validates the NUL-terminated string encoding by:
//  1. Verifying roundtrip correctness for representative strings (empty, plain,
//     alphanumeric, multiline).
//  2. Asserting that strings with interior NUL bytes are rejected with
//     ErrNulInString.
//  3. Ensuring encoded byte order matches natural lexicographic string order.
func TestEncodeDecodeString(t *testing.T) {
	tests := []string{"", "hello", "world123", "a\nb"}
	for _, v := range tests {
		encoded, err := EncodeString(v)
		if err != nil {
			t.Errorf("EncodeString failed: %v", err)
		}
		decoded, err := DecodeString(encoded)
		if err != nil {
			t.Errorf("DecodeString failed: %v", err)
		}
		if v != decoded {
			t.Errorf("String roundtrip failed: got %v, want %v", decoded, v)
		}
	}

	// Test NUL string
	_, err := EncodeString("hello\x00world")
	if !errors.Is(err, ErrNulInString) {
		t.Errorf("Expected ErrNulInString, got %v", err)
	}

	// Test order
	enc1, _ := EncodeString("apple")
	enc2, _ := EncodeString("banana")
	if bytes.Compare(enc1, enc2) >= 0 {
		t.Errorf("String ordering failed: apple >= banana")
	}
}

// TestEncodeDecodeBool validates the boolean encoding by verifying roundtrip
// correctness for both values and ensuring false (0x00) sorts before true (0x01).
func TestEncodeDecodeBool(t *testing.T) {
	encF := EncodeBool(false)
	encT := EncodeBool(true)

	if DecodeBool(encF) != false {
		t.Errorf("DecodeBool(false) failed")
	}
	if DecodeBool(encT) != true {
		t.Errorf("DecodeBool(true) failed")
	}
	if bytes.Compare(encF, encT) >= 0 {
		t.Errorf("Bool ordering failed: false >= true")
	}
}

// TestEncodeDecodeTimestamp validates the timestamp encoding by verifying
// roundtrip correctness for two UTC timestamps and ensuring the chronologically
// earlier timestamp sorts before the later one in encoded byte representation.
func TestEncodeDecodeTimestamp(t *testing.T) {
	t1 := time.Unix(0, 0).UTC()
	t2 := time.Unix(1000, 0).UTC()

	enc1 := EncodeTimestamp(t1)
	enc2 := EncodeTimestamp(t2)

	if !DecodeTimestamp(enc1).Equal(t1) {
		t.Errorf("DecodeTimestamp failed for t1")
	}
	if !DecodeTimestamp(enc2).Equal(t2) {
		t.Errorf("DecodeTimestamp failed for t2")
	}
	if bytes.Compare(enc1, enc2) >= 0 {
		t.Errorf("Timestamp ordering failed: t1 >= t2")
	}
}

// TestCompositePK validates the composite primary key encoding by roundtripping
// a key containing Int, Varchar, and Boolean columns and verifying that each
// decoded column value matches its original input.
func TestCompositePK(t *testing.T) {
	cols := []ast.DataTypeKind{ast.TypeInt, ast.TypeVarchar, ast.TypeBoolean}
	vals := []any{int32(42), "user", true}

	encoded, err := EncodePK(cols, vals)
	if err != nil {
		t.Fatalf("EncodePK failed: %v", err)
	}

	decoded, err := DecodePK(cols, encoded)
	if err != nil {
		t.Fatalf("DecodePK failed: %v", err)
	}

	if len(decoded) != len(vals) {
		t.Fatalf("DecodePK count mismatch: got %d, want %d", len(decoded), len(vals))
	}

	if decoded[0].(int32) != vals[0] || decoded[1].(string) != vals[1] || decoded[2].(bool) != vals[2] {
		t.Fatalf("DecodePK values mismatch: got %v, want %v", decoded, vals)
	}
}

// TestEncodeDecodeRowKey validates the full row key encoding by roundtripping
// a key with a database name, table name, and raw primary key bytes through
// EncodeRowKey and DecodeParts, verifying all three segments are preserved.
func TestEncodeDecodeRowKey(t *testing.T) {
	db := "mydb"
	table := "users"
	pk := []byte{0x01, 0x02, 0x03}

	rowKey, err := EncodeRowKey(db, table, pk)
	if err != nil {
		t.Fatalf("EncodeRowKey failed: %v", err)
	}

	dDB, dTable, dPK, err := DecodeParts(rowKey)
	if err != nil {
		t.Fatalf("DecodeParts failed: %v", err)
	}

	if dDB != db || dTable != table || !bytes.Equal(dPK, pk) {
		t.Fatalf("DecodeParts mismatch: got %s, %s, %v, want %s, %s, %v", dDB, dTable, dPK, db, table, pk)
	}
}

// TestErrNameTooLong validates that all key-encoding functions that accept
// database or table names correctly reject names exceeding the maximum length
// (65535 bytes) with ErrNameTooLong. This covers EncodeScanPrefix,
// EncodeRowKey, EncodeCatalogDBKey, EncodeCatalogTableKey, and
// EncodeCatalogSeqKey for both the database and table name parameters.
func TestErrNameTooLong(t *testing.T) {
	longName := strings.Repeat("x", maxNameLen+1)

	// EncodeScanPrefix — long db name
	_, err := EncodeScanPrefix(longName, "t")
	if !errors.Is(err, ErrNameTooLong) {
		t.Errorf("EncodeScanPrefix(longDB): expected ErrNameTooLong, got %v", err)
	}

	// EncodeScanPrefix — long table name
	_, err = EncodeScanPrefix("db", longName)
	if !errors.Is(err, ErrNameTooLong) {
		t.Errorf("EncodeScanPrefix(longTable): expected ErrNameTooLong, got %v", err)
	}

	// EncodeRowKey propagates
	_, err = EncodeRowKey(longName, "t", []byte{0x01})
	if !errors.Is(err, ErrNameTooLong) {
		t.Errorf("EncodeRowKey(longDB): expected ErrNameTooLong, got %v", err)
	}

	// EncodeCatalogDBKey
	_, err = EncodeCatalogDBKey(longName)
	if !errors.Is(err, ErrNameTooLong) {
		t.Errorf("EncodeCatalogDBKey(longDB): expected ErrNameTooLong, got %v", err)
	}

	// EncodeCatalogTableKey — long db
	_, err = EncodeCatalogTableKey(longName, "t")
	if !errors.Is(err, ErrNameTooLong) {
		t.Errorf("EncodeCatalogTableKey(longDB): expected ErrNameTooLong, got %v", err)
	}

	// EncodeCatalogTableKey — long table
	_, err = EncodeCatalogTableKey("db", longName)
	if !errors.Is(err, ErrNameTooLong) {
		t.Errorf("EncodeCatalogTableKey(longTable): expected ErrNameTooLong, got %v", err)
	}

	// EncodeCatalogSeqKey — long db
	_, err = EncodeCatalogSeqKey(longName, "t")
	if !errors.Is(err, ErrNameTooLong) {
		t.Errorf("EncodeCatalogSeqKey(longDB): expected ErrNameTooLong, got %v", err)
	}

	// EncodeCatalogSeqKey — long table
	_, err = EncodeCatalogSeqKey("db", longName)
	if !errors.Is(err, ErrNameTooLong) {
		t.Errorf("EncodeCatalogSeqKey(longTable): expected ErrNameTooLong, got %v", err)
	}
}

// TestDecodeParts_MalformedKey validates that DecodeParts correctly rejects
// structurally invalid keys by returning the appropriate sentinel error:
//   - ErrMalformedKey for wrong namespace prefix, missing db separator, and
//     missing table separator.
//   - ErrKeyTooShort for empty keys and keys truncated after the namespace byte.
func TestDecodeParts_MalformedKey(t *testing.T) {
	// Wrong namespace prefix
	badNS := []byte{0xFF, 0x00, 0x02, 'a', 'b', 0x00, 0x00, 0x01, 't', 0x00}
	_, _, _, err := DecodeParts(badNS)
	if !errors.Is(err, ErrMalformedKey) {
		t.Errorf("wrong namespace: expected ErrMalformedKey, got %v", err)
	}

	// Missing separator after DB segment — put a non-zero byte where 0x00 should be
	missingSep := []byte{NamespaceUser, 0x00, 0x02, 'd', 'b', 0xFF, 0x00, 0x01, 't', 0x00}
	_, _, _, err = DecodeParts(missingSep)
	if !errors.Is(err, ErrMalformedKey) {
		t.Errorf("missing db separator: expected ErrMalformedKey, got %v", err)
	}

	// Missing separator after Table segment
	missingTableSep := []byte{NamespaceUser, 0x00, 0x02, 'd', 'b', 0x00, 0x00, 0x01, 't', 0xFF}
	_, _, _, err = DecodeParts(missingTableSep)
	if !errors.Is(err, ErrMalformedKey) {
		t.Errorf("missing table separator: expected ErrMalformedKey, got %v", err)
	}

	// Empty key
	_, _, _, err = DecodeParts([]byte{})
	if !errors.Is(err, ErrKeyTooShort) {
		t.Errorf("empty key: expected ErrKeyTooShort, got %v", err)
	}

	// Truncated after namespace byte
	_, _, _, err = DecodeParts([]byte{NamespaceUser})
	if !errors.Is(err, ErrKeyTooShort) {
		t.Errorf("truncated after namespace: expected ErrKeyTooShort, got %v", err)
	}
}
