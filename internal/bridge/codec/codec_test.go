package codec

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

// encodeRow is a shortcut that creates a Row with codecVersion 0x01
// and the supplied column values, then encodes it.
func encodeRow(t *testing.T, values ...ColumnValue) []byte {
	t.Helper()
	row := &Row{CodecVersion: 0x01, Values: values}
	data, err := Encode(row)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	return data
}

// TestRoundTripInt verifies that INT column values survive an Encode + Decode cycle.
func TestRoundTripInt(t *testing.T) {
	cases := []int32{0, 1, -1, math.MaxInt32, math.MinInt32, 42, -42}
	for _, v := range cases {
		data := encodeRow(t, IntValue(v))
		row, err := Decode(data)
		if err != nil {
			t.Fatalf("Decode failed for %d: %v", v, err)
		}
		got, err := row.Values[0].AsInt()
		if err != nil {
			t.Fatalf("AsInt failed for %d: %v", v, err)
		}
		if got != v {
			t.Errorf("INT round-trip: got %d, want %d", got, v)
		}
	}
}

// TestRoundTripBigInt verifies that BIGINT column values survive an Encode + Decode cycle.
func TestRoundTripBigInt(t *testing.T) {
	cases := []int64{0, 1, -1, math.MaxInt64, math.MinInt64, 1e12}
	for _, v := range cases {
		data := encodeRow(t, BigIntValue(v))
		row, err := Decode(data)
		if err != nil {
			t.Fatalf("Decode failed for %d: %v", v, err)
		}
		got, err := row.Values[0].AsBigInt()
		if err != nil {
			t.Fatalf("AsBigInt failed for %d: %v", v, err)
		}
		if got != v {
			t.Errorf("BIGINT round-trip: got %d, want %d", got, v)
		}
	}
}

// TestRoundTripVarchar verifies VARCHAR values through Encode + Decode.
func TestRoundTripVarchar(t *testing.T) {
	cases := []string{"", "hello", "café", "日本語", "with spaces", "0123456789"}
	for _, v := range cases {
		data := encodeRow(t, VarcharValue(v))
		row, err := Decode(data)
		if err != nil {
			t.Fatalf("Decode failed for %q: %v", v, err)
		}
		got, err := row.Values[0].AsString()
		if err != nil {
			t.Fatalf("AsString failed for %q: %v", v, err)
		}
		if got != v {
			t.Errorf("VARCHAR round-trip: got %q, want %q", got, v)
		}
	}
}

// TestRoundTripText verifies TEXT values through Encode + Decode.
func TestRoundTripText(t *testing.T) {
	cases := []string{"", "hello world", "long " + string(make([]byte, 1000))}
	for _, v := range cases {
		data := encodeRow(t, TextValue(v))
		row, err := Decode(data)
		if err != nil {
			t.Fatalf("Decode failed for text len=%d: %v", len(v), err)
		}
		got, err := row.Values[0].AsString()
		if err != nil {
			t.Fatalf("AsString failed for text len=%d: %v", len(v), err)
		}
		if got != v {
			t.Errorf("TEXT round-trip: lengths differ: got %d, want %d", len(got), len(v))
		}
	}
}

// TestRoundTripBoolean verifies BOOLEAN values through Encode + Decode.
func TestRoundTripBoolean(t *testing.T) {
	for _, v := range []bool{true, false} {
		data := encodeRow(t, BoolValue(v))
		row, err := Decode(data)
		if err != nil {
			t.Fatalf("Decode failed for %v: %v", v, err)
		}
		got, err := row.Values[0].AsBool()
		if err != nil {
			t.Fatalf("AsBool failed for %v: %v", v, err)
		}
		if got != v {
			t.Errorf("BOOLEAN round-trip: got %v, want %v", got, v)
		}
	}
}

