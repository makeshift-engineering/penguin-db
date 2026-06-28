package compactor

import (
	"bytes"
	"container/heap"
	"testing"
)

// TestMergeHeap_Len verifies that the Len method correctly returns the number of elements
// in the heap, testing both empty and populated states.
func TestMergeHeap_Len(t *testing.T) {
	h := &MergeHeap{}
	if h.Len() != 0 {
		t.Errorf("Expected length 0, got %d", h.Len())
	}
	heap.Push(h, &MergeNode{Key: []byte("a")})
	if h.Len() != 1 {
		t.Errorf("Expected length 1, got %d", h.Len())
	}
}

// TestMergeHeap_Swap verifies that the Swap method correctly swaps the elements
// at the specified indices in the underlying slice.
func TestMergeHeap_Swap(t *testing.T) {
	n1 := &MergeNode{Key: []byte("a")}
	n2 := &MergeNode{Key: []byte("b")}
	h := MergeHeap{n1, n2}
	h.Swap(0, 1)
	if h[0] != n2 || h[1] != n1 {
		t.Errorf("Swap failed")
	}
}

// TestMergeHeap_Less verifies the Less method's sorting logic. It ensures that
// nodes are sorted primarily by their Keys lexicographically, and secondarily
// by their FileID in descending order (newer data first) when keys match.
func TestMergeHeap_Less(t *testing.T) {
	tests := []struct {
		name string
		i    *MergeNode
		j    *MergeNode
		want bool
	}{
		{
			name: "Key i < Key j",
			i:    &MergeNode{Key: []byte("a"), FileID: 1},
			j:    &MergeNode{Key: []byte("b"), FileID: 1},
			want: true,
		},
		{
			name: "Key i > Key j",
			i:    &MergeNode{Key: []byte("c"), FileID: 1},
			j:    &MergeNode{Key: []byte("b"), FileID: 1},
			want: false,
		},
		{
			name: "Key i == Key j, FileID i > FileID j",
			i:    &MergeNode{Key: []byte("a"), FileID: 2},
			j:    &MergeNode{Key: []byte("a"), FileID: 1},
			want: true,
		},
		{
			name: "Key i == Key j, FileID i < FileID j",
			i:    &MergeNode{Key: []byte("a"), FileID: 1},
			j:    &MergeNode{Key: []byte("a"), FileID: 2},
			want: false,
		},
		{
			name: "Key i == Key j, FileID i == FileID j",
			i:    &MergeNode{Key: []byte("a"), FileID: 1},
			j:    &MergeNode{Key: []byte("a"), FileID: 1},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := MergeHeap{tt.i, tt.j}
			if got := h.Less(0, 1); got != tt.want {
				t.Errorf("MergeHeap.Less() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMergeHeap_PushPop validates the integration of MergeHeap with the
// standard library's container/heap package. It pushes multiple nodes and
// verifies that they are popped out in the correct sorted order.
func TestMergeHeap_PushPop(t *testing.T) {
	h := &MergeHeap{}
	heap.Init(h)

	nodes := []*MergeNode{
		{Key: []byte("b"), FileID: 1},
		{Key: []byte("a"), FileID: 2}, // should be first
		{Key: []byte("c"), FileID: 1},
		{Key: []byte("a"), FileID: 1}, // should be second (smaller fileID)
	}

	for _, n := range nodes {
		heap.Push(h, n)
	}

	expectedOrder := []*MergeNode{
		{Key: []byte("a"), FileID: 2},
		{Key: []byte("a"), FileID: 1},
		{Key: []byte("b"), FileID: 1},
		{Key: []byte("c"), FileID: 1},
	}

	for i, expected := range expectedOrder {
		if h.Len() != len(expectedOrder)-i {
			t.Errorf("Expected length %d, got %d", len(expectedOrder)-i, h.Len())
		}
		got := heap.Pop(h).(*MergeNode)
		if !bytes.Equal(got.Key, expected.Key) || got.FileID != expected.FileID {
			t.Errorf("Pop at index %d = %+v, want %+v", i, got, expected)
		}
	}
}

// TestMergeHeap_DirectMethods tests the pointer receiver Push and Pop methods
// directly to ensure they accurately append to and slice the underlying array
// without relying on the container/heap wrapper.
func TestMergeHeap_DirectMethods(t *testing.T) {
	// Test the direct methods Push and Pop without the heap package
	// to ensure they behave as expected on the slice.
	h := &MergeHeap{}

	node1 := &MergeNode{Key: []byte("test1")}
	node2 := &MergeNode{Key: []byte("test2")}

	h.Push(node1)
	if h.Len() != 1 || (*h)[0] != node1 {
		t.Errorf("Direct Push failed")
	}

	h.Push(node2)
	if h.Len() != 2 || (*h)[1] != node2 {
		t.Errorf("Direct Push failed")
	}

	popped := h.Pop()
	if popped != node2 {
		t.Errorf("Direct Pop expected the last element added")
	}
	if h.Len() != 1 {
		t.Errorf("Expected length 1 after direct pop")
	}
}
