package executor

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/codec"
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

// decimalValue is the runtime representation of a DECIMAL column value.
type decimalValue string

func (d decimalValue) String() string {
	return string(d)
}

// columnValueToAny converts a decoded codec.ColumnValue into a Go any
// value. NULL columns become nil.
func columnValueToAny(cv codec.ColumnValue) (any, error) {
	if cv.IsNull {
		return nil, nil
	}
	switch cv.Type {
	case ast.TypeInt:
		v, err := cv.AsInt()
		return v, err
	case ast.TypeBigInt:
		v, err := cv.AsBigInt()
		return v, err
	case ast.TypeFloat:
		v, err := cv.AsFloat()
		return v, err
	case ast.TypeDouble:
		v, err := cv.AsDouble()
		return v, err
	case ast.TypeDecimal:
		v, err := cv.AsDecimal()
		if err != nil {
			return nil, err
		}
		return decimalValue(v), nil
	case ast.TypeVarchar, ast.TypeText:
		v, err := cv.AsString()
		return v, err
	case ast.TypeBoolean:
		v, err := cv.AsBool()
		return v, err
	case ast.TypeTimestamp:
		v, err := cv.AsTimestamp()
		return v, err
	default:
		return nil, fmt.Errorf("columnValueToAny: unsupported type %d", cv.Type)
	}
}

// anyToColumnValue converts a Go any value into a codec.ColumnValue for
// the given target type. A nil input produces a NULL ColumnValue.
//
// Numeric widening is handled: e.g. an int32 stored in a BIGINT column is
// promoted to int64. This is necessary because the planner's type promotion
// can produce values wider than the literal's original Go type.
func anyToColumnValue(v any, typ ast.DataTypeKind) (codec.ColumnValue, error) {
	if v == nil {
		return codec.NullValue(typ), nil
	}
	switch typ {
	case ast.TypeInt:
		n, err := toInt32(v)
		if err != nil {
			return codec.ColumnValue{}, err
		}
		return codec.IntValue(n), nil
	case ast.TypeBigInt:
		n, err := toInt64(v)
		if err != nil {
			return codec.ColumnValue{}, err
		}
		return codec.BigIntValue(n), nil
	case ast.TypeFloat:
		f, err := toFloat32(v)
		if err != nil {
			return codec.ColumnValue{}, err
		}
		return codec.FloatValue(f), nil
	case ast.TypeDouble:
		f, err := toFloat64(v)
		if err != nil {
			return codec.ColumnValue{}, err
		}
		return codec.DoubleValue(f), nil
	case ast.TypeDecimal:
		switch s := v.(type) {
		case decimalValue:
			return codec.DecimalValue(string(s))
		case string:
			return codec.DecimalValue(s)
		default:
			return codec.ColumnValue{}, fmt.Errorf("%w: cannot convert %T to DECIMAL", ErrTypeMismatch, v)
		}
	case ast.TypeVarchar:
		s, ok := v.(string)
		if !ok {
			return codec.ColumnValue{}, fmt.Errorf("%w: cannot convert %T to VARCHAR", ErrTypeMismatch, v)
		}
		return codec.VarcharValue(s)
	case ast.TypeText:
		s, ok := v.(string)
		if !ok {
			return codec.ColumnValue{}, fmt.Errorf("%w: cannot convert %T to TEXT", ErrTypeMismatch, v)
		}
		return codec.TextValue(s)
	case ast.TypeBoolean:
		b, ok := v.(bool)
		if !ok {
			return codec.ColumnValue{}, fmt.Errorf("%w: cannot convert %T to BOOLEAN", ErrTypeMismatch, v)
		}
		return codec.BoolValue(b), nil
	case ast.TypeTimestamp:
		t, ok := v.(time.Time)
		if !ok {
			return codec.ColumnValue{}, fmt.Errorf("%w: cannot convert %T to TIMESTAMP", ErrTypeMismatch, v)
		}
		return codec.TimestampValue(t), nil
	default:
		return codec.ColumnValue{}, fmt.Errorf("anyToColumnValue: unsupported type %d", typ)
	}
}

