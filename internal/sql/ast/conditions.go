package ast

import "github.com/makeshift-engineering/penguin-db/internal/sql/lexer"

type BinaryCondition struct {
	CondBase
	Left  Condition
	Op    lexer.TokenType
	Right Condition
}

type NotCondition struct {
	CondBase
	Operand Condition
}

type ComparisonPredicate struct {
	CondBase
	Left  Expression
	Op    lexer.TokenType
	Right Expression
}

type LikePredicate struct {
	CondBase
	Left    Expression
	Pattern Expression
	Negated bool
}

type IsNullPredicate struct {
	CondBase
	Expr    Expression
	Negated bool
}

type InPredicate struct {
	CondBase
	Expr    Expression
	Values  []Expression
	Negated bool
}

type BetweenPredicate struct {
	CondBase
	Expr    Expression
	Low     Expression
	High    Expression
	Negated bool
}

type ParenCondition struct {
	CondBase
	Inner Condition
}

type ExprCondition struct {
	CondBase
	Expr Expression
}
