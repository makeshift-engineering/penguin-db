package compactor

import (
	"bytes"

	"github.com/makeshift-engineering/penguin-db/internal/storage/sstable"
)

// MergeNode represents a single sorted stream element in the merge heap.
// It tracks the current key-value pair, operation code, source file identifier,
// and the iterator to fetch subsequent entries.
type MergeNode struct {
	Key      []byte
	Value    []byte
	Opcode   uint8
	FileID   int
	Iterator *sstable.Iterator
}

// MergeHeap is a collection of MergeNodes used to perform a multi-way merge sort.
// It implements the container/heap.Interface.
type MergeHeap []*MergeNode

// Len returns the number of elements currently in the heap.
func (heap MergeHeap) Len() int { return len(heap) }

// Swap swaps the elements at indexes i and j.
func (heap MergeHeap) Swap(i, j int) { heap[i], heap[j] = heap[j], heap[i] }

// Less reports whether the element at index i should sort before the element at index j.
// It sorts keys lexicographically. For identical keys, the node with the larger FileID
// (representing newer data) is sorted first to ensure newer entries overwrite older ones.
func (heap MergeHeap) Less(i, j int) bool {
	cmp := bytes.Compare(heap[i].Key, heap[j].Key)
	if cmp != 0 {
		return cmp < 0
	}

	return heap[i].FileID > heap[j].FileID
}

// Push adds x as a *MergeNode to the heap slice.
func (heap *MergeHeap) Push(x any) {
	*heap = append(*heap, x.(*MergeNode))
}

// Pop removes and returns the minimum element from the heap slice.
func (heap *MergeHeap) Pop() any {
	h := *heap
	lastIdx := len(h) - 1

	item := h[lastIdx]
	h[lastIdx] = nil
	*heap = h[:lastIdx]

	return item
}
