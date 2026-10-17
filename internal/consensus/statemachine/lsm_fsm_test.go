package statemachine

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"

	consensuspb "github.com/makeshift-engineering/penguin-db/gen/go/consensus/v1"
	storagepb "github.com/makeshift-engineering/penguin-db/gen/go/storage/v1"
	"github.com/makeshift-engineering/penguin-db/internal/storage"
)

// openTestEngine creates an isolated LSM storage engine in a temporary directory.
func openTestEngine(t *testing.T) storage.Engine {
	t.Helper()
	dir := t.TempDir()
	opts := storage.DefaultOptions()
	engine, err := storage.NewEngine(dir, opts)
	if err != nil {
		t.Fatalf("failed to initialize test storage engine: %v", err)
	}
	t.Cleanup(func() {
		_ = engine.Close()
	})
	return engine
}

// TestLSMStateMachine_ApplyNormalWrites verifies that committed normal log entries
// containing protobuf WriteBatchRequests are correctly applied to the LSM storage engine.
func TestLSMStateMachine_ApplyNormalWrites(t *testing.T) {
	engine := openTestEngine(t)
	fsm := NewLSMStateMachine(engine)
	defer fsm.Close()

	batchReq := &storagepb.WriteBatchRequest{
		Operations: []*storagepb.Op{
			{
				Type:  storagepb.OpType_OP_TYPE_PUT,
				Key:   []byte("alpha"),
				Value: []byte("first-value"),
			},
			{
				Type:  storagepb.OpType_OP_TYPE_PUT,
				Key:   []byte("bravo"),
				Value: []byte("second-value"),
			},
		},
	}
	data, err := proto.Marshal(batchReq)
	if err != nil {
		t.Fatalf("failed marshaling WriteBatchRequest: %v", err)
	}

	entry := &consensuspb.LogEntry{
		Index: 1,
		Term:  1,
		Type:  consensuspb.EntryType_ENTRY_TYPE_NORMAL,
		Data:  data,
	}

	res, err := fsm.Apply(entry)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if res != nil {
		t.Errorf("expected nil result, got %v", res)
	}

	valAlpha, err := engine.Get([]byte("alpha"))
	if err != nil {
		t.Fatalf("failed retrieving alpha: %v", err)
	}
	if !bytes.Equal(valAlpha, []byte("first-value")) {
		t.Errorf("alpha value mismatch: got %q, want %q", valAlpha, "first-value")
	}

	valBravo, err := engine.Get([]byte("bravo"))
	if err != nil {
		t.Fatalf("failed retrieving bravo: %v", err)
	}
	if !bytes.Equal(valBravo, []byte("second-value")) {
		t.Errorf("bravo value mismatch: got %q, want %q", valBravo, "second-value")
	}
}

// TestLSMStateMachine_ApplyDeletes verifies that tombstone delete operations
// within committed batches remove previously persisted keys.
func TestLSMStateMachine_ApplyDeletes(t *testing.T) {
	engine := openTestEngine(t)
	fsm := NewLSMStateMachine(engine)
	defer fsm.Close()

	initialBatch := &storagepb.WriteBatchRequest{
		Operations: []*storagepb.Op{
			{
				Type:  storagepb.OpType_OP_TYPE_PUT,
				Key:   []byte("session-1"),
				Value: []byte("active"),
			},
			{
				Type:  storagepb.OpType_OP_TYPE_PUT,
				Key:   []byte("session-2"),
				Value: []byte("active"),
			},
		},
	}
	initialData, _ := proto.Marshal(initialBatch)
	_, err := fsm.Apply(&consensuspb.LogEntry{
		Index: 1,
		Term:  1,
		Type:  consensuspb.EntryType_ENTRY_TYPE_NORMAL,
		Data:  initialData,
	})
	if err != nil {
		t.Fatalf("initial Apply failed: %v", err)
	}

	deleteBatch := &storagepb.WriteBatchRequest{
		Operations: []*storagepb.Op{
			{
				Type: storagepb.OpType_OP_TYPE_DELETE,
				Key:  []byte("session-1"),
			},
		},
	}
	deleteData, _ := proto.Marshal(deleteBatch)
	_, err = fsm.Apply(&consensuspb.LogEntry{
		Index: 2,
		Term:  1,
		Type:  consensuspb.EntryType_ENTRY_TYPE_NORMAL,
		Data:  deleteData,
	})
	if err != nil {
		t.Fatalf("delete Apply failed: %v", err)
	}

	_, err = engine.Get([]byte("session-1"))
	if !errors.Is(err, storage.ErrKeyNotFound) {
		t.Fatalf("expected ErrKeyNotFound for deleted session-1, got %v", err)
	}

	val2, err := engine.Get([]byte("session-2"))
	if err != nil || !bytes.Equal(val2, []byte("active")) {
		t.Fatalf("session-2 should remain intact, got (%s, %v)", val2, err)
	}
}

