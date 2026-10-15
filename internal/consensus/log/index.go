package log

import (
	"errors"
	"sync"
)

var (
	ErrIndexNotFound = errors.New("log index not present in index")
	ErrIndexTooSmall = errors.New("requested index precedes earliest retained log index")
)

type entryLocation struct {
	segmentBase uint64
	offset      int64
	length      int64
	term        uint64
}

type LogIndex struct {
	mu         sync.RWMutex
	firstIndex uint64
	entries    []entryLocation
}

func NewLogIndex(firstIndex uint64) *LogIndex {
	return &LogIndex{
		firstIndex: firstIndex,
		entries:    make([]entryLocation, 0, 1024),
	}
}

func (li *LogIndex) Append(index uint64, term uint64, segmentBase uint64, offset, length int64) {
	li.mu.Lock()
	defer li.mu.Unlock()

	if len(li.entries) == 0 {
		li.firstIndex = index
	}

	li.entries = append(li.entries, entryLocation{
		segmentBase: segmentBase,
		offset:      offset,
		length:      length,
		term:        term,
	})
}

func (li *LogIndex) LookUp(index uint64) (entryLocation, error) {
	li.mu.RLock()
	defer li.mu.RUnlock()

	if len(li.entries) == 0 || index < li.firstIndex {
		return entryLocation{}, ErrIndexTooSmall
	}

	slot := index - li.firstIndex
	if slot >= uint64(len(li.entries)) {
		return entryLocation{}, ErrIndexNotFound
	}

	return li.entries[slot], nil
}

func (li *LogIndex) LastIndexAndTerm() (uint64, uint64) {
	li.mu.RLock()
	defer li.mu.RUnlock()

	if len(li.entries) == 0 {
		return li.firstIndex - 1, 0
	}

	lastIndex := li.firstIndex + uint64(len(li.entries)) - 1
	lastTerm := li.entries[len(li.entries)-1].term
	return lastIndex, lastTerm
}

func (li *LogIndex) FirstIndex() uint64 {
	li.mu.RLock()
	defer li.mu.RUnlock()
	return li.firstIndex
}

func (li *LogIndex) TruncateSuffix(fromIndex uint64) {
	li.mu.Lock()
	defer li.mu.Unlock()

	if fromIndex <= li.firstIndex {
		li.entries = li.entries[:0]
		return
	}

	slot := fromIndex - li.firstIndex
	if slot < uint64(len(li.entries)) {
		li.entries = li.entries[:slot]
	}
}

func (li *LogIndex) TruncatePrefix(upToIndex uint64) {
	li.mu.Lock()
	defer li.mu.Unlock()
	if upToIndex < li.firstIndex {
		return
	}
	discardCount := upToIndex - li.firstIndex + 1
	if discardCount >= uint64(len(li.entries)) {
		li.entries = li.entries[:0]
		li.firstIndex = upToIndex + 1
		return
	}

	remaining := make([]entryLocation, uint64(len(li.entries))-discardCount)
	copy(remaining, li.entries[discardCount:])
	li.entries = remaining
	li.firstIndex = upToIndex + 1
}
