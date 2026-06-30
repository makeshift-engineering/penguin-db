package encoding

import (
	"bytes"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

// TestEncodeInt32_Roundtrip verifies that encoding and decoding int32 values
// preserves the original value across the full signed range.
func TestEncodeInt32_Roundtrip(t *testing.T) {
	cases := []struct {
		name string
		val  int32
	}{
		{"zero", 0},
		{"positive_one", 1},
		{"negative_one", -1},
		{"max", math.MaxInt32},
		{"min", math.MinInt32},
		{"arbitrary_positive", 123456},
		{"arbitrary_negative", -98765},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			enc := EncodeInt32(tc.val)
			if len(enc) != 4 {
				t.Fatalf("EncodeInt32(%d): expected 4 bytes, got %d", tc.val, len(enc))
			}
			got := DecodeInt32(enc)
			if got != tc.val {
				t.Errorf("roundtrip: got %d, want %d", got, tc.val)
			}
		})
	}
}

// TestEncodeInt32_SortOrder verifies that the lexicographic byte order of
// encoded int32s matches the natural numeric order.
func TestEncodeInt32_SortOrder(t *testing.T) {
	ordered := []int32{math.MinInt32, -1000, -1, 0, 1, 1000, math.MaxInt32}
	for i := 0; i < len(ordered)-1; i++ {
		a := EncodeInt32(ordered[i])
		b := EncodeInt32(ordered[i+1])
		if bytes.Compare(a, b) >= 0 {
			t.Errorf("sort order violation: encode(%d) >= encode(%d)", ordered[i], ordered[i+1])
		}
	}
}

// TestDecodeInt32_ShortBuffer verifies that DecodeInt32 returns zero when
// given a buffer that is too short to contain a valid encoded int32.
func TestDecodeInt32_ShortBuffer(t *testing.T) {
	for _, b := range [][]byte{nil, {}, {0x01}, {0x01, 0x02}, {0x01, 0x02, 0x03}} {
		got := DecodeInt32(b)
		if got != 0 {
			t.Errorf("DecodeInt32(%v): expected 0, got %d", b, got)
		}
	}
}

// TestEncodeInt64_Roundtrip verifies that encoding and decoding int64 values
// preserves the original value across the full signed range.
func TestEncodeInt64_Roundtrip(t *testing.T) {
	cases := []struct {
		name string
		val  int64
	}{
		{"zero", 0},
		{"positive_one", 1},
		{"negative_one", -1},
		{"max", math.MaxInt64},
		{"min", math.MinInt64},
		{"large_positive", 1_000_000_000_000},
		{"large_negative", -1_000_000_000_000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			enc := EncodeInt64(tc.val)
			if len(enc) != 8 {
				t.Fatalf("EncodeInt64(%d): expected 8 bytes, got %d", tc.val, len(enc))
			}
			got := DecodeInt64(enc)
			if got != tc.val {
				t.Errorf("roundtrip: got %d, want %d", got, tc.val)
			}
		})
	}
}

// TestEncodeInt64_SortOrder verifies that lexicographic byte order of encoded
// int64s matches natural numeric order.
func TestEncodeInt64_SortOrder(t *testing.T) {
	ordered := []int64{math.MinInt64, -1_000_000, -1, 0, 1, 1_000_000, math.MaxInt64}
	for i := 0; i < len(ordered)-1; i++ {
		a := EncodeInt64(ordered[i])
		b := EncodeInt64(ordered[i+1])
		if bytes.Compare(a, b) >= 0 {
			t.Errorf("sort order violation: encode(%d) >= encode(%d)", ordered[i], ordered[i+1])
		}
	}
}

// TestDecodeInt64_ShortBuffer verifies that DecodeInt64 returns zero when the
// input buffer has fewer than 8 bytes.
func TestDecodeInt64_ShortBuffer(t *testing.T) {
	for _, b := range [][]byte{nil, {}, make([]byte, 7)} {
		got := DecodeInt64(b)
		if got != 0 {
			t.Errorf("DecodeInt64(len=%d): expected 0, got %d", len(b), got)
		}
	}
}

