package encoding

import (
	"encoding/binary"
)

const (
	// NamespaceSystem is the prefix byte used for all internal catalog and metadata keys.
	// This ensures system keys always sort before user data keys in the KV store.
	NamespaceSystem byte = 0x00

	// NamespaceUser is the prefix byte used for all user table row keys.
	NamespaceUser   byte = 0x01
)

// EncodeRowKey generates a fully qualified KV store key for a given row.
// The layout is: [NamespaceUser (1 byte)] + [DB Length (2 bytes BE)] + [DB Bytes] + [0x00] +
// [Table Length (2 bytes BE)] + [Table Bytes] + [0x00] + [Primary Key Bytes].
// This structure guarantees that keys from different tables or databases never overlap.
func EncodeRowKey(db, table string, pk []byte) []byte {
	prefix := EncodeScanPrefix(db, table)
	var key []byte
	key = append(key, prefix...)
	key = append(key, pk...)
	return key
}

// EncodeScanPrefix generates the exact prefix shared by all rows in a given table.
// The returned byte slice corresponds to: [NamespaceUser] + [DB Length] + [DB Bytes] + [0x00] +
// [Table Length] + [Table Bytes] + [0x00]. It can be used directly for a prefix scan over a table.
func EncodeScanPrefix(db, table string) []byte {
	var buf []byte
	buf = append(buf, NamespaceUser)

	dbBytes := []byte(db)
	var lengthBuf [2]byte
	binary.BigEndian.PutUint16(lengthBuf[:], uint16(len(dbBytes)))
	buf = append(buf, lengthBuf[:]...)
	buf = append(buf, dbBytes...)
	buf = append(buf, 0x00)

	tableBytes := []byte(table)
	binary.BigEndian.PutUint16(lengthBuf[:], uint16(len(tableBytes)))
	buf = append(buf, lengthBuf[:]...)
	buf = append(buf, tableBytes...)
	buf = append(buf, 0x00)

	return buf
}

// DecodeParts parses a user data key into its constituent database name, table name, and primary key bytes.
// It reverses the format created by EncodeRowKey, parsing the length prefixes to safely extract the variable-length name segments.
func DecodeParts(key []byte) (db, table string, pk []byte, err error) {
	if len(key) == 0 || key[0] != NamespaceUser {
		return "", "", nil, ErrKeyTooShort
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

	if offset >= len(key) || key[offset] != 0x00 {
		return "", "", nil, ErrKeyTooShort
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

	if offset >= len(key) || key[offset] != 0x00 {
		return "", "", nil, ErrKeyTooShort
	}
	offset++

	pk = key[offset:]
	return db, table, pk, nil
}

// EncodeCatalogDBKey generates a system KV key for database-level metadata.
// Format: [NamespaceSystem (0x00)] + "db\x00" + [DB Length (2 bytes BE)] + [DB Bytes].
func EncodeCatalogDBKey(db string) []byte {
	var buf []byte
	buf = append(buf, NamespaceSystem)
	buf = append(buf, []byte("db\x00")...)
	var lengthBuf [2]byte
	binary.BigEndian.PutUint16(lengthBuf[:], uint16(len(db)))
	buf = append(buf, lengthBuf[:]...)
	buf = append(buf, []byte(db)...)
	return buf
}

// EncodeCatalogTableKey generates a system KV key for table schema metadata.
// Format: [NamespaceSystem (0x00)] + "tbl\x00" + [DB Length] + [DB Bytes] + [0x00] + [Table Length] + [Table Bytes].
func EncodeCatalogTableKey(db, table string) []byte {
	var buf []byte
	buf = append(buf, NamespaceSystem)
	buf = append(buf, []byte("tbl\x00")...)
	var lengthBuf [2]byte
	binary.BigEndian.PutUint16(lengthBuf[:], uint16(len(db)))
	buf = append(buf, lengthBuf[:]...)
	buf = append(buf, []byte(db)...)
	buf = append(buf, 0x00)
	binary.BigEndian.PutUint16(lengthBuf[:], uint16(len(table)))
	buf = append(buf, lengthBuf[:]...)
	buf = append(buf, []byte(table)...)
	return buf
}

// EncodeCatalogSeqKey generates a system KV key for auto-increment sequence counters.
// Format: [NamespaceSystem (0x00)] + "seq\x00" + [DB Length] + [DB Bytes] + [0x00] + [Table Length] + [Table Bytes].
func EncodeCatalogSeqKey(db, table string) []byte {
	var buf []byte
	buf = append(buf, NamespaceSystem)
	buf = append(buf, []byte("seq\x00")...)
	var lengthBuf [2]byte
	binary.BigEndian.PutUint16(lengthBuf[:], uint16(len(db)))
	buf = append(buf, lengthBuf[:]...)
	buf = append(buf, []byte(db)...)
	buf = append(buf, 0x00)
	binary.BigEndian.PutUint16(lengthBuf[:], uint16(len(table)))
	buf = append(buf, lengthBuf[:]...)
	buf = append(buf, []byte(table)...)
	return buf
}

// CatalogDBScanPrefix returns the prefix byte sequence that covers all database metadata system keys.
func CatalogDBScanPrefix() []byte {
	return []byte{NamespaceSystem, 'd', 'b', 0x00}
}

// CatalogTableScanPrefix returns the prefix byte sequence that covers all table metadata system keys.
func CatalogTableScanPrefix() []byte {
	return []byte{NamespaceSystem, 't', 'b', 'l', 0x00}
}
