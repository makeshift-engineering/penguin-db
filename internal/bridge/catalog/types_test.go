package catalog_test

import (
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

func TestActiveColumns(t *testing.T) {
	meta := testTable()
	active := meta.ActiveColumns()
	if len(active) != 4 {
		t.Fatalf("expected 4 active columns, got %d", len(active))
	}
}

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

func TestActiveColumns_AllDropped(t *testing.T) {
	meta := &catalog.TableMeta{
		Columns: []catalog.ColumnMeta{
			{Name: "a", Type: ast.TypeInt, Dropped: true},
			{Name: "b", Type: ast.TypeInt, Dropped: true},
		},
	}
	active := meta.ActiveColumns()
	if len(active) != 0 {
		t.Errorf("expected 0 active columns, got %d", len(active))
	}
}

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

func TestFindColumn_DroppedColumn(t *testing.T) {
	meta := testTable()
	meta.Columns[2].Dropped = true

	col := meta.FindColumn("email")
	if col != nil {
		t.Error("expected nil for dropped column")
	}
}

func TestFindColumn_NonExistent(t *testing.T) {
	meta := testTable()
	col := meta.FindColumn("nonexistent")
	if col != nil {
		t.Error("expected nil for non-existent column")
	}
}

func TestFindColumn_EmptyColumns(t *testing.T) {
	meta := &catalog.TableMeta{}
	col := meta.FindColumn("anything")
	if col != nil {
		t.Error("expected nil for empty column list")
	}
}
