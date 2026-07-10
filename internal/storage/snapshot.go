package storage

import (
	"fmt"
	"sync"

	"github.com/makeshift-engineering/penguin-db/internal/storage/memtable"
	"github.com/makeshift-engineering/penguin-db/internal/storage/sstable"
)

// Snapshot provides a read-only point-in-time view of the database.
type Snapshot interface {
	// Get retrieves a value for a given key from the snapshot.
	Get(key []byte) ([]byte, error)
	// Scan returns a prefix-filtering iterator sorted by key over the snapshot's point-in-time state.
	Scan(prefix []byte) (Iterator, error)
	// Close releases the pinned readers of the snapshot.
	Close()
}

// dbSnapshot implements the Snapshot interface.
type dbSnapshot struct {
	levels  map[int][]*sstable.Reader
	pinned  []*sstable.Reader
	imm     []*memtable.SkipList
	mu      sync.Mutex
	closed  bool
	onClose func()
	onScan  func(prefix []byte, level0, level1 []*sstable.Reader, imm []*memtable.SkipList) (Iterator, error)
}

func (s *dbSnapshot) Get(key []byte) ([]byte, error) {
	if len(key) == 0 {
		return nil, ErrEmptyKey
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, ErrSnapshotClosed
	}
	s.mu.Unlock()

	// Search queue of immutable memtables (newest to oldest)
	for i := len(s.imm) - 1; i >= 0; i-- {
		value, found, deleted, err := s.imm[i].Get(key)
		if err != nil {
			return nil, err
		}
		if found {
			if deleted {
				return nil, ErrKeyNotFound
			}
			return value, nil
		}
	}

	level0 := s.levels[levelZero]
	level1 := s.levels[levelOne]

	value, found, deleted, _, err := searchLevels(level0, level1, key)
	if err != nil {
		return nil, err
	}
	if found {
		if deleted {
			return nil, ErrKeyNotFound
		}
		return value, nil
	}

	return nil, ErrKeyNotFound
}

func (s *dbSnapshot) Scan(prefix []byte) (Iterator, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, ErrSnapshotClosed
	}
	s.mu.Unlock()

	if s.onScan == nil {
		return nil, fmt.Errorf("snapshot scan uninitialized")
	}
	return s.onScan(prefix, s.levels[levelZero], s.levels[levelOne], s.imm)
}

func (s *dbSnapshot) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()

	if s.onClose != nil {
		s.onClose()
	}
}

// Snapshot returns a point-in-time Snapshot of the database.
func (engine *dbEngine) Snapshot() (Snapshot, error) {
	engine.writeMu.Lock()
	defer engine.writeMu.Unlock()

	engine.mu.Lock()
	if engine.bgErr != nil {
		engine.mu.Unlock()
		return nil, engine.bgErr
	}
	if engine.isClosing {
		engine.mu.Unlock()
		return nil, ErrEngineClosed
	}

	// Rotate active memtable if it contains any data to freeze its state for snapshot isolation
	if engine.memtable.Size() > 0 {
		oldWAL, newSegmentID := engine.freezeActiveMemTable()
		engine.mu.Unlock()

		if err := engine.commitRotation(oldWAL, newSegmentID); err != nil {
			return nil, err
		}
		engine.mu.Lock()

		if engine.bgErr != nil {
			engine.mu.Unlock()
			return nil, engine.bgErr
		}
		if engine.isClosing {
			engine.mu.Unlock()
			return nil, ErrEngineClosed
		}
	}

	level0 := make([]*sstable.Reader, 0, len(engine.levels[levelZero]))
	level0 = append(level0, engine.levels[levelZero]...)

	level1 := make([]*sstable.Reader, 0, len(engine.levels[levelOne]))
	level1 = append(level1, engine.levels[levelOne]...)

	// Pin all readers
	pinned := engine.pinReaders(level0, level1)

	// Copy immutable memtables in newest-to-oldest order so the merge
	// iterator's first-match tie-break correctly resolves duplicate keys.
	imm := make([]*memtable.SkipList, len(engine.immMemtables))
	for i, j := 0, len(engine.immMemtables)-1; j >= 0; i, j = i+1, j-1 {
		imm[i] = engine.immMemtables[j]
	}

	engine.iterWg.Add(1)
	engine.mu.Unlock()

	levels := make(map[int][]*sstable.Reader)
	levels[levelZero] = level0
	levels[levelOne] = level1

	onClose := func() {
		engine.unpinReaders(pinned)
		engine.iterWg.Done()
	}

	onScan := func(prefix []byte, lvl0, lvl1 []*sstable.Reader, immMem []*memtable.SkipList) (Iterator, error) {
		engine.mu.RLock()
		pinned := engine.pinReaders(lvl0, lvl1)
		engine.iterWg.Add(1)
		engine.mu.RUnlock()

		iter, err := engine.scanInternal(prefix, lvl0, lvl1, immMem, pinned)
		if err != nil {
			engine.unpinReaders(pinned)
			engine.iterWg.Done()
			return nil, err
		}
		return iter, nil
	}

	return &dbSnapshot{
		levels:  levels,
		pinned:  pinned,
		imm:     imm,
		onClose: onClose,
		onScan:  onScan,
	}, nil
}