// TestEncodeFloat64_Roundtrip verifies that encoding and decoding float64
// values preserves the original value for all finite values and infinities.
func TestEncodeFloat64_Roundtrip(t *testing.T) {
	cases := []struct {
		name string
		val  float64
	}{
		{"zero", 0.0},
		{"negative_zero", math.Copysign(0, -1)},
		{"one", 1.0},
		{"negative_one", -1.0},
		{"pi", math.Pi},
		{"neg_pi", -math.Pi},
		{"max", math.MaxFloat64},
		{"neg_max", -math.MaxFloat64},
		{"smallest_positive", math.SmallestNonzeroFloat64},
		{"neg_smallest", -math.SmallestNonzeroFloat64},
		{"pos_inf", math.Inf(1)},
		{"neg_inf", math.Inf(-1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			enc, err := EncodeFloat64(tc.val)
			if err != nil {
				t.Fatalf("EncodeFloat64(%v): unexpected error: %v", tc.val, err)
			}
			if len(enc) != 8 {
				t.Fatalf("expected 8 bytes, got %d", len(enc))
			}
			got := DecodeFloat64(enc)
			if got != tc.val {
				t.Errorf("roundtrip: got %v, want %v", got, tc.val)
			}
		})
	}
}

// TestEncodeFloat64_NaN verifies that encoding a NaN float64 returns the
// ErrNaNNotAllowed sentinel error.
func TestEncodeFloat64_NaN(t *testing.T) {
	_, err := EncodeFloat64(math.NaN())
	if !errors.Is(err, ErrNaNNotAllowed) {
		t.Errorf("expected ErrNaNNotAllowed, got %v", err)
	}
}

// TestEncodeFloat64_SortOrder verifies that encoded float64 bytes sort in the
// same order as the numeric float64 values. This covers the critical cross-sign
// boundary where standard IEEE 754 bytes would fail.
func TestEncodeFloat64_SortOrder(t *testing.T) {
	ordered := []float64{
		math.Inf(-1),
		-math.MaxFloat64,
		-1.0,
		-math.SmallestNonzeroFloat64,
		math.Copysign(0, -1), // -0
		0.0,
		math.SmallestNonzeroFloat64,
		1.0,
		math.MaxFloat64,
		math.Inf(1),
	}
	for i := 0; i < len(ordered)-1; i++ {
		a, _ := EncodeFloat64(ordered[i])
		b, _ := EncodeFloat64(ordered[i+1])
		if bytes.Compare(a, b) >= 0 {
			t.Errorf("sort order violation: encode(%v) >= encode(%v)", ordered[i], ordered[i+1])
		}
	}
}

// TestDecodeFloat64_ShortBuffer verifies that DecodeFloat64 returns 0 when the
// input buffer is shorter than 8 bytes.
func TestDecodeFloat64_ShortBuffer(t *testing.T) {
	for _, b := range [][]byte{nil, {}, make([]byte, 7)} {
		got := DecodeFloat64(b)
		if got != 0 {
			t.Errorf("DecodeFloat64(len=%d): expected 0, got %v", len(b), got)
		}
	}
}

// TestEncodeString_Roundtrip verifies that encoding and decoding string values
// preserves the original string, including the empty string.
func TestEncodeString_Roundtrip(t *testing.T) {
	cases := []string{"", "a", "hello", "world123", "emoji 🐧", "a\nb\tc"}
	for _, v := range cases {
		enc, err := EncodeString(v)
		if err != nil {
			t.Fatalf("EncodeString(%q): unexpected error: %v", v, err)
		}
		// Encoded form must end with a NUL terminator.
		if enc[len(enc)-1] != 0x00 {
			t.Errorf("EncodeString(%q): missing NUL terminator", v)
		}
		got, err := DecodeString(enc)
		if err != nil {
			t.Fatalf("DecodeString(%q): unexpected error: %v", v, err)
		}
		if got != v {
			t.Errorf("roundtrip: got %q, want %q", got, v)
		}
	}
}

