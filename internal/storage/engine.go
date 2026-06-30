package storage

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/makeshift-engineering/penguin-db/internal/storage/compactor"
	"github.com/makeshift-engineering/penguin-db/internal/storage/memtable"
	"github.com/makeshift-engineering/penguin-db/internal/storage/sstable"
	"github.com/makeshift-engineering/penguin-db/internal/storage/wal"
)

// ErrKeyNotFound is returned when the key is not found in the storage engine.
var ErrKeyNotFound = errors.New("key not found")

// OpType represents the operation type in a WriteBatch.
type OpType uint8

const (
	// OpPut represents an insert or update operation.
	OpPut OpType = 0x01
	// OpDelete represents a logical deletion tombstone operation.
	OpDelete OpType = 0x02
)

// Op represents a single Put or Delete operation within a WriteBatch.
type Op struct {
	// Type specifies whether the operation is a Put or Delete.
	Type OpType
	// Key is the record key to be written.
	Key []byte
	// Value is the payload to be written (nil for OpDelete).
	Value []byte
}

// Engine defines the top-level interface for the storage engine.
type Engine interface {
	// Put writes a single key-value pair to the database.
	Put(key, value []byte) error

	// Get retrieves a value for a given key. Returns ErrKeyNotFound if absent or logically deleted.
	Get(key []byte) ([]byte, error)

	// Delete writes a tombstone for a key, marking it as logically deleted.
	Delete(key []byte) error

	// Scan returns a prefix-filtered sorted iterator starting at the first key >= prefix.
	Scan(prefix []byte) Iterator

	// WriteBatch writes multiple operations atomically to the database.
	WriteBatch(operations []Op) error

	// Close flushes memory tables and closes all open files and background workers.
	Close() error
}

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

// Options configures runtime parameters for the storage engine.
type Options struct {
	// MaxMemTableSize is the maximum size in bytes of the active MemTable before freezing and flushing.
	MaxMemTableSize int64
	// MemTableMaxLevel is the maximum level height configuration for SkipLists.
	MemTableMaxLevel int
	// CompactionThreshold is the number of L0 files allowed before triggering compaction.
	CompactionThreshold int
	// WALOptions provides functional options for the WAL LogWriter.
	WALOptions wal.Options
}

// DefaultOptions returns the standard parameters.
func DefaultOptions() Options {
	return Options{
		MaxMemTableSize:     4 * 1024 * 1024,
		MemTableMaxLevel:    12,
		CompactionThreshold: 4,
		WALOptions:          wal.DefaultOptions(),
	}
}

// sstableRef tracks the reference count of active iterators using an SSTable.
type sstableRef struct {
	// reader is the underlying sstable reader instance.
	reader *sstable.Reader
	// refs is the active reader references count.
	refs int
	// obsolete is marked true when compaction has replaced this file.
	obsolete bool
}

// dbEngine is the concrete implementation of the Engine interface.
type dbEngine struct {
	// dir is the base database directory.
	dir string
	// walDir is the subdirectory where WAL files are kept.
	// NOTE: WAL segment files are named "<segmentID>.wal" and live under walDir.
	// SSTable files are named "<segmentID>.sst" and live under dir.
	// Both share the same monotonic nextSegmentID counter, but are stored in
	// different directories so there is no filename collision.
	walDir string
	// opts stores the engine options configuration.
	opts Options

	// mu synchronizes access to all engine state fields below.
	mu sync.RWMutex

	// lock is the exclusive directory lock closer.
	lock io.Closer

	// wal is the active Write-Ahead Log writer.
	wal *wal.LogWriter
	// activeWALSegmentID is the segment ID of the active WAL writer.
	activeWALSegmentID int
	// memtable is the active in-memory skip list.
	memtable *memtable.SkipList
	// immMemtable is the read-only frozen memtable currently flushing to disk.
	immMemtable *memtable.SkipList
	// immWALSegmentID is the WAL segment ID corresponding to the immMemtable.
	immWALSegmentID int

	// levels maps Level ID to the sorted slice of active SSTable readers.
	levels map[int][]*sstable.Reader

	// sstRefs coordinates reference counting to keep obsoleted files open for active iterators.
	// Access to sstRefs is protected by sstRefsMu (not the main mu), allowing pin/unpin
	// to avoid acquiring a full write lock during reads.
	sstRefs   map[*sstable.Reader]*sstableRef
	sstRefsMu sync.Mutex

	// nextSegmentID tracks the next unique ID for WAL/SSTable files.
	nextSegmentID int

	// flushChan triggers background memtable flushes.
	flushChan chan struct{}
	// flushCloseChan signals the background flush worker to stop.
	flushCloseChan chan struct{}
	// compactChan triggers background compactions.
	compactChan chan struct{}
	// compactCloseChan signals the background compaction worker to stop.
	compactCloseChan chan struct{}

	// flushCond throttles writes when both active and immutable memtables are full.
	flushCond *sync.Cond

	// flushWg tracks the flush worker lifecycle.
	flushWg sync.WaitGroup
	// compactWg tracks the compaction worker lifecycle.
	compactWg sync.WaitGroup

	// bgErr records background worker errors to prevent further writes.
	bgErr error
	// isCompacting tracks if a compaction is currently running.
	isCompacting bool
	// isClosing tracks if Close was invoked.
	isClosing bool
	// iterWg coordinates graceful shutdown by waiting for active user iterators.
	iterWg sync.WaitGroup
}

