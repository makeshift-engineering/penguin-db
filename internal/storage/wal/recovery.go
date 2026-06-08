package wal

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
)

// MemTable defines the minimal interface for in-memory storage engine components
// that can consume replayed WAL records during recovery.
type MemTable interface {
	Put(key, value []byte) error
	Delete(key []byte) error
}

// Replay scans the specified WAL directory, identifies all segment files
// matching the *.wal pattern, and replays their logged operations onto the
// target MemTable. It returns the highest segment ID found or 1 if fresh.
func Replay(directory string, engine MemTable) (int, error) {
	slog.Debug("starting WAL recovery sequence", "directory", directory)

	entries, err := os.ReadDir(directory)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Debug("WAL directory does not exist, starting fresh", "directory", directory)
			return 1, nil
		}
		return 0, fmt.Errorf("failed to scan WAL directory during recovery boot: %w", err)
	}

	var walFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".wal" {
			walFiles = append(walFiles, entry.Name())
		}
	}

	slog.Debug("found WAL segments for replay", "count", len(walFiles))
	sort.Strings(walFiles)

	highestSegmentID := 0

	for _, fileName := range walFiles {
		var segmentID int
		if n, _ := fmt.Sscanf(fileName, "%d.wal", &segmentID); n != 1 {
			slog.Debug("skipping WAL file with unexpected name format", "file", fileName)
			continue
		}
		if segmentID > highestSegmentID {
			highestSegmentID = segmentID
		}

		filePath := filepath.Join(directory, fileName)
		slog.Debug("replaying WAL segment", "segment_id", segmentID, "file", fileName)

		if err := replayFile(filePath, engine); err != nil {
			return 0, fmt.Errorf("critical failure while replaying segment %s: %w", fileName, err)
		}
	}

	if highestSegmentID == 0 {
		highestSegmentID = 1
	}

	slog.Debug("WAL recovery complete", "highest_segment_id", highestSegmentID)
	return highestSegmentID, nil
}

// replayFile opens a single WAL segment, reads it frame-by-frame, validates
// check-sums and frame sizes, and applies the operations onto the MemTable.
// If it encounters corruption or a partial write, it truncates the segment.
func replayFile(filePath string, engine MemTable) (err error) {
	file, err := os.OpenFile(filePath, os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("unable to open WAL segment for reading: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close WAL segment %s: %w", filePath, closeErr)
		}
	}()

	var validBytes int64
	var recordsRecovered int
	headerBuffer := make([]byte, 8)

	for {
		_, err := io.ReadFull(file, headerBuffer)
		if err != nil {
			if errors.Is(err, io.EOF) {
				slog.Debug("reached clean EOF for WAL segment",
					"file", filepath.Base(filePath),
					"records_recovered", recordsRecovered)
				break
			}
			if errors.Is(err, io.ErrUnexpectedEOF) {
				slog.Debug("unexpected EOF in header, truncating segment",
					"file", filepath.Base(filePath),
					"valid_bytes", validBytes)

				if truncErr := file.Truncate(validBytes); truncErr != nil {
					return truncErr
				}
				return file.Sync()
			}
			return fmt.Errorf("unexpected disk error reading frame header: %w", err)
		}

		totalFrameSizeBytes := binary.LittleEndian.Uint32(headerBuffer[4:8])
		if totalFrameSizeBytes < 8 || totalFrameSizeBytes > 128*1024*1024 {
			slog.Debug("invalid frame size in header, truncating segment",
				"file", filepath.Base(filePath),
				"frame_size", totalFrameSizeBytes,
				"valid_bytes", validBytes)

			if truncErr := file.Truncate(validBytes); truncErr != nil {
				return truncErr
			}
			return file.Sync()
		}
		payloadSizeBytes := totalFrameSizeBytes - 8

		payloadBuffer := make([]byte, payloadSizeBytes)
		_, err = io.ReadFull(file, payloadBuffer)
		if err != nil {
			slog.Debug("unexpected EOF in payload, truncating segment",
				"file", filepath.Base(filePath),
				"valid_bytes", validBytes)

			if truncErr := file.Truncate(validBytes); truncErr != nil {
				return truncErr
			}
			return file.Sync()
		}

		fullFrame := make([]byte, 8+payloadSizeBytes)
		copy(fullFrame[:8], headerBuffer)
		copy(fullFrame[8:], payloadBuffer)

		record, err := UnmarshalRecord(fullFrame)
		if err != nil {
			if errors.Is(err, ErrInvalidCRC) || errors.Is(err, ErrTruncated) {
				slog.Debug("corrupted frame detected, truncating segment",
					"file", filepath.Base(filePath),
					"valid_bytes", validBytes,
					"error", err)

				if truncErr := file.Truncate(validBytes); truncErr != nil {
					return truncErr
				}
				return file.Sync()
			}
			return fmt.Errorf("failed to decode valid frame payload: %w", err)
		}

		switch record.Opcode {
		case OpcodePut:
			if putErr := engine.Put(record.Key, record.Value); putErr != nil {
				return fmt.Errorf("memtable rejected recovered put operation: %w", putErr)
			}
		case OpcodeDelete:
			if delErr := engine.Delete(record.Key); delErr != nil {
				return fmt.Errorf("memtable rejected recovered delete operation: %w", delErr)
			}
		}

		validBytes += int64(totalFrameSizeBytes)
		recordsRecovered++
	}

	return nil
}
