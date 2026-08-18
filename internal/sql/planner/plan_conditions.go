package planner

import (
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
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
	return &ResolvedBinaryCond{Left: left, Op: bc.Op, Right: right}, nil
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
	if isOrderingOp(cp.Op) {
		leftNull, rightNull := isNullExpr(left), isNullExpr(right)
		switch {
		case !leftNull && !rightNull && !typesOrderable(left.ResolvedType(), right.ResolvedType()):
			if typesCompatible(left.ResolvedType(), right.ResolvedType()) {
				return nil, pc.errorf(
					cp.Span(), CodeNonOrderableType,
					"operator %s is not supported for %s", cp.Op, exprTypeName(left),
				)
			}
			return nil, pc.errorf(
				cp.Span(), CodeTypeMismatch,
				"cannot compare %s and %s", exprTypeName(left), exprTypeName(right),
			)
		case leftNull && !rightNull && !isOrderableType(right.ResolvedType()):
			return nil, pc.errorf(
				cp.Span(), CodeNonOrderableType,
				"operator %s is not supported for %s", cp.Op, exprTypeName(right),
			)
		case !leftNull && rightNull && !isOrderableType(left.ResolvedType()):
			return nil, pc.errorf(
				cp.Span(), CodeNonOrderableType,
				"operator %s is not supported for %s", cp.Op, exprTypeName(left),
			)
		}
	} else if !exprsCompatible(left, right) {
		return nil, pc.errorf(
			cp.Span(), CodeTypeMismatch,
			"cannot compare %s and %s", exprTypeName(left), exprTypeName(right),
		)
	}
	return &ResolvedComparison{Left: left, Op: cp.Op, Right: right}, nil
}

// isOrderingOp reports whether op is a strict ordering operator that
// requires orderable operands, as opposed to an equality operator.
func isOrderingOp(op utils.TokenType) bool {
	return op == utils.TOKEN_LT || op == utils.TOKEN_GT || op == utils.TOKEN_LTE || op == utils.TOKEN_GTE
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
		rv, err := pc.resolveExpr(scope, v)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if !exprsCompatible(expr, rv) {
			err := pc.errorf(v.Span(), CodeTypeMismatch, "cannot compare %s and %s", exprTypeName(expr), exprTypeName(rv))
			if firstErr == nil {
				firstErr = err
			}
			continue
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
	if !isNullExpr(expr) && !isNullExpr(low) {
		if !typesOrderable(expr.ResolvedType(), low.ResolvedType()) {
			if typesCompatible(expr.ResolvedType(), low.ResolvedType()) {
				firstErr = pc.errorf(bp.Low.Span(), CodeNonOrderableType, "BETWEEN is not supported for %s", exprTypeName(expr))
			} else {
				firstErr = pc.errorf(bp.Low.Span(), CodeTypeMismatch, "cannot compare %s and %s", exprTypeName(expr), exprTypeName(low))
			}
		}
	}
	if !isNullExpr(expr) && !isNullExpr(high) {
		if !typesOrderable(expr.ResolvedType(), high.ResolvedType()) {
			var err error
			if typesCompatible(expr.ResolvedType(), high.ResolvedType()) {
				err = pc.errorf(bp.High.Span(), CodeNonOrderableType, "BETWEEN is not supported for %s", exprTypeName(expr))
			} else {
				err = pc.errorf(bp.High.Span(), CodeTypeMismatch, "cannot compare %s and %s", exprTypeName(expr), exprTypeName(high))
			}
			if firstErr == nil {
				firstErr = err
			}
		}
	}
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
