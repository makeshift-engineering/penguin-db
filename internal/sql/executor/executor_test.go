package executor

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
	"github.com/makeshift-engineering/penguin-db/internal/bridge/kv"
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/planner"
)

// --- In-memory KV implementation for tests ---

// memKV is a minimal in-memory KV store for executor tests.
type memKV struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newMemKV() *memKV {
	return &memKV{data: make(map[string][]byte)}
}

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

func (m *memKV) Put(_ context.Context, key, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(value))
	copy(cp, value)
	m.data[string(key)] = cp
	return nil
}

func (m *memKV) Delete(_ context.Context, key []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.data[string(key)]; !ok {
		return kv.ErrKeyNotFound
	}
	delete(m.data, string(key))
	return nil
}

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
		keyCp := make([]byte, len(k))
		copy(keyCp, []byte(k))
		valCp := make([]byte, len(v))
		copy(valCp, v)
		entries[i] = memKVEntry{key: keyCp, value: valCp}
	}
	return &memIter{entries: entries, pos: 0}, nil
}

func (m *memKV) WriteBatch(_ context.Context, ops []kv.Op) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, op := range ops {
		switch op.Type {
		case kv.OpPut:
			cp := make([]byte, len(op.Value))
			copy(cp, op.Value)
			m.data[string(op.Key)] = cp
		case kv.OpDelete:
			delete(m.data, string(op.Key))
		}
	}
	return nil
}

type memKVEntry struct {
	key   []byte
	value []byte
}

type memIter struct {
	entries []memKVEntry
	pos     int
}

func (it *memIter) Valid() bool { return it.pos < len(it.entries) }
func (it *memIter) Next() (key, value []byte) {
	if it.pos >= len(it.entries) {
		return nil, nil
	}
	e := it.entries[it.pos]
	it.pos++
	return e.key, e.value
}
func (it *memIter) Close() { it.pos = len(it.entries) }
func (it *memIter) Err() error { return nil }

// --- Test helpers ---

func intPtr(v int) *int       { return &v }
func strPtr(v string) *string { return &v }

// setupExecutor creates a fresh in-memory KV, empty catalog, and executor.
func setupExecutor(t *testing.T) (*Executor, *catalog.Catalog, kv.KV) {
	t.Helper()
	store := newMemKV()
	cat := catalog.NewEmptyCatalog()
	exec := New(store, cat)
	return exec, cat, store
}

// mustExec executes a plan and fails the test on error.
func mustExec(t *testing.T, exec *Executor, plan planner.Plan) *Result {
	t.Helper()
	result, err := exec.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	return result
}

