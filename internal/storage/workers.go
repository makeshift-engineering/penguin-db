package storage

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/makeshift-engineering/penguin-db/internal/storage/compactor"
	"github.com/makeshift-engineering/penguin-db/internal/storage/memtable"
	"github.com/makeshift-engineering/penguin-db/internal/storage/sstable"
)

// nextFlushWork checks if there is any immutable memtable to flush.
// Returns the memtable, its segment ID, and a boolean indicating whether the worker should exit.
// Must be called with engine.mu held.
func (engine *dbEngine) nextFlushWork() (*memtable.SkipList, int, bool) {
	if len(engine.immMemtables) == 0 {
		return nil, 0, engine.isClosing
	}
	return engine.immMemtables[0], engine.immWALSegmentIDs[0], false
}

// failFlush registers a background flush error, dequeues the failed memtable, and broadcasts.
// Must be called with engine.mu held.
func (engine *dbEngine) failFlush(err error) {
	engine.bgErr = err
	engine.flushCond.Broadcast()
}

// registerFlushedSSTable inserts the flushed L0 reader, dequeues the flushed memtable,
// and returns the manifest to write.
// Must be called with engine.mu held.
func (engine *dbEngine) registerFlushedSSTable(sstableReader *sstable.Reader, segmentID int) *Manifest {
	// Prepend the new file to Level 0 (newest first).
	engine.levels[levelZero] = append([]*sstable.Reader{sstableReader}, engine.levels[levelZero]...)
	engine.sstRefsMu.Lock()
	engine.sstRefs[sstableReader] = &sstableRef{reader: sstableReader, refs: 0, obsolete: false}
	engine.sstRefsMu.Unlock()

	engine.immMemtables = engine.immMemtables[1:]
	engine.immWALSegmentIDs = engine.immWALSegmentIDs[1:]
	engine.flushedSegmentID = segmentID
	engine.flushCond.Broadcast()

	return &Manifest{
		NextSegmentID:    engine.nextSegmentID,
		Levels:           engine.manifestLevels(),
		FlushedSegmentID: engine.flushedSegmentID,
	}
}

