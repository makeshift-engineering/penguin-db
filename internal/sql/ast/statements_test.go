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

// TestStatement_TypeSwitchCoverage verifies that every concrete Statement
// type is handled in a type-switch, guarding against missing cases.
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

// TestStatement_Validation exercises the Validate method on all Statement
// node types, covering valid inputs, nil fields, empty required lists,
// mutually exclusive flags, and recursive validation propagation.
func TestStatement_Validation(t *testing.T) {
	validCol := &ast.ColumnDef{Name: "id", Type: &ast.DataType{Kind: ast.TypeInt}}
	validIdent := &ast.Identifier{Name: "t"}
	validSelectCol := &ast.SelectColumn{Star: true}
	validAction := &ast.AlterAction{Kind: ast.AlterAdd, Column: validCol}

	tests := []struct {
		name    string
		node    ast.Node
		wantErr error
	}{
		// CreateDatabaseStmt
		{name: "CreateDatabaseStmt valid", node: &ast.CreateDatabaseStmt{Name: "db"}},
		{name: "CreateDatabaseStmt empty name", node: &ast.CreateDatabaseStmt{}, wantErr: ast.ErrEmptyDatabaseName},
		// UseDatabaseStmt
		{name: "UseDatabaseStmt valid", node: &ast.UseDatabaseStmt{Name: "db"}},
		{name: "UseDatabaseStmt empty name", node: &ast.UseDatabaseStmt{}, wantErr: ast.ErrEmptyDatabaseName},
		// DropDatabaseStmt
		{name: "DropDatabaseStmt valid", node: &ast.DropDatabaseStmt{Name: "db"}},
		{name: "DropDatabaseStmt empty name", node: &ast.DropDatabaseStmt{}, wantErr: ast.ErrEmptyDatabaseName},
		// CreateTableStmt
		{
			name:    "CreateTableStmt nil table",
			node:    &ast.CreateTableStmt{Columns: []*ast.ColumnDef{validCol}},
			wantErr: ast.ErrNilIdentifier,
		},
		{
			name:    "CreateTableStmt empty columns",
			node:    &ast.CreateTableStmt{Table: validIdent},
			wantErr: ast.ErrEmptyCreateTableColumns,
		},
		{
			name: "CreateTableStmt valid",
			node: &ast.CreateTableStmt{Table: validIdent, Columns: []*ast.ColumnDef{validCol}},
		},
		{
			name:    "CreateTableStmt nil column element",
			node:    &ast.CreateTableStmt{Table: validIdent, Columns: []*ast.ColumnDef{nil}},
			wantErr: ast.ErrNilClause,
		},
		// AlterTableStmt
		{
			name: "AlterTableStmt valid",
			node: &ast.AlterTableStmt{Table: validIdent, Action: validAction},
		},
		{
			name:    "AlterTableStmt nil table",
			node:    &ast.AlterTableStmt{Action: validAction},
			wantErr: ast.ErrNilIdentifier,
		},
		{
			name:    "AlterTableStmt nil action",
			node:    &ast.AlterTableStmt{Table: validIdent},
			wantErr: ast.ErrNilAlterTableAction,
		},
		// DropTableStmt
		{
			name: "DropTableStmt valid",
			node: &ast.DropTableStmt{Table: validIdent},
		},
		{
			name:    "DropTableStmt nil table",
			node:    &ast.DropTableStmt{},
			wantErr: ast.ErrNilIdentifier,
		},
		// SelectStmt
		{
			name:    "SelectStmt valid",
			node:    &ast.SelectStmt{Columns: []*ast.SelectColumn{validSelectCol}},
			wantErr: nil,
		},
		{
			name:    "SelectStmt empty columns",
			node:    &ast.SelectStmt{},
			wantErr: ast.ErrEmptySelectColumns,
		},
		{
			name:    "SelectStmt both distinct and all",
			node:    &ast.SelectStmt{Distinct: true, All: true, Columns: []*ast.SelectColumn{validSelectCol}},
			wantErr: ast.ErrMutuallyExclusiveSelectModifiers,
		},
		{
			name:    "SelectStmt nil column element",
			node:    &ast.SelectStmt{Columns: []*ast.SelectColumn{nil}},
			wantErr: ast.ErrNilClause,
		},
		{
			name:    "SelectStmt nil from element",
			node:    &ast.SelectStmt{Columns: []*ast.SelectColumn{validSelectCol}, From: []*ast.TableRef{nil}},
			wantErr: ast.ErrNilClause,
		},
		{
			name: "SelectStmt with where nil cond",
			node: &ast.SelectStmt{
				Columns: []*ast.SelectColumn{validSelectCol},
				Where:   &ast.WhereClause{},
			},
			wantErr: ast.ErrNilCondition,
		},
		// InsertStmt
		{
			name: "InsertStmt valid rows",
			node: &ast.InsertStmt{
				Table: validIdent,
				Rows:  [][]*ast.SelectExpression{{{Expr: &ast.IntegerLiteral{Value: "1"}}}},
			},
		},
		{
			name: "InsertStmt valid source",
			node: &ast.InsertStmt{
				Table:  validIdent,
				Source: &ast.SelectStmt{Columns: []*ast.SelectColumn{validSelectCol}},
			},
		},
		{
			name:    "InsertStmt nil table",
			node:    &ast.InsertStmt{Rows: [][]*ast.SelectExpression{{{Expr: &ast.IntegerLiteral{Value: "1"}}}}},
			wantErr: ast.ErrNilIdentifier,
		},
		{
			name:    "InsertStmt neither rows nor source",
			node:    &ast.InsertStmt{Table: validIdent},
			wantErr: ast.ErrInvalidInsertStmt,
		},
		{
			name: "InsertStmt both rows and source",
			node: &ast.InsertStmt{
				Table:  validIdent,
				Rows:   [][]*ast.SelectExpression{{{Expr: &ast.IntegerLiteral{Value: "1"}}}},
				Source: &ast.SelectStmt{Columns: []*ast.SelectColumn{validSelectCol}},
			},
			wantErr: ast.ErrInvalidInsertStmt,
		},
		{
			name: "InsertStmt nil value in row",
			node: &ast.InsertStmt{
				Table: validIdent,
				Rows:  [][]*ast.SelectExpression{{nil}},
			},
			wantErr: ast.ErrNilExpression,
		},
		// UpdateStmt
		{
			name: "UpdateStmt valid",
			node: &ast.UpdateStmt{
				Table: validIdent,
				Set:   []*ast.SetItem{{Column: &ast.Identifier{Name: "c"}, Value: &ast.IntegerLiteral{Value: "1"}}},
			},
		},
		{
			name:    "UpdateStmt nil table",
			node:    &ast.UpdateStmt{Set: []*ast.SetItem{{Column: &ast.Identifier{Name: "c"}, Value: &ast.IntegerLiteral{Value: "1"}}}},
			wantErr: ast.ErrNilIdentifier,
		},
		{
			name:    "UpdateStmt empty set",
			node:    &ast.UpdateStmt{Table: validIdent},
			wantErr: ast.ErrEmptyUpdateAssignments,
		},
		{
			name:    "UpdateStmt nil set item",
			node:    &ast.UpdateStmt{Table: validIdent, Set: []*ast.SetItem{nil}},
			wantErr: ast.ErrNilClause,
		},
		// DeleteStmt
		{
			name: "DeleteStmt valid",
			node: &ast.DeleteStmt{Table: validIdent},
		},
		{
			name:    "DeleteStmt nil table",
			node:    &ast.DeleteStmt{},
			wantErr: ast.ErrNilIdentifier,
		},
		{
			name: "DeleteStmt with where nil cond",
			node: &ast.DeleteStmt{
				Table: validIdent,
				Where: &ast.WhereClause{},
			},
			wantErr: ast.ErrNilCondition,
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

