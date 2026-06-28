package encoding

import (
	"encoding/binary"
	"math"
)

const (
	// NamespaceSystem is the prefix byte used for all internal catalog and metadata keys.
	// This ensures system keys always sort before user data keys in the KV store.
	NamespaceSystem byte = 0x00

	// NamespaceUser is the prefix byte used for all user table row keys.
	NamespaceUser byte = 0x01
)

// maxNameLen is the maximum length in bytes for a database or table name.
// Names are length-prefixed with a 2-byte big-endian uint16.
const maxNameLen = math.MaxUint16

// EncodeRowKey generates a fully qualified KV store key for a given row.
// It prepends the table prefix produced by EncodeScanPrefix and appends the
// raw primary key bytes.
//
// Row Key Layout:
// +-----------+-------------+----------+------+--------------+-------------+------+---------+
// | Namespace | DB Length   | DB Bytes | 0x00 | Table Length | Table Bytes | 0x00 | PK      |
// | (1 byte)  | (2 bytes BE)| (n bytes)| sep  | (2 bytes BE) | (m bytes)   | sep  | (varies)|
// +-----------+-------------+----------+------+--------------+-------------+------+---------+
func EncodeRowKey(db, table string, pk []byte) (key []byte, err error) {
	prefix, err := EncodeScanPrefix(db, table)
	if err != nil {
		return nil, err
	}
	key = make([]byte, 0, len(prefix)+len(pk))
	key = append(key, prefix...)
	key = append(key, pk...)
	return key, nil
}

// EncodeScanPrefix generates the exact prefix shared by all rows in a given
// table. The returned slice can be used directly for a prefix scan over every
// row in the table.
//
// Scan Prefix Layout:
// +-----------+-------------+----------+------+--------------+-------------+------+
// | Namespace | DB Length   | DB Bytes | 0x00 | Table Length | Table Bytes | 0x00 |
// | (1 byte)  | (2 bytes BE)| (n bytes)| sep  | (2 bytes BE) | (m bytes)   | sep  |
// +-----------+-------------+----------+------+--------------+-------------+------+
func EncodeScanPrefix(db, table string) (buf []byte, err error) {
	if len(db) > maxNameLen {
		return nil, ErrNameTooLong
	}
	if len(table) > maxNameLen {
		return nil, ErrNameTooLong
	}

	dbBytes := []byte(db)
	tableBytes := []byte(table)

	buf = make([]byte, 0, 7+len(dbBytes)+len(tableBytes))
	buf = append(buf, NamespaceUser)

	var lengthBuf [2]byte
	binary.BigEndian.PutUint16(lengthBuf[:], uint16(len(dbBytes)))
	buf = append(buf, lengthBuf[:]...)
	buf = append(buf, dbBytes...)
	buf = append(buf, 0x00)
	binary.BigEndian.PutUint16(lengthBuf[:], uint16(len(tableBytes)))
	buf = append(buf, lengthBuf[:]...)
	buf = append(buf, tableBytes...)
	buf = append(buf, 0x00)

	return buf, nil
}

// DecodeParts parses a user data key into its constituent database name,
// table name, and primary key bytes. It reverses the format created by
// [EncodeRowKey], parsing the length prefixes to safely extract the
// variable-length name segments.
//
// Returns [ErrMalformedKey] if the namespace prefix is wrong or a required
// 0x00 separator byte is missing. Returns [ErrKeyTooShort] if the key is
// truncated before all fields can be read.
func DecodeParts(key []byte) (db, table string, pk []byte, err error) {
	if len(key) == 0 {
		return "", "", nil, ErrKeyTooShort
	}
	if key[0] != NamespaceUser {
		return "", "", nil, ErrMalformedKey
	}

	offset := 1

	if offset+2 > len(key) {
		return "", "", nil, ErrKeyTooShort
	}
	dbLen := int(binary.BigEndian.Uint16(key[offset : offset+2]))
	offset += 2
	if offset+dbLen > len(key) {
		return "", "", nil, ErrKeyTooShort
	}
	db = string(key[offset : offset+dbLen])
	offset += dbLen

	if offset >= len(key) {
		return "", "", nil, ErrKeyTooShort
	}
	if key[offset] != 0x00 {
		return "", "", nil, ErrMalformedKey
	}
	offset++

	if offset+2 > len(key) {
		return "", "", nil, ErrKeyTooShort
	}
	tableLen := int(binary.BigEndian.Uint16(key[offset : offset+2]))
	offset += 2
	if offset+tableLen > len(key) {
		return "", "", nil, ErrKeyTooShort
	}
	table = string(key[offset : offset+tableLen])
	offset += tableLen

	if offset >= len(key) {
		return "", "", nil, ErrKeyTooShort
	}
	if key[offset] != 0x00 {
		return "", "", nil, ErrMalformedKey
	}
	offset++

	pk = key[offset:]
	return db, table, pk, nil
}

