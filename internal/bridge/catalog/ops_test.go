package catalog_test

import (
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
	"github.com/makeshift-engineering/penguin-db/internal/bridge/kv"
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

// ---------- BuildCreateDatabaseOps ----------

func TestBuildCreateDatabaseOps(t *testing.T) {
	meta := testDB()
	ops, err := catalog.BuildCreateDatabaseOps(meta)
	if err != nil {
		t.Fatalf("BuildCreateDatabaseOps: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	if ops[0].Type != kv.OpPut {
		t.Errorf("expected OpPut, got %d", ops[0].Type)
	}
}

// ---------- BuildDropDatabaseOps ----------

func TestBuildDropDatabaseOps(t *testing.T) {
	c := catalog.NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())
	c.ApplyCreateTable(testTable())

	tables, err := c.ListTables("testdb")
	if err != nil {
		t.Fatalf("ListTables: %v", err)
	}

	ops, err := catalog.BuildDropDatabaseOps("testdb", tables)
	if err != nil {
		t.Fatalf("BuildDropDatabaseOps: %v", err)
	}
	// 1 delete for DB + 1 delete for table key + 1 delete for seq key.
	if len(ops) != 3 {
		t.Fatalf("expected 3 ops, got %d", len(ops))
	}
}

func TestBuildDropDatabaseOps_NoTables(t *testing.T) {
	ops, err := catalog.BuildDropDatabaseOps("emptydb", nil)
	if err != nil {
		t.Fatalf("BuildDropDatabaseOps: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("expected 1 op (db delete only), got %d", len(ops))
	}
	if ops[0].Type != kv.OpDelete {
		t.Errorf("expected OpDelete, got %d", ops[0].Type)
	}
}

// ---------- BuildCreateTableOps ----------

func TestBuildCreateTableOps(t *testing.T) {
	c := catalog.NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())

	ops, err := catalog.BuildCreateTableOps(c, testTable())
	if err != nil {
		t.Fatalf("BuildCreateTableOps: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	if ops[0].Type != kv.OpPut {
		t.Errorf("expected OpPut, got %d", ops[0].Type)
	}
}

func TestBuildCreateTableOps_NoDB(t *testing.T) {
	c := catalog.NewEmptyCatalog()
	_, err := catalog.BuildCreateTableOps(c, testTable())
	if err != catalog.ErrCatalogDBMissing {
		t.Errorf("expected ErrCatalogDBMissing, got %v", err)
	}
}

func TestBuildCreateTableOps_DuplicateTable(t *testing.T) {
	c := catalog.NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())
	c.ApplyCreateTable(testTable())

	_, err := catalog.BuildCreateTableOps(c, testTable())
	if err != catalog.ErrTableExists {
		t.Errorf("expected ErrTableExists, got %v", err)
	}
}

func TestBuildCreateTableOps_DuplicateColumns(t *testing.T) {
	c := catalog.NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())

	meta := &catalog.TableMeta{
		Database: "testdb",
		Name:     "bad",
		Columns: []catalog.ColumnMeta{
			{Name: "id", Type: ast.TypeInt},
			{Name: "id", Type: ast.TypeVarchar},
		},
		PrimaryKey: []string{"id"},
		Version:    1,
	}

	_, err := catalog.BuildCreateTableOps(c, meta)
	if err != catalog.ErrDuplicateColumn {
		t.Errorf("expected ErrDuplicateColumn, got %v", err)
	}
}

func TestBuildCreateTableOps_PKColumnNotFound(t *testing.T) {
	c := catalog.NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())

	meta := &catalog.TableMeta{
		Database:   "testdb",
		Name:       "bad",
		Columns:    []catalog.ColumnMeta{{Name: "id", Type: ast.TypeInt}},
		PrimaryKey: []string{"nonexistent"},
		Version:    1,
	}

	_, err := catalog.BuildCreateTableOps(c, meta)
	if err != catalog.ErrPKColumnNotFound {
		t.Errorf("expected ErrPKColumnNotFound, got %v", err)
	}
}

// ---------- BuildDropTableOps ----------

func TestBuildDropTableOps(t *testing.T) {
	ops, err := catalog.BuildDropTableOps("testdb", "users")
	if err != nil {
		t.Fatalf("BuildDropTableOps: %v", err)
	}
	if len(ops) != 2 {
		t.Fatalf("expected 2 ops (table + seq), got %d", len(ops))
	}
	if ops[0].Type != kv.OpDelete || ops[1].Type != kv.OpDelete {
		t.Error("expected both ops to be OpDelete")
	}
}

// ---------- BuildAlterTableOps ----------

