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

	var currentWriter *sstable.Writer
	var currentFilePath string
	var nextSegmentID = task.NextSegmentID
	var newFilesCreated []string
	var writerClosed bool

	// rollWriter closes the currently active writer (if any), records the finalized
	// file path, and initializes a new sstable.Writer for the next segment.
	rollWriter := func() error {
		if currentWriter != nil {
			if err := currentWriter.Close(); err != nil {
				return err
			}
			newFilesCreated = append(newFilesCreated, currentFilePath)
			currentWriter = nil
			currentFilePath = ""
		}

		outFileName := fmt.Sprintf("%06d.sst", nextSegmentID)
		currentFilePath = filepath.Join(task.OutputDirectory, outFileName)
		nextSegmentID++

		w, err := sstable.NewWriter(currentFilePath, config.EstimatedKeys)
		if err != nil {
			return err
		}
		currentWriter = w
		return nil
	}

	// Defer cleanup to close any active writer and delete all created files
	// on failure (preventing partial compaction files from leaking).
	defer func() {
		if err != nil {
			if currentWriter != nil {
				_ = currentWriter.Close()
			}
			if currentFilePath != "" {
				_ = os.Remove(currentFilePath)
			}
			for _, f := range newFilesCreated {
				_ = os.Remove(f)
			}
			return
		}
		if !writerClosed && currentWriter != nil {
			_ = currentWriter.Close()
		}
	}()

	if err := rollWriter(); err != nil {
		return nil, err
	}

	var lastKey []byte
	var keysWritten uint32
	var bytesWritten uint64

	// Multi-way merge loop. We retrieve the smallest key from the min-heap,
	// write it to the current segment, and advance the corresponding iterator.
	for minHeap.Len() > 0 {
		node := (*minHeap)[0]

		// Skip duplicate keys (retaining the version from the newest input file,
		// which was pushed last and thus resides on top).
		if lastKey != nil && bytes.Equal(lastKey, node.Key) {
			if err := fixOrPop(minHeap, node); err != nil {
				return nil, err
			}
			continue
		}

		// Elide tombstones if this is a bottom-level compaction.
		if task.IsBottomLevel && node.Opcode == sstable.OpcodeDelete {
			lastKey = append(lastKey[:0], node.Key...)
			if err := fixOrPop(minHeap, node); err != nil {
				return nil, err
			}
			continue
		}

		// Write entry to current SSTable writer.
		if err := currentWriter.Add(node.Key, node.Value, node.Opcode); err != nil {
			return nil, fmt.Errorf("failed to write to output sstable: %w", err)
		}

		keysWritten++
		bytesWritten += uint64(len(node.Key) + len(node.Value) + 7)
		lastKey = append(lastKey[:0], node.Key...)

		// Check if the current writer has exceeded the target size threshold.
		if currentWriter.EstimatedSize() >= config.MaxSSTableSize {
			if err := rollWriter(); err != nil {
				return nil, fmt.Errorf("failed to roll compaction writer: %w", err)
			}
		}

		if err := fixOrPop(minHeap, node); err != nil {
			return nil, err
		}
	}

	// Finalize the last active SSTable.
	if currentWriter != nil {
		if err := currentWriter.Close(); err != nil {
			return nil, fmt.Errorf("failed to finalize compacted sstable: %w", err)
		}
		newFilesCreated = append(newFilesCreated, currentFilePath)
	}
	writerClosed = true

	return &Result{
		NewFilesCreated: newFilesCreated,
		ObsoleteFiles:   task.InputFiles,
		BytesWritten:    bytesWritten,
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
		reader, err := sstable.Open(filePath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to open sstable %s: %w", filePath, err)
		}

		iter, err := reader.NewIterator(
			sstable.WithBufferSize(config.ReadBufferSize),
		)
		reader.Close()

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