// testDBMeta returns a test database metadata.
func testDBMeta() *catalog.DatabaseMeta {
	return &catalog.DatabaseMeta{
		Name:      "testdb",
		CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

// testTableMeta returns a test table schema (users table).
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

// --- DDL Tests ---

func TestCreateDatabase(t *testing.T) {
	exec, cat, _ := setupExecutor(t)

	result := mustExec(t, exec, &planner.CreateDatabasePlan{Name: "mydb"})
	if result.Type != ResultDDL {
		t.Errorf("expected ResultDDL, got %d", result.Type)
	}
	if !cat.DatabaseExists("mydb") {
		t.Error("database 'mydb' should exist in catalog after creation")
	}
}

func TestCreateDatabaseNoOp(t *testing.T) {
	exec, _, _ := setupExecutor(t)

	result := mustExec(t, exec, &planner.CreateDatabasePlan{Name: "mydb", NoOp: true})
	if result.Type != ResultDDL {
		t.Errorf("expected ResultDDL, got %d", result.Type)
	}
}

func TestUseDatabase(t *testing.T) {
	exec, _, _ := setupExecutor(t)

	result := mustExec(t, exec, &planner.UseDatabasePlan{Name: "mydb"})
	if result.SessionUpdate == nil {
		t.Fatal("expected SessionUpdate")
	}
	if result.SessionUpdate.ActiveDatabase != "mydb" {
		t.Errorf("expected active database 'mydb', got %q", result.SessionUpdate.ActiveDatabase)
	}
}

func TestDropDatabase(t *testing.T) {
	exec, cat, _ := setupExecutor(t)

	mustExec(t, exec, &planner.CreateDatabasePlan{Name: "dropme"})
	if !cat.DatabaseExists("dropme") {
		t.Fatal("database should exist")
	}

	result := mustExec(t, exec, &planner.DropDatabasePlan{Name: "dropme"})
	if result.Type != ResultDDL {
		t.Errorf("expected ResultDDL, got %d", result.Type)
	}
	if cat.DatabaseExists("dropme") {
		t.Error("database should not exist after drop")
	}
}

func TestCreateTable(t *testing.T) {
	exec, cat, _ := setupExecutor(t)

	// Create the database first.
	mustExec(t, exec, &planner.CreateDatabasePlan{Name: "testdb"})

	schema := testTableMeta()
	result := mustExec(t, exec, &planner.CreateTablePlan{
		Database: "testdb",
		Table:    "users",
		Schema:   schema,
	})
	if result.Type != ResultDDL {
		t.Errorf("expected ResultDDL, got %d", result.Type)
	}

	_, err := cat.GetTable("testdb", "users")
	if err != nil {
		t.Errorf("table should exist in catalog: %v", err)
	}
}

func TestDropTable(t *testing.T) {
	exec, cat, _ := setupExecutor(t)

	mustExec(t, exec, &planner.CreateDatabasePlan{Name: "testdb"})
	mustExec(t, exec, &planner.CreateTablePlan{
		Database: "testdb",
		Table:    "users",
		Schema:   testTableMeta(),
	})

	result := mustExec(t, exec, &planner.DropTablePlan{Database: "testdb", Table: "users"})
	if result.Type != ResultDDL {
		t.Errorf("expected ResultDDL, got %d", result.Type)
	}

	_, err := cat.GetTable("testdb", "users")
	if err == nil {
		t.Error("table should not exist after drop")
	}
}

// --- DML + Query Tests ---

// setupUsersTable creates testdb and users table, returning the executor.
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

// buildResolvedColumns returns resolved columns for the test users table.
func buildResolvedColumns() []planner.ResolvedColumn {
	return []planner.ResolvedColumn{
		{Database: "testdb", Table: "users", Name: "id", Index: 0, Type: ast.TypeInt, Nullable: false},
		{Database: "testdb", Table: "users", Name: "name", Index: 1, Type: ast.TypeVarchar, VarcharLen: intPtr(255), Nullable: true},
		{Database: "testdb", Table: "users", Name: "email", Index: 2, Type: ast.TypeText, Nullable: true},
		{Database: "testdb", Table: "users", Name: "active", Index: 3, Type: ast.TypeBoolean, Nullable: false},
	}
}

func TestInsertAndSelect(t *testing.T) {
	exec := setupUsersTable(t)
	cols := buildResolvedColumns()
	schema := testTableMeta()

	// INSERT one row.
	mustExec(t, exec, &planner.InsertPlan{
		Database: "testdb",
		Table:    "users",
		Schema:   schema,
		Columns:  cols,
		Rows: [][]planner.ResolvedExpr{
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Alice"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "alice@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
		},
	})

	// SELECT * FROM users (scan all).
	resolvedTable := &planner.ResolvedTable{
		Database: "testdb",
		Table:    "users",
		Binding:  "users",
		Schema:   schema,
	}
	// We can't use the private constructor, so we'll test via QueryPlan.
	// Build a simple scan+project query plan.
	scanNode := &planner.ScanNode{Table: resolvedTable}
	result := mustExec(t, exec, &planner.QueryPlan{
		Root: &planner.ProjectNode{
			Input: scanNode,
			Items: []planner.ProjectItem{
				{Expr: &planner.ResolvedColumnRef{Column: cols[0]}, Alias: "id"},
				{Expr: &planner.ResolvedColumnRef{Column: cols[1]}, Alias: "name"},
				{Expr: &planner.ResolvedColumnRef{Column: cols[2]}, Alias: "email"},
				{Expr: &planner.ResolvedColumnRef{Column: cols[3]}, Alias: "active"},
			},
		},
		Columns: []planner.OutputColumn{
			{Name: "id", Type: ast.TypeInt},
			{Name: "name", Type: ast.TypeVarchar},
			{Name: "email", Type: ast.TypeText},
			{Name: "active", Type: ast.TypeBoolean},
		},
	})

	if result.Type != ResultQuery {
		t.Fatalf("expected ResultQuery, got %d", result.Type)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	row := result.Rows[0]
	if row[0] != int32(1) {
		t.Errorf("id: expected int32(1), got %v (%T)", row[0], row[0])
	}
	if row[1] != "Alice" {
		t.Errorf("name: expected 'Alice', got %v", row[1])
	}
	if row[2] != "alice@test.com" {
		t.Errorf("email: expected 'alice@test.com', got %v", row[2])
	}
	if row[3] != true {
		t.Errorf("active: expected true, got %v", row[3])
	}
}

func TestInsertMultipleAndSelectWithFilter(t *testing.T) {
	exec := setupUsersTable(t)
	cols := buildResolvedColumns()
	schema := testTableMeta()

	// Insert 3 rows.
	mustExec(t, exec, &planner.InsertPlan{
		Database: "testdb",
		Table:    "users",
		Schema:   schema,
		Columns:  cols,
		Rows: [][]planner.ResolvedExpr{
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Alice"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "alice@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 2},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Bob"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "bob@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: false},
			},
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 3},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Charlie"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "charlie@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
		},
	})

	resolvedTable := &planner.ResolvedTable{
		Database: "testdb", Table: "users", Binding: "users", Schema: schema,
	}

	// SELECT id, name FROM users WHERE active = TRUE
	result := mustExec(t, exec, &planner.QueryPlan{
		Root: &planner.ProjectNode{
			Input: &planner.FilterNode{
				Input: &planner.ScanNode{Table: resolvedTable},
				Cond: &planner.ResolvedComparison{
					Left:  &planner.ResolvedColumnRef{Column: cols[3]},
					Op:    planner.OpEq,
					Right: &planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
				},
			},
			Items: []planner.ProjectItem{
				{Expr: &planner.ResolvedColumnRef{Column: cols[0]}, Alias: "id"},
				{Expr: &planner.ResolvedColumnRef{Column: cols[1]}, Alias: "name"},
			},
		},
		Columns: []planner.OutputColumn{
			{Name: "id", Type: ast.TypeInt},
			{Name: "name", Type: ast.TypeVarchar},
		},
	})

	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 active users, got %d", len(result.Rows))
	}
	// Alice (id=1) and Charlie (id=3) should be the active users.
	if result.Rows[0][0] != int32(1) {
		t.Errorf("first active user should be id=1, got %v", result.Rows[0][0])
	}
	if result.Rows[1][0] != int32(3) {
		t.Errorf("second active user should be id=3, got %v", result.Rows[1][0])
	}
}

