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

	iterators, minHeap, err := initializeInputs(task, config)
	if err != nil {
		return nil, err
	}
	defer func() {
		for _, it := range iterators {
			_ = it.Close()
		}
	}()

	outFilePath, writer, err := createOutputWriter(task, config)
	if err != nil {
		return nil, err
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

	keysWritten, bytesWritten, err := performMerge(task, minHeap, writer)
	if err != nil {
		return nil, err
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

// createOutputWriter prepares a new output SSTable file and initializes
// a writer to store the compacted data, using the estimated keys from the configuration.
func createOutputWriter(task *Task, config *Options) (outFilePath string, writer *sstable.Writer, err error) {
	outFileName := fmt.Sprintf("%06d.sst", task.NextSegmentID)
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
func performMerge(task *Task, minHeap *MergeHeap, writer *sstable.Writer) (keysWritten uint32, bytesWritten uint64, err error) {
	var lastKey []byte

	for minHeap.Len() > 0 {
		node := (*minHeap)[0]

		if lastKey != nil && bytes.Equal(lastKey, node.Key) {
			if err := fixOrPop(minHeap, node); err != nil {
				return 0, 0, err
			}
			continue
		}

		if task.IsBottomLevel && node.Opcode == sstable.OpcodeDelete {
			lastKey = append(lastKey[:0], node.Key...)
			if err := fixOrPop(minHeap, node); err != nil {
				return 0, 0, err
			}
			continue
		}

		if err := writer.Add(node.Key, node.Value, node.Opcode); err != nil {
			return 0, 0, fmt.Errorf("failed to write to output sstable: %w", err)
		}

		lastKey = append(lastKey[:0], node.Key...)

		if err := fixOrPop(minHeap, node); err != nil {
			return 0, 0, err
		}
	}

	return writer.EntryCount(), writer.Size(), nil
}

// fixOrPop advances the iterator of the root node.
// If another entry is available, it updates the node in-place and calls heap.Fix to re-order the heap.
// If the iterator is exhausted, it removes the node from the heap using heap.Pop.
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
