package catalog_test

import (
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

func TestApplyCreateDatabase(t *testing.T) {
	c := catalog.NewEmptyCatalog()
	meta := testDB()

	c.ApplyCreateDatabase(meta)

	if !c.DatabaseExists("testdb") {
		t.Error("database should exist after ApplyCreateDatabase")
	}
}

func TestApplyCreateDatabase_InitializesTableMap(t *testing.T) {
	c := catalog.NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())

	// ListTables should succeed (not panic) even with no tables.
	tables, err := c.ListTables("testdb")
	if err != nil {
		t.Fatalf("ListTables: %v", err)
	}
	if len(tables) != 0 {
		t.Errorf("expected 0 tables, got %d", len(tables))
	}
}

func TestApplyDropDatabase(t *testing.T) {
	c := catalog.NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())
	c.ApplyCreateTable(testTable())

	c.ApplyDropDatabase("testdb")

	if c.DatabaseExists("testdb") {
		t.Error("database should not exist after drop")
	}
	// Tables should also be gone.
	_, err := c.GetTable("testdb", "users")
	if err != catalog.ErrTableNotFound {
		t.Errorf("expected ErrTableNotFound, got %v", err)
	}
}

func TestApplyDropDatabase_NonExistent(t *testing.T) {
	c := catalog.NewEmptyCatalog()
	// Should not panic on a non-existent database.
	c.ApplyDropDatabase("nonexistent")
}

func TestApplyCreateTable(t *testing.T) {
	c := catalog.NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())

	meta := testTable()
	c.ApplyCreateTable(meta)

	got, err := c.GetTable("testdb", "users")
	if err != nil {
		t.Fatalf("GetTable: %v", err)
	}
	if got.Name != "users" {
		t.Errorf("expected 'users', got %q", got.Name)
	}
	if got.Version != 1 {
		t.Errorf("expected version 1, got %d", got.Version)
	}
}

func TestApplyCreateTable_WithoutPriorDatabase(t *testing.T) {
	c := catalog.NewEmptyCatalog()
	// ApplyCreateTable should lazily create the database table map.
	meta := testTable()
	c.ApplyCreateTable(meta)

	got, err := c.GetTable("testdb", "users")
	if err != nil {
		t.Fatalf("GetTable: %v", err)
	}
	if got.Name != "users" {
		t.Errorf("expected 'users', got %q", got.Name)
	}
}

func TestApplyDropTable(t *testing.T) {
	c := catalog.NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())
	c.ApplyCreateTable(testTable())

	c.ApplyDropTable("testdb", "users")

	_, err := c.GetTable("testdb", "users")
	if err != catalog.ErrTableNotFound {
		t.Errorf("expected ErrTableNotFound, got %v", err)
	}
}

func TestApplyDropTable_NonExistent(t *testing.T) {
	c := catalog.NewEmptyCatalog()
	// Should not panic.
	c.ApplyDropTable("nodb", "notable")
}

func TestApplyAlterTable(t *testing.T) {
	c := catalog.NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())
	c.ApplyCreateTable(testTable())

	newMeta := &catalog.TableMeta{
		Database:   "testdb",
		Name:       "users",
		PrimaryKey: []string{"id"},
		Columns: []catalog.ColumnMeta{
			{Name: "id", Type: ast.TypeInt, NotNull: true, PrimaryKey: true},
			{Name: "name", Type: ast.TypeVarchar, VarcharLen: intPtr(255)},
			{Name: "email", Type: ast.TypeText},
			{Name: "active", Type: ast.TypeBoolean, NotNull: true, DefaultValue: strPtr("true")},
			{Name: "age", Type: ast.TypeInt},
		},
		Version: 2,
	}
	c.ApplyAlterTable(newMeta)

	got, err := c.GetTable("testdb", "users")
	if err != nil {
		t.Fatalf("GetTable: %v", err)
	}
	if got.Version != 2 {
		t.Errorf("expected version 2, got %d", got.Version)
	}
	if len(got.Columns) != 5 {
		t.Errorf("expected 5 columns, got %d", len(got.Columns))
	}
}

func TestApplyRenameTable(t *testing.T) {
	c := catalog.NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())
	c.ApplyCreateTable(testTable())

	meta, _ := c.GetTable("testdb", "users")
	renamed := *meta
	renamed.Name = "customers"

	c.ApplyRenameTable("testdb", "users", "customers", &renamed)

	_, err := c.GetTable("testdb", "users")
	if err != catalog.ErrTableNotFound {
		t.Errorf("old name should not exist after rename")
	}
	got, err := c.GetTable("testdb", "customers")
	if err != nil {
		t.Fatalf("new name should exist: %v", err)
	}
	if got.Name != "customers" {
		t.Errorf("expected 'customers', got %q", got.Name)
	}
}

func TestApplyRenameTable_NonExistentOldName(t *testing.T) {
	c := catalog.NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())

	meta := &catalog.TableMeta{Database: "testdb", Name: "new", Version: 1}
	// Should not panic when old name doesn't exist.
	c.ApplyRenameTable("testdb", "nonexistent", "new", meta)
}
