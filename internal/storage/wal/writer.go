package wal

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

const MaxSegmentSizeBytes int64 = 32 * 1024 * 1024

// commitTicket represents an ingestion task containing serialized record data
// and a channel to communicate the result of the log write and fsync.
type commitTicket struct {
	frameData  []byte
	resultChan chan error
}

// LogWriter manages sequential appending to WAL segment files. It coordinates
// concurrent appends, runs a batching background worker to flush writes, and
// handles file rotation when a segment exceeds its capacity.
type LogWriter struct {
	directory        string
	activeFile       *os.File
	currentSegmentID int
	currentSizeBytes int64

	ingestionChannel chan *commitTicket
	shutdownSignal   chan struct{}
	workerWaitGroup  sync.WaitGroup
	closeOnce        sync.Once
}

// NewLogWriter creates a new LogWriter instance, initializing the WAL directory,
// creating the initial active segment, and launching the background batch worker.
func NewLogWriter(directory string, nextSegmentID int) (*LogWriter, error) {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, fmt.Errorf("failed to initialize WAL directory structure at %s: %w", directory, err)
	}

	writer := &LogWriter{
		directory:        directory,
		currentSegmentID: nextSegmentID,
		ingestionChannel: make(chan *commitTicket, 10000),
		shutdownSignal:   make(chan struct{}),
	}

	if err := writer.rotateActiveFile(); err != nil {
		return nil, err
	}

	writer.workerWaitGroup.Add(1)
	go writer.batchWorker()

	return writer, nil
}

// rotateActiveFile closes the current active segment file after fsyncing its data,
// increments the segment ID, and opens a new segment file for write access.
func (writer *LogWriter) rotateActiveFile() error {
	if writer.activeFile != nil {
		if err := writer.activeFile.Sync(); err != nil {
			return fmt.Errorf("failed to sync WAL segment %d during rotation: %w", writer.currentSegmentID, err)
		}
		if err := writer.activeFile.Close(); err != nil {
			return fmt.Errorf("failed to close WAL segment %d during rotation: %w", writer.currentSegmentID, err)
		}
		writer.currentSegmentID++
	}

	segmentPath := filepath.Join(writer.directory, fmt.Sprintf("%06d.wal", writer.currentSegmentID))
	file, err := os.OpenFile(segmentPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open new WAL segment %s: %w", segmentPath, err)
	}

	writer.activeFile = file
	writer.currentSizeBytes = 0
	return nil
}

// Append writes a single Record into the Write-Ahead Log. It blocks until the
// record is durably persisted (written and synced) or the log is closed.
func (writer *LogWriter) Append(record *Record) error {
	if len(record.Key) == 0 {
		return ErrEmptyKey
	}

	frame := record.Marshal()

	ticket := &commitTicket{
		frameData:  frame,
		resultChan: make(chan error, 1),
	}

	slog.Debug("network thread: dropping record into ingestion channel", "frame_size", len(frame))

	select {
	case writer.ingestionChannel <- ticket:
		select {
		case err := <-ticket.resultChan:
			return err
		case <-writer.shutdownSignal:
			return fmt.Errorf("database engine is currently shutting down, write rejected")
		}
	case <-writer.shutdownSignal:
		return fmt.Errorf("database engine is currently shutting down, write rejected")
	}
}

// batchWorker runs in a background goroutine, receiving commit tickets from the
// ingestion channel, batching them, writing to the active file, and syncing them.
func (writer *LogWriter) batchWorker() {
	defer writer.workerWaitGroup.Done()

	var commitBatch []*commitTicket
	var writeBuffer []byte

	for {
		select {
		case <-writer.shutdownSignal:
			return

		case firstTicket := <-writer.ingestionChannel:
			commitBatch = append(commitBatch[:0], firstTicket)
			writeBuffer = append(writeBuffer[:0], firstTicket.frameData...)

			pendingWrites := len(writer.ingestionChannel)
			for i := 0; i < pendingWrites; i++ {
				ticket := <-writer.ingestionChannel
				commitBatch = append(commitBatch, ticket)
				writeBuffer = append(writeBuffer, ticket.frameData...)
			}

			if writer.currentSizeBytes+int64(len(writeBuffer)) > MaxSegmentSizeBytes {
				if err := writer.rotateActiveFile(); err != nil {
					for _, ticket := range commitBatch {
						ticket.resultChan <- err
					}
					continue
				}
			}

			slog.Debug("batch worker: executing group commit",
				"batch_size", len(commitBatch),
				"total_bytes", len(writeBuffer))

			_, err := writer.activeFile.Write(writeBuffer)
			if err == nil {
				err = writer.activeFile.Sync()
			}
			if err == nil {
				writer.currentSizeBytes += int64(len(writeBuffer))
			}

			if err != nil {
				slog.Debug("batch worker: fsync failed", "error", err)
			} else {
				slog.Debug("batch worker: fsync successful")
			}

			for _, ticket := range commitBatch {
				ticket.resultChan <- err
			}
		}
	}
}

// Close closes the active WAL segment file, terminates the background worker,
// and ensures all pending writes have been durably synced to disk.
func (writer *LogWriter) Close() error {
	var closeErr error
	writer.closeOnce.Do(func() {
		close(writer.shutdownSignal)
		writer.workerWaitGroup.Wait()
		if writer.activeFile != nil {
			if syncErr := writer.activeFile.Sync(); syncErr != nil {
				closeErr = syncErr
			}
			if err := writer.activeFile.Close(); err != nil && closeErr == nil {
				closeErr = err
			}
		}
	})
	return closeErr
}