// toInt32 coerces a numeric any value to int32, with range checking.
func toInt32(v any) (int32, error) {
	switch n := v.(type) {
	case int32:
		return n, nil
	case int64:
		if n < math.MinInt32 || n > math.MaxInt32 {
			return 0, fmt.Errorf("%w: int64 %d overflows INT", ErrTypeMismatch, n)
		}
		return int32(n), nil
	case float32:
		return toInt32(float64(n))
	case float64:
		if math.IsNaN(n) || n < math.MinInt32 || n > math.MaxInt32 {
			return 0, fmt.Errorf("%w: %v overflows INT", ErrTypeMismatch, n)
		}
		return int32(n), nil
	default:
		return 0, fmt.Errorf("%w: cannot convert %T to INT", ErrTypeMismatch, v)
	}
}

// toInt64 coerces a numeric any value to int64.
func toInt64(v any) (int64, error) {
	switch n := v.(type) {
	case int32:
		return int64(n), nil
	case int64:
		return n, nil
	case float32:
		return toInt64(float64(n))
	case float64:
		if math.IsNaN(n) || n < math.MinInt64 || n >= float64(uint64(1)<<63) {
			return 0, fmt.Errorf("%w: %v overflows BIGINT", ErrTypeMismatch, n)
		}
		return int64(n), nil
	default:
		return 0, fmt.Errorf("%w: cannot convert %T to BIGINT", ErrTypeMismatch, v)
	}
}

// toFloat32 coerces a numeric any value to float32.
func toFloat32(v any) (float32, error) {
	switch n := v.(type) {
	case int32:
		return float32(n), nil
	case int64:
		return float32(n), nil
	case float32:
		return n, nil
	case float64:
		return float32(n), nil
	case decimalValue:
		f, err := strconv.ParseFloat(string(n), 32)
		if err != nil {
			return 0, fmt.Errorf("%w: cannot convert decimal %q to FLOAT: %w", ErrTypeMismatch, n, err)
		}
		return float32(f), nil
	default:
		return 0, fmt.Errorf("%w: cannot convert %T to FLOAT", ErrTypeMismatch, v)
	}
}

// toFloat64 coerces a numeric any value to float64.
func toFloat64(v any) (float64, error) {
	switch n := v.(type) {
	case int32:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case float32:
		return float64(n), nil
	case float64:
		return n, nil
	case decimalValue:
		f, err := strconv.ParseFloat(string(n), 64)
		if err != nil {
			return 0, fmt.Errorf("%w: cannot convert decimal %q to numeric: %w", ErrTypeMismatch, n, err)
		}
		return f, nil
	default:
		return 0, fmt.Errorf("%w: cannot convert %T to numeric", ErrTypeMismatch, v)
	}
}

// compareValues compares two non-nil Go values of compatible types.
// Returns -1, 0, or 1 like strings.Compare. Both values must be non-nil;
// NULL handling is done by the caller.
func compareValues(a, b any) (int, error) {
	if _, ok := a.(decimalValue); ok {
		return compareAsRat(a, b)
	}
	if _, ok := b.(decimalValue); ok {
		return compareAsRat(a, b)
	}

	switch av := a.(type) {
	case int32:
		if _, ok := b.(float32); ok {
			bv, err := toFloat64(b)
			if err != nil {
				return 0, err
			}
			return cmpOrdered(float64(av), bv), nil
		}
		if _, ok := b.(float64); ok {
			bv, err := toFloat64(b)
			if err != nil {
				return 0, err
			}
			return cmpOrdered(float64(av), bv), nil
		}
		bv, err := toInt64(b)
		if err != nil {
			return 0, err
		}
		return cmpOrdered(int64(av), bv), nil
	case int64:
		if _, ok := b.(float32); ok {
			bv, err := toFloat64(b)
			if err != nil {
				return 0, err
			}
			return cmpOrdered(float64(av), bv), nil
		}
		if _, ok := b.(float64); ok {
			bv, err := toFloat64(b)
			if err != nil {
				return 0, err
			}
			return cmpOrdered(float64(av), bv), nil
		}
		bv, err := toInt64(b)
		if err != nil {
			return 0, err
		}
		return cmpOrdered(av, bv), nil
	case float32:
		bv, err := toFloat64(b)
		if err != nil {
			return 0, err
		}
		return cmpOrdered(float64(av), bv), nil
	case float64:
		bv, err := toFloat64(b)
		if err != nil {
			return 0, err
		}
		return cmpOrdered(av, bv), nil
	case string:
		bv, ok := b.(string)
		if !ok {
			return 0, fmt.Errorf("%w: cannot compare string with %T", ErrTypeMismatch, b)
		}
		return cmpOrdered(av, bv), nil
	case bool:
		bv, ok := b.(bool)
		if !ok {
			return 0, fmt.Errorf("%w: cannot compare bool with %T", ErrTypeMismatch, b)
		}
		ai, bi := boolToInt(av), boolToInt(bv)
		return cmpOrdered(ai, bi), nil
	case time.Time:
		bv, ok := b.(time.Time)
		if !ok {
			return 0, fmt.Errorf("%w: cannot compare time with %T", ErrTypeMismatch, b)
		}
		switch {
		case av.Before(bv):
			return -1, nil
		case av.After(bv):
			return 1, nil
		default:
			return 0, nil
		}
	default:
		return 0, fmt.Errorf("%w: cannot compare %T", ErrTypeMismatch, a)
	}
}

