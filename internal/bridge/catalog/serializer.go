package catalog

import "encoding/json"

// encodeTableMeta serializes a TableMeta to JSON bytes for storage in the
// KV store. The encoding is wrapped in a helper so the JSON format can be
// versioned independently of the Go types.
func encodeTableMeta(meta *TableMeta) ([]byte, error) {
	return json.Marshal(meta)
}

// decodeTableMeta deserializes JSON bytes from the KV store into a
// TableMeta. Unknown fields are silently ignored for forward compatibility.
func decodeTableMeta(data []byte) (*TableMeta, error) {
	var meta TableMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// encodeDatabaseMeta serializes a DatabaseMeta to JSON bytes for storage
// in the KV store.
func encodeDatabaseMeta(meta *DatabaseMeta) ([]byte, error) {
	return json.Marshal(meta)
}

// decodeDatabaseMeta deserializes JSON bytes from the KV store into a
// DatabaseMeta.
func decodeDatabaseMeta(data []byte) (*DatabaseMeta, error) {
	var meta DatabaseMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}
