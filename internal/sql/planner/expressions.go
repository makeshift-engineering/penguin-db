package planner

import (
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

// ResolvedExpr is the sealed interface for a fully resolved, type-annotated
// arithmetic expression. Every concrete type carries the ast.DataTypeKind
// its value will have at execution time, computed once during resolution so
// a nesting expression never has to re-derive a child's type.
//
// ResolvedNullLiteral is the one exception: its ResolvedType return value is
// a placeholder and must not be relied on directly — check for
// *ResolvedNullLiteral (see isNullExpr) before comparing types.
type ResolvedExpr interface {
	resolvedExprNode()
	ResolvedType() ast.DataTypeKind
}

// ResolvedExprBase is embedded in every ResolvedExpr implementation whose
// result type is just a value set once at resolution time — it supplies
// both the marker method and ResolvedType() by storing that value directly.
// Types whose result type is computed rather than stored (ResolvedColumnRef,
// derived from its column; ResolvedNullLiteral, a meaningless placeholder;
// ResolvedConditionExpr, a fixed constant) implement ResolvedExpr directly
// instead of embedding this.
type ResolvedExprBase struct {
	Type ast.DataTypeKind
}

func (*ResolvedExprBase) resolvedExprNode()                {}
func (b *ResolvedExprBase) ResolvedType() ast.DataTypeKind { return b.Type }

// newExprBase is a shorthand for constructing a ResolvedExprBase with a
// given type, reducing the verbose ResolvedExprBase{Type: t} pattern that
// appears at every expression construction site.
func newExprBase(t ast.DataTypeKind) ResolvedExprBase {
	return ResolvedExprBase{Type: t}
}

// ResolvedIntLiteral is a resolved integer literal. Type is ast.TypeInt
// when the value fits a 32-bit signed range, otherwise ast.TypeBigInt.
type ResolvedIntLiteral struct {
	ResolvedExprBase
	Value int64
}

// ResolvedFloatLiteral is a resolved fractional literal. Type is always
// ast.TypeDouble.
type ResolvedFloatLiteral struct {
	ResolvedExprBase
	Value float64
}

// ResolvedStringLiteral is a resolved string literal. Type is always
// ast.TypeText.
type ResolvedStringLiteral struct {
	ResolvedExprBase
	Value string
}

// ResolvedBoolLiteral is a resolved TRUE/FALSE literal. Type is always
// ast.TypeBoolean.
type ResolvedBoolLiteral struct {
	ResolvedExprBase
	Value bool
}

// ResolvedNullLiteral is a resolved NULL literal. It is compatible with
// every type and every operator; callers must check for this type (via
// isNullExpr) before consulting ResolvedType, whose return value here is a
// meaningless placeholder.
type ResolvedNullLiteral struct{}

func (*ResolvedNullLiteral) resolvedExprNode()              {}
func (*ResolvedNullLiteral) ResolvedType() ast.DataTypeKind { return ast.TypeInt }

// ResolvedColumnRef is a resolved reference to a table column, produced by
// looking an ast.Identifier up in a Scope. Its type is read live off the
// resolved column rather than copied, so it doesn't use ResolvedExprBase.
type ResolvedColumnRef struct {
	Column *ResolvedColumn
}

func (*ResolvedColumnRef) resolvedExprNode()                {}
func (r *ResolvedColumnRef) ResolvedType() ast.DataTypeKind { return r.Column.Type }

// ResolvedBinaryExpr is a resolved infix arithmetic expression. Type is the
// promoted result type of Left and Right — see arithmeticResultType.
// ast.ParenExpr has no resolved counterpart: parentheses only affect parse
// precedence, so resolution unwraps them and returns the inner node as-is.
type ResolvedBinaryExpr struct {
	ResolvedExprBase
	Left  ResolvedExpr
	Op    utils.TokenType
	Right ResolvedExpr
}

// ResolvedUnaryExpr is a resolved prefix unary expression (+/-). Type
// matches the operand's type unchanged.
type ResolvedUnaryExpr struct {
	ResolvedExprBase
	Op      utils.TokenType
	Operand ResolvedExpr
}

// ResolvedFunctionCall is a resolved aggregate function invocation. Args is
// empty when Star is true (COUNT(*)); otherwise it holds exactly one
// resolved argument, since v1 only supports single-argument aggregates.
// Type is computed by aggregateResultType.
type ResolvedFunctionCall struct {
	ResolvedExprBase
	Name     string
	Distinct bool
	Star     bool
	Args     []ResolvedExpr
}

// ResolvedConditionExpr wraps a resolved boolean condition so it can be used
// wherever a ResolvedExpr is expected. This is the bridge for the
// tagged-union ast.SelectExpression: when its Cond side is set — a
// comparison or predicate used as a select-list item, function argument, or
// INSERT VALUES entry — resolution goes through resolveCond instead of
// resolveExpr, and the result is wrapped here so every other part of the
// planner can treat it like any other boolean-typed expression. Its type is
// a fixed constant, so it implements ResolvedExpr directly rather than
// embedding ResolvedExprBase.
type ResolvedConditionExpr struct {
	Cond ResolvedCond
}

func (*ResolvedConditionExpr) resolvedExprNode()              {}
func (*ResolvedConditionExpr) ResolvedType() ast.DataTypeKind { return ast.TypeBoolean }
