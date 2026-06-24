package ast

import "github.com/makeshift-engineering/penguin-db/internal/sql/lexer"

type IntegerLiteral struct {
	ExprBase
	Value string
}

type FloatLiteral struct {
	ExprBase
	Value string
}

type StringLiteral struct {
	ExprBase
	Value string
}

type BooleanLiteral struct {
	ExprBase
	Value bool
}

type NullLiteral struct {
	ExprBase
}

type Identifier struct {
	ExprBase
	Name      string
	Qualifier string
}

type BinaryExpr struct {
	ExprBase
	Left  Expression
	Op    lexer.TokenType
	Right Expression
}

type UnaryExpr struct {
	ExprBase
	Op      lexer.TokenType
	Operand Expression
}

type ParenExpr struct {
	ExprBase
	Inner Expression
}

type FunctionCall struct {
	ExprBase
	Name     string
	Distinct bool
	Args     []*SelectExpression
	Star     bool
}

type SelectExpression struct {
	NodeBase
	Expr Expression
	Cond Condition
}
