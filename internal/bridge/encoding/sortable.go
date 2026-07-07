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
func EncodeInt32(v int32) (b []byte) {
	u := uint32(v) ^ 0x80000000
	b = make([]byte, 4)
	binary.BigEndian.PutUint32(b, u)
	return b
}

// DecodeInt32 decodes a 4-byte slice produced by EncodeInt32 back into an int32.
// It reads the big-endian uint32 and XORs the sign bit to restore the original signed value.
func DecodeInt32(b []byte) (v int32) {
	if len(b) < 4 {
		return 0
	}
	u := binary.BigEndian.Uint32(b)
	return int32(u ^ 0x80000000)
}

// EncodeInt64 encodes an int64 into an 8-byte slice such that lexicographical byte comparison
// matches numerical comparison. It XORs the most significant bit (sign bit) before big-endian encoding.
func EncodeInt64(v int64) (b []byte) {
	u := uint64(v) ^ 0x8000000000000000
	b = make([]byte, 8)
	binary.BigEndian.PutUint64(b, u)
	return b
}

// DecodeInt64 decodes an 8-byte slice produced by EncodeInt64 back into an int64.
// It restores the original value by applying the reverse XOR operation on the most significant bit.
func DecodeInt64(b []byte) (v int64) {
	if len(b) < 8 {
		return 0
	}
	u := binary.BigEndian.Uint64(b)
	return int64(u ^ 0x8000000000000000)
}

// EncodeFloat32 encodes a float32 into a 4-byte slice preserving numeric sort order.
// It uses the same sign-flipping logic as EncodeFloat64.
func EncodeFloat32(v float32) (b []byte, err error) {
	if math.IsNaN(float64(v)) {
		return nil, ErrNaNNotAllowed
	}
	if v == 0 {
		v = 0.0 // Normalize -0.0 to 0.0
	}
	u := math.Float32bits(v)
	if (u & 0x80000000) != 0 {
		u ^= 0xFFFFFFFF
	} else {
		u ^= 0x80000000
	}
	b = make([]byte, 4)
	binary.BigEndian.PutUint32(b, u)
	return b, nil
}

// DecodeFloat32 decodes a 4-byte slice produced by EncodeFloat32 back into a float32.
func DecodeFloat32(b []byte) (v float32) {
	if len(b) < 4 {
		return 0
	}
	u := binary.BigEndian.Uint32(b)
	if (u & 0x80000000) != 0 {
		u ^= 0x80000000
	} else {
		u ^= 0xFFFFFFFF
	}
	return math.Float32frombits(u)
}

// EncodeFloat64 encodes a float64 into an 8-byte slice preserving numeric sort order.
// Standard IEEE 754 float bytes do not sort correctly for negative numbers. The encoding fixes this by:
// 1. If the number is negative (sign bit = 1), XOR all 64 bits to invert the value.
// 2. If the number is positive (sign bit = 0), XOR only the sign bit.
// NaN values are explicitly rejected and will return ErrNaNNotAllowed.
func EncodeFloat64(v float64) (b []byte, err error) {
	if math.IsNaN(v) {
		return nil, ErrNaNNotAllowed
	}
	if v == 0 {
		v = 0.0 // Normalize -0.0 to 0.0
	}
	u := math.Float64bits(v)
	if (u & 0x8000000000000000) != 0 {
		u ^= 0xFFFFFFFFFFFFFFFF
	} else {
		u ^= 0x8000000000000000
	}
	b = make([]byte, 8)
	binary.BigEndian.PutUint64(b, u)
	return b, nil
}

// DecodeFloat64 decodes an 8-byte slice produced by EncodeFloat64 back into a float64.
// It examines the sign bit of the encoded bytes to determine whether to invert all bits
// (if original was negative) or just the sign bit (if original was positive) before interpreting as IEEE 754.
func DecodeFloat64(b []byte) (v float64) {
	if len(b) < 8 {
		return 0
	}
	u := binary.BigEndian.Uint64(b)
	if (u & 0x8000000000000000) != 0 {
		u ^= 0x8000000000000000
	} else {
		u ^= 0xFFFFFFFFFFFFFFFF
	}
	return math.Float64frombits(u)
}

