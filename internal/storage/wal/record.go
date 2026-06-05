package wal

import (
	"encoding/binary"
	"hash/crc32"
)

const (
	// OpcodePut represents a put/insert operation in the WAL.
	OpcodePut    uint8 = 0
	// OpcodeDelete represents a delete operation in the WAL.
	OpcodeDelete uint8 = 1
)

// Record represents a single change logged in the WAL, wrapping an operation
// type (Opcode), key, and value payload.
type Record struct {
	Opcode uint8
	Key    []byte
	Value  []byte
}

// Marshal serializes the Record into a binary frame with a 4-byte CRC checksum,
// a 4-byte frame size header, a 1-byte opcode, a 2-byte key length, and the
// raw key and value bytes.
func (record *Record) Marshal() []byte {
	metadataAndDataSize := 3 + len(record.Key) + len(record.Value)
	totalFrameSizeBytes := 8 + metadataAndDataSize

	frameBuffer := make([]byte, totalFrameSizeBytes)

	binary.LittleEndian.PutUint32(frameBuffer[4:8], uint32(totalFrameSizeBytes))
	frameBuffer[8] = record.Opcode
	binary.LittleEndian.PutUint16(frameBuffer[9:11], uint16(len(record.Key)))

	copy(frameBuffer[11:], record.Key)
	copy(frameBuffer[11+len(record.Key):], record.Value)

	calculatedChecksum := crc32.ChecksumIEEE(frameBuffer[4:])
	binary.LittleEndian.PutUint32(frameBuffer[0:4], calculatedChecksum)

	return frameBuffer
}

// UnmarshalRecord deserializes a raw binary frame and reconstructs the original
// Record. It validates the data integrity using the CRC checksum and performs
// bounds checking on payload lengths.
func UnmarshalRecord(frameData []byte) (*Record, error) {
	if len(frameData) < 11 {
		return nil, ErrTruncated
	}

	storedChecksum := binary.LittleEndian.Uint32(frameData[0:4])
	calculatedChecksum := crc32.ChecksumIEEE(frameData[4:])

	if storedChecksum != calculatedChecksum {
		return nil, ErrInvalidCRC
	}

	extractedOpcode := frameData[8]
	extractedKeyLength := binary.LittleEndian.Uint16(frameData[9:11])

	if len(frameData) < 11+int(extractedKeyLength) {
		return nil, ErrTruncated
	}

	extractedKey := make([]byte, extractedKeyLength)
	copy(extractedKey, frameData[11:11+int(extractedKeyLength)])

	extractedValueLength := len(frameData) - (11 + int(extractedKeyLength))
	var extractedValue []byte
	if extractedValueLength > 0 {
		extractedValue = make([]byte, extractedValueLength)
		copy(extractedValue, frameData[11+extractedKeyLength:])
	}

	return &Record{
		Opcode: extractedOpcode,
		Key:    extractedKey,
		Value:  extractedValue,
	}, nil
}
