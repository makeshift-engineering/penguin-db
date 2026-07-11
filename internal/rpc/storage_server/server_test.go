package storage_server

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	storagepb "github.com/makeshift-engineering/penguin-db/gen/go/storage/v1"
	"github.com/makeshift-engineering/penguin-db/internal/storage"
)

// startTestServer bootstraps an in-memory storage engine and launches an in-process
// TCP gRPC listener on a random ephemeral port. It returns the client connection
// client stub and a cleanup closure to tear down the server and temp directory.
func startTestServer(t *testing.T) (storagepb.StorageServiceClient, func()) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "penguin-db-rpc-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	opts := storage.DefaultOptions()
	// Disable metrics for testing
	engine, err := storage.NewEngine(tempDir, opts)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("failed to create storage engine: %v", err)
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = engine.Close()
		os.RemoveAll(tempDir)
		t.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	storageServer := NewStorageServer(engine)
	storagepb.RegisterStorageServiceServer(grpcServer, storageServer)

	go func() {
		if err := grpcServer.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("grpc server failed to serve: %v", err)
		}
	}()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		grpcServer.Stop()
		_ = engine.Close()
		os.RemoveAll(tempDir)
		t.Fatalf("failed to dial: %v", err)
	}

	client := storagepb.NewStorageServiceClient(conn)

	cleanup := func() {
		conn.Close()
		grpcServer.GracefulStop()
		storageServer.ReleaseAllSnapshots()
		_ = engine.Close()
		os.RemoveAll(tempDir)
	}

	return client, cleanup
}

// TestPutGetDelete verifies basic key insertion, retrieval, logical deletion,
// and invalid query responses of the gRPC server.
func TestPutGetDelete(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Get non-existent key -> NotFound
	_, err := client.Get(ctx, &storagepb.GetRequest{Key: []byte("non-existent")})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got: %v", err)
	}

	// 2. Put key
	_, err = client.Put(ctx, &storagepb.PutRequest{Key: []byte("k1"), Value: []byte("v1")})
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// 3. Get key
	getRes, err := client.Get(ctx, &storagepb.GetRequest{Key: []byte("k1")})
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(getRes.Value) != "v1" {
		t.Fatalf("expected v1, got: %s", string(getRes.Value))
	}

	// 4. Delete key
	_, err = client.Delete(ctx, &storagepb.DeleteRequest{Key: []byte("k1")})
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// 5. Get deleted key -> NotFound
	_, err = client.Get(ctx, &storagepb.GetRequest{Key: []byte("k1")})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound after Delete, got: %v", err)
	}
}

// TestScan verifies that prefix range scans yield the expected keys sorted
// lexicographically and delivered as a gRPC server stream.
func TestScan(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	keys := []string{"a/1", "a/2", "b/1"}
	vals := []string{"val1", "val2", "val3"}

	for i := range keys {
		_, err := client.Put(ctx, &storagepb.PutRequest{Key: []byte(keys[i]), Value: []byte(vals[i])})
		if err != nil {
			t.Fatalf("failed to Put key %s: %v", keys[i], err)
		}
	}

	stream, err := client.Scan(ctx, &storagepb.ScanRequest{Prefix: []byte("a/")})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	var results []string
	for {
		res, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("stream receive error: %v", err)
		}
		results = append(results, string(res.Key)+":"+string(res.Value))
	}

	expected := []string{"a/1:val1", "a/2:val2"}
	if len(results) != len(expected) {
		t.Fatalf("expected %d scan results, got %d: %v", len(expected), len(results), results)
	}
	for i := range expected {
		if results[i] != expected[i] {
			t.Fatalf("expected results[%d] = %s, got %s", i, expected[i], results[i])
		}
	}
}

// TestWriteBatch asserts that atomic batch operations (updates/deletions) are
// fully committed or rolled back over gRPC.
func TestWriteBatch(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.Put(ctx, &storagepb.PutRequest{Key: []byte("k1"), Value: []byte("old")})
	if err != nil {
		t.Fatalf("failed to Put: %v", err)
	}

	req := &storagepb.WriteBatchRequest{
		Operations: []*storagepb.Op{
			{
				Type:  storagepb.OpType_OP_PUT,
				Key:   []byte("k1"),
				Value: []byte("new"),
			},
			{
				Type: storagepb.OpType_OP_DELETE,
				Key:  []byte("k2"),
			},
			{
				Type:  storagepb.OpType_OP_PUT,
				Key:   []byte("k3"),
				Value: []byte("v3"),
			},
		},
	}

	_, err = client.WriteBatch(ctx, req)
	if err != nil {
		t.Fatalf("WriteBatch failed: %v", err)
	}

	res1, err := client.Get(ctx, &storagepb.GetRequest{Key: []byte("k1")})
	if err != nil || string(res1.Value) != "new" {
		t.Fatalf("k1 was not updated correctly: %v, val: %s", err, string(res1.Value))
	}

	res3, err := client.Get(ctx, &storagepb.GetRequest{Key: []byte("k3")})
	if err != nil || string(res3.Value) != "v3" {
		t.Fatalf("k3 was not written correctly: %v, val: %s", err, string(res3.Value))
	}
}

// TestSnapshotIsolation verifies that point-in-time snapshots keep reading isolated
// from concurrent writes to the database.
func TestSnapshotIsolation(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.Put(ctx, &storagepb.PutRequest{Key: []byte("k1"), Value: []byte("v1")})
	if err != nil {
		t.Fatalf("failed to Put: %v", err)
	}

	snapRes, err := client.CreateSnapshot(ctx, &storagepb.CreateSnapshotRequest{})
	if err != nil {
		t.Fatalf("failed to CreateSnapshot: %v", err)
	}
	snapID := snapRes.SnapshotId
	if snapID == "" {
		t.Fatalf("expected non-empty snapshot_id")
	}

	_, err = client.Put(ctx, &storagepb.PutRequest{Key: []byte("k1"), Value: []byte("v2")})
	if err != nil {
		t.Fatalf("failed to Put: %v", err)
	}

	resActive, err := client.Get(ctx, &storagepb.GetRequest{Key: []byte("k1")})
	if err != nil || string(resActive.Value) != "v2" {
		t.Fatalf("expected v2, got %s (err: %v)", string(resActive.Value), err)
	}

	resSnap, err := client.Get(ctx, &storagepb.GetRequest{Key: []byte("k1"), SnapshotId: snapID})
	if err != nil || string(resSnap.Value) != "v1" {
		t.Fatalf("expected snapshot to return v1, got %s (err: %v)", string(resSnap.Value), err)
	}

	_, err = client.ReleaseSnapshot(ctx, &storagepb.ReleaseSnapshotRequest{SnapshotId: snapID})
	if err != nil {
		t.Fatalf("failed to ReleaseSnapshot: %v", err)
	}

	_, err = client.Get(ctx, &storagepb.GetRequest{Key: []byte("k1"), SnapshotId: snapID})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected snapshot read to fail with NotFound after release, got: %v", err)
	}
}
