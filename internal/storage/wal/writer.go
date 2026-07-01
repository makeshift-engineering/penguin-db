package wal

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

const (
	// maxFrameSizeBytes is the upper bound on a single serialized WAL frame.
	// It is the single source of truth: MaxSegmentSizeBytes is derived from it,
	// ensuring a record can never be larger than one segment.
	maxFrameSizeBytes = 32 * 1024 * 1024 // 32 MiB

	// MaxSegmentSizeBytes is the maximum size a WAL segment file may grow to
	// before the writer rotates to a new segment.
	MaxSegmentSizeBytes int64 = maxFrameSizeBytes

	// MaxBatchSizeBytes is the maximum number of bytes gathered into a single
	// group-commit batch before writing to disk.
	MaxBatchSizeBytes int64 = 4 * 1024 * 1024

	// ingestChannelCapacity is the number of in-flight uncommitted tickets that
	// can queue in the ingestion channel before callers block. Sized to absorb
	// burst writes at typical database workloads (~10k concurrent operations).
	ingestChannelCapacity = 10_000
)

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
	options          Options

	ingestionChannel chan *commitTicket
	stateMutex       sync.RWMutex
	isClosed         bool
	terminalErr      error

	workerWaitGroup sync.WaitGroup
	closeOnce       sync.Once
}

// Options configures the behavior of a LogWriter.
type Options struct {
	// SegmentSizeBytes is the maximum size a WAL segment file may grow to
	// before the writer rotates to a new segment.
	SegmentSizeBytes int64

	// BatchSizeBytes is the maximum number of bytes gathered into a single
	// group-commit batch before writing to disk.
	BatchSizeBytes int64

	// IngestChannelCapacity is the number of in-flight uncommitted tickets that
	// can queue in the ingestion channel before callers block.
	IngestChannelCapacity int
}

// DefaultOptions returns the default configuration options for LogWriter.
func DefaultOptions() Options {
	return Options{
		SegmentSizeBytes:      MaxSegmentSizeBytes,
		BatchSizeBytes:        MaxBatchSizeBytes,
		IngestChannelCapacity: ingestChannelCapacity,
	}
}

// Option is a functional option for configuring a LogWriter.
type Option func(*Options)

// WithSegmentSizeBytes sets the maximum segment size.
func WithSegmentSizeBytes(size int64) Option {
	return func(o *Options) {
		o.SegmentSizeBytes = size
	}
}

// WithBatchSizeBytes sets the maximum batch size for group commits.
func WithBatchSizeBytes(size int64) Option {
	return func(o *Options) {
		o.BatchSizeBytes = size
	}
}

// WithIngestChannelCapacity sets the ingestion channel capacity.
func WithIngestChannelCapacity(capacity int) Option {
	return func(o *Options) {
		o.IngestChannelCapacity = capacity
	}
}

// NewLogWriter creates a new LogWriter instance, initializing the WAL directory,
// creating the initial active segment, and launching the background batch worker.
// Configuration can be customized by passing Option functional parameters.
func NewLogWriter(directory string, nextSegmentID int, opts ...Option) (*LogWriter, error) {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, fmt.Errorf("failed to initialize WAL directory structure at %s: %w", directory, err)
	}

	options := DefaultOptions()
	for _, opt := range opts {
		opt(&options)
	}

	if options.SegmentSizeBytes <= 0 {
		return nil, fmt.Errorf("SegmentSizeBytes must be greater than 0")
	}
	if options.BatchSizeBytes <= 0 {
		return nil, fmt.Errorf("BatchSizeBytes must be greater than 0")
	}
	if options.IngestChannelCapacity <= 0 {
		return nil, fmt.Errorf("IngestChannelCapacity must be greater than 0")
	}

	writer := &LogWriter{
		directory:        directory,
		currentSegmentID: nextSegmentID,
		options:          options,
		ingestionChannel: make(chan *commitTicket, options.IngestChannelCapacity),
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

	info, err := file.Stat()
	if err != nil {
		closeErr := file.Close()
		return fmt.Errorf("failed to stat WAL segment %s: %w", segmentPath, errors.Join(err, closeErr))
	}

	writer.activeFile = file
	writer.currentSizeBytes = info.Size()
	return nil
}

