package wal

import (
	"encoding/binary"
	"hash/crc32"
	"math"
)

const (
	// OpcodePut represents a put/insert operation in the WAL.
	OpcodePut uint8 = 0
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

// Define sizes for each field in the frame
const (
	checksumSize  = 4
	frameSizeSize = 4
	opcodeSize    = 1
	keyLengthSize = 2

	// Fixed header size is the sum of all fixed-length fields
	fixedHeaderSize = checksumSize + frameSizeSize + opcodeSize + keyLengthSize
)

// Define offsets for each field to eliminate magic slice indices
const (
	checksumOffset  = 0
	frameSizeOffset = checksumOffset + checksumSize
	opcodeOffset    = frameSizeOffset + frameSizeSize
	keyLengthOffset = opcodeOffset + opcodeSize
	keyOffset       = keyLengthOffset + keyLengthSize
)

// Marshal serializes the Record into a binary frame.
//
// Frame Layout:
// +-------------+-------------+----------+------------+-----------+-----------+
// | Checksum    | Frame Size  | Opcode   | Key Length | Key       | Value     |
// | (4 bytes)   | (4 bytes)   | (1 byte) | (2 bytes)  | (n bytes) | (m bytes) |
// +-------------+-------------+----------+------------+-----------+-----------+
//
// Note: The Checksum (CRC32) covers all bytes starting from the Frame Size.
//
// Marshal returns ErrKeyTooLarge if the key exceeds math.MaxUint16 bytes, or
// ErrFrameTooLarge if the total frame size exceeds maxFrameSizeBytes (32 MiB,
// equal to MaxSegmentSizeBytes — ensuring no single record can span segments).
func (record *Record) Marshal() ([]byte, error) {
	keyLen := len(record.Key)
	valLen := len(record.Value)

	if keyLen > math.MaxUint16 {
		return nil, ErrKeyTooLarge
	}

	totalFrameSizeBytes := fixedHeaderSize + keyLen + valLen

	if totalFrameSizeBytes > maxFrameSizeBytes {
		return nil, ErrFrameTooLarge
	}

	frameBuffer := make([]byte, totalFrameSizeBytes)

	binary.LittleEndian.PutUint32(frameBuffer[frameSizeOffset:opcodeOffset], uint32(totalFrameSizeBytes))
	frameBuffer[opcodeOffset] = record.Opcode
	binary.LittleEndian.PutUint16(frameBuffer[keyLengthOffset:keyOffset], uint16(keyLen))

	copy(frameBuffer[keyOffset:], record.Key)
	valueOffset := keyOffset + keyLen
	copy(frameBuffer[valueOffset:], record.Value)

	calculatedChecksum := crc32.ChecksumIEEE(frameBuffer[frameSizeOffset:])
	binary.LittleEndian.PutUint32(frameBuffer[checksumOffset:frameSizeOffset], calculatedChecksum)

	return frameBuffer, nil
}

// UnmarshalRecord deserializes a raw binary frame and reconstructs the original
// Record. It validates the data integrity using the CRC checksum and performs
// bounds checking on payload lengths.
func UnmarshalRecord(frameData []byte) (*Record, error) {
	if len(frameData) < fixedHeaderSize {
		return nil, ErrTruncated
	}

	storedChecksum := binary.LittleEndian.Uint32(frameData[checksumOffset:frameSizeOffset])
	calculatedChecksum := crc32.ChecksumIEEE(frameData[frameSizeOffset:])

	if storedChecksum != calculatedChecksum {
		return nil, ErrInvalidCRC
	}

	extractedOpcode := frameData[opcodeOffset]
	extractedKeyLength := binary.LittleEndian.Uint16(frameData[keyLengthOffset:keyOffset])

	if len(frameData) < fixedHeaderSize+int(extractedKeyLength) {
		return nil, ErrTruncated
	}

	extractedKey := make([]byte, extractedKeyLength)
	copy(extractedKey, frameData[keyOffset:keyOffset+int(extractedKeyLength)])

	extractedValueLength := len(frameData) - (fixedHeaderSize + int(extractedKeyLength))
	var extractedValue []byte

	if extractedValueLength > 0 {
		extractedValue = make([]byte, extractedValueLength)
		valueOffset := keyOffset + int(extractedKeyLength)
		copy(extractedValue, frameData[valueOffset:])
	}

	return &Record{
		Opcode: extractedOpcode,
		Key:    extractedKey,
		Value:  extractedValue,
	}, nil
}
