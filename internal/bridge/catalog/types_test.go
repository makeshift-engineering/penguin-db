package catalog

import (
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

// TestActiveColumns verifies that ActiveColumns returns all columns when none
// have been dropped.
func TestActiveColumns(t *testing.T) {
	meta := testTable()
	active := meta.ActiveColumns()
	if len(active) != 4 {
		t.Fatalf("expected 4 active columns, got %d", len(active))
	}
}

// TestActiveColumns_WithDropped verifies that ActiveColumns excludes dropped
// columns from the result and preserves the order of the remaining columns.
func TestActiveColumns_WithDropped(t *testing.T) {
	meta := testTable()
	meta.Columns[1].Dropped = true

	active := meta.ActiveColumns()
	if len(active) != 3 {
		t.Fatalf("expected 3 active columns, got %d", len(active))
	}
	if active[0].Name != "id" || active[1].Name != "email" || active[2].Name != "active" {
		t.Errorf("unexpected names: %s, %s, %s", active[0].Name, active[1].Name, active[2].Name)
	}
}

// TestActiveColumns_AllDropped verifies that ActiveColumns returns an empty
// slice when every column has been marked as dropped.
func TestActiveColumns_AllDropped(t *testing.T) {
	meta := &TableMeta{
		Columns: []ColumnMeta{
			{Name: "a", Type: ast.TypeInt, Dropped: true},
			{Name: "b", Type: ast.TypeInt, Dropped: true},
		},
	}
	active := meta.ActiveColumns()
	if len(active) != 0 {
		t.Errorf("expected 0 active columns, got %d", len(active))
	}
}

// TestFindColumn verifies that FindColumn returns the correct column metadata
// when searching by name for an existing, non-dropped column.
func TestFindColumn(t *testing.T) {
	meta := testTable()

	col := meta.FindColumn("email")
	if col == nil {
		t.Fatal("expected to find 'email'")
	}
	if col.Type != ast.TypeText {
		t.Errorf("expected TypeText, got %d", col.Type)
	}
}

// TestFindColumn_DroppedColumn verifies that FindColumn returns nil for a
// column that exists in the schema but has been marked as dropped.
func TestFindColumn_DroppedColumn(t *testing.T) {
	meta := testTable()
	meta.Columns[2].Dropped = true

	col := meta.FindColumn("email")
	if col != nil {
		t.Error("expected nil for dropped column")
	}
}

// TestFindColumn_NonExistent verifies that FindColumn returns nil when the
// requested column name does not exist in the table schema at all.
func TestFindColumn_NonExistent(t *testing.T) {
	meta := testTable()
	col := meta.FindColumn("nonexistent")
	if col != nil {
		t.Error("expected nil for non-existent column")
	}
}

// TestFindColumn_EmptyColumns verifies that FindColumn returns nil for a table
// that has an empty column list.
func TestFindColumn_EmptyColumns(t *testing.T) {
	meta := &TableMeta{}
	col := meta.FindColumn("anything")
	if col != nil {
		t.Error("expected nil for empty column list")
	}
}