// NewEngine opens or creates a new storage engine instance in the specified directory.
func NewEngine(dir string, opts Options) (Engine, error) {
	walDir := filepath.Join(dir, "wal")
	if err := os.MkdirAll(walDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create WAL directory: %w", err)
	}

	// Acquire exclusive lock on base directory to prevent dual-open corruption.
	lock, err := lockDirectory(dir)
	if err != nil {
		return nil, err
	}

	success := false
	defer func() {
		if !success {
			_ = lock.Close()
		}
	}()

	manifest, err := loadManifest(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to load manifest: %w", err)
	}

	levels := make(map[int][]*sstable.Reader)
	levels[0] = nil
	levels[1] = nil

	sstRefs := make(map[*sstable.Reader]*sstableRef)

	// Open all active SSTables listed in manifest.
	for level, filenames := range manifest.Levels {
		readers := make([]*sstable.Reader, 0, len(filenames))
		for _, name := range filenames {
			path := filepath.Join(dir, name)
			sstableReader, err := sstable.Open(path)
			if err != nil {
				// Cleanup previously opened files before returning.
				for _, readerList := range levels {
					for _, openedReader := range readerList {
						_ = openedReader.Close()
					}
				}
				return nil, fmt.Errorf("failed to open SSTable %s: %w", path, err)
			}
			readers = append(readers, sstableReader)
			sstRefs[sstableReader] = &sstableRef{reader: sstableReader, refs: 0, obsolete: false}
		}
		levels[level] = readers
	}

	// Replay WAL segments to reconstruct in-memory state after a crash.
	recoveryMem := memtable.NewSkipList(opts.MaxMemTableSize, opts.MemTableMaxLevel)
	highestWALSegmentID, err := wal.Replay(walDir, recoveryMem)
	if err != nil {
		for _, readerList := range levels {
			for _, sstableReader := range readerList {
				_ = sstableReader.Close()
			}
		}
		return nil, fmt.Errorf("failed to replay WAL: %w", err)
	}

	var activeMem *memtable.SkipList
	var activeWAL *wal.LogWriter
	nextSegmentID := manifest.NextSegmentID
	activeSegmentID := nextSegmentID

	if recoveryMem.Size() > opts.MaxMemTableSize {
		// Recovery memtable exceeded the size limit: flush it directly to a
		// new Level 0 SSTable before starting the engine.
		sstableFilename := fmt.Sprintf("%06d.sst", nextSegmentID)
		sstablePath := filepath.Join(dir, sstableFilename)

		sstableWriter, err := sstable.NewWriter(sstablePath, 10000)
		if err != nil {
			for _, readerList := range levels {
				for _, sstableReader := range readerList {
					_ = sstableReader.Close()
				}
			}
			return nil, fmt.Errorf("failed to create recovery SSTable writer: %w", err)
		}

		iterator := recoveryMem.NewIterator()
		for iterator.Valid() {
			key, value, isDeleted := iterator.Next()
			var opcode uint8 = sstable.OpcodePut
			if isDeleted {
				opcode = sstable.OpcodeDelete
			}
			if err := sstableWriter.Add(key, value, opcode); err != nil {
				_ = sstableWriter.Close()
				_ = os.Remove(sstablePath)
				for _, readerList := range levels {
					for _, sstableReader := range readerList {
						_ = sstableReader.Close()
					}
				}
				return nil, fmt.Errorf("failed to write recovery SSTable entry: %w", err)
			}
		}

		if err := sstableWriter.Close(); err != nil {
			_ = os.Remove(sstablePath)
			for _, readerList := range levels {
				for _, sstableReader := range readerList {
					_ = sstableReader.Close()
				}
			}
			return nil, fmt.Errorf("failed to finalize recovery SSTable: %w", err)
		}

		sstableReader, err := sstable.Open(sstablePath)
		if err != nil {
			for _, readerList := range levels {
				for _, openedReader := range readerList {
					_ = openedReader.Close()
				}
			}
			return nil, fmt.Errorf("failed to open recovery SSTable reader: %w", err)
		}

		levels[0] = append([]*sstable.Reader{sstableReader}, levels[0]...)
		manifest.Levels[0] = append([]string{sstableFilename}, manifest.Levels[0]...)
		sstRefs[sstableReader] = &sstableRef{reader: sstableReader, refs: 0, obsolete: false}

		nextSegmentID++
		activeSegmentID = nextSegmentID
		nextSegmentID++

		manifest.NextSegmentID = nextSegmentID
		if err := writeManifest(dir, manifest); err != nil {
			sstableReader.Close()
			for _, readerList := range levels {
				for _, openedReader := range readerList {
					_ = openedReader.Close()
				}
			}
			return nil, fmt.Errorf("failed to save manifest: %w", err)
		}

		// WAL segments that were fully recovered and flushed can now be deleted.
		cleanupWALFiles(walDir, highestWALSegmentID)

		activeMem = memtable.NewSkipList(opts.MaxMemTableSize, opts.MemTableMaxLevel)
		activeWAL, err = createWALWriter(walDir, activeSegmentID, opts.WALOptions)
		if err != nil {
			for _, readerList := range levels {
				for _, openedReader := range readerList {
					_ = openedReader.Close()
				}
			}
			return nil, fmt.Errorf("failed to initialize active WAL writer: %w", err)
		}
	} else {
		// Recovery memtable fits in memory: resume from the highest replayed WAL segment.
		activeMem = recoveryMem
		activeSegmentID = highestWALSegmentID
		activeWAL, err = createWALWriter(walDir, activeSegmentID, opts.WALOptions)
		if err != nil {
			for _, readerList := range levels {
				for _, openedReader := range readerList {
					_ = openedReader.Close()
				}
			}
			return nil, fmt.Errorf("failed to resume active WAL writer: %w", err)
		}
		if activeSegmentID >= nextSegmentID {
			nextSegmentID = activeSegmentID + 1
			manifest.NextSegmentID = nextSegmentID
			_ = writeManifest(dir, manifest)
		}
	}

	engineInstance := &dbEngine{
		dir:                dir,
		walDir:             walDir,
		opts:               opts,
		lock:               lock,
		wal:                activeWAL,
		activeWALSegmentID: activeSegmentID,
		memtable:           activeMem,
		levels:             levels,
		sstRefs:            sstRefs,
		nextSegmentID:      nextSegmentID,
		flushChan:          make(chan struct{}, 1),
		flushCloseChan:     make(chan struct{}),
		compactChan:        make(chan struct{}, 1),
		compactCloseChan:   make(chan struct{}),
	}
	engineInstance.flushCond = sync.NewCond(&engineInstance.mu)

	// Launch background workers.
	engineInstance.flushWg.Add(1)
	go engineInstance.flushWorker()

	engineInstance.compactWg.Add(1)
	go engineInstance.compactionWorker()

	success = true
	return engineInstance, nil
}

