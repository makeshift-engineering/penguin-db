package ast

import "github.com/makeshift-engineering/penguin-db/internal/sql/lexer"

// IntegerLiteral represents a whole-number literal (e.g. 42).
type IntegerLiteral struct {
	ExprBase
	Value string
}

// FloatLiteral represents a fractional-number literal (e.g. 3.14, .5, 10.).
type FloatLiteral struct {
	ExprBase
	Value string
}

// StringLiteral represents a single-quoted string value (e.g. 'hello').
type StringLiteral struct {
	ExprBase
	Value string
}

// BooleanLiteral represents a TRUE or FALSE keyword.
type BooleanLiteral struct {
	ExprBase
	Value bool
}

// NullLiteral represents the NULL keyword.
type NullLiteral struct {
	ExprBase
}

// Identifier represents a simple or dot-qualified SQL name.
// For unqualified names (e.g. "id"), Qualifier is empty.
// For qualified names (e.g. "users.id"), Qualifier holds the prefix.
type Identifier struct {
	ExprBase
	Name      string
	Qualifier string
}

// BinaryExpr represents an infix arithmetic expression: Left Op Right.
// Op is one of TOKEN_PLUS, TOKEN_MINUS, TOKEN_STAR, TOKEN_SLASH, or TOKEN_PERCENT.
type BinaryExpr struct {
	ExprBase
	Left  Expression
	Op    lexer.TokenType
	Right Expression
}

// UnaryExpr represents a prefix unary expression: (+|-) Operand.
type UnaryExpr struct {
	ExprBase
	Op      lexer.TokenType
	Operand Expression
}

// ParenExpr represents a parenthesized expression: ( Inner ).
type ParenExpr struct {
	ExprBase
	Inner Expression
}

// FunctionCall represents a SQL function invocation: Name( Args ).
// Star is true for COUNT(*). Distinct is true for aggregate calls
// like COUNT(DISTINCT col).
type FunctionCall struct {
	ExprBase
	Name     string
	Distinct bool
	Args     []*SelectExpression
	Star     bool
}

// SelectExpression is a tagged union that can hold either an arithmetic
// [Expression] or a boolean [Condition]. It appears in SELECT column lists
// and function arguments. Exactly one of Expr or Cond is non-nil.
type SelectExpression struct {
	NodeBase
	Expr Expression
	Cond Condition
}