// TestLSMStateMachine_ApplyNoOpAndConfigChange verifies that protocol-level
// No-Op entries and configuration change entries succeed as clean bypasses without errors.
func TestLSMStateMachine_ApplyNoOpAndConfigChange(t *testing.T) {
	engine := openTestEngine(t)
	fsm := NewLSMStateMachine(engine)
	defer fsm.Close()

	noOpEntry := &consensuspb.LogEntry{
		Index: 1,
		Term:  2,
		Type:  consensuspb.EntryType_ENTRY_TYPE_NOOP,
		Data:  nil,
	}
	if _, err := fsm.Apply(noOpEntry); err != nil {
		t.Fatalf("Apply No-Op failed: %v", err)
	}

	configEntry := &consensuspb.LogEntry{
		Index: 2,
		Term:  2,
		Type:  consensuspb.EntryType_ENTRY_TYPE_CONFIG_CHANGE,
		Data:  []byte("cluster-membership-change-payload"),
	}
	if _, err := fsm.Apply(configEntry); err != nil {
		t.Fatalf("Apply Config Change failed: %v", err)
	}
}

// TestLSMStateMachine_ApplyValidationErrors ensures that invalid inputs, such as
// nil log entries, unsupported entry types, or corrupted protobuf payloads, are rejected.
func TestLSMStateMachine_ApplyValidationErrors(t *testing.T) {
	engine := openTestEngine(t)
	fsm := NewLSMStateMachine(engine)
	defer fsm.Close()

	if _, err := fsm.Apply(nil); !errors.Is(err, ErrNilLogEntry) {
		t.Fatalf("expected ErrNilLogEntry, got %v", err)
	}

	invalidTypeEntry := &consensuspb.LogEntry{
		Index: 1,
		Term:  1,
		Type:  consensuspb.EntryType(999),
		Data:  nil,
	}
	if _, err := fsm.Apply(invalidTypeEntry); !errors.Is(err, ErrUnsupportedEntryType) {
		t.Fatalf("expected ErrUnsupportedEntryType, got %v", err)
	}

	corruptDataEntry := &consensuspb.LogEntry{
		Index: 2,
		Term:  1,
		Type:  consensuspb.EntryType_ENTRY_TYPE_NORMAL,
		Data:  []byte("non-protobuf-garbage-bytes"),
	}
	if _, err := fsm.Apply(corruptDataEntry); err == nil {
		t.Fatal("expected error for corrupted protobuf payload, got nil")
	}
}

