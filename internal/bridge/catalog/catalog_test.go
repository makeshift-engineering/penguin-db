package catalog_test

import (
	"context"
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
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

// ---------- NewCatalog bootstrap ----------

func TestNewCatalog_EmptyKV(t *testing.T) {
	store := newMockKV()
	c, err := catalog.NewCatalog(context.Background(), store)
	if err != nil {
		t.Fatalf("NewCatalog on empty KV: %v", err)
	}
	if c.DatabaseExists("anything") {
		t.Error("empty catalog should not contain any databases")
	}
}

func TestNewCatalog_BootstrapFromExistingData(t *testing.T) {
	store := newMockKV()
	ctx := context.Background()

	// Persist a database and table via Build*Ops + WriteBatch.
	dbMeta := testDB()
	dbOps, err := catalog.BuildCreateDatabaseOps(dbMeta)
	if err != nil {
		t.Fatalf("BuildCreateDatabaseOps: %v", err)
	}
	if err := store.WriteBatch(ctx, dbOps); err != nil {
		t.Fatalf("WriteBatch (db): %v", err)
	}

	tempCat := catalog.NewEmptyCatalog()
	tempCat.ApplyCreateDatabase(dbMeta)

	tblMeta := testTable()
	tblOps, err := catalog.BuildCreateTableOps(tempCat, tblMeta)
	if err != nil {
		t.Fatalf("BuildCreateTableOps: %v", err)
	}
	if err := store.WriteBatch(ctx, tblOps); err != nil {
		t.Fatalf("WriteBatch (table): %v", err)
	}

	// Bootstrap a fresh catalog from the KV store.
	c, err := catalog.NewCatalog(ctx, store)
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

// ---------- NewEmptyCatalog ----------

func TestNewEmptyCatalog(t *testing.T) {
	c := catalog.NewEmptyCatalog()
	if c.DatabaseExists("any") {
		t.Error("empty catalog should not contain any databases")
	}
	dbs := c.ListDatabases()
	if len(dbs) != 0 {
		t.Errorf("expected 0 databases, got %d", len(dbs))
	}
}

// ---------- Read methods ----------

func TestDatabaseExists(t *testing.T) {
	c := catalog.NewEmptyCatalog()
	if c.DatabaseExists("testdb") {
		t.Error("should not exist before creation")
	}
	c.ApplyCreateDatabase(testDB())
	if !c.DatabaseExists("testdb") {
		t.Error("should exist after creation")
	}
}

func TestGetDatabase(t *testing.T) {
	c := catalog.NewEmptyCatalog()

	_, err := c.GetDatabase("nonexistent")
	if err != catalog.ErrDatabaseNotFound {
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

func TestGetTable(t *testing.T) {
	c := catalog.NewEmptyCatalog()

	_, err := c.GetTable("nodb", "notable")
	if err != catalog.ErrTableNotFound {
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

func TestListTables(t *testing.T) {
	c := catalog.NewEmptyCatalog()

	_, err := c.ListTables("nodb")
	if err != catalog.ErrDatabaseNotFound {
		t.Errorf("expected ErrDatabaseNotFound, got %v", err)
	}

	c.ApplyCreateDatabase(testDB())
	c.ApplyCreateTable(testTable())
	c.ApplyCreateTable(&catalog.TableMeta{
		Database: "testdb",
		Name:     "orders",
		Columns:  []catalog.ColumnMeta{{Name: "id", Type: ast.TypeInt}},
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

func TestListDatabases(t *testing.T) {
	c := catalog.NewEmptyCatalog()
	c.ApplyCreateDatabase(&catalog.DatabaseMeta{Name: "db1"})
	c.ApplyCreateDatabase(&catalog.DatabaseMeta{Name: "db2"})

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

func TestResolveColumn(t *testing.T) {
	c := catalog.NewEmptyCatalog()
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

func TestResolveColumn_NotFound(t *testing.T) {
	c := catalog.NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())
	c.ApplyCreateTable(testTable())

	_, err := c.ResolveColumn("testdb", "users", "nonexistent")
	if err != catalog.ErrColumnNotFound {
		t.Errorf("expected ErrColumnNotFound, got %v", err)
	}
}

func TestResolveColumn_DroppedColumn(t *testing.T) {
	c := catalog.NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())

	meta := testTable()
	meta.Columns[1].Dropped = true
	c.ApplyCreateTable(meta)

	_, err := c.ResolveColumn("testdb", "users", "name")
	if err != catalog.ErrColumnNotFound {
		t.Errorf("expected ErrColumnNotFound for dropped column, got %v", err)
	}
}

func TestResolveColumn_NoTable(t *testing.T) {
	c := catalog.NewEmptyCatalog()
	_, err := c.ResolveColumn("nodb", "notable", "nocol")
	if err != catalog.ErrTableNotFound {
		t.Errorf("expected ErrTableNotFound, got %v", err)
	}
}

func TestPKColumnTypes(t *testing.T) {
	c := catalog.NewEmptyCatalog()
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

func TestPKColumnTypes_SnowflakeID(t *testing.T) {
	c := catalog.NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())
	c.ApplyCreateTable(&catalog.TableMeta{
		Database:       "testdb",
		Name:           "events",
		Columns:        []catalog.ColumnMeta{{Name: "data", Type: ast.TypeText}},
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

func TestPKColumnTypes_CompositePK(t *testing.T) {
	c := catalog.NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())
	c.ApplyCreateTable(&catalog.TableMeta{
		Database: "testdb",
		Name:     "composite",
		Columns: []catalog.ColumnMeta{
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

func TestPKColumnTypes_NotFound(t *testing.T) {
	c := catalog.NewEmptyCatalog()
	_, err := c.PKColumnTypes("nodb", "notable")
	if err != catalog.ErrTableNotFound {
		t.Errorf("expected ErrTableNotFound, got %v", err)
	}
}

// ---------- Concurrent access ----------

func TestCatalog_ConcurrentReadsDuringWrite(t *testing.T) {
	c := catalog.NewEmptyCatalog()
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
		meta := &catalog.TableMeta{
			Database: "testdb",
			Name:     "table_" + string(rune('A'+i%26)),
			Columns:  []catalog.ColumnMeta{{Name: "id", Type: ast.TypeInt}},
			Version:  1,
		}
		c.ApplyCreateTable(meta)
	}

	<-done
}
