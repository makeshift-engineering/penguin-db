package executor

import (
	"fmt"
	"math"
	"strings"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/planner"
)

// row is the in-memory representation of a single row during query
// execution. Values are indexed by active-column position, matching
// ResolvedColumn.Index. For joins, values from multiple tables are
// concatenated: the left table's columns occupy indices [0, leftLen),
// and the right table's columns occupy [leftLen, leftLen+rightLen).
type row struct {
	values []any
}

// evalExpr evaluates a resolved expression against a concrete row,
// returning the result as a Go any value. NULL is represented as nil.
func evalExpr(expr planner.ResolvedExpr, r row) (any, error) {
	switch e := expr.(type) {
	case *planner.ResolvedIntLiteral:
		return e.Value, nil
	case *planner.ResolvedFloatLiteral:
		return e.Value, nil
	case *planner.ResolvedStringLiteral:
		return e.Value, nil
	case *planner.ResolvedBoolLiteral:
		return e.Value, nil
	case *planner.ResolvedNullLiteral:
		return nil, nil
	case *planner.ResolvedColumnRef:
		idx := e.Column.Index
		if idx < 0 || idx >= len(r.values) {
			return nil, fmt.Errorf("column index %d out of range (row has %d values)", idx, len(r.values))
		}
		return r.values[idx], nil
	case *planner.ResolvedBinaryExpr:
		return evalBinaryExpr(e, r)
	case *planner.ResolvedUnaryExpr:
		return evalUnaryExpr(e, r)
	case *planner.ResolvedFunctionCall:
		// Aggregate functions are not evaluated per-row here — they are
		// handled by the aggregate node in exec_query.go. If we reach
		// here, it means a function call appeared outside aggregation
		// context, which shouldn't happen after planning. Return an error.
		return nil, fmt.Errorf("%w: aggregate function %q in non-aggregate context", ErrUnsupportedExpr, e.Name)
	case *planner.ResolvedConditionExpr:
		b, err := evalCond(e.Cond, r)
		if err != nil {
			return nil, err
		}
		return b, nil
	default:
		return nil, fmt.Errorf("%w: %T", ErrUnsupportedExpr, expr)
	}
}

// computeBinaryArithmetic applies a binary arithmetic operation (add, sub, mul, div, mod)
// to two float64 operands, returning ErrDivisionByZero if dividing or modding by zero.
func computeBinaryArithmetic(op planner.Op, lf, rf float64) (float64, error) {
	switch op {
	case planner.OpAdd:
		return lf + rf, nil
	case planner.OpSub:
		return lf - rf, nil
	case planner.OpMul:
		return lf * rf, nil
	case planner.OpDiv:
		if rf == 0 {
			return 0, ErrDivisionByZero
		}
		return lf / rf, nil
	case planner.OpMod:
		if rf == 0 {
			return 0, ErrDivisionByZero
		}
		return math.Mod(lf, rf), nil
	default:
		return 0, fmt.Errorf("%w: unknown binary operator %d", ErrUnsupportedExpr, op)
	}
}

// evalBinaryExpr evaluates a binary arithmetic expression.
func evalBinaryExpr(e *planner.ResolvedBinaryExpr, r row) (any, error) {
	left, err := evalExpr(e.Left, r)
	if err != nil {
		return nil, err
	}
	right, err := evalExpr(e.Right, r)
	if err != nil {
		return nil, err
	}
	// NULL propagation: any operand NULL → result NULL.
	if left == nil || right == nil {
		return nil, nil
	}

	lf, err := toFloat64(left)
	if err != nil {
		return nil, err
	}
	rf, err := toFloat64(right)
	if err != nil {
		return nil, err
	}

	result, err := computeBinaryArithmetic(e.Op, lf, rf)
	if err != nil {
		return nil, err
	}

	// Preserve integer type when both operands are integer and the
	// operation is integer-safe.
	if isIntegerAny(left) && isIntegerAny(right) && e.Op != planner.OpDiv {
		return coerceResultToIntKind(result, e.ResolvedType()), nil
	}

	return coerceResultByKind(result, e.ResolvedType()), nil
}

// evalUnaryExpr evaluates a prefix +/- expression.
func evalUnaryExpr(e *planner.ResolvedUnaryExpr, r row) (any, error) {
	operand, err := evalExpr(e.Operand, r)
	if err != nil {
		return nil, err
	}
	if operand == nil {
		return nil, nil
	}
	if e.Op == planner.OpAdd {
		return operand, nil
	}
	// TOKEN_MINUS: negate.
	f, err := toFloat64(operand)
	if err != nil {
		return nil, err
	}
	neg := -f
	if isIntegerAny(operand) {
		return coerceResultToIntKind(neg, e.ResolvedType()), nil
	}
	return coerceResultByKind(neg, e.ResolvedType()), nil
}

