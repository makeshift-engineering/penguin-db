package compactor

import (
	"bytes"
	"container/heap"
	"fmt"
	"path/filepath"

	"github.com/makeshift-engineering/penguin-db/internal/storage/sstable"
)

const (
	DefaultReaderBufferSize = 1024 * 1024
	DefaultEstimatedKeys    = 100000
)

type Options struct {
	ReadBufferSize int
	EstimatedKeys  int
}

type Option func(*Options)

func WithReadBufferSize(size int) Option {
	return func(option *Options) {
		if size > 0 {
			option.ReadBufferSize = size
		}
	}
}

func WithEstimatedKeys(keys int) Option {
	return func(option *Options) {
		if keys > 0 {
			option.EstimatedKeys = keys
		}
	}
}

func Run(task *Task, opts ...Option) (*Result, error) {
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

	return &Result{
		NewFilesCreated: []string{outFilePath},
		ObsoleteFiles:   task.InputFiles,
		BytesWritten:    bytesWritten,
		KeysWritten:     keysWritten,
	}, nil

}

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
