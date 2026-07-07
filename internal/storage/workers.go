package storage

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/makeshift-engineering/penguin-db/internal/storage/compactor"
	"github.com/makeshift-engineering/penguin-db/internal/storage/sstable"
)

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
		if len(engine.immMemtables) == 0 {
			if engine.isClosing {
				engine.mu.Unlock()
				return
			}
			engine.mu.Unlock()
			continue
		}
		immutable := engine.immMemtables[0]
		segmentID := engine.immWALSegmentIDs[0]
		engine.mu.Unlock()

		sstableFilename := fmt.Sprintf("%06d.sst", segmentID)
		sstablePath := filepath.Join(engine.dir, sstableFilename)

		startTime := time.Now()
		sstableReader, flushErr := writeMemTableToSSTable(sstablePath, immutable)

		engine.mu.Lock()
		if flushErr != nil {
			_ = os.Remove(sstablePath)
			engine.bgErr = flushErr
			engine.immMemtables = engine.immMemtables[1:]
			engine.immWALSegmentIDs = engine.immWALSegmentIDs[1:]
			engine.flushCond.Broadcast()
			engine.mu.Unlock()
			return
		}

		// Prepend the new file to Level 0 (newest first).
		engine.levels[levelZero] = append([]*sstable.Reader{sstableReader}, engine.levels[levelZero]...)
		engine.sstRefsMu.Lock()
		engine.sstRefs[sstableReader] = &sstableRef{reader: sstableReader, refs: 0, obsolete: false}
		engine.sstRefsMu.Unlock()

		engine.immMemtables = engine.immMemtables[1:]
		engine.immWALSegmentIDs = engine.immWALSegmentIDs[1:]
		engine.flushCond.Broadcast()

		manifest := &Manifest{
			NextSegmentID:    engine.nextSegmentID,
			Levels:           engine.manifestLevels(),
			FlushedSegmentID: segmentID,
		}
		engine.mu.Unlock()

		writeErr := engine.writeManifestDurable(manifest)

		engine.mu.Lock()
		if writeErr != nil {
			engine.bgErr = writeErr
			engine.flushCond.Broadcast()
			engine.mu.Unlock()
			return
		}

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
	capacity := len(engine.levels[levelZero]) + len(engine.levels[levelOne])
	inputFiles = make([]string, 0, capacity)
	fileIDs = make([]int, 0, capacity)
	obsoleteReaders = make([]*sstable.Reader, 0, capacity)

	for _, sstableReader := range engine.levels[levelZero] {
		inputFiles = append(inputFiles, sstableReader.FilePath())
		var segmentID int
		_, _ = fmt.Sscanf(filepath.Base(sstableReader.FilePath()), "%d.sst", &segmentID)
		fileIDs = append(fileIDs, segmentID)
		obsoleteReaders = append(obsoleteReaders, sstableReader)
	}
	for _, sstableReader := range engine.levels[levelOne] {
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
				_ = os.Remove(r.FilePath())
			}
			engine.bgErr = fmt.Errorf("compaction failed to open output file %s: %w", compactedPath, err)
			engine.isCompacting = false
			engine.mu.Unlock()
			return err
		}
		newL1Readers = append(newL1Readers, sstableReader)
	}

	// Apply levels update in memory before releasing the lock.
	// Filter out the compacted files from Level 0 (preserving any concurrent flushes).
	var newL0 []*sstable.Reader
	for _, r := range engine.levels[levelZero] {
		isObsolete := false
		for _, obs := range obsoleteReaders {
			if r == obs {
				isObsolete = true
				break
			}
		}
		if !isObsolete {
			newL0 = append(newL0, r)
		}
	}
	engine.levels[levelZero] = newL0
	engine.levels[levelOne] = newL1Readers

	// Register the new L1 readers.
	engine.sstRefsMu.Lock()
	for _, sstableReader := range newL1Readers {
		engine.sstRefs[sstableReader] = &sstableRef{reader: sstableReader, refs: 0, obsolete: false}
	}
	engine.sstRefsMu.Unlock()

	// Update nextSegmentID to prevent name collisions
	if res.NextSegmentID > engine.nextSegmentID {
		engine.nextSegmentID = res.NextSegmentID
	}

	manifest := &Manifest{
		NextSegmentID:    engine.nextSegmentID,
		Levels:           engine.manifestLevels(),
		FlushedSegmentID: task.NextSegmentID, // Update to the last compacted segment ID
	}
	engine.mu.Unlock()

	writeErr := engine.writeManifestDurable(manifest)

	engine.mu.Lock()
	if writeErr != nil {
		engine.bgErr = fmt.Errorf("compaction failed to write manifest: %w", writeErr)
		engine.isCompacting = false
		engine.flushCond.Broadcast()
		engine.mu.Unlock()
		return writeErr
	}

	// Mark old readers as obsolete and remove them if unreferenced.
	engine.sstRefsMu.Lock()
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
