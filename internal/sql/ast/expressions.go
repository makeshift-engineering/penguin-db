package ast

import "github.com/makeshift-engineering/penguin-db/internal/sql/utils"

// IntegerLiteral represents a whole-number literal (e.g. 42).
type IntegerLiteral struct {
	ExprBase
	Value string
}

// FloatLiteral represents a fractional-number literal (e.g. 3.14, .5).
type FloatLiteral struct {
	ExprBase
	Value string
}

// StringLiteral represents a single-quoted string value (e.g. 'hello').
// The lexer strips the surrounding quotes and decodes SQL escape sequences
// before storing the value here.
type StringLiteral struct {
	ExprBase
	Value string
}

// BooleanLiteral represents the TRUE or FALSE keyword.
type BooleanLiteral struct {
	ExprBase
	Value string // "TRUE" or "FALSE" — kept as string to preserve original casing
}

// NullLiteral represents the NULL keyword.
type NullLiteral struct {
	ExprBase
}

// Identifier represents a simple or dot-qualified SQL name.
//
//	users.id  →  Identifier{Name: "id", Qualifier: "users"}
//	id        →  Identifier{Name: "id", Qualifier: ""}
type Identifier struct {
	ExprBase
	Name      string
	Qualifier string
}

func (i *Identifier) Validate() error {
	if i.Name == "" {
		return ErrEmptyIdentifierName
	}
	return nil
}

// BinaryExpr represents an infix arithmetic expression: Left Op Right.
// Op is one of TOKEN_PLUS, TOKEN_MINUS, TOKEN_STAR, TOKEN_SLASH, TOKEN_PERCENT.
type BinaryExpr struct {
	ExprBase
	Left  Expression
	Op    utils.TokenType
	Right Expression
}

func (b *BinaryExpr) Validate() error {
	if b.Left == nil || b.Right == nil {
		return ErrNilExpression
	}
	switch b.Op {
	case utils.TOKEN_PLUS, utils.TOKEN_MINUS,
		utils.TOKEN_STAR, utils.TOKEN_SLASH, utils.TOKEN_PERCENT:
	default:
		return ErrInvalidBinaryOperator
	}
	if err := b.Left.Validate(); err != nil {
		return err
	}
	return b.Right.Validate()
}

// UnaryExpr represents a prefix unary expression: Op Operand.
// Op is TOKEN_PLUS or TOKEN_MINUS.
type UnaryExpr struct {
	ExprBase
	Op      utils.TokenType
	Operand Expression
}

func (u *UnaryExpr) Validate() error {
	if u.Operand == nil {
		return ErrNilExpression
	}
	switch u.Op {
	case utils.TOKEN_PLUS, utils.TOKEN_MINUS:
	default:
		return ErrInvalidUnaryOperator
	}
	return u.Operand.Validate()
}

// ParenExpr represents a parenthesised arithmetic expression: ( Inner ).
type ParenExpr struct {
	ExprBase
	Inner Expression
}

func (p *ParenExpr) Validate() error {
	if p.Inner == nil {
		return ErrNilExpression
	}
	return p.Inner.Validate()
}

// FunctionCall represents a SQL function invocation: Name( Args ).
// Star is true for COUNT(*). Distinct is true for aggregate calls like
// COUNT(DISTINCT col). Star and Distinct are mutually exclusive.
type FunctionCall struct {
	ExprBase
	Name     string
	Distinct bool
	Args     []*SelectExpression
	Star     bool
}

func (f *FunctionCall) Validate() error {
	if f.Name == "" {
		return ErrEmptyFunctionName
	}
	if f.Star && len(f.Args) > 0 {
		return ErrStarFunctionArgs
	}
	for _, arg := range f.Args {
		if arg == nil {
			return ErrNilExpression
		}
		if err := arg.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// SelectExpression is the tagged union that can hold either an arithmetic
// Expression or a boolean Condition. It appears in SELECT column lists and
// function arguments. Exactly one of Expr or Cond must be non-nil.
type SelectExpression struct {
	NodeBase
	Expr Expression
	Cond Condition
}

func (s *SelectExpression) Validate() error {
	if (s.Expr == nil) == (s.Cond == nil) {
		return ErrInvalidSelectExpression
	}
	if s.Expr != nil {
		return s.Expr.Validate()
	}
	return s.Cond.Validate()
}
