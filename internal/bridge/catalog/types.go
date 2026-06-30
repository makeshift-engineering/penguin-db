package catalog

import (
	"time"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

// DatabaseMeta holds the metadata for a single database. It is serialized
// to JSON and stored under a catalog DB key in the KV store.
type DatabaseMeta struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// TableMeta holds the full schema definition for a single table. It is
// serialized to JSON and stored under a catalog table key in the KV store.
//
// Version starts at 1 (not 0) so that a version read from a newly-decoded
// catalog entry is always distinguishable from an uninitialized zero value.
type TableMeta struct {
	Database       string       `json:"database"`
	Name           string       `json:"name"`
	Columns        []ColumnMeta `json:"columns"`
	PrimaryKey     []string     `json:"primary_key"`
	CreatedAt      time.Time    `json:"created_at"`
	Version        uint64       `json:"version"`
	HasSnowflakeID bool         `json:"has_snowflake_id"`
}

// ActiveColumns returns only the non-dropped columns in declaration order.
func (tm *TableMeta) ActiveColumns() []ColumnMeta {
	active := make([]ColumnMeta, 0, len(tm.Columns))
	for _, col := range tm.Columns {
		if !col.Dropped {
			active = append(active, col)
		}
	}
	return active
}

// FindColumn searches for a non-dropped column by name. Returns nil if
// the column is not found or has been dropped.
func (tm *TableMeta) FindColumn(name string) *ColumnMeta {
	for _, col := range tm.Columns {
		if col.Name == name && !col.Dropped {
			return &col
		}
	}
	return nil
}

// ColumnMeta describes a single column within a table.
//
// The Dropped flag supports schema evolution: when a column is dropped, it
// is marked Dropped rather than removed from the Columns slice. This
// preserves index positions so that old rows (which still contain bytes for
// the dropped column) can be decoded correctly.
type ColumnMeta struct {
	Name         string           `json:"name"`
	Type         ast.DataTypeKind `json:"type"`
	VarcharLen   *int             `json:"varchar_len,omitempty"`
	NotNull      bool             `json:"not_null,omitempty"`
	PrimaryKey   bool             `json:"primary_key,omitempty"`
	Unique       bool             `json:"unique,omitempty"`
	DefaultValue *string          `json:"default_value,omitempty"`
	ForeignKey   *ForeignKeyRef   `json:"foreign_key,omitempty"`
	Dropped      bool             `json:"dropped,omitempty"`
}

// ForeignKeyRef records a column-level foreign key reference to another
// table's column.
type ForeignKeyRef struct {
	ReferencedDB     string `json:"referenced_db"`
	ReferencedTable  string `json:"referenced_table"`
	ReferencedColumn string `json:"referenced_column"`
}
