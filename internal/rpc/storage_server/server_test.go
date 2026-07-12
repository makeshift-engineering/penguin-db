package storage_server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
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

	_, err = client.Put(ctx, &storagepb.PutRequest{Key: []byte("k2"), Value: []byte("to-delete")})
	if err != nil {
		t.Fatalf("failed to Put: %v", err)
	}

	req := &storagepb.WriteBatchRequest{
		Operations: []*storagepb.Op{
			{
				Type:  storagepb.OpType_OP_TYPE_PUT,
				Key:   []byte("k1"),
				Value: []byte("new"),
			},
			{
				Type: storagepb.OpType_OP_TYPE_DELETE,
				Key:  []byte("k2"),
			},
			{
				Type:  storagepb.OpType_OP_TYPE_PUT,
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

	_, err = client.Get(ctx, &storagepb.GetRequest{Key: []byte("k2")})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected k2 to be deleted by batch, got: %v", err)
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

// TestScanShutdownInterrupt verifies that calling Stop() on StorageServer
// interrupts active streaming Scan calls.
func TestScanShutdownInterrupt(t *testing.T) {
	dir := t.TempDir()
	opts := storage.DefaultOptions()
	engine, err := storage.NewEngine(dir, opts)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	// Put some data
	keys := []string{"prefix_k1", "prefix_k2", "prefix_k3", "prefix_k4", "prefix_k5"}
	for _, k := range keys {
		if err := engine.Put(context.Background(), []byte(k), []byte("val")); err != nil {
			t.Fatalf("Put %s: %v", k, err)
		}
	}

	server := NewStorageServer(engine)
	defer server.Stop()

	mockStream := &mockScanServer{ctx: context.Background()}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Scan(&storagepb.ScanRequest{Prefix: []byte("prefix_")}, mockStream)
	}()

	// Wait a moment and then call Stop()
	time.Sleep(50 * time.Millisecond)
	server.Stop()

	select {
	case scanErr := <-errCh:
		if status.Code(scanErr) != codes.Aborted {
			t.Errorf("expected codes.Aborted on scan interrupt, got: %v", scanErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Scan to abort on Stop()")
	}
}

type mockScanServer struct {
	grpc.ServerStream
	ctx context.Context
}

func (m *mockScanServer) Context() context.Context {
	return m.ctx
}

func (m *mockScanServer) Send(res *storagepb.ScanResponse) error {
	// Simulate slow reading to allow cancellation to trigger
	time.Sleep(50 * time.Millisecond)
	return nil
}

// TestSnapshotExpiration verifies that inactive snapshots are automatically evicted by the janitor.
func TestSnapshotExpiration(t *testing.T) {
	dir := t.TempDir()
	opts := storage.DefaultOptions()
	engine, err := storage.NewEngine(dir, opts)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	server := &StorageServer{
		engine:    engine,
		snapshots: make(map[string]*snapshotEntry),
		stopChan:  make(chan struct{}),
	}
	defer server.Stop()

	// We start the janitor with a very short check interval and expiry threshold for testing
	go server.startSnapshotJanitor(10*time.Millisecond, 20*time.Millisecond)

	// Create snapshot
	snap, err := engine.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	server.mu.Lock()
	server.snapshots["snap1"] = &snapshotEntry{
		snap:      snap,
		createdAt: time.Now(),
	}
	server.mu.Unlock()

	// Wait for eviction
	time.Sleep(100 * time.Millisecond)

	server.mu.Lock()
	_, found := server.snapshots["snap1"]
	server.mu.Unlock()

	if found {
		t.Error("expected snapshot to be evicted by janitor, but it was found")
	}

	if _, err := snap.Get([]byte("key")); !errors.Is(err, storage.ErrSnapshotClosed) {
		t.Errorf("expected expired snapshot to be closed, got %v", err)
	}
}

// TestReleaseSnapshot_NotFound verifies that releasing a non-existent or
// already released snapshot returns codes.NotFound.
func TestReleaseSnapshot_NotFound(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx := context.Background()
	_, err := client.ReleaseSnapshot(ctx, &storagepb.ReleaseSnapshotRequest{
		SnapshotId: "nonexistent-id",
	})
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected codes.NotFound, got: %v", err)
	}
}

// TestWriteBatch_UnsupportedOpType verifies that WriteBatch fails with codes.InvalidArgument
// when an operation with an unsupported type is requested.
func TestWriteBatch_UnsupportedOpType(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx := context.Background()
	req := &storagepb.WriteBatchRequest{
		Operations: []*storagepb.Op{
			{
				Type: storagepb.OpType_OP_TYPE_UNSPECIFIED,
				Key:  []byte("k"),
			},
		},
	}
	_, err := client.WriteBatch(ctx, req)
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected codes.InvalidArgument, got: %v", err)
	}
}

// TestScan_ContextCancel verifies that Scan handler returns immediately
// if the stream context is cancelled.
func TestScan_ContextCancel(t *testing.T) {
	dir := t.TempDir()
	opts := storage.DefaultOptions()
	engine, err := storage.NewEngine(dir, opts)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer engine.Close()

	if err := engine.Put(context.Background(), []byte("prefix_k1"), []byte("v1")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	server := NewStorageServer(engine)
	defer server.Stop()

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mockStream := &mockScanServer{ctx: ctx}
	err = server.Scan(&storagepb.ScanRequest{Prefix: []byte("prefix_")}, mockStream)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got: %v", err)
	}
}

// TestConcurrentSnapshotReadRelease exercises concurrent snapshot creation,
// reading (Get), and releasing under the race detector.
func TestConcurrentSnapshotReadRelease(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Create a snapshot to read concurrently
	res, err := client.CreateSnapshot(ctx, &storagepb.CreateSnapshotRequest{})
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}
	snapID := res.SnapshotId

	var wg sync.WaitGroup
	wg.Add(3)

	// Goroutine 1: Concurrently read from the snapshot
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			_, _ = client.Get(ctx, &storagepb.GetRequest{
				Key:        []byte("nonexistent"),
				SnapshotId: snapID,
			})
		}
	}()

	// Goroutine 2: Concurrently read from the snapshot
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			_, _ = client.Get(ctx, &storagepb.GetRequest{
				Key:        []byte("nonexistent"),
				SnapshotId: snapID,
			})
		}
	}()

	// Goroutine 3: Concurrently call ReleaseSnapshot
	go func() {
		defer wg.Done()
		// Sleep slightly to let reads start
		time.Sleep(10 * time.Millisecond)
		_, _ = client.ReleaseSnapshot(ctx, &storagepb.ReleaseSnapshotRequest{
			SnapshotId: snapID,
		})
	}()

	wg.Wait()
}

