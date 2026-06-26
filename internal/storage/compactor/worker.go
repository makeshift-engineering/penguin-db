package compactor

import (
	"bytes"
	"container/heap"
	"fmt"
	"os"
	"path/filepath"

	"github.com/makeshift-engineering/penguin-db/internal/storage/sstable"
)

// Default reader buffer sizes and estimated key sizes for SSTable generation.
const (
	DefaultReaderBufferSize = 1024 * 1024
	DefaultEstimatedKeys    = 100000
)

// Options holds runtime parameters for compaction buffer size and estimated key size.
type Options struct {
	ReadBufferSize int
	EstimatedKeys  int
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

// Run executes the multi-way merge compaction algorithm on the inputs defined by Task.
// It opens each input SSTable, merges and deduplicates their keys using a min-heap,
// and writes the result to a new compacted SSTable file. In bottom-level compactions,
// expired tombstone entries (Delete opcode) are elided.
func Run(task *Task, opts ...Option) (res *Result, err error) {
	if err := task.Validate(); err != nil {
		return nil, err
	}

	config := &Options{
		ReadBufferSize: DefaultReaderBufferSize,
		EstimatedKeys:  DefaultEstimatedKeys,
	}

	for _, opt := range opts {
		opt(config)
	}

	var iterators []*sstable.Iterator
	defer func() {
		for _, it := range iterators {
			_ = it.Close()
		}
	}()

	var minHeap MergeHeap
	heap.Init(&minHeap)

	for i, filePath := range task.InputFiles {
		reader, err := sstable.Open(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to open sstable %s: %w", filePath, err)
		}

		iter, err := reader.NewIterator(
			sstable.WithBufferSize(config.ReadBufferSize),
		)
		reader.Close()

		if err != nil {
			return nil, fmt.Errorf("failed to create iterator for %s: %w", filePath, err)
		}

		iterators = append(iterators, iter)

		if iter.Next() {
			heap.Push(&minHeap, &MergeNode{
				Key:      iter.Key(),
				Value:    iter.Value(),
				Opcode:   iter.Opcode(),
				FileID:   task.FileIDs[i],
				Iterator: iter,
			})
		} else if err := iter.Error(); err != nil {
			return nil, fmt.Errorf("iterator error on initialization: %w", err)
		}
	}

	outFileName := fmt.Sprintf("%06d.sst", task.NextSegmentID)
	outFilePath := filepath.Join(task.OutputDirectory, outFileName)

	writer, err := sstable.NewWriter(outFilePath, config.EstimatedKeys)
	if err != nil {
		return nil, fmt.Errorf("failed to create compaction writer: %w", err)
	}
	var writerClosed bool
	defer func() {
		if err != nil {
			_ = writer.Close()
			_ = os.Remove(outFilePath)
			return
		}
		if !writerClosed {
			_ = writer.Close()
		}
	}()

	var lastKey []byte
	var keysWritten uint32
	var bytesWritten uint64

	for minHeap.Len() > 0 {
		node := heap.Pop(&minHeap).(*MergeNode)

		if lastKey != nil && bytes.Equal(lastKey, node.Key) {
			if err := advanceAndPush(&minHeap, node); err != nil {
				return nil, err
			}
			continue
		}

		if task.IsBottomLevel && node.Opcode == sstable.OpcodeDelete {
			lastKey = append(lastKey[:0], node.Key...)
			if err := advanceAndPush(&minHeap, node); err != nil {
				return nil, err
			}
			continue
		}

		if err := writer.Add(node.Key, node.Value, node.Opcode); err != nil {
			return nil, fmt.Errorf("failed to write to output sstable: %w", err)
		}

		keysWritten++
		bytesWritten += uint64(len(node.Key) + len(node.Value) + 7)
		lastKey = append(lastKey[:0], node.Key...)

		if err := advanceAndPush(&minHeap, node); err != nil {
			return nil, err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize compacted sstable: %w", err)
	}
	writerClosed = true

	return &Result{
		NewFilesCreated: []string{outFilePath},
		ObsoleteFiles:   task.InputFiles,
		BytesWritten:    bytesWritten,
		KeysWritten:     keysWritten,
	}, nil
}

// advanceAndPush advances the iterator of the given merge node.
// If another entry is available, it updates the node and pushes it back into the heap.
// If an error is encountered during iterator advancement, it is returned immediately.
func advanceAndPush(h *MergeHeap, node *MergeNode) error {
	if node.Iterator.Next() {
		node.Key = node.Iterator.Key()
		node.Value = node.Iterator.Value()
		node.Opcode = node.Iterator.Opcode()
		heap.Push(h, node)
	} else if err := node.Iterator.Error(); err != nil {
		return err
	}
	return nil
}
