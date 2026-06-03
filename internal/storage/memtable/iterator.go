package memtable

// Iterator provides a forward-only, snapshot-style cursor over all nodes in the
// skip list, including tombstone nodes. It is used by the flush mechanism to read
// the complete contents of the memtable in sorted key order when writing an SSTable.
//
// An Iterator must be created via SkipList.NewIterator. Once created, Valid should
// be checked before each call to Next.
//
// Iterator is not safe for concurrent use by multiple goroutines. Each flushing
// goroutine should create its own Iterator.
type Iterator struct {
	skipList    *SkipList
	currentNode *node
}

// NewIterator returns a new Iterator positioned at the first node of the skip list.
// The iterator reflects the state of the skip list at the moment of creation.
// NewIterator acquires a short-lived shared read lock only to read the initial
// pointer; subsequent calls to Next each acquire their own short-lived read lock.
func (skipList *SkipList) NewIterator() *Iterator {
	skipList.mutex.RLock()
	defer skipList.mutex.RUnlock()

	firstNode := skipList.headNode.next[0]

	return &Iterator{
		skipList:    skipList,
		currentNode: firstNode,
	}
}

// Valid reports whether the iterator is positioned on a valid node. It returns
// false when the iterator has advanced past the last node in the skip list.
func (iterator *Iterator) Valid() bool {
	return iterator.currentNode != nil
}

// Next returns the key, value and deletion status of the current node and advances
// the iterator to the next node in sorted key order. Tombstone nodes are returned
// with isDeleted set to true and a nil value.
//
// Next returns (nil, nil, false) when the iterator is exhausted. Callers should
// check Valid before calling Next to avoid consuming the sentinel return values.
func (iterator *Iterator) Next() (key, value []byte, isDeleted bool) {
	iterator.skipList.mutex.RLock()
	defer iterator.skipList.mutex.RUnlock()

	if iterator.currentNode == nil {
		return nil, nil, false
	}

	key = iterator.currentNode.key
	value = iterator.currentNode.value
	isDeleted = iterator.currentNode.isDeleted

	iterator.currentNode = iterator.currentNode.next[0]

	return key, value, isDeleted
}
