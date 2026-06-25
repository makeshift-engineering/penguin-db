package ast

import "github.com/makeshift-engineering/penguin-db/internal/sql/lexer"

// BinaryCondition represents two conditions joined by AND or OR.
type BinaryCondition struct {
	CondBase
	Left  Condition
	Op    lexer.TokenType
	Right Condition
}

// NotCondition represents the logical negation of a condition: NOT Operand.
type NotCondition struct {
	CondBase
	Operand Condition
}

// ComparisonPredicate represents a comparison between two expressions
// using one of =, !=, <>, <, >, <=, >=.
type ComparisonPredicate struct {
	CondBase
	Left  Expression
	Op    lexer.TokenType
	Right Expression
}

// LikePredicate represents a pattern-matching predicate: Left [NOT] LIKE Pattern.
// Negated is true for NOT LIKE.
type LikePredicate struct {
	CondBase
	Left    Expression
	Pattern Expression
	Negated bool
}

// IsNullPredicate represents a null check: Expr IS [NOT] NULL.
// Negated is true for IS NOT NULL.
type IsNullPredicate struct {
	CondBase
	Expr    Expression
	Negated bool
}

// InPredicate represents a set membership test: Expr [NOT] IN (Values...).
// Negated is true for NOT IN.
type InPredicate struct {
	CondBase
	Expr    Expression
	Values  []Expression
	Negated bool
}

// BetweenPredicate represents a range test: Expr [NOT] BETWEEN Low AND High.
// Negated is true for NOT BETWEEN.
type BetweenPredicate struct {
	CondBase
	Expr    Expression
	Low     Expression
	High    Expression
	Negated bool
}

// ParenCondition represents a parenthesized condition: ( Inner ).
type ParenCondition struct {
	CondBase
	Inner Condition
}

// ExprCondition wraps a bare [Expression] used as a truth value in a
// condition context (e.g. a boolean column reference in a WHERE clause).
type ExprCondition struct {
	CondBase
	Expr Expression
}