// compareAsRat compares two numeric operands as rational numbers using big.Rat.
func compareAsRat(a, b any) (int, error) {
	ra, err := toRat(a)
	if err != nil {
		return 0, err
	}
	rb, err := toRat(b)
	if err != nil {
		return 0, err
	}
	return ra.Cmp(rb), nil
}

// toRat converts a numeric value (int32, int64, float32, float64, decimalValue)
// into a *big.Rat.
func toRat(v any) (*big.Rat, error) {
	switch val := v.(type) {
	case *big.Rat:
		return new(big.Rat).Set(val), nil
	case decimalValue:
		r := new(big.Rat)
		if _, ok := r.SetString(string(val)); !ok {
			return nil, fmt.Errorf("%w: invalid decimal %q", ErrTypeMismatch, val)
		}
		return r, nil
	case int32:
		return new(big.Rat).SetInt64(int64(val)), nil
	case int64:
		return new(big.Rat).SetInt64(val), nil
	case float32:
		f := float64(val)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, fmt.Errorf("%w: cannot convert %v to big.Rat", ErrTypeMismatch, f)
		}
		return new(big.Rat).SetFloat64(f), nil
	case float64:
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return nil, fmt.Errorf("%w: cannot convert %v to big.Rat", ErrTypeMismatch, val)
		}
		return new(big.Rat).SetFloat64(val), nil
	default:
		return nil, fmt.Errorf("%w: cannot convert %T to big.Rat", ErrTypeMismatch, v)
	}
}

// isDecimalAny reports whether v is a decimalValue runtime type.
func isDecimalAny(v any) bool {
	_, ok := v.(decimalValue)
	return ok
}

// ratToDecimal converts a *big.Rat into a decimalValue string representation,
// preserving exact precision for terminating decimals and formatting non-terminating
// decimals up to 30 decimal places without trailing zeros.
func ratToDecimal(r *big.Rat) decimalValue {
	if r.Sign() == 0 {
		return "0"
	}

	denom := new(big.Int).Set(r.Denom())
	scale2 := int(denom.TrailingZeroBits())
	denom.Rsh(denom, uint(scale2))

	five := big.NewInt(5)
	rem := new(big.Int)
	scale5 := 0
	for denom.Sign() > 0 {
		d, m := new(big.Int).DivMod(denom, five, rem)
		if m.Sign() != 0 {
			break
		}
		denom = d
		scale5++
	}

	prec := 0
	if denom.Cmp(big.NewInt(1)) == 0 {
		// Terminating decimal: exact scale is max(scale2, scale5).
		prec = scale2
		if scale5 > prec {
			prec = scale5
		}
	} else {
		// Non-terminating decimal: use default precision.
		prec = 30
		if scale2 > prec {
			prec = scale2
		}
		if scale5 > prec {
			prec = scale5
		}
	}

	const maxDecimalPrec = 1000
	if prec > maxDecimalPrec {
		prec = maxDecimalPrec
	}

	s := r.FloatString(prec)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	if s == "-0" || s == "" {
		s = "0"
	}
	return decimalValue(s)
}