// Put writes a single key-value record to the engine.
func (engine *dbEngine) Put(key, value []byte) error {
	return engine.WriteBatch([]Op{
		{Type: OpPut, Key: key, Value: value},
	})
}

// Delete logically deletes a key by appending a tombstone record.
func (engine *dbEngine) Delete(key []byte) error {
	return engine.WriteBatch([]Op{
		{Type: OpDelete, Key: key, Value: nil},
	})
}

// WriteBatch writes multiple operations atomically to the WAL and memtable.
func (engine *dbEngine) WriteBatch(operations []Op) error {
	if len(operations) == 0 {
		return nil
	}

	// Pre-validate all operations and compute total batch byte size.
	var batchSize int64
	for _, operation := range operations {
		if len(operation.Key) == 0 {
			return memtable.ErrEmptyKey
		}
		switch operation.Type {
		case OpPut:
			batchSize += int64(len(operation.Key) + len(operation.Value))
		case OpDelete:
			batchSize += int64(len(operation.Key))
		default:
			return fmt.Errorf("invalid write operation type: %v", operation.Type)
		}
	}

	engine.mu.Lock()

	for {
		if engine.bgErr != nil {
			engine.mu.Unlock()
			return engine.bgErr
		}
		if engine.isClosing {
			engine.mu.Unlock()
			return fmt.Errorf("engine is closing")
		}

		// If the batch would overflow the active memtable, freeze it and rotate.
		if engine.memtable.Size()+batchSize > engine.opts.MaxMemTableSize {
			if engine.immMemtable != nil {
				// An immutable memtable is still being flushed; wait for it to complete.
				engine.flushCond.Wait()
				continue
			}

			// Freeze the active memtable and move it to the immutable slot.
			engine.immMemtable = engine.memtable
			engine.immWALSegmentID = engine.activeWALSegmentID

			// Close the active WAL segment.
			activeWAL := engine.wal
			engine.wal = nil
			engine.mu.Unlock()

			if err := activeWAL.Close(); err != nil {
				engine.mu.Lock()
				engine.bgErr = err
				engine.mu.Unlock()
				return err
			}

			engine.mu.Lock()

			// Initialize a fresh active memtable and a new WAL segment.
			engine.memtable = memtable.NewSkipList(engine.opts.MaxMemTableSize, engine.opts.MemTableMaxLevel)
			engine.activeWALSegmentID = engine.nextSegmentID
			engine.nextSegmentID++

			newWAL, err := createWALWriter(engine.walDir, engine.activeWALSegmentID, engine.opts.WALOptions)
			if err != nil {
				engine.bgErr = err
				engine.mu.Unlock()
				return err
			}
			engine.wal = newWAL

			manifest := &Manifest{
				NextSegmentID: engine.nextSegmentID,
				Levels:        engine.manifestLevels(),
			}
			if err := writeManifest(engine.dir, manifest); err != nil {
				engine.bgErr = err
				engine.mu.Unlock()
				return err
			}

			// Signal the background flush worker.
			select {
			case engine.flushChan <- struct{}{}:
			default:
			}

			continue
		}

		// Build the WAL records for this batch.
		walRecords := make([]*wal.Record, 0, len(operations))
		for _, operation := range operations {
			walOpcode := uint8(wal.OpcodePut)
			if operation.Type == OpDelete {
				walOpcode = wal.OpcodeDelete
			}
			walRecords = append(walRecords, &wal.Record{
				Opcode: walOpcode,
				Key:    operation.Key,
				Value:  operation.Value,
			})
		}

		activeWAL := engine.wal
		engine.mu.Unlock()

		if err := activeWAL.AppendBatch(walRecords); err != nil {
			engine.mu.Lock()
			engine.bgErr = err
			engine.mu.Unlock()
			return fmt.Errorf("failed to append batch to WAL: %w", err)
		}

		// Re-acquire lock to apply the same operations to the memtable.
		// The WAL is durable at this point, so a crash before the memtable
		// update is safe: replay will restore the state on the next open.
		engine.mu.Lock()

		// Guard against a racing Close() or background error that occurred
		// while the lock was dropped for WAL I/O.
		if engine.bgErr != nil {
			engine.mu.Unlock()
			return engine.bgErr
		}

		for _, operation := range operations {
			if operation.Type == OpPut {
				_ = engine.memtable.Put(operation.Key, operation.Value)
			} else {
				_ = engine.memtable.Delete(operation.Key)
			}
		}

		engine.mu.Unlock()
		return nil
	}
}