// evalCond evaluates a resolved condition against a concrete row,
// returning a Go bool. NULL comparisons follow SQL three-valued logic:
// any comparison involving NULL returns false (the row does not match).
func evalCond(cond planner.ResolvedCond, r row) (bool, error) {
	switch c := cond.(type) {
	case *planner.ResolvedComparison:
		return evalComparison(c, r)
	case *planner.ResolvedBinaryCond:
		return evalBinaryCond(c, r)
	case *planner.ResolvedNotCond:
		v, err := evalCond(c.Operand, r)
		if err != nil {
			return false, err
		}
		return !v, nil
	case *planner.ResolvedLike:
		return evalLike(c, r)
	case *planner.ResolvedIsNull:
		return evalIsNull(c, r)
	case *planner.ResolvedIn:
		return evalIn(c, r)
	case *planner.ResolvedBetween:
		return evalBetween(c, r)
	case *planner.ResolvedExprCond:
		v, err := evalExpr(c.Expr, r)
		if err != nil {
			return false, err
		}
		if v == nil {
			return false, nil
		}
		b, ok := v.(bool)
		if !ok {
			return false, fmt.Errorf("%w: expected bool from expression condition, got %T", ErrTypeMismatch, v)
		}
		return b, nil
	default:
		return false, fmt.Errorf("%w: %T", ErrUnsupportedCond, cond)
	}
}

// evalComparison evaluates a comparison condition (=, !=, <>, <, >, <=, >=).
func evalComparison(c *planner.ResolvedComparison, r row) (bool, error) {
	left, err := evalExpr(c.Left, r)
	if err != nil {
		return false, err
	}
	right, err := evalExpr(c.Right, r)
	if err != nil {
		return false, err
	}
	// SQL NULL semantics: any comparison with NULL is false.
	if left == nil || right == nil {
		return false, nil
	}
	cmp, err := compareValues(left, right)
	if err != nil {
		return false, err
	}
	switch c.Op {
	case planner.OpEq:
		return cmp == 0, nil
	case planner.OpNeq:
		return cmp != 0, nil
	case planner.OpLt:
		return cmp < 0, nil
	case planner.OpGt:
		return cmp > 0, nil
	case planner.OpLte:
		return cmp <= 0, nil
	case planner.OpGte:
		return cmp >= 0, nil
	default:
		return false, fmt.Errorf("%w: unknown comparison operator %d", ErrUnsupportedCond, c.Op)
	}
}

// evalBinaryCond evaluates AND/OR.
func evalBinaryCond(c *planner.ResolvedBinaryCond, r row) (bool, error) {
	left, err := evalCond(c.Left, r)
	if err != nil {
		return false, err
	}
	switch c.Op {
	case planner.OpAnd:
		if !left {
			return false, nil // short-circuit
		}
		return evalCond(c.Right, r)
	case planner.OpOr:
		if left {
			return true, nil // short-circuit
		}
		return evalCond(c.Right, r)
	default:
		return false, fmt.Errorf("%w: unknown binary condition operator %d", ErrUnsupportedCond, c.Op)
	}
}

// evalLike evaluates a [NOT] LIKE predicate using SQL LIKE semantics:
// '%' matches any sequence, '_' matches any single character.
func evalLike(c *planner.ResolvedLike, r row) (bool, error) {
	leftVal, err := evalExpr(c.Left, r)
	if err != nil {
		return false, err
	}
	patternVal, err := evalExpr(c.Pattern, r)
	if err != nil {
		return false, err
	}
	if leftVal == nil || patternVal == nil {
		return false, nil
	}
	str, ok := leftVal.(string)
	if !ok {
		return false, fmt.Errorf("%w: LIKE left operand must be a string, got %T", ErrTypeMismatch, leftVal)
	}
	pattern, ok := patternVal.(string)
	if !ok {
		return false, ErrInvalidLikePattern
	}
	matched := matchLike(str, pattern)
	if c.Negated {
		return !matched, nil
	}
	return matched, nil
}

// matchLike implements SQL LIKE pattern matching.
// '%' matches zero or more characters, '_' matches exactly one character.
func matchLike(str, pattern string) bool {
	return matchLikeRecursive(str, 0, pattern, 0)
}

