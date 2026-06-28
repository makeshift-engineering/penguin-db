package compactor

import (
	"bytes"
	"container/heap"
	"fmt"
	"os"
	"path/filepath"

	"github.com/makeshift-engineering/penguin-db/internal/storage/sstable"
)

// Default reader buffer sizes, estimated key sizes, and target file size limits for SSTable generation.
const (
	DefaultReaderBufferSize = 1024 * 1024
	DefaultEstimatedKeys    = 100000
	DefaultMaxSSTableSize   = 2 * 1024 * 1024
)

// Options holds runtime configuration parameters for compaction input buffer size,
// estimated keys count, and the maximum allowed size of generated SSTable segments.
type Options struct {
	ReadBufferSize int
	EstimatedKeys  int
	MaxSSTableSize uint64
}

// Option configures custom parameter fields inside Options.
type Option func(*Options)

// WithReadBufferSize configures a custom read buffer size for input SSTable iterators.
func WithReadBufferSize(size int) Option {
	return func(option *Options) {
		if size > 0 {
			option.ReadBufferSize = size
		}
	}
}

// WithEstimatedKeys specifies the estimated keys count to optimize bloom filters for the output SSTable.
func WithEstimatedKeys(keys int) Option {
	return func(option *Options) {
		if keys > 0 {
			option.EstimatedKeys = keys
		}
	}
}

// WithMaxSSTableSize sets the maximum size limit for generated SSTables. If the estimated
// size of the current SSTable exceeds this limit, the compactor will close the current
// file and roll over to a new segment.
func WithMaxSSTableSize(size uint64) Option {
	return func(option *Options) {
		if size > 0 {
			option.MaxSSTableSize = size
		}
	}
}

// Run executes the multi-way merge compaction algorithm on the inputs defined by Task.
// It opens each input SSTable, merges and deduplicates their keys using a min-heap,
// and writes the result to one or more new compacted SSTable files. If a generated SSTable
// exceeds the configured MaxSSTableSize limit, the compactor rolls over to write the
// subsequent keys to a new SSTable file. In bottom-level compactions, expired tombstone
// entries (Delete opcode) are elided.
func Run(task *Task, opts ...Option) (res *Result, err error) {
	if err := task.Validate(); err != nil {
		return nil, err
	}

	config := &Options{
		ReadBufferSize: DefaultReaderBufferSize,
		EstimatedKeys:  DefaultEstimatedKeys,
		MaxSSTableSize: DefaultMaxSSTableSize,
	}

	for _, opt := range opts {
		opt(config)
	}

	iterators, minHeap, err := initializeInputs(task, config)
	if err != nil {
		return nil, err
	}
	defer func() {
		for _, it := range iterators {
			_ = it.Close()
		}
	}()

	newFilesCreated, keysWritten, err := performMerge(task, minHeap, config)
	if err != nil {
		return nil, err
	}

	// Calculate the actual total bytes written on disk across all segments.
	var totalBytesWritten uint64
	for _, path := range newFilesCreated {
		fi, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("failed to stat compacted file %s: %w", path, err)
		}
		totalBytesWritten += uint64(fi.Size())
	}

	return &Result{
		NewFilesCreated: newFilesCreated,
		ObsoleteFiles:   task.InputFiles,
		BytesWritten:    totalBytesWritten,
		KeysWritten:     keysWritten,
	}, nil
}

// initializeInputs opens the input SSTables specified in the compaction task,
// creates an iterator for each, and initializes the multi-way merge min-heap
// with the first valid entry from each iterator.
func initializeInputs(task *Task, config *Options) (iterators []*sstable.Iterator, heapPtr *MergeHeap, err error) {
	var minHeap MergeHeap
	heap.Init(&minHeap)

	var iters []*sstable.Iterator
	var success bool
	defer func() {
		if !success {
			for _, it := range iters {
				_ = it.Close()
			}
		}
	}()

	for i, filePath := range task.InputFiles {
		iter, err := sstable.NewIterator(
			filePath,
			sstable.WithBufferSize(config.ReadBufferSize),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create iterator for %s: %w", filePath, err)
		}

		iters = append(iters, iter)

		if iter.Next() {
			heap.Push(&minHeap, &MergeNode{
				Key:      iter.Key(),
				Value:    iter.Value(),
				Opcode:   iter.Opcode(),
				FileID:   task.FileIDs[i],
				Iterator: iter,
			})
		} else if err := iter.Error(); err != nil {
			return nil, nil, fmt.Errorf("iterator error on initialization: %w", err)
		}
	}

	success = true
	return iters, &minHeap, nil
}

// createOutputWriter prepares a new output SSTable file and initializes
// a writer to store the compacted data, using the estimated keys from the configuration.
func createOutputWriter(task *Task, config *Options, segmentID int) (outFilePath string, writer *sstable.Writer, err error) {
	outFileName := fmt.Sprintf("%06d.sst", segmentID)
	outFilePath = filepath.Join(task.OutputDirectory, outFileName)

	writer, err = sstable.NewWriter(outFilePath, config.EstimatedKeys)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create compaction writer: %w", err)
	}

	return outFilePath, writer, nil
}

