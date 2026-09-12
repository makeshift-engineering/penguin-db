package wire

import (
	"bytes"
	"encoding/binary"
	"io"
)

const (
	ProtocolVersion3 = 196608
	SSLRequestCode   = 80877103
)

const (
	MessageQuery     byte = 'Q'
	MessageTerminate byte = 'X'
)

const (
	MessageAuth            byte = 'R'
	MessageParameterStatus byte = 'S'
	MessageReadyForQuery   byte = 'Z'
	MessageRowDescription  byte = 'T'
	MessageDataRow         byte = 'D'
	MessageCommandComplete byte = 'C'
	MessageErrorResponse   byte = 'E'
)

const (
	AuthOk int32 = 0
)

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
	var tempBuff [2]byte
	binary.BigEndian.PutUint16(tempBuff[:], uint16(v))
	b.buffer.Write(tempBuff[:])
}

func (b *messageBuffer) writeInt32(v int32) {
	var tempBuff [4]byte
	binary.BigEndian.PutUint16(tempBuff[:], uint16(v))
	b.buffer.Write(tempBuff[:])
}

func (b *messageBuffer) writeCString(v string) {
	b.buffer.WriteString(v)
	b.buffer.WriteByte(0)
}

func (b *messageBuffer) writeBytes(v []byte) {
	b.buffer.Write(v)
}

func (b *messageBuffer) finish(messageType byte) []byte {
	payload := b.buffer.Bytes()
	length := int32(len(payload) + 4)

	var header bytes.Buffer
	if messageType != 0 {
		header.WriteByte(messageType)
	}
	var tempBuff [4]byte
	binary.BigEndian.PutUint32(tempBuff[:], uint32(length))
	header.Write(tempBuff[:])
	header.Write(payload)

	return header.Bytes()
}

func readInt32(r io.Reader) (int32, error) {
	var buffer [4]byte
	if _, err := io.ReadFull(r, buffer[:]); err != nil {
		return 0, err
	}
	return int32(binary.BigEndian.Uint32(buffer[:])), nil
}
func readInt16(r io.Reader) (int16, error) {
	var buffer [2]byte
	if _, err := io.ReadFull(r, buffer[:]); err != nil {
		return 0, err
	}
	return int16(binary.BigEndian.Uint16(buffer[:])), nil
}
func readCString(r io.Reader) (string, error) {
	var buffer bytes.Buffer
	var oneByte [1]byte
	for {
		if _, err := io.ReadFull(r, oneByte[:]); err != nil {
			return "", err
		}
		if oneByte[0] == 0 {
			break
		}
		buffer.WriteByte(oneByte[0])
	}
	return buffer.String(), nil
}

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
