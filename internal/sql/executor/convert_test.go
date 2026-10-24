package executor

import (
	"errors"
	"math"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/codec"
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

func mustVarchar(v string) codec.ColumnValue {
	cv, _ := codec.VarcharValue(v)
	return cv
}

func mustText(v string) codec.ColumnValue {
	cv, _ := codec.TextValue(v)
	return cv
}

func mustDecimal(v string) codec.ColumnValue {
	cv, _ := codec.DecimalValue(v)
	return cv
}

func TestToFloat32(t *testing.T) {
	tests := []struct {
		in      any
		want    float32
		wantErr bool
	}{
		{int32(42), 42.0, false},
		{int64(42), 42.0, false},
		{float32(42.5), 42.5, false},
		{float64(42.5), 42.5, false},
		{"not-a-number", 0, true},
	}

	for _, tc := range tests {
		got, err := toFloat32(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("toFloat32(%v) error = %v, wantErr %v", tc.in, err, tc.wantErr)
			continue
		}
		if got != tc.want {
			t.Errorf("toFloat32(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		in      any
		want    float64
		wantErr bool
	}{
		{int32(42), 42.0, false},
		{int64(42), 42.0, false},
		{float32(42.5), 42.5, false},
		{float64(42.5), 42.5, false},
		{"not-a-number", 0, true},
	}

	for _, tc := range tests {
		got, err := toFloat64(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("toFloat64(%v) error = %v, wantErr %v", tc.in, err, tc.wantErr)
			continue
		}
		if got != tc.want {
			t.Errorf("toFloat64(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestToInt64(t *testing.T) {
	tests := []struct {
		in      any
		want    int64
		wantErr bool
	}{
		{int32(42), 42, false},
		{int64(42), 42, false},
		{float32(42.0), 42, false},
		{float64(42.0), 42, false},
		{float32(2.7), 2, false},
		{float64(math.NaN()), 0, true},
		{float64(1 << 63), 0, true},
		{"not-a-number", 0, true},
	}

	for _, tc := range tests {
		got, err := toInt64(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("toInt64(%v) error = %v, wantErr %v", tc.in, err, tc.wantErr)
			continue
		}
		if got != tc.want {
			t.Errorf("toInt64(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestToInt32(t *testing.T) {
	tests := []struct {
		in      any
		want    int32
		wantErr bool
	}{
		{int32(42), 42, false},
		{int64(42), 42, false},
		{int64(math.MaxInt64), 0, true}, // overflow
		{float32(42.0), 42, false},
		{float64(42.0), 42, false},
		{float32(2.7), 2, false},
		{float64(math.NaN()), 0, true},
		{float64(math.MaxInt32) + 1, 0, true},
		{"not-a-number", 0, true},
	}

	for _, tc := range tests {
		got, err := toInt32(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("toInt32(%v) error = %v, wantErr %v", tc.in, err, tc.wantErr)
			continue
		}
		if got != tc.want {
			t.Errorf("toInt32(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestAnyToPKValue(t *testing.T) {
	now := time.Now()
	tests := []struct {
		in      any
		typ     ast.DataTypeKind
		want    any
		wantErr bool
	}{
		{nil, ast.TypeInt, nil, true}, // ErrNullPrimaryKey
		{int64(42), ast.TypeInt, int32(42), false},
		{int32(42), ast.TypeBigInt, int64(42), false},
		{int32(42), ast.TypeFloat, float32(42), false},
		{int32(42), ast.TypeDouble, float64(42), false},
		{"hello", ast.TypeVarchar, "hello", false},
		{"hello", ast.TypeText, "hello", false},
		{"1.23", ast.TypeDecimal, "1.23", false},
		{decimalValue("1.23"), ast.TypeDecimal, "1.23", false},
		{123, ast.TypeVarchar, nil, true},
		{true, ast.TypeBoolean, true, false},
		{123, ast.TypeBoolean, nil, true},
		{now, ast.TypeTimestamp, now, false},
		{123, ast.TypeTimestamp, nil, true},
		{123, 999, nil, true}, // unsupported type
	}

	for _, tc := range tests {
		got, err := anyToPKValue(tc.in, tc.typ)
		if (err != nil) != tc.wantErr {
			t.Errorf("anyToPKValue(%v, %v) error = %v, wantErr %v", tc.in, tc.typ, err, tc.wantErr)
			continue
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("anyToPKValue(%v, %v) = %v, want %v", tc.in, tc.typ, got, tc.want)
		}
	}
}

func TestAnyToColumnValue(t *testing.T) {
	now := time.Now()
	tests := []struct {
		in      any
		typ     ast.DataTypeKind
		want    codec.ColumnValue
		wantErr bool
	}{
		{nil, ast.TypeInt, codec.NullValue(ast.TypeInt), false},
		{int64(42), ast.TypeInt, codec.IntValue(42), false},
		{int32(42), ast.TypeBigInt, codec.BigIntValue(42), false},
		{int32(42), ast.TypeFloat, codec.FloatValue(42), false},
		{int32(42), ast.TypeDouble, codec.DoubleValue(42), false},
		{"hello", ast.TypeVarchar, mustVarchar("hello"), false},
		{"hello", ast.TypeText, mustText("hello"), false},
		{"1.23", ast.TypeDecimal, mustDecimal("1.23"), false},
		{decimalValue("1.23"), ast.TypeDecimal, mustDecimal("1.23"), false},
		{123, ast.TypeVarchar, codec.ColumnValue{}, true},
		{123, ast.TypeText, codec.ColumnValue{}, true},
		{123, ast.TypeDecimal, codec.ColumnValue{}, true},
		{true, ast.TypeBoolean, codec.BoolValue(true), false},
		{123, ast.TypeBoolean, codec.ColumnValue{}, true},
		{now, ast.TypeTimestamp, codec.TimestampValue(now), false},
		{123, ast.TypeTimestamp, codec.ColumnValue{}, true},
		{123, 999, codec.ColumnValue{}, true}, // unsupported type
		{"bad-float", ast.TypeFloat, codec.ColumnValue{}, true},
		{"bad-double", ast.TypeDouble, codec.ColumnValue{}, true},
		{"bad-int", ast.TypeInt, codec.ColumnValue{}, true},
		{"bad-bigint", ast.TypeBigInt, codec.ColumnValue{}, true},
	}

	for _, tc := range tests {
		got, err := anyToColumnValue(tc.in, tc.typ)
		if (err != nil) != tc.wantErr {
			t.Errorf("anyToColumnValue(%v, %v) error = %v, wantErr %v", tc.in, tc.typ, err, tc.wantErr)
			continue
		}
		if !tc.wantErr && (got.Type != tc.want.Type || got.IsNull != tc.want.IsNull) {
			t.Errorf("anyToColumnValue(%v, %v) = %v, want %v", tc.in, tc.typ, got, tc.want)
		}
	}
}

func TestSequenceValueEncoding(t *testing.T) {
	tests := []uint64{0, 1, 42, math.MaxUint64}

	for _, seq := range tests {
		encoded := encodeSequenceValue(seq)
		if len(encoded) != 8 {
			t.Errorf("encodeSequenceValue(%d) length = %d, want 8", seq, len(encoded))
		}
		decoded, err := decodeSequenceValue(encoded)
		if err != nil {
			t.Errorf("decodeSequenceValue failed: %v", err)
		}
		if decoded != seq {
			t.Errorf("decodeSequenceValue(encodeSequenceValue(%d)) = %d, want %d", seq, decoded, seq)
		}
	}

	// Test short buffer returns an error
	if _, err := decodeSequenceValue([]byte{1, 2, 3}); err == nil {
		t.Error("decodeSequenceValue(short_buffer) expected error, got nil")
	}
}

func TestCompareValues(t *testing.T) {
	tests := []struct {
		a, b    any
		want    int
		wantErr bool
	}{
		{int32(1), int64(2), -1, false},
		{int64(2), int64(1), 1, false},
		{int32(2), float64(2.7), -1, false},
		{int64(3), float32(2.7), 1, false},
		{int32(2), float64(2.0), 0, false},
		{float32(1.0), float64(1.0), 0, false},
		{float64(2.0), float64(1.0), 1, false},
		{"a", "b", -1, false},
		{"b", "a", 1, false},
		{"a", "a", 0, false},
		{"10", "2", -1, false},
		{"2", "10", 1, false},
		{"10.0", "2.5", -1, false},
		{"2.5", "10.0", 1, false},
		{"10.0", "10", 1, false},
		{"10.0", "2x", -1, false},
		{true, false, 1, false},
		{false, true, -1, false},
		{true, true, 0, false},
		// DECIMAL comparisons
		{decimalValue("10.0"), decimalValue("2.5"), 1, false},
		{decimalValue("2.5"), decimalValue("10.0"), -1, false},
		{decimalValue("10.0"), decimalValue("10"), 0, false},
		{decimalValue("10.0"), float64(2.5), 1, false},
		{float64(2.5), decimalValue("10.0"), -1, false},
		{decimalValue("10.0"), int32(10), 0, false},
		{int64(10), decimalValue("10.0"), 0, false},
		// Precision preservation beyond float64
		{decimalValue("100000000000000000000.00000000000000000001"), decimalValue("100000000000000000000.00000000000000000002"), -1, false},
		// DECIMAL vs non-numeric
		{decimalValue("10.0"), "10.0", 0, true},
		{"10.0", decimalValue("10.0"), 0, true},
		{decimalValue("10.0"), true, 0, true},
		{int32(1), "1", 0, true},
		{int64(1), "1", 0, true},
		{float32(1.0), "1", 0, true},
		{float64(1.0), "1", 0, true},
		{"1", 1, 0, true},
		{true, 1, 0, true},
		{1, 1, 0, true}, // unhandled int vs int without cast
		{time.Unix(0, 0), time.Unix(1, 0), -1, false},
		{time.Unix(1, 0), time.Unix(0, 0), 1, false},
		{time.Unix(0, 0), time.Unix(0, 0), 0, false},
		{time.Unix(0, 0), 0, 0, true},
	}

	for _, tc := range tests {
		got, err := compareValues(tc.a, tc.b)
		if (err != nil) != tc.wantErr {
			t.Errorf("compareValues(%v, %v) error = %v, wantErr %v", tc.a, tc.b, err, tc.wantErr)
			continue
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("compareValues(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestColumnValueToAny(t *testing.T) {
	now := time.Now()
	tests := []struct {
		cv      codec.ColumnValue
		want    any
		wantErr bool
	}{
		{codec.NullValue(ast.TypeInt), nil, false},
		{codec.IntValue(42), int32(42), false},
		{codec.BigIntValue(42), int64(42), false},
		{codec.FloatValue(42.0), float32(42.0), false},
		{codec.DoubleValue(42.0), float64(42.0), false},
		{mustDecimal("1.23"), decimalValue("1.23"), false},
		{mustVarchar("hello"), "hello", false},
		{codec.BoolValue(true), true, false},
		{codec.TimestampValue(now), now, false},
		{codec.ColumnValue{Type: 999}, nil, true}, // unsupported type
	}

	for _, tc := range tests {
		got, err := columnValueToAny(tc.cv)
		if (err != nil) != tc.wantErr {
			t.Errorf("columnValueToAny(%v) error = %v, wantErr %v", tc.cv, err, tc.wantErr)
			continue
		}
		if tc.wantErr {
			continue
		}
		if gotT, ok := got.(time.Time); ok {
			if wantT, ok := tc.want.(time.Time); !ok || !gotT.Equal(wantT) {
				t.Errorf("columnValueToAny(%v) = %v, want %v", tc.cv, got, tc.want)
			}
			continue
		}
		if got != tc.want {
			t.Errorf("columnValueToAny(%v) = %v (%T), want %v (%T)", tc.cv, got, got, tc.want, tc.want)
		}
	}
}

func TestCoerceRatResult_DecimalPrecision(t *testing.T) {
	// 1 / 10^1001 terminating decimal should preserve required scale (1001 places) and not truncate to 0.
	denom1001 := new(big.Int).Exp(big.NewInt(10), big.NewInt(1001), nil)
	r1001 := new(big.Rat).SetFrac(big.NewInt(1), denom1001)
	res, err := coerceRatResult(r1001, ast.TypeDecimal)
	if err != nil {
		t.Fatalf("coerceRatResult(1/10^1001) failed: %v", err)
	}
	want1001 := decimalValue("0." + strings.Repeat("0", 1000) + "1")
	if res != want1001 {
		t.Fatalf("coerceRatResult(1/10^1001) = %q, want %q", res, want1001)
	}

	// Precision exceeding maxDecimalPrec should return ErrNumericOverflow.
	denomOversized := new(big.Int).Exp(big.NewInt(10), big.NewInt(10001), nil)
	rOversized := new(big.Rat).SetFrac(big.NewInt(1), denomOversized)
	_, err = coerceRatResult(rOversized, ast.TypeDecimal)
	if !errors.Is(err, ErrNumericOverflow) {
		t.Fatalf("expected ErrNumericOverflow for precision exceeding max, got %v", err)
	}

	// Non-terminating decimal with large prime denominator rounding to zero should return ErrNumericOverflow.
	denomPrime := new(big.Int).Exp(big.NewInt(10), big.NewInt(50), nil)
	denomPrime.Add(denomPrime, big.NewInt(1)) // 10^50 + 1 (coprime to 2 and 5)
	rUnderflow := new(big.Rat).SetFrac(big.NewInt(1), denomPrime)
	_, err = coerceRatResult(rUnderflow, ast.TypeDecimal)
	if !errors.Is(err, ErrNumericOverflow) {
		t.Fatalf("expected ErrNumericOverflow for underflow to zero, got %v", err)
	}
}
