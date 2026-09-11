package kv

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	storagepb "github.com/makeshift-engineering/penguin-db/gen/go/storage/v1"
	"github.com/makeshift-engineering/penguin-db/internal/rpc/storage_server"
	"github.com/makeshift-engineering/penguin-db/internal/storage"
)

func setupRemoteKVTest(t *testing.T) (KV, func()) {
	t.Helper()

	tempDirectory, err := os.MkdirTemp("", "penguin-db-remote-kv-test-*")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}

	options := storage.DefaultOptions()
	engine, err := storage.NewEngine(tempDirectory, options)
	if err != nil {
		os.RemoveAll(tempDirectory)
		t.Fatalf("failed to create storage engine: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = engine.Close()
		os.RemoveAll(tempDirectory)
		t.Fatalf("failed to listen on ephemeral port: %v", err)
	}

	grpcServer := grpc.NewServer()
	storageServer := storage_server.NewStorageServer(engine)
	storagepb.RegisterStorageServiceServer(grpcServer, storageServer)

	go func() {
		if err := grpcServer.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("gRPC server failed to serve: %v", err)
		}
	}()

	clientConn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		grpcServer.Stop()
		_ = engine.Close()
		os.RemoveAll(tempDirectory)
		t.Fatalf("failed to dial gRPC server: %v", err)
	}

	client := storagepb.NewStorageServiceClient(clientConn)
	remoteKVStore := NewRemoteKV(client)

	cleanupFunc := func() {
		clientConn.Close()
		grpcServer.GracefulStop()
		storageServer.ReleaseAllSnapshots()
		_ = engine.Close()
		os.RemoveAll(tempDirectory)
	}

	return remoteKVStore, cleanupFunc
}

func TestRemoteKV_PutGetDelete(t *testing.T) {
	store, cleanup := setupRemoteKVTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get non-existent key -> expect ErrKeyNotFound
	_, err := store.Get(ctx, []byte("nonexistent-key"))
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("expected ErrKeyNotFound, got: %v", err)
	}

	// Put key-value pair
	key := []byte("user:101")
	value := []byte("Alice")
	if err := store.Put(ctx, key, value); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Get existing key
	gotValue, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(gotValue) != string(value) {
		t.Fatalf("expected value %s, got %s", string(value), string(gotValue))
	}

	// Delete key
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Get deleted key -> expect ErrKeyNotFound
	_, err = store.Get(ctx, key)
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("expected ErrKeyNotFound after deletion, got: %v", err)
	}
}

func TestRemoteKV_WriteBatch(t *testing.T) {
	store, cleanup := setupRemoteKVTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Pre-populate key to delete in batch
	if err := store.Put(ctx, []byte("k2"), []byte("to-delete")); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	batchOps := []Op{
		{Type: OpPut, Key: []byte("k1"), Value: []byte("v1")},
		{Type: OpDelete, Key: []byte("k2")},
		{Type: OpPut, Key: []byte("k3"), Value: []byte("v3")},
	}

	if err := store.WriteBatch(ctx, batchOps); err != nil {
		t.Fatalf("WriteBatch failed: %v", err)
	}

	// Assert k1 was inserted
	v1, err := store.Get(ctx, []byte("k1"))
	if err != nil || string(v1) != "v1" {
		t.Fatalf("k1 expected v1, got: %s (err: %v)", string(v1), err)
	}

	// Assert k2 was deleted
	_, err = store.Get(ctx, []byte("k2"))
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("expected k2 to be deleted by batch, got: %v", err)
	}

	// Assert k3 was inserted
	v3, err := store.Get(ctx, []byte("k3"))
	if err != nil || string(v3) != "v3" {
		t.Fatalf("k3 expected v3, got: %s (err: %v)", string(v3), err)
	}
}

func TestRemoteKV_Scan(t *testing.T) {
	store, cleanup := setupRemoteKVTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	entries := []struct {
		key   string
		value string
	}{
		{"prefix:1", "val1"},
		{"prefix:2", "val2"},
		{"prefix:3", "val3"},
		{"other:1", "val4"},
	}

	for _, entry := range entries {
		if err := store.Put(ctx, []byte(entry.key), []byte(entry.value)); err != nil {
			t.Fatalf("failed to Put key %s: %v", entry.key, err)
		}
	}

	iterator, err := store.Scan(ctx, []byte("prefix:"))
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	defer iterator.Close()

	var scannedKeys []string
	var scannedValues []string

	for iterator.Valid() {
		k, v := iterator.Next()
		scannedKeys = append(scannedKeys, string(k))
		scannedValues = append(scannedValues, string(v))
	}

	if err := iterator.Err(); err != nil {
		t.Fatalf("iterator error: %v", err)
	}

	expectedKeys := []string{"prefix:1", "prefix:2", "prefix:3"}
	expectedValues := []string{"val1", "val2", "val3"}

	if len(scannedKeys) != len(expectedKeys) {
		t.Fatalf("expected %d scanned entries, got %d", len(expectedKeys), len(scannedKeys))
	}

	for i := range expectedKeys {
		if scannedKeys[i] != expectedKeys[i] || scannedValues[i] != expectedValues[i] {
			t.Fatalf("entry %d mismatch: got (%s, %s), expected (%s, %s)", i, scannedKeys[i], scannedValues[i], expectedKeys[i], expectedValues[i])
		}
	}
}

func TestRemoteKV_ScanEmpty(t *testing.T) {
	store, cleanup := setupRemoteKVTest(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	iterator, err := store.Scan(ctx, []byte("empty-prefix:"))
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	defer iterator.Close()

	if iterator.Valid() {
		t.Fatalf("expected Valid() to be false for empty prefix")
	}

	key, val := iterator.Next()
	if key != nil || val != nil {
		t.Fatalf("expected Next() to return (nil, nil), got (%v, %v)", key, val)
	}
}
