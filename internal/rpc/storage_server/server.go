package storage_server

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	storagepb "github.com/makeshift-engineering/penguin-db/gen/go/storage/v1"
	"github.com/makeshift-engineering/penguin-db/internal/storage"
)

// StorageServer implements the storagepb.StorageServiceServer gRPC interface,
// bridging incoming network requests to the underlying LSM-tree storage engine.
// It also manages point-in-time snapshots to provide read isolation.
type StorageServer struct {
	storagepb.UnimplementedStorageServiceServer
	engine storage.Engine

	mu        sync.RWMutex
	snapshots map[string]storage.Snapshot
}

// NewStorageServer instantiates a new StorageServer backed by the given storage engine.
func NewStorageServer(engine storage.Engine) *StorageServer {
	return &StorageServer{
		engine:    engine,
		snapshots: make(map[string]storage.Snapshot),
	}
}

// Register binds this StorageServer implementation to the provided gRPC Server.
func (s *StorageServer) Register(server *grpc.Server) {
	storagepb.RegisterStorageServiceServer(server, s)
}

// ReleaseAllSnapshots iterates over all registered snapshots, closes them to release
// pinned SSTable file resources, and clears them from memory.
func (s *StorageServer) ReleaseAllSnapshots() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, snap := range s.snapshots {
		snap.Close()
		delete(s.snapshots, id)
	}
}

// mapError translates engine-specific error sentinel types into standard gRPC status codes.
func mapError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, storage.ErrKeyNotFound):
		return status.Error(codes.NotFound, "key not found")
	case errors.Is(err, storage.ErrEmptyKey):
		return status.Error(codes.InvalidArgument, "key cannot be empty")
	case errors.Is(err, storage.ErrEngineClosed):
		return status.Error(codes.FailedPrecondition, "storage engine is closed")
	case errors.Is(err, storage.ErrSnapshotClosed):
		return status.Error(codes.FailedPrecondition, "snapshot is closed")
	default:
		return status.Errorf(codes.Internal, "storage internal error: %v", err)
	}
}

// Put writes a single key-value record to the underlying database engine.
func (s *StorageServer) Put(ctx context.Context, req *storagepb.PutRequest) (*storagepb.PutResponse, error) {
	if len(req.Key) == 0 {
		return nil, status.Error(codes.InvalidArgument, "key cannot be empty")
	}
	if err := s.engine.Put(req.Key, req.Value); err != nil {
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
		s.mu.RLock()
		snap, ok := s.snapshots[req.SnapshotId]
		s.mu.RUnlock()
		if !ok {
			return nil, status.Errorf(codes.NotFound, "snapshot %s not found", req.SnapshotId)
		}
		val, err = snap.Get(req.Key)
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
	if err := s.engine.Delete(req.Key); err != nil {
		return nil, mapError(err)
	}
	return &storagepb.DeleteResponse{}, nil
}

// Scan returns a prefix-filtered stream of sorted key-value pairs from the engine or snapshot.
func (s *StorageServer) Scan(req *storagepb.ScanRequest, stream storagepb.StorageService_ScanServer) error {
	var iter storage.Iterator
	var err error
	if req.SnapshotId != "" {
		s.mu.RLock()
		snap, ok := s.snapshots[req.SnapshotId]
		s.mu.RUnlock()
		if !ok {
			return status.Errorf(codes.NotFound, "snapshot %s not found", req.SnapshotId)
		}
		iter, err = snap.Scan(req.Prefix)
	} else {
		iter, err = s.engine.Scan(req.Prefix)
	}
	if err != nil {
		return mapError(err)
	}
	defer iter.Close()

	for iter.Valid() {
		if err := stream.Context().Err(); err != nil {
			return err
		}
		key, val := iter.Next()
		if err := stream.Send(&storagepb.ScanResponse{Key: key, Value: val}); err != nil {
			return err
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
	if err := s.engine.WriteBatch(ops); err != nil {
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
	s.snapshots[id] = snap
	s.mu.Unlock()
	return &storagepb.CreateSnapshotResponse{SnapshotId: id}, nil
}

// ReleaseSnapshot closes/disposes of a registered point-in-time snapshot.
func (s *StorageServer) ReleaseSnapshot(ctx context.Context, req *storagepb.ReleaseSnapshotRequest) (*storagepb.ReleaseSnapshotResponse, error) {
	if req.SnapshotId == "" {
		return nil, status.Error(codes.InvalidArgument, "snapshot_id cannot be empty")
	}
	s.mu.Lock()
	snap, ok := s.snapshots[req.SnapshotId]
	if ok {
		delete(s.snapshots, req.SnapshotId)
	}
	s.mu.Unlock()

	if !ok {
		return nil, status.Errorf(codes.NotFound, "snapshot %s not found", req.SnapshotId)
	}
	snap.Close()
	return &storagepb.ReleaseSnapshotResponse{}, nil
}
