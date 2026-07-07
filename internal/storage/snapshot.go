package storage

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/makeshift-engineering/penguin-db/internal/storage/memtable"
	"github.com/makeshift-engineering/penguin-db/internal/storage/sstable"
)

// Snapshot provides a read-only point-in-time view of the database.
type Snapshot interface {
	// Get retrieves a value for a given key from the snapshot.
	Get(key []byte) ([]byte, error)
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
		value, found, deleted, err := s.imm[i].GetWithTombstone(key)
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

	// Search Level 0 SSTables
	for _, sstableReader := range level0 {
		if sstableReader.BloomMayContain(key) {
			value, found, deleted, err := sstableReader.Get(key)
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
	}

	// Search Level 1 SSTables
	if len(level1) > 0 {
		index := sort.Search(len(level1), func(i int) bool {
			return bytes.Compare(level1[i].MaxKey(), key) >= 0
		})
		if index < len(level1) {
			sstableReader := level1[index]
			if bytes.Compare(sstableReader.MinKey(), key) <= 0 {
				if sstableReader.BloomMayContain(key) {
					value, found, deleted, err := sstableReader.Get(key)
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
			}
		}
	}

	return nil, ErrKeyNotFound
}

func (s *dbSnapshot) Close() {
	s.engine.sstRefsMu.Lock()
	for _, sstableReader := range s.pinned {
		s.engine.unpinSSTable(sstableReader)
	}
	s.engine.sstRefsMu.Unlock()
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
	engine.sstRefsMu.Lock()
	pinned := make([]*sstable.Reader, 0, len(level0)+len(level1))
	for _, r := range level0 {
		engine.pinSSTable(r)
		pinned = append(pinned, r)
	}
	for _, r := range level1 {
		engine.pinSSTable(r)
		pinned = append(pinned, r)
	}
	engine.sstRefsMu.Unlock()

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