// TestLSMStateMachine_SnapshotAndRestore exercises point-in-time snapshotting
// and restores state into an independent target engine, verifying full data fidelity.
func TestLSMStateMachine_SnapshotAndRestore(t *testing.T) {
	sourceEngine := openTestEngine(t)
	sourceFSM := NewLSMStateMachine(sourceEngine)
	defer sourceFSM.Close()

	const recordCount = 40
	ops := make([]*storagepb.Op, recordCount)
	for i := range ops {
		ops[i] = &storagepb.Op{
			Type:  storagepb.OpType_OP_TYPE_PUT,
			Key:   fmt.Appendf(nil, "user-%04d", i),
			Value: fmt.Appendf(nil, "payload-data-%04d", i),
		}
	}

	batchData, err := proto.Marshal(&storagepb.WriteBatchRequest{Operations: ops})
	if err != nil {
		t.Fatalf("failed marshaling batch: %v", err)
	}

	_, err = sourceFSM.Apply(&consensuspb.LogEntry{
		Index: 1,
		Term:  1,
		Type:  consensuspb.EntryType_ENTRY_TYPE_NORMAL,
		Data:  batchData,
	})
	if err != nil {
		t.Fatalf("source Apply failed: %v", err)
	}

	snapReader, err := sourceFSM.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot creation failed: %v", err)
	}
	defer snapReader.Close()

	if snapReader.Size() <= 0 {
		t.Fatalf("expected positive snapshot size, got %d", snapReader.Size())
	}

	targetEngine := openTestEngine(t)
	targetFSM := NewLSMStateMachine(targetEngine)
	defer targetFSM.Close()

	_, err = targetEngine.Get([]byte("user-0000"))
	if !errors.Is(err, storage.ErrKeyNotFound) {
		t.Fatalf("target engine should be empty before restore, got err: %v", err)
	}

	if err := targetFSM.Restore(snapReader); err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	for i := 0; i < recordCount; i++ {
		key := fmt.Appendf(nil, "user-%04d", i)
		expectedVal := fmt.Appendf(nil, "payload-data-%04d", i)

		val, err := targetEngine.Get(key)
		if err != nil {
			t.Fatalf("target engine missing key %s after restore: %v", key, err)
		}
		if !bytes.Equal(val, expectedVal) {
			t.Errorf("value mismatch for key %s: got %q, want %q", key, val, expectedVal)
		}
	}
}

// TestLSMStateMachine_RestoreCorruptData verifies that truncated or malformed
// snapshot payloads return ErrCorruptSnapshotData during restoration.
func TestLSMStateMachine_RestoreCorruptData(t *testing.T) {
	engine := openTestEngine(t)
	fsm := NewLSMStateMachine(engine)
	defer fsm.Close()

	truncatedHeader := bytes.NewReader([]byte{0x01, 0x02, 0x03})
	if err := fsm.Restore(truncatedHeader); !errors.Is(err, ErrCorruptSnapshotData) {
		t.Fatalf("expected ErrCorruptSnapshotData for truncated header, got %v", err)
	}

	truncatedPayload := bytes.NewReader([]byte{
		0x05, 0x00, 0x00, 0x00, // keyLen = 5
		0x0A, 0x00, 0x00, 0x00, // valLen = 10
		'a', 'b', 'c', // only 3 bytes of key provided
	})
	if err := fsm.Restore(truncatedPayload); !errors.Is(err, ErrCorruptSnapshotData) {
		t.Fatalf("expected ErrCorruptSnapshotData for truncated payload, got %v", err)
	}
}

// TestLSMStateMachine_CloseEnsuresNoFurtherOperations verifies that all methods
// return ErrEngineClosed once the state machine adapter has been closed.
func TestLSMStateMachine_CloseEnsuresNoFurtherOperations(t *testing.T) {
	engine := openTestEngine(t)
	fsm := NewLSMStateMachine(engine)

	if err := fsm.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	entry := &consensuspb.LogEntry{
		Index: 1,
		Term:  1,
		Type:  consensuspb.EntryType_ENTRY_TYPE_NOOP,
	}
	if _, err := fsm.Apply(entry); !errors.Is(err, storage.ErrEngineClosed) {
		t.Errorf("expected ErrEngineClosed on Apply, got %v", err)
	}

	if _, err := fsm.Snapshot(); !errors.Is(err, storage.ErrEngineClosed) {
		t.Errorf("expected ErrEngineClosed on Snapshot, got %v", err)
	}

	if err := fsm.Restore(bytes.NewReader(nil)); !errors.Is(err, storage.ErrEngineClosed) {
		t.Errorf("expected ErrEngineClosed on Restore, got %v", err)
	}
}
