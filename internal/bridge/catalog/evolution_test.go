package catalog_test

import (
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

func TestBuildColumnIndexMap_NoChanges(t *testing.T) {
	meta := testTable()
	mapping := catalog.BuildColumnIndexMap(4, meta)

	expected := []int{0, 1, 2, 3}
	if len(mapping) != len(expected) {
		t.Fatalf("expected mapping len %d, got %d", len(expected), len(mapping))
	}
	for i, v := range expected {
		if mapping[i] != v {
			t.Errorf("mapping[%d] = %d, want %d", i, mapping[i], v)
		}
	}
}

func TestBuildColumnIndexMap_DroppedColumn(t *testing.T) {
	meta := testTable()
	meta.Columns[1].Dropped = true // Drop "name" (index 1)

	mapping := catalog.BuildColumnIndexMap(4, meta)

	// Column 0 (id) → active index 0
	// Column 1 (name, dropped) → -1
	// Column 2 (email) → active index 1
	// Column 3 (active) → active index 2
	expected := []int{0, -1, 1, 2}
	for i, v := range expected {
		if mapping[i] != v {
			t.Errorf("mapping[%d] = %d, want %d", i, mapping[i], v)
		}
	}
}

func TestBuildColumnIndexMap_MoreWriteTimeCols(t *testing.T) {
	meta := testTable()
	// Row was written with 6 columns, but current schema only has 4.
	mapping := catalog.BuildColumnIndexMap(6, meta)

	if len(mapping) != 6 {
		t.Fatalf("expected mapping len 6, got %d", len(mapping))
	}
	if mapping[4] != -1 || mapping[5] != -1 {
		t.Errorf("extra columns should map to -1, got %d and %d", mapping[4], mapping[5])
	}
}

func TestBuildColumnIndexMap_FewerWriteTimeCols(t *testing.T) {
	meta := testTable()
	// Row was written with 2 columns, but current schema has 4.
	mapping := catalog.BuildColumnIndexMap(2, meta)

	if len(mapping) != 2 {
		t.Fatalf("expected mapping len 2, got %d", len(mapping))
	}
	if mapping[0] != 0 || mapping[1] != 1 {
		t.Errorf("expected [0, 1], got [%d, %d]", mapping[0], mapping[1])
	}
}

func TestBuildColumnIndexMap_ZeroCols(t *testing.T) {
	meta := testTable()
	mapping := catalog.BuildColumnIndexMap(0, meta)
	if len(mapping) != 0 {
		t.Errorf("expected empty mapping, got len %d", len(mapping))
	}
}

func TestCountActiveColumns(t *testing.T) {
	meta := testTable()
	if catalog.CountActiveColumns(meta) != 4 {
		t.Errorf("expected 4, got %d", catalog.CountActiveColumns(meta))
	}

	meta.Columns[1].Dropped = true
	if catalog.CountActiveColumns(meta) != 3 {
		t.Errorf("expected 3 after drop, got %d", catalog.CountActiveColumns(meta))
	}
}

func TestCountActiveColumns_AllDropped(t *testing.T) {
	meta := &catalog.TableMeta{
		Columns: []catalog.ColumnMeta{
			{Name: "a", Type: ast.TypeInt, Dropped: true},
			{Name: "b", Type: ast.TypeInt, Dropped: true},
		},
	}
	if catalog.CountActiveColumns(meta) != 0 {
		t.Errorf("expected 0, got %d", catalog.CountActiveColumns(meta))
	}
}

func TestCountActiveColumns_Empty(t *testing.T) {
	meta := &catalog.TableMeta{}
	if catalog.CountActiveColumns(meta) != 0 {
		t.Errorf("expected 0 for empty columns, got %d", catalog.CountActiveColumns(meta))
	}
}
