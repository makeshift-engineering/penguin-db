package planner

// ResolvedCond is the sealed interface for a fully resolved boolean
// condition. Unlike ResolvedExpr, it carries no type field: every
// ResolvedCond is boolean by definition.
type ResolvedCond interface {
	resolvedCondNode()
}

// ResolvedCondBase is embedded in every ResolvedCond implementation to
// satisfy the interface's marker method.
type ResolvedCondBase struct{}

func (*ResolvedCondBase) resolvedCondNode() {}

// ResolvedBinaryCond joins two conditions with AND or OR.
type ResolvedBinaryCond struct {
	ResolvedCondBase
	Left  ResolvedCond
	Op    Op // OpAnd or OpOr
	Right ResolvedCond
}

// ResolvedNotCond negates a condition.
type ResolvedNotCond struct {
	ResolvedCondBase
	Operand ResolvedCond
}

// ResolvedComparison is a resolved comparison between two expressions:
// Left Op Right where Op is one of =, !=, <>, <, >, <=, >=.
type ResolvedComparison struct {
	ResolvedCondBase
	Left  ResolvedExpr
	Op    Op
	Right ResolvedExpr
}

// ResolvedLike is a resolved [NOT] LIKE predicate.
type ResolvedLike struct {
	ResolvedCondBase
	Left    ResolvedExpr
	Pattern ResolvedExpr
	Negated bool
}

// ResolvedIsNull is a resolved IS [NOT] NULL predicate.
type ResolvedIsNull struct {
	ResolvedCondBase
	Expr    ResolvedExpr
	Negated bool
}

// ResolvedIn is a resolved [NOT] IN predicate.
type ResolvedIn struct {
	ResolvedCondBase
	Expr    ResolvedExpr
	Values  []ResolvedExpr
	Negated bool
}

// ResolvedBetween is a resolved [NOT] BETWEEN predicate.
type ResolvedBetween struct {
	ResolvedCondBase
	Expr    ResolvedExpr
	Low     ResolvedExpr
	High    ResolvedExpr
	Negated bool
}

// ResolvedExprCond wraps a bare boolean expression used as a condition,
// e.g. a boolean column reference used directly in a WHERE clause.
// ast.ParenCondition has no resolved counterpart, for the same reason
// ResolvedExpr has none for ast.ParenExpr: resolution unwraps it.
type ResolvedExprCond struct {
	ResolvedCondBase
	Expr ResolvedExpr
}
