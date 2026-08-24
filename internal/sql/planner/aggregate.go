package planner

import "github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"

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

// condHasAggregate is exprHasAggregate for a resolved condition tree.
func condHasAggregate(cond ResolvedCond) bool {
	switch c := cond.(type) {
	case *ResolvedComparison:
		return exprHasAggregate(c.Left) || exprHasAggregate(c.Right)
	case *ResolvedLike:
		return exprHasAggregate(c.Left) || exprHasAggregate(c.Pattern)
	case *ResolvedIsNull:
		return exprHasAggregate(c.Expr)
	case *ResolvedIn:
		if exprHasAggregate(c.Expr) {
			return true
		}
		for _, v := range c.Values {
			if exprHasAggregate(v) {
				return true
			}
		}
		return false
	case *ResolvedBetween:
		return exprHasAggregate(c.Expr) || exprHasAggregate(c.Low) || exprHasAggregate(c.High)
	case *ResolvedBinaryCond:
		return condHasAggregate(c.Left) || condHasAggregate(c.Right)
	case *ResolvedNotCond:
		return condHasAggregate(c.Operand)
	case *ResolvedExprCond:
		return exprHasAggregate(c.Expr)
	default:
		return false
	}
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
	switch c := cond.(type) {
	case *ResolvedComparison:
		if err := pc.validateGroupedExpr(c.Left, groupKeys, span); err != nil {
			return err
		}
		return pc.validateGroupedExpr(c.Right, groupKeys, span)
	case *ResolvedLike:
		if err := pc.validateGroupedExpr(c.Left, groupKeys, span); err != nil {
			return err
		}
		return pc.validateGroupedExpr(c.Pattern, groupKeys, span)
	case *ResolvedIsNull:
		return pc.validateGroupedExpr(c.Expr, groupKeys, span)
	case *ResolvedIn:
		if err := pc.validateGroupedExpr(c.Expr, groupKeys, span); err != nil {
			return err
		}
		for _, v := range c.Values {
			if err := pc.validateGroupedExpr(v, groupKeys, span); err != nil {
				return err
			}
		}
		return nil
	case *ResolvedBetween:
		if err := pc.validateGroupedExpr(c.Expr, groupKeys, span); err != nil {
			return err
		}
		if err := pc.validateGroupedExpr(c.Low, groupKeys, span); err != nil {
			return err
		}
		return pc.validateGroupedExpr(c.High, groupKeys, span)
	case *ResolvedBinaryCond:
		if err := pc.validateGroupedCond(c.Left, groupKeys, span); err != nil {
			return err
		}
		return pc.validateGroupedCond(c.Right, groupKeys, span)
	case *ResolvedNotCond:
		return pc.validateGroupedCond(c.Operand, groupKeys, span)
	case *ResolvedExprCond:
		return pc.validateGroupedExpr(c.Expr, groupKeys, span)
	default:
		return nil
	}
}
