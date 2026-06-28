package encoding

import (
	"bytes"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

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

func TestEncodeDecodeRowKey(t *testing.T) {
	db := "mydb"
	table := "users"
	pk := []byte{0x01, 0x02, 0x03}

	rowKey := EncodeRowKey(db, table, pk)

	dDB, dTable, dPK, err := DecodeParts(rowKey)
	if err != nil {
		t.Fatalf("DecodeParts failed: %v", err)
	}

	if dDB != db || dTable != table || !bytes.Equal(dPK, pk) {
		t.Fatalf("DecodeParts mismatch: got %s, %s, %v, want %s, %s, %v", dDB, dTable, dPK, db, table, pk)
	}
}
