package ast_test

import (
	"errors"
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

var (
	_ ast.Statement = (*ast.CreateDatabaseStmt)(nil)
	_ ast.Statement = (*ast.UseDatabaseStmt)(nil)
	_ ast.Statement = (*ast.DropDatabaseStmt)(nil)
	_ ast.Statement = (*ast.CreateTableStmt)(nil)
	_ ast.Statement = (*ast.AlterTableStmt)(nil)
	_ ast.Statement = (*ast.DropTableStmt)(nil)
	_ ast.Statement = (*ast.SelectStmt)(nil)
	_ ast.Statement = (*ast.InsertStmt)(nil)
	_ ast.Statement = (*ast.UpdateStmt)(nil)
	_ ast.Statement = (*ast.DeleteStmt)(nil)
)

func TestStatement_TypeSwitchCoverage(t *testing.T) {
	stmts := []ast.Statement{
		&ast.CreateDatabaseStmt{},
		&ast.UseDatabaseStmt{},
		&ast.DropDatabaseStmt{},
		&ast.CreateTableStmt{},
		&ast.AlterTableStmt{},
		&ast.DropTableStmt{},
		&ast.SelectStmt{},
		&ast.InsertStmt{},
		&ast.UpdateStmt{},
		&ast.DeleteStmt{},
	}

	for _, s := range stmts {
		switch s.(type) {
		case *ast.CreateDatabaseStmt:
		case *ast.UseDatabaseStmt:
		case *ast.DropDatabaseStmt:
		case *ast.CreateTableStmt:
		case *ast.AlterTableStmt:
		case *ast.DropTableStmt:
		case *ast.SelectStmt:
		case *ast.InsertStmt:
		case *ast.UpdateStmt:
		case *ast.DeleteStmt:
		default:
			t.Errorf("unhandled Statement type: %T", s)
		}
	}
}

func TestStatement_Validation(t *testing.T) {
	tests := []struct {
		name    string
		node    ast.Node
		wantErr error
	}{
		{
			name:    "CreateDatabaseStmt empty name",
			node:    &ast.CreateDatabaseStmt{Name: ""},
			wantErr: ast.ErrEmptyDatabaseName,
		},
		{
			name: "CreateTableStmt empty columns",
			node: &ast.CreateTableStmt{
				Table: &ast.Identifier{Name: "t"},
			},
			wantErr: ast.ErrEmptyCreateTableColumns,
		},
		{
			name: "CreateTableStmt nil table",
			node: &ast.CreateTableStmt{
				Columns: []*ast.ColumnDef{
					{Name: "id", Type: &ast.DataType{Kind: ast.TypeInt}},
				},
			},
			wantErr: ast.ErrNilIdentifier,
		},
		{
			name: "SelectStmt valid",
			node: &ast.SelectStmt{
				Columns: []*ast.SelectColumn{
					{Star: true},
				},
			},
			wantErr: nil,
		},
		{
			name: "SelectStmt both distinct and all",
			node: &ast.SelectStmt{
				Distinct: true,
				All:      true,
				Columns: []*ast.SelectColumn{
					{Star: true},
				},
			},
			wantErr: ast.ErrMutuallyExclusiveSelectModifiers,
		},
		{
			name: "SelectStmt empty columns",
			node: &ast.SelectStmt{
				Columns: []*ast.SelectColumn{},
			},
			wantErr: ast.ErrEmptySelectColumns,
		},
		{
			name: "InsertStmt valid",
			node: &ast.InsertStmt{
				Table: &ast.Identifier{Name: "t"},
				Rows:  [][]*ast.SelectExpression{{{Expr: &ast.IntegerLiteral{Value: "1"}}}},
			},
			wantErr: nil,
		},
		{
			name: "InsertStmt both rows and source nil",
			node: &ast.InsertStmt{
				Table: &ast.Identifier{Name: "t"},
			},
			wantErr: ast.ErrInvalidInsertStmt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.node.Validate()
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
