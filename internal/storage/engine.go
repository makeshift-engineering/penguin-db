package storage

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
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

	sstRefs := make(map[*sstable.Reader]*sstableRef)
	levels, err := openManifestLevels(dir, manifest.Levels, sstRefs)
	if err != nil {
		return nil, err
	}

	// Replay WAL segments to reconstruct in-memory state after a crash.
	recoveryMem := memtable.NewSkipList(math.MaxInt64, opts.MemTableMaxLevel)
	highestWALSegmentID, err := wal.Replay(walDir, recoveryMem)
	if err != nil {
		closeOpenedLevels(levels)
		return nil, fmt.Errorf("failed to replay WAL: %w", err)
	}

	engineInstance := &dbEngine{
		dir:              dir,
		walDir:           walDir,
		opts:             opts,
		lock:             lock,
		levels:           levels,
		sstRefs:          sstRefs,
		nextSegmentID:    manifest.NextSegmentID,
		flushChan:        make(chan struct{}, 1),
		flushCloseChan:   make(chan struct{}),
		compactChan:      make(chan struct{}, 1),
		compactCloseChan: make(chan struct{}),
	}
	engineInstance.flushCond = sync.NewCond(&engineInstance.mu)

	// Recover active WAL and MemTable state.
	if err := engineInstance.recoverActiveState(recoveryMem, manifest, highestWALSegmentID); err != nil {
		closeOpenedLevels(levels)
		return nil, err
	}

	// Launch background workers.
	engineInstance.flushWg.Add(1)
	go engineInstance.flushWorker()

	engineInstance.compactWg.Add(1)
	go engineInstance.compactionWorker()

	success = true
	return engineInstance, nil
}

// openManifestLevels opens all SSTables listed in the manifest and registers them in the ref map.
func openManifestLevels(dir string, manifestLevels map[int][]string, sstRefs map[*sstable.Reader]*sstableRef) (map[int][]*sstable.Reader, error) {
	levels := make(map[int][]*sstable.Reader)
	levels[0] = nil
	levels[1] = nil

	for level, filenames := range manifestLevels {
		readers := make([]*sstable.Reader, 0, len(filenames))
		for _, name := range filenames {
			path := filepath.Join(dir, name)
			sstableReader, err := sstable.Open(path)
			if err != nil {
				levels[level] = readers
				closeOpenedLevels(levels)
				return nil, fmt.Errorf("failed to open SSTable %s: %w", path, err)
			}
			readers = append(readers, sstableReader)
			sstRefs[sstableReader] = &sstableRef{reader: sstableReader, refs: 0, obsolete: false}
		}
		levels[level] = readers
	}
	return levels, nil
}

// closeOpenedLevels closes all opened readers registered in the levels map.
func closeOpenedLevels(levels map[int][]*sstable.Reader) {
	for _, readerList := range levels {
		for _, openedReader := range readerList {
			_ = openedReader.Close()
		}
	}
}

// recoverActiveState initializes the active WAL and MemTable from the replayed/recovered state.
func (engine *dbEngine) recoverActiveState(recoveryMem *memtable.SkipList, manifest *Manifest, highestWALSegmentID int) error {
	var err error
	if recoveryMem.Size() > engine.opts.MaxMemTableSize {
		// Recovery memtable exceeded the size limit: flush it directly to L0.
		sstableFilename := fmt.Sprintf("%06d.sst", engine.nextSegmentID)
		sstablePath := filepath.Join(engine.dir, sstableFilename)

		sstableReader, err := writeMemTableToSSTable(sstablePath, recoveryMem)
		if err != nil {
			return fmt.Errorf("failed to flush recovery memtable: %w", err)
		}

		engine.levels[0] = append([]*sstable.Reader{sstableReader}, engine.levels[0]...)
		manifest.Levels[0] = append([]string{sstableFilename}, manifest.Levels[0]...)
		engine.sstRefs[sstableReader] = &sstableRef{reader: sstableReader, refs: 0, obsolete: false}

		engine.nextSegmentID++
		engine.activeWALSegmentID = engine.nextSegmentID
		engine.nextSegmentID++

		manifest.NextSegmentID = engine.nextSegmentID
		if err := writeManifest(engine.dir, manifest); err != nil {
			sstableReader.Close()
			return fmt.Errorf("failed to save manifest during recovery flush: %w", err)
		}

		cleanupWALFiles(engine.walDir, highestWALSegmentID)

		engine.memtable = memtable.NewSkipList(engine.opts.MaxMemTableSize, engine.opts.MemTableMaxLevel)
		engine.wal, err = createWALWriter(engine.walDir, engine.activeWALSegmentID, engine.opts.WALOptions)
		if err != nil {
			return fmt.Errorf("failed to initialize active WAL writer: %w", err)
		}
	} else {
		// Recovery memtable fits in memory: resume from the highest replayed WAL segment.
		engine.memtable = recoveryMem
		engine.activeWALSegmentID = highestWALSegmentID
		engine.wal, err = createWALWriter(engine.walDir, engine.activeWALSegmentID, engine.opts.WALOptions)
		if err != nil {
			return fmt.Errorf("failed to resume active WAL writer: %w", err)
		}
		if engine.activeWALSegmentID >= engine.nextSegmentID {
			engine.nextSegmentID = engine.activeWALSegmentID + 1
			manifest.NextSegmentID = engine.nextSegmentID
			_ = writeManifest(engine.dir, manifest)
		}
	}
	return nil
}