// TestEncodeString_NulInString verifies that EncodeString rejects strings
// containing an interior NUL byte with ErrNulInString.
func TestEncodeString_NulInString(t *testing.T) {
	_, err := EncodeString("hello\x00world")
	if !errors.Is(err, ErrNulInString) {
		t.Errorf("expected ErrNulInString, got %v", err)
	}
	// NUL at position 0.
	_, err = EncodeString("\x00start")
	if !errors.Is(err, ErrNulInString) {
		t.Errorf("expected ErrNulInString for leading NUL, got %v", err)
	}
}

// TestDecodeString_NoTerminator verifies that DecodeString returns
// ErrKeyTooShort when the input does not contain a NUL terminator.
func TestDecodeString_NoTerminator(t *testing.T) {
	_, err := DecodeString([]byte("hello"))
	if !errors.Is(err, ErrKeyTooShort) {
		t.Errorf("expected ErrKeyTooShort, got %v", err)
	}
}

// TestEncodeString_SortOrder verifies that encoded strings sort
// lexicographically by byte value.
func TestEncodeString_SortOrder(t *testing.T) {
	ordered := []string{"apple", "banana", "cherry", "date"}
	for i := 0; i < len(ordered)-1; i++ {
		a, _ := EncodeString(ordered[i])
		b, _ := EncodeString(ordered[i+1])
		if bytes.Compare(a, b) >= 0 {
			t.Errorf("sort order violation: encode(%q) >= encode(%q)", ordered[i], ordered[i+1])
		}
	}
}

// TestEncodeBool_Roundtrip verifies that encoding and decoding booleans
// preserves the original value.
func TestEncodeBool_Roundtrip(t *testing.T) {
	for _, v := range []bool{false, true} {
		enc := EncodeBool(v)
		if len(enc) != 1 {
			t.Fatalf("EncodeBool(%v): expected 1 byte, got %d", v, len(enc))
		}
		got := DecodeBool(enc)
		if got != v {
			t.Errorf("roundtrip: got %v, want %v", got, v)
		}
	}
}

// TestEncodeBool_SortOrder verifies that false sorts before true in encoded
// byte representation.
func TestEncodeBool_SortOrder(t *testing.T) {
	encF := EncodeBool(false)
	encT := EncodeBool(true)
	if bytes.Compare(encF, encT) >= 0 {
		t.Errorf("sort order violation: encode(false) >= encode(true)")
	}
}

// TestDecodeBool_EmptySlice verifies that DecodeBool returns false when given
// an empty or nil slice.
func TestDecodeBool_EmptySlice(t *testing.T) {
	if DecodeBool(nil) != false {
		t.Error("DecodeBool(nil): expected false")
	}
	if DecodeBool([]byte{}) != false {
		t.Error("DecodeBool([]byte{}): expected false")
	}
}

// TestDecodeBool_NonCanonical verifies that only 0x01 decodes to true; any
// other non-zero byte decodes to false.
func TestDecodeBool_NonCanonical(t *testing.T) {
	if DecodeBool([]byte{0x02}) != false {
		t.Error("DecodeBool(0x02): expected false for non-canonical byte")
	}
	if DecodeBool([]byte{0xFF}) != false {
		t.Error("DecodeBool(0xFF): expected false for non-canonical byte")
	}
}

// TestEncodeTimestamp_Roundtrip verifies that encoding and decoding timestamps
// preserves the original time at nanosecond precision.
func TestEncodeTimestamp_Roundtrip(t *testing.T) {
	cases := []struct {
		name string
		ts   time.Time
	}{
		{"epoch", time.Unix(0, 0).UTC()},
		{"before_epoch", time.Unix(-1000, 0).UTC()},
		{"recent", time.Date(2025, 6, 15, 12, 30, 0, 0, time.UTC)},
		{"with_nanos", time.Unix(1000, 123456789).UTC()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			enc := EncodeTimestamp(tc.ts)
			if len(enc) != 8 {
				t.Fatalf("expected 8 bytes, got %d", len(enc))
			}
			got := DecodeTimestamp(enc)
			if !got.Equal(tc.ts) {
				t.Errorf("roundtrip: got %v, want %v", got, tc.ts)
			}
		})
	}
}

