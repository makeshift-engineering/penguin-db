package storage

import (
	"fmt"

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
	engine *dbEngine
	levels map[int][]*sstable.Reader
	pinned []*sstable.Reader
	imm    []*memtable.SkipList
}

func (s *dbSnapshot) Get(key []byte) ([]byte, error) {
	if len(key) == 0 {
		return nil, memtable.ErrEmptyKey
	}

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

	// Use shared SSTable search helper.
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
	s.engine.mu.RLock()
	if s.engine.isClosing {
		s.engine.mu.RUnlock()
		return nil, fmt.Errorf("engine is closing")
	}
	s.engine.mu.RUnlock()

	level0 := s.levels[levelZero]
	level1 := s.levels[levelOne]
	return s.engine.scanInternal(prefix, level0, level1, s.imm), nil
}

func (s *dbSnapshot) Close() {
	s.engine.unpinReaders(s.pinned)
	s.engine.iterWg.Done()
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
		return nil, fmt.Errorf("engine is closing")
	}

	// Rotate active memtable if it contains any data to freeze its state for snapshot isolation
	if engine.memtable.Size() > 0 {
		if err := engine.rotateActiveMemTableAndWAL(); err != nil {
			engine.mu.Unlock()
			return nil, err
		}
	}

	level0 := make([]*sstable.Reader, 0, len(engine.levels[levelZero]))
	level0 = append(level0, engine.levels[levelZero]...)

	level1 := make([]*sstable.Reader, 0, len(engine.levels[levelOne]))
	level1 = append(level1, engine.levels[levelOne]...)

	// Pin all readers
	pinned := engine.pinReaders(level0, level1)

	imm := make([]*memtable.SkipList, len(engine.immMemtables))
	copy(imm, engine.immMemtables)

	engine.iterWg.Add(1)
	engine.mu.Unlock()

	levels := make(map[int][]*sstable.Reader)
	levels[levelZero] = level0
	levels[levelOne] = level1

	return &dbSnapshot{
		engine: engine,
		levels: levels,
		pinned: pinned,
		imm:    imm,
	}, nil
}
