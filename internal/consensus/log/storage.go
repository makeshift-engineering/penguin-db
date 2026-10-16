package log

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	consensuspb "github.com/makeshift-engineering/penguin-db/gen/go/consensus/v1"
)

const (
	defaultMaxSegmentSize = 64 * 1024 * 1024
	segmentFileExt        = ".log"
)

var (
	ErrEntryNotFound     = errors.New("requested log entry not found")
	ErrCompactPrecedes   = errors.New("requested entry has been compacted by snapshot")
	ErrInvalidIndexRange = errors.New("invalid entry index range")
)

type RaftLog interface {
	CurrentTerm() uint64
	VotedFor() string
	SetMeta(term uint64, votedFor string) error

	Append(entries []*consensuspb.LogEntry) error
	GetEntry(index uint64) (*consensuspb.LogEntry, error)
	GetEntries(start, end uint64) ([]*consensuspb.LogEntry, error)
	LastIndexAndTerm() (index uint64, term uint64)
	FirstIndex() uint64

	TruncateSuffix(fromIndex uint64) error
	TruncatePrefix(upToIndex uint64) error

	Sync() error
	Close() error
}

type PersistentLog struct {
	mu            sync.RWMutex
	dir           string
	metaStore     *MetaStore
	index         *LogIndex
	segments      map[uint64]*Segment
	activeSegment *Segment
	maxSegSize    int64
}

func OpenPersistentLog(dir string) (*PersistentLog, error) {
	if err := os.MkdirAll(dir, 0o75); err != nil {
		return nil, fmt.Errorf("failed to create log dir %s: %w", dir, err)
	}

	metaStore, err := NewMetaStore(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to open metadata store: %w", err)
	}

	pl := &PersistentLog{
		dir:        dir,
		metaStore:  metaStore,
		segments:   make(map[uint64]*Segment),
		index:      NewLogIndex(1),
		maxSegSize: defaultMaxSegmentSize,
	}

	if err := pl.recover(); err != nil {
		return nil, fmt.Errorf("log recovery failed: %w", err)
	}

	return pl, nil
}

func (pl *PersistentLog) recover() error {
	entries, err := os.ReadDir(pl.dir)
	if err != nil {
		return err
	}

	var baseIndices []uint64
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), segmentFileExt) {
			continue
		}
		baseStr := strings.TrimSuffix(entry.Name(), segmentFileExt)
		baseIdx, parseErr := strconv.ParseUint(baseStr, 10, 64)
		if parseErr != nil {
			continue
		}
		baseIndices = append(baseIndices, baseIdx)
	}

	sort.Slice(baseIndices, func(i, j int) bool {
		return baseIndices[i] < baseIndices[j]
	})

	if len(baseIndices) == 0 {
		segPath := filepath.Join(pl.dir, fmt.Sprintf("%020d%s", 1, segmentFileExt))
		seg, err := OpenSegment(segPath, 1)
		if err != nil {
			return err
		}
		pl.segments[1] = seg
		pl.activeSegment = seg
		return nil
	}

	for _, base := range baseIndices {
		segPath := filepath.Join(pl.dir, fmt.Sprintf("%020d%s", base, segmentFileExt))
		seg, err := OpenSegment(segPath, base)
		if err != nil {
			return err
		}
		pl.segments[base] = seg

		err = seg.Scan(func(e *consensuspb.LogEntry, offset, length int64) error {
			pl.index.Append(e.Index, e.Term, base, offset, length)
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed scanning segment %d: %w", base, err)
		}
	}

	lastBase := baseIndices[len(baseIndices)-1]
	pl.activeSegment = pl.segments[lastBase]
	return nil
}

func (pl *PersistentLog) CurrentTerm() uint64 {
	term, _ := pl.metaStore.Get()
	return term
}

func (pl *PersistentLog) VotedFor() string {
	_, votedFor := pl.metaStore.Get()
	return votedFor
}

func (pl *PersistentLog) SetMeta(term uint64, votedFor string) error {
	return pl.metaStore.Set(term, votedFor)
}

