package kv

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	storagepb "github.com/makeshift-engineering/penguin-db/gen/go/storage/v1"
)

// Ensure RemoteKV satisfies the KV interface at compile time.
var _ KV = (*RemoteKV)(nil)

// RemoteKV adapts a gRPC StorageServiceClient to satisfy the KV interface.
// It allows database layers to communicate with a remote storage server over gRPC.
type RemoteKV struct {
	client storagepb.StorageServiceClient
}

// NewRemoteKV returns a new RemoteKV instance wrapping the provided gRPC client.
func NewRemoteKV(client storagepb.StorageServiceClient) *RemoteKV {
	return &RemoteKV{client: client}
}

// Get retrieves the value associated with key from the remote storage server.
// Returns ErrKeyNotFound if the key does not exist.
func (r *RemoteKV) Get(ctx context.Context, key []byte) ([]byte, error) {
	request := &storagepb.GetRequest{
		Key: key,
	}
	response, err := r.client.Get(ctx, request)
	if err != nil {
		return nil, mapGRPCError(err)
	}
	return response.Value, nil
}

// Put writes a key-value pair to the remote storage server.
func (r *RemoteKV) Put(ctx context.Context, key, value []byte) error {
	request := &storagepb.PutRequest{
		Key:   key,
		Value: value,
	}
	_, err := r.client.Put(ctx, request)
	return mapGRPCError(err)
}

// Delete removes a key from the remote storage server.
// Returns ErrKeyNotFound if the key does not exist.
func (r *RemoteKV) Delete(ctx context.Context, key []byte) error {
	request := &storagepb.DeleteRequest{
		Key: key,
	}
	_, err := r.client.Delete(ctx, request)
	return mapGRPCError(err)
}

// Scan initiates a streaming scan for all keys sharing the specified prefix.
func (r *RemoteKV) Scan(ctx context.Context, prefix []byte) (Iterator, error) {
	streamContext, cancelFunc := context.WithCancel(ctx)
	request := &storagepb.ScanRequest{
		Prefix: prefix,
	}
	streamClient, err := r.client.Scan(streamContext, request)
	if err != nil {
		cancelFunc()
		return nil, mapGRPCError(err)
	}
	return newRemoteIterator(streamClient, cancelFunc), nil
}

// WriteBatch atomically applies a slice of Op values to the remote storage server.
func (r *RemoteKV) WriteBatch(ctx context.Context, ops []Op) error {
	if len(ops) == 0 {
		return nil
	}

	operations := make([]*storagepb.Op, len(ops))
	for i, operation := range ops {
		var operationType storagepb.OpType
		switch operation.Type {
		case OpPut:
			operationType = storagepb.OpType_OP_TYPE_PUT
		case OpDelete:
			operationType = storagepb.OpType_OP_TYPE_DELETE
		default:
			return fmt.Errorf("kv: unsupported operation type %d", operation.Type)
		}

		operations[i] = &storagepb.Op{
			Type:  operationType,
			Key:   operation.Key,
			Value: operation.Value,
		}
	}

	request := &storagepb.WriteBatchRequest{
		Operations: operations,
	}
	_, err := r.client.WriteBatch(ctx, request)
	return mapGRPCError(err)
}

// mapGRPCError translates gRPC status codes to clean, user-safe kv errors.
// It strips the "rpc error: code = ... desc = ..." framing so internal
// details never leak to higher layers.
func mapGRPCError(err error) error {
	if err == nil {
		return nil
	}

	st, ok := status.FromError(err)
	if !ok {
		return err // not a gRPC error, pass through
	}

	switch st.Code() {
	case codes.NotFound:
		return ErrKeyNotFound
	case codes.Unavailable:
		return fmt.Errorf("kv: storage server unavailable: %s", st.Message())
	case codes.DeadlineExceeded:
		return context.DeadlineExceeded
	case codes.Canceled:
		return context.Canceled
	case codes.InvalidArgument:
		return fmt.Errorf("kv: invalid argument: %s", st.Message())
	case codes.FailedPrecondition:
		return fmt.Errorf("kv: precondition failed: %s", st.Message())
	case codes.Internal:
		return errors.New("kv: internal storage error")
	default:
		return fmt.Errorf("kv: storage error: %s", st.Message())
	}
}
