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

type MemTable interface {
	Put(key, value []byte) error
	Delete(key []byte) error
}

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
		fmt.Sscanf(fileName, "%d.wal", &segmentID)
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

func replayFile(filePath string, engine MemTable) error {
	file, err := os.OpenFile(filePath, os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("unable to open WAL segment for reading: %w", err)
	}
	defer file.Close()

	var validBytes int64 = 0
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
				return file.Truncate(validBytes)
			}
			return fmt.Errorf("unexpected disk error reading frame header: %w", err)
		}

		totalFrameSizeBytes := binary.LittleEndian.Uint32(headerBuffer[4:8])
		payloadSizeBytes := totalFrameSizeBytes - 8

		payloadBuffer := make([]byte, payloadSizeBytes)
		_, err = io.ReadFull(file, payloadBuffer)
		if err != nil {
			slog.Debug("unexpected EOF in payload, truncating segment",
				"file", filepath.Base(filePath),
				"valid_bytes", validBytes)
			return file.Truncate(validBytes)
		}

		fullFrame := append(headerBuffer, payloadBuffer...)

		record, err := UnmarshalRecord(fullFrame)
		if err != nil {
			if errors.Is(err, ErrInvalidCRC) || errors.Is(err, ErrTruncated) {
				slog.Debug("corrupted frame detected, truncating segment",
					"file", filepath.Base(filePath),
					"valid_bytes", validBytes,
					"error", err)
				return file.Truncate(validBytes)
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