func (pl *PersistentLog) Append(entries []*consensuspb.LogEntry) error {
	pl.mu.Lock()
	defer pl.mu.Unlock()

	for _, entry := range entries {
		if pl.activeSegment.activeSize >= pl.maxSegSize {
			if err := pl.rollSegment(entry.Index); err != nil {
				return err
			}
		}

		offset, length, err := pl.activeSegment.Append(entry)
		if err != nil {
			return err
		}

		pl.index.Append(entry.Index, entry.Term, pl.activeSegment.baseIndex, offset, length)
	}

	return pl.activeSegment.Sync()
}

func (pl *PersistentLog) rollSegment(newBase uint64) error {
	if err := pl.activeSegment.Sync(); err != nil {
		return err
	}

	newPath := filepath.Join(pl.dir, fmt.Sprintf("%020d%s", newBase, segmentFileExt))
	seg, err := OpenSegment(newPath, newBase)
	if err != nil {
		return fmt.Errorf("failed to create new segment %s: %w", newPath, err)
	}

	pl.segments[newBase] = seg
	pl.activeSegment = seg
	return nil
}

func (pl *PersistentLog) GetEntry(index uint64) (*consensuspb.LogEntry, error) {
	pl.mu.RLock()
	defer pl.mu.RUnlock()

	loc, err := pl.index.LookUp(index)
	if err != nil {
		if errors.Is(err, ErrIndexTooSmall) {
			return nil, ErrCompactPrecedes
		}
		return nil, ErrEntryNotFound
	}

	seg, ok := pl.segments[loc.segmentBase]
	if !ok {
		return nil, fmt.Errorf("segment %d missing from memory map", loc.segmentBase)
	}

	return seg.ReadAt(loc.offset)
}

func (pl *PersistentLog) GetEntries(start, end uint64) ([]*consensuspb.LogEntry, error) {
	if start >= end {
		return nil, nil
	}

	pl.mu.RLock()
	defer pl.mu.RUnlock()

	entries := make([]*consensuspb.LogEntry, 0, end-start)
	for i := start; i < end; i++ {
		loc, err := pl.index.LookUp(i)
		if err != nil {
			if errors.Is(err, ErrIndexTooSmall) {
				return nil, ErrCompactPrecedes
			}
			return nil, ErrEntryNotFound
		}

		seg, ok := pl.segments[loc.segmentBase]
		if !ok {
			return nil, fmt.Errorf("segment %d missing from memory map", loc.segmentBase)
		}

		entry, err := seg.ReadAt(loc.offset)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

func (pl *PersistentLog) LastIndexAndTerm() (uint64, uint64) {
	return pl.index.LastIndexAndTerm()
}

func (pl *PersistentLog) FirstIndex() uint64 {
	return pl.index.FirstIndex()
}

func (pl *PersistentLog) TruncateSuffix(fromIndex uint64) error {
	pl.mu.Lock()
	defer pl.mu.Unlock()

	loc, err := pl.index.LookUp(fromIndex)
	if err != nil {
		if errors.Is(err, ErrIndexNotFound) {
			return nil
		}
		return err
	}

	pl.index.TruncateSuffix(fromIndex)

	seg, ok := pl.segments[loc.segmentBase]
	if ok {
		if err := seg.truncateTo(loc.offset); err != nil {
			return err
		}
	}

	for base, s := range pl.segments {
		if base > loc.segmentBase {
			_ = s.Close()
			_ = os.Remove(s.path)
			delete(pl.segments, base)
		}
	}

	pl.activeSegment = seg
	return nil
}

func (pl *PersistentLog) TruncatePrefix(uptoIndex uint64) error {
	pl.mu.Lock()
	defer pl.mu.Unlock()

	pl.index.TruncatePrefix(uptoIndex)

	for base, seg := range pl.segments {
		if seg == pl.activeSegment {
			continue
		}

		if base <= uptoIndex {
			_ = seg.Close()
			_ = os.Remove(seg.path)
			delete(pl.segments, base)
		}
	}
	return nil
}

func (pl *PersistentLog) Sync() error {
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	return pl.activeSegment.Sync()
}

func (pl *PersistentLog) Close() error {
	pl.mu.Lock()
	defer pl.mu.Unlock()

	for _, seg := range pl.segments {
		_ = seg.Close()
	}
	return nil
}
