package wal

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"testing"
)

// buildValidFrame is a helper that constructs a valid binary frame from a Record.
func buildValidFrame(r *Record) []byte {
	return r.Marshal()
}

// TestMarshal_FrameLayout tests that the layout of the marshaled frame meets
// specification requirements (proper size, opcode location, and key/value placement).
func TestMarshal_FrameLayout(t *testing.T) {
	r := &Record{Opcode: OpcodePut, Key: []byte("hello"), Value: []byte("world")}
	frame := r.Marshal()

	wantSize := fixedHeaderSize + len(r.Key) + len(r.Value)
	if len(frame) != wantSize {
		t.Fatalf("frame length = %d, want %d", len(frame), wantSize)
	}

	storedSize := binary.LittleEndian.Uint32(frame[frameSizeOffset:opcodeOffset])
	if int(storedSize) != wantSize {
		t.Errorf("stored size field = %d, want %d", storedSize, wantSize)
	}

	if frame[opcodeOffset] != OpcodePut {
		t.Errorf("opcode byte = %d, want %d", frame[opcodeOffset], OpcodePut)
	}

	storedKeyLen := binary.LittleEndian.Uint16(frame[keyLengthOffset:keyOffset])
	if int(storedKeyLen) != len(r.Key) {
		t.Errorf("stored key length = %d, want %d", storedKeyLen, len(r.Key))
	}

	if !bytes.Equal(frame[keyOffset:keyOffset+len(r.Key)], r.Key) {
		t.Error("key bytes mismatch in frame")
	}

	if !bytes.Equal(frame[keyOffset+len(r.Key):], r.Value) {
		t.Error("value bytes mismatch in frame")
	}
}

// TestMarshal_CRCCoversPayload verifies that the CRC-32 checksum is calculated
// correctly over the rest of the frame payload.
func TestMarshal_CRCCoversPayload(t *testing.T) {
	r := &Record{Opcode: OpcodeDelete, Key: []byte("k"), Value: nil}
	frame := r.Marshal()

	storedCRC := binary.LittleEndian.Uint32(frame[checksumOffset:frameSizeOffset])
	if storedCRC != crc32.ChecksumIEEE(frame[frameSizeOffset:]) {
		t.Errorf("CRC mismatch: stored=%d calculated=%d", storedCRC, crc32.ChecksumIEEE(frame[frameSizeOffset:]))
	}
}

// TestMarshal_OpcodeDelete verifies that deletion records marshal with OpcodeDelete.
func TestMarshal_OpcodeDelete(t *testing.T) {
	r := &Record{Opcode: OpcodeDelete, Key: []byte("mykey"), Value: nil}
	frame := r.Marshal()
	if frame[opcodeOffset] != OpcodeDelete {
		t.Errorf("opcode = %d, want OpcodeDelete (%d)", frame[opcodeOffset], OpcodeDelete)
	}
}

// TestMarshal_ZeroLengthKey tests marshaling records with empty or nil keys.
func TestMarshal_ZeroLengthKey(t *testing.T) {
	cases := []struct {
		name string
		key  []byte
	}{
		{"empty", []byte{}},
		{"nil", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			frame := (&Record{Opcode: OpcodePut, Key: tc.key, Value: []byte("v")}).Marshal()
			if keyLen := binary.LittleEndian.Uint16(frame[keyLengthOffset:keyOffset]); keyLen != 0 {
				t.Errorf("key length = %d, want 0", keyLen)
			}
		})
	}
}

// TestMarshal_ZeroLengthValue tests marshaling records with empty or nil values.
func TestMarshal_ZeroLengthValue(t *testing.T) {
	cases := []struct {
		name  string
		value []byte
	}{
		{"empty", []byte{}},
		{"nil", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &Record{Opcode: OpcodePut, Key: []byte("key"), Value: tc.value}
			frame := r.Marshal()
			wantSize := fixedHeaderSize + len(r.Key)
			if len(frame) != wantSize {
				t.Errorf("frame length = %d, want %d", len(frame), wantSize)
			}
		})
	}
}

// TestMarshal_LargePayload tests marshalling records with large keys and values.
func TestMarshal_LargePayload(t *testing.T) {
	key := bytes.Repeat([]byte("k"), 1024)
	value := bytes.Repeat([]byte("v"), 4096)
	r := &Record{Opcode: OpcodePut, Key: key, Value: value}
	frame := r.Marshal()
	wantSize := fixedHeaderSize + len(key) + len(value)
	if len(frame) != wantSize {
		t.Errorf("frame size = %d, want %d", len(frame), wantSize)
	}
	storedCRC := binary.LittleEndian.Uint32(frame[checksumOffset:frameSizeOffset])
	if storedCRC != crc32.ChecksumIEEE(frame[frameSizeOffset:]) {
		t.Error("CRC invalid for large payload")
	}
}

// TestMarshal_BinaryKeyAndValue ensures that arbitrary binary data is preserved
// during marshalling and unmarshalling.
func TestMarshal_BinaryKeyAndValue(t *testing.T) {
	key := []byte{0x00, 0xFF, 0x7F, 0x80}
	value := []byte{0x01, 0x02, 0x03}
	r := &Record{Opcode: OpcodePut, Key: key, Value: value}
	frame := r.Marshal()

	recovered, err := UnmarshalRecord(frame)
	if err != nil {
		t.Fatalf("UnmarshalRecord failed: %v", err)
	}
	if !bytes.Equal(recovered.Key, key) {
		t.Errorf("key mismatch: got %v, want %v", recovered.Key, key)
	}
	if !bytes.Equal(recovered.Value, value) {
		t.Errorf("value mismatch: got %v, want %v", recovered.Value, value)
	}
}