func TestUpdate(t *testing.T) {
	exec := setupUsersTable(t)
	cols := buildResolvedColumns()
	schema := testTableMeta()

	// Insert a row.
	mustExec(t, exec, &planner.InsertPlan{
		Database: "testdb",
		Table:    "users",
		Schema:   schema,
		Columns:  cols,
		Rows: [][]planner.ResolvedExpr{
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Alice"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "alice@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
		},
	})

	// UPDATE users SET name = 'Alice Updated' WHERE id = 1
	updateResult := mustExec(t, exec, &planner.UpdatePlan{
		Database: "testdb",
		Table:    "users",
		Schema:   schema,
		Assignments: []planner.Assignment{
			{
				Column: cols[1],
				Value:  &planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Alice Updated"},
			},
		},
		Where: &planner.ResolvedComparison{
			Left:  &planner.ResolvedColumnRef{Column: cols[0]},
			Op:    planner.OpEq,
			Right: &planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1},
		},
	})

	if updateResult.RowsAffected != 1 {
		t.Errorf("expected 1 row affected, got %d", updateResult.RowsAffected)
	}

	// Verify the update by selecting.
	resolvedTable := &planner.ResolvedTable{
		Database: "testdb", Table: "users", Binding: "users", Schema: schema,
	}
	selectResult := mustExec(t, exec, &planner.QueryPlan{
		Root: &planner.ProjectNode{
			Input: &planner.ScanNode{Table: resolvedTable},
			Items: []planner.ProjectItem{
				{Expr: &planner.ResolvedColumnRef{Column: cols[1]}, Alias: "name"},
			},
		},
		Columns: []planner.OutputColumn{{Name: "name", Type: ast.TypeVarchar}},
	})
	if len(selectResult.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(selectResult.Rows))
	}
	if selectResult.Rows[0][0] != "Alice Updated" {
		t.Errorf("expected 'Alice Updated', got %v", selectResult.Rows[0][0])
	}
}

func TestDelete(t *testing.T) {
	exec := setupUsersTable(t)
	cols := buildResolvedColumns()
	schema := testTableMeta()

	// Insert 2 rows.
	mustExec(t, exec, &planner.InsertPlan{
		Database: "testdb",
		Table:    "users",
		Schema:   schema,
		Columns:  cols,
		Rows: [][]planner.ResolvedExpr{
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Alice"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "alice@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 2},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Bob"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "bob@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: false},
			},
		},
	})

	// DELETE FROM users WHERE id = 1
	deleteResult := mustExec(t, exec, &planner.DeletePlan{
		Database: "testdb",
		Table:    "users",
		Schema:   schema,
		Where: &planner.ResolvedComparison{
			Left:  &planner.ResolvedColumnRef{Column: cols[0]},
			Op:    planner.OpEq,
			Right: &planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1},
		},
	})

	if deleteResult.RowsAffected != 1 {
		t.Errorf("expected 1 row affected, got %d", deleteResult.RowsAffected)
	}

	// Verify only Bob remains.
	resolvedTable := &planner.ResolvedTable{
		Database: "testdb", Table: "users", Binding: "users", Schema: schema,
	}
	selectResult := mustExec(t, exec, &planner.QueryPlan{
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
	})
	if len(selectResult.Rows) != 1 {
		t.Fatalf("expected 1 remaining row, got %d", len(selectResult.Rows))
	}
	if selectResult.Rows[0][0] != int32(2) {
		t.Errorf("remaining user should be id=2, got %v", selectResult.Rows[0][0])
	}
}

