package wal

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"testing"
)

func buildValidFrame(r *Record) []byte {
	return r.Marshal()
}

func TestMarshal_FrameLayout(t *testing.T) {
	r := &Record{Opcode: OpcodePut, Key: []byte("hello"), Value: []byte("world")}
	frame := r.Marshal()

	wantSize := 8 + 3 + len(r.Key) + len(r.Value)
	if len(frame) != wantSize {
		t.Fatalf("frame length = %d, want %d", len(frame), wantSize)
	}

	storedSize := binary.LittleEndian.Uint32(frame[4:8])
	if int(storedSize) != wantSize {
		t.Errorf("stored size field = %d, want %d", storedSize, wantSize)
	}

	if frame[8] != OpcodePut {
		t.Errorf("opcode byte = %d, want %d", frame[8], OpcodePut)
	}

	storedKeyLen := binary.LittleEndian.Uint16(frame[9:11])
	if int(storedKeyLen) != len(r.Key) {
		t.Errorf("stored key length = %d, want %d", storedKeyLen, len(r.Key))
	}

	if !bytes.Equal(frame[11:11+len(r.Key)], r.Key) {
		t.Error("key bytes mismatch in frame")
	}

	if !bytes.Equal(frame[11+len(r.Key):], r.Value) {
		t.Error("value bytes mismatch in frame")
	}
}

func TestMarshal_CRCCoversPayload(t *testing.T) {
	r := &Record{Opcode: OpcodeDelete, Key: []byte("k"), Value: nil}
	frame := r.Marshal()

	storedCRC := binary.LittleEndian.Uint32(frame[0:4])
	if storedCRC != crc32.ChecksumIEEE(frame[4:]) {
		t.Errorf("CRC mismatch: stored=%d calculated=%d", storedCRC, crc32.ChecksumIEEE(frame[4:]))
	}
}

func TestMarshal_OpcodeDelete(t *testing.T) {
	r := &Record{Opcode: OpcodeDelete, Key: []byte("mykey"), Value: nil}
	frame := r.Marshal()
	if frame[8] != OpcodeDelete {
		t.Errorf("opcode = %d, want OpcodeDelete (%d)", frame[8], OpcodeDelete)
	}
}

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
			if keyLen := binary.LittleEndian.Uint16(frame[9:11]); keyLen != 0 {
				t.Errorf("key length = %d, want 0", keyLen)
			}
		})
	}
}

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
			wantSize := 8 + 3 + len(r.Key)
			if len(frame) != wantSize {
				t.Errorf("frame length = %d, want %d", len(frame), wantSize)
			}
		})
	}
}

func TestMarshal_LargePayload(t *testing.T) {
	key := bytes.Repeat([]byte("k"), 1024)
	value := bytes.Repeat([]byte("v"), 4096)
	r := &Record{Opcode: OpcodePut, Key: key, Value: value}
	frame := r.Marshal()
	wantSize := 8 + 3 + len(key) + len(value)
	if len(frame) != wantSize {
		t.Errorf("frame size = %d, want %d", len(frame), wantSize)
	}
	storedCRC := binary.LittleEndian.Uint32(frame[0:4])
	if storedCRC != crc32.ChecksumIEEE(frame[4:]) {
		t.Error("CRC invalid for large payload")
	}
}

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

func TestMarshal_Idempotent(t *testing.T) {
	r := &Record{Opcode: OpcodePut, Key: []byte("idempotent"), Value: []byte("yes")}
	if !bytes.Equal(r.Marshal(), r.Marshal()) {
		t.Error("Marshal is not idempotent for the same record")
	}
}

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

func TestUnmarshal_CRCCorruption(t *testing.T) {
	cases := []struct {
		name      string
		corruptFn func([]byte)
	}{
		{"payload byte", func(f []byte) { f[12] ^= 0x01 }},
		{"crc field", func(f []byte) { f[0] ^= 0xFF }},
		{"size field", func(f []byte) { binary.LittleEndian.PutUint32(f[4:8], 9999) }},
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

func TestUnmarshal_KeyLengthExceedsFrame(t *testing.T) {
	r := &Record{Opcode: OpcodePut, Key: []byte("ab"), Value: []byte("v")}
	frame := r.Marshal()

	binary.LittleEndian.PutUint16(frame[9:11], 65535)
	newCRC := crc32.ChecksumIEEE(frame[4:])
	binary.LittleEndian.PutUint32(frame[0:4], newCRC)

	_, err := UnmarshalRecord(frame)
	if !errors.Is(err, ErrTruncated) {
		t.Errorf("expected ErrTruncated when keyLen > frame size, got %v", err)
	}
}

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

func TestOpcodeValues(t *testing.T) {
	if OpcodePut != 0 {
		t.Errorf("OpcodePut = %d, want 0", OpcodePut)
	}
	if OpcodeDelete != 1 {
		t.Errorf("OpcodeDelete = %d, want 1", OpcodeDelete)
	}
}