// TestEncodeTimestamp_SortOrder verifies that timestamps sort chronologically
// when compared lexicographically as encoded byte slices.
func TestEncodeTimestamp_SortOrder(t *testing.T) {
	ordered := []time.Time{
		time.Unix(-1000, 0).UTC(),
		time.Unix(0, 0).UTC(),
		time.Unix(1000, 0).UTC(),
		time.Unix(2000, 500).UTC(),
	}
	for i := 0; i < len(ordered)-1; i++ {
		a := EncodeTimestamp(ordered[i])
		b := EncodeTimestamp(ordered[i+1])
		if bytes.Compare(a, b) >= 0 {
			t.Errorf("sort order violation: encode(%v) >= encode(%v)", ordered[i], ordered[i+1])
		}
	}
}

// TestEncodePK_AllTypes verifies that a composite primary key using every
// supported column type roundtrips correctly through EncodePK / DecodePK.
func TestEncodePK_AllTypes(t *testing.T) {
	cols := []ast.DataTypeKind{
		ast.TypeInt,
		ast.TypeBigInt,
		ast.TypeVarchar,
		ast.TypeText,
		ast.TypeBoolean,
		ast.TypeTimestamp,
	}
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	vals := []any{int32(42), int64(-999), "hello", "world", true, ts}

	encoded, err := EncodePK(cols, vals)
	if err != nil {
		t.Fatalf("EncodePK: unexpected error: %v", err)
	}
	decoded, err := DecodePK(cols, encoded)
	if err != nil {
		t.Fatalf("DecodePK: unexpected error: %v", err)
	}
	if len(decoded) != len(vals) {
		t.Fatalf("DecodePK: got %d values, want %d", len(decoded), len(vals))
	}
	if decoded[0].(int32) != int32(42) {
		t.Errorf("int32: got %v", decoded[0])
	}
	if decoded[1].(int64) != int64(-999) {
		t.Errorf("int64: got %v", decoded[1])
	}
	if decoded[2].(string) != "hello" {
		t.Errorf("varchar: got %v", decoded[2])
	}
	if decoded[3].(string) != "world" {
		t.Errorf("text: got %v", decoded[3])
	}
	if decoded[4].(bool) != true {
		t.Errorf("bool: got %v", decoded[4])
	}
	if !decoded[5].(time.Time).Equal(ts) {
		t.Errorf("timestamp: got %v", decoded[5])
	}
}

// TestEncodePK_SingleColumn verifies EncodePK/DecodePK for each individual
// column type in isolation.
func TestEncodePK_SingleColumn(t *testing.T) {
	ts := time.Unix(1234567890, 0).UTC()
	cases := []struct {
		name string
		col  ast.DataTypeKind
		val  any
	}{
		{"int", ast.TypeInt, int32(7)},
		{"bigint", ast.TypeBigInt, int64(42)},
		{"varchar", ast.TypeVarchar, "test"},
		{"text", ast.TypeText, "text_value"},
		{"bool_true", ast.TypeBoolean, true},
		{"bool_false", ast.TypeBoolean, false},
		{"timestamp", ast.TypeTimestamp, ts},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cols := []ast.DataTypeKind{tc.col}
			vals := []any{tc.val}
			enc, err := EncodePK(cols, vals)
			if err != nil {
				t.Fatalf("EncodePK: %v", err)
			}
			dec, err := DecodePK(cols, enc)
			if err != nil {
				t.Fatalf("DecodePK: %v", err)
			}
			if len(dec) != 1 {
				t.Fatalf("expected 1 value, got %d", len(dec))
			}
			// Compare based on type.
			switch want := tc.val.(type) {
			case time.Time:
				if !dec[0].(time.Time).Equal(want) {
					t.Errorf("got %v, want %v", dec[0], want)
				}
			default:
				if dec[0] != want {
					t.Errorf("got %v, want %v", dec[0], want)
				}
			}
		})
	}
}

