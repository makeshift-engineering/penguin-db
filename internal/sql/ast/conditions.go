package ast

import "github.com/makeshift-engineering/penguin-db/internal/sql/lexer"

// BinaryCondition represents two conditions joined by AND or OR.
type BinaryCondition struct {
	CondBase
	Left  Condition
	Op    lexer.TokenType
	Right Condition
}

func (b *BinaryCondition) Validate() error {
	if b.Left == nil || b.Right == nil {
		return ErrNilCondition
	}
	if b.Op != lexer.TOKEN_AND && b.Op != lexer.TOKEN_OR {
		return ErrInvalidConditionOperator
	}
	if err := b.Left.Validate(); err != nil {
		return err
	}
	return b.Right.Validate()
}

// NotCondition represents the logical negation of a condition: NOT Operand.
type NotCondition struct {
	CondBase
	Operand Condition
}

func (n *NotCondition) Validate() error {
	if n.Operand == nil {
		return ErrNilCondition
	}
	return n.Operand.Validate()
}

// ComparisonPredicate represents a comparison between two expressions
// using one of =, !=, <>, <, >, <=, >=.
type ComparisonPredicate struct {
	CondBase
	Left  Expression
	Op    lexer.TokenType
	Right Expression
}

func (c *ComparisonPredicate) Validate() error {
	if c.Left == nil || c.Right == nil {
		return ErrNilExpression
	}
	switch c.Op {
	case lexer.TOKEN_EQ, lexer.TOKEN_NEQ, lexer.TOKEN_LT, lexer.TOKEN_GT, lexer.TOKEN_LTE, lexer.TOKEN_GTE:
	default:
		return ErrInvalidComparisonOperator
	}
	if err := c.Left.Validate(); err != nil {
		return err
	}
	return c.Right.Validate()
}

// LikePredicate represents a pattern-matching predicate: Left [NOT] LIKE Pattern.
// Negated is true for NOT LIKE.
type LikePredicate struct {
	CondBase
	Left    Expression
	Pattern Expression
	Negated bool
}

func (l *LikePredicate) Validate() error {
	if l.Left == nil || l.Pattern == nil {
		return ErrNilExpression
	}
	if err := l.Left.Validate(); err != nil {
		return err
	}
	return l.Pattern.Validate()
}

// IsNullPredicate represents a null check: Expr IS [NOT] NULL.
// Negated is true for IS NOT NULL.
type IsNullPredicate struct {
	CondBase
	Expr    Expression
	Negated bool
}

func (i *IsNullPredicate) Validate() error {
	if i.Expr == nil {
		return ErrNilExpression
	}
	return i.Expr.Validate()
}

// InPredicate represents a set membership test: Expr [NOT] IN (Values...).
// Negated is true for NOT IN.
type InPredicate struct {
	CondBase
	Expr    Expression
	Values  []Expression
	Negated bool
}

func (i *InPredicate) Validate() error {
	if i.Expr == nil {
		return ErrNilExpression
	}
	if err := i.Expr.Validate(); err != nil {
		return err
	}
	for _, val := range i.Values {
		if val == nil {
			return ErrNilExpression
		}
		if err := val.Validate(); err != nil {
			return err
		}
	}
	return nil
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

func (b *BetweenPredicate) Validate() error {
	if b.Expr == nil || b.Low == nil || b.High == nil {
		return ErrNilExpression
	}
	if err := b.Expr.Validate(); err != nil {
		return err
	}
	if err := b.Low.Validate(); err != nil {
		return err
	}
	return b.High.Validate()
}

// ParenCondition represents a parenthesized condition: ( Inner ).
type ParenCondition struct {
	CondBase
	Inner Condition
}

func (p *ParenCondition) Validate() error {
	if p.Inner == nil {
		return ErrNilCondition
	}
	return p.Inner.Validate()
}

// ExprCondition wraps a bare [Expression] used as a truth value in a
// condition context (e.g. a boolean column reference in a WHERE clause).
type ExprCondition struct {
	CondBase
	Expr Expression
}

func (e *ExprCondition) Validate() error {
	if e.Expr == nil {
		return ErrNilExpression
	}
	return e.Expr.Validate()
}
