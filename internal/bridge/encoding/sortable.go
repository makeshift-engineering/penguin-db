package encoding

import (
	"bytes"
	"encoding/binary"
	"math"
	"time"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

// EncodeInt32 encodes an int32 into a 4-byte slice such that lexicographical byte comparison
// corresponds to numerical comparison. This is achieved by XORing the sign bit with 1, which maps
// the signed domain [-2^31, 2^31 - 1] to the unsigned domain [0, 2^32 - 1] before writing as big-endian.
func EncodeInt32(v int32) []byte {
	u := uint32(v) ^ 0x80000000
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, u)
	return b
}

// DecodeInt32 decodes a 4-byte slice produced by EncodeInt32 back into an int32.
// It reads the big-endian uint32 and XORs the sign bit to restore the original signed value.
func DecodeInt32(b []byte) int32 {
	if len(b) < 4 {
		return 0
	}
	u := binary.BigEndian.Uint32(b)
	return int32(u ^ 0x80000000)
}

// EncodeInt64 encodes an int64 into an 8-byte slice such that lexicographical byte comparison
// matches numerical comparison. It XORs the most significant bit (sign bit) before big-endian encoding.
func EncodeInt64(v int64) []byte {
	u := uint64(v) ^ 0x8000000000000000
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, u)
	return b
}

// DecodeInt64 decodes an 8-byte slice produced by EncodeInt64 back into an int64.
// It restores the original value by applying the reverse XOR operation on the most significant bit.
func DecodeInt64(b []byte) int64 {
	if len(b) < 8 {
		return 0
	}
	u := binary.BigEndian.Uint64(b)
	return int64(u ^ 0x8000000000000000)
}

// EncodeFloat64 encodes a float64 into an 8-byte slice preserving numeric sort order.
// Standard IEEE 754 float bytes do not sort correctly for negative numbers. The encoding fixes this by:
// 1. If the number is negative (sign bit = 1), XOR all 64 bits to invert the value.
// 2. If the number is positive (sign bit = 0), XOR only the sign bit.
// NaN values are explicitly rejected and will return ErrNaNNotAllowed.
func EncodeFloat64(v float64) ([]byte, error) {
	if math.IsNaN(v) {
		return nil, ErrNaNNotAllowed
	}
	u := math.Float64bits(v)
	if (u & 0x8000000000000000) != 0 {
		u = u ^ 0xFFFFFFFFFFFFFFFF
	} else {
		u = u ^ 0x8000000000000000
	}
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, u)
	return b, nil
}

// DecodeFloat64 decodes an 8-byte slice produced by EncodeFloat64 back into a float64.
// It examines the sign bit of the encoded bytes to determine whether to invert all bits
// (if original was negative) or just the sign bit (if original was positive) before interpreting as IEEE 754.
func DecodeFloat64(b []byte) float64 {
	if len(b) < 8 {
		return 0
	}
	u := binary.BigEndian.Uint64(b)
	if (u & 0x8000000000000000) != 0 {
		u = u ^ 0x8000000000000000
	} else {
		u = u ^ 0xFFFFFFFFFFFFFFFF
	}
	return math.Float64frombits(u)
}

// EncodeString encodes a UTF-8 string into a byte slice, appending a single NUL (0x00) terminator byte.
// The terminator makes variable-length strings self-delimiting within composite keys.
// If the input string contains an interior NUL byte, this function returns ErrNulInString.
func EncodeString(v string) ([]byte, error) {
	b := []byte(v)
	if bytes.IndexByte(b, 0x00) >= 0 {
		return nil, ErrNulInString
	}
	return append(b, 0x00), nil
}

// DecodeString extracts a string from a NUL-terminated byte slice produced by EncodeString.
// It scans for the first 0x00 byte, returning the string up to that point. The terminator is discarded.
func DecodeString(b []byte) (string, error) {
	idx := bytes.IndexByte(b, 0x00)
	if idx < 0 {
		return "", ErrKeyTooShort
	}
	return string(b[:idx]), nil
}

// EncodeBool encodes a boolean into a single byte.
// False is represented as 0x00 and True is represented as 0x01, ensuring False sorts before True.
func EncodeBool(v bool) []byte {
	if v {
		return []byte{0x01}
	}
	return []byte{0x00}
}

