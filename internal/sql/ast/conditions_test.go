package ast_test

import (
	"errors"
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

var (
	_ ast.Condition = (*ast.BinaryCondition)(nil)
	_ ast.Condition = (*ast.NotCondition)(nil)
	_ ast.Condition = (*ast.ComparisonPredicate)(nil)
	_ ast.Condition = (*ast.LikePredicate)(nil)
	_ ast.Condition = (*ast.IsNullPredicate)(nil)
	_ ast.Condition = (*ast.InPredicate)(nil)
	_ ast.Condition = (*ast.BetweenPredicate)(nil)
	_ ast.Condition = (*ast.ParenCondition)(nil)
	_ ast.Condition = (*ast.ExprCondition)(nil)
)

// TestCondition_TypeSwitchCoverage verifies that every concrete Condition
// type is handled in a type-switch, guarding against missing cases.
func TestCondition_TypeSwitchCoverage(t *testing.T) {
	conditions := []ast.Condition{
		&ast.BinaryCondition{},
		&ast.NotCondition{},
		&ast.ComparisonPredicate{},
		&ast.LikePredicate{},
		&ast.IsNullPredicate{},
		&ast.InPredicate{},
		&ast.BetweenPredicate{},
		&ast.ParenCondition{},
		&ast.ExprCondition{},
	}

	for _, c := range conditions {
		switch c.(type) {
		case *ast.BinaryCondition:
		case *ast.NotCondition:
		case *ast.ComparisonPredicate:
		case *ast.LikePredicate:
		case *ast.IsNullPredicate:
		case *ast.InPredicate:
		case *ast.BetweenPredicate:
		case *ast.ParenCondition:
		case *ast.ExprCondition:
		default:
			t.Errorf("unhandled Condition type: %T", c)
		}
	}
}

// TestCondition_Validation exercises the Validate method on all Condition
// node types, covering valid inputs, nil operands, invalid operators,
// recursive validation propagation, and negated predicate variants.
func TestCondition_Validation(t *testing.T) {
	tests := []struct {
		name    string
		node    ast.Node
		wantErr error
	}{
		// BinaryCondition
		{
			name: "BinaryCondition valid AND",
			node: &ast.BinaryCondition{
				Left:  &ast.ExprCondition{Expr: &ast.IntegerLiteral{Value: "1"}},
				Op:    utils.TOKEN_AND,
				Right: &ast.ExprCondition{Expr: &ast.IntegerLiteral{Value: "2"}},
			},
			wantErr: nil,
		},
		{
			name: "BinaryCondition valid OR",
			node: &ast.BinaryCondition{
				Left:  &ast.ExprCondition{Expr: &ast.IntegerLiteral{Value: "1"}},
				Op:    utils.TOKEN_OR,
				Right: &ast.ExprCondition{Expr: &ast.IntegerLiteral{Value: "2"}},
			},
			wantErr: nil,
		},
		{
			name: "BinaryCondition nil left",
			node: &ast.BinaryCondition{
				Op:    utils.TOKEN_AND,
				Right: &ast.ExprCondition{Expr: &ast.IntegerLiteral{Value: "1"}},
			},
			wantErr: ast.ErrNilCondition,
		},
		{
			name: "BinaryCondition nil right",
			node: &ast.BinaryCondition{
				Left: &ast.ExprCondition{Expr: &ast.IntegerLiteral{Value: "1"}},
				Op:   utils.TOKEN_AND,
			},
			wantErr: ast.ErrNilCondition,
		},
		{
			name: "BinaryCondition invalid operator",
			node: &ast.BinaryCondition{
				Left:  &ast.ExprCondition{Expr: &ast.IntegerLiteral{Value: "1"}},
				Op:    utils.TOKEN_PLUS,
				Right: &ast.ExprCondition{Expr: &ast.IntegerLiteral{Value: "2"}},
			},
			wantErr: ast.ErrInvalidConditionOperator,
		},
		{
			name: "BinaryCondition recursive left error",
			node: &ast.BinaryCondition{
				Left:  &ast.ExprCondition{}, // nil Expr
				Op:    utils.TOKEN_AND,
				Right: &ast.ExprCondition{Expr: &ast.IntegerLiteral{Value: "1"}},
			},
			wantErr: ast.ErrNilExpression,
		},
		{
			name: "BinaryCondition recursive right error",
			node: &ast.BinaryCondition{
				Left:  &ast.ExprCondition{Expr: &ast.IntegerLiteral{Value: "1"}},
				Op:    utils.TOKEN_AND,
				Right: &ast.ExprCondition{}, // nil Expr
			},
			wantErr: ast.ErrNilExpression,
		},
		// NotCondition
		{
			name: "NotCondition valid",
			node: &ast.NotCondition{
				Operand: &ast.ExprCondition{Expr: &ast.IntegerLiteral{Value: "1"}},
			},
			wantErr: nil,
		},
		{
			name:    "NotCondition nil operand",
			node:    &ast.NotCondition{},
			wantErr: ast.ErrNilCondition,
		},
		{
			name: "NotCondition recursive error",
			node: &ast.NotCondition{
				Operand: &ast.ExprCondition{}, // nil Expr
			},
			wantErr: ast.ErrNilExpression,
		},
		// ComparisonPredicate
		{
			name: "ComparisonPredicate valid EQ",
			node: &ast.ComparisonPredicate{
				Left: &ast.IntegerLiteral{Value: "1"}, Op: utils.TOKEN_EQ,
				Right: &ast.IntegerLiteral{Value: "1"},
			},
			wantErr: nil,
		},
		{
			name: "ComparisonPredicate nil left",
			node: &ast.ComparisonPredicate{
				Op: utils.TOKEN_EQ, Right: &ast.IntegerLiteral{Value: "1"},
			},
			wantErr: ast.ErrNilExpression,
		},
		{
			name: "ComparisonPredicate nil right",
			node: &ast.ComparisonPredicate{
				Left: &ast.IntegerLiteral{Value: "1"}, Op: utils.TOKEN_EQ,
			},
			wantErr: ast.ErrNilExpression,
		},
		{
			name: "ComparisonPredicate invalid operator",
			node: &ast.ComparisonPredicate{
				Left: &ast.IntegerLiteral{Value: "1"}, Op: utils.TOKEN_AND,
				Right: &ast.IntegerLiteral{Value: "1"},
			},
			wantErr: ast.ErrInvalidComparisonOperator,
		},
		{
			name: "ComparisonPredicate recursive left error",
			node: &ast.ComparisonPredicate{
				Left: &ast.Identifier{Name: ""}, Op: utils.TOKEN_EQ,
				Right: &ast.IntegerLiteral{Value: "1"},
			},
			wantErr: ast.ErrEmptyIdentifierName,
		},
		// LikePredicate
		{
			name: "LikePredicate valid",
			node: &ast.LikePredicate{
				Left:    &ast.Identifier{Name: "name"},
				Pattern: &ast.StringLiteral{Value: "foo%"},
			},
			wantErr: nil,
		},
		{
			name:    "LikePredicate nil left",
			node:    &ast.LikePredicate{Pattern: &ast.StringLiteral{Value: "foo%"}},
			wantErr: ast.ErrNilExpression,
		},
		{
			name:    "LikePredicate nil pattern",
			node:    &ast.LikePredicate{Left: &ast.Identifier{Name: "n"}},
			wantErr: ast.ErrNilExpression,
		},
		{
			name: "LikePredicate recursive left error",
			node: &ast.LikePredicate{
				Left:    &ast.Identifier{Name: ""},
				Pattern: &ast.StringLiteral{Value: "x"},
			},
			wantErr: ast.ErrEmptyIdentifierName,
		},
		// IsNullPredicate
		{
			name:    "IsNullPredicate valid",
			node:    &ast.IsNullPredicate{Expr: &ast.Identifier{Name: "x"}},
			wantErr: nil,
		},
		{
			name:    "IsNullPredicate negated valid",
			node:    &ast.IsNullPredicate{Expr: &ast.Identifier{Name: "x"}, Negated: true},
			wantErr: nil,
		},
		{
			name:    "IsNullPredicate nil expr",
			node:    &ast.IsNullPredicate{},
			wantErr: ast.ErrNilExpression,
		},
		// InPredicate
		{
			name: "InPredicate valid",
			node: &ast.InPredicate{
				Expr:   &ast.Identifier{Name: "id"},
				Values: []ast.Expression{&ast.IntegerLiteral{Value: "1"}},
			},
			wantErr: nil,
		},
		{
			name: "InPredicate nil expr",
			node: &ast.InPredicate{
				Values: []ast.Expression{&ast.IntegerLiteral{Value: "1"}},
			},
			wantErr: ast.ErrNilExpression,
		},
		{
			name: "InPredicate nil value element",
			node: &ast.InPredicate{
				Expr:   &ast.Identifier{Name: "id"},
				Values: []ast.Expression{nil},
			},
			wantErr: ast.ErrNilExpression,
		},
		{
			name: "InPredicate recursive expr error",
			node: &ast.InPredicate{
				Expr:   &ast.Identifier{Name: ""},
				Values: []ast.Expression{&ast.IntegerLiteral{Value: "1"}},
			},
			wantErr: ast.ErrEmptyIdentifierName,
		},
		// BetweenPredicate
		{
			name: "BetweenPredicate valid",
			node: &ast.BetweenPredicate{
				Expr: &ast.Identifier{Name: "x"},
				Low:  &ast.IntegerLiteral{Value: "1"},
				High: &ast.IntegerLiteral{Value: "10"},
			},
			wantErr: nil,
		},
		{
			name: "BetweenPredicate nil expr",
			node: &ast.BetweenPredicate{
				Low: &ast.IntegerLiteral{Value: "1"}, High: &ast.IntegerLiteral{Value: "10"},
			},
			wantErr: ast.ErrNilExpression,
		},
		{
			name: "BetweenPredicate nil low",
			node: &ast.BetweenPredicate{
				Expr: &ast.Identifier{Name: "x"}, High: &ast.IntegerLiteral{Value: "10"},
			},
			wantErr: ast.ErrNilExpression,
		},
		{
			name: "BetweenPredicate nil high",
			node: &ast.BetweenPredicate{
				Expr: &ast.Identifier{Name: "x"}, Low: &ast.IntegerLiteral{Value: "1"},
			},
			wantErr: ast.ErrNilExpression,
		},
		// ParenCondition
		{
			name: "ParenCondition valid",
			node: &ast.ParenCondition{
				Inner: &ast.ExprCondition{Expr: &ast.IntegerLiteral{Value: "1"}},
			},
			wantErr: nil,
		},
		{
			name:    "ParenCondition nil inner",
			node:    &ast.ParenCondition{},
			wantErr: ast.ErrNilCondition,
		},
		// ExprCondition
		{
			name:    "ExprCondition valid",
			node:    &ast.ExprCondition{Expr: &ast.Identifier{Name: "active"}},
			wantErr: nil,
		},
		{
			name:    "ExprCondition nil expr",
			node:    &ast.ExprCondition{},
			wantErr: ast.ErrNilExpression,
		},
		{
			name:    "ExprCondition recursive error",
			node:    &ast.ExprCondition{Expr: &ast.Identifier{Name: ""}},
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
