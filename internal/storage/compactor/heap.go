package compactor

import (
	"bytes"

	"github.com/makeshift-engineering/penguin-db/internal/storage/sstable"
)

type MergeNode struct {
	Key      []byte
	Value    []byte
	Opcode   uint8
	FileID   int
	Iterator *sstable.Iterator
}

type MergeHeap []*MergeNode

func (heap MergeHeap) Len() int { return len(heap) }

func (heap MergeHeap) Swap(i, j int) { heap[i], heap[j] = heap[j], heap[i] }

func (heap MergeHeap) Less(i, j int) bool {
	cmp := bytes.Compare(heap[i].Key, heap[j].Key)
	if cmp != 0 {
		return cmp < 0
	}

	return heap[i].FileID > heap[j].FileID
}

func (heap *MergeHeap) Push(x any) {
	*heap = append(*heap, x.(*MergeNode))
}

func (h *MergeHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*h = old[0 : n-1]
	return item
}