// DecodeBool decodes a boolean from a single byte.
// Returns true if the byte is 0x01, otherwise false.
func DecodeBool(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	return b[0] == 0x01
}

// EncodeTimestamp encodes a time.Time into an 8-byte slice by converting it to Unix nanoseconds
// and applying the same sort-preserving sign-flip technique used by EncodeInt64.
func EncodeTimestamp(t time.Time) []byte {
	return EncodeInt64(t.UnixNano())
}

// DecodeTimestamp decodes an 8-byte slice produced by EncodeTimestamp back into a time.Time.
// The decoded int64 represents Unix nanoseconds, which is parsed as a UTC timestamp.
func DecodeTimestamp(b []byte) time.Time {
	nanos := DecodeInt64(b)
	return time.Unix(0, nanos).UTC()
}

// EncodePK iteratively encodes a sequence of primitive values into a single composite byte slice
// representing a Primary Key. It uses the provided AST data type kinds to dispatch to the correct
// sort-preserving type encoder for each column value.
func EncodePK(cols []ast.DataTypeKind, vals []any) ([]byte, error) {
	if len(cols) != len(vals) {
		return nil, ErrInvalidPK
	}
	var out []byte
	for i, kind := range cols {
		val := vals[i]
		switch kind {
		case ast.TypeInt:
			v, ok := val.(int32)
			if !ok {
				return nil, ErrInvalidPK
			}
			out = append(out, EncodeInt32(v)...)
		case ast.TypeBigInt:
			v, ok := val.(int64)
			if !ok {
				return nil, ErrInvalidPK
			}
			out = append(out, EncodeInt64(v)...)
		case ast.TypeVarchar, ast.TypeText:
			v, ok := val.(string)
			if !ok {
				return nil, ErrInvalidPK
			}
			b, err := EncodeString(v)
			if err != nil {
				return nil, err
			}
			out = append(out, b...)
		case ast.TypeBoolean:
			v, ok := val.(bool)
			if !ok {
				return nil, ErrInvalidPK
			}
			out = append(out, EncodeBool(v)...)
		case ast.TypeTimestamp:
			v, ok := val.(time.Time)
			if !ok {
				return nil, ErrInvalidPK
			}
			out = append(out, EncodeTimestamp(v)...)
		default:
			return nil, ErrInvalidPK
		}
	}
	return out, nil
}

// DecodePK reads a composite byte slice produced by EncodePK and reconstructs the sequence of column values.
// It relies on fixed-width advances for integers/floats/booleans/timestamps and scans for NUL terminators
// for variable-width types (VARCHAR/TEXT).
func DecodePK(cols []ast.DataTypeKind, pk []byte) ([]any, error) {
	var vals []any
	offset := 0
	for _, kind := range cols {
		if offset >= len(pk) {
			return nil, ErrKeyTooShort
		}
		switch kind {
		case ast.TypeInt:
			if offset+4 > len(pk) {
				return nil, ErrKeyTooShort
			}
			vals = append(vals, DecodeInt32(pk[offset:offset+4]))
			offset += 4
		case ast.TypeBigInt:
			if offset+8 > len(pk) {
				return nil, ErrKeyTooShort
			}
			vals = append(vals, DecodeInt64(pk[offset:offset+8]))
			offset += 8
		case ast.TypeBoolean:
			if offset+1 > len(pk) {
				return nil, ErrKeyTooShort
			}
			vals = append(vals, DecodeBool(pk[offset:offset+1]))
			offset += 1
		case ast.TypeTimestamp:
			if offset+8 > len(pk) {
				return nil, ErrKeyTooShort
			}
			vals = append(vals, DecodeTimestamp(pk[offset:offset+8]))
			offset += 8
		case ast.TypeVarchar, ast.TypeText:
			idx := bytes.IndexByte(pk[offset:], 0x00)
			if idx < 0 {
				return nil, ErrKeyTooShort
			}
			str, err := DecodeString(pk[offset : offset+idx+1])
			if err != nil {
				return nil, err
			}
			vals = append(vals, str)
			offset += idx + 1
		default:
			return nil, ErrInvalidPK
		}
	}
	if offset != len(pk) {
		return nil, ErrInvalidPK
	}
	return vals, nil
}