// AppendBatch writes a slice of Records into the Write-Ahead Log as a single
// atomic unit. All records are pre-marshalled into one contiguous byte buffer
// and submitted to the batch worker as a single commit ticket. This guarantees
// that either all records are durably written and synced, or none are — making
// it safe to use for atomic multi-operation batches (e.g., WriteBatch).
//
// AppendBatch blocks until the combined write is durably persisted or the log
// is closed. An error is returned if any record fails to marshal, or if the
// combined write fails.
func (writer *LogWriter) AppendBatch(records []*Record) error {
	if len(records) == 0 {
		return nil
	}

	// Pre-validate and marshal all records into a single contiguous buffer.
	// This happens before acquiring any lock so that marshalling errors are
	// returned before any I/O is attempted.
	var combinedFrameBuffer []byte
	for i, record := range records {
		if len(record.Key) == 0 {
			return fmt.Errorf("record at index %d: %w", i, ErrEmptyKey)
		}
		if record.Opcode != OpcodePut && record.Opcode != OpcodeDelete {
			return fmt.Errorf("record at index %d: %w", i, ErrInvalidOpcode)
		}
		frame, err := record.Marshal()
		if err != nil {
			return fmt.Errorf("record at index %d: failed to marshal: %w", i, err)
		}
		combinedFrameBuffer = append(combinedFrameBuffer, frame...)
	}

	// Submit the entire pre-marshalled buffer as one atomic commit ticket.
	// The batch worker will write and fsync it in a single I/O pass.
	ticket := &commitTicket{
		frameData:  combinedFrameBuffer,
		resultChan: make(chan error, 1),
	}

	writer.stateMutex.RLock()
	if writer.isClosed {
		writer.stateMutex.RUnlock()
		return ErrWriterClosed
	}
	if writer.terminalErr != nil {
		writer.stateMutex.RUnlock()
		return writer.terminalErr
	}

	writer.ingestionChannel <- ticket
	writer.stateMutex.RUnlock()

	return <-ticket.resultChan
}

// Append writes a single Record into the Write-Ahead Log. It blocks until the
// record is durably persisted (written and synced) or the log is closed.
func (writer *LogWriter) Append(record *Record) error {
	if len(record.Key) == 0 {
		return ErrEmptyKey
	}

	if record.Opcode != OpcodePut && record.Opcode != OpcodeDelete {
		return ErrInvalidOpcode
	}

	frame, err := record.Marshal()
	if err != nil {
		return err
	}

	ticket := &commitTicket{
		frameData:  frame,
		resultChan: make(chan error, 1),
	}

	writer.stateMutex.RLock()
	if writer.isClosed {
		writer.stateMutex.RUnlock()
		return ErrWriterClosed
	}
	if writer.terminalErr != nil {
		writer.stateMutex.RUnlock()
		return writer.terminalErr
	}

	slog.Debug("caller: enqueuing record into ingestion channel", "frame_size", len(frame))
	writer.ingestionChannel <- ticket
	writer.stateMutex.RUnlock()

	return <-ticket.resultChan
}