// writeMemTableToSSTable dumps the contents of a MemTable SkipList to a new SSTable file.
func writeMemTableToSSTable(path string, mem *memtable.SkipList) (*sstable.Reader, error) {
	sstableWriter, err := sstable.NewWriter(path, 10000)
	if err != nil {
		return nil, err
	}

	iterator := mem.NewIterator()
	for iterator.Valid() {
		key, value, isDeleted := iterator.Next()
		opcode := sstable.OpcodePut
		if isDeleted {
			opcode = sstable.OpcodeDelete
		}
		if err := sstableWriter.Add(key, value, opcode); err != nil {
			_ = sstableWriter.Close()
			_ = os.Remove(path)
			return nil, err
		}
	}

	if err := sstableWriter.Close(); err != nil {
		_ = os.Remove(path)
		return nil, err
	}

	return sstable.Open(path)
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

// validateOperations checks constraints on WriteBatch parameters and calculates cumulative size.
func validateOperations(operations []Op) (int64, error) {
	var size int64
	for _, operation := range operations {
		if len(operation.Key) == 0 {
			return 0, memtable.ErrEmptyKey
		}
		switch operation.Type {
		case OpPut:
			size += int64(len(operation.Key) + len(operation.Value))
		case OpDelete:
			size += int64(len(operation.Key))
		default:
			return 0, fmt.Errorf("invalid write operation type: %v", operation.Type)
		}
	}
	return size, nil
}

// rotateActiveMemTableAndWAL rotates full active components and kicks off a flush.
// Must be called with lock held.
func (engine *dbEngine) rotateActiveMemTableAndWAL() error {
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
		return err
	}
	engine.wal = newWAL

	manifest := &Manifest{
		NextSegmentID: engine.nextSegmentID,
		Levels:        engine.manifestLevels(),
	}
	if err := writeManifest(engine.dir, manifest); err != nil {
		engine.bgErr = err
		return err
	}

	// Signal the background flush worker.
	select {
	case engine.flushChan <- struct{}{}:
	default:
	}

	return nil
}

// WriteBatch writes multiple operations atomically to the WAL and memtable.
func (engine *dbEngine) WriteBatch(operations []Op) error {
	if len(operations) == 0 {
		return nil
	}

	batchSize, err := validateOperations(operations)
	if err != nil {
		return err
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
				engine.flushCond.Wait()
				continue
			}

			if err := engine.rotateActiveMemTableAndWAL(); err != nil {
				engine.mu.Unlock()
				return err
			}
			continue
		}

		// Build the WAL records for this batch.
		walRecords := make([]*wal.Record, 0, len(operations))
		for _, operation := range operations {
			walOpcode := wal.OpcodePut
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

		engine.mu.Lock()

		// Guard against a racing Close() or background error while lock was dropped.
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

	level0 := make([]*sstable.Reader, 0, len(engine.levels[0]))
	level0 = append(level0, engine.levels[0]...)

	level1 := make([]*sstable.Reader, 0, len(engine.levels[1]))
	level1 = append(level1, engine.levels[1]...)

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

	pinned := make([]*sstable.Reader, 0, len(engine.levels[0])+len(engine.levels[1]))

	level0 := make([]*sstable.Reader, 0, len(engine.levels[0]))
	for _, sstableReader := range engine.levels[0] {
		engine.sstRefsMu.Lock()
		engine.pinSSTable(sstableReader)
		engine.sstRefsMu.Unlock()
		level0 = append(level0, sstableReader)
		pinned = append(pinned, sstableReader)
	}

	level1 := make([]*sstable.Reader, 0, len(engine.levels[1]))
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
		for engine.immMemtable != nil {
			engine.flushCond.Wait()
		}

		engine.immMemtable = engine.memtable
		engine.immWALSegmentID = engine.activeWALSegmentID

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

	// Signal both workers to terminate.
	close(engine.flushCloseChan)
	close(engine.compactCloseChan)

	engine.flushWg.Wait()
	engine.compactWg.Wait()

	engine.mu.Lock()
	defer engine.mu.Unlock()

	if engine.wal != nil {
		_ = engine.wal.Close()
		engine.wal = nil
	}

	if engine.lock != nil {
		_ = engine.lock.Close()
	}

	for _, readerList := range engine.levels {
		for _, sstableReader := range readerList {
			_ = sstableReader.Close()
		}
	}

	return engine.bgErr
}

