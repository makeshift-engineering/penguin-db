package catalog

import (
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

// TestApplyCreateDatabase verifies that a database becomes visible in the catalog
// after calling ApplyCreateDatabase with valid metadata.
func TestApplyCreateDatabase(t *testing.T) {
	c := NewEmptyCatalog()
	meta := testDB()

	c.ApplyCreateDatabase(meta)

	if !c.DatabaseExists("testdb") {
		t.Error("database should exist after ApplyCreateDatabase")
	}
}

// TestApplyCreateDatabase_InitializesTableMap verifies that ApplyCreateDatabase
// initializes the internal table map so that ListTables succeeds without panicking,
// even when no tables have been added yet.
func TestApplyCreateDatabase_InitializesTableMap(t *testing.T) {
	c := NewEmptyCatalog()
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

// TestApplyDropDatabase verifies that dropping a database removes it from the
// catalog and also removes all tables that belonged to it.
func TestApplyDropDatabase(t *testing.T) {
	c := NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())
	c.ApplyCreateTable(testTable())

	c.ApplyDropDatabase("testdb")

	if c.DatabaseExists("testdb") {
		t.Error("database should not exist after drop")
	}
	// Tables should also be gone.
	_, err := c.GetTable("testdb", "users")
	if err != ErrTableNotFound {
		t.Errorf("expected ErrTableNotFound, got %v", err)
	}
}

// TestApplyDropDatabase_NonExistent verifies that dropping a database that does
// not exist is a safe no-op and does not panic.
func TestApplyDropDatabase_NonExistent(t *testing.T) {
	c := NewEmptyCatalog()
	// Should not panic on a non-existent database.
	c.ApplyDropDatabase("nonexistent")
}

// TestApplyCreateTable verifies that a table is retrievable from the catalog
// after calling ApplyCreateTable, and that its name and version are set correctly.
func TestApplyCreateTable(t *testing.T) {
	c := NewEmptyCatalog()
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

// TestApplyCreateTable_WithoutPriorDatabase verifies that ApplyCreateTable
// lazily creates the database's table map when the database has not been
// explicitly created beforehand.
func TestApplyCreateTable_WithoutPriorDatabase(t *testing.T) {
	c := NewEmptyCatalog()
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

// TestApplyDropTable verifies that a table is no longer found after it has
// been dropped via ApplyDropTable.
func TestApplyDropTable(t *testing.T) {
	c := NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())
	c.ApplyCreateTable(testTable())

	c.ApplyDropTable("testdb", "users")

	_, err := c.GetTable("testdb", "users")
	if err != ErrTableNotFound {
		t.Errorf("expected ErrTableNotFound, got %v", err)
	}
}

// TestApplyDropTable_NonExistent verifies that dropping a table from a
// non-existent database is a safe no-op and does not panic.
func TestApplyDropTable_NonExistent(t *testing.T) {
	c := NewEmptyCatalog()
	// Should not panic.
	c.ApplyDropTable("nodb", "notable")
}

// TestApplyAlterTable verifies that ApplyAlterTable replaces the existing table
// metadata with the new version, updating both the column list and the version number.
func TestApplyAlterTable(t *testing.T) {
	c := NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())
	c.ApplyCreateTable(testTable())

	newMeta := &TableMeta{
		Database:   "testdb",
		Name:       "users",
		PrimaryKey: []string{"id"},
		Columns: []ColumnMeta{
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

// TestApplyRenameTable verifies that ApplyRenameTable removes the old table
// name and registers the table under the new name with correct metadata.
func TestApplyRenameTable(t *testing.T) {
	c := NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())
	c.ApplyCreateTable(testTable())

	meta, _ := c.GetTable("testdb", "users")
	renamed := *meta
	renamed.Name = "customers"

	c.ApplyRenameTable("testdb", "users", "customers", &renamed)

	_, err := c.GetTable("testdb", "users")
	if err != ErrTableNotFound {
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

// TestApplyRenameTable_NonExistentOldName verifies that renaming a table whose
// old name does not exist is a safe no-op and does not panic.
func TestApplyRenameTable_NonExistentOldName(t *testing.T) {
	c := NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())

	meta := &TableMeta{Database: "testdb", Name: "new", Version: 1}
	// Should not panic when old name doesn't exist.
	c.ApplyRenameTable("testdb", "nonexistent", "new", meta)
}