// flushWorker is the background goroutine that serializes immutable memtables to Level 0 SSTables.
func (engine *dbEngine) flushWorker() {
	defer engine.flushWg.Done()

	for {
		select {
		case <-engine.flushChan:
		case <-engine.flushCloseChan:
			engine.mu.Lock()
			if len(engine.immMemtables) == 0 {
				engine.mu.Unlock()
				return
			}
			engine.mu.Unlock()
		}

		engine.mu.Lock()
		immutable, segmentID, shouldExit := engine.nextFlushWork()
		if shouldExit {
			engine.mu.Unlock()
			return
		}
		if immutable == nil {
			engine.mu.Unlock()
			continue
		}
		engine.mu.Unlock()

		sstableFilename := fmt.Sprintf("%06d.sst", segmentID)
		sstablePath := filepath.Join(engine.dir, sstableFilename)

		startTime := time.Now()
		sstableReader, flushErr := writeMemTableToSSTable(sstablePath, immutable)

		if flushErr != nil {
			if err := os.Remove(sstablePath); err != nil && !os.IsNotExist(err) {
				slog.Warn("failed to clean up sstable file after flush error", "path", sstablePath, "error", err)
			}
			engine.mu.Lock()
			engine.failFlush(flushErr)
			engine.mu.Unlock()
			return
		}

		engine.manifestMu.Lock()
		engine.mu.Lock()
		manifest := engine.registerFlushedSSTable(sstableReader, segmentID)
		engine.mu.Unlock()

		writeErr := writeManifest(engine.dir, manifest)
		engine.manifestMu.Unlock()

		engine.mu.Lock()
		if writeErr != nil {
			engine.bgErr = writeErr
			engine.flushCond.Broadcast()
			engine.mu.Unlock()
			return
		}
		engine.mu.Unlock()

		// Remove the corresponding WAL segment.
		walPath := filepath.Join(engine.walDir, fmt.Sprintf("%06d.wal", segmentID))
		if err := os.Remove(walPath); err != nil {
			slog.Debug("failed to remove flushed WAL file (might be open on Windows)", "segment_id", segmentID, "error", err)
		}

		var bytesWritten int64
		if fi, err := os.Stat(sstablePath); err == nil {
			bytesWritten = fi.Size()
		}
		engine.opts.Metrics.RecordFlush(time.Since(startTime).Milliseconds(), bytesWritten)

		engine.mu.Lock()
		triggerCompaction := len(engine.levels[levelZero]) >= engine.opts.CompactionThreshold
		engine.mu.Unlock()

		if triggerCompaction {
			select {
			case engine.compactChan <- struct{}{}:
			default:
			}
		}

		engine.mu.Lock()
		isClosingNow := engine.isClosing && len(engine.immMemtables) == 0
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
			shouldCompact := !engine.isCompacting && len(engine.levels[levelZero]) >= engine.opts.CompactionThreshold
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
		if len(engine.levels[levelZero]) < engine.opts.CompactionThreshold {
			if engine.isClosing {
				engine.mu.Unlock()
				return
			}
			engine.mu.Unlock()
			continue
		}

		engine.isCompacting = true

		inputFiles, fileIDs, obsoleteReaders, err := engine.collectCompactionInputs()
		if err != nil {
			engine.bgErr = err
			engine.isCompacting = false
			engine.flushCond.Broadcast()
			engine.mu.Unlock()
			return
		}

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
func (engine *dbEngine) collectCompactionInputs() (inputFiles []string, fileIDs []int, obsoleteReaders []*sstable.Reader, err error) {
	capacity := len(engine.levels[levelZero]) + len(engine.levels[levelOne])
	inputFiles = make([]string, 0, capacity)
	fileIDs = make([]int, 0, capacity)
	obsoleteReaders = make([]*sstable.Reader, 0, capacity)

	for _, level := range []int{levelZero, levelOne} {
		for _, sstableReader := range engine.levels[level] {
			inputFiles = append(inputFiles, sstableReader.FilePath())
			segmentID, parseErr := parseSegmentID(filepath.Base(sstableReader.FilePath()))
			if parseErr != nil {
				return nil, nil, nil, fmt.Errorf("failed to parse segment ID from sstable path %s: %w", sstableReader.FilePath(), parseErr)
			}
			fileIDs = append(fileIDs, segmentID)
			obsoleteReaders = append(obsoleteReaders, sstableReader)
		}
	}
	return inputFiles, fileIDs, obsoleteReaders, nil
}

// runAndRegisterCompaction runs the compaction work and registers the result in Level 1.
func (engine *dbEngine) runAndRegisterCompaction(inputFiles []string, fileIDs []int, compactionSegID int, obsoleteReaders []*sstable.Reader) error {
	task := &compactor.Task{
		InputFiles:      inputFiles,
		FileIDs:         fileIDs,
		OutputDirectory: engine.dir,
		NextSegmentID:   compactionSegID,
		IsBottomLevel:   levelOne == numLevels-1,
	}

	var bytesRead int64
	for _, path := range inputFiles {
		if fi, err := os.Stat(path); err == nil {
			bytesRead += fi.Size()
		}
	}

	startTime := time.Now()
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
				filePath := r.FilePath()
				if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
					slog.Warn("failed to clean up compacted file after compaction failure", "path", filePath, "error", err)
				}
			}
			engine.bgErr = fmt.Errorf("compaction failed to open output file %s: %w", compactedPath, err)
			engine.isCompacting = false
			engine.mu.Unlock()
			return err
		}
		newL1Readers = append(newL1Readers, sstableReader)
	}

	engine.mu.Unlock()

	engine.manifestMu.Lock()
	defer engine.manifestMu.Unlock()

	engine.mu.Lock()
	// Build prospective level slices without mutating engine.levels yet.
	// Filter out the compacted files from Level 0 (preserving any concurrent flushes).
	var newL0Readers []*sstable.Reader
	for _, r := range engine.levels[levelZero] {
		isObsolete := slices.Contains(obsoleteReaders, r)
		if !isObsolete {
			newL0Readers = append(newL0Readers, r)
		}
	}

	// Update nextSegmentID to prevent name collisions
	if res.NextSegmentID > engine.nextSegmentID {
		engine.nextSegmentID = res.NextSegmentID
	}

	// Build the prospective manifest from the new level slices (without swapping live state).
	prospectiveLevels := make(map[int][]string)
	l0Names := make([]string, 0, len(newL0Readers))
	for _, r := range newL0Readers {
		l0Names = append(l0Names, filepath.Base(r.FilePath()))
	}
	prospectiveLevels[levelZero] = l0Names
	l1Names := make([]string, 0, len(newL1Readers))
	for _, r := range newL1Readers {
		l1Names = append(l1Names, filepath.Base(r.FilePath()))
	}
	prospectiveLevels[levelOne] = l1Names

	manifest := &Manifest{
		NextSegmentID:    engine.nextSegmentID,
		Levels:           prospectiveLevels,
		FlushedSegmentID: engine.flushedSegmentID,
	}
	engine.mu.Unlock()

	writeErr := writeManifest(engine.dir, manifest)

	engine.mu.Lock()
	if writeErr != nil {
		// Manifest write failed; do NOT swap levels so in-memory state
		// stays consistent with what is persisted on disk. Clean up
		// the new L1 readers that were never registered.
		for _, r := range newL1Readers {
			_ = r.Close()
			filePath := r.FilePath()
			if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
				slog.Warn("failed to clean up compacted file after manifest write failure", "path", filePath, "error", err)
			}
		}
		engine.bgErr = fmt.Errorf("compaction failed to write manifest: %w", writeErr)
		engine.isCompacting = false
		engine.flushCond.Broadcast()
		engine.mu.Unlock()
		return writeErr
	}

	// Manifest is durable — now commit the level swap atomically.
	engine.levels[levelZero] = newL0Readers
	engine.levels[levelOne] = newL1Readers

	// Register the new L1 readers and mark old readers as obsolete.
	engine.sstRefsMu.Lock()
	for _, sstableReader := range newL1Readers {
		engine.sstRefs[sstableReader] = &sstableRef{reader: sstableReader, refs: 0, obsolete: false}
	}
	for _, obsR := range obsoleteReaders {
		if ref, ok := engine.sstRefs[obsR]; ok {
			ref.obsolete = true
			if ref.refs == 0 {
				_ = obsR.Close()
				filePath := obsR.FilePath()
				if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
					slog.Warn("failed to remove obsolete reader file after compaction", "path", filePath, "error", err)
				}
				delete(engine.sstRefs, obsR)
			}
		}
	}
	engine.sstRefsMu.Unlock()

	engine.opts.Metrics.RecordCompaction(time.Since(startTime).Milliseconds(), bytesRead, int64(res.BytesWritten))

	engine.isCompacting = false
	triggerCompaction := len(engine.levels[levelZero]) >= engine.opts.CompactionThreshold
	engine.mu.Unlock()

	if triggerCompaction {
		select {
		case engine.compactChan <- struct{}{}:
		default:
		}
	}
	return nil
}