func TestSelectOrderByAndLimit(t *testing.T) {
	exec := setupUsersTable(t)
	cols := buildResolvedColumns()
	schema := testTableMeta()

	// Insert 3 rows.
	mustExec(t, exec, &planner.InsertPlan{
		Database: "testdb",
		Table:    "users",
		Schema:   schema,
		Columns:  cols,
		Rows: [][]planner.ResolvedExpr{
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 3},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Charlie"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "c@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Alice"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "a@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 2},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Bob"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "b@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
		},
	})

	resolvedTable := &planner.ResolvedTable{
		Database: "testdb", Table: "users", Binding: "users", Schema: schema,
	}

	// SELECT id, name FROM users ORDER BY name ASC LIMIT 2
	result := mustExec(t, exec, &planner.QueryPlan{
		Root: &planner.LimitNode{
			Input: &planner.SortNode{
				Input: &planner.ProjectNode{
					Input: &planner.ScanNode{Table: resolvedTable},
					Items: []planner.ProjectItem{
						{Expr: &planner.ResolvedColumnRef{Column: cols[0]}, Alias: "id"},
						{Expr: &planner.ResolvedColumnRef{Column: cols[1]}, Alias: "name"},
					},
				},
				Items: []planner.SortItem{
					{Expr: &planner.ResolvedColumnRef{Column: &planner.ResolvedColumn{
						Database: "testdb", Table: "users", Name: "name", Index: 1, Type: ast.TypeVarchar,
					}}, Direction: ast.OrderAsc},
				},
			},
			Count: 2,
		},
		Columns: []planner.OutputColumn{
			{Name: "id", Type: ast.TypeInt},
			{Name: "name", Type: ast.TypeVarchar},
		},
	})

	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result.Rows))
	}
	// After sorting by name ASC: Alice, Bob, Charlie → LIMIT 2 → Alice, Bob.
	if result.Rows[0][1] != "Alice" {
		t.Errorf("first row name: expected 'Alice', got %v", result.Rows[0][1])
	}
	if result.Rows[1][1] != "Bob" {
		t.Errorf("second row name: expected 'Bob', got %v", result.Rows[1][1])
	}
}

func TestSelectAggregate(t *testing.T) {
	exec := setupUsersTable(t)
	cols := buildResolvedColumns()
	schema := testTableMeta()

	// Insert 3 rows.
	mustExec(t, exec, &planner.InsertPlan{
		Database: "testdb",
		Table:    "users",
		Schema:   schema,
		Columns:  cols,
		Rows: [][]planner.ResolvedExpr{
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Alice"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "a@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 2},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Bob"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "b@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: false},
			},
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 3},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Charlie"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "c@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
		},
	})

	resolvedTable := &planner.ResolvedTable{
		Database: "testdb", Table: "users", Binding: "users", Schema: schema,
	}

	// SELECT COUNT(*) FROM users
	result := mustExec(t, exec, &planner.QueryPlan{
		Root: &planner.AggregateNode{
			Input: &planner.ScanNode{Table: resolvedTable},
			Aggregates: []planner.ProjectItem{
				{
					Expr: &planner.ResolvedFunctionCall{
						ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBigInt},
						Name:             "COUNT",
						Star:             true,
					},
					Alias: "count",
				},
			},
		},
		Columns: []planner.OutputColumn{
			{Name: "count", Type: ast.TypeBigInt},
		},
	})

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	if result.Rows[0][0] != int64(3) {
		t.Errorf("COUNT(*) expected 3, got %v (%T)", result.Rows[0][0], result.Rows[0][0])
	}
}

