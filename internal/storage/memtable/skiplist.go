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

	_, targetNode := skipList.findPredecessors(key)

	if targetNode != nil && bytes.Equal(targetNode.key, key) {
		if targetNode.isDeleted {
			return nil, ErrKeyNotFound
		}
		return targetNode.value, nil
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

	predecessorNodes, targetNode := skipList.findPredecessors(key)

	if targetNode != nil && bytes.Equal(targetNode.key, key) {
		sizeDifference := int64(len(value)) - int64(len(targetNode.value))
		if skipList.currentSizeBytes+int64(sizeDifference) > skipList.maxSizeBytes {
			return ErrMemTableFull
		}
		skipList.currentSizeBytes += int64(sizeDifference)

		targetNode.value = value
		targetNode.isDeleted = false
		return nil
	}

	if err := skipList.ensureCapacity(int64(len(key)) + int64(len(value))); err != nil {
		return err
	}

	skipList.insertNode(key, value, false, predecessorNodes)
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

	predecessorNodes, targetNode := skipList.findPredecessors(key)

	if targetNode != nil && bytes.Equal(targetNode.key, key) {
		if !targetNode.isDeleted {
			skipList.currentSizeBytes -= int64(len(targetNode.value))
			targetNode.isDeleted = true
			targetNode.value = nil
		}
		return nil
	}

	if err := skipList.ensureCapacity(int64(len(key))); err != nil {
		return err
	}

	skipList.insertNode(key, nil, true, predecessorNodes)
	return nil
}

// ensureCapacity verifies if the skip list can accommodate the additional bytes.
// This method does not acquire locks; the caller must hold the appropriate write mutex.
func (skipList *SkipList) ensureCapacity(addedBytes int64) error {
	if skipList.currentSizeBytes+addedBytes > skipList.maxSizeBytes {
		return ErrMemTableFull
	}
	return nil
}

// findPredecessors traverses the skip list to find the insertion/deletion path for a key.
// It returns an array of the right-most nodes traversed at each level, and the node
// immediately following the search path at level 0 (which may or may not match the target key).
// This method does not acquire locks; the caller must hold the appropriate mutex.
func (skipList *SkipList) findPredecessors(key []byte) ([MaxLevel]*node, *node) {
	var predecessorNodes [MaxLevel]*node
	currentNode := skipList.headNode

	for levelIndex := skipList.highestActiveLevel - 1; levelIndex >= 0; levelIndex-- {
		nextNode := currentNode.next[levelIndex]
		for currentNode.next[levelIndex] != nil && bytes.Compare(nextNode.key, key) < 0 {
			currentNode = nextNode
			nextNode = currentNode.next[levelIndex]
		}
		predecessorNodes[levelIndex] = currentNode
	}

	return predecessorNodes, currentNode.next[0]
}

// insertNode handles the common pointer wiring and level generation for brand new keys and tombstones.
// This method does not acquire locks; the caller must hold the appropriate write mutex.
func (skipList *SkipList) insertNode(key, value []byte, isDeleted bool, predecessorNodes [MaxLevel]*node) {
	newNodeHeight := randomLevel()
	if newNodeHeight > skipList.highestActiveLevel {
		for levelIndex := skipList.highestActiveLevel; levelIndex < newNodeHeight; levelIndex++ {
			predecessorNodes[levelIndex] = skipList.headNode
		}
		skipList.highestActiveLevel = newNodeHeight
	}

	newNode := newNode(key, value, newNodeHeight)
	newNode.isDeleted = isDeleted

	for levelIndex := 0; levelIndex < newNodeHeight; levelIndex++ {
		newNode.next[levelIndex] = predecessorNodes[levelIndex].next[levelIndex]
		predecessorNodes[levelIndex].next[levelIndex] = newNode
	}

	skipList.currentSizeBytes += int64(len(key)) + int64(len(value))
}