// TestRoundTripTimestamp verifies TIMESTAMP values through Encode + Decode.
func TestRoundTripTimestamp(t *testing.T) {
	cases := []time.Time{
		time.Unix(0, 0).UTC(),
		time.Date(2024, 6, 15, 12, 30, 0, 0, time.UTC),
		time.Date(1969, 7, 20, 20, 17, 0, 0, time.UTC), // before epoch
		time.Date(2093, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	for _, v := range cases {
		data := encodeRow(t, TimestampValue(v))
		row, err := Decode(data)
		if err != nil {
			t.Fatalf("Decode failed for %v: %v", v, err)
		}
		got, err := row.Values[0].AsTimestamp()
		if err != nil {
			t.Fatalf("AsTimestamp failed for %v: %v", v, err)
		}
		if !got.Equal(v) {
			t.Errorf("TIMESTAMP round-trip: got %v, want %v", got, v)
		}
	}
}

// TestRoundTripNull verifies that NULL columns survive an Encode + Decode cycle
// for every supported type.
func TestRoundTripNull(t *testing.T) {
	types := []ast.DataTypeKind{
		ast.TypeInt, ast.TypeBigInt, ast.TypeVarchar,
		ast.TypeBoolean, ast.TypeText, ast.TypeTimestamp,
	}
	for _, typ := range types {
		data := encodeRow(t, NullValue(typ))
		row, err := Decode(data)
		if err != nil {
			t.Fatalf("Decode failed for NULL type %d: %v", typ, err)
		}
		if !row.Values[0].IsNull {
			t.Errorf("NULL round-trip for type %d: IsNull is false", typ)
		}
	}
}

// TestMultiColumnRoundTrip encodes a row with every type and verifies that
// all columns decode correctly, preserving order and values.
func TestMultiColumnRoundTrip(t *testing.T) {
	ts := time.Date(2025, 3, 14, 15, 9, 26, 0, time.UTC)
	row := &Row{
		CodecVersion: 0x01,
		Values: []ColumnValue{
			IntValue(42),
			BigIntValue(-999),
			VarcharValue("hello"),
			BoolValue(true),
			TextValue("world"),
			TimestampValue(ts),
			NullValue(ast.TypeInt),
		},
	}
	data, err := Encode(row)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if decoded.CodecVersion != 0x01 {
		t.Errorf("CodecVersion: got %d, want 1", decoded.CodecVersion)
	}
	if len(decoded.Values) != 7 {
		t.Fatalf("col_count: got %d, want 7", len(decoded.Values))
	}

	// INT
	iv, _ := decoded.Values[0].AsInt()
	if iv != 42 {
		t.Errorf("col 0 INT: got %d, want 42", iv)
	}
	// BIGINT
	bv, _ := decoded.Values[1].AsBigInt()
	if bv != -999 {
		t.Errorf("col 1 BIGINT: got %d, want -999", bv)
	}
	// VARCHAR
	sv, _ := decoded.Values[2].AsString()
	if sv != "hello" {
		t.Errorf("col 2 VARCHAR: got %q, want %q", sv, "hello")
	}
	// BOOLEAN
	boolv, _ := decoded.Values[3].AsBool()
	if !boolv {
		t.Errorf("col 3 BOOLEAN: got false, want true")
	}
	// TEXT
	tv, _ := decoded.Values[4].AsString()
	if tv != "world" {
		t.Errorf("col 4 TEXT: got %q, want %q", tv, "world")
	}
	// TIMESTAMP
	tsv, _ := decoded.Values[5].AsTimestamp()
	if !tsv.Equal(ts) {
		t.Errorf("col 5 TIMESTAMP: got %v, want %v", tsv, ts)
	}
	// NULL
	if !decoded.Values[6].IsNull {
		t.Errorf("col 6: expected NULL")
	}
}

// TestWireFormatHeader verifies that the first three bytes of the encoded
// output contain the correct codec_version and col_count.
func TestWireFormatHeader(t *testing.T) {
	data := encodeRow(t, IntValue(1), VarcharValue("abc"))

	if data[0] != 0x01 {
		t.Errorf("codec_version byte: got 0x%02x, want 0x01", data[0])
	}
	colCount := binary.BigEndian.Uint16(data[1:3])
	if colCount != 2 {
		t.Errorf("col_count: got %d, want 2", colCount)
	}
}

// TestWireFormatNullColumn verifies that a NULL column is encoded as exactly
// 2 bytes: null_flag=0x01 + type_tag.
func TestWireFormatNullColumn(t *testing.T) {
	data := encodeRow(t, NullValue(ast.TypeInt))

	// Header: 3 bytes. Then null_flag (1) + type_tag (1) = 2 bytes total.
	if len(data) != 5 {
		t.Fatalf("encoded length: got %d, want 5", len(data))
	}
	if data[3] != nullFlagNull {
		t.Errorf("null_flag: got 0x%02x, want 0x%02x", data[3], nullFlagNull)
	}
	if data[4] != byte(tagInt) {
		t.Errorf("type_tag: got 0x%02x, want 0x%02x", data[4], byte(tagInt))
	}
}

// TestWireFormatIntColumn verifies the exact byte layout of a non-null INT
// column: null_flag=0x00, type_tag=0x00, 4-byte big-endian int32.
func TestWireFormatIntColumn(t *testing.T) {
	data := encodeRow(t, IntValue(42))
	// Header (3) + null_flag (1) + type_tag (1) + int32 (4) = 9 bytes.
	if len(data) != 9 {
		t.Fatalf("encoded length: got %d, want 9", len(data))
	}
	if data[3] != nullFlagNotNull {
		t.Errorf("null_flag: got 0x%02x, want 0x%02x", data[3], nullFlagNotNull)
	}
	if data[4] != byte(tagInt) {
		t.Errorf("type_tag: got 0x%02x, want 0x%02x", data[4], byte(tagInt))
	}
	raw := data[5:9]
	gotVal := int32(binary.BigEndian.Uint32(raw))
	if gotVal != 42 {
		t.Errorf("int32 value: got %d, want 42", gotVal)
	}
}

// TestDecodeUnknownCodecVersion verifies that Decode rejects a version byte
// other than 0x01.
func TestDecodeUnknownCodecVersion(t *testing.T) {
	data := []byte{0x02, 0x00, 0x00} // version 2, 0 columns
	_, err := Decode(data)
	if !errors.Is(err, ErrUnknownCodecVersion) {
		t.Errorf("expected ErrUnknownCodecVersion, got %v", err)
	}
}

// TestDecodeTruncatedHeader verifies that Decode returns ErrTruncatedValue
// when fewer than 3 bytes are provided.
func TestDecodeTruncatedHeader(t *testing.T) {
	shortPayloads := [][]byte{nil, {0x01}, {0x01, 0x00}}
	for _, data := range shortPayloads {
		_, err := Decode(data)
		if !errors.Is(err, ErrTruncatedValue) {
			t.Errorf("expected ErrTruncatedValue for len=%d, got %v", len(data), err)
		}
	}
}

// TestDecodeTruncatedColumn verifies that Decode returns ErrTruncatedValue
// when the col_count claims more columns than the data contains.
func TestDecodeTruncatedColumn(t *testing.T) {
	// Header says 1 column but body is empty.
	data := []byte{0x01, 0x00, 0x01}
	_, err := Decode(data)
	if !errors.Is(err, ErrTruncatedValue) {
		t.Errorf("expected ErrTruncatedValue, got %v", err)
	}
}

// TestDecodeTruncatedValueBytes verifies truncation detection when the value
// bytes are shorter than the type requires.
func TestDecodeTruncatedValueBytes(t *testing.T) {
	// Header (3) + null_flag (1) + type_tag INT (1) + only 2 bytes of int32.
	data := []byte{0x01, 0x00, 0x01, 0x00, 0x00, 0xAB, 0xCD}
	_, err := Decode(data)
	if !errors.Is(err, ErrTruncatedValue) {
		t.Errorf("expected ErrTruncatedValue, got %v", err)
	}
}

// TestDecodeUnknownTypeTag verifies that Decode returns ErrUnknownTypeTag
// for an unrecognized type tag byte.
func TestDecodeUnknownTypeTag(t *testing.T) {
	// Header (3) + null_flag (1) + invalid type_tag (1).
	data := []byte{0x01, 0x00, 0x01, 0x01, 0xFF}
	_, err := Decode(data)
	if !errors.Is(err, ErrUnknownTypeTag) {
		t.Errorf("expected ErrUnknownTypeTag, got %v", err)
	}
}

// TestAccessorTypeMismatch verifies that accessing a column with the wrong
// typed accessor returns ErrTypeMismatch.
func TestAccessorTypeMismatch(t *testing.T) {
	cv := IntValue(42)
	_, err := cv.AsBigInt()
	if !errors.Is(err, ErrTypeMismatch) {
		t.Errorf("AsBigInt on INT: expected ErrTypeMismatch, got %v", err)
	}
	_, err = cv.AsString()
	if !errors.Is(err, ErrTypeMismatch) {
		t.Errorf("AsString on INT: expected ErrTypeMismatch, got %v", err)
	}
	_, err = cv.AsBool()
	if !errors.Is(err, ErrTypeMismatch) {
		t.Errorf("AsBool on INT: expected ErrTypeMismatch, got %v", err)
	}
	_, err = cv.AsTimestamp()
	if !errors.Is(err, ErrTypeMismatch) {
		t.Errorf("AsTimestamp on INT: expected ErrTypeMismatch, got %v", err)
	}
}

// TestAccessorNullAccess verifies that accessing a NULL column returns
// ErrNullAccess for every typed accessor.
func TestAccessorNullAccess(t *testing.T) {
	tests := []struct {
		name string
		cv   ColumnValue
		fn   func(ColumnValue) error
	}{
		{"AsInt", NullValue(ast.TypeInt), func(cv ColumnValue) error { _, e := cv.AsInt(); return e }},
		{"AsBigInt", NullValue(ast.TypeBigInt), func(cv ColumnValue) error { _, e := cv.AsBigInt(); return e }},
		{"AsString/Varchar", NullValue(ast.TypeVarchar), func(cv ColumnValue) error { _, e := cv.AsString(); return e }},
		{"AsString/Text", NullValue(ast.TypeText), func(cv ColumnValue) error { _, e := cv.AsString(); return e }},
		{"AsBool", NullValue(ast.TypeBoolean), func(cv ColumnValue) error { _, e := cv.AsBool(); return e }},
		{"AsTimestamp", NullValue(ast.TypeTimestamp), func(cv ColumnValue) error { _, e := cv.AsTimestamp(); return e }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn(tt.cv)
			if !errors.Is(err, ErrNullAccess) {
				t.Errorf("expected ErrNullAccess, got %v", err)
			}
		})
	}
}

// TestEmptyRow verifies that a row with zero columns encodes and decodes
// correctly (3-byte header only).
func TestEmptyRow(t *testing.T) {
	row := &Row{CodecVersion: 0x01, Values: nil}
	data, err := Encode(row)
	if err != nil {
		t.Fatalf("Encode empty row: %v", err)
	}
	if len(data) != 3 {
		t.Errorf("empty row length: got %d, want 3", len(data))
	}
	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode empty row: %v", err)
	}
	if len(decoded.Values) != 0 {
		t.Errorf("decoded col_count: got %d, want 0", len(decoded.Values))
	}
}