// EncodeString encodes a UTF-8 string into a byte slice, appending a single NUL (0x00) terminator byte.
// The terminator makes variable-length strings self-delimiting within composite keys.
// If the input string contains an interior NUL byte, this function returns ErrNulInString.
func EncodeString(v string) (out []byte, err error) {
	b := []byte(v)
	if bytes.IndexByte(b, 0x00) >= 0 {
		return nil, ErrNulInString
	}
	return append(b, 0x00), nil
}

// DecodeString extracts a string from a NUL-terminated byte slice produced by EncodeString.
// It scans for the first 0x00 byte, returning the string up to that point. The terminator is discarded.
func DecodeString(b []byte) (str string, err error) {
	idx := bytes.IndexByte(b, 0x00)
	if idx < 0 {
		return "", ErrKeyTooShort
	}
	return string(b[:idx]), nil
}

// EncodeBool encodes a boolean into a single byte.
// False is represented as 0x00 and True is represented as 0x01, ensuring False sorts before True.
func EncodeBool(v bool) (b []byte) {
	if v {
		return []byte{0x01}
	}
	return []byte{0x00}
}

// DecodeBool decodes a boolean from a single byte.
// Returns true if the byte is 0x01, otherwise false.
func DecodeBool(b []byte) (v bool) {
	if len(b) == 0 {
		return false
	}
	return b[0] == 0x01
}

// EncodeTimestamp encodes a time.Time into an 8-byte slice by converting it to Unix nanoseconds
// and applying the same sort-preserving sign-flip technique used by EncodeInt64.
func EncodeTimestamp(t time.Time) (b []byte) {
	return EncodeInt64(t.UnixNano())
}

// DecodeTimestamp decodes an 8-byte slice produced by EncodeTimestamp back into a time.Time.
// The decoded int64 represents Unix nanoseconds, which is parsed as a UTC timestamp.
func DecodeTimestamp(b []byte) (t time.Time) {
	nanos := DecodeInt64(b)
	return time.Unix(0, nanos).UTC()
}

// EncodePK iteratively encodes a sequence of primitive values into a single composite byte slice
// representing a Primary Key. It uses the provided AST data type kinds to dispatch to the correct
// sort-preserving type encoder for each column value.
func EncodePK(cols []ast.DataTypeKind, vals []any) (out []byte, err error) {
	if len(cols) != len(vals) {
		return nil, ErrInvalidPK
	}
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
		case ast.TypeFloat:
			v, ok := val.(float32)
			if !ok {
				return nil, ErrInvalidPK
			}
			b, err := EncodeFloat32(v)
			if err != nil {
				return nil, err
			}
			out = append(out, b...)
		case ast.TypeDouble:
			v, ok := val.(float64)
			if !ok {
				return nil, ErrInvalidPK
			}
			b, err := EncodeFloat64(v)
			if err != nil {
				return nil, err
			}
			out = append(out, b...)
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
		case ast.TypeDecimal:
			v, ok := val.(string)
			if !ok {
				return nil, ErrInvalidPK
			}
			b, err := EncodeDecimal(v)
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
func DecodePK(cols []ast.DataTypeKind, pk []byte) (vals []any, err error) {
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
			offset++
		case ast.TypeTimestamp:
			if offset+8 > len(pk) {
				return nil, ErrKeyTooShort
			}
			vals = append(vals, DecodeTimestamp(pk[offset:offset+8]))
			offset += 8
		case ast.TypeFloat:
			if offset+4 > len(pk) {
				return nil, ErrKeyTooShort
			}
			vals = append(vals, DecodeFloat32(pk[offset:offset+4]))
			offset += 4
		case ast.TypeDouble:
			if offset+8 > len(pk) {
				return nil, ErrKeyTooShort
			}
			vals = append(vals, DecodeFloat64(pk[offset:offset+8]))
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
		case ast.TypeDecimal:
			if offset+decimalEncodedLen > len(pk) {
				return nil, ErrKeyTooShort
			}
			str, err := DecodeDecimal(pk[offset : offset+decimalEncodedLen])
			if err != nil {
				return nil, err
			}
			vals = append(vals, str)
			offset += decimalEncodedLen
		default:
			return nil, ErrInvalidPK
		}
	}
	if offset != len(pk) {
		return nil, ErrInvalidPK
	}
	return vals, nil
}

