// Package codec implements binary row serialization for PenguinDB's KV store.
//
// The wire format is designed for forward and backward compatibility across
// schema changes. Each encoded row carries a codec_version byte, a column
// count, and self-describing column entries (null flag + type tag + value
// bytes). Type tags are embedded in the value so that a decoder can skip
// unknown or dropped columns without consulting the catalog.
//
// Wire format overview:
// +----------------+---------------+---------------------+
// | codec_version  | col_count     | column_1 … column_N |
// | (1 byte)       | (2 bytes BE)  | (variable)          |
// +----------------+---------------+---------------------+
//
// Each column entry:
// +------------+-----------+--------------------------------+
// | null_flag  | type_tag  | value_bytes (absent when null) |
// | (1 byte)   | (1 byte)  | (type-determined length)       |
// +------------+-----------+--------------------------------+
package codec

import (
	"encoding/binary"
)

// codecVersion is the current on-disk format version byte. It is incremented
// only when the binary layout changes in a backward-incompatible way.
const codecVersion uint8 = 0x01

const (
	headerSize    = 3 // 1 byte version + 2 bytes colCount
	colHeaderSize = 2 // 1 byte null_flag + 1 byte type_tag
)

// null flag bytes
const (
	nullFlagNotNull byte = 0x00
	nullFlagNull    byte = 0x01
)

// Row is the result of decoding one KV value. CodecVersion records the format
// version from the row header. Values contains the decoded columns in
// declaration order.
type Row struct {
	CodecVersion uint8
	Values       []ColumnValue
}

// Encode serializes a Row into the binary wire format.
//
// Returns [ErrStringTooLong] if a VARCHAR exceeds 65535 bytes or a TEXT value
// exceeds the 4 GB limit. Returns [ErrUnknownTypeTag] if a column has an
// unrecognized DataTypeKind.
func Encode(row *Row) ([]byte, error) {
	// Pre-compute an estimated capacity for the output buffer.
	// Header size + estimated 10 bytes per column is a reasonable guess.
	colCount := len(row.Values)
	buf := make([]byte, 0, headerSize+colCount*10)

	buf = append(buf, row.CodecVersion)
	var countBuf [2]byte
	binary.BigEndian.PutUint16(countBuf[:], uint16(colCount))
	buf = append(buf, countBuf[:]...)

	for _, cv := range row.Values {

		tag, ok := typeTagFromKind(cv.Type)
		if !ok {
			return nil, ErrUnknownTypeTag
		}

		if cv.IsNull {
			buf = append(buf, nullFlagNull, byte(tag))
			continue
		}

		buf = append(buf, nullFlagNotNull, byte(tag))

		// Validate string lengths before writing.
		switch tag {
		case tagVarchar:
			if len(cv.Raw) < sizeVarcharPrefix {
				return nil, ErrTruncatedValue
			}
			if len(cv.Raw) > maxVarcharLen+sizeVarcharPrefix {
				return nil, ErrStringTooLong
			}
		case tagDecimal:
			if len(cv.Raw) < sizeDecimalPrefix {
				return nil, ErrTruncatedValue
			}
			if len(cv.Raw) > maxVarcharLen+sizeDecimalPrefix {
				return nil, ErrStringTooLong
			}
		case tagText:
			if len(cv.Raw) < sizeTextPrefix {
				return nil, ErrTruncatedValue
			}
			if int64(len(cv.Raw)) > int64(maxTextLen)+sizeTextPrefix {
				return nil, ErrStringTooLong
			}
		}

		buf = append(buf, cv.Raw...)
	}

	return buf, nil
}

// Decode deserializes a byte slice into a Row. It reads the codec_version
// header, verifies it, then decodes col_count column entries.
//
// Each column entry is self-describing: the type_tag tells the decoder how
// many value bytes to consume (or, for variable-width types, where to find
// the length prefix). This allows decoding to proceed even when the schema
// has changed since the row was written.
//
// Decode returns [ErrUnknownCodecVersion] if the version byte is not 0x01,
// [ErrTruncatedValue] if the data ends before all columns are read, or
// [ErrUnknownTypeTag] if a column's type tag is unrecognized.
func Decode(data []byte) (*Row, error) {
	if len(data) < headerSize {
		return nil, ErrTruncatedValue
	}

	version := data[0]
	if version != codecVersion {
		return nil, ErrUnknownCodecVersion
	}

	colCount := int(binary.BigEndian.Uint16(data[1:3]))
	offset := headerSize

	values := make([]ColumnValue, 0, colCount)

	for range colCount {
		if offset+colHeaderSize > len(data) {
			return nil, ErrTruncatedValue
		}

		nullFlag := data[offset]
		tag := typeTag(data[offset+1])
		offset += colHeaderSize

		kind, ok := kindFromTypeTag(tag)
		if !ok {
			return nil, ErrUnknownTypeTag
		}

		if nullFlag == nullFlagNull {
			values = append(values, ColumnValue{
				Type:   kind,
				IsNull: true,
			})
			continue
		}

		// Determine how many value bytes to read.
		nbytes := valueBytesLen(tag, data, offset)
		if nbytes < 0 {
			return nil, ErrTruncatedValue
		}
		if offset+nbytes > len(data) {
			return nil, ErrTruncatedValue
		}

		// Copy the raw bytes so the returned Row does not alias the input.
		raw := make([]byte, nbytes)
		copy(raw, data[offset:offset+nbytes])
		offset += nbytes

		values = append(values, ColumnValue{
			Type: kind,
			Raw:  raw,
		})
	}

	return &Row{
		CodecVersion: version,
		Values:       values,
	}, nil
}