// matchLikeRecursive recursively matches a string against a pattern slice.
func matchLikeRecursive(str string, si int, pattern string, pi int) bool {
	for pi < len(pattern) {
		switch pattern[pi] {
		case '%':
			// Skip consecutive '%' characters.
			for pi < len(pattern) && pattern[pi] == '%' {
				pi++
			}
			if pi == len(pattern) {
				return true // trailing '%' matches everything
			}
			for i := si; i <= len(str); i++ {
				if matchLikeRecursive(str, i, pattern, pi) {
					return true
				}
			}
			return false
		case '_':
			if si >= len(str) {
				return false
			}
			si++
			pi++
		default:
			if si >= len(str) || !strings.EqualFold(string(str[si]), string(pattern[pi])) {
				return false
			}
			si++
			pi++
		}
	}
	return si == len(str)
}

// evalIsNull evaluates an IS [NOT] NULL predicate.
func evalIsNull(c *planner.ResolvedIsNull, r row) (bool, error) {
	val, err := evalExpr(c.Expr, r)
	if err != nil {
		return false, err
	}
	isNull := val == nil
	if c.Negated {
		return !isNull, nil
	}
	return isNull, nil
}

// evalIn evaluates a [NOT] IN predicate.
func evalIn(c *planner.ResolvedIn, r row) (bool, error) {
	left, err := evalExpr(c.Expr, r)
	if err != nil {
		return false, err
	}
	if left == nil {
		return false, nil
	}
	found := false
	for _, vExpr := range c.Values {
		right, err := evalExpr(vExpr, r)
		if err != nil {
			return false, err
		}
		if right == nil {
			continue
		}
		cmp, err := compareValues(left, right)
		if err != nil {
			return false, err
		}
		if cmp == 0 {
			found = true
			break
		}
	}
	if c.Negated {
		return !found, nil
	}
	return found, nil
}

// evalBetween evaluates a [NOT] BETWEEN predicate.
func evalBetween(c *planner.ResolvedBetween, r row) (bool, error) {
	val, err := evalExpr(c.Expr, r)
	if err != nil {
		return false, err
	}
	low, err := evalExpr(c.Low, r)
	if err != nil {
		return false, err
	}
	high, err := evalExpr(c.High, r)
	if err != nil {
		return false, err
	}
	if val == nil || low == nil || high == nil {
		return false, nil
	}
	cmpLow, err := compareValues(val, low)
	if err != nil {
		return false, err
	}
	cmpHigh, err := compareValues(val, high)
	if err != nil {
		return false, err
	}
	between := cmpLow >= 0 && cmpHigh <= 0
	if c.Negated {
		return !between, nil
	}
	return between, nil
}

// isIntegerAny reports whether v is an integer type (int32 or int64).
func isIntegerAny(v any) bool {
	switch v.(type) {
	case int32, int64:
		return true
	default:
		return false
	}
}

// coerceResultToIntKind converts a float64 arithmetic result back to the
// integer type indicated by the planner's resolved type kind.
func coerceResultToIntKind(f float64, kind ast.DataTypeKind) any {
	switch kind {
	case ast.TypeInt:
		return int32(f)
	case ast.TypeBigInt:
		return int64(f)
	default:
		return coerceResultByKind(f, kind)
	}
}

// coerceResultByKind converts a float64 result to the appropriate Go type.
func coerceResultByKind(f float64, kind ast.DataTypeKind) any {
	switch kind {
	case ast.TypeInt:
		return int32(f)
	case ast.TypeBigInt:
		return int64(f)
	case ast.TypeFloat:
		return float32(f)
	case ast.TypeDouble:
		return f
	default:
		return f
	}
}

// accumulateNumericGroup evaluates a numeric aggregate expression (for SUM or AVG)
// over group rows, respecting DISTINCT filtering. It returns the running sum and
// the count of non-NULL evaluated values.
func accumulateNumericGroup(fn *planner.ResolvedFunctionCall, groupRows []row) (sum float64, count int64, err error) {
	var seen map[any]struct{}
	if fn.Distinct {
		seen = make(map[any]struct{})
	}

	for _, r := range groupRows {
		v, err := evalExpr(fn.Args[0], r)
		if err != nil {
			return 0, 0, err
		}
		if v == nil {
			continue
		}
		if fn.Distinct {
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
		}
		f, err := toFloat64(v)
		if err != nil {
			return 0, 0, err
		}
		sum += f
		count++
	}
	return sum, count, nil
}
