package wal

import (
	"log/slog"
	"os"
	"sync"
)

type commitTicket struct {
	frameData  []byte
	resultChan chan error
}

type LogWriter struct {
	activeFile       *os.File
	ingestionChannel chan *commitTicket
	shutdownSignal   chan struct{}
	workerWaitGroup  sync.WaitGroup
}

func NewLogWriter(filePath string) (*LogWriter, error) {
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	writer := &LogWriter{
		activeFile:       file,
		ingestionChannel: make(chan *commitTicket, 10000),
		shutdownSignal:   make(chan struct{}),
	}

	writer.workerWaitGroup.Add(1)
	go writer.batchWorker()

	return writer, nil
}

func (writer *LogWriter) Append(record *Record) error {
	frame := record.Marshal()

	ticket := &commitTicket{
		frameData:  frame,
		resultChan: make(chan error, 1),
	}

	slog.Debug("network thread: dropping record into ingestion channel", "frame_size", len(frame))

	select {
	case writer.ingestionChannel <- ticket:
		return <-ticket.resultChan
	case <-writer.shutdownSignal:
		return os.ErrClosed
	}
}

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

			slog.Debug("batch worker: executing group commit",
				"batch_size", len(commitBatch),
				"total_bytes", len(writeBuffer))

			_, err := writer.activeFile.Write(writeBuffer)
			if err == nil {
				err = writer.activeFile.Sync()
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

func (writer *LogWriter) Close() error {
	close(writer.shutdownSignal)
	writer.workerWaitGroup.Wait()
	return writer.activeFile.Close()
}