// TestDecodeFewerColumnsThanSchema simulates reading a row written before a
// column was added. The decoded row should have fewer Values than the current
// schema - the Row Store layer (not the codec) is responsible for filling in
// defaults.
func TestDecodeFewerColumnsThanSchema(t *testing.T) {
	// Encode a row with 1 column.
	data := encodeRow(t, IntValue(100))
	row, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(row.Values) != 1 {
		t.Fatalf("expected 1 value, got %d", len(row.Values))
	}
	// The schema might now have 3 columns; the codec returns only 1.
	// The Row Store appends defaults or NULLs for the missing trailing columns.
}

// TestDecodeMoreColumnsThanSchema simulates reading a row written before a
// column was dropped. The decoded row should contain all the original columns
// (the Row Store filters out dropped columns using the column index map).
func TestDecodeMoreColumnsThanSchema(t *testing.T) {
	// Encode a row with 3 columns (schema at write time had 3 columns).
	data := encodeRow(t, IntValue(1), VarcharValue("old_col"), BoolValue(true))
	row, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	// The current schema might only have 2 columns. The codec still returns
	// all 3 - the Row Store will discard the dropped one.
	if len(row.Values) != 3 {
		t.Errorf("expected 3 values, got %d", len(row.Values))
	}
}

// TestEncodeDeterministic verifies that encoding the same row twice produces
// identical byte output.
func TestEncodeDeterministic(t *testing.T) {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	makeRow := func() *Row {
		return &Row{
			CodecVersion: 0x01,
			Values: []ColumnValue{
				IntValue(-1),
				BigIntValue(math.MaxInt64),
				VarcharValue("determinism"),
				BoolValue(false),
				TextValue("check"),
				TimestampValue(ts),
				NullValue(ast.TypeVarchar),
			},
		}
	}
	data1, err := Encode(makeRow())
	if err != nil {
		t.Fatalf("Encode 1: %v", err)
	}
	data2, err := Encode(makeRow())
	if err != nil {
		t.Fatalf("Encode 2: %v", err)
	}
	if !bytes.Equal(data1, data2) {
		t.Errorf("determinism check failed: two identical rows produced different bytes")
	}
}