// EncodeCatalogDBKey generates a system KV key for database-level metadata.
//
// Catalog DB Key Layout:
// +-----------+--------+-------------+----------+
// | Namespace | Tag    | DB Length   | DB Bytes |
// | (0x00)    | "db\0" | (2 bytes BE)| (n bytes)|
// +-----------+--------+-------------+----------+
func EncodeCatalogDBKey(db string) (buf []byte, err error) {
	if len(db) > maxNameLen {
		return nil, ErrNameTooLong
	}
	buf = make([]byte, 0, 1+3+2+len(db))
	buf = append(buf, NamespaceSystem)
	buf = append(buf, []byte("db\x00")...)
	var lengthBuf [2]byte
	binary.BigEndian.PutUint16(lengthBuf[:], uint16(len(db)))
	buf = append(buf, lengthBuf[:]...)
	buf = append(buf, []byte(db)...)
	return buf, nil
}

// EncodeCatalogTableKey generates a system KV key for table schema metadata.
//
// Catalog Table Key Layout:
// +-----------+---------+-------------+----------+------+--------------+-------------+
// | Namespace | Tag     | DB Length   | DB Bytes | 0x00 | Table Length | Table Bytes |
// | (0x00)    | "tbl\0" | (2 bytes BE)| (n bytes)| sep  | (2 bytes BE) | (m bytes)   |
// +-----------+---------+-------------+----------+------+--------------+-------------+
func EncodeCatalogTableKey(db, table string) (key []byte, err error) {
	return encodeCatalogCompoundKey("tbl\x00", db, table)
}

// EncodeCatalogSeqKey generates a system KV key for auto-increment sequence
// counters.
//
// Catalog Seq Key Layout:
// +-----------+---------+-------------+----------+------+--------------+-------------+
// | Namespace | Tag     | DB Length   | DB Bytes | 0x00 | Table Length | Table Bytes |
// | (0x00)    | "seq\0" | (2 bytes BE)| (n bytes)| sep  | (2 bytes BE) | (m bytes)   |
// +-----------+---------+-------------+----------+------+--------------+-------------+
func EncodeCatalogSeqKey(db, table string) (key []byte, err error) {
	return encodeCatalogCompoundKey("seq\x00", db, table)
}

// encodeCatalogCompoundKey builds a system catalog key composed of a namespace
// byte, a tag prefix, and two length-prefixed name segments separated by 0x00.
// This is the shared implementation behind EncodeCatalogTableKey and
// EncodeCatalogSeqKey.
func encodeCatalogCompoundKey(tag, db, table string) (buf []byte, err error) {
	if len(db) > maxNameLen {
		return nil, ErrNameTooLong
	}
	if len(table) > maxNameLen {
		return nil, ErrNameTooLong
	}
	buf = make([]byte, 0, 1+len(tag)+2+len(db)+1+2+len(table))
	buf = append(buf, NamespaceSystem)
	buf = append(buf, []byte(tag)...)
	var lengthBuf [2]byte
	binary.BigEndian.PutUint16(lengthBuf[:], uint16(len(db)))
	buf = append(buf, lengthBuf[:]...)
	buf = append(buf, []byte(db)...)
	buf = append(buf, 0x00)
	binary.BigEndian.PutUint16(lengthBuf[:], uint16(len(table)))
	buf = append(buf, lengthBuf[:]...)
	buf = append(buf, []byte(table)...)
	return buf, nil
}

// CatalogDBScanPrefix returns the prefix byte sequence that covers all
// database metadata system keys.
func CatalogDBScanPrefix() (prefix []byte) {
	return []byte{NamespaceSystem, 'd', 'b', 0x00}
}

// CatalogTableScanPrefix returns the prefix byte sequence that covers all
// table metadata system keys.
func CatalogTableScanPrefix() (prefix []byte) {
	return []byte{NamespaceSystem, 't', 'b', 'l', 0x00}
}
