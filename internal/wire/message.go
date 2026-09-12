package wire

import (
	"bytes"
	"encoding/binary"
	"io"
)

// Protocol constants and magic numbers for PostgreSQL Wire Protocol v3.
const (
	ProtocolVersion3 = 196608   // Protocol version 3.0 (0x00030000)
	SSLRequestCode   = 80877103 // SSL negotiation request code (0x04D2162F)
)

// Frontend message type identifiers sent from client to server.
const (
	MessageQuery     byte = 'Q' // Simple Query message
	MessageTerminate byte = 'X' // Terminate Connection message
)

// Backend message type identifiers sent from server to client.
const (
	MessageAuth            byte = 'R' // Authentication Response payload
	MessageParameterStatus byte = 'S' // Session Parameter Status notification
	MessageReadyForQuery   byte = 'Z' // Ready For Query status indicator
	MessageRowDescription  byte = 'T' // Row Header metadata definition
	MessageDataRow         byte = 'D' // Data Row tuple payload
	MessageCommandComplete byte = 'C' // Command Execution Complete tag
	MessageErrorResponse   byte = 'E' // Error response message
)

// Authentication sub-type codes.
const (
	AuthOk int32 = 0 // Trust authentication success
)

// Buffer helper for building big-endian binary PostgreSQL wire protocol frames.
type messageBuffer struct {
	buffer bytes.Buffer
}

func newMessageBuffer() *messageBuffer {
	return &messageBuffer{}
}

func (b *messageBuffer) writeByte(v byte) {
	b.buffer.WriteByte(v)
}

func (b *messageBuffer) writeInt16(v int16) {
	var scratch [2]byte
	binary.BigEndian.PutUint16(scratch[:], uint16(v))
	b.buffer.Write(scratch[:])
}

func (b *messageBuffer) writeInt32(v int32) {
	var scratch [4]byte
	binary.BigEndian.PutUint32(scratch[:], uint32(v))
	b.buffer.Write(scratch[:])
}

func (b *messageBuffer) writeCString(v string) {
	b.buffer.WriteString(v)
	b.buffer.WriteByte(0)
}

func (b *messageBuffer) writeBytes(v []byte) {
	b.buffer.Write(v)
}

// finish constructs a framed binary packet with the format [MessageType][PayloadLength][Payload].
func (b *messageBuffer) finish(messageType byte) []byte {
	payload := b.buffer.Bytes()
	length := int32(len(payload) + 4)

	var header bytes.Buffer
	if messageType != 0 {
		header.WriteByte(messageType)
	}
	var scratch [4]byte
	binary.BigEndian.PutUint32(scratch[:], uint32(length))
	header.Write(scratch[:])
	header.Write(payload)

	return header.Bytes()
}

// Low-level binary stream reader helper functions for incoming frontend packets.

func readInt32(r io.Reader) (int32, error) {
	var buf [4]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return int32(binary.BigEndian.Uint32(buf[:])), nil
}

func readInt16(r io.Reader) (int16, error) {
	var buf [2]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return int16(binary.BigEndian.Uint16(buf[:])), nil
}

func readCString(r io.Reader) (string, error) {
	var buf bytes.Buffer
	var oneByte [1]byte
	for {
		if _, err := io.ReadFull(r, oneByte[:]); err != nil {
			return "", err
		}
		if oneByte[0] == 0 {
			break
		}
		buf.WriteByte(oneByte[0])
	}
	return buf.String(), nil
}

// Packet encoder functions for constructing server responses.

func encodeAuthOk() []byte {
	buf := newMessageBuffer()
	buf.writeInt32(AuthOk)
	return buf.finish(MessageAuth)
}

func encodeParameterStatus(key, val string) []byte {
	buf := newMessageBuffer()
	buf.writeCString(key)
	buf.writeCString(val)
	return buf.finish(MessageParameterStatus)
}

func encodeReadyForQuery(status byte) []byte {
	buf := newMessageBuffer()
	buf.writeByte(status)
	return buf.finish(MessageReadyForQuery)
}

func encodeCommandComplete(tag string) []byte {
	buf := newMessageBuffer()
	buf.writeCString(tag)
	return buf.finish(MessageCommandComplete)
}

func encodeErrorResponse(severity, code, message string) []byte {
	buf := newMessageBuffer()
	buf.writeByte('S')
	buf.writeCString(severity)
	buf.writeByte('C')
	buf.writeCString(code)
	buf.writeByte('M')
	buf.writeCString(message)
	buf.writeByte(0)
	return buf.finish(MessageErrorResponse)
}
