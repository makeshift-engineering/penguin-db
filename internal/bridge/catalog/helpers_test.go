package catalog

import (
	"time"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

// intPtr returns a pointer to v. It is a convenience helper for constructing
// test metadata that uses optional int fields.
func intPtr(v int) *int { return &v }

// strPtr returns a pointer to v. It is a convenience helper for constructing
// test metadata that uses optional string fields.
func strPtr(v string) *string { return &v }

// testDB returns a canonical DatabaseMeta fixture named "testdb" used as the
// standard database across all catalog tests.
func testDB() *DatabaseMeta {
	return &DatabaseMeta{
		Name:      "testdb",
		CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

// testTable returns a canonical TableMeta fixture for a "users" table in the
// "testdb" database with four columns (id, name, email, active) and a
// single-column primary key on id.
func testTable() *TableMeta {
	return &TableMeta{
		Database: "testdb",
		Name:     "users",
		Columns: []ColumnMeta{
			{Name: "id", Type: ast.TypeInt, NotNull: true, PrimaryKey: true},
			{Name: "name", Type: ast.TypeVarchar, VarcharLen: intPtr(255)},
			{Name: "email", Type: ast.TypeText},
			{Name: "active", Type: ast.TypeBoolean, NotNull: true, DefaultValue: strPtr("true")},
		},
		PrimaryKey:     []string{"id"},
		CreatedAt:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Version:        1,
		HasSnowflakeID: false,
	}
}
