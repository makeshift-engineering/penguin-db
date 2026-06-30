package catalog

import (
	"context"
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/kv"
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

// mockKV is a minimal in-memory KV implementation used only by
// NewCatalog bootstrap tests. It is the only place in the catalog
// test suite that depends on the kv package.
type mockKV struct {
	entries []mockEntry
}

type mockEntry struct {
	key   []byte
	value []byte
}

func newMockKV() *mockKV { return &mockKV{} }

func (m *mockKV) Get(_ context.Context, key []byte) ([]byte, error) {
	for _, e := range m.entries {
		if bytesEqual(e.key, key) {
			return e.value, nil
		}
	}
	return nil, kv.ErrKeyNotFound
}

func (m *mockKV) Put(_ context.Context, key, value []byte) error {
	for i, e := range m.entries {
		if bytesEqual(e.key, key) {
			m.entries[i].value = value
			return nil
		}
	}
	m.entries = append(m.entries, mockEntry{key: dupBytes(key), value: dupBytes(value)})
	return nil
}

func (m *mockKV) Delete(_ context.Context, key []byte) error {
	for i, e := range m.entries {
		if bytesEqual(e.key, key) {
			m.entries = append(m.entries[:i], m.entries[i+1:]...)
			return nil
		}
	}
	return kv.ErrKeyNotFound
}

func (m *mockKV) Scan(_ context.Context, prefix []byte) (kv.Iterator, error) {
	var matches []mockEntry
	for _, e := range m.entries {
		if hasPrefix(e.key, prefix) {
			matches = append(matches, e)
		}
	}
	return &mockIterator{entries: matches, pos: -1}, nil
}

func (m *mockKV) WriteBatch(_ context.Context, ops []kv.Op) error {
	ctx := context.Background()
	for _, op := range ops {
		switch op.Type {
		case kv.OpPut:
			if err := m.Put(ctx, op.Key, op.Value); err != nil {
				return err
			}
		case kv.OpDelete:
			_ = m.Delete(ctx, op.Key)
		}
	}
	return nil
}

type mockIterator struct {
	entries []mockEntry
	pos     int
}

func (it *mockIterator) Valid() bool {
	return it.pos+1 < len(it.entries)
}

func (it *mockIterator) Next() ([]byte, []byte) {
	it.pos++
	if it.pos >= len(it.entries) {
		return nil, nil
	}
	return it.entries[it.pos].key, it.entries[it.pos].value
}

func (it *mockIterator) Close() {}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func hasPrefix(data, prefix []byte) bool {
	if len(data) < len(prefix) {
		return false
	}
	for i := range prefix {
		if data[i] != prefix[i] {
			return false
		}
	}
	return true
}

func dupBytes(b []byte) []byte {
	c := make([]byte, len(b))
	copy(c, b)
	return c
}

// TestNewCatalog_EmptyKV verifies that NewCatalog succeeds when bootstrapping
// from an empty KV store and produces a catalog with no databases.
func TestNewCatalog_EmptyKV(t *testing.T) {
	store := newMockKV()
	c, err := NewCatalog(context.Background(), store)
	if err != nil {
		t.Fatalf("NewCatalog on empty KV: %v", err)
	}
	if c.DatabaseExists("anything") {
		t.Error("empty catalog should not contain any databases")
	}
}

// TestNewCatalog_BootstrapFromExistingData verifies that NewCatalog correctly
// reconstructs the in-memory catalog state from databases and tables that were
// previously persisted to the KV store.
func TestNewCatalog_BootstrapFromExistingData(t *testing.T) {
	store := newMockKV()
	ctx := context.Background()

	// Persist a database and table via Build*Ops + WriteBatch.
	dbMeta := testDB()
	dbOps, err := BuildCreateDatabaseOps(dbMeta)
	if err != nil {
		t.Fatalf("BuildCreateDatabaseOps: %v", err)
	}
	if err := store.WriteBatch(ctx, dbOps); err != nil {
		t.Fatalf("WriteBatch (db): %v", err)
	}

	tempCat := NewEmptyCatalog()
	tempCat.ApplyCreateDatabase(dbMeta)

	tblMeta := testTable()
	tblOps, err := BuildCreateTableOps(tempCat, tblMeta)
	if err != nil {
		t.Fatalf("BuildCreateTableOps: %v", err)
	}
	if err := store.WriteBatch(ctx, tblOps); err != nil {
		t.Fatalf("WriteBatch (table): %v", err)
	}

	// Bootstrap a fresh catalog from the KV store.
	c, err := NewCatalog(ctx, store)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}

	if !c.DatabaseExists("testdb") {
		t.Error("bootstrapped catalog should contain 'testdb'")
	}

	got, err := c.GetTable("testdb", "users")
	if err != nil {
		t.Fatalf("GetTable: %v", err)
	}
	if got.Name != "users" {
		t.Errorf("expected table name 'users', got %q", got.Name)
	}
	if len(got.Columns) != 4 {
		t.Errorf("expected 4 columns, got %d", len(got.Columns))
	}
}