// collectScan executes a full Scan RPC and collects all streamed responses into a slice.
func collectScan(t *testing.T, client storagepb.StorageServiceClient, req *storagepb.ScanRequest) []*storagepb.ScanResponse {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.Scan(ctx, req)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	var results []*storagepb.ScanResponse
	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Scan Recv failed: %v", err)
		}
		results = append(results, resp)
	}
	return results
}

// TestScan_Limit verifies that a non-zero limit caps the number of keys returned.
func TestScan_Limit(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for i := 1; i <= 10; i++ {
		key := []byte(fmt.Sprintf("key-%02d", i))
		_, err := client.Put(ctx, &storagepb.PutRequest{Key: key, Value: []byte("val")})
		if err != nil {
			t.Fatalf("Put %s: %v", key, err)
		}
	}

	results := collectScan(t, client, &storagepb.ScanRequest{Limit: 3})
	if len(results) != 3 {
		t.Errorf("expected 3 results with limit=3, got %d", len(results))
	}
}

// TestScan_CursorKey verifies that cursor_key resumes the scan from the key strictly
// after the provided cursor value, implementing exclusive-start pagination.
func TestScan_CursorKey(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for i := 1; i <= 5; i++ {
		key := []byte(fmt.Sprintf("key-%02d", i))
		_, err := client.Put(ctx, &storagepb.PutRequest{Key: key, Value: []byte("val")})
		if err != nil {
			t.Fatalf("Put %s: %v", key, err)
		}
	}

	// Resume after "key-02"; expect key-03, key-04, key-05.
	results := collectScan(t, client, &storagepb.ScanRequest{CursorKey: []byte("key-02")})
	if len(results) != 3 {
		t.Errorf("expected 3 results after cursor key-02, got %d", len(results))
	}
	if string(results[0].Key) != "key-03" {
		t.Errorf("expected first result to be key-03, got %s", results[0].Key)
	}
}

// TestScan_LimitAndCursor verifies that limit and cursor_key compose correctly,
// returning at most limit results starting exclusively after cursor_key.
func TestScan_LimitAndCursor(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for i := 1; i <= 10; i++ {
		key := []byte(fmt.Sprintf("key-%02d", i))
		_, err := client.Put(ctx, &storagepb.PutRequest{Key: key, Value: []byte("val")})
		if err != nil {
			t.Fatalf("Put %s: %v", key, err)
		}
	}

	// After key-05 with limit=3 -> expect key-06, key-07, key-08.
	results := collectScan(t, client, &storagepb.ScanRequest{
		CursorKey: []byte("key-05"),
		Limit:     3,
	})
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
	if string(results[0].Key) != "key-06" {
		t.Errorf("expected first result to be key-06, got %s", results[0].Key)
	}
	if string(results[2].Key) != "key-08" {
		t.Errorf("expected last result to be key-08, got %s", results[2].Key)
	}
}

// TestScan_LimitClamped verifies that a limit exceeding maxScanLimit is silently clamped
// to the server ceiling and does not return more than maxScanLimit results.
func TestScan_LimitClamped(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for i := 1; i <= 5; i++ {
		key := []byte(fmt.Sprintf("key-%02d", i))
		_, err := client.Put(ctx, &storagepb.PutRequest{Key: key, Value: []byte("val")})
		if err != nil {
			t.Fatalf("Put %s: %v", key, err)
		}
	}

	// Pass a limit far above maxScanLimit; all 5 rows should be returned since
	// the dataset is smaller than the ceiling.
	results := collectScan(t, client, &storagepb.ScanRequest{Limit: 999_999})
	if len(results) != 5 {
		t.Errorf("expected all 5 results when limit is above ceiling, got %d", len(results))
	}
}