// coerceRatResult converts a big.Rat result to the target DataTypeKind.
// Returns ErrNumericOverflow if an integer or float target overflows.
func coerceRatResult(r *big.Rat, kind ast.DataTypeKind) (any, error) {
	switch kind {
	case ast.TypeDecimal:
		return ratToDecimal(r), nil
	case ast.TypeInt:
		intPart := new(big.Int).Quo(r.Num(), r.Denom())
		if !intPart.IsInt64() {
			return nil, ErrNumericOverflow
		}
		val := intPart.Int64()
		if val < math.MinInt32 || val > math.MaxInt32 {
			return nil, ErrNumericOverflow
		}
		return int32(val), nil
	case ast.TypeBigInt:
		intPart := new(big.Int).Quo(r.Num(), r.Denom())
		if !intPart.IsInt64() {
			return nil, ErrNumericOverflow
		}
		return intPart.Int64(), nil
	case ast.TypeFloat:
		f, _ := r.Float64()
		if math.IsInf(f, 0) || math.Abs(f) > math.MaxFloat32 {
			return nil, ErrNumericOverflow
		}
		return float32(f), nil
	case ast.TypeDouble:
		f, _ := r.Float64()
		if math.IsInf(f, 0) {
			return nil, ErrNumericOverflow
		}
		return f, nil
	default:
		return ratToDecimal(r), nil
	}
}

// cmpOrdered returns -1, 0, or 1 for ordered types.
func cmpOrdered[T int | int64 | float64 | string](a, b T) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// boolToInt converts a boolean value to 1 for true or 0 for false for ordered comparison.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// anyToPKValue converts an any value to the concrete type that
// encoding.EncodePK expects for the given column type. This handles
// the mismatch where evalExpr may produce int64 for an int32 PK column.
func anyToPKValue(v any, typ ast.DataTypeKind) (any, error) {
	if v == nil {
		return nil, ErrNullPrimaryKey
	}
	switch typ {
	case ast.TypeInt:
		n, err := toInt32(v)
		return n, err
	case ast.TypeBigInt:
		n, err := toInt64(v)
		return n, err
	case ast.TypeFloat:
		f, err := toFloat32(v)
		return f, err
	case ast.TypeDouble:
		f, err := toFloat64(v)
		return f, err
	case ast.TypeVarchar, ast.TypeText:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%w: cannot convert %T to string for PK", ErrTypeMismatch, v)
		}
		return s, nil
	case ast.TypeDecimal:
		switch s := v.(type) {
		case decimalValue:
			return string(s), nil
		case string:
			return s, nil
		default:
			return nil, fmt.Errorf("%w: cannot convert %T to string for PK", ErrTypeMismatch, v)
		}
	case ast.TypeBoolean:
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("%w: cannot convert %T to bool for PK", ErrTypeMismatch, v)
		}
		return b, nil
	case ast.TypeTimestamp:
		t, ok := v.(time.Time)
		if !ok {
			return nil, fmt.Errorf("%w: cannot convert %T to time for PK", ErrTypeMismatch, v)
		}
		return t, nil
	default:
		return nil, fmt.Errorf("anyToPKValue: unsupported PK type %d", typ)
	}
}

// decodeRowToAny decodes a codec.Row into a []any slice matching the
// current schema's active columns. It handles schema evolution: if the
// encoded row has fewer columns than the current schema (columns were
// added), the missing positions are filled with nil (NULL). If the
// encoded row has more columns than the current schema has total columns
// (shouldn't happen normally), extra columns are ignored. Dropped
// columns are skipped using the column-index mapping.
func decodeRowToAny(encoded *codec.Row, colCount int, indexMap []int) ([]any, error) {
	result := make([]any, colCount)
	for writeIdx, cv := range encoded.Values {
		if writeIdx >= len(indexMap) {
			break
		}
		activeIdx := indexMap[writeIdx]
		if activeIdx < 0 {
			// Column was dropped — skip it.
			continue
		}
		v, err := columnValueToAny(cv)
		if err != nil {
			return nil, fmt.Errorf("decoding column %d: %w", writeIdx, err)
		}
		result[activeIdx] = v
	}
	return result, nil
}

// encodeSequenceValue serializes a uint64 sequence counter to 8 big-endian
// bytes for storage as a KV value under the sequence key.
func encodeSequenceValue(seq uint64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, seq)
	return buf
}

// decodeSequenceValue deserializes a uint64 sequence counter from 8
// big-endian bytes read from the sequence key. Returns an error if the
// data is malformed (fewer than 8 bytes).
func decodeSequenceValue(data []byte) (uint64, error) {
	if len(data) < 8 {
		return 0, fmt.Errorf("executor: bad sequence data: expected at least 8 bytes, got %d", len(data))
	}
	return binary.BigEndian.Uint64(data), nil
}