// TestNewEmptyCatalog verifies that a freshly created empty catalog reports
// no databases and returns an empty database list.
func TestNewEmptyCatalog(t *testing.T) {
	c := NewEmptyCatalog()
	if c.DatabaseExists("any") {
		t.Error("empty catalog should not contain any databases")
	}
	dbs := c.ListDatabases()
	if len(dbs) != 0 {
		t.Errorf("expected 0 databases, got %d", len(dbs))
	}
}

// TestDatabaseExists verifies that DatabaseExists returns false before creation
// and true after a database has been added to the
func TestDatabaseExists(t *testing.T) {
	c := NewEmptyCatalog()
	if c.DatabaseExists("testdb") {
		t.Error("should not exist before creation")
	}
	c.ApplyCreateDatabase(testDB())
	if !c.DatabaseExists("testdb") {
		t.Error("should exist after creation")
	}
}

// TestGetDatabase verifies that GetDatabase returns ErrDatabaseNotFound for a
// missing database and returns the correct metadata after creation.
func TestGetDatabase(t *testing.T) {
	c := NewEmptyCatalog()

	_, err := c.GetDatabase("nonexistent")
	if err != ErrDatabaseNotFound {
		t.Errorf("expected ErrDatabaseNotFound, got %v", err)
	}

	c.ApplyCreateDatabase(testDB())
	got, err := c.GetDatabase("testdb")
	if err != nil {
		t.Fatalf("GetDatabase: %v", err)
	}
	if got.Name != "testdb" {
		t.Errorf("expected 'testdb', got %q", got.Name)
	}
}

// TestGetTable verifies that GetTable returns ErrTableNotFound when neither the
// database nor the table exist, and returns correct metadata after creation.
func TestGetTable(t *testing.T) {
	c := NewEmptyCatalog()

	_, err := c.GetTable("nodb", "notable")
	if err != ErrTableNotFound {
		t.Errorf("expected ErrTableNotFound, got %v", err)
	}

	c.ApplyCreateDatabase(testDB())
	c.ApplyCreateTable(testTable())

	got, err := c.GetTable("testdb", "users")
	if err != nil {
		t.Fatalf("GetTable: %v", err)
	}
	if got.Name != "users" {
		t.Errorf("expected 'users', got %q", got.Name)
	}
}

// TestListTables verifies that ListTables returns ErrDatabaseNotFound for a
// missing database and returns all tables after they have been added.
func TestListTables(t *testing.T) {
	c := NewEmptyCatalog()

	_, err := c.ListTables("nodb")
	if err != ErrDatabaseNotFound {
		t.Errorf("expected ErrDatabaseNotFound, got %v", err)
	}

	c.ApplyCreateDatabase(testDB())
	c.ApplyCreateTable(testTable())
	c.ApplyCreateTable(&TableMeta{
		Database: "testdb",
		Name:     "orders",
		Columns:  []ColumnMeta{{Name: "id", Type: ast.TypeInt}},
		Version:  1,
	})

	tables, err := c.ListTables("testdb")
	if err != nil {
		t.Fatalf("ListTables: %v", err)
	}
	if len(tables) != 2 {
		t.Errorf("expected 2 tables, got %d", len(tables))
	}
}

// TestListDatabases verifies that ListDatabases returns all databases that
// have been added to the
func TestListDatabases(t *testing.T) {
	c := NewEmptyCatalog()
	c.ApplyCreateDatabase(&DatabaseMeta{Name: "db1"})
	c.ApplyCreateDatabase(&DatabaseMeta{Name: "db2"})

	dbs := c.ListDatabases()
	if len(dbs) != 2 {
		t.Fatalf("expected 2 databases, got %d", len(dbs))
	}

	names := make(map[string]bool)
	for _, db := range dbs {
		names[db.Name] = true
	}
	if !names["db1"] || !names["db2"] {
		t.Error("expected both db1 and db2")
	}
}

// TestResolveColumn verifies that ResolveColumn returns the correct column
// metadata (name and type) for an existing column.
func TestResolveColumn(t *testing.T) {
	c := NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())
	c.ApplyCreateTable(testTable())

	col, err := c.ResolveColumn("testdb", "users", "name")
	if err != nil {
		t.Fatalf("ResolveColumn: %v", err)
	}
	if col.Name != "name" {
		t.Errorf("expected 'name', got %q", col.Name)
	}
	if col.Type != ast.TypeVarchar {
		t.Errorf("expected TypeVarchar, got %d", col.Type)
	}
}