// TestIntValueRawBytes verifies that IntValue stores raw two's-complement
// big-endian bytes (NOT sign-flipped - sign flip is only for key encoding).
func TestIntValueRawBytes(t *testing.T) {
	cv := IntValue(-1)
	if len(cv.Raw) != 4 {
		t.Fatalf("raw length: got %d, want 4", len(cv.Raw))
	}
	// -1 as uint32 = 0xFFFFFFFF in two's complement.
	got := binary.BigEndian.Uint32(cv.Raw)
	if got != 0xFFFFFFFF {
		t.Errorf("raw bytes for -1: got 0x%08X, want 0xFFFFFFFF", got)
	}
}

// TestBigIntValueRawBytes verifies BigIntValue stores raw big-endian int64.
func TestBigIntValueRawBytes(t *testing.T) {
	cv := BigIntValue(0)
	got := binary.BigEndian.Uint64(cv.Raw)
	if got != 0 {
		t.Errorf("raw bytes for 0: got %d, want 0", got)
	}
}

// TestVarcharValueRawBytes verifies the VARCHAR wire format:
// [2-byte BE length][UTF-8 bytes].
func TestVarcharValueRawBytes(t *testing.T) {
	cv := VarcharValue("abc")
	if len(cv.Raw) != 5 { // 2 + 3
		t.Fatalf("raw length: got %d, want 5", len(cv.Raw))
	}
	strLen := binary.BigEndian.Uint16(cv.Raw[:2])
	if strLen != 3 {
		t.Errorf("varchar length prefix: got %d, want 3", strLen)
	}
	if string(cv.Raw[2:]) != "abc" {
		t.Errorf("varchar bytes: got %q, want %q", string(cv.Raw[2:]), "abc")
	}
}