// Get retrieves a key-value record from memory or SSTable files.
func (engine *dbEngine) Get(key []byte) ([]byte, error) {
	if len(key) == 0 {
		return nil, memtable.ErrEmptyKey
	}

	engine.mu.RLock()
	if engine.bgErr != nil {
		engine.mu.RUnlock()
		return nil, engine.bgErr
	}
	if engine.isClosing {
		engine.mu.RUnlock()
		return nil, fmt.Errorf("engine is closing")
	}

	// Search active memtable.
	value, found, deleted, err := engine.memtable.GetWithTombstone(key)
	if err != nil {
		engine.mu.RUnlock()
		return nil, err
	}
	if found {
		engine.mu.RUnlock()
		if deleted {
			return nil, ErrKeyNotFound
		}
		return value, nil
	}

	// Search immutable memtable.
	if engine.immMemtable != nil {
		value, found, deleted, err := engine.immMemtable.GetWithTombstone(key)
		if err != nil {
			engine.mu.RUnlock()
			return nil, err
		}
		if found {
			engine.mu.RUnlock()
			if deleted {
				return nil, ErrKeyNotFound
			}
			return value, nil
		}
	}

	var level0 []*sstable.Reader
	var level1 []*sstable.Reader

	for _, sstableReader := range engine.levels[0] {
		level0 = append(level0, sstableReader)
	}
	for _, sstableReader := range engine.levels[1] {
		level1 = append(level1, sstableReader)
	}

	engine.mu.RUnlock()

	// Pin all snapshotted readers using the lightweight sstRefsMu.
	engine.sstRefsMu.Lock()
	for _, sstableReader := range level0 {
		engine.pinSSTable(sstableReader)
	}
	for _, sstableReader := range level1 {
		engine.pinSSTable(sstableReader)
	}
	engine.sstRefsMu.Unlock()

	defer func() {
		engine.sstRefsMu.Lock()
		for _, sstableReader := range level0 {
			engine.unpinSSTable(sstableReader)
		}
		for _, sstableReader := range level1 {
			engine.unpinSSTable(sstableReader)
		}
		engine.sstRefsMu.Unlock()
	}()

	// Search Level 0 SSTables (overlapping ranges, search newest to oldest).
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

	// Search Level 1 SSTables (non-overlapping ranges, binary search on MaxKey).
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

// Scan returns a prefix-filtering iterator sorted by key.
func (engine *dbEngine) Scan(prefix []byte) Iterator {
	engine.mu.Lock()
	defer engine.mu.Unlock()

	var pinned []*sstable.Reader

	var level0 []*sstable.Reader
	for _, sstableReader := range engine.levels[0] {
		engine.sstRefsMu.Lock()
		engine.pinSSTable(sstableReader)
		engine.sstRefsMu.Unlock()
		level0 = append(level0, sstableReader)
		pinned = append(pinned, sstableReader)
	}

	var level1 []*sstable.Reader
	for _, sstableReader := range engine.levels[1] {
		engine.sstRefsMu.Lock()
		engine.pinSSTable(sstableReader)
		engine.sstRefsMu.Unlock()
		level1 = append(level1, sstableReader)
		pinned = append(pinned, sstableReader)
	}

	var iterators []internalIterator

	// Active memtable iterator.
	iterators = append(iterators, newMemAdapter(engine.memtable.NewIteratorAt(prefix)))

	// Immutable memtable iterator (if flushing is in progress).
	if engine.immMemtable != nil {
		iterators = append(iterators, newMemAdapter(engine.immMemtable.NewIteratorAt(prefix)))
	}

	// Level 0 SSTable iterators (all files, as L0 ranges overlap).
	for _, sstableReader := range level0 {
		sstableIterator, err := sstableReader.NewIteratorAt(prefix)
		if err == nil {
			iterators = append(iterators, newSstAdapter(sstableIterator))
		}
	}

	// Level 1 SSTable iterators (only files whose key range overlaps the prefix).
	var prefixLimit []byte
	if len(prefix) > 0 {
		prefixLimit = make([]byte, len(prefix))
		copy(prefixLimit, prefix)
		// Compute the exclusive upper bound of the prefix by incrementing the
		// last byte, carrying over if it wraps (handles all-0xFF prefixes).
		for i := len(prefixLimit) - 1; i >= 0; i-- {
			prefixLimit[i]++
			if prefixLimit[i] != 0 {
				break
			}
		}
	}

	for _, sstableReader := range level1 {
		overlap := true
		if len(prefix) > 0 {
			if bytes.Compare(sstableReader.MaxKey(), prefix) < 0 {
				overlap = false
			}
			if len(prefixLimit) > 0 && bytes.Compare(sstableReader.MinKey(), prefixLimit) >= 0 {
				overlap = false
			}
		}
		if overlap {
			sstableIterator, err := sstableReader.NewIteratorAt(prefix)
			if err == nil {
				iterators = append(iterators, newSstAdapter(sstableIterator))
			}
		}
	}

	engine.iterWg.Add(1)

	mergingIteratorInstance := &mergingIterator{
		engine: engine,
		pinned: pinned,
		iters:  iterators,
		prefix: prefix,
	}
	mergingIteratorInstance.findNext()

	return mergingIteratorInstance
}

// Close flushes in-memory contents and safely releases lock and worker resources.
func (engine *dbEngine) Close() error {
	engine.mu.Lock()
	if engine.isClosing {
		engine.mu.Unlock()
		return nil
	}
	engine.isClosing = true
	engine.mu.Unlock()

	// Wait for all active user iterators to release their pinned readers.
	engine.iterWg.Wait()

	// Trigger a final flush of the active memtable if it holds any data.
	engine.mu.Lock()
	if engine.memtable.Size() > 0 && engine.bgErr == nil {
		// Wait for any in-progress flush to complete before freezing.
		for engine.immMemtable != nil {
			engine.flushCond.Wait()
		}

		engine.immMemtable = engine.memtable
		engine.immWALSegmentID = engine.activeWALSegmentID

		// Close the WAL exactly once here and nil the pointer to
		// prevent a second Close() call in the cleanup block below.
		if engine.wal != nil {
			_ = engine.wal.Close()
			engine.wal = nil
		}

		engine.memtable = memtable.NewSkipList(engine.opts.MaxMemTableSize, engine.opts.MemTableMaxLevel)

		select {
		case engine.flushChan <- struct{}{}:
		default:
		}
	}
	engine.mu.Unlock()

	// Signal both workers to terminate. Close their channels so a blocked
	// select wakes immediately.
	close(engine.flushCloseChan)
	close(engine.compactCloseChan)

	engine.flushWg.Wait()
	engine.compactWg.Wait()

	engine.mu.Lock()
	defer engine.mu.Unlock()

	// Close the WAL if it wasn't already closed above.
	if engine.wal != nil {
		_ = engine.wal.Close()
		engine.wal = nil
	}

	// Release the exclusive directory lock.
	if engine.lock != nil {
		_ = engine.lock.Close()
	}

	// Close all remaining SSTable readers.
	for _, readerList := range engine.levels {
		for _, sstableReader := range readerList {
			_ = sstableReader.Close()
		}
	}

	return engine.bgErr
}

// flushWorker is the background goroutine that serializes immutable memtables
// to Level 0 SSTables.
//
// When the close channel fires, the worker always checks for a pending
// immMemtable and flushes it before returning, preventing data loss on graceful
// shutdown. The close channel is only used to interrupt a blocking wait when
// there is nothing to flush; it never causes an early exit if work remains.
func (engine *dbEngine) flushWorker() {
	defer engine.flushWg.Done()

	for {
		select {
		case <-engine.flushChan:
		case <-engine.flushCloseChan:
			// Drain any final immMemtable before exiting. Close() always
			// enqueues a flush on flushChan before closing flushCloseChan,
			// but we also check here to handle the race where Go's select
			// picks this branch non-deterministically.
			engine.mu.Lock()
			if engine.immMemtable == nil {
				engine.mu.Unlock()
				return
			}
			// Fall through to flush the pending immutable memtable below.
			engine.mu.Unlock()
		}

		engine.mu.Lock()
		if engine.immMemtable == nil {
			// Nothing to flush. If we are closing, exit; otherwise wait for work.
			if engine.isClosing {
				engine.mu.Unlock()
				return
			}
			engine.mu.Unlock()
			continue
		}
		immutable := engine.immMemtable
		segmentID := engine.immWALSegmentID
		engine.mu.Unlock()

		// Write the immutable memtable to a new Level 0 SSTable.
		// The lock is not held during disk I/O.
		sstableFilename := fmt.Sprintf("%06d.sst", segmentID)
		sstablePath := filepath.Join(engine.dir, sstableFilename)

		var flushErr error
		sstableWriter, err := sstable.NewWriter(sstablePath, 10000)
		if err != nil {
			flushErr = fmt.Errorf("flush failed to create writer: %w", err)
		} else {
			iterator := immutable.NewIterator()
			for iterator.Valid() {
				key, value, isDeleted := iterator.Next()
				var opcode uint8 = sstable.OpcodePut
				if isDeleted {
					opcode = sstable.OpcodeDelete
				}
				if err := sstableWriter.Add(key, value, opcode); err != nil {
					flushErr = fmt.Errorf("flush failed to add key: %w", err)
					break
				}
			}
			if closeErr := sstableWriter.Close(); closeErr != nil && flushErr == nil {
				flushErr = fmt.Errorf("flush failed to finalize: %w", closeErr)
			}
		}

		var sstableReader *sstable.Reader
		if flushErr == nil {
			sstableReader, err = sstable.Open(sstablePath)
			if err != nil {
				flushErr = fmt.Errorf("flush failed to open reader: %w", err)
			}
		}

		engine.mu.Lock()
		if flushErr != nil {
			_ = os.Remove(sstablePath)
			engine.bgErr = flushErr
			engine.immMemtable = nil
			engine.flushCond.Broadcast()
			engine.mu.Unlock()
			return
		}

		// Prepend the new file to Level 0 (newest first).
		engine.levels[0] = append([]*sstable.Reader{sstableReader}, engine.levels[0]...)
		engine.sstRefsMu.Lock()
		engine.sstRefs[sstableReader] = &sstableRef{reader: sstableReader, refs: 0, obsolete: false}
		engine.sstRefsMu.Unlock()

		engine.immMemtable = nil
		engine.flushCond.Broadcast()

		manifest := &Manifest{
			NextSegmentID: engine.nextSegmentID,
			Levels:        engine.manifestLevels(),
		}
		_ = writeManifest(engine.dir, manifest)

		// Remove the corresponding WAL segment — its data is now in the SSTable.
		_ = os.Remove(filepath.Join(engine.walDir, fmt.Sprintf("%06d.wal", segmentID)))

		triggerCompaction := len(engine.levels[0]) >= engine.opts.CompactionThreshold
		engine.mu.Unlock()

		if triggerCompaction {
			select {
			case engine.compactChan <- struct{}{}:
			default:
			}
		}

		// If Close() fired the close channel and this was the final flush, exit.
		engine.mu.Lock()
		isClosingNow := engine.isClosing && engine.immMemtable == nil
		engine.mu.Unlock()
		if isClosingNow {
			return
		}
	}
}

// compactionWorker is the background goroutine that merges Level 0 and Level 1
// SSTables when the L0 file count exceeds the configured threshold.
//
// The worker has a clean exit path for both the close channel and the
// normal-termination condition. When closing, it skips a new compaction if one
// is not already warranted, rather than looping forever on a closed channel.
func (engine *dbEngine) compactionWorker() {
	defer engine.compactWg.Done()

	for {
		select {
		case <-engine.compactChan:
			// Normal compaction trigger.
		case <-engine.compactCloseChan:
			// Shutdown signal: perform one final compaction pass if needed,
			// then exit. This drains any pending trigger that arrived before close.
			engine.mu.Lock()
			shouldCompact := !engine.isCompacting && len(engine.levels[0]) >= engine.opts.CompactionThreshold
			engine.mu.Unlock()
			if !shouldCompact {
				return
			}
		}

		engine.mu.Lock()
		if engine.isCompacting {
			// A compaction is already running; skip this trigger.
			engine.mu.Unlock()
			continue
		}
		if len(engine.levels[0]) < engine.opts.CompactionThreshold {
			// L0 is below the threshold; nothing to compact.
			if engine.isClosing {
				engine.mu.Unlock()
				return
			}
			engine.mu.Unlock()
			continue
		}

		engine.isCompacting = true

		// Collect paths and IDs for all L0 and L1 input files.
		var inputFiles []string
		var fileIDs []int
		var obsoleteReaders []*sstable.Reader

		for _, sstableReader := range engine.levels[0] {
			inputFiles = append(inputFiles, sstableReader.FilePath())
			var segmentID int
			_, _ = fmt.Sscanf(filepath.Base(sstableReader.FilePath()), "%d.sst", &segmentID)
			fileIDs = append(fileIDs, segmentID)
			obsoleteReaders = append(obsoleteReaders, sstableReader)
		}
		for _, sstableReader := range engine.levels[1] {
			inputFiles = append(inputFiles, sstableReader.FilePath())
			var segmentID int
			_, _ = fmt.Sscanf(filepath.Base(sstableReader.FilePath()), "%d.sst", &segmentID)
			fileIDs = append(fileIDs, segmentID)
			obsoleteReaders = append(obsoleteReaders, sstableReader)
		}

		// Reserve a contiguous block of segment IDs for the compaction output files.
		// The compactor receives only the first ID; it will produce one or more files.
		compactionSegID := engine.nextSegmentID
		engine.nextSegmentID++

		engine.mu.Unlock()

		task := &compactor.Task{
			InputFiles:      inputFiles,
			FileIDs:         fileIDs,
			OutputDirectory: engine.dir,
			NextSegmentID:   compactionSegID,
			IsBottomLevel:   true,
		}

		res, err := compactor.Run(task)

		engine.mu.Lock()
		if err != nil {
			engine.bgErr = fmt.Errorf("compaction failed: %w", err)
			engine.isCompacting = false
			engine.mu.Unlock()
			return
		}

		var newL1Readers []*sstable.Reader
		for _, compactedPath := range res.NewFilesCreated {
			sstableReader, err := sstable.Open(compactedPath)
			if err != nil {
				// If any output file fails to open, mark it as a background error
				// and clean up the readers that were successfully opened.
				for _, r := range newL1Readers {
					_ = r.Close()
					_ = os.Remove(r.FilePath())
				}
				engine.bgErr = fmt.Errorf("compaction failed to open output file %s: %w", compactedPath, err)
				engine.isCompacting = false
				engine.mu.Unlock()
				return
			}
			newL1Readers = append(newL1Readers, sstableReader)
		}

		// Replace L0 and the old L1 with the newly compacted files in L1.
		engine.levels[0] = nil
		engine.levels[1] = newL1Readers

		engine.sstRefsMu.Lock()
		for _, sstableReader := range newL1Readers {
			engine.sstRefs[sstableReader] = &sstableRef{reader: sstableReader, refs: 0, obsolete: false}
		}

		// Mark all pre-compaction files as obsolete. Files that are currently
		// pinned by active iterators are deferred for deletion until their last
		// reference is released via unpinSSTable.
		for _, obsR := range obsoleteReaders {
			if ref, ok := engine.sstRefs[obsR]; ok {
				ref.obsolete = true
				if ref.refs == 0 {
					_ = obsR.Close()
					_ = os.Remove(obsR.FilePath())
					delete(engine.sstRefs, obsR)
				}
			}
		}
		engine.sstRefsMu.Unlock()

		manifest := &Manifest{
			NextSegmentID: engine.nextSegmentID,
			Levels:        engine.manifestLevels(),
		}
		_ = writeManifest(engine.dir, manifest)

		engine.isCompacting = false
		engine.mu.Unlock()
	}
}

// pinSSTable increments the active reference count for the given SSTable reader.
// Must be called with sstRefsMu held.
func (engine *dbEngine) pinSSTable(sstableReader *sstable.Reader) {
	if ref, ok := engine.sstRefs[sstableReader]; ok {
		ref.refs++
	}
}

// unpinSSTable decrements the active reference count for the given SSTable reader.
// When the count reaches zero and the file has been marked obsolete by compaction,
// the reader is closed and the file is deleted from disk.
// Must be called with sstRefsMu held.
func (engine *dbEngine) unpinSSTable(sstableReader *sstable.Reader) {
	if ref, ok := engine.sstRefs[sstableReader]; ok {
		ref.refs--
		if ref.refs == 0 && ref.obsolete {
			_ = sstableReader.Close()
			_ = os.Remove(sstableReader.FilePath())
			delete(engine.sstRefs, sstableReader)
		}
	}
}

// manifestLevels builds the file basename mapping required by the atomic manifest writer.
// Must be called with engine.mu held.
func (engine *dbEngine) manifestLevels() map[int][]string {
	mLevels := make(map[int][]string)
	for level, readerList := range engine.levels {
		names := make([]string, 0, len(readerList))
		for _, sstableReader := range readerList {
			names = append(names, filepath.Base(sstableReader.FilePath()))
		}
		mLevels[level] = names
	}
	return mLevels
}

// internalIterator wraps memory & SSTable iterators into a uniform peekable cursor
// interface consumed by mergingIterator.
type internalIterator interface {
	Valid() bool
	Key() []byte
	Value() []byte
	IsDeleted() bool
	Next()
	Close()
}

// memAdapter adapts a *memtable.Iterator into the internalIterator interface.
type memAdapter struct {
	// iter is the underlying memtable iterator.
	iter *memtable.Iterator
	// hasCurrent is true when the adapter is positioned on a valid entry.
	hasCurrent bool
	// currKey is the key of the current entry.
	currKey []byte
	// currVal is the value of the current entry.
	currVal []byte
	// currDel is true when the current entry is a tombstone.
	currDel bool
}

// newMemAdapter creates a memAdapter and positions it on the first valid entry.
func newMemAdapter(iter *memtable.Iterator) *memAdapter {
	adapter := &memAdapter{iter: iter}
	adapter.Next()
	return adapter
}

func (adapter *memAdapter) Valid() bool     { return adapter.hasCurrent }
func (adapter *memAdapter) Key() []byte     { return adapter.currKey }
func (adapter *memAdapter) Value() []byte   { return adapter.currVal }
func (adapter *memAdapter) IsDeleted() bool { return adapter.currDel }

func (adapter *memAdapter) Next() {
	if adapter.iter.Valid() {
		adapter.currKey, adapter.currVal, adapter.currDel = adapter.iter.Next()
		adapter.hasCurrent = true
	} else {
		adapter.hasCurrent = false
		adapter.currKey, adapter.currVal = nil, nil
	}
}

func (adapter *memAdapter) Close() {}

// sstAdapter adapts a *sstable.Iterator into the internalIterator interface.
type sstAdapter struct {
	// iter is the underlying SSTable iterator.
	iter *sstable.Iterator
	// hasCurrent is true when the iterator advanced to a valid entry.
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

// mergingIterator merges multiple internalIterators into a single sorted,
// deduplicated, tombstone-filtered cursor that implements the public Iterator
// interface.
type mergingIterator struct {
	// engine is a back-reference used to release pinned readers on Close.
	engine *dbEngine
	// pinned holds all SSTable readers pinned for the duration of this iterator.
	pinned []*sstable.Reader
	// iters are the underlying per-source iterators being merged.
	iters []internalIterator
	// prefix is the scan prefix filter; an empty slice means no filtering.
	prefix []byte
	// currKey is the key of the current merged entry.
	currKey []byte
	// currVal is the value of the current merged entry.
	currVal []byte
	// valid is true when the iterator holds a usable entry.
	valid bool
	// closed tracks whether Close has been called.
	closed bool
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
//
// The deduplicate-and-advance loop compares each sub-iterator's key
// against keyCopy (a safe heap copy) rather than the raw smallestKey pointer.
// smallestKey is a direct reference into the sub-iterator's internal buffer,
// which may be overwritten the moment that iterator's Next() is called. Using
// keyCopy prevents stale/garbage comparisons.
func (iterator *mergingIterator) findNext() {
	for {
		var smallestKey []byte
		var smallestIdx = -1

		// Find the sub-iterator holding the lexicographically smallest key.
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
			// All sub-iterators are exhausted.
			iterator.valid = false
			iterator.currKey, iterator.currVal = nil, nil
			return
		}

		// Stop scanning if the smallest key no longer shares the prefix.
		if len(iterator.prefix) > 0 && !bytes.HasPrefix(smallestKey, iterator.prefix) {
			iterator.valid = false
			iterator.currKey, iterator.currVal = nil, nil
			return
		}

		// Capture the deletion flag and value before advancing any iterator.
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

		// Advance every sub-iterator that is positioned on the same key.
		// Use keyCopy here (Fix #4), not smallestKey, to avoid comparing
		// against a buffer that has been mutated by a preceding Next() call.
		for _, subIterator := range iterator.iters {
			if subIterator.Valid() && bytes.Equal(subIterator.Key(), keyCopy) {
				subIterator.Next()
			}
		}

		// Skip tombstone entries; they represent logical deletions.
		if !isDeleted {
			iterator.currKey = keyCopy
			iterator.currVal = valCopy
			iterator.valid = true
			return
		}
	}
}

// createWALWriter applies WALOptions configuration functionally to initialize a LogWriter.
func createWALWriter(walDir string, segmentID int, walOptions wal.Options) (*wal.LogWriter, error) {
	var walOpts []wal.Option
	if walOptions.SegmentSizeBytes > 0 {
		walOpts = append(walOpts, wal.WithSegmentSizeBytes(walOptions.SegmentSizeBytes))
	}
	if walOptions.BatchSizeBytes > 0 {
		walOpts = append(walOpts, wal.WithBatchSizeBytes(walOptions.BatchSizeBytes))
	}
	if walOptions.IngestChannelCapacity > 0 {
		walOpts = append(walOpts, wal.WithIngestChannelCapacity(walOptions.IngestChannelCapacity))
	}
	return wal.NewLogWriter(walDir, segmentID, walOpts...)
}

// cleanupWALFiles deletes WAL segment files with IDs up to and including
// upToSegmentID. These segments have already been fully recovered and flushed
// to an SSTable, so they are no longer needed for crash recovery.
func cleanupWALFiles(walDir string, upToSegmentID int) {
	entries, err := os.ReadDir(walDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".wal" {
			continue
		}
		var segmentID int
		if n, _ := fmt.Sscanf(entry.Name(), "%d.wal", &segmentID); n == 1 {
			if segmentID <= upToSegmentID {
				_ = os.Remove(filepath.Join(walDir, entry.Name()))
			}
		}
	}
}