// flushWorker is the background goroutine that serializes immutable memtables to Level 0 SSTables.
func (engine *dbEngine) flushWorker() {
	defer engine.flushWg.Done()

	for {
		select {
		case <-engine.flushChan:
		case <-engine.flushCloseChan:
			engine.mu.Lock()
			if engine.immMemtable == nil {
				engine.mu.Unlock()
				return
			}
			engine.mu.Unlock()
		}

		engine.mu.Lock()
		if engine.immMemtable == nil {
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

		sstableFilename := fmt.Sprintf("%06d.sst", segmentID)
		sstablePath := filepath.Join(engine.dir, sstableFilename)

		sstableReader, flushErr := writeMemTableToSSTable(sstablePath, immutable)

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

		// Remove the corresponding WAL segment.
		_ = os.Remove(filepath.Join(engine.walDir, fmt.Sprintf("%06d.wal", segmentID)))

		triggerCompaction := len(engine.levels[0]) >= engine.opts.CompactionThreshold
		engine.mu.Unlock()

		if triggerCompaction {
			select {
			case engine.compactChan <- struct{}{}:
			default:
			}
		}

		engine.mu.Lock()
		isClosingNow := engine.isClosing && engine.immMemtable == nil
		engine.mu.Unlock()
		if isClosingNow {
			return
		}
	}
}

// compactionWorker is the background goroutine that merges Level 0 and Level 1 SSTables.
func (engine *dbEngine) compactionWorker() {
	defer engine.compactWg.Done()

	for {
		select {
		case <-engine.compactChan:
		case <-engine.compactCloseChan:
			engine.mu.Lock()
			shouldCompact := !engine.isCompacting && len(engine.levels[0]) >= engine.opts.CompactionThreshold
			engine.mu.Unlock()
			if !shouldCompact {
				return
			}
		}

		engine.mu.Lock()
		if engine.isCompacting {
			engine.mu.Unlock()
			continue
		}
		if len(engine.levels[0]) < engine.opts.CompactionThreshold {
			if engine.isClosing {
				engine.mu.Unlock()
				return
			}
			engine.mu.Unlock()
			continue
		}

		engine.isCompacting = true

		inputFiles, fileIDs, obsoleteReaders := engine.collectCompactionInputs()

		compactionSegID := engine.nextSegmentID
		engine.nextSegmentID++

		engine.mu.Unlock()

		if err := engine.runAndRegisterCompaction(inputFiles, fileIDs, compactionSegID, obsoleteReaders); err != nil {
			return
		}
	}
}

// collectCompactionInputs gathers the files and readers that will be merged.
// Must be called with lock held.
func (engine *dbEngine) collectCompactionInputs() (inputFiles []string, fileIDs []int, obsoleteReaders []*sstable.Reader) {
	capacity := len(engine.levels[0]) + len(engine.levels[1])
	inputFiles = make([]string, 0, capacity)
	fileIDs = make([]int, 0, capacity)
	obsoleteReaders = make([]*sstable.Reader, 0, capacity)

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
	return inputFiles, fileIDs, obsoleteReaders
}

// runAndRegisterCompaction runs the compaction work and registers the result in Level 1.
func (engine *dbEngine) runAndRegisterCompaction(inputFiles []string, fileIDs []int, compactionSegID int, obsoleteReaders []*sstable.Reader) error {
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
		return err
	}

	var newL1Readers []*sstable.Reader
	for _, compactedPath := range res.NewFilesCreated {
		sstableReader, err := sstable.Open(compactedPath)
		if err != nil {
			for _, r := range newL1Readers {
				_ = r.Close()
				_ = os.Remove(r.FilePath())
			}
			engine.bgErr = fmt.Errorf("compaction failed to open output file %s: %w", compactedPath, err)
			engine.isCompacting = false
			engine.mu.Unlock()
			return err
		}
		newL1Readers = append(newL1Readers, sstableReader)
	}

	engine.levels[0] = nil
	engine.levels[1] = newL1Readers

	engine.sstRefsMu.Lock()
	for _, sstableReader := range newL1Readers {
		engine.sstRefs[sstableReader] = &sstableRef{reader: sstableReader, refs: 0, obsolete: false}
	}

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
	return nil
}

// pinSSTable increments the active reference count for the given SSTable reader.
// Must be called with sstRefsMu held.
func (engine *dbEngine) pinSSTable(sstableReader *sstable.Reader) {
	if ref, ok := engine.sstRefs[sstableReader]; ok {
		ref.refs++
	}
}

// unpinSSTable decrements the active reference count for the given SSTable reader.
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

// internalIterator wraps memory & SSTable iterators into a uniform peekable cursor.
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
	iter       *memtable.Iterator
	hasCurrent bool
	currKey    []byte
	currVal    []byte
	currDel    bool
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

// cleanupWALFiles deletes WAL segment files with IDs up to and including upToSegmentID.
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
