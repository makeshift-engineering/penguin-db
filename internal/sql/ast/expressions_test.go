package ast_test

import (
	"errors"
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

var (
	_ ast.Expression = (*ast.IntegerLiteral)(nil)
	_ ast.Expression = (*ast.FloatLiteral)(nil)
	_ ast.Expression = (*ast.StringLiteral)(nil)
	_ ast.Expression = (*ast.BooleanLiteral)(nil)
	_ ast.Expression = (*ast.NullLiteral)(nil)
	_ ast.Expression = (*ast.Identifier)(nil)
	_ ast.Expression = (*ast.BinaryExpr)(nil)
	_ ast.Expression = (*ast.UnaryExpr)(nil)
	_ ast.Expression = (*ast.ParenExpr)(nil)
	_ ast.Expression = (*ast.FunctionCall)(nil)
)

var _ ast.Node = (*ast.SelectExpression)(nil)

// TestExpression_TypeSwitchCoverage verifies that every concrete Expression
// type is handled in a type-switch, guarding against missing cases.
func TestExpression_TypeSwitchCoverage(t *testing.T) {
	exprs := []ast.Expression{
		&ast.IntegerLiteral{},
		&ast.FloatLiteral{},
		&ast.StringLiteral{},
		&ast.BooleanLiteral{},
		&ast.NullLiteral{},
		&ast.Identifier{},
		&ast.BinaryExpr{},
		&ast.UnaryExpr{},
		&ast.ParenExpr{},
		&ast.FunctionCall{},
	}

	for _, e := range exprs {
		switch e.(type) {
		case *ast.IntegerLiteral:
		case *ast.FloatLiteral:
		case *ast.StringLiteral:
		case *ast.BooleanLiteral:
		case *ast.NullLiteral:
		case *ast.Identifier:
		case *ast.BinaryExpr:
		case *ast.UnaryExpr:
		case *ast.ParenExpr:
		case *ast.FunctionCall:
		default:
			t.Errorf("unhandled Expression type: %T", e)
		}
	}
}

// TestExpression_Validation exercises the Validate method on all Expression
// node types, covering valid inputs, nil fields, invalid operators, recursive
// validation propagation, and SelectExpression/FunctionCall edge cases.
func TestExpression_Validation(t *testing.T) {
	tests := []struct {
		name    string
		node    ast.Node
		wantErr error
	}{
		{
			name:    "Identifier valid",
			node:    &ast.Identifier{Name: "col"},
			wantErr: nil,
		},
		{
			name:    "Identifier empty name",
			node:    &ast.Identifier{Name: ""},
			wantErr: ast.ErrEmptyIdentifierName,
		},
		{
			name: "BinaryExpr valid",
			node: &ast.BinaryExpr{
				Left:  &ast.IntegerLiteral{Value: "1"},
				Op:    utils.TOKEN_PLUS,
				Right: &ast.IntegerLiteral{Value: "2"},
			},
			wantErr: nil,
		},
		{
			name: "BinaryExpr nil left",
			node: &ast.BinaryExpr{
				Op:    utils.TOKEN_PLUS,
				Right: &ast.IntegerLiteral{Value: "2"},
			},
			wantErr: ast.ErrNilExpression,
		},
		{
			name: "BinaryExpr invalid operator",
			node: &ast.BinaryExpr{
				Left:  &ast.IntegerLiteral{Value: "1"},
				Op:    utils.TOKEN_AND,
				Right: &ast.IntegerLiteral{Value: "2"},
			},
			wantErr: ast.ErrInvalidBinaryOperator,
		},
		{
			name: "BinaryExpr recursive error",
			node: &ast.BinaryExpr{
				Left:  &ast.Identifier{Name: ""},
				Op:    utils.TOKEN_PLUS,
				Right: &ast.IntegerLiteral{Value: "2"},
			},
			wantErr: ast.ErrEmptyIdentifierName,
		},
		{
			name: "UnaryExpr valid",
			node: &ast.UnaryExpr{
				Op:      utils.TOKEN_MINUS,
				Operand: &ast.IntegerLiteral{Value: "5"},
			},
			wantErr: nil,
		},
		{
			name: "UnaryExpr nil operand",
			node: &ast.UnaryExpr{
				Op: utils.TOKEN_MINUS,
			},
			wantErr: ast.ErrNilExpression,
		},
		{
			name: "UnaryExpr invalid operator",
			node: &ast.UnaryExpr{
				Op:      utils.TOKEN_NOT,
				Operand: &ast.IntegerLiteral{Value: "5"},
			},
			wantErr: ast.ErrInvalidUnaryOperator,
		},
		{
			name: "ParenExpr valid",
			node: &ast.ParenExpr{
				Inner: &ast.IntegerLiteral{Value: "5"},
			},
			wantErr: nil,
		},
		{
			name:    "ParenExpr nil inner",
			node:    &ast.ParenExpr{},
			wantErr: ast.ErrNilExpression,
		},
		{
			name: "FunctionCall valid star",
			node: &ast.FunctionCall{
				Name: "COUNT",
				Star: true,
			},
			wantErr: nil,
		},
		{
			name: "FunctionCall invalid star with args",
			node: &ast.FunctionCall{
				Name: "COUNT",
				Star: true,
				Args: []*ast.SelectExpression{
					{Expr: &ast.IntegerLiteral{Value: "1"}},
				},
			},
			wantErr: ast.ErrStarFunctionArgs,
		},
		{
			name: "FunctionCall empty name",
			node: &ast.FunctionCall{
				Name: "",
			},
			wantErr: ast.ErrEmptyFunctionName,
		},
		{
			name: "SelectExpression valid expr",
			node: &ast.SelectExpression{
				Expr: &ast.IntegerLiteral{Value: "1"},
			},
			wantErr: nil,
		},
		{
			name: "SelectExpression valid cond",
			node: &ast.SelectExpression{
				Cond: &ast.ExprCondition{Expr: &ast.IntegerLiteral{Value: "1"}},
			},
			wantErr: nil,
		},
		{
			name:    "SelectExpression both nil",
			node:    &ast.SelectExpression{},
			wantErr: ast.ErrInvalidSelectExpression,
		},
		{
			name: "SelectExpression both set",
			node: &ast.SelectExpression{
				Expr: &ast.IntegerLiteral{Value: "1"},
				Cond: &ast.ExprCondition{Expr: &ast.IntegerLiteral{Value: "1"}},
			},
			wantErr: ast.ErrInvalidSelectExpression,
		},
		{
			name: "FunctionCall valid args",
			node: &ast.FunctionCall{
				Name: "SUM",
				Args: []*ast.SelectExpression{{Expr: &ast.IntegerLiteral{Value: "1"}}},
			},
			wantErr: nil,
		},
		{
			name: "FunctionCall nil arg",
			node: &ast.FunctionCall{
				Name: "SUM",
				Args: []*ast.SelectExpression{nil},
			},
			wantErr: ast.ErrNilExpression,
		},
		{
			name: "FunctionCall distinct valid",
			node: &ast.FunctionCall{
				Name:     "COUNT",
				Distinct: true,
				Args:     []*ast.SelectExpression{{Expr: &ast.Identifier{Name: "col"}}},
			},
			wantErr: nil,
		},
		{
			name: "BinaryExpr nil right",
			node: &ast.BinaryExpr{
				Left: &ast.IntegerLiteral{Value: "1"},
				Op:   utils.TOKEN_PLUS,
			},
			wantErr: ast.ErrNilExpression,
		},
		{
			name: "BinaryExpr recursive right error",
			node: &ast.BinaryExpr{
				Left:  &ast.IntegerLiteral{Value: "1"},
				Op:    utils.TOKEN_PLUS,
				Right: &ast.Identifier{Name: ""},
			},
			wantErr: ast.ErrEmptyIdentifierName,
		},
		{
			name: "UnaryExpr recursive error",
			node: &ast.UnaryExpr{
				Op:      utils.TOKEN_MINUS,
				Operand: &ast.Identifier{Name: ""},
			},
			wantErr: ast.ErrEmptyIdentifierName,
		},
		{
			name: "ParenExpr recursive error",
			node: &ast.ParenExpr{
				Inner: &ast.Identifier{Name: ""},
			},
			wantErr: ast.ErrEmptyIdentifierName,
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