func TestSelectDistinct(t *testing.T) {
	exec := setupUsersTable(t)
	cols := buildResolvedColumns()
	schema := testTableMeta()

	// Insert rows with duplicate active values.
	mustExec(t, exec, &planner.InsertPlan{
		Database: "testdb",
		Table:    "users",
		Schema:   schema,
		Columns:  cols,
		Rows: [][]planner.ResolvedExpr{
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Alice"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "a@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 2},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Bob"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "b@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 3},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Charlie"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "c@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: false},
			},
		},
	})

	resolvedTable := &planner.ResolvedTable{
		Database: "testdb", Table: "users", Binding: "users", Schema: schema,
	}

	// SELECT DISTINCT active FROM users
	result := mustExec(t, exec, &planner.QueryPlan{
		Root: &planner.DistinctNode{
			Input: &planner.ProjectNode{
				Input: &planner.ScanNode{Table: resolvedTable},
				Items: []planner.ProjectItem{
					{Expr: &planner.ResolvedColumnRef{Column: cols[3]}, Alias: "active"},
				},
			},
		},
		Columns: []planner.OutputColumn{
			{Name: "active", Type: ast.TypeBoolean},
		},
	})

	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 distinct values, got %d", len(result.Rows))
	}
}

func TestSelectFromLessQuery(t *testing.T) {
	exec, _, _ := setupExecutor(t)

	// SELECT 1 + 2 (no FROM clause).
	result := mustExec(t, exec, &planner.QueryPlan{
		Root: &planner.ProjectNode{
			Input: nil, // FROM-less → nil input → single implicit row
			Items: []planner.ProjectItem{
				{
					Expr: &planner.ResolvedBinaryExpr{
						ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt},
						Left:             &planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1},
						Op:               planner.OpAdd,
						Right:            &planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 2},
					},
					Alias: "result",
				},
			},
		},
		Columns: []planner.OutputColumn{{Name: "result", Type: ast.TypeInt}},
	})

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	if result.Rows[0][0] != int32(3) {
		t.Errorf("expected int32(3), got %v (%T)", result.Rows[0][0], result.Rows[0][0])
	}
}

func TestNullHandling(t *testing.T) {
	exec := setupUsersTable(t)
	cols := buildResolvedColumns()
	schema := testTableMeta()

	// Insert with NULL name and email.
	mustExec(t, exec, &planner.InsertPlan{
		Database: "testdb",
		Table:    "users",
		Schema:   schema,
		Columns:  cols,
		Rows: [][]planner.ResolvedExpr{
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1},
				&planner.ResolvedNullLiteral{},
				&planner.ResolvedNullLiteral{},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
		},
	})

	resolvedTable := &planner.ResolvedTable{
		Database: "testdb", Table: "users", Binding: "users", Schema: schema,
	}

	// SELECT name IS NULL FROM users
	result := mustExec(t, exec, &planner.QueryPlan{
		Root: &planner.ProjectNode{
			Input: &planner.ScanNode{Table: resolvedTable},
			Items: []planner.ProjectItem{
				{Expr: &planner.ResolvedColumnRef{Column: cols[1]}, Alias: "name"},
			},
		},
		Columns: []planner.OutputColumn{{Name: "name", Type: ast.TypeVarchar}},
	})

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	if result.Rows[0][0] != nil {
		t.Errorf("expected nil (NULL), got %v", result.Rows[0][0])
	}
}

func TestDuplicateKeyError(t *testing.T) {
	exec := setupUsersTable(t)
	cols := buildResolvedColumns()
	schema := testTableMeta()

	insertPlan := &planner.InsertPlan{
		Database: "testdb",
		Table:    "users",
		Schema:   schema,
		Columns:  cols,
		Rows: [][]planner.ResolvedExpr{
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Alice"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "a@test.com"},
				&planner.ResolvedBoolLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeBoolean}, Value: true},
			},
		},
	}

	mustExec(t, exec, insertPlan)

	// Insert again with same PK should fail.
	_, err := exec.Execute(context.Background(), insertPlan)
	if err == nil {
		t.Fatal("expected duplicate key error")
	}
}

func TestNotNullViolation(t *testing.T) {
	exec := setupUsersTable(t)
	cols := buildResolvedColumns()
	schema := testTableMeta()

	// Try to insert NULL into NOT NULL column 'active'.
	_, err := exec.Execute(context.Background(), &planner.InsertPlan{
		Database: "testdb",
		Table:    "users",
		Schema:   schema,
		Columns:  cols,
		Rows: [][]planner.ResolvedExpr{
			{
				&planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: 1},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeVarchar}, Value: "Alice"},
				&planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: "a@test.com"},
				&planner.ResolvedNullLiteral{}, // NULL for NOT NULL column
			},
		},
	})
	if err == nil {
		t.Fatal("expected NOT NULL violation error")
	}
}