// TestMarshal_Idempotent verifies that Marshal output is identical for successive calls.
func TestMarshal_Idempotent(t *testing.T) {
	r := &Record{Opcode: OpcodePut, Key: []byte("idempotent"), Value: []byte("yes")}
	if !bytes.Equal(r.Marshal(), r.Marshal()) {
		t.Error("Marshal is not idempotent for the same record")
	}
}

// TestUnmarshal_RoundTrip_Put checks that a serialized Put record can be accurately reconstructed.
func TestUnmarshal_RoundTrip_Put(t *testing.T) {
	original := &Record{Opcode: OpcodePut, Key: []byte("name"), Value: []byte("penguin")}
	frame := buildValidFrame(original)

	recovered, err := UnmarshalRecord(frame)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recovered.Opcode != original.Opcode {
		t.Errorf("opcode: got %d, want %d", recovered.Opcode, original.Opcode)
	}
	if !bytes.Equal(recovered.Key, original.Key) {
		t.Errorf("key: got %q, want %q", recovered.Key, original.Key)
	}
	if !bytes.Equal(recovered.Value, original.Value) {
		t.Errorf("value: got %q, want %q", recovered.Value, original.Value)
	}
}

// TestUnmarshal_RoundTrip_Delete checks that a serialized Delete record can be accurately reconstructed.
func TestUnmarshal_RoundTrip_Delete(t *testing.T) {
	original := &Record{Opcode: OpcodeDelete, Key: []byte("to-remove"), Value: nil}
	frame := buildValidFrame(original)

	recovered, err := UnmarshalRecord(frame)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recovered.Opcode != OpcodeDelete {
		t.Errorf("opcode: got %d, want OpcodeDelete", recovered.Opcode)
	}
	if !bytes.Equal(recovered.Key, original.Key) {
		t.Errorf("key mismatch")
	}
}

// TestUnmarshal_TruncatedFrame_TooShort verifies that short inputs return ErrTruncated.
func TestUnmarshal_TruncatedFrame_TooShort(t *testing.T) {
	cases := [][]byte{
		{},
		{0x01},
		{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A},
	}
	for _, data := range cases {
		_, err := UnmarshalRecord(data)
		if !errors.Is(err, ErrTruncated) {
			t.Errorf("input len=%d: expected ErrTruncated, got %v", len(data), err)
		}
	}
}

// TestUnmarshal_MinimalFrame_EmptyKeyNoValue verifies parsing of minimally sized frames.
func TestUnmarshal_MinimalFrame_EmptyKeyNoValue(t *testing.T) {
	r := &Record{Opcode: OpcodePut, Key: []byte{}, Value: nil}
	frame := r.Marshal()
	recovered, err := UnmarshalRecord(frame)
	if err != nil {
		t.Fatalf("unexpected error for minimal frame: %v", err)
	}
	if len(recovered.Key) != 0 {
		t.Errorf("expected empty key, got %v", recovered.Key)
	}
}

// TestUnmarshal_CRCCorruption tests that altered headers or payloads result in checksum errors.
func TestUnmarshal_CRCCorruption(t *testing.T) {
	cases := []struct {
		name      string
		corruptFn func([]byte)
	}{
		{"payload byte", func(f []byte) { f[keyOffset+1] ^= 0x01 }},
		{"crc field", func(f []byte) { f[checksumOffset] ^= 0xFF }},
		{"size field", func(f []byte) { binary.LittleEndian.PutUint32(f[frameSizeOffset:opcodeOffset], 9999) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &Record{Opcode: OpcodePut, Key: []byte("key"), Value: []byte("val")}
			frame := r.Marshal()
			tc.corruptFn(frame)
			_, err := UnmarshalRecord(frame)
			if !errors.Is(err, ErrInvalidCRC) {
				t.Errorf("expected ErrInvalidCRC, got %v", err)
			}
		})
	}
}

// TestUnmarshal_KeyLengthExceedsFrame checks that key lengths exceeding frame limits are rejected.
func TestUnmarshal_KeyLengthExceedsFrame(t *testing.T) {
	r := &Record{Opcode: OpcodePut, Key: []byte("ab"), Value: []byte("v")}
	frame := r.Marshal()

	binary.LittleEndian.PutUint16(frame[keyLengthOffset:keyOffset], 65535)
	newCRC := crc32.ChecksumIEEE(frame[frameSizeOffset:])
	binary.LittleEndian.PutUint32(frame[checksumOffset:frameSizeOffset], newCRC)

	_, err := UnmarshalRecord(frame)
	if !errors.Is(err, ErrTruncated) {
		t.Errorf("expected ErrTruncated when keyLen > frame size, got %v", err)
	}
}

// TestUnmarshal_NilValueForDeleteRecord checks that deleted records correctly parse nil values.
func TestUnmarshal_NilValueForDeleteRecord(t *testing.T) {
	r := &Record{Opcode: OpcodeDelete, Key: []byte("mykey"), Value: nil}
	frame := r.Marshal()

	recovered, err := UnmarshalRecord(frame)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recovered.Value != nil {
		t.Errorf("expected nil Value for delete record, got %v", recovered.Value)
	}
}

// TestOpcodeValues validates correctness of package opcode constants.
func TestOpcodeValues(t *testing.T) {
	if OpcodePut != 0 {
		t.Errorf("OpcodePut = %d, want 0", OpcodePut)
	}
	if OpcodeDelete != 1 {
		t.Errorf("OpcodeDelete = %d, want 1", OpcodeDelete)
	}
}
