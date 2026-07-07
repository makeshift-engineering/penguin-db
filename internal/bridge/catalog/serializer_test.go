package catalog

import (
	"testing"
	"time"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

// TestSerializeRoundTrip_DatabaseMeta verifies that a DatabaseMeta value
// survives an encode→decode round-trip with all fields (Name, CreatedAt)
// preserved exactly.
func TestSerializeRoundTrip_DatabaseMeta(t *testing.T) {
	original := &DatabaseMeta{
		Name:      "mydb",
		CreatedAt: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
	}

	data, err := encodeDatabaseMeta(original)
	if err != nil {
		t.Fatalf("encodeDatabaseMeta: %v", err)
	}

	decoded, err := decodeDatabaseMeta(data)
	if err != nil {
		t.Fatalf("decodeDatabaseMeta: %v", err)
	}

	if decoded.Name != original.Name {
		t.Errorf("Name: got %q, want %q", decoded.Name, original.Name)
	}
	if !decoded.CreatedAt.Equal(original.CreatedAt) {
		t.Errorf("CreatedAt: got %v, want %v", decoded.CreatedAt, original.CreatedAt)
	}
}

// TestSerializeRoundTrip_TableMeta verifies that a full TableMeta value
// (including columns with optional fields like VarcharLen and DefaultValue)
// survives an encode→decode round-trip with all fields preserved.
func TestSerializeRoundTrip_TableMeta(t *testing.T) {
	varcharLen := 255
	defaultVal := "true"

	original := &TableMeta{
		Database: "testdb",
		Name:     "users",
		Columns: []ColumnMeta{
			{Name: "id", Type: ast.TypeInt, NotNull: true, PrimaryKey: true},
			{Name: "name", Type: ast.TypeVarchar, VarcharLen: &varcharLen},
			{Name: "email", Type: ast.TypeText},
			{Name: "active", Type: ast.TypeBoolean, NotNull: true, DefaultValue: &defaultVal},
		},
		PrimaryKey:     []string{"id"},
		CreatedAt:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Version:        1,
		HasSnowflakeID: false,
	}

	data, err := encodeTableMeta(original)
	if err != nil {
		t.Fatalf("encodeTableMeta: %v", err)
	}

	decoded, err := decodeTableMeta(data)
	if err != nil {
		t.Fatalf("decodeTableMeta: %v", err)
	}

	if decoded.Name != original.Name {
		t.Errorf("Name: got %q, want %q", decoded.Name, original.Name)
	}
	if decoded.Database != original.Database {
		t.Errorf("Database: got %q, want %q", decoded.Database, original.Database)
	}
	if decoded.Version != original.Version {
		t.Errorf("Version: got %d, want %d", decoded.Version, original.Version)
	}
	if decoded.HasSnowflakeID != original.HasSnowflakeID {
		t.Errorf("HasSnowflakeID: got %v, want %v", decoded.HasSnowflakeID, original.HasSnowflakeID)
	}
	if len(decoded.Columns) != len(original.Columns) {
		t.Fatalf("Columns count: got %d, want %d", len(decoded.Columns), len(original.Columns))
	}
	for i, col := range decoded.Columns {
		orig := original.Columns[i]
		if col.Name != orig.Name {
			t.Errorf("Columns[%d].Name: got %q, want %q", i, col.Name, orig.Name)
		}
		if col.Type != orig.Type {
			t.Errorf("Columns[%d].Type: got %d, want %d", i, col.Type, orig.Type)
		}
		if col.NotNull != orig.NotNull {
			t.Errorf("Columns[%d].NotNull: got %v, want %v", i, col.NotNull, orig.NotNull)
		}
		if col.PrimaryKey != orig.PrimaryKey {
			t.Errorf("Columns[%d].PrimaryKey: got %v, want %v", i, col.PrimaryKey, orig.PrimaryKey)
		}
	}
	if len(decoded.PrimaryKey) != len(original.PrimaryKey) {
		t.Fatalf("PK count: got %d, want %d", len(decoded.PrimaryKey), len(original.PrimaryKey))
	}
	for i := range decoded.PrimaryKey {
		if decoded.PrimaryKey[i] != original.PrimaryKey[i] {
			t.Errorf("PK[%d]: got %q, want %q", i, decoded.PrimaryKey[i], original.PrimaryKey[i])
		}
	}
}

// TestSerializeRoundTrip_DroppedColumn verifies that the Dropped flag on a
// column is correctly preserved through an encode→decode round-trip.
func TestSerializeRoundTrip_DroppedColumn(t *testing.T) {
	original := &TableMeta{
		Database: "db",
		Name:     "t",
		Columns: []ColumnMeta{
			{Name: "a", Type: ast.TypeInt},
			{Name: "b", Type: ast.TypeText, Dropped: true},
			{Name: "c", Type: ast.TypeBoolean},
		},
		Version: 2,
	}

	data, err := encodeTableMeta(original)
	if err != nil {
		t.Fatalf("encodeTableMeta: %v", err)
	}

	decoded, err := decodeTableMeta(data)
	if err != nil {
		t.Fatalf("decodeTableMeta: %v", err)
	}

	if !decoded.Columns[1].Dropped {
		t.Error("expected Columns[1].Dropped to be true after round-trip")
	}
}

// TestSerializeRoundTrip_ForeignKey verifies that a column's ForeignKey
// reference (including ReferencedDB, ReferencedTable, and ReferencedColumn)
// is correctly preserved through an encode→decode round-trip.
func TestSerializeRoundTrip_ForeignKey(t *testing.T) {
	original := &TableMeta{
		Database: "db",
		Name:     "orders",
		Columns: []ColumnMeta{
			{
				Name:    "user_id",
				Type:    ast.TypeInt,
				NotNull: true,
				ForeignKey: &ForeignKeyRef{
					ReferencedDB:     "db",
					ReferencedTable:  "users",
					ReferencedColumn: "id",
				},
			},
		},
		Version: 1,
	}

	data, err := encodeTableMeta(original)
	if err != nil {
		t.Fatalf("encodeTableMeta: %v", err)
	}

	decoded, err := decodeTableMeta(data)
	if err != nil {
		t.Fatalf("decodeTableMeta: %v", err)
	}

	fk := decoded.Columns[0].ForeignKey
	if fk == nil {
		t.Fatal("expected ForeignKey to be non-nil")
	}
	if fk.ReferencedTable != "users" {
		t.Errorf("ReferencedTable: got %q, want 'users'", fk.ReferencedTable)
	}
	if fk.ReferencedColumn != "id" {
		t.Errorf("ReferencedColumn: got %q, want 'id'", fk.ReferencedColumn)
	}
}

// TestDecodeDatabaseMeta_InvalidJSON verifies that decodeDatabaseMeta returns
// an error when given input that is not valid JSON.
func TestDecodeDatabaseMeta_InvalidJSON(t *testing.T) {
	_, err := decodeDatabaseMeta([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// TestDecodeTableMeta_InvalidJSON verifies that decodeTableMeta returns an
// error when given input that is not valid JSON.
func TestDecodeTableMeta_InvalidJSON(t *testing.T) {
	_, err := decodeTableMeta([]byte("{invalid"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
