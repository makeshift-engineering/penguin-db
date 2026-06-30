package encoding

import "errors"

// Sentinel errors returned by the encoding package.
var (
	// ErrNulInString is returned when a string value to be encoded contains an interior NUL byte,
	// which is prohibited as NUL is used as a delimiter for variable-width fields.
	ErrNulInString = errors.New("encoding: string value contains a NUL byte")

	// ErrNaNNotAllowed is returned when attempting to encode a NaN float64 value.
	// NaN values are not supported in sortable keys as they break total ordering.
	ErrNaNNotAllowed = errors.New("encoding: float value is NaN")

	// ErrKeyTooShort is returned when decoding fails because the provided byte slice is shorter
	// than expected for the data type being decoded.
	ErrKeyTooShort = errors.New("encoding: key is too short or malformed")

	// ErrMalformedKey is returned when a key is structurally invalid — for example,
	// it has an incorrect namespace prefix or is missing a required separator byte.
	ErrMalformedKey = errors.New("encoding: key is structurally malformed")

	// ErrNameTooLong is returned when a database or table name exceeds the maximum
	// length that can be represented in a 2-byte big-endian length prefix (65535 bytes).
	ErrNameTooLong = errors.New("encoding: name exceeds maximum length of 65535 bytes")

	// ErrInvalidPK is returned when the number of types provided for decoding a primary key
	// does not match the encoded data, or if an unsupported data type is encountered.
	ErrInvalidPK = errors.New("encoding: invalid primary key type mismatch")
)
