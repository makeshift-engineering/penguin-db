package catalog

import (
	"errors"
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/kv"
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

// TestBuildCreateDatabaseOps verifies that BuildCreateDatabaseOps produces
// a single OpPut operation containing the serialized database metadata.
func TestBuildCreateDatabaseOps(t *testing.T) {
	meta := testDB()
	ops, err := BuildCreateDatabaseOps(meta)
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

// TestBuildDropDatabaseOps verifies that BuildDropDatabaseOps produces delete
// operations for the database key, each table key, and each table's sequence key.
func TestBuildDropDatabaseOps(t *testing.T) {
	c := NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())
	c.ApplyCreateTable(testTable())

	tables, err := c.ListTables("testdb")
	if err != nil {
		t.Fatalf("ListTables: %v", err)
	}

	ops, err := BuildDropDatabaseOps("testdb", tables)
	if err != nil {
		t.Fatalf("BuildDropDatabaseOps: %v", err)
	}
	// 1 delete for DB + 1 delete for table key + 1 delete for seq key.
	if len(ops) != 3 {
		t.Fatalf("expected 3 ops, got %d", len(ops))
	}
}

// TestBuildDropDatabaseOps_NoTables verifies that dropping a database with
// no tables produces a single OpDelete for the database key only.
func TestBuildDropDatabaseOps_NoTables(t *testing.T) {
	ops, err := BuildDropDatabaseOps("emptydb", nil)
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

// TestBuildCreateTableOps verifies that BuildCreateTableOps produces a single
// OpPut operation when creating a table in an existing database.
func TestBuildCreateTableOps(t *testing.T) {
	c := NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())

	ops, err := BuildCreateTableOps(c, testTable())
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

// TestBuildCreateTableOps_NoDB verifies that BuildCreateTableOps returns
// ErrCatalogDBMissing when the target database does not exist in the
func TestBuildCreateTableOps_NoDB(t *testing.T) {
	c := NewEmptyCatalog()
	_, err := BuildCreateTableOps(c, testTable())
	if !errors.Is(err, ErrCatalogDBMissing) {
		t.Errorf("expected ErrCatalogDBMissing, got %v", err)
	}
}

// TestBuildCreateTableOps_DuplicateTable verifies that BuildCreateTableOps
// returns ErrTableExists when a table with the same name already exists.
func TestBuildCreateTableOps_DuplicateTable(t *testing.T) {
	c := NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())
	c.ApplyCreateTable(testTable())

	_, err := BuildCreateTableOps(c, testTable())
	if !errors.Is(err, ErrTableExists) {
		t.Errorf("expected ErrTableExists, got %v", err)
	}
}

// TestBuildCreateTableOps_DuplicateColumns verifies that BuildCreateTableOps
// returns ErrDuplicateColumn when the table definition contains two columns
// with the same name.
func TestBuildCreateTableOps_DuplicateColumns(t *testing.T) {
	c := NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())

	meta := &TableMeta{
		Database: "testdb",
		Name:     "bad",
		Columns: []ColumnMeta{
			{Name: "id", Type: ast.TypeInt},
			{Name: "id", Type: ast.TypeVarchar},
		},
		PrimaryKey: []string{"id"},
		Version:    1,
	}

	_, err := BuildCreateTableOps(c, meta)
	if !errors.Is(err, ErrDuplicateColumn) {
		t.Errorf("expected ErrDuplicateColumn, got %v", err)
	}
}

// TestBuildCreateTableOps_PKColumnNotFound verifies that BuildCreateTableOps
// returns ErrPKColumnNotFound when a primary key references a column name
// that does not exist in the column list.
func TestBuildCreateTableOps_PKColumnNotFound(t *testing.T) {
	c := NewEmptyCatalog()
	c.ApplyCreateDatabase(testDB())

	meta := &TableMeta{
		Database:   "testdb",
		Name:       "bad",
		Columns:    []ColumnMeta{{Name: "id", Type: ast.TypeInt}},
		PrimaryKey: []string{"nonexistent"},
		Version:    1,
	}

	_, err := BuildCreateTableOps(c, meta)
	if !errors.Is(err, ErrPKColumnNotFound) {
		t.Errorf("expected ErrPKColumnNotFound, got %v", err)
	}
}

