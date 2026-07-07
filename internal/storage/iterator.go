package storage

import (
	"bytes"

	"github.com/makeshift-engineering/penguin-db/internal/storage/sstable"
)

// Iterator defines the interface for scanning range queries.
type Iterator interface {
	// Valid returns true if the iterator is positioned on a valid key-value entry.
	Valid() bool

	// Next returns the current key-value pair and advances the iterator to the next entry.
	// Returns (nil, nil) when exhausted.
	Next() (key, value []byte)

	// Close releases resources associated with the iterator.
	Close()
}

// internalIterator wraps memory & SSTable iterators into a uniform peekable cursor.
//
// memtable.Iterator satisfies this interface structurally (via its peek-based
// Key/Value/IsDeleted/Next/Valid/Close methods). sstable.Iterator requires the
// sstAdapter wrapper below because its Next() returns bool and Close() returns error.
type internalIterator interface {
	Valid() bool
	Key() []byte
	Value() []byte
	IsDeleted() bool
	Next()
	Close()
}

// sstAdapter adapts a *sstable.Iterator into the internalIterator interface.
// This adapter is necessary because sstable.Iterator has different method
// signatures (Next() bool, Close() error) that cannot structurally satisfy
// internalIterator without wrapper logic.
type sstAdapter struct {
	iter       *sstable.Iterator
	hasCurrent bool
}

// newSstAdapter creates an sstAdapter and positions it on the first valid entry.
func newSstAdapter(iter *sstable.Iterator) *sstAdapter {
	adapter := &sstAdapter{iter: iter}
	adapter.Next()
	return adapter
}

func (adapter *sstAdapter) Valid() bool     { return adapter.hasCurrent && adapter.iter.Error() == nil }
func (adapter *sstAdapter) Key() []byte     { return adapter.iter.Key() }
func (adapter *sstAdapter) Value() []byte   { return adapter.iter.Value() }
func (adapter *sstAdapter) IsDeleted() bool { return adapter.iter.Opcode() == sstable.OpcodeDelete }
func (adapter *sstAdapter) Next()           { adapter.hasCurrent = adapter.iter.Next() }
func (adapter *sstAdapter) Close()          { _ = adapter.iter.Close() }

// mergingIterator merges multiple internalIterators into a single sorted cursor.
type mergingIterator struct {
	engine  *dbEngine
	pinned  []*sstable.Reader
	iters   []internalIterator
	prefix  []byte
	currKey []byte
	currVal []byte
	valid   bool
	closed  bool
}

// Valid returns true if the merging iterator is currently holding a valid entry.
func (iterator *mergingIterator) Valid() bool {
	return iterator.valid && !iterator.closed
}

// Next retrieves the current entry and steps the iterator forward to the next.
func (iterator *mergingIterator) Next() (key, value []byte) {
	if iterator.closed || !iterator.valid {
		return nil, nil
	}
	returnedKey := iterator.currKey
	returnedValue := iterator.currVal

	iterator.findNext()

	return returnedKey, returnedValue
}

// Close releases the reference counts of all pinned SSTable readers and signals
// the engine that this iterator is no longer active.
func (iterator *mergingIterator) Close() {
	if iterator.closed {
		return
	}
	iterator.closed = true
	for _, subIterator := range iterator.iters {
		subIterator.Close()
	}
	iterator.engine.sstRefsMu.Lock()
	for _, sstableReader := range iterator.pinned {
		iterator.engine.unpinSSTable(sstableReader)
	}
	iterator.engine.sstRefsMu.Unlock()
	iterator.engine.iterWg.Done()
}

// findNext advances the merging iterator to the next live, non-tombstone entry.
func (iterator *mergingIterator) findNext() {
	for {
		var smallestKey []byte
		var smallestIdx = -1

		for i, subIterator := range iterator.iters {
			if !subIterator.Valid() {
				continue
			}
			key := subIterator.Key()
			if smallestKey == nil || bytes.Compare(key, smallestKey) < 0 {
				smallestKey = key
				smallestIdx = i
			}
		}

		if smallestIdx == -1 {
			iterator.valid = false
			iterator.currKey, iterator.currVal = nil, nil
			return
		}

		if len(iterator.prefix) > 0 && !bytes.HasPrefix(smallestKey, iterator.prefix) {
			iterator.valid = false
			iterator.currKey, iterator.currVal = nil, nil
			return
		}

		isDeleted := iterator.iters[smallestIdx].IsDeleted()
		valueCopy := iterator.iters[smallestIdx].Value()

		// Make a safe copy of the key. This must happen before calling Next()
		// on any sub-iterator, as Next() may overwrite the underlying buffer.
		keyCopy := make([]byte, len(smallestKey))
		copy(keyCopy, smallestKey)

		var valCopy []byte
		if valueCopy != nil {
			valCopy = make([]byte, len(valueCopy))
			copy(valCopy, valueCopy)
		}

		for _, subIterator := range iterator.iters {
			if subIterator.Valid() && bytes.Equal(subIterator.Key(), keyCopy) {
				subIterator.Next()
			}
		}

		if !isDeleted {
			iterator.currKey = keyCopy
			iterator.currVal = valCopy
			iterator.valid = true
			return
		}
	}
}
