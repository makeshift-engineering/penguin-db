package catalog

import (
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

// TestBuildColumnIndexMap_NoChanges verifies that BuildColumnIndexMap produces
// an identity mapping when the write-time column count matches the current
// schema and no columns have been dropped.
func TestBuildColumnIndexMap_NoChanges(t *testing.T) {
	meta := testTable()
	mapping := BuildColumnIndexMap(4, meta)

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

// TestBuildColumnIndexMap_DroppedColumn verifies that BuildColumnIndexMap maps
// a dropped column to -1 and adjusts the active-column indices for the
// remaining columns accordingly.
func TestBuildColumnIndexMap_DroppedColumn(t *testing.T) {
	meta := testTable()
	meta.Columns[1].Dropped = true // Drop "name" (index 1)

	mapping := BuildColumnIndexMap(4, meta)

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

// TestBuildColumnIndexMap_MoreWriteTimeCols verifies that when a row was
// written with more columns than the current schema defines, the extra
// columns are mapped to -1 (ignored on read).
func TestBuildColumnIndexMap_MoreWriteTimeCols(t *testing.T) {
	meta := testTable()
	// Row was written with 6 columns, but current schema only has 4.
	mapping := BuildColumnIndexMap(6, meta)

	if len(mapping) != 6 {
		t.Fatalf("expected mapping len 6, got %d", len(mapping))
	}
	if mapping[4] != -1 || mapping[5] != -1 {
		t.Errorf("extra columns should map to -1, got %d and %d", mapping[4], mapping[5])
	}
}

// TestBuildColumnIndexMap_FewerWriteTimeCols verifies that when a row was
// written with fewer columns than the current schema defines, only the
// available columns are mapped and no out-of-bounds entries are produced.
func TestBuildColumnIndexMap_FewerWriteTimeCols(t *testing.T) {
	meta := testTable()
	// Row was written with 2 columns, but current schema has 4.
	mapping := BuildColumnIndexMap(2, meta)

	if len(mapping) != 2 {
		t.Fatalf("expected mapping len 2, got %d", len(mapping))
	}
	if mapping[0] != 0 || mapping[1] != 1 {
		t.Errorf("expected [0, 1], got [%d, %d]", mapping[0], mapping[1])
	}
}

// TestBuildColumnIndexMap_ZeroCols verifies that BuildColumnIndexMap returns
// an empty mapping when the write-time column count is zero.
func TestBuildColumnIndexMap_ZeroCols(t *testing.T) {
	meta := testTable()
	mapping := BuildColumnIndexMap(0, meta)
	if len(mapping) != 0 {
		t.Errorf("expected empty mapping, got len %d", len(mapping))
	}
}

// TestCountActiveColumns verifies that CountActiveColumns returns the correct
// count of non-dropped columns, both before and after marking a column as dropped.
func TestCountActiveColumns(t *testing.T) {
	meta := testTable()
	if CountActiveColumns(meta) != 4 {
		t.Errorf("expected 4, got %d", CountActiveColumns(meta))
	}

	meta.Columns[1].Dropped = true
	if CountActiveColumns(meta) != 3 {
		t.Errorf("expected 3 after drop, got %d", CountActiveColumns(meta))
	}
}

// TestCountActiveColumns_AllDropped verifies that CountActiveColumns returns
// zero when every column in the table has been marked as dropped.
func TestCountActiveColumns_AllDropped(t *testing.T) {
	meta := &TableMeta{
		Columns: []ColumnMeta{
			{Name: "a", Type: ast.TypeInt, Dropped: true},
			{Name: "b", Type: ast.TypeInt, Dropped: true},
		},
	}
	if CountActiveColumns(meta) != 0 {
		t.Errorf("expected 0, got %d", CountActiveColumns(meta))
	}
}

// TestCountActiveColumns_Empty verifies that CountActiveColumns returns zero
// for a table with no columns at all.
func TestCountActiveColumns_Empty(t *testing.T) {
	meta := &TableMeta{}
	if CountActiveColumns(meta) != 0 {
		t.Errorf("expected 0 for empty columns, got %d", CountActiveColumns(meta))
	}
}