// TestTextValueRawBytes verifies the TEXT wire format:
// [4-byte BE length][UTF-8 bytes].
func TestTextValueRawBytes(t *testing.T) {
	cv := TextValue("xyz")
	if len(cv.Raw) != 7 { // 4 + 3
		t.Fatalf("raw length: got %d, want 7", len(cv.Raw))
	}
	strLen := binary.BigEndian.Uint32(cv.Raw[:4])
	if strLen != 3 {
		t.Errorf("text length prefix: got %d, want 3", strLen)
	}
	if string(cv.Raw[4:]) != "xyz" {
		t.Errorf("text bytes: got %q, want %q", string(cv.Raw[4:]), "xyz")
	}
}

// TestTypeTagRoundTrip verifies that every AST DataTypeKind can be mapped to a
// type tag and back.
func TestTypeTagRoundTrip(t *testing.T) {
	kinds := []ast.DataTypeKind{
		ast.TypeInt, ast.TypeBigInt, ast.TypeVarchar,
		ast.TypeBoolean, ast.TypeText, ast.TypeTimestamp,
	}
	for _, kind := range kinds {
		tag, ok := typeTagFromKind(kind)
		if !ok {
			t.Errorf("typeTagFromKind(%d) failed", kind)
			continue
		}
		back, ok := kindFromTypeTag(tag)
		if !ok {
			t.Errorf("kindFromTypeTag(0x%02x) failed", tag)
			continue
		}
		if back != kind {
			t.Errorf("round-trip mismatch: %d -> 0x%02x -> %d", kind, tag, back)
		}
	}
}

// TestValueBytesLenFixedWidth verifies byte lengths for fixed-width type tags.
func TestValueBytesLenFixedWidth(t *testing.T) {
	tests := []struct {
		tag  typeTag
		want int
	}{
		{tagInt, 4},
		{tagBigInt, 8},
		{tagTimestamp, 8},
		{tagFloat, 4},
		{tagDouble, 8},
		{tagBoolean, 1},
	}
	for _, tt := range tests {
		got := valueBytesLen(tt.tag, nil, 0)
		if got != tt.want {
			t.Errorf("valueBytesLen(0x%02x): got %d, want %d", tt.tag, got, tt.want)
		}
	}
}

