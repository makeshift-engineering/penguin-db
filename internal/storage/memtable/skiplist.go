package memtable

import (
	"bytes"
	"log/slog"
	"math/rand"
	"sync"
)

// maxAllowedLevel is the maximum tower height allowed for any skip list node.
// This restricts the size of stack-allocated predecessor arrays to prevent stack overflow.
const maxAllowedLevel = 32

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
	maxLevel           int
	mutex              sync.RWMutex
}

// NewSkipList allocates and returns an empty SkipList whose total byte capacity
// is capped at maxSize and whose tower height is configured by maxLevel. Writes
// that would cause the byte usage to exceed maxSize are rejected with ErrMemTableFull.
func NewSkipList(maxSize int64, maxLevel int) *SkipList {
	if maxLevel < 1 || maxLevel > maxAllowedLevel {
		maxLevel = 12
	}
	slog.Debug("initializing new skip list", "maxSizeBytes", maxSize, "maxLevel", maxLevel)
	return &SkipList{
		headNode:           newNode(nil, nil, maxLevel),
		highestActiveLevel: 1,
		currentSizeBytes:   0,
		maxSizeBytes:       maxSize,
		maxLevel:           maxLevel,
	}
}

// Get returns the value associated with key. It returns ErrKeyNotFound if the key
// is absent or if the key is present but marked as deleted by a tombstone. Get
// acquires a shared read lock and is safe to call concurrently with other Gets.
func (skipList *SkipList) Get(key []byte) ([]byte, error) {
	if len(key) == 0 {
		slog.Debug("get failed: empty key provided")
		return nil, ErrEmptyKey
	}

	skipList.mutex.RLock()
	defer skipList.mutex.RUnlock()

	_, targetNode := skipList.findPredecessors(key)

	if targetNode != nil && bytes.Equal(targetNode.key, key) {
		if targetNode.isDeleted {
			slog.Debug("get: key has tombstone marker (logically deleted)", "key", string(key))
			return nil, ErrKeyNotFound
		}
		slog.Debug("get: key found", "key", string(key), "valueLength", len(targetNode.value))
		return targetNode.value, nil
	}

	slog.Debug("get: key not found", "key", string(key))
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
	if len(key) == 0 {
		slog.Debug("put failed: empty key provided")
		return ErrEmptyKey
	}

	skipList.mutex.Lock()
	defer skipList.mutex.Unlock()

	predecessorNodes, targetNode := skipList.findPredecessors(key)

	if targetNode != nil && bytes.Equal(targetNode.key, key) {
		sizeDifference := int64(len(value)) - int64(len(targetNode.value))
		if skipList.currentSizeBytes+int64(sizeDifference) > skipList.maxSizeBytes {
			slog.Debug("put failed: size limit exceeded on update",
				"key", string(key),
				"valueLength", len(value),
				"currentSizeBytes", skipList.currentSizeBytes,
				"sizeDifference", sizeDifference,
				"maxSizeBytes", skipList.maxSizeBytes,
			)
			return ErrMemTableFull
		}
		skipList.currentSizeBytes += int64(sizeDifference)

		targetNode.value = value
		targetNode.isDeleted = false
		slog.Debug("put: updated existing key",
			"key", string(key),
			"valueLength", len(value),
			"sizeDifference", sizeDifference,
			"currentSizeBytes", skipList.currentSizeBytes,
		)
		return nil
	}

	if err := skipList.ensureCapacity(int64(len(key)) + int64(len(value))); err != nil {
		slog.Debug("put failed: size limit exceeded on insert",
			"key", string(key),
			"valueLength", len(value),
			"currentSizeBytes", skipList.currentSizeBytes,
			"requiredBytes", int64(len(key))+int64(len(value)),
			"maxSizeBytes", skipList.maxSizeBytes,
		)
		return err
	}

	skipList.insertNode(key, value, false, predecessorNodes)
	slog.Debug("put: inserted new key",
		"key", string(key),
		"valueLength", len(value),
		"currentSizeBytes", skipList.currentSizeBytes,
	)
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
	if len(key) == 0 {
		slog.Debug("delete failed: empty key provided")
		return ErrEmptyKey
	}

	skipList.mutex.Lock()
	defer skipList.mutex.Unlock()

	predecessorNodes, targetNode := skipList.findPredecessors(key)

	if targetNode != nil && bytes.Equal(targetNode.key, key) {
		if !targetNode.isDeleted {
			skipList.currentSizeBytes -= int64(len(targetNode.value))
			targetNode.isDeleted = true
			targetNode.value = nil
			slog.Debug("delete: marked existing key as tombstone",
				"key", string(key),
				"currentSizeBytes", skipList.currentSizeBytes,
			)
		} else {
			slog.Debug("delete: key already marked as tombstone (no-op)", "key", string(key))
		}
		return nil
	}

	if err := skipList.ensureCapacity(int64(len(key))); err != nil {
		slog.Debug("delete failed: size limit exceeded for tombstone insert",
			"key", string(key),
			"currentSizeBytes", skipList.currentSizeBytes,
			"requiredBytes", int64(len(key)),
			"maxSizeBytes", skipList.maxSizeBytes,
		)
		return err
	}

	skipList.insertNode(key, nil, true, predecessorNodes)
	slog.Debug("delete: inserted new tombstone node",
		"key", string(key),
		"currentSizeBytes", skipList.currentSizeBytes,
	)
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
func (skipList *SkipList) findPredecessors(key []byte) ([maxAllowedLevel]*node, *node) {
	var predecessorNodes [maxAllowedLevel]*node
	currentNode := skipList.headNode

	for levelIndex := skipList.highestActiveLevel - 1; levelIndex >= 0; levelIndex-- {
		nextNode := currentNode.next[levelIndex]
		for nextNode != nil && bytes.Compare(nextNode.key, key) < 0 {
			currentNode = nextNode
			nextNode = currentNode.next[levelIndex]
		}
		predecessorNodes[levelIndex] = currentNode
	}

	return predecessorNodes, currentNode.next[0]
}

// insertNode handles the common pointer wiring and level generation for brand new keys and tombstones.
// This method does not acquire locks; the caller must hold the appropriate write mutex.
func (skipList *SkipList) insertNode(key, value []byte, isDeleted bool, predecessorNodes [maxAllowedLevel]*node) {
	newNodeHeight := skipList.randomLevel()
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
	slog.Debug("inserted skip list node internally",
		"key", string(key),
		"height", newNodeHeight,
		"isDeleted", isDeleted,
	)
}

// randomLevel determines the height of a newly inserted node's tower using a
// geometric distribution with a promotion probability of 0.5. The returned level
// is always in the range [1, skipList.maxLevel].
func (skipList *SkipList) randomLevel() int {
	level := 1
	for rand.Float32() < 0.5 && level < skipList.maxLevel {
		level++
	}
	return level
}
