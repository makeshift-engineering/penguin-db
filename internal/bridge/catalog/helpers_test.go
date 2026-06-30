package catalog_test

import (
	"time"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

func intPtr(v int) *int       { return &v }
func strPtr(v string) *string { return &v }

func testDB() *catalog.DatabaseMeta {
	return &catalog.DatabaseMeta{
		Name:      "testdb",
		CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func testTable() *catalog.TableMeta {
	return &catalog.TableMeta{
		Database: "testdb",
		Name:     "users",
		Columns: []catalog.ColumnMeta{
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