// TestEncodePK_LenMismatch verifies that EncodePK returns ErrInvalidPK when
// the number of column types does not match the number of values.
func TestEncodePK_LenMismatch(t *testing.T) {
	_, err := EncodePK(
		[]ast.DataTypeKind{ast.TypeInt, ast.TypeVarchar},
		[]any{int32(1)},
	)
	if !errors.Is(err, ErrInvalidPK) {
		t.Errorf("expected ErrInvalidPK for length mismatch, got %v", err)
	}
}

// TestEncodePK_TypeMismatch verifies that EncodePK returns ErrInvalidPK when a
// value does not match the expected Go type for the declared column type.
func TestEncodePK_TypeMismatch(t *testing.T) {
	cases := []struct {
		name string
		col  ast.DataTypeKind
		val  any
	}{
		{"int_wants_int32", ast.TypeInt, int64(1)},            // int64 instead of int32
		{"bigint_wants_int64", ast.TypeBigInt, int32(1)},      // int32 instead of int64
		{"varchar_wants_string", ast.TypeVarchar, 123},        // int instead of string
		{"text_wants_string", ast.TypeText, 123},              // int instead of string
		{"bool_wants_bool", ast.TypeBoolean, 1},               // int instead of bool
		{"timestamp_wants_time", ast.TypeTimestamp, int64(0)}, // int64 instead of time.Time
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := EncodePK([]ast.DataTypeKind{tc.col}, []any{tc.val})
			if !errors.Is(err, ErrInvalidPK) {
				t.Errorf("expected ErrInvalidPK, got %v", err)
			}
		})
	}
}

// TestEncodePK_UnsupportedType verifies that EncodePK returns ErrInvalidPK
// when an unrecognized DataTypeKind is used.
func TestEncodePK_UnsupportedType(t *testing.T) {
	_, err := EncodePK(
		[]ast.DataTypeKind{ast.DataTypeKind(999)},
		[]any{"whatever"},
	)
	if !errors.Is(err, ErrInvalidPK) {
		t.Errorf("expected ErrInvalidPK for unsupported type, got %v", err)
	}
}

// TestEncodePK_NulStringError verifies that EncodePK propagates the
// ErrNulInString error when a string column value contains NUL.
func TestEncodePK_NulStringError(t *testing.T) {
	_, err := EncodePK(
		[]ast.DataTypeKind{ast.TypeVarchar},
		[]any{"hello\x00world"},
	)
	if !errors.Is(err, ErrNulInString) {
		t.Errorf("expected ErrNulInString, got %v", err)
	}
}

// TestEncodePK_EmptySlices verifies that empty column/value slices produce
// an empty encoded key that decodes back to an empty slice.
func TestEncodePK_EmptySlices(t *testing.T) {
	enc, err := EncodePK(nil, nil)
	if err != nil {
		t.Fatalf("EncodePK(nil, nil): %v", err)
	}
	if len(enc) != 0 {
		t.Fatalf("expected empty output, got %d bytes", len(enc))
	}
	dec, err := DecodePK(nil, enc)
	if err != nil {
		t.Fatalf("DecodePK(nil, nil): %v", err)
	}
	if len(dec) != 0 {
		t.Fatalf("expected 0 decoded values, got %d", len(dec))
	}
}

// TestDecodePK_TruncatedKey verifies that DecodePK returns ErrKeyTooShort when
// the encoded key is truncated for each column type.
func TestDecodePK_TruncatedKey(t *testing.T) {
	cases := []struct {
		name string
		col  ast.DataTypeKind
		key  []byte
	}{
		{"int_too_short", ast.TypeInt, []byte{0x80, 0x00}},
		{"bigint_too_short", ast.TypeBigInt, []byte{0x80, 0x00, 0x00, 0x00}},
		{"bool_empty", ast.TypeBoolean, []byte{}},
		{"timestamp_short", ast.TypeTimestamp, make([]byte, 5)},
		{"varchar_no_terminator", ast.TypeVarchar, []byte("hello")},
		{"text_no_terminator", ast.TypeText, []byte("hello")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodePK([]ast.DataTypeKind{tc.col}, tc.key)
			if !errors.Is(err, ErrKeyTooShort) {
				t.Errorf("expected ErrKeyTooShort, got %v", err)
			}
		})
	}
}

