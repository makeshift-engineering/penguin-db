package storage_server

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	storagepb "github.com/makeshift-engineering/penguin-db/gen/go/storage/v1"
	"github.com/makeshift-engineering/penguin-db/internal/storage"
)

const (
	defaultSnapshotExpiry        = 10 * time.Minute
	defaultSnapshotCheckInterval = 1 * time.Minute
	// maxScanLimit caps the number of keys returned in a single Scan call to prevent
	// unbounded memory and network usage. Clients requesting 0 or a value above this
	// threshold are silently clamped to this ceiling.
	maxScanLimit = 10000
)

type snapshotEntry struct {
	snap      storage.Snapshot
	createdAt time.Time
	mu        sync.Mutex
	refCount  int
	released  bool
}

// StorageServer implements the storagepb.StorageServiceServer gRPC interface,
// bridging incoming network requests to the underlying LSM-tree storage engine.
// It also manages point-in-time snapshots to provide read isolation.
type StorageServer struct {
	storagepb.UnimplementedStorageServiceServer
	engine storage.Engine

	mu        sync.RWMutex
	snapshots map[string]*snapshotEntry
	stopChan  chan struct{}
}

// NewStorageServer instantiates a new StorageServer backed by the given storage engine.
func NewStorageServer(engine storage.Engine) *StorageServer {
	s := &StorageServer{
		engine:    engine,
		snapshots: make(map[string]*snapshotEntry),
		stopChan:  make(chan struct{}),
	}
	go s.startSnapshotJanitor(defaultSnapshotCheckInterval, defaultSnapshotExpiry)
	return s
}

// Register binds this StorageServer implementation to the provided gRPC Server.
func (s *StorageServer) Register(server *grpc.Server) {
	storagepb.RegisterStorageServiceServer(server, s)
}

// acquireSnapshot increments the refcount of the snapshot if it exists and is not released.
func (s *StorageServer) acquireSnapshot(id string) (*snapshotEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.snapshots[id]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "snapshot %s not found", id)
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.released {
		return nil, status.Errorf(codes.FailedPrecondition, "snapshot %s is closed", id)
	}
	entry.refCount++
	return entry, nil
}

// releaseSnapshotRef decrements the refcount and closes the snapshot if released and refcount reaches 0.
func (s *StorageServer) releaseSnapshotRef(entry *snapshotEntry) {
	entry.mu.Lock()
	entry.refCount--
	shouldClose := entry.released && entry.refCount == 0
	entry.mu.Unlock()

	if shouldClose {
		entry.snap.Close()
	}
}

// Stop signals the server is shutting down, closes stopChan to abort active scans,
// and releases all snapshots.
func (s *StorageServer) Stop() {
	s.mu.Lock()
	select {
	case <-s.stopChan:
		s.mu.Unlock()
		return
	default:
		close(s.stopChan)
	}
	s.mu.Unlock()
	s.ReleaseAllSnapshots()
}

// ReleaseAllSnapshots iterates over all registered snapshots, marks them as released,
// and closes them if there are no active readers.
func (s *StorageServer) ReleaseAllSnapshots() {
	s.mu.Lock()
	entries := make([]*snapshotEntry, 0, len(s.snapshots))
	for id, entry := range s.snapshots {
		entries = append(entries, entry)
		delete(s.snapshots, id)
	}
	s.mu.Unlock()

	for _, entry := range entries {
		entry.mu.Lock()
		entry.released = true
		shouldClose := entry.refCount == 0
		entry.mu.Unlock()

		if shouldClose {
			entry.snap.Close()
		}
	}
}

// startSnapshotJanitor periodically removes expired snapshots to prevent unbounded resource pinning.
func (s *StorageServer) startSnapshotJanitor(interval, expiry time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.mu.Lock()
			now := time.Now()
			var expired []*snapshotEntry
			for id, entry := range s.snapshots {
				if now.Sub(entry.createdAt) > expiry {
					expired = append(expired, entry)
					delete(s.snapshots, id)
					slog.Warn("Evicted expired snapshot due to inactivity", "snapshot_id", id, "age", now.Sub(entry.createdAt))
				}
			}
			s.mu.Unlock()

			for _, entry := range expired {
				entry.mu.Lock()
				entry.released = true
				shouldClose := entry.refCount == 0
				entry.mu.Unlock()

				if shouldClose {
					entry.snap.Close()
				}
			}
		}
	}
}

// mapError translates engine-specific error sentinel types into standard gRPC status codes.
// It sanitizes internal storage errors to prevent leaking internal detail traces to clients.
func mapError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "request was canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "deadline exceeded")
	case errors.Is(err, storage.ErrKeyNotFound):
		return status.Error(codes.NotFound, "key not found")
	case errors.Is(err, storage.ErrEmptyKey):
		return status.Error(codes.InvalidArgument, "key cannot be empty")
	case errors.Is(err, storage.ErrEngineClosed):
		return status.Error(codes.FailedPrecondition, "storage engine is closed")
	case errors.Is(err, storage.ErrSnapshotClosed):
		return status.Error(codes.FailedPrecondition, "snapshot is closed")
	default:
		slog.Error("Storage internal error", "error", err)
		return status.Error(codes.Internal, "internal server error")
	}
}

// Put writes a single key-value record to the underlying database engine.
func (s *StorageServer) Put(ctx context.Context, req *storagepb.PutRequest) (*storagepb.PutResponse, error) {
	if len(req.Key) == 0 {
		return nil, status.Error(codes.InvalidArgument, "key cannot be empty")
	}
	if err := s.engine.Put(ctx, req.Key, req.Value); err != nil {
		return nil, mapError(err)
	}
	return &storagepb.PutResponse{}, nil
}

