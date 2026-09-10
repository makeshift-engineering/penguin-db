package planner

import (
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

// resolveCond resolves an ast.Condition against scope, producing a fully
// resolved ResolvedCond. Resolution errors are recorded as diagnostics on
// pc.
func (pc *planContext) resolveCond(scope *Scope, cond ast.Condition) (ResolvedCond, error) {
	switch c := cond.(type) {
	case *ast.BinaryCondition:
		return pc.resolveBinaryCond(scope, c)
	case *ast.NotCondition:
		operand, err := pc.resolveCond(scope, c.Operand)
		if err != nil {
			return nil, err
		}
		return &ResolvedNotCond{Operand: operand}, nil
	case *ast.ComparisonPredicate:
		return pc.resolveComparison(scope, c)
	case *ast.LikePredicate:
		return pc.resolveLike(scope, c)
	case *ast.IsNullPredicate:
		return pc.resolveIsNull(scope, c)
	case *ast.InPredicate:
		return pc.resolveIn(scope, c)
	case *ast.BetweenPredicate:
		return pc.resolveBetween(scope, c)
	case *ast.ParenCondition:
		return pc.resolveCond(scope, c.Inner)
	case *ast.ExprCondition:
		return pc.resolveExprCond(scope, c)
	default:
		return nil, pc.errorf(cond.Span(), CodeUnsupportedCondition, "unsupported condition type %T", cond)
	}
}

func (pc *planContext) resolveBinaryCond(scope *Scope, bc *ast.BinaryCondition) (ResolvedCond, error) {
	left, err := pc.resolveCond(scope, bc.Left)
	if err != nil {
		return nil, err
	}
	right, err := pc.resolveCond(scope, bc.Right)
	if err != nil {
		return nil, err
	}
	return &ResolvedBinaryCond{Left: left, Op: tokenToOp(bc.Op), Right: right}, nil
}

func (pc *planContext) typeMismatchErr(span ast.Node, left, right ResolvedExpr) error {
	return pc.errorf(span.Span(), CodeTypeMismatch, "cannot compare %s and %s", exprTypeName(left), exprTypeName(right))
}

func (pc *planContext) checkOrderableBound(expr, bound ResolvedExpr, span ast.Node) error {
	if !isNullExpr(expr) && !isNullExpr(bound) {
		if !typesOrderable(expr.ResolvedType(), bound.ResolvedType()) {
			if typesCompatible(expr.ResolvedType(), bound.ResolvedType()) {
				return pc.errorf(span.Span(), CodeNonOrderableType, "BETWEEN is not supported for %s", exprTypeName(expr))
			}
			return pc.typeMismatchErr(span, expr, bound)
		}
	}
	return nil
}

func (pc *planContext) resolveComparison(scope *Scope, cp *ast.ComparisonPredicate) (ResolvedCond, error) {
	left, err := pc.resolveExpr(scope, cp.Left)
	if err != nil {
		return nil, err
	}
	right, err := pc.resolveExpr(scope, cp.Right)
	if err != nil {
		return nil, err
	}
	op := tokenToOp(cp.Op)
	if isOrderingOp(op) {
		leftNull, rightNull := isNullExpr(left), isNullExpr(right)
		switch {
		case !leftNull && !rightNull && !typesOrderable(left.ResolvedType(), right.ResolvedType()):
			if typesCompatible(left.ResolvedType(), right.ResolvedType()) {
				return nil, pc.errorf(
					cp.Span(), CodeNonOrderableType,
					"operator %s is not supported for %s", op, exprTypeName(left),
				)
			}
			return nil, pc.typeMismatchErr(cp, left, right)
		case leftNull && !rightNull && !isOrderableType(right.ResolvedType()):
			return nil, pc.errorf(
				cp.Span(), CodeNonOrderableType,
				"operator %s is not supported for %s", op, exprTypeName(right),
			)
		case !leftNull && rightNull && !isOrderableType(left.ResolvedType()):
			return nil, pc.errorf(
				cp.Span(), CodeNonOrderableType,
				"operator %s is not supported for %s", op, exprTypeName(left),
			)
		}
	} else if !exprsCompatible(left, right) {
		return nil, pc.typeMismatchErr(cp, left, right)
	}

	// Warn when comparing with NULL using = or !=. In SQL, NULL = NULL
	// and NULL != NULL both evaluate to NULL (unknown), never TRUE or
	// FALSE. This is almost certainly a user mistake — use IS [NOT] NULL
	// instead.
	if op == OpEq || op == OpNeq {
		if isNullExpr(left) || isNullExpr(right) {
			pc.warnf(cp.Span(), CodeNullComparison,
				"comparison with NULL using %s always yields NULL; use IS [NOT] NULL instead", op)
		}
	}

	return &ResolvedComparison{Left: left, Op: op, Right: right}, nil
}

func (pc *planContext) resolveLike(scope *Scope, lp *ast.LikePredicate) (ResolvedCond, error) {
	left, err := pc.resolveExpr(scope, lp.Left)
	if err != nil {
		return nil, err
	}
	pattern, err := pc.resolveExpr(scope, lp.Pattern)
	if err != nil {
		return nil, err
	}
	if !isNullExpr(left) && !isStringType(left.ResolvedType()) {
		return nil, pc.errorf(lp.Span(), CodeNonStringOperand, "LIKE requires a string operand, got %s", exprTypeName(left))
	}
	if !isNullExpr(pattern) && !isStringType(pattern.ResolvedType()) {
		return nil, pc.errorf(lp.Span(), CodeNonStringOperand, "LIKE pattern must be a string, got %s", exprTypeName(pattern))
	}
	return &ResolvedLike{Left: left, Pattern: pattern, Negated: lp.Negated}, nil
}

func (pc *planContext) resolveIsNull(scope *Scope, in *ast.IsNullPredicate) (ResolvedCond, error) {
	expr, err := pc.resolveExpr(scope, in.Expr)
	if err != nil {
		return nil, err
	}
	return &ResolvedIsNull{Expr: expr, Negated: in.Negated}, nil
}

// resolveIn resolves every value in an IN list even after one fails or
// mismatches, so a query with several bad values reports all of them in one
// pass. It still returns the first error encountered, since a partially
// resolved ResolvedIn cannot be used by the caller.
func (pc *planContext) resolveIn(scope *Scope, ip *ast.InPredicate) (ResolvedCond, error) {
	expr, err := pc.resolveExpr(scope, ip.Expr)
	if err != nil {
		return nil, err
	}

	values := make([]ResolvedExpr, 0, len(ip.Values))
	var firstErr error
	for _, v := range ip.Values {
		if err := pc.checkContext(); err != nil {
			return nil, err
		}
		rv, err := pc.resolveExpr(scope, v)
		if err != nil {
			recordErr(&firstErr, err)
			continue
		}
		if !exprsCompatible(expr, rv) {
			recordErr(&firstErr, pc.typeMismatchErr(v, expr, rv))
			continue
		}
		// Warn about NULL literals in IN lists. In SQL, any comparison
		// with NULL yields UNKNOWN. For x IN (1, NULL, 3), a non-matching
		// x evaluates to NULL (not FALSE), which WHERE still filters out.
		// For x NOT IN (1, NULL, 3), a non-matching x also evaluates to
		// NULL instead of TRUE, silently excluding rows that would
		// otherwise pass. Removing the NULL may change query results.
		if isNullExpr(rv) {
			pc.warnf(v.Span(), CodeNullInList,
				"NULL in IN list: comparisons with NULL yield UNKNOWN, which may silently filter rows in NOT IN")
		}
		values = append(values, rv)
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return &ResolvedIn{Expr: expr, Values: values, Negated: ip.Negated}, nil
}

func (pc *planContext) resolveBetween(scope *Scope, bp *ast.BetweenPredicate) (ResolvedCond, error) {
	expr, err := pc.resolveExpr(scope, bp.Expr)
	if err != nil {
		return nil, err
	}
	low, err := pc.resolveExpr(scope, bp.Low)
	if err != nil {
		return nil, err
	}
	high, err := pc.resolveExpr(scope, bp.High)
	if err != nil {
		return nil, err
	}

	// Reject non-orderable base expression eagerly: if the base itself is
	// not orderable (e.g. BOOLEAN), BETWEEN must fail regardless of what
	// the bounds are. This catches TRUE BETWEEN NULL AND NULL.
	if !isNullExpr(expr) && !isOrderableType(expr.ResolvedType()) {
		return nil, pc.errorf(bp.Expr.Span(), CodeNonOrderableType, "BETWEEN is not supported for %s", exprTypeName(expr))
	}

	var firstErr error
	recordErr(&firstErr, pc.checkOrderableBound(expr, low, bp.Low))
	recordErr(&firstErr, pc.checkOrderableBound(expr, high, bp.High))
	if firstErr != nil {
		return nil, firstErr
	}
	return &ResolvedBetween{Expr: expr, Low: low, High: high, Negated: bp.Negated}, nil
}

func (pc *planContext) resolveExprCond(scope *Scope, ec *ast.ExprCondition) (ResolvedCond, error) {
	expr, err := pc.resolveExpr(scope, ec.Expr)
	if err != nil {
		return nil, err
	}
	if !isNullExpr(expr) && expr.ResolvedType() != ast.TypeBoolean {
		return nil, pc.errorf(
			ec.Span(), CodeNonBooleanCondition,
			"expression used as a condition must be boolean, got %s", exprTypeName(expr),
		)
	}
	return &ResolvedExprCond{Expr: expr}, nil
}
