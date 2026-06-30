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

func TestClause_Validation(t *testing.T) {
	tests := []struct {
		name    string
		node    ast.Node
		wantErr error
	}{
		{
			name: "DataType valid varchar",
			node: func() ast.Node {
				lenVal := 255
				return &ast.DataType{
					Kind:       ast.TypeVarchar,
					VarcharLen: &lenVal,
				}
			}(),
			wantErr: nil,
		},
		{
			name: "DataType invalid varchar nil len",
			node: &ast.DataType{
				Kind: ast.TypeVarchar,
			},
			wantErr: ast.ErrVarcharLengthRequired,
		},
		{
			name: "DataType invalid int with len",
			node: func() ast.Node {
				lenVal := 10
				return &ast.DataType{
					Kind:       ast.TypeInt,
					VarcharLen: &lenVal,
				}
			}(),
			wantErr: ast.ErrLengthNotSupported,
		},
		{
			name: "DataType valid float",
			node: &ast.DataType{
				Kind: ast.TypeFloat,
			},
			wantErr: nil,
		},
		{
			name: "DataType valid double",
			node: &ast.DataType{
				Kind: ast.TypeDouble,
			},
			wantErr: nil,
		},
		{
			name: "DataType valid decimal no params",
			node: &ast.DataType{
				Kind: ast.TypeDecimal,
			},
			wantErr: nil,
		},
		{
			name: "DataType valid decimal precision",
			node: func() ast.Node {
				precVal := 10
				return &ast.DataType{
					Kind:        ast.TypeDecimal,
					DecimalPrec: &precVal,
				}
			}(),
			wantErr: nil,
		},
		{
			name: "DataType valid decimal precision and scale",
			node: func() ast.Node {
				precVal := 10
				scaleVal := 2
				return &ast.DataType{
					Kind:         ast.TypeDecimal,
					DecimalPrec:  &precVal,
					DecimalScale: &scaleVal,
				}
			}(),
			wantErr: nil,
		},
		{
			name: "DataType invalid decimal negative precision",
			node: func() ast.Node {
				precVal := -5
				return &ast.DataType{
					Kind:        ast.TypeDecimal,
					DecimalPrec: &precVal,
				}
			}(),
			wantErr: ast.ErrDecimalPrecisionInvalid,
		},
		{
			name: "DataType invalid decimal zero precision",
			node: func() ast.Node {
				precVal := 0
				return &ast.DataType{
					Kind:        ast.TypeDecimal,
					DecimalPrec: &precVal,
				}
			}(),
			wantErr: ast.ErrDecimalPrecisionInvalid,
		},
		{
			name: "DataType invalid decimal scale without precision",
			node: func() ast.Node {
				scaleVal := 2
				return &ast.DataType{
					Kind:         ast.TypeDecimal,
					DecimalScale: &scaleVal,
				}
			}(),
			wantErr: ast.ErrDecimalScaleInvalid,
		},
		{
			name: "DataType invalid decimal scale negative",
			node: func() ast.Node {
				precVal := 10
				scaleVal := -1
				return &ast.DataType{
					Kind:         ast.TypeDecimal,
					DecimalPrec:  &precVal,
					DecimalScale: &scaleVal,
				}
			}(),
			wantErr: ast.ErrDecimalScaleInvalid,
		},
		{
			name: "DataType invalid decimal scale larger than precision",
			node: func() ast.Node {
				precVal := 10
				scaleVal := 11
				return &ast.DataType{
					Kind:         ast.TypeDecimal,
					DecimalPrec:  &precVal,
					DecimalScale: &scaleVal,
				}
			}(),
			wantErr: ast.ErrDecimalScaleInvalid,
		},
		{
			name: "DataType invalid int with decimal params",
			node: func() ast.Node {
				precVal := 10
				return &ast.DataType{
					Kind:        ast.TypeInt,
					DecimalPrec: &precVal,
				}
			}(),
			wantErr: ast.ErrDecimalParamsNotSupported,
		},
		{
			name: "JoinClause valid inner",
			node: &ast.JoinClause{
				Type:  ast.JoinInner,
				Right: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}},
				On:    &ast.ExprCondition{Expr: &ast.IntegerLiteral{Value: "1"}},
			},
			wantErr: nil,
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
			name: "JoinClause cross with ON",
			node: &ast.JoinClause{
				Type:  ast.JoinCross,
				Right: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}},
				On:    &ast.ExprCondition{Expr: &ast.IntegerLiteral{Value: "1"}},
			},
			wantErr: ast.ErrCrossJoinWithOn,
		},
		{
			name: "LimitClause negative count",
			node: &ast.LimitClause{
				Count: -1,
			},
			wantErr: ast.ErrNegativeLimitCount,
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