// performMerge executes the multi-way merge logic. It continually pops the minimum
// entry from the heap, deduplicates identical keys, elides tombstone entries if
// applicable, and adds the resulting entries to the output SSTable writer.
// It rolls over and creates multiple SSTable files when size limits are exceeded.
// compactionState manages the writer rollover and file creation state during a compaction run.
type compactionState struct {
	task            *Task
	config          *Options
	currentWriter   *sstable.Writer
	currentFilePath string
	nextSegmentID   int
	newFilesCreated []string
}

// openWriter creates a new SSTable segment writer.
func (s *compactionState) openWriter() error {
	path, w, err := createOutputWriter(s.task, s.config, s.nextSegmentID)
	if err != nil {
		return err
	}
	s.currentFilePath = path
	s.currentWriter = w
	s.nextSegmentID++
	return nil
}

// finalizeWriter closes and finalizes the active SSTable writer.
func (s *compactionState) finalizeWriter() error {
	if s.currentWriter == nil {
		return nil
	}
	if err := s.currentWriter.Close(); err != nil {
		return err
	}
	s.newFilesCreated = append(s.newFilesCreated, s.currentFilePath)
	s.currentWriter = nil
	s.currentFilePath = ""
	return nil
}

// performMerge executes the multi-way merge logic. It continually pops the minimum
// entry from the heap, deduplicates identical keys, elides tombstone entries if
// applicable, and adds the resulting entries to the output SSTable writer.
// It rolls over and creates multiple SSTable files when size limits are exceeded.
func performMerge(task *Task, minHeap *MergeHeap, config *Options) (newFiles []string, keysWritten uint32, err error) {
	state := &compactionState{
		task:          task,
		config:        config,
		nextSegmentID: task.NextSegmentID,
	}

	var writerClosed bool
	defer func() {
		if err != nil {
			if state.currentWriter != nil {
				_ = state.currentWriter.Close()
			}
			if state.currentFilePath != "" {
				_ = os.Remove(state.currentFilePath)
			}
			for _, f := range state.newFilesCreated {
				_ = os.Remove(f)
			}
			return
		}
		if !writerClosed && state.currentWriter != nil {
			_ = state.currentWriter.Close()
		}
	}()

	var lastKey []byte

	for minHeap.Len() > 0 {
		node := (*minHeap)[0]

		// Skip duplicate keys (retaining the version from the newest input file,
		// which was pushed last and thus resides on top).
		if lastKey != nil && bytes.Equal(lastKey, node.Key) {
			if err := fixOrPop(minHeap, node); err != nil {
				return nil, 0, err
			}
			continue
		}

		// Elide tombstones if this is a bottom-level compaction.
		if task.IsBottomLevel && node.Opcode == sstable.OpcodeDelete {
			lastKey = append(lastKey[:0], node.Key...)
			if err := fixOrPop(minHeap, node); err != nil {
				return nil, 0, err
			}
			continue
		}

		// Lazily open the output writer if it is not already initialized.
		if state.currentWriter == nil {
			if err := state.openWriter(); err != nil {
				return nil, 0, fmt.Errorf("failed to create compaction writer: %w", err)
			}
		}

		// Write entry to current SSTable writer.
		if err := state.currentWriter.Add(node.Key, node.Value, node.Opcode); err != nil {
			return nil, 0, fmt.Errorf("failed to write to output sstable: %w", err)
		}

		keysWritten++
		lastKey = append(lastKey[:0], node.Key...)

		// Check if the current writer has exceeded the target size threshold.
		if state.currentWriter.EstimatedSize() >= config.MaxSSTableSize {
			if err := state.finalizeWriter(); err != nil {
				return nil, 0, fmt.Errorf("failed to roll compaction writer: %w", err)
			}
		}

		if err := fixOrPop(minHeap, node); err != nil {
			return nil, 0, err
		}
	}

	// Finalize the last active SSTable.
	if err := state.finalizeWriter(); err != nil {
		return nil, 0, fmt.Errorf("failed to finalize compacted sstable: %w", err)
	}
	writerClosed = true

	return state.newFilesCreated, keysWritten, nil
}

// fixOrPop advances the iterator of the root merge node in the heap.
// If the iterator has more entries, the node is updated in-place and heap.Fix is
// called to re-establish the heap invariant. If the iterator is exhausted, the node
// is popped and removed from the heap.
func fixOrPop(h *MergeHeap, node *MergeNode) (err error) {
	if node.Iterator.Next() {
		node.Key = node.Iterator.Key()
		node.Value = node.Iterator.Value()
		node.Opcode = node.Iterator.Opcode()
		heap.Fix(h, 0)
	} else {
		if err := node.Iterator.Error(); err != nil {
			return err
		}
		heap.Pop(h)
	}
	return nil
}
