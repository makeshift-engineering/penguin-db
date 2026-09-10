package planner

import (
	"errors"

	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
)

// columnRefsEqual reports whether two resolved columns refer to the exact
// same physical column — used to check whether a SELECT-list or HAVING
// column reference matches one of the query's GROUP BY keys.
func columnRefsEqual(a, b *ResolvedColumn) bool {
	return a.Database == b.Database && a.Table == b.Table && a.Name == b.Name && a.Index == b.Index
}

// exprHasAggregate reports whether expr contains an aggregate function call
// anywhere in its tree. Used both to decide whether a SELECT statement is
// an aggregate query at all, and to reject aggregates used inside WHERE.
func exprHasAggregate(expr ResolvedExpr) bool {
	switch e := expr.(type) {
	case *ResolvedFunctionCall:
		return true
	case *ResolvedBinaryExpr:
		return exprHasAggregate(e.Left) || exprHasAggregate(e.Right)
	case *ResolvedUnaryExpr:
		return exprHasAggregate(e.Operand)
	case *ResolvedConditionExpr:
		return condHasAggregate(e.Cond)
	default:
		return false // literals and column references never contain one
	}
}

// errStopWalk is an internal sentinel error to halt tree traversal early.
var errStopWalk = errors.New("stop walk")

// walkCond traverses a condition tree, invoking fnExpr on child expressions
// and fnCond on child conditions.
func walkCond(cond ResolvedCond, fnExpr func(ResolvedExpr) error, fnCond func(ResolvedCond) error) error {
	switch c := cond.(type) {
	case *ResolvedComparison:
		if err := fnExpr(c.Left); err != nil {
			return err
		}
		return fnExpr(c.Right)
	case *ResolvedLike:
		if err := fnExpr(c.Left); err != nil {
			return err
		}
		return fnExpr(c.Pattern)
	case *ResolvedIsNull:
		return fnExpr(c.Expr)
	case *ResolvedIn:
		if err := fnExpr(c.Expr); err != nil {
			return err
		}
		for _, v := range c.Values {
			if err := fnExpr(v); err != nil {
				return err
			}
		}
		return nil
	case *ResolvedBetween:
		if err := fnExpr(c.Expr); err != nil {
			return err
		}
		if err := fnExpr(c.Low); err != nil {
			return err
		}
		return fnExpr(c.High)
	case *ResolvedBinaryCond:
		if err := fnCond(c.Left); err != nil {
			return err
		}
		return fnCond(c.Right)
	case *ResolvedNotCond:
		return fnCond(c.Operand)
	case *ResolvedExprCond:
		return fnExpr(c.Expr)
	default:
		return nil
	}
}

// condHasAggregate is exprHasAggregate for a resolved condition tree.
func condHasAggregate(cond ResolvedCond) bool {
	hasAgg := false
	_ = walkCond(cond,
		func(e ResolvedExpr) error {
			if exprHasAggregate(e) {
				hasAgg = true
				return errStopWalk
			}
			return nil
		},
		func(c ResolvedCond) error {
			if condHasAggregate(c) {
				hasAgg = true
				return errStopWalk
			}
			return nil
		},
	)
	return hasAgg
}

// anyItemHasAggregate reports whether any SELECT-list item contains an
// aggregate function call — one of the two triggers (along with an
// explicit GROUP BY clause) that make a SELECT statement an aggregate
// query.
func anyItemHasAggregate(items []ProjectItem) bool {
	for _, item := range items {
		if exprHasAggregate(item.Expr) {
			return true
		}
	}
	return false
}

// validateGroupedExpr checks that expr is legal inside an aggregate
// query's SELECT list or HAVING clause: built entirely from aggregate
// function calls (whose own arguments don't need to satisfy this — once
// inside an aggregate, any column reference is fine, since it's the thing
// being aggregated), references to GROUP BY key columns, and literal
// constants. A composite expression (e.g. price * 1.1) is fine as long as
// every leaf independently satisfies one of those rules — so e.g. `a + 1`
// is legal when `a` is a GROUP BY key, since `1` is a literal.
//
// When groupKeys is empty (aggregate functions present but no GROUP BY
// clause — SQL's "single implicit group" case), no bare column reference
// can ever pass, which is exactly correct: there is no grouping key for it
// to align with.
func (pc *planContext) validateGroupedExpr(expr ResolvedExpr, groupKeys []ResolvedColumn, span diagnostic.Span) error {
	switch e := expr.(type) {
	case *ResolvedFunctionCall:
		return nil
	case *ResolvedColumnRef:
		for i := range groupKeys {
			if columnRefsEqual(&e.Column, &groupKeys[i]) {
				return nil
			}
		}
		return pc.errorf(
			span, CodeMissingGroupBy,
			"column %q must appear in the GROUP BY clause or be used inside an aggregate function", e.Column.Name,
		)
	case *ResolvedBinaryExpr:
		if err := pc.validateGroupedExpr(e.Left, groupKeys, span); err != nil {
			return err
		}
		return pc.validateGroupedExpr(e.Right, groupKeys, span)
	case *ResolvedUnaryExpr:
		return pc.validateGroupedExpr(e.Operand, groupKeys, span)
	case *ResolvedConditionExpr:
		return pc.validateGroupedCond(e.Cond, groupKeys, span)
	default:
		return nil // literals: always fine regardless of grouping
	}
}

// validateGroupedCond is validateGroupedExpr for a resolved condition tree
// — used for HAVING, and for a ResolvedExprCond nested inside a
// SELECT-list item.
func (pc *planContext) validateGroupedCond(cond ResolvedCond, groupKeys []ResolvedColumn, span diagnostic.Span) error {
	return walkCond(cond,
		func(e ResolvedExpr) error { return pc.validateGroupedExpr(e, groupKeys, span) },
		func(c ResolvedCond) error { return pc.validateGroupedCond(c, groupKeys, span) },
	)
}