// Get retrieves a key-value record from either the active storage engine or the specified snapshot.
func (s *StorageServer) Get(ctx context.Context, req *storagepb.GetRequest) (*storagepb.GetResponse, error) {
	if len(req.Key) == 0 {
		return nil, status.Error(codes.InvalidArgument, "key cannot be empty")
	}
	var val []byte
	var err error
	if req.SnapshotId != "" {
		var entry *snapshotEntry
		entry, err = s.acquireSnapshot(req.SnapshotId)
		if err != nil {
			return nil, err
		}
		defer s.releaseSnapshotRef(entry)
		val, err = entry.snap.Get(req.Key)
	} else {
		val, err = s.engine.Get(req.Key)
	}
	if err != nil {
		return nil, mapError(err)
	}
	return &storagepb.GetResponse{Value: val}, nil
}

// Delete logically deletes a key by appending a tombstone record in the engine.
func (s *StorageServer) Delete(ctx context.Context, req *storagepb.DeleteRequest) (*storagepb.DeleteResponse, error) {
	if len(req.Key) == 0 {
		return nil, status.Error(codes.InvalidArgument, "key cannot be empty")
	}
	if err := s.engine.Delete(ctx, req.Key); err != nil {
		return nil, mapError(err)
	}
	return &storagepb.DeleteResponse{}, nil
}

// Scan returns a prefix-filtered, cursor-paginated stream of sorted key-value pairs from
// the engine or a named snapshot. Pagination is controlled by the limit and cursor_key fields
// of the request: limit caps the number of results streamed (0 defaults to maxScanLimit);
// cursor_key, when set, skips all keys up to and including that value (exclusive start).
func (s *StorageServer) Scan(req *storagepb.ScanRequest, stream storagepb.StorageService_ScanServer) error {
	var iter storage.Iterator
	var err error
	if req.SnapshotId != "" {
		var entry *snapshotEntry
		entry, err = s.acquireSnapshot(req.SnapshotId)
		if err != nil {
			return err
		}
		defer s.releaseSnapshotRef(entry)
		iter, err = entry.snap.Scan(req.Prefix)
	} else {
		iter, err = s.engine.Scan(req.Prefix)
	}
	if err != nil {
		return mapError(err)
	}
	defer iter.Close()

	limit := req.Limit
	if limit == 0 || limit > maxScanLimit {
		limit = maxScanLimit
	}
	var sent uint32

	for iter.Valid() {
		select {
		case <-s.stopChan:
			return status.Error(codes.Aborted, "server is shutting down")
		case <-stream.Context().Done():
			return stream.Context().Err()
		default:
		}
		key, val := iter.Next()
		if len(req.CursorKey) > 0 && bytes.Compare(key, req.CursorKey) <= 0 {
			continue
		}
		if err := stream.Send(&storagepb.ScanResponse{Key: key, Value: val}); err != nil {
			return err
		}
		sent++
		if sent >= limit {
			break
		}
	}
	return nil
}

// WriteBatch applies multiple Put/Delete operations atomically to the database.
func (s *StorageServer) WriteBatch(ctx context.Context, req *storagepb.WriteBatchRequest) (*storagepb.WriteBatchResponse, error) {
	if len(req.Operations) == 0 {
		return &storagepb.WriteBatchResponse{}, nil
	}
	ops := make([]storage.Op, len(req.Operations))
	for i, op := range req.Operations {
		var opType storage.OpType
		switch op.Type {
		case storagepb.OpType_OP_TYPE_PUT:
			opType = storage.OpPut
		case storagepb.OpType_OP_TYPE_DELETE:
			opType = storage.OpDelete
		default:
			return nil, status.Errorf(codes.InvalidArgument, "unsupported operation type: %v", op.Type)
		}
		ops[i] = storage.Op{
			Type:  opType,
			Key:   op.Key,
			Value: op.Value,
		}
	}
	if err := s.engine.WriteBatch(ctx, ops); err != nil {
		return nil, mapError(err)
	}
	return &storagepb.WriteBatchResponse{}, nil
}

// CreateSnapshot registers a new point-in-time snapshot and returns its generated UUID.
func (s *StorageServer) CreateSnapshot(ctx context.Context, req *storagepb.CreateSnapshotRequest) (*storagepb.CreateSnapshotResponse, error) {
	snap, err := s.engine.Snapshot()
	if err != nil {
		return nil, mapError(err)
	}
	id := uuid.New().String()
	s.mu.Lock()
	select {
	case <-s.stopChan:
		s.mu.Unlock()
		snap.Close()
		return nil, status.Error(codes.Aborted, "server is shutting down")
	default:
	}
	s.snapshots[id] = &snapshotEntry{
		snap:      snap,
		createdAt: time.Now(),
	}
	s.mu.Unlock()
	return &storagepb.CreateSnapshotResponse{SnapshotId: id}, nil
}

// ReleaseSnapshot closes/disposes of a registered point-in-time snapshot.
func (s *StorageServer) ReleaseSnapshot(ctx context.Context, req *storagepb.ReleaseSnapshotRequest) (*storagepb.ReleaseSnapshotResponse, error) {
	if req.SnapshotId == "" {
		return nil, status.Error(codes.InvalidArgument, "snapshot_id cannot be empty")
	}
	s.mu.Lock()
	entry, ok := s.snapshots[req.SnapshotId]
	if ok {
		delete(s.snapshots, req.SnapshotId)
	}
	s.mu.Unlock()

	if !ok {
		return nil, status.Errorf(codes.NotFound, "snapshot %s not found", req.SnapshotId)
	}

	entry.mu.Lock()
	entry.released = true
	shouldClose := entry.refCount == 0
	entry.mu.Unlock()

	if shouldClose {
		entry.snap.Close()
	}
	return &storagepb.ReleaseSnapshotResponse{}, nil
}