const decimalPrecision = 64
const decimalEncodedLen = 1 + decimalPrecision + decimalPrecision

// EncodeDecimal encodes a decimal string into a fixed-width byte slice
// that preserves numeric sort order lexicographically.
func EncodeDecimal(v string) ([]byte, error) {
	if v == "" {
		return nil, ErrInvalidDecimal
	}

	isNeg := false
	switch v[0] {
	case '-':
		isNeg = true
		v = v[1:]
	case '+':
		v = v[1:]
	}

	if v == "" {
		return nil, ErrInvalidDecimal
	}

	var intPart, fracPart string
	dotIdx := -1
	hasDigit := false
	for i := 0; i < len(v); i++ {
		switch {
		case v[i] == '.':
			if dotIdx != -1 {
				return nil, ErrInvalidDecimal // multiple dots
			}
			dotIdx = i
		case v[i] < '0' || v[i] > '9':
			return nil, ErrInvalidDecimal // invalid char
		default:
			hasDigit = true
		}
	}

	if !hasDigit {
		return nil, ErrInvalidDecimal
	}

	if dotIdx == -1 {
		intPart = v
	} else {
		intPart = v[:dotIdx]
		fracPart = v[dotIdx+1:]
	}

	// Trim leading zeros from int part
	start := 0
	for start < len(intPart) && intPart[start] == '0' {
		start++
	}
	intPart = intPart[start:]
	if intPart == "" {
		intPart = "0"
	}

	// Trim trailing zeros from frac part
	end := len(fracPart)
	for end > 0 && fracPart[end-1] == '0' {
		end--
	}
	fracPart = fracPart[:end]

	if intPart == "0" && fracPart == "" {
		isNeg = false // normalize -0 to +0
	}

	if len(intPart) > decimalPrecision {
		return nil, ErrDecimalTooLarge
	}
	if len(fracPart) > decimalPrecision {
		return nil, ErrDecimalTooLarge
	}

	out := make([]byte, decimalEncodedLen)

	if isNeg {
		out[0] = 0x00
	} else {
		out[0] = 0x01
	}

	// Fill integer part with leading zeros
	intOffset := 1
	padInt := decimalPrecision - len(intPart)
	for i := range padInt {
		out[intOffset+i] = '0'
	}
	copy(out[intOffset+padInt:], intPart)

	// Fill fractional part with trailing zeros
	fracOffset := 1 + decimalPrecision
	copy(out[fracOffset:], fracPart)
	padFrac := decimalPrecision - len(fracPart)
	for i := range padFrac {
		out[fracOffset+len(fracPart)+i] = '0'
	}

	// Invert digits if negative to correct sort order
	if isNeg {
		for i := 1; i < decimalEncodedLen; i++ {
			out[i] = '9' - (out[i] - '0')
		}
	}

	return out, nil
}

// DecodeDecimal extracts a canonical decimal string from its sortable encoding.
func DecodeDecimal(b []byte) (string, error) {
	if len(b) < decimalEncodedLen {
		return "", ErrKeyTooShort
	}

	isNeg := b[0] == 0x00

	intPartBytes := make([]byte, decimalPrecision)
	copy(intPartBytes, b[1:1+decimalPrecision])

	fracPartBytes := make([]byte, decimalPrecision)
	copy(fracPartBytes, b[1+decimalPrecision:decimalEncodedLen])

	if isNeg {
		for i := range decimalPrecision {
			intPartBytes[i] = '9' - (intPartBytes[i] - '0')
			fracPartBytes[i] = '9' - (fracPartBytes[i] - '0')
		}
	}

	// Trim leading zeros
	start := 0
	for start < decimalPrecision && intPartBytes[start] == '0' {
		start++
	}
	intPart := string(intPartBytes[start:])
	if intPart == "" {
		intPart = "0"
	}

	// Trim trailing zeros
	end := decimalPrecision
	for end > 0 && fracPartBytes[end-1] == '0' {
		end--
	}
	fracPart := string(fracPartBytes[:end])

	if fracPart == "" {
		if isNeg && intPart != "0" {
			return "-" + intPart, nil
		}
		return intPart, nil
	}

	if isNeg {
		return "-" + intPart + "." + fracPart, nil
	}
	return intPart + "." + fracPart, nil
}
