package log

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"sync"

	consensuspb "github.com/makeshift-engineering/penguin-db/gen/go/consensus/v1"
)

/*
+---------------+---------------+---------------+-------------------+--------------------+--------------------+
| Magic (2B)    | CRC32 (4B)    | Index (8B)    | Term (8B)         | Type (1B)          | PayloadLen (4B)    |
| 0x50 0x47     | IEEE Checksum | uint64        | uint64            | uint8              | uint32             |
+---------------+---------------+---------------+-------------------+--------------------+--------------------+
| Variable Data Payload (PayloadLen Bytes)                                                                        |
+-----------------------------------------------------------------------------------------------------------------+
*/

const (
	segmentMagic   uint16 = 0x5047
	headerSize            = 27
	maxPayloadSize uint32 = 64 * 1024 * 1024
)

var (
	ErrTornWrite     = errors.New("torn write detected at end of segment")
	ErrCorruptRecord = errors.New("corrupted log record checksum mismatch")
	ErrClosedSegment = errors.New("segment file is closed")
)

type Segment struct {
	mu         sync.RWMutex
	file       *os.File
	path       string
	baseIndex  uint64
	activeSize int64
	closed     bool
}

func OpenSegment(path string, baseIndex uint64) (*Segment, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("failed to open segment %s: %w", path, err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}

	return &Segment{
		file:       file,
		path:       path,
		baseIndex:  baseIndex,
		activeSize: info.Size(),
	}, nil
}

func (s *Segment) Append(entry *consensuspb.LogEntry) (int64, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return 0, 0, ErrClosedSegment
	}

	payloadLen := uint32(len(entry.Data))
	if payloadLen > maxPayloadSize {
		return 0, 0, fmt.Errorf("entry data exceeds max payload size: %d", payloadLen)
	}

	recordLen := int64(headerSize + payloadLen)
	buf := make([]byte, recordLen)

	binary.LittleEndian.PutUint16(buf[0:2], segmentMagic)
	binary.LittleEndian.PutUint64(buf[6:14], entry.Index)
	binary.LittleEndian.PutUint64(buf[14:22], entry.Term)
	buf[22] = uint8(entry.Type)
	binary.LittleEndian.PutUint32(buf[23:27], payloadLen)
	copy(buf[27:], entry.Data)

	crc := crc32.ChecksumIEEE(buf[6:])
	binary.LittleEndian.PutUint32(buf[2:6], crc)

	writeOffset := s.activeSize
	if _, err := s.file.WriteAt(buf, writeOffset); err != nil {
		return 0, 0, fmt.Errorf("failed to write record at offset %d: %w", writeOffset, err)
	}

	s.activeSize += recordLen
	return writeOffset, recordLen, nil
}

func (s *Segment) ReadAt(offset int64) (*consensuspb.LogEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrClosedSegment
	}

	var header [headerSize]byte
	if _, err := s.file.ReadAt(header[:], offset); err != nil {
		return nil, err
	}

	magic := binary.LittleEndian.Uint16(header[0:2])
	if magic != segmentMagic {
		return nil, fmt.Errorf("%w: invalid magic 0x%x at offset %d", ErrCorruptRecord, magic, offset)
	}

	expectedCRC := binary.LittleEndian.Uint32(header[2:6])
	index := binary.LittleEndian.Uint64(header[6:14])
	term := binary.LittleEndian.Uint64(header[14:22])
	entryType := consensuspb.EntryType(header[22])
	payloadLen := binary.LittleEndian.Uint32(header[23:27])

	if payloadLen > maxPayloadSize {
		return nil, fmt.Errorf("%w: payload length %d exceeds max allowed at offset %d", ErrCorruptRecord, payloadLen, offset)
	}

	payload := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := s.file.ReadAt(payload, offset+headerSize); err != nil {
			return nil, err
		}
	}

	hash := crc32.NewIEEE()
	hash.Write(header[6:])
	hash.Write(payload)
	if hash.Sum32() != expectedCRC {
		return nil, fmt.Errorf("%w: CRC mismatch at offset %d", ErrCorruptRecord, offset)
	}

	return &consensuspb.LogEntry{
		Index: index,
		Term:  term,
		Type:  entryType,
		Data:  payload,
	}, nil
}

type ScanRecord func(entry *consensuspb.LogEntry, offset int64, length int64) error

func (s *Segment) Scan(fn ScanRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var offset int64 = 0
	for offset < s.activeSize {
		if offset+headerSize > s.activeSize {
			if err := s.truncateTo(offset); err != nil {
				return err
			}
			break
		}

		var header [headerSize]byte
		if _, err := s.file.ReadAt(header[:], offset); err != nil {
			return err
		}

		magic := binary.LittleEndian.Uint16(header[0:2])
		if magic != segmentMagic {
			if err := s.truncateTo(offset); err != nil {
				return err
			}
			break
		}

		expectedCRC := binary.LittleEndian.Uint32(header[2:6])
		index := binary.LittleEndian.Uint64(header[6:14])
		term := binary.LittleEndian.Uint64(header[14:22])
		entryType := consensuspb.EntryType(header[22])
		payloadLen := binary.LittleEndian.Uint32(header[23:27])
		recordLen := int64(headerSize + payloadLen)
		if offset+recordLen > s.activeSize {
			if err := s.truncateTo(offset); err != nil {
				return err
			}
			break
		}

		payload := make([]byte, payloadLen)
		if payloadLen > 0 {
			if _, err := s.file.ReadAt(payload, offset+headerSize); err != nil {
				return err
			}
		}

		hash := crc32.NewIEEE()
		hash.Write(header[6:])
		hash.Write(payload)
		if hash.Sum32() != expectedCRC {
			if err := s.truncateTo(offset); err != nil {
				return err
			}
			break
		}

		entry := &consensuspb.LogEntry{
			Index: index,
			Term:  term,
			Type:  entryType,
			Data:  payload,
		}

		if err := fn(entry, offset, recordLen); err != nil {
			return err
		}
		offset += recordLen
	}
	return nil
}

func (s *Segment) truncateTo(size int64) error {
	if err := s.file.Truncate(size); err != nil {
		return fmt.Errorf("failed to truncate segment to %d: %w", size, err)
	}
	s.activeSize = size
	return s.file.Sync()
}

func (s *Segment) Sync() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.file.Sync()
}

func (s *Segment) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return s.file.Close()
}
