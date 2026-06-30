package codec

import "errors"

// Sentinel errors returned by the codec package.
var (
	// ErrUnknownCodecVersion is returned when the codec_version byte in the
	// row header is not a recognized version. Currently only 0x01 is supported.
	ErrUnknownCodecVersion = errors.New("codec: unknown codec version")

	// ErrTruncatedValue is returned when the byte slice ends before all
	// columns described by the col_count header could be fully decoded.
	ErrTruncatedValue = errors.New("codec: value is truncated")

	// ErrTypeMismatch is returned when a typed accessor (e.g. AsInt) is
	// called on a ColumnValue whose Type does not match the requested type.
	ErrTypeMismatch = errors.New("codec: type mismatch")

	// ErrNullAccess is returned when a typed accessor is called on a
	// ColumnValue that is NULL.
	ErrNullAccess = errors.New("codec: cannot read a NULL value")

	// ErrStringTooLong is returned when a VARCHAR value exceeds 65535 bytes
	// or a TEXT value exceeds 4GB.
	ErrStringTooLong = errors.New("codec: string value exceeds maximum length")

	// ErrUnknownTypeTag is returned during decoding when a type_tag byte
	// does not correspond to any known SQL type.
	ErrUnknownTypeTag = errors.New("codec: unknown type tag")
)