// TestResolveColumn_NotFound verifies that ResolveColumn returns
// ErrColumnNotFound when the requested column does not exist in the table.
func TestResolveColumn_NotFound(t *testing.T) {
	c := NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())
	c.ApplyCreateTable(testTable())

	_, err := c.ResolveColumn("testdb", "users", "nonexistent")
	if err != ErrColumnNotFound {
		t.Errorf("expected ErrColumnNotFound, got %v", err)
	}
}

// TestResolveColumn_DroppedColumn verifies that ResolveColumn returns
// ErrColumnNotFound when the requested column exists but has been marked as dropped.
func TestResolveColumn_DroppedColumn(t *testing.T) {
	c := NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())

	meta := testTable()
	meta.Columns[1].Dropped = true
	c.ApplyCreateTable(meta)

	_, err := c.ResolveColumn("testdb", "users", "name")
	if err != ErrColumnNotFound {
		t.Errorf("expected ErrColumnNotFound for dropped column, got %v", err)
	}
}

// TestResolveColumn_NoTable verifies that ResolveColumn returns
// ErrTableNotFound when neither the database nor the table exist.
func TestResolveColumn_NoTable(t *testing.T) {
	c := NewEmptyCatalog()
	_, err := c.ResolveColumn("nodb", "notable", "nocol")
	if err != ErrTableNotFound {
		t.Errorf("expected ErrTableNotFound, got %v", err)
	}
}

// TestPKColumnTypes verifies that PKColumnTypes returns the correct column
// types for a table with a single-column primary key.
func TestPKColumnTypes(t *testing.T) {
	c := NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())
	c.ApplyCreateTable(testTable())

	types, err := c.PKColumnTypes("testdb", "users")
	if err != nil {
		t.Fatalf("PKColumnTypes: %v", err)
	}
	if len(types) != 1 || types[0] != ast.TypeInt {
		t.Errorf("expected [TypeInt], got %v", types)
	}
}

// TestPKColumnTypes_SnowflakeID verifies that PKColumnTypes returns TypeBigInt
// for tables that use a snowflake ID as their primary key.
func TestPKColumnTypes_SnowflakeID(t *testing.T) {
	c := NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())
	c.ApplyCreateTable(&TableMeta{
		Database:       "testdb",
		Name:           "events",
		Columns:        []ColumnMeta{{Name: "data", Type: ast.TypeText}},
		HasSnowflakeID: true,
		Version:        1,
	})

	types, err := c.PKColumnTypes("testdb", "events")
	if err != nil {
		t.Fatalf("PKColumnTypes: %v", err)
	}
	if len(types) != 1 || types[0] != ast.TypeBigInt {
		t.Errorf("expected [TypeBigInt] for snowflake, got %v", types)
	}
}

// TestPKColumnTypes_CompositePK verifies that PKColumnTypes returns the
// correct ordered list of types for a table with a composite primary key.
func TestPKColumnTypes_CompositePK(t *testing.T) {
	c := NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())
	c.ApplyCreateTable(&TableMeta{
		Database: "testdb",
		Name:     "composite",
		Columns: []ColumnMeta{
			{Name: "tenant", Type: ast.TypeVarchar, VarcharLen: intPtr(100)},
			{Name: "id", Type: ast.TypeBigInt},
			{Name: "data", Type: ast.TypeText},
		},
		PrimaryKey: []string{"tenant", "id"},
		Version:    1,
	})

	types, err := c.PKColumnTypes("testdb", "composite")
	if err != nil {
		t.Fatalf("PKColumnTypes: %v", err)
	}
	if len(types) != 2 || types[0] != ast.TypeVarchar || types[1] != ast.TypeBigInt {
		t.Errorf("expected [TypeVarchar, TypeBigInt], got %v", types)
	}
}

// TestPKColumnTypes_NotFound verifies that PKColumnTypes returns
// ErrTableNotFound when the requested table does not exist.
func TestPKColumnTypes_NotFound(t *testing.T) {
	c := NewEmptyCatalog()
	_, err := c.PKColumnTypes("nodb", "notable")
	if err != ErrTableNotFound {
		t.Errorf("expected ErrTableNotFound, got %v", err)
	}
}

// TestCatalog_ConcurrentReadsDuringWrite exercises the catalog under concurrent
// read and write pressure to verify that the RWMutex-based synchronization does
// not produce data races or panics.
func TestCatalog_ConcurrentReadsDuringWrite(t *testing.T) {
	c := NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())
	c.ApplyCreateTable(testTable())

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 1000 {
			_, _ = c.GetTable("testdb", "users")
			_ = c.DatabaseExists("testdb")
			_, _ = c.ListTables("testdb")
		}
	}()

	for i := range 100 {
		meta := &TableMeta{
			Database: "testdb",
			Name:     "table_" + string(rune('A'+i%26)),
			Columns:  []ColumnMeta{{Name: "id", Type: ast.TypeInt}},
			Version:  1,
		}
		c.ApplyCreateTable(meta)
	}

	<-done
}
