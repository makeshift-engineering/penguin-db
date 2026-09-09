package executor

import (
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/codec"
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

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
		return v, err
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
		s, ok := v.(string)
		if !ok {
			return codec.ColumnValue{}, fmt.Errorf("%w: cannot convert %T to DECIMAL", ErrTypeMismatch, v)
		}
		return codec.DecimalValue(s)
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
		return int32(n), nil
	case float64:
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
		return int64(n), nil
	case float64:
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
	default:
		return 0, fmt.Errorf("%w: cannot convert %T to numeric", ErrTypeMismatch, v)
	}
}

// compareValues compares two non-nil Go values of compatible types.
// Returns -1, 0, or 1 like strings.Compare. Both values must be non-nil;
// NULL handling is done by the caller.
func compareValues(a, b any) (int, error) {
	switch av := a.(type) {
	case int32:
		bv, err := toInt64(b)
		if err != nil {
			return 0, err
		}
		return cmpOrdered(int64(av), bv), nil
	case int64:
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
	case ast.TypeVarchar, ast.TypeText, ast.TypeDecimal:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%w: cannot convert %T to string for PK", ErrTypeMismatch, v)
		}
		return s, nil
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
// big-endian bytes read from the sequence key.
func decodeSequenceValue(data []byte) uint64 {
	if len(data) < 8 {
		return 0
	}
	return binary.BigEndian.Uint64(data)
}
