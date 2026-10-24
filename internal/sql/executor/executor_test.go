package executor

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
	"github.com/makeshift-engineering/penguin-db/internal/bridge/kv"
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/planner"
)

func TestExecuteUnsupportedPlan(t *testing.T) {
	exec, _, _ := setupExecutor(t)

	_, err := exec.Execute(context.Background(), nil)
	if !errors.Is(err, ErrUnsupportedPlan) {
		t.Fatalf("expected ErrUnsupportedPlan, got %v", err)
	}
}

// memKV is a concurrency-safe in-memory KV store used by executor tests.
type memKV struct {
	mu   sync.RWMutex
	data map[string][]byte
}

// newMemKV creates an empty in-memory KV store.
func newMemKV() *memKV {
	return &memKV{data: make(map[string][]byte)}
}

// Get returns a copy of the value stored under key.
func (m *memKV) Get(_ context.Context, key []byte) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[string(key)]
	if !ok {
		return nil, kv.ErrKeyNotFound
	}
	cp := make([]byte, len(v))
	copy(cp, v)
	return cp, nil
}

// Put stores a copy of value under key.
func (m *memKV) Put(_ context.Context, key, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(value))
	copy(cp, value)
	m.data[string(key)] = cp
	return nil
}

// Delete removes key from the store.
func (m *memKV) Delete(_ context.Context, key []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.data[string(key)]; !ok {
		return kv.ErrKeyNotFound
	}
	delete(m.data, string(key))
	return nil
}

// Scan returns an iterator over entries whose keys have prefix.
func (m *memKV) Scan(_ context.Context, prefix []byte) (kv.Iterator, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var keys []string
	for k := range m.data {
		if len(k) >= len(prefix) && k[:len(prefix)] == string(prefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	entries := make([]memKVEntry, len(keys))
	for i, k := range keys {
		v := m.data[k]
		keyCopy := []byte(k)
		valueCopy := make([]byte, len(v))
		copy(valueCopy, v)
		entries[i] = memKVEntry{key: keyCopy, value: valueCopy}
	}
	return &memIter{entries: entries}, nil
}

// WriteBatch applies all KV operations to the in-memory store.
func (m *memKV) WriteBatch(_ context.Context, ops []kv.Op) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, op := range ops {
		switch op.Type {
		case kv.OpPut:
			valueCopy := make([]byte, len(op.Value))
			copy(valueCopy, op.Value)
			m.data[string(op.Key)] = valueCopy
		case kv.OpDelete:
			delete(m.data, string(op.Key))
		}
	}
	return nil
}

// memKVEntry stores one copied in-memory KV entry for iteration.
type memKVEntry struct {
	key   []byte
	value []byte
}

// memIter iterates over a fixed set of in-memory KV entries.
type memIter struct {
	entries []memKVEntry
	pos     int
}

// Valid reports whether the iterator has another entry.
func (it *memIter) Valid() bool {
	return it.pos < len(it.entries)
}

// Next returns the current entry and advances the iterator.
func (it *memIter) Next() (key, value []byte) {
	if it.pos >= len(it.entries) {
		return nil, nil
	}
	e := it.entries[it.pos]
	it.pos++
	return e.key, e.value
}

// Close exhausts the iterator.
func (it *memIter) Close() {
	it.pos = len(it.entries)
}

// Err reports iterator errors; the in-memory iterator never produces one.
func (it *memIter) Err() error {
	return nil
}

// intPtr returns a pointer to v for optional schema metadata fields.
func intPtr(v int) *int {
	return &v
}

// setupExecutor creates a fresh in-memory KV, catalog, and executor.
func setupExecutor(t *testing.T) (*Executor, *catalog.Catalog, kv.KV) {
	t.Helper()
	store := newMemKV()
	cat := catalog.NewEmptyCatalog()
	exec := New(store, cat)
	return exec, cat, store
}

// mustExec executes plan and fails the test if execution returns an error.
func mustExec(t *testing.T, exec *Executor, plan planner.Plan) *Result {
	t.Helper()
	result, err := exec.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	return result
}

// testTableMeta returns the shared users-table schema fixture.
func testTableMeta() *catalog.TableMeta {
	return &catalog.TableMeta{
		Database: "testdb",
		Name:     "users",
		Columns: []catalog.ColumnMeta{
			{Name: "id", Type: ast.TypeInt, NotNull: true, PrimaryKey: true},
			{Name: "name", Type: ast.TypeVarchar, VarcharLen: intPtr(255)},
			{Name: "email", Type: ast.TypeText},
			{Name: "active", Type: ast.TypeBoolean, NotNull: true},
		},
		PrimaryKey: []string{"id"},
		CreatedAt:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Version:    1,
	}
}

// setupUsersTable creates the shared test database and users table.
func setupUsersTable(t *testing.T) *Executor {
	t.Helper()
	exec, _, _ := setupExecutor(t)
	mustExec(t, exec, &planner.CreateDatabasePlan{Name: "testdb"})
	mustExec(t, exec, &planner.CreateTablePlan{
		Database: "testdb",
		Table:    "users",
		Schema:   testTableMeta(),
	})
	return exec
}

// buildResolvedColumns returns resolved columns for the users-table fixture.
func buildResolvedColumns() []planner.ResolvedColumn {
	return []planner.ResolvedColumn{
		{Database: "testdb", Table: "users", Name: "id", Index: 0, Type: ast.TypeInt, Nullable: false},
		{Database: "testdb", Table: "users", Name: "name", Index: 1, Type: ast.TypeVarchar, VarcharLen: intPtr(255), Nullable: true},
		{Database: "testdb", Table: "users", Name: "email", Index: 2, Type: ast.TypeText, Nullable: true},
		{Database: "testdb", Table: "users", Name: "active", Index: 3, Type: ast.TypeBoolean, Nullable: false},
	}
}
