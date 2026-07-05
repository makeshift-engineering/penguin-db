package ast_test

import (
	"errors"
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

var (
	_ ast.Clause = (*ast.DataType)(nil)
	_ ast.Clause = (*ast.ColumnDef)(nil)
	_ ast.Clause = (*ast.SignedLiteral)(nil)
	_ ast.Clause = (*ast.PrimaryKeyConstraint)(nil)
	_ ast.Clause = (*ast.UniqueConstraint)(nil)
	_ ast.Clause = (*ast.NotNullConstraint)(nil)
	_ ast.Clause = (*ast.NullConstraint)(nil)
	_ ast.Clause = (*ast.DefaultConstraint)(nil)
	_ ast.Clause = (*ast.ReferencesConstraint)(nil)
	_ ast.Clause = (*ast.AlterAction)(nil)
	_ ast.Clause = (*ast.SelectColumn)(nil)
	_ ast.Clause = (*ast.TableRef)(nil)
	_ ast.Clause = (*ast.TablePrimary)(nil)
	_ ast.Clause = (*ast.JoinClause)(nil)
	_ ast.Clause = (*ast.WhereClause)(nil)
	_ ast.Clause = (*ast.GroupByClause)(nil)
	_ ast.Clause = (*ast.HavingClause)(nil)
	_ ast.Clause = (*ast.OrderByClause)(nil)
	_ ast.Clause = (*ast.OrderByItem)(nil)
	_ ast.Clause = (*ast.LimitClause)(nil)
	_ ast.Clause = (*ast.SetItem)(nil)
)

// TestClause_TypeSwitchCoverage verifies that every concrete Clause type
// is handled in a type-switch, guarding against missing cases.
func TestClause_TypeSwitchCoverage(t *testing.T) {
	clauses := []ast.Clause{
		&ast.DataType{},
		&ast.ColumnDef{},
		&ast.SignedLiteral{},
		&ast.PrimaryKeyConstraint{},
		&ast.UniqueConstraint{},
		&ast.NotNullConstraint{},
		&ast.NullConstraint{},
		&ast.DefaultConstraint{},
		&ast.ReferencesConstraint{},
		&ast.AlterAction{},
		&ast.SelectColumn{},
		&ast.TableRef{},
		&ast.TablePrimary{},
		&ast.JoinClause{},
		&ast.WhereClause{},
		&ast.GroupByClause{},
		&ast.HavingClause{},
		&ast.OrderByClause{},
		&ast.OrderByItem{},
		&ast.LimitClause{},
		&ast.SetItem{},
	}

	for _, c := range clauses {
		switch c.(type) {
		case *ast.DataType:
		case *ast.ColumnDef:
		case *ast.SignedLiteral:
		case *ast.PrimaryKeyConstraint:
		case *ast.UniqueConstraint:
		case *ast.NotNullConstraint:
		case *ast.NullConstraint:
		case *ast.DefaultConstraint:
		case *ast.ReferencesConstraint:
		case *ast.AlterAction:
		case *ast.SelectColumn:
		case *ast.TableRef:
		case *ast.TablePrimary:
		case *ast.JoinClause:
		case *ast.WhereClause:
		case *ast.GroupByClause:
		case *ast.HavingClause:
		case *ast.OrderByClause:
		case *ast.OrderByItem:
		case *ast.LimitClause:
		case *ast.SetItem:
		default:
			t.Errorf("unhandled Clause type: %T", c)
		}
	}
}

// TestClause_Validation exercises the Validate method on all Clause node
// types, covering valid inputs, nil fields, boundary values, recursive
// validation propagation, and constraint-specific edge cases.
func TestClause_Validation(t *testing.T) {
	intPtr := func(v int) *int { return &v }

	tests := []struct {
		name    string
		node    ast.Node
		wantErr error
	}{
		// DataType
		{"DataType valid varchar", &ast.DataType{Kind: ast.TypeVarchar, VarcharLen: intPtr(255)}, nil},
		{"DataType varchar nil len", &ast.DataType{Kind: ast.TypeVarchar}, ast.ErrVarcharLengthRequired},
		{"DataType varchar zero len", &ast.DataType{Kind: ast.TypeVarchar, VarcharLen: intPtr(0)}, ast.ErrVarcharLengthInvalid},
		{"DataType varchar negative len", &ast.DataType{Kind: ast.TypeVarchar, VarcharLen: intPtr(-5)}, ast.ErrVarcharLengthInvalid},
		{"DataType int with len", &ast.DataType{Kind: ast.TypeInt, VarcharLen: intPtr(10)}, ast.ErrLengthNotSupported},
		{"DataType valid float", &ast.DataType{Kind: ast.TypeFloat}, nil},
		{"DataType valid double", &ast.DataType{Kind: ast.TypeDouble}, nil},
		{"DataType valid decimal no params", &ast.DataType{Kind: ast.TypeDecimal}, nil},
		{"DataType decimal valid prec", &ast.DataType{Kind: ast.TypeDecimal, DecimalPrec: intPtr(10)}, nil},
		{"DataType decimal valid prec+scale", &ast.DataType{Kind: ast.TypeDecimal, DecimalPrec: intPtr(10), DecimalScale: intPtr(2)}, nil},
		{"DataType decimal negative prec", &ast.DataType{Kind: ast.TypeDecimal, DecimalPrec: intPtr(-5)}, ast.ErrDecimalPrecisionInvalid},
		{"DataType decimal zero prec", &ast.DataType{Kind: ast.TypeDecimal, DecimalPrec: intPtr(0)}, ast.ErrDecimalPrecisionInvalid},
		{"DataType decimal scale without prec", &ast.DataType{Kind: ast.TypeDecimal, DecimalScale: intPtr(2)}, ast.ErrDecimalScaleInvalid},
		{"DataType decimal negative scale", &ast.DataType{Kind: ast.TypeDecimal, DecimalPrec: intPtr(10), DecimalScale: intPtr(-1)}, ast.ErrDecimalScaleInvalid},
		{"DataType decimal scale > prec", &ast.DataType{Kind: ast.TypeDecimal, DecimalPrec: intPtr(10), DecimalScale: intPtr(11)}, ast.ErrDecimalScaleInvalid},
		{"DataType int with decimal params", &ast.DataType{Kind: ast.TypeInt, DecimalPrec: intPtr(10)}, ast.ErrDecimalParamsNotSupported},
		{"DataType valid int", &ast.DataType{Kind: ast.TypeInt}, nil},
		{"DataType valid bigint", &ast.DataType{Kind: ast.TypeBigInt}, nil},
		{"DataType valid boolean", &ast.DataType{Kind: ast.TypeBoolean}, nil},
		{"DataType valid text", &ast.DataType{Kind: ast.TypeText}, nil},
		{"DataType valid timestamp", &ast.DataType{Kind: ast.TypeTimestamp}, nil},
		// ColumnDef
		{
			name: "ColumnDef valid",
			node: &ast.ColumnDef{Name: "id", Type: &ast.DataType{Kind: ast.TypeInt}},
		},
		{
			name:    "ColumnDef empty name",
			node:    &ast.ColumnDef{Name: "", Type: &ast.DataType{Kind: ast.TypeInt}},
			wantErr: ast.ErrEmptyIdentifierName,
		},
		{
			name:    "ColumnDef nil type",
			node:    &ast.ColumnDef{Name: "id"},
			wantErr: ast.ErrNilColumnType,
		},
		{
			name:    "ColumnDef nil constraint",
			node:    &ast.ColumnDef{Name: "id", Type: &ast.DataType{Kind: ast.TypeInt}, Constraints: []ast.Clause{nil}},
			wantErr: ast.ErrNilClause,
		},
		{
			name: "ColumnDef recursive type error",
			node: &ast.ColumnDef{Name: "id", Type: &ast.DataType{Kind: ast.TypeVarchar}},
			wantErr: ast.ErrVarcharLengthRequired,
		},
		// SignedLiteral
		{
			name: "SignedLiteral valid",
			node: &ast.SignedLiteral{Value: &ast.IntegerLiteral{Value: "42"}},
		},
		{
			name:    "SignedLiteral nil value",
			node:    &ast.SignedLiteral{},
			wantErr: ast.ErrNilExpression,
		},
		// DefaultConstraint
		{
			name: "DefaultConstraint valid",
			node: &ast.DefaultConstraint{Value: &ast.SignedLiteral{Value: &ast.IntegerLiteral{Value: "1"}}},
		},
		{
			name:    "DefaultConstraint nil value",
			node:    &ast.DefaultConstraint{},
			wantErr: ast.ErrNilDefaultValue,
		},
		// ReferencesConstraint
		{
			name: "ReferencesConstraint valid",
			node: &ast.ReferencesConstraint{Table: "users", Column: "id"},
		},
		{
			name:    "ReferencesConstraint empty table",
			node:    &ast.ReferencesConstraint{Column: "id"},
			wantErr: ast.ErrEmptyReferencesTable,
		},
		{
			name:    "ReferencesConstraint empty column",
			node:    &ast.ReferencesConstraint{Table: "users"},
			wantErr: ast.ErrEmptyReferencesColumn,
		},
		// ForeignRef
		{
			name: "ForeignRef valid",
			node: &ast.ForeignRef{Table: "orders", Column: "id"},
		},
		{
			name:    "ForeignRef empty table",
			node:    &ast.ForeignRef{Column: "id"},
			wantErr: ast.ErrEmptyForeignTable,
		},
		{
			name:    "ForeignRef empty column",
			node:    &ast.ForeignRef{Table: "orders"},
			wantErr: ast.ErrEmptyForeignColumn,
		},
		// AlterAction
		{
			name: "AlterAction add valid",
			node: &ast.AlterAction{Kind: ast.AlterAdd, Column: &ast.ColumnDef{Name: "c", Type: &ast.DataType{Kind: ast.TypeInt}}},
		},
		{
			name:    "AlterAction add nil column",
			node:    &ast.AlterAction{Kind: ast.AlterAdd},
			wantErr: ast.ErrNilColumnDefinition,
		},
		{
			name: "AlterAction modify valid",
			node: &ast.AlterAction{Kind: ast.AlterModify, Column: &ast.ColumnDef{Name: "c", Type: &ast.DataType{Kind: ast.TypeInt}}},
		},
		{
			name:    "AlterAction modify nil column",
			node:    &ast.AlterAction{Kind: ast.AlterModify},
			wantErr: ast.ErrNilColumnDefinition,
		},
		{
			name: "AlterAction rename table valid",
			node: &ast.AlterAction{Kind: ast.AlterRenameTable, NewName: "new_t"},
		},
		{
			name:    "AlterAction rename table empty",
			node:    &ast.AlterAction{Kind: ast.AlterRenameTable},
			wantErr: ast.ErrEmptyNewTableName,
		},
		{
			name: "AlterAction rename column valid",
			node: &ast.AlterAction{Kind: ast.AlterRenameColumn, OldName: "a", NewName: "b"},
		},
		{
			name:    "AlterAction rename column empty old",
			node:    &ast.AlterAction{Kind: ast.AlterRenameColumn, NewName: "b"},
			wantErr: ast.ErrEmptyOldOrNewColumnName,
		},
		{
			name:    "AlterAction rename column empty new",
			node:    &ast.AlterAction{Kind: ast.AlterRenameColumn, OldName: "a"},
			wantErr: ast.ErrEmptyOldOrNewColumnName,
		},
		{
			name: "AlterAction drop column valid",
			node: &ast.AlterAction{Kind: ast.AlterDropColumn, DropName: "c"},
		},
		{
			name:    "AlterAction drop column empty",
			node:    &ast.AlterAction{Kind: ast.AlterDropColumn},
			wantErr: ast.ErrEmptyDropColumnName,
		},
		{
			name:    "AlterAction invalid kind",
			node:    &ast.AlterAction{Kind: ast.AlterActionKind(99)},
			wantErr: ast.ErrInvalidAlterAction,
		},
		// SelectColumn
		{
			name: "SelectColumn star valid",
			node: &ast.SelectColumn{Star: true},
		},
		{
			name: "SelectColumn expr valid",
			node: &ast.SelectColumn{Expr: &ast.SelectExpression{Expr: &ast.IntegerLiteral{Value: "1"}}},
		},
		{
			name: "SelectColumn qualified star valid",
			node: &ast.SelectColumn{QualifiedStar: &ast.Identifier{Name: "t"}},
		},
		{
			name:    "SelectColumn none set",
			node:    &ast.SelectColumn{},
			wantErr: ast.ErrInvalidSelectColumn,
		},
		{
			name: "SelectColumn star and expr",
			node: &ast.SelectColumn{
				Star: true,
				Expr: &ast.SelectExpression{Expr: &ast.IntegerLiteral{Value: "1"}},
			},
			wantErr: ast.ErrInvalidSelectColumn,
		},
		// TableRef
		{
			name: "TableRef primary valid",
			node: &ast.TableRef{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}},
		},
		{
			name:    "TableRef neither set",
			node:    &ast.TableRef{},
			wantErr: ast.ErrInvalidTableRef,
		},
		{
			name: "TableRef both set",
			node: &ast.TableRef{
				Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}},
				Paren:   &ast.TableRef{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}},
			},
			wantErr: ast.ErrInvalidTableRef,
		},
		{
			name: "TableRef nil join element",
			node: &ast.TableRef{
				Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}},
				Joins:   []*ast.JoinClause{nil},
			},
			wantErr: ast.ErrNilClause,
		},
		// TablePrimary
		{
			name: "TablePrimary valid",
			node: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}},
		},
		{
			name:    "TablePrimary nil name",
			node:    &ast.TablePrimary{},
			wantErr: ast.ErrNilIdentifier,
		},
		// JoinClause
		{
			name: "JoinClause inner valid",
			node: &ast.JoinClause{
				Type:  ast.JoinInner,
				Right: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}},
				On:    &ast.ExprCondition{Expr: &ast.IntegerLiteral{Value: "1"}},
			},
		},
		{
			name: "JoinClause inner missing ON",
			node: &ast.JoinClause{
				Type:  ast.JoinInner,
				Right: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}},
			},
			wantErr: ast.ErrNonCrossJoinWithoutOn,
		},
		{
			name: "JoinClause cross valid",
			node: &ast.JoinClause{
				Type:  ast.JoinCross,
				Right: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}},
			},
		},
		{
			name: "JoinClause cross with ON",
			node: &ast.JoinClause{
				Type:  ast.JoinCross,
				Right: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}},
				On:    &ast.ExprCondition{Expr: &ast.IntegerLiteral{Value: "1"}},
			},
			wantErr: ast.ErrCrossJoinWithOn,
		},
		{
			name:    "JoinClause nil right",
			node:    &ast.JoinClause{Type: ast.JoinInner},
			wantErr: ast.ErrNilJoinRightTable,
		},
		// WhereClause
		{
			name: "WhereClause valid",
			node: &ast.WhereClause{Cond: &ast.ExprCondition{Expr: &ast.IntegerLiteral{Value: "1"}}},
		},
		{
			name:    "WhereClause nil cond",
			node:    &ast.WhereClause{},
			wantErr: ast.ErrNilCondition,
		},
		// GroupByClause
		{
			name: "GroupByClause valid",
			node: &ast.GroupByClause{Columns: []*ast.Identifier{{Name: "dept"}}},
		},
		{
			name:    "GroupByClause empty",
			node:    &ast.GroupByClause{},
			wantErr: ast.ErrEmptyGroupBy,
		},
		{
			name:    "GroupByClause nil column",
			node:    &ast.GroupByClause{Columns: []*ast.Identifier{nil}},
			wantErr: ast.ErrNilIdentifier,
		},
		// HavingClause
		{
			name: "HavingClause valid",
			node: &ast.HavingClause{Cond: &ast.ExprCondition{Expr: &ast.IntegerLiteral{Value: "1"}}},
		},
		{
			name:    "HavingClause nil cond",
			node:    &ast.HavingClause{},
			wantErr: ast.ErrNilCondition,
		},
		// OrderByClause
		{
			name: "OrderByClause valid",
			node: &ast.OrderByClause{Items: []*ast.OrderByItem{{Expr: &ast.Identifier{Name: "c"}}}},
		},
		{
			name:    "OrderByClause empty",
			node:    &ast.OrderByClause{},
			wantErr: ast.ErrEmptyOrderBy,
		},
		{
			name:    "OrderByClause nil item",
			node:    &ast.OrderByClause{Items: []*ast.OrderByItem{nil}},
			wantErr: ast.ErrNilClause,
		},
		// OrderByItem
		{
			name: "OrderByItem valid ASC",
			node: &ast.OrderByItem{Expr: &ast.Identifier{Name: "c"}, Direction: ast.OrderAsc},
		},
		{
			name: "OrderByItem valid DESC",
			node: &ast.OrderByItem{Expr: &ast.Identifier{Name: "c"}, Direction: ast.OrderDesc},
		},
		{
			name:    "OrderByItem nil expr",
			node:    &ast.OrderByItem{},
			wantErr: ast.ErrNilExpression,
		},
		// LimitClause
		{name: "LimitClause valid", node: &ast.LimitClause{Count: 10}},
		{name: "LimitClause zero", node: &ast.LimitClause{Count: 0}},
		{name: "LimitClause negative", node: &ast.LimitClause{Count: -1}, wantErr: ast.ErrNegativeLimitCount},
		{name: "LimitClause valid offset", node: &ast.LimitClause{Count: 10, Offset: intPtr(5)}},
		{name: "LimitClause zero offset", node: &ast.LimitClause{Count: 10, Offset: intPtr(0)}},
		{name: "LimitClause negative offset", node: &ast.LimitClause{Count: 10, Offset: intPtr(-1)}, wantErr: ast.ErrNegativeLimitOffset},
		// SetItem
		{
			name: "SetItem valid",
			node: &ast.SetItem{Column: &ast.Identifier{Name: "c"}, Value: &ast.IntegerLiteral{Value: "1"}},
		},
		{
			name:    "SetItem nil column",
			node:    &ast.SetItem{Value: &ast.IntegerLiteral{Value: "1"}},
			wantErr: ast.ErrNilIdentifier,
		},
		{
			name:    "SetItem nil value",
			node:    &ast.SetItem{Column: &ast.Identifier{Name: "c"}},
			wantErr: ast.ErrNilExpression,
		},
		{
			name:    "SetItem recursive column error",
			node:    &ast.SetItem{Column: &ast.Identifier{Name: ""}, Value: &ast.IntegerLiteral{Value: "1"}},
			wantErr: ast.ErrEmptyIdentifierName,
		},
		// PrimaryKeyConstraint, UniqueConstraint, NotNullConstraint, NullConstraint (always valid)
		{name: "PrimaryKeyConstraint valid", node: &ast.PrimaryKeyConstraint{}},
		{name: "UniqueConstraint valid", node: &ast.UniqueConstraint{}},
		{name: "NotNullConstraint valid", node: &ast.NotNullConstraint{}},
		{name: "NullConstraint valid", node: &ast.NullConstraint{}},
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

