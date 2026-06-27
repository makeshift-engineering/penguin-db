package ast_test

import (
	"errors"
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/lexer"
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

func TestCondition_Validation(t *testing.T) {
	tests := []struct {
		name    string
		node    ast.Node
		wantErr error
	}{
		{
			name: "BinaryCondition valid",
			node: &ast.BinaryCondition{
				Left:  &ast.ExprCondition{Expr: &ast.IntegerLiteral{Value: "1"}},
				Op:    lexer.TOKEN_AND,
				Right: &ast.ExprCondition{Expr: &ast.IntegerLiteral{Value: "2"}},
			},
			wantErr: nil,
		},
		{
			name: "BinaryCondition invalid operator",
			node: &ast.BinaryCondition{
				Left:  &ast.ExprCondition{Expr: &ast.IntegerLiteral{Value: "1"}},
				Op:    lexer.TOKEN_PLUS,
				Right: &ast.ExprCondition{Expr: &ast.IntegerLiteral{Value: "2"}},
			},
			wantErr: ast.ErrInvalidConditionOperator,
		},
		{
			name: "ComparisonPredicate valid",
			node: &ast.ComparisonPredicate{
				Left:  &ast.IntegerLiteral{Value: "1"},
				Op:    lexer.TOKEN_EQ,
				Right: &ast.IntegerLiteral{Value: "1"},
			},
			wantErr: nil,
		},
		{
			name: "ComparisonPredicate invalid operator",
			node: &ast.ComparisonPredicate{
				Left:  &ast.IntegerLiteral{Value: "1"},
				Op:    lexer.TOKEN_AND,
				Right: &ast.IntegerLiteral{Value: "1"},
			},
			wantErr: ast.ErrInvalidComparisonOperator,
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
