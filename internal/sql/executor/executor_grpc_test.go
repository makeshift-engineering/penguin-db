package executor

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	storagepb "github.com/makeshift-engineering/penguin-db/gen/go/storage/v1"
	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
	"github.com/makeshift-engineering/penguin-db/internal/bridge/kv"
	"github.com/makeshift-engineering/penguin-db/internal/rpc/storage_server"
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/planner"
	"github.com/makeshift-engineering/penguin-db/internal/storage"
)


// mockSnapshot adapts mockEngine to storage.Snapshot
type mockSnapshot struct {
	m *mockEngine
}

func (s *mockSnapshot) Get(key []byte) ([]byte, error) { return s.m.Get(key) }
func (s *mockSnapshot) Scan(prefix []byte) (storage.Iterator, error) {
	return s.m.Scan(prefix)
}
func (s *mockSnapshot) Close() {}

// mockEngine implements storage.Engine using memKV
type mockEngine struct {
	memKV *memKV
}

func (m *mockEngine) Get(key []byte) ([]byte, error) {
	v, err := m.memKV.Get(context.Background(), key)
	if err == kv.ErrKeyNotFound {
		return nil, storage.ErrKeyNotFound
	}
	return v, err
}
func (m *mockEngine) Put(ctx context.Context, key, value []byte) error {
	return m.memKV.Put(ctx, key, value)
}
func (m *mockEngine) Delete(ctx context.Context, key []byte) error {
	err := m.memKV.Delete(ctx, key)
	if err == kv.ErrKeyNotFound {
		return storage.ErrKeyNotFound
	}
	return err
}
func (m *mockEngine) Scan(prefix []byte) (storage.Iterator, error) {
	it, err := m.memKV.Scan(context.Background(), prefix)
	if err != nil {
		return nil, err
	}
	return it, nil
}
func (m *mockEngine) WriteBatch(ctx context.Context, ops []storage.Op) error {
	var kvops []kv.Op
	for _, o := range ops {
		t := kv.OpPut
		if o.Type == storage.OpDelete {
			t = kv.OpDelete
		}
		kvops = append(kvops, kv.Op{Type: t, Key: o.Key, Value: o.Value})
	}
	return m.memKV.WriteBatch(ctx, kvops)
}
func (m *mockEngine) Snapshot() (storage.Snapshot, error) {
	return &mockSnapshot{m: m}, nil
}
func (m *mockEngine) Close() error { return nil }

func setupGrpcExecutor(t *testing.T) (*Executor, *catalog.Catalog, func()) {
	t.Helper()

	// 1. Setup mock engine
	mem := newMemKV()
	engine := &mockEngine{memKV: mem}

	// 2. Setup gRPC server
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	svr := grpc.NewServer()
	storageServer := storage_server.NewStorageServer(engine)
	storagepb.RegisterStorageServiceServer(svr, storageServer)

	go func() {
		if err := svr.Serve(lis); err != nil {
			panic(err)
		}
	}()

	// 3. Setup gRPC client
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}

	client := storagepb.NewStorageServiceClient(conn)
	grpcKV := kv.NewGrpcKV(client)

	// 4. Setup Executor
	cat := catalog.NewEmptyCatalog()
	exec := New(grpcKV, cat)

	cleanup := func() {
		conn.Close()
		svr.Stop()
	}

	return exec, cat, cleanup
}

func TestGrpcKVExecution(t *testing.T) {
	exec, cat, cleanup := setupGrpcExecutor(t)
	defer cleanup()

	// CREATE DATABASE
	mustExec(t, exec, &planner.CreateDatabasePlan{Name: "testdb"})
	if !cat.DatabaseExists("testdb") {
		t.Fatal("database should exist")
	}

	// CREATE TABLE
	schema := testTableMeta()
	mustExec(t, exec, &planner.CreateTablePlan{
		Database: "testdb",
		Table:    "users",
		Schema:   schema,
	})

	cols := buildResolvedColumns()

	// INSERT
	insertPlan := &planner.InsertPlan{
		Database: "testdb",
		Table:    "users",
		Schema:   schema,
		Columns:  cols,
		Rows: [][]planner.ResolvedExpr{
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 10},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "GrpcUser"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "grpc@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
		},
	}
	res := mustExec(t, exec, insertPlan)
	if res.RowsAffected != 1 {
		t.Errorf("expected 1 row inserted, got %d", res.RowsAffected)
	}

	// SELECT
	resolvedTable := &planner.ResolvedTable{
		Database: "testdb", Table: "users", Binding: "users", Schema: schema,
	}
	selectPlan := &planner.QueryPlan{
		Root: &planner.ProjectNode{
			Input: &planner.ScanNode{Table: resolvedTable},
			Items: []planner.ProjectItem{
				{Expr: &planner.ResolvedColumnRef{Column: cols[0]}, Alias: "id"},
				{Expr: &planner.ResolvedColumnRef{Column: cols[1]}, Alias: "name"},
			},
		},
		Columns: []planner.OutputColumn{
			{Name: "id", Type: ast.TypeInt},
			{Name: "name", Type: ast.TypeVarchar},
		},
	}

	res = mustExec(t, exec, selectPlan)
	if len(res.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(res.Rows))
	}
	if res.Rows[0][1] != "GrpcUser" {
		t.Errorf("expected 'GrpcUser', got %v", res.Rows[0][1])
	}
}