// batchWorker runs in a background goroutine, receiving commit tickets from the
// ingestion channel, batching them, writing to the active file, and syncing them.
func (writer *LogWriter) batchWorker() {
	defer writer.workerWaitGroup.Done()

	var commitBatch []*commitTicket
	var writeBuffer []byte
	var leftoverTicket *commitTicket

	for {
		writer.stateMutex.RLock()
		tErr := writer.terminalErr
		writer.stateMutex.RUnlock()
		if tErr != nil {
			if leftoverTicket != nil {
				leftoverTicket.resultChan <- tErr
			}
			for ticket := range writer.ingestionChannel {
				ticket.resultChan <- tErr
			}
			break
		}

		var ticket *commitTicket
		if leftoverTicket != nil {
			ticket = leftoverTicket
			leftoverTicket = nil
		} else {
			var ok bool
			ticket, ok = <-writer.ingestionChannel
			if !ok {
				break
			}
		}

		commitBatch, writeBuffer, leftoverTicket = writer.gatherBatch(ticket, commitBatch, writeBuffer)

		slog.Debug("batch worker: executing group commit",
			"batch_size", len(commitBatch),
			"total_bytes", len(writeBuffer))

		writer.writeAndSyncBatch(commitBatch, writeBuffer)
	}
}

// Close closes the active WAL segment file, terminates the background worker,
// and ensures all pending writes have been durably synced to disk.
func (writer *LogWriter) Close() error {
	var closeErr error
	writer.closeOnce.Do(func() {
		writer.stateMutex.Lock()
		writer.isClosed = true
		close(writer.ingestionChannel)
		writer.stateMutex.Unlock()

		writer.workerWaitGroup.Wait()

		if writer.activeFile != nil {
			syncErr := writer.activeFile.Sync()
			fileCloseErr := writer.activeFile.Close()
			closeErr = errors.Join(syncErr, fileCloseErr)
		}
	})
	return closeErr
}

// writeAndSyncBatch handles the disk I/O, segment rotation, and caller goroutine
// notification for a gathered batch of records.
func (writer *LogWriter) writeAndSyncBatch(batch []*commitTicket, buffer []byte) {
	if writer.currentSizeBytes+int64(len(buffer)) > writer.options.SegmentSizeBytes {
		if err := writer.rotateActiveFile(); err != nil {
			writer.markTerminalError(err)
			for _, ticket := range batch {
				ticket.resultChan <- err
			}
			return
		}
	}

	n, err := writer.activeFile.Write(buffer)
	if err == nil && n < len(buffer) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = writer.activeFile.Sync()
	}

	if err == nil {
		writer.currentSizeBytes += int64(len(buffer))
	} else {
		slog.Debug("batch worker: fsync/write failed", "error", err)
		writer.markTerminalError(err)
	}

	for _, ticket := range batch {
		ticket.resultChan <- err
	}
}

// markTerminalError sets a terminal error state under the state mutex.
func (writer *LogWriter) markTerminalError(err error) {
	writer.stateMutex.Lock()
	if writer.terminalErr == nil {
		writer.terminalErr = fmt.Errorf("terminal WAL I/O error: %w", err)
	}
	writer.stateMutex.Unlock()
}

// gatherBatch pulls tickets from the ingestion channel up to the BatchSizeBytes limit.
// It takes the first ticket and drains the remaining buffered tickets.
// If a ticket would cause the batch to exceed BatchSizeBytes, it is returned as a leftover.
func (writer *LogWriter) gatherBatch(firstTicket *commitTicket, inBatch []*commitTicket, inBuffer []byte) (outBatch []*commitTicket, outBuffer []byte, leftover *commitTicket) {
	outBatch = inBatch[:0]
	outBuffer = inBuffer[:0]

	outBatch = append(outBatch, firstTicket)
	outBuffer = append(outBuffer, firstTicket.frameData...)

	pendingWrites := len(writer.ingestionChannel)
	for range pendingWrites {
		ticket, ok := <-writer.ingestionChannel
		if !ok {
			break
		}

		if int64(len(outBuffer)+len(ticket.frameData)) > writer.options.BatchSizeBytes {
			leftover = ticket
			break
		}

		outBatch = append(outBatch, ticket)
		outBuffer = append(outBuffer, ticket.frameData...)
	}

	return outBatch, outBuffer, leftover
}
