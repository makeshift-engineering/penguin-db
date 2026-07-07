package memtable

import (
	"log/slog"
)

// Iterator provides a forward-only, peek-based cursor over all nodes in the
// skip list, including tombstone nodes. It is used by the flush mechanism to read
// the complete contents of the memtable in sorted key order when writing an SSTable,
// and by the merging iterator in the storage engine for range scans.
//
// An Iterator must be created via SkipList.NewIterator or SkipList.NewIteratorAt.
// Once created, Valid should be checked before accessing Key, Value, or IsDeleted.
// Next advances the iterator to the next entry.
//
// Iterator is not safe for concurrent use by multiple goroutines. Each consumer
// should create its own Iterator.
type Iterator struct {
	skipList    *SkipList
	currentNode *node
	currKey     []byte
	currVal     []byte
	currDel     bool
	hasCurrent  bool
}

// bufferCurrent reads the current node's data into the buffered fields.
// Must be called while the caller holds at least an RLock on the skip list.
func (iterator *Iterator) bufferCurrent() {
	if iterator.currentNode != nil {
		iterator.currKey = iterator.currentNode.key
		iterator.currVal = iterator.currentNode.value
		iterator.currDel = iterator.currentNode.isDeleted
		iterator.hasCurrent = true
	} else {
		iterator.hasCurrent = false
		iterator.currKey = nil
		iterator.currVal = nil
		iterator.currDel = false
	}
}

// NewIterator returns a new Iterator positioned at the first node of the skip list.
// The iterator buffers the first entry's data under the read lock, providing a
// consistent snapshot of the initial position.
func (skipList *SkipList) NewIterator() *Iterator {
	skipList.mutex.RLock()
	defer skipList.mutex.RUnlock()

	firstNode := skipList.headNode.next[0]

	slog.Debug("created new iterator starting at first node", "hasNext", firstNode != nil)
	iter := &Iterator{
		skipList:    skipList,
		currentNode: firstNode,
	}
	iter.bufferCurrent()
	return iter
}

// Valid reports whether the iterator is positioned on a valid node. It returns
// false when the iterator has advanced past the last node in the skip list.
func (iterator *Iterator) Valid() bool {
	return iterator.hasCurrent
}

// Key returns the key of the current entry. The returned slice is valid until
// the next call to Next().
func (iterator *Iterator) Key() []byte {
	return iterator.currKey
}

// Value returns the value of the current entry. The returned slice is valid until
// the next call to Next(). Tombstone nodes return nil.
func (iterator *Iterator) Value() []byte {
	return iterator.currVal
}

// IsDeleted reports whether the current entry is a tombstone (logically deleted).
func (iterator *Iterator) IsDeleted() bool {
	return iterator.currDel
}

// Next advances the iterator to the next node in sorted key order.
// After calling Next, check Valid() before accessing Key/Value/IsDeleted.
func (iterator *Iterator) Next() {
	iterator.skipList.mutex.RLock()
	defer iterator.skipList.mutex.RUnlock()

	if iterator.currentNode == nil {
		iterator.hasCurrent = false
		return
	}

	iterator.currentNode = iterator.currentNode.next[0]
	iterator.bufferCurrent()

	slog.Debug("iterator: advancing to next node",
		"key", string(iterator.currKey),
		"isDeleted", iterator.currDel,
		"hasNext", iterator.currentNode != nil,
	)
}

// Close is a no-op for memtable iterators. It exists to satisfy the
// internalIterator interface in the storage package.
func (iterator *Iterator) Close() {}