// TestValueBytesLenVarchar verifies that valueBytesLen reads the 2-byte length
// prefix for VARCHAR and DECIMAL and returns prefix + payload length.
func TestValueBytesLenVarcharAndDecimal(t *testing.T) {
	// Length prefix says 5 bytes.
	data := make([]byte, 7) // 2 + 5
	binary.BigEndian.PutUint16(data, 5)

	got := valueBytesLen(tagVarchar, data, 0)
	if got != 7 {
		t.Errorf("valueBytesLen(VARCHAR): got %d, want 7", got)
	}

	gotDecimal := valueBytesLen(tagDecimal, data, 0)
	if gotDecimal != 7 {
		t.Errorf("valueBytesLen(DECIMAL): got %d, want 7", gotDecimal)
	}
}

// TestValueBytesLenText verifies that valueBytesLen reads the 4-byte length
// prefix for TEXT and returns prefix + payload length.
func TestValueBytesLenText(t *testing.T) {
	// Length prefix says 10 bytes.
	data := make([]byte, 14) // 4 + 10
	binary.BigEndian.PutUint32(data, 10)
	got := valueBytesLen(tagText, data, 0)
	if got != 14 {
		t.Errorf("valueBytesLen(TEXT): got %d, want 14", got)
	}
}

// TestValueBytesLenTruncated verifies that valueBytesLen returns -1 when
// the data is too short to read the length prefix.
func TestValueBytesLenTruncated(t *testing.T) {
	// VARCHAR with only 1 byte (need 2 for prefix).
	got := valueBytesLen(tagVarchar, []byte{0x00}, 0)
	if got != -1 {
		t.Errorf("valueBytesLen(VARCHAR, short): got %d, want -1", got)
	}
	// TEXT with only 2 bytes (need 4 for prefix).
	got = valueBytesLen(tagText, []byte{0x00, 0x00}, 0)
	if got != -1 {
		t.Errorf("valueBytesLen(TEXT, short): got %d, want -1", got)
	}
}

// TestDecodeNonAliasing verifies that the decoded Row's Raw byte slices are
// independent copies of the input - mutating the input after Decode must not
// affect the decoded values.
func TestDecodeNonAliasing(t *testing.T) {
	data := encodeRow(t, IntValue(42))
	row, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	// Corrupt the original data.
	for i := range data {
		data[i] = 0xFF
	}

	got, err := row.Values[0].AsInt()
	if err != nil {
		t.Fatalf("AsInt after corruption: %v", err)
	}
	if got != 42 {
		t.Errorf("value changed after input mutation: got %d, want 42", got)
	}
}

// TestManyColumns verifies that a row with many columns (255) encodes and
// decodes correctly, stress-testing the col_count header and loop logic.
func TestManyColumns(t *testing.T) {
	const n = 255
	values := make([]ColumnValue, n)
	for i := range n {
		values[i] = IntValue(int32(i))
	}
	row := &Row{CodecVersion: 0x01, Values: values}
	data, err := Encode(row)
	if err != nil {
		t.Fatalf("Encode %d columns: %v", n, err)
	}
	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode %d columns: %v", n, err)
	}
	if len(decoded.Values) != n {
		t.Fatalf("col_count: got %d, want %d", len(decoded.Values), n)
	}
	for i := range n {
		got, err := decoded.Values[i].AsInt()
		if err != nil {
			t.Fatalf("col %d AsInt: %v", i, err)
		}
		if got != int32(i) {
			t.Errorf("col %d: got %d, want %d", i, got, i)
		}
	}
}

// TestMixedNullNonNull encodes a row with alternating NULL and non-NULL
// columns to verify the decoder correctly handles the interleaving.
func TestMixedNullNonNull(t *testing.T) {
	values := []ColumnValue{
		NullValue(ast.TypeInt),
		VarcharValue("hello"),
		NullValue(ast.TypeBoolean),
		BigIntValue(42),
		NullValue(ast.TypeText),
	}
	data := encodeRow(t, values...)
	row, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(row.Values) != 5 {
		t.Fatalf("col_count: got %d, want 5", len(row.Values))
	}
	// Check NULLs.
	if !row.Values[0].IsNull {
		t.Errorf("col 0 should be NULL")
	}
	if !row.Values[2].IsNull {
		t.Errorf("col 2 should be NULL")
	}
	if !row.Values[4].IsNull {
		t.Errorf("col 4 should be NULL")
	}
	// Check non-NULLs.
	sv, _ := row.Values[1].AsString()
	if sv != "hello" {
		t.Errorf("col 1: got %q, want %q", sv, "hello")
	}
	bv, _ := row.Values[3].AsBigInt()
	if bv != 42 {
		t.Errorf("col 3: got %d, want 42", bv)
	}
}
