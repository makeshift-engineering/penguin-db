package memtable

import (
	"bytes"
	"sync"
)

// MaxLevel is the maximum tower height allowed for any skip list node.
// A value of 12 supports roughly 4096 elements at the ideal 50 % promotion
// probability before the extra levels start providing diminishing returns.
const MaxLevel = 12

// SkipList is a concurrent, size-bounded in-memory ordered map backed by a
// probabilistic skip list. It is the primary data structure used by the memtable.
//
// Reads are protected by a shared read lock, allowing multiple concurrent readers.
// Writes (Put and Delete) are serialised with an exclusive write lock.
//
// Size is tracked in bytes as the sum of all live key and value lengths. Tombstone
// nodes produced by Delete count only the key length because their value is nil.
type SkipList struct {
	headNode           *node
	highestActiveLevel int
	currentSizeBytes   int64
	maxSizeBytes       int64
	mutex              sync.RWMutex
}

// NewSkipList allocates and returns an empty SkipList whose total byte capacity
// is capped at maxSize. Writes that would cause the byte usage to exceed maxSize
// are rejected with ErrMemTableFull.
func NewSkipList(maxSize int64) *SkipList {
	return &SkipList{
		headNode:           newNode(nil, nil, MaxLevel),
		highestActiveLevel: 1,
		currentSizeBytes:   0,
		maxSizeBytes:       maxSize,
	}
}

// Get returns the value associated with key. It returns ErrKeyNotFound if the key
// is absent or if the key is present but marked as deleted by a tombstone. Get
// acquires a shared read lock and is safe to call concurrently with other Gets.
func (skipList *SkipList) Get(key []byte) ([]byte, error) {
	skipList.mutex.RLock()
	defer skipList.mutex.RUnlock()

	currentNode := skipList.headNode
	for levelIndex := skipList.highestActiveLevel - 1; levelIndex >= 0; levelIndex-- {
		for currentNode.next[levelIndex] != nil && bytes.Compare(currentNode.next[levelIndex].key, key) < 0 {
			currentNode = currentNode.next[levelIndex]
		}
	}

	currentNode = currentNode.next[0]
	if currentNode != nil && bytes.Equal(currentNode.key, key) {
		if currentNode.isDeleted {
			return nil, ErrKeyNotFound
		}
		return currentNode.value, nil
	}

	return nil, ErrKeyNotFound
}

// Put inserts or updates the key-value pair in the skip list.
//
// If the key already exists, its value is replaced in-place and the size counter
// is adjusted by the difference between the old and new value lengths. If the
// key is currently marked as deleted (tombstone), the tombstone flag is cleared.
//
// If the key does not exist, a new node is inserted at a randomly chosen tower
// height. The combined byte size of the key and value is added to the size counter.
//
// Put returns ErrMemTableFull if the write would exceed the configured capacity.
// Put acquires an exclusive write lock.
func (skipList *SkipList) Put(key, value []byte) error {
	skipList.mutex.Lock()
	defer skipList.mutex.Unlock()

	var predecessorNodes [MaxLevel]*node
	currentNode := skipList.headNode

	for levelIndex := skipList.highestActiveLevel - 1; levelIndex >= 0; levelIndex-- {
		for currentNode.next[levelIndex] != nil && bytes.Compare(currentNode.next[levelIndex].key, key) < 0 {
			currentNode = currentNode.next[levelIndex]
		}
		predecessorNodes[levelIndex] = currentNode
	}

	currentNode = currentNode.next[0]

	if currentNode != nil && bytes.Equal(currentNode.key, key) {
		sizeDifference := len(value) - len(currentNode.value)
		if skipList.currentSizeBytes+int64(sizeDifference) > skipList.maxSizeBytes {
			return ErrMemTableFull
		}
		skipList.currentSizeBytes += int64(sizeDifference)

		currentNode.value = value
		currentNode.isDeleted = false
		return nil
	}

	if skipList.currentSizeBytes+int64(len(key)+len(value)) > skipList.maxSizeBytes {
		return ErrMemTableFull
	}

	newNodeHeight := randomLevel()
	if newNodeHeight > skipList.highestActiveLevel {
		for levelIndex := skipList.highestActiveLevel; levelIndex < newNodeHeight; levelIndex++ {
			predecessorNodes[levelIndex] = skipList.headNode
		}
		skipList.highestActiveLevel = newNodeHeight
	}

	newNode := newNode(key, value, newNodeHeight)
	for levelIndex := 0; levelIndex < newNodeHeight; levelIndex++ {
		newNode.next[levelIndex] = predecessorNodes[levelIndex].next[levelIndex]
		predecessorNodes[levelIndex].next[levelIndex] = newNode
	}

	skipList.currentSizeBytes += int64(len(key) + len(value))
	return nil
}

// Delete logically removes a key from the skip list using LSM-tree tombstone semantics.
//
// If the key already exists and is not yet deleted, its value is set to nil, the
// isDeleted flag is set to true, and the size counter is decreased by the old value
// length. The key length itself remains accounted for in the size counter because
// the node must be retained to shadow older versions in SSTables.
//
// If the key does not exist at all, a new tombstone node is inserted so that the
// deletion is propagated to the SSTable compaction layer on flush. The key byte
// length is added to the size counter.
//
// If the key is already a tombstone, Delete is a no-op and returns nil.
//
// Delete returns ErrMemTableFull if inserting a new tombstone would exceed capacity.
// Delete acquires an exclusive write lock.
func (skipList *SkipList) Delete(key []byte) error {
	skipList.mutex.Lock()
	defer skipList.mutex.Unlock()

	var predecessorNodes [MaxLevel]*node
	currentNode := skipList.headNode

	for levelIndex := skipList.highestActiveLevel - 1; levelIndex >= 0; levelIndex-- {
		for currentNode.next[levelIndex] != nil && bytes.Compare(currentNode.next[levelIndex].key, key) < 0 {
			currentNode = currentNode.next[levelIndex]
		}
		predecessorNodes[levelIndex] = currentNode
	}

	currentNode = currentNode.next[0]

	if currentNode != nil && bytes.Equal(currentNode.key, key) {
		if !currentNode.isDeleted {
			skipList.currentSizeBytes -= int64(len(currentNode.value))
			currentNode.isDeleted = true
			currentNode.value = nil
		}
		return nil
	}

	if skipList.currentSizeBytes+int64(len(key)) > skipList.maxSizeBytes {
		return ErrMemTableFull
	}

	newNodeHeight := randomLevel()
	if newNodeHeight > skipList.highestActiveLevel {
		for levelIndex := skipList.highestActiveLevel; levelIndex < newNodeHeight; levelIndex++ {
			predecessorNodes[levelIndex] = skipList.headNode
		}
		skipList.highestActiveLevel = newNodeHeight
	}

	newNode := newNode(key, nil, newNodeHeight)
	newNode.isDeleted = true

	for levelIndex := 0; levelIndex < newNodeHeight; levelIndex++ {
		newNode.next[levelIndex] = predecessorNodes[levelIndex].next[levelIndex]
		predecessorNodes[levelIndex].next[levelIndex] = newNode
	}

	skipList.currentSizeBytes += int64(len(key))
	return nil
}