func TestBuildAlterTableOps_AddNullableColumn(t *testing.T) {
	old := testTable()
	newMeta := &catalog.TableMeta{
		Database:   "testdb",
		Name:       "users",
		PrimaryKey: []string{"id"},
		Columns: append(append([]catalog.ColumnMeta{}, old.Columns...),
			catalog.ColumnMeta{Name: "age", Type: ast.TypeInt},
		),
		Version: old.Version,
	}

	ops, err := catalog.BuildAlterTableOps(old, newMeta)
	if err != nil {
		t.Fatalf("BuildAlterTableOps: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	if newMeta.Version != old.Version+1 {
		t.Errorf("expected version %d, got %d", old.Version+1, newMeta.Version)
	}
}

func TestBuildAlterTableOps_AddNotNullWithDefault(t *testing.T) {
	old := testTable()
	newMeta := &catalog.TableMeta{
		Database:   "testdb",
		Name:       "users",
		PrimaryKey: []string{"id"},
		Columns: append(append([]catalog.ColumnMeta{}, old.Columns...),
			catalog.ColumnMeta{
				Name:         "role",
				Type:         ast.TypeVarchar,
				VarcharLen:   intPtr(50),
				NotNull:      true,
				DefaultValue: strPtr("'user'"),
			},
		),
		Version: old.Version,
	}

	_, err := catalog.BuildAlterTableOps(old, newMeta)
	if err != nil {
		t.Fatalf("expected success for NOT NULL with DEFAULT: %v", err)
	}
}

func TestBuildAlterTableOps_AddNotNullWithoutDefault_Rejected(t *testing.T) {
	old := testTable()
	newMeta := &catalog.TableMeta{
		Database:   "testdb",
		Name:       "users",
		PrimaryKey: []string{"id"},
		Columns: append(append([]catalog.ColumnMeta{}, old.Columns...),
			catalog.ColumnMeta{Name: "required", Type: ast.TypeInt, NotNull: true},
		),
		Version: old.Version,
	}

	_, err := catalog.BuildAlterTableOps(old, newMeta)
	if err != catalog.ErrUnsupportedAlter {
		t.Errorf("expected ErrUnsupportedAlter, got %v", err)
	}
}

func TestBuildAlterTableOps_DropColumn(t *testing.T) {
	old := testTable()
	newCols := make([]catalog.ColumnMeta, len(old.Columns))
	copy(newCols, old.Columns)
	newCols[2].Dropped = true // Drop "email"

	newMeta := &catalog.TableMeta{
		Database:   "testdb",
		Name:       "users",
		PrimaryKey: []string{"id"},
		Columns:    newCols,
		Version:    old.Version,
	}

	_, err := catalog.BuildAlterTableOps(old, newMeta)
	if err != nil {
		t.Fatalf("expected drop column to succeed: %v", err)
	}
}

func TestBuildAlterTableOps_DropPKColumn_Rejected(t *testing.T) {
	old := testTable()
	newCols := make([]catalog.ColumnMeta, len(old.Columns))
	copy(newCols, old.Columns)
	newCols[0].Dropped = true // Drop "id" — the PK

	newMeta := &catalog.TableMeta{
		Database:   "testdb",
		Name:       "users",
		PrimaryKey: []string{"id"},
		Columns:    newCols,
		Version:    old.Version,
	}

	_, err := catalog.BuildAlterTableOps(old, newMeta)
	if err != catalog.ErrCannotDropPKColumn {
		t.Errorf("expected ErrCannotDropPKColumn, got %v", err)
	}
}

func TestBuildAlterTableOps_ChangeColumnType_Rejected(t *testing.T) {
	old := testTable()
	newCols := make([]catalog.ColumnMeta, len(old.Columns))
	copy(newCols, old.Columns)
	newCols[1].Type = ast.TypeInt // VARCHAR → INT

	newMeta := &catalog.TableMeta{
		Database:   "testdb",
		Name:       "users",
		PrimaryKey: []string{"id"},
		Columns:    newCols,
		Version:    old.Version,
	}

	_, err := catalog.BuildAlterTableOps(old, newMeta)
	if err != catalog.ErrUnsupportedAlter {
		t.Errorf("expected ErrUnsupportedAlter, got %v", err)
	}
}

func TestBuildAlterTableOps_ChangePK_Rejected(t *testing.T) {
	old := testTable()
	newMeta := &catalog.TableMeta{
		Database:   "testdb",
		Name:       "users",
		PrimaryKey: []string{"id", "name"},
		Columns:    old.Columns,
		Version:    old.Version,
	}

	_, err := catalog.BuildAlterTableOps(old, newMeta)
	if err != catalog.ErrUnsupportedAlter {
		t.Errorf("expected ErrUnsupportedAlter, got %v", err)
	}
}

// ---------- BuildRenameTableOps ----------

func TestBuildRenameTableOps(t *testing.T) {
	meta := testTable()
	metaCopy := *meta

	ops, err := catalog.BuildRenameTableOps("testdb", "users", "customers", &metaCopy)
	if err != nil {
		t.Fatalf("BuildRenameTableOps: %v", err)
	}
	if len(ops) != 2 {
		t.Fatalf("expected 2 ops (delete + put), got %d", len(ops))
	}
	if ops[0].Type != kv.OpDelete {
		t.Errorf("first op should be OpDelete, got %d", ops[0].Type)
	}
	if ops[1].Type != kv.OpPut {
		t.Errorf("second op should be OpPut, got %d", ops[1].Type)
	}
	if metaCopy.Name != "customers" {
		t.Errorf("expected meta.Name updated to 'customers', got %q", metaCopy.Name)
	}
}