// TestDecodePK_TrailingBytes verifies that DecodePK returns ErrInvalidPK when
// there are unconsumed trailing bytes after decoding all columns.
func TestDecodePK_TrailingBytes(t *testing.T) {
	cols := []ast.DataTypeKind{ast.TypeInt}
	vals := []any{int32(1)}
	enc, err := EncodePK(cols, vals)
	if err != nil {
		t.Fatalf("EncodePK: %v", err)
	}
	// Append extra byte to simulate trailing data.
	enc = append(enc, 0xFF)
	_, err = DecodePK(cols, enc)
	if !errors.Is(err, ErrInvalidPK) {
		t.Errorf("expected ErrInvalidPK for trailing bytes, got %v", err)
	}
}

// TestDecodePK_UnsupportedType verifies that DecodePK returns ErrInvalidPK
// when encountering an unrecognised column type.
func TestDecodePK_UnsupportedType(t *testing.T) {
	_, err := DecodePK(
		[]ast.DataTypeKind{ast.DataTypeKind(999)},
		[]byte{0x80, 0x00, 0x00, 0x00},
	)
	if !errors.Is(err, ErrInvalidPK) {
		t.Errorf("expected ErrInvalidPK for unsupported type, got %v", err)
	}
}

// TestEncodePK_SortOrder verifies that composite primary keys using a mix of
// int and string columns sort correctly through lexicographic byte comparison.
func TestEncodePK_SortOrder(t *testing.T) {
	cols := []ast.DataTypeKind{ast.TypeInt, ast.TypeVarchar}

	// Row A: (1, "alpha")  should sort before Row B: (1, "beta")
	a, err := EncodePK(cols, []any{int32(1), "alpha"})
	if err != nil {
		t.Fatalf("EncodePK row A: %v", err)
	}
	b, err := EncodePK(cols, []any{int32(1), "beta"})
	if err != nil {
		t.Fatalf("EncodePK row B: %v", err)
	}
	if bytes.Compare(a, b) >= 0 {
		t.Error("sort order violation: (1, alpha) >= (1, beta)")
	}

	// Row C: (0, "z") should sort before Row D: (1, "a")
	c, err := EncodePK(cols, []any{int32(0), "z"})
	if err != nil {
		t.Fatalf("EncodePK row C: %v", err)
	}
	d, err := EncodePK(cols, []any{int32(1), "a"})
	if err != nil {
		t.Fatalf("EncodePK row D: %v", err)
	}
	if bytes.Compare(c, d) >= 0 {
		t.Error("sort order violation: (0, z) >= (1, a)")
	}
}

// TestDecodePK_MultipleVarcharColumns verifies that decoding works correctly
// when multiple NUL-terminated string columns are adjacent in the encoded key.
func TestDecodePK_MultipleVarcharColumns(t *testing.T) {
	cols := []ast.DataTypeKind{ast.TypeVarchar, ast.TypeText, ast.TypeVarchar}
	vals := []any{"first", "second", "third"}

	enc, err := EncodePK(cols, vals)
	if err != nil {
		t.Fatalf("EncodePK: %v", err)
	}
	dec, err := DecodePK(cols, enc)
	if err != nil {
		t.Fatalf("DecodePK: %v", err)
	}
	for i, want := range vals {
		if dec[i].(string) != want.(string) {
			t.Errorf("column %d: got %q, want %q", i, dec[i], want)
		}
	}
}

// TestDecodePK_EmptyString verifies that an empty string column roundtrips
// correctly through the composite PK encoding.
func TestDecodePK_EmptyString(t *testing.T) {
	cols := []ast.DataTypeKind{ast.TypeVarchar, ast.TypeInt}
	vals := []any{"", int32(99)}

	enc, err := EncodePK(cols, vals)
	if err != nil {
		t.Fatalf("EncodePK: %v", err)
	}
	dec, err := DecodePK(cols, enc)
	if err != nil {
		t.Fatalf("DecodePK: %v", err)
	}
	if dec[0].(string) != "" {
		t.Errorf("empty string: got %q", dec[0])
	}
	if dec[1].(int32) != int32(99) {
		t.Errorf("int32: got %v", dec[1])
	}
}
