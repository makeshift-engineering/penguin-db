package statemachine

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"

	consensuspb "github.com/makeshift-engineering/penguin-db/gen/go/consensus/v1"
	storagepb "github.com/makeshift-engineering/penguin-db/gen/go/storage/v1"
	"github.com/makeshift-engineering/penguin-db/internal/storage"
	"google.golang.org/protobuf/proto"
)

var (
	ErrNilLogEntry          = errors.New("cannot apply nil log entry")
	ErrUnsupportedEntryType = errors.New("unsupported consensus entry type")
	ErrCorruptSnapshotData  = errors.New("corrupted snapshot stream payload")
)

type LSMStateMachine struct {
	mu     sync.RWMutex
	engine storage.Engine
	closed bool
}

func NewLSMStateMachine(engine storage.Engine) *LSMStateMachine {
	return &LSMStateMachine{
		engine: engine,
	}
}

func (f *LSMStateMachine) Apply(entry *consensuspb.LogEntry) (any, error) {
	if entry == nil {
		return nil, ErrNilLogEntry
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return nil, storage.ErrEngineClosed
	}

	switch entry.Type {
	case consensuspb.EntryType_ENTRY_TYPE_NOOP:
		return nil, nil
	case consensuspb.EntryType_ENTRY_TYPE_CONFIG_CHANGE:
		return nil, nil
	case consensuspb.EntryType_ENTRY_TYPE_NORMAL:
		return f.applyWriteBatch(entry.Data)
	default:
		return nil, fmt.Errorf("%w: %v", ErrUnsupportedEntryType, entry.Type)
	}
}

func (f *LSMStateMachine) applyWriteBatch(data []byte) (any, error) {
	var batch storagepb.WriteBatchRequest
	if err := proto.Unmarshal(data, &batch); err != nil {
		return nil, fmt.Errorf("failed to unmarshal WriteBatchRequest: %w", err)
	}

	if len(batch.Operations) == 0 {
		return nil, nil
	}

	engineOps := make([]storage.Op, 0, len(batch.Operations))
	for _, op := range batch.Operations {
		var opType storage.OpType
		switch op.Type {
		case storagepb.OpType_OP_TYPE_PUT:
			opType = storage.OpPut
		case storagepb.OpType_OP_TYPE_DELETE:
			opType = storage.OpDelete
		default:
			return nil, fmt.Errorf("unsupported batch operation type: %v", op.Type)
		}

		engineOps = append(engineOps, storage.Op{
			Type:  opType,
			Key:   op.Key,
			Value: op.Value,
		})
	}

	if err := f.engine.WriteBatch(context.Background(), engineOps); err != nil {
		return nil, fmt.Errorf("engine WriteBatch failed: %w", err)
	}

	return nil, nil
}

type lsmSnapshotReader struct {
	reader io.ReadCloser
	size   int64
}

func (r *lsmSnapshotReader) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

func (r *lsmSnapshotReader) Close() error {
	return r.reader.Close()
}
func (r *lsmSnapshotReader) Size() int64 {
	return r.size
}

func (f *LSMStateMachine) Snapshot() (SnapshotReader, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if f.closed {
		return nil, storage.ErrEngineClosed
	}

	snap, err := f.engine.Snapshot()
	if err != nil {
		return nil, fmt.Errorf("failed to create engine snapshot: %w", err)
	}

	iter, err := snap.Scan(nil)
	if err != nil {
		snap.Close()
		return nil, fmt.Errorf("failed to open snapshot iterator: %w", err)
	}

	var buf bytes.Buffer
	for iter.Valid() {
		key, value := iter.Next()
		keyLen := uint32(len(key))
		valueLen := uint32(len(value))

		var lenBuf [8]byte
		binary.LittleEndian.PutUint32(lenBuf[0:4], keyLen)
		binary.LittleEndian.PutUint32(lenBuf[4:8], valueLen)
		buf.Write(lenBuf[:])
		buf.Write(key)
		buf.Write(value)
	}
	iter.Close()
	snap.Close()

	snapshotBytes := buf.Bytes()
	return &lsmSnapshotReader{
		reader: io.NopCloser(bytes.NewReader(snapshotBytes)),
		size:   int64(len(snapshotBytes)),
	}, nil
}

func (f *LSMStateMachine) Restore(reader io.Reader) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return storage.ErrEngineClosed
	}

	const restoreBatchSize = 1000
	batchOps := make([]storage.Op, 0, restoreBatchSize)

	var lenBuf [8]byte
	for {
		_, err := io.ReadFull(reader, lenBuf[:])
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("%w: failed to read entry lengths: %v", ErrCorruptSnapshotData, err)
		}

		keyLen := binary.LittleEndian.Uint32(lenBuf[0:4])
		valueLen := binary.LittleEndian.Uint32(lenBuf[4:8])

		key := make([]byte, keyLen)
		if keyLen > 0 {
			if _, err := io.ReadFull(reader, key); err != nil {
				return fmt.Errorf("%w: truncated key data: %v", ErrCorruptSnapshotData, err)
			}
		}

		value := make([]byte, valueLen)
		if valueLen > 0 {
			if _, err := io.ReadFull(reader, value); err != nil {
				return fmt.Errorf("%w: truncated value data: %v", ErrCorruptSnapshotData, err)
			}
		}

		batchOps = append(batchOps, storage.Op{
			Type:  storage.OpPut,
			Key:   key,
			Value: value,
		})

		if len(batchOps) >= restoreBatchSize {
			if err := f.engine.WriteBatch(context.Background(), batchOps); err != nil {
				return fmt.Errorf("failed applying restore batch: %w", err)
			}
			batchOps = batchOps[:0]
		}
	}

	if len(batchOps) > 0 {
		if err := f.engine.WriteBatch(context.Background(), batchOps); err != nil {
			return fmt.Errorf("failed applying final restore batch: %w", err)
		}
	}

	return nil
}

func (f *LSMStateMachine) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}
