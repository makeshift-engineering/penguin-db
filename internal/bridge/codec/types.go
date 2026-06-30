package codec

import (
	"encoding/binary"
	"math"
	"time"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

// typeTag is the 1-byte identifier written into the value encoding for each
// column. It identifies the SQL type so that decoders can skip columns without
// consulting the catalog — critical for schema evolution when columns are
// dropped.
type typeTag byte

const (
	tagInt       typeTag = 0x00
	tagBigInt    typeTag = 0x01
	tagVarchar   typeTag = 0x02
	tagBoolean   typeTag = 0x03
	tagText      typeTag = 0x04
	tagTimestamp typeTag = 0x05
	tagFloat     typeTag = 0x06
	tagDouble    typeTag = 0x07
	tagDecimal   typeTag = 0x08
)

// maxVarcharLen is the maximum byte length for a VARCHAR value (2-byte prefix → 65535).
const maxVarcharLen = math.MaxUint16

// maxTextLen is the maximum byte length for a TEXT value (4-byte prefix → 4294967295).
const maxTextLen = math.MaxUint32

// size constants for encoded types
const (
	sizeBoolean       = 1
	sizeInt           = 4
	sizeFloat         = 4
	sizeBigInt        = 8
	sizeDouble        = 8
	sizeTimestamp     = 8
	sizeVarcharPrefix = 2
	sizeDecimalPrefix = 2
	sizeTextPrefix    = 4
)

// ColumnValue holds a single decoded column value from a KV row.
// The Type field records the SQL type. IsNull indicates a SQL NULL.
// When IsNull is false, Raw contains the type-specific binary payload
// (not sort-preserving — raw binary representation).
type ColumnValue struct {
	Type   ast.DataTypeKind
	IsNull bool
	Raw    []byte // nil when IsNull is true
}

// IntValue creates a non-null INT ColumnValue from v.
// The raw bytes are 4-byte big-endian int32 (two's complement, not sign-flipped).
func IntValue(v int32) ColumnValue {
	raw := make([]byte, sizeInt)
	binary.BigEndian.PutUint32(raw, uint32(v))
	return ColumnValue{Type: ast.TypeInt, Raw: raw}
}

// BigIntValue creates a non-null BIGINT ColumnValue from v.
// The raw bytes are 8-byte big-endian int64.
func BigIntValue(v int64) ColumnValue {
	raw := make([]byte, sizeBigInt)
	binary.BigEndian.PutUint64(raw, uint64(v))
	return ColumnValue{Type: ast.TypeBigInt, Raw: raw}
}

// FloatValue creates a non-null FLOAT ColumnValue from v.
// The raw bytes are 4-byte IEEE 754 single in big-endian byte order.
func FloatValue(v float32) ColumnValue {
	raw := make([]byte, sizeFloat)
	binary.BigEndian.PutUint32(raw, math.Float32bits(v))
	return ColumnValue{Type: ast.TypeFloat, Raw: raw}
}

// DoubleValue creates a non-null DOUBLE ColumnValue from v.
// The raw bytes are 8-byte IEEE 754 double in big-endian byte order.
func DoubleValue(v float64) ColumnValue {
	raw := make([]byte, sizeDouble)
	binary.BigEndian.PutUint64(raw, math.Float64bits(v))
	return ColumnValue{Type: ast.TypeDouble, Raw: raw}
}

// DecimalValue creates a non-null DECIMAL ColumnValue from v.
// The raw bytes are [2-byte BE uint16 length][UTF-8 bytes], encoding the string representation.
func DecimalValue(v string) ColumnValue {
	b := []byte(v)
	raw := make([]byte, sizeDecimalPrefix+len(b))
	binary.BigEndian.PutUint16(raw, uint16(len(b)))
	copy(raw[sizeDecimalPrefix:], b)
	return ColumnValue{Type: ast.TypeDecimal, Raw: raw}
}

// VarcharValue creates a non-null VARCHAR ColumnValue from v.
// The raw bytes are [2-byte BE uint16 length][UTF-8 bytes].
func VarcharValue(v string) ColumnValue {
	b := []byte(v)
	raw := make([]byte, sizeVarcharPrefix+len(b))
	binary.BigEndian.PutUint16(raw, uint16(len(b)))
	copy(raw[sizeVarcharPrefix:], b)
	return ColumnValue{Type: ast.TypeVarchar, Raw: raw}
}

// BoolValue creates a non-null BOOLEAN ColumnValue from v.
// false → 0x00, true → 0x01.
func BoolValue(v bool) ColumnValue {
	raw := []byte{0x00}
	if v {
		raw[0] = 0x01
	}
	return ColumnValue{Type: ast.TypeBoolean, Raw: raw}
}

// TextValue creates a non-null TEXT ColumnValue from v.
// The raw bytes are [4-byte BE uint32 length][UTF-8 bytes].
func TextValue(v string) ColumnValue {
	b := []byte(v)
	raw := make([]byte, sizeTextPrefix+len(b))
	binary.BigEndian.PutUint32(raw, uint32(len(b)))
	copy(raw[sizeTextPrefix:], b)
	return ColumnValue{Type: ast.TypeText, Raw: raw}
}

// TimestampValue creates a non-null TIMESTAMP ColumnValue from t.
// The raw bytes are 8-byte big-endian int64 holding Unix nanoseconds.
// No sign-bit flip is applied — sort order is irrelevant inside values.
func TimestampValue(t time.Time) ColumnValue {
	raw := make([]byte, sizeTimestamp)
	binary.BigEndian.PutUint64(raw, uint64(t.UnixNano()))
	return ColumnValue{Type: ast.TypeTimestamp, Raw: raw}
}

// NullValue creates a NULL ColumnValue for the given SQL type.
func NullValue(typ ast.DataTypeKind) ColumnValue {
	return ColumnValue{Type: typ, IsNull: true}
}

// AsInt returns the int32 value of an INT column.
func (cv ColumnValue) AsInt() (int32, error) {
	if cv.IsNull {
		return 0, ErrNullAccess
	}
	if cv.Type != ast.TypeInt {
		return 0, ErrTypeMismatch
	}
	if len(cv.Raw) < sizeInt {
		return 0, ErrTruncatedValue
	}
	return int32(binary.BigEndian.Uint32(cv.Raw)), nil
}

// AsBigInt returns the int64 value of a BIGINT column.
func (cv ColumnValue) AsBigInt() (int64, error) {
	if cv.IsNull {
		return 0, ErrNullAccess
	}
	if cv.Type != ast.TypeBigInt {
		return 0, ErrTypeMismatch
	}
	if len(cv.Raw) < sizeBigInt {
		return 0, ErrTruncatedValue
	}
	return int64(binary.BigEndian.Uint64(cv.Raw)), nil
}

// AsString returns the string value of a VARCHAR or TEXT column.
func (cv ColumnValue) AsString() (string, error) {
	if cv.IsNull {
		return "", ErrNullAccess
	}
	switch cv.Type {
	case ast.TypeVarchar:
		if len(cv.Raw) < sizeVarcharPrefix {
			return "", ErrTruncatedValue
		}
		strLen := int(binary.BigEndian.Uint16(cv.Raw[:sizeVarcharPrefix]))
		if len(cv.Raw) < sizeVarcharPrefix+strLen {
			return "", ErrTruncatedValue
		}
		return string(cv.Raw[sizeVarcharPrefix : sizeVarcharPrefix+strLen]), nil
	case ast.TypeText:
		if len(cv.Raw) < sizeTextPrefix {
			return "", ErrTruncatedValue
		}
		strLen := int(binary.BigEndian.Uint32(cv.Raw[:sizeTextPrefix]))
		if len(cv.Raw) < sizeTextPrefix+strLen {
			return "", ErrTruncatedValue
		}
		return string(cv.Raw[sizeTextPrefix : sizeTextPrefix+strLen]), nil
	default:
		return "", ErrTypeMismatch
	}
}

// AsBool returns the boolean value of a BOOLEAN column.
func (cv ColumnValue) AsBool() (bool, error) {
	if cv.IsNull {
		return false, ErrNullAccess
	}
	if cv.Type != ast.TypeBoolean {
		return false, ErrTypeMismatch
	}
	if len(cv.Raw) < sizeBoolean {
		return false, ErrTruncatedValue
	}
	return cv.Raw[0] == 0x01, nil
}

// AsTimestamp returns the time.Time value of a TIMESTAMP column.
func (cv ColumnValue) AsTimestamp() (time.Time, error) {
	if cv.IsNull {
		return time.Time{}, ErrNullAccess
	}
	if cv.Type != ast.TypeTimestamp {
		return time.Time{}, ErrTypeMismatch
	}
	if len(cv.Raw) < sizeTimestamp {
		return time.Time{}, ErrTruncatedValue
	}
	nanos := int64(binary.BigEndian.Uint64(cv.Raw))
	return time.Unix(0, nanos).UTC(), nil
}

// AsFloat returns the float32 value of a FLOAT column.
func (cv ColumnValue) AsFloat() (float32, error) {
	if cv.IsNull {
		return 0, ErrNullAccess
	}
	if cv.Type != ast.TypeFloat {
		return 0, ErrTypeMismatch
	}
	if len(cv.Raw) < sizeFloat {
		return 0, ErrTruncatedValue
	}
	return math.Float32frombits(binary.BigEndian.Uint32(cv.Raw)), nil
}

// AsDouble returns the float64 value of a DOUBLE column.
func (cv ColumnValue) AsDouble() (float64, error) {
	if cv.IsNull {
		return 0, ErrNullAccess
	}
	if cv.Type != ast.TypeDouble {
		return 0, ErrTypeMismatch
	}
	if len(cv.Raw) < sizeDouble {
		return 0, ErrTruncatedValue
	}
	return math.Float64frombits(binary.BigEndian.Uint64(cv.Raw)), nil
}

// AsDecimal returns the string representation of a DECIMAL column.
func (cv ColumnValue) AsDecimal() (string, error) {
	if cv.IsNull {
		return "", ErrNullAccess
	}
	if cv.Type != ast.TypeDecimal {
		return "", ErrTypeMismatch
	}
	if len(cv.Raw) < sizeDecimalPrefix {
		return "", ErrTruncatedValue
	}
	strLen := int(binary.BigEndian.Uint16(cv.Raw[:sizeDecimalPrefix]))
	if len(cv.Raw) < sizeDecimalPrefix+strLen {
		return "", ErrTruncatedValue
	}
	return string(cv.Raw[sizeDecimalPrefix : sizeDecimalPrefix+strLen]), nil
}

// typeTagFromKind maps an ast.DataTypeKind to the wire-format type tag byte.
func typeTagFromKind(kind ast.DataTypeKind) (typeTag, bool) {
	switch kind {
	case ast.TypeInt:
		return tagInt, true
	case ast.TypeBigInt:
		return tagBigInt, true
	case ast.TypeVarchar:
		return tagVarchar, true
	case ast.TypeBoolean:
		return tagBoolean, true
	case ast.TypeText:
		return tagText, true
	case ast.TypeTimestamp:
		return tagTimestamp, true
	case ast.TypeFloat:
		return tagFloat, true
	case ast.TypeDouble:
		return tagDouble, true
	case ast.TypeDecimal:
		return tagDecimal, true
	default:
		return 0, false
	}
}

// kindFromTypeTag maps a wire-format type tag byte back to an ast.DataTypeKind.
func kindFromTypeTag(tag typeTag) (ast.DataTypeKind, bool) {
	switch tag {
	case tagInt:
		return ast.TypeInt, true
	case tagBigInt:
		return ast.TypeBigInt, true
	case tagVarchar:
		return ast.TypeVarchar, true
	case tagBoolean:
		return ast.TypeBoolean, true
	case tagText:
		return ast.TypeText, true
	case tagTimestamp:
		return ast.TypeTimestamp, true
	case tagFloat:
		return ast.TypeFloat, true
	case tagDouble:
		return ast.TypeDouble, true
	case tagDecimal:
		return ast.TypeDecimal, true
	default:
		return 0, false
	}
}

// valueBytesLen returns the number of value_bytes to read for a given type tag.
// For variable-length types, it reads the length prefix from data starting at
// offset and returns the total bytes (prefix + payload). Returns -1 if the
// data is truncated.
func valueBytesLen(tag typeTag, data []byte, offset int) int {
	switch tag {
	case tagInt, tagFloat:
		return sizeInt
	case tagBigInt, tagTimestamp, tagDouble:
		return sizeBigInt
	case tagBoolean:
		return sizeBoolean
	case tagVarchar:
		if offset+sizeVarcharPrefix > len(data) {
			return -1
		}
		strLen := int(binary.BigEndian.Uint16(data[offset : offset+sizeVarcharPrefix]))
		return sizeVarcharPrefix + strLen
	case tagDecimal:
		if offset+sizeDecimalPrefix > len(data) {
			return -1
		}
		strLen := int(binary.BigEndian.Uint16(data[offset : offset+sizeDecimalPrefix]))
		return sizeDecimalPrefix + strLen
	case tagText:
		if offset+sizeTextPrefix > len(data) {
			return -1
		}
		strLen := int(binary.BigEndian.Uint32(data[offset : offset+sizeTextPrefix]))
		return sizeTextPrefix + strLen
	default:
		return -1
	}
}