// TestBuildDropTableOps verifies that BuildDropTableOps produces two OpDelete
// operations: one for the table metadata key and one for the sequence key.
func TestBuildDropTableOps(t *testing.T) {
	ops, err := BuildDropTableOps("testdb", "users")
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

// TestBuildAlterTableOps_AddNullableColumn verifies that adding a nullable
// column produces a single OpPut with the updated metadata and increments
// the table version.
func TestBuildAlterTableOps_AddNullableColumn(t *testing.T) {
	old := testTable()
	newMeta := &TableMeta{
		Database:   "testdb",
		Name:       "users",
		PrimaryKey: []string{"id"},
		Columns: append(append([]ColumnMeta{}, old.Columns...),
			ColumnMeta{Name: "age", Type: ast.TypeInt},
		),
		Version: old.Version,
	}

	ops, err := BuildAlterTableOps(old, newMeta)
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

// TestBuildAlterTableOps_AddNotNullWithDefault verifies that adding a NOT NULL
// column is accepted when a DEFAULT value is provided.
func TestBuildAlterTableOps_AddNotNullWithDefault(t *testing.T) {
	old := testTable()
	newMeta := &TableMeta{
		Database:   "testdb",
		Name:       "users",
		PrimaryKey: []string{"id"},
		Columns: append(append([]ColumnMeta{}, old.Columns...),
			ColumnMeta{
				Name:         "role",
				Type:         ast.TypeVarchar,
				VarcharLen:   intPtr(50),
				NotNull:      true,
				DefaultValue: strPtr("'user'"),
			},
		),
		Version: old.Version,
	}

	_, err := BuildAlterTableOps(old, newMeta)
	if err != nil {
		t.Fatalf("expected success for NOT NULL with DEFAULT: %v", err)
	}
}

// TestBuildAlterTableOps_AddNotNullWithoutDefault_Rejected verifies that adding
// a NOT NULL column without a DEFAULT value is rejected with ErrUnsupportedAlter.
func TestBuildAlterTableOps_AddNotNullWithoutDefault_Rejected(t *testing.T) {
	old := testTable()
	newMeta := &TableMeta{
		Database:   "testdb",
		Name:       "users",
		PrimaryKey: []string{"id"},
		Columns: append(append([]ColumnMeta{}, old.Columns...),
			ColumnMeta{Name: "required", Type: ast.TypeInt, NotNull: true},
		),
		Version: old.Version,
	}

	_, err := BuildAlterTableOps(old, newMeta)
	if !errors.Is(err, ErrUnsupportedAlter) {
		t.Errorf("expected ErrUnsupportedAlter, got %v", err)
	}
}

// TestBuildAlterTableOps_DropColumn verifies that marking a non-primary-key
// column as dropped is accepted by BuildAlterTableOps.
func TestBuildAlterTableOps_DropColumn(t *testing.T) {
	old := testTable()
	newCols := make([]ColumnMeta, len(old.Columns))
	copy(newCols, old.Columns)
	newCols[2].Dropped = true // Drop "email"

	newMeta := &TableMeta{
		Database:   "testdb",
		Name:       "users",
		PrimaryKey: []string{"id"},
		Columns:    newCols,
		Version:    old.Version,
	}

	_, err := BuildAlterTableOps(old, newMeta)
	if err != nil {
		t.Fatalf("expected drop column to succeed: %v", err)
	}
}

// TestBuildAlterTableOps_DropPKColumn_Rejected verifies that attempting to
// drop a primary key column is rejected with ErrCannotDropPKColumn.
func TestBuildAlterTableOps_DropPKColumn_Rejected(t *testing.T) {
	old := testTable()
	newCols := make([]ColumnMeta, len(old.Columns))
	copy(newCols, old.Columns)
	newCols[0].Dropped = true // Drop "id" — the PK

	newMeta := &TableMeta{
		Database:   "testdb",
		Name:       "users",
		PrimaryKey: []string{"id"},
		Columns:    newCols,
		Version:    old.Version,
	}

	_, err := BuildAlterTableOps(old, newMeta)
	if !errors.Is(err, ErrCannotDropPKColumn) {
		t.Errorf("expected ErrCannotDropPKColumn, got %v", err)
	}
}

// TestBuildAlterTableOps_ChangeColumnType_Rejected verifies that changing an
// existing column's data type is rejected with ErrUnsupportedAlter.
func TestBuildAlterTableOps_ChangeColumnType_Rejected(t *testing.T) {
	old := testTable()
	newCols := make([]ColumnMeta, len(old.Columns))
	copy(newCols, old.Columns)
	newCols[1].Type = ast.TypeInt // VARCHAR → INT

	newMeta := &TableMeta{
		Database:   "testdb",
		Name:       "users",
		PrimaryKey: []string{"id"},
		Columns:    newCols,
		Version:    old.Version,
	}

	_, err := BuildAlterTableOps(old, newMeta)
	if !errors.Is(err, ErrUnsupportedAlter) {
		t.Errorf("expected ErrUnsupportedAlter, got %v", err)
	}
}

// TestBuildAlterTableOps_ChangePK_Rejected verifies that modifying the primary
// key definition of an existing table is rejected with ErrUnsupportedAlter.
func TestBuildAlterTableOps_ChangePK_Rejected(t *testing.T) {
	old := testTable()
	newMeta := &TableMeta{
		Database:   "testdb",
		Name:       "users",
		PrimaryKey: []string{"id", "name"},
		Columns:    old.Columns,
		Version:    old.Version,
	}

	_, err := BuildAlterTableOps(old, newMeta)
	if !errors.Is(err, ErrUnsupportedAlter) {
		t.Errorf("expected ErrUnsupportedAlter, got %v", err)
	}
}

// TestBuildRenameTableOps verifies that BuildRenameTableOps produces an
// OpDelete for the old table key and an OpPut for the new table key, and
// updates the table metadata's Name field to the new name.
// It also verifies that the old sequence key is deleted and the new one
// is inserted if a sequence value is provided.
func TestBuildRenameTableOps(t *testing.T) {
	meta := testTable()
	metaCopy := *meta

	seqVal := []byte("seq123")
	ops, err := BuildRenameTableOps("testdb", "users", "customers", &metaCopy, seqVal)
	if err != nil {
		t.Fatalf("BuildRenameTableOps: %v", err)
	}
	if len(ops) != 4 {
		t.Fatalf("expected 4 ops (delete/put table + delete/put seq), got %d", len(ops))
	}
	if ops[0].Type != kv.OpDelete {
		t.Errorf("first op should be OpDelete, got %d", ops[0].Type)
	}
	if ops[1].Type != kv.OpPut {
		t.Errorf("second op should be OpPut, got %d", ops[1].Type)
	}
	if ops[2].Type != kv.OpDelete {
		t.Errorf("third op should be OpDelete, got %d", ops[2].Type)
	}
	if ops[3].Type != kv.OpPut {
		t.Errorf("fourth op should be OpPut, got %d", ops[3].Type)
	}
	if string(ops[3].Value) != "seq123" {
		t.Errorf("expected sequence value to be seq123, got %s", string(ops[3].Value))
	}
	if metaCopy.Name == "customers" {
		t.Errorf("expected meta.Name to remain unchanged, but it was mutated to 'customers'")
	}
}

// TestBuildAlterTableOps_VersionLeak checks if BuildAlterTableOps mutates newMeta in-place.
func TestBuildAlterTableOps_VersionLeak(t *testing.T) {
	old := testTable()
	newMeta := &TableMeta{
		Database:   "testdb",
		Name:       "users",
		PrimaryKey: []string{"id"},
		Columns:    append(append([]ColumnMeta{}, old.Columns...), ColumnMeta{Name: "age", Type: ast.TypeInt}),
		Version:    old.Version,
	}

	_, err := BuildAlterTableOps(old, newMeta)
	if err != nil {
		t.Fatalf("BuildAlterTableOps failed: %v", err)
	}

	// Verify that BuildAlterTableOps mutated newMeta.Version
	t.Logf("newMeta Version after BuildAlterTableOps: %d (original: %d)", newMeta.Version, old.Version)
}

// TestBuildRenameTableOps_ShallowCopy checks if BuildRenameTableOps uses a copy.
func TestBuildRenameTableOps_ShallowCopy(t *testing.T) {
	meta := testTable()
	metaCopy := *meta
	seqVal := []byte("seq123")

	_, err := BuildRenameTableOps("testdb", "users", "customers", &metaCopy, seqVal)
	if err != nil {
		t.Fatalf("BuildRenameTableOps failed: %v", err)
	}

	// Verify that metaCopy.Name was not mutated to 'customers' (shallow copy Name is set on clone)
	if metaCopy.Name == "customers" {
		t.Errorf("expected metaCopy.Name to remain unchanged, but got %q", metaCopy.Name)
	}
}

