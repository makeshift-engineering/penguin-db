package executor

import (
	"fmt"
	"math"
	"math/big"
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

// computeIntegerArithmetic evaluates integer arithmetic without converting
// operands through float64, preserving BIGINT precision and detecting overflow.
func computeIntegerArithmetic(op planner.Op, left, right int64) (int64, error) {
	if op == planner.OpMod {
		if right == 0 {
			return 0, ErrDivisionByZero
		}
		return left % right, nil
	}

	result := new(big.Int)
	switch op {
	case planner.OpAdd:
		result.Add(big.NewInt(left), big.NewInt(right))
	case planner.OpSub:
		result.Sub(big.NewInt(left), big.NewInt(right))
	case planner.OpMul:
		result.Mul(big.NewInt(left), big.NewInt(right))
	default:
		return 0, fmt.Errorf("%w: unknown integer operator %d", ErrUnsupportedExpr, op)
	}

	if !result.IsInt64() {
		return 0, ErrNumericOverflow
	}
	return result.Int64(), nil
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
	if isIntegerAny(left) && isIntegerAny(right) && e.Op != planner.OpDiv {
		li, _ := toInt64(left)
		ri, _ := toInt64(right)
		result, err := computeIntegerArithmetic(e.Op, li, ri)
		if err != nil {
			return nil, err
		}
		return coerceIntegerResult(result, e.ResolvedType()), nil
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
	if isIntegerAny(operand) {
		value, _ := toInt64(operand)
		result, err := computeIntegerArithmetic(planner.OpSub, 0, value)
		if err != nil {
			return nil, err
		}
		return coerceIntegerResult(result, e.ResolvedType()), nil
	}
	f, err := toFloat64(operand)
	if err != nil {
		return nil, err
	}
	neg := -f
	return coerceResultByKind(neg, e.ResolvedType()), nil
}

type condResult struct {
	value   bool
	unknown bool
}

// evalCond converts SQL UNKNOWN to false at the WHERE boundary.
func evalCond(cond planner.ResolvedCond, r row) (bool, error) {
	result, err := evalCondValue(cond, r)
	if err != nil {
		return false, err
	}
	return result.value && !result.unknown, nil
}

func evalCondValue(cond planner.ResolvedCond, r row) (condResult, error) {
	switch c := cond.(type) {
	case *planner.ResolvedComparison:
		return evalComparisonValue(c, r)
	case *planner.ResolvedBinaryCond:
		return evalBinaryCondValue(c, r)
	case *planner.ResolvedNotCond:
		result, err := evalCondValue(c.Operand, r)
		if err != nil || result.unknown {
			return result, err
		}
		return condResult{value: !result.value}, nil
	case *planner.ResolvedLike:
		return evalLikeValue(c, r)
	case *planner.ResolvedIsNull:
		value, err := evalIsNull(c, r)
		return condResult{value: value}, err
	case *planner.ResolvedIn:
		return evalInValue(c, r)
	case *planner.ResolvedBetween:
		return evalBetweenValue(c, r)
	case *planner.ResolvedExprCond:
		value, err := evalExpr(c.Expr, r)
		if err != nil {
			return condResult{}, err
		}
		if value == nil {
			return condResult{unknown: true}, nil
		}
		boolean, ok := value.(bool)
		if !ok {
			return condResult{}, fmt.Errorf("%w: expected bool from expression condition, got %T", ErrTypeMismatch, value)
		}
		return condResult{value: boolean}, nil
	default:
		return condResult{}, fmt.Errorf("%w: %T", ErrUnsupportedCond, cond)
	}
}

// evalComparison evaluates a comparison condition (=, !=, <>, <, >, <=, >=).
func evalComparison(c *planner.ResolvedComparison, r row) (bool, error) {
	result, err := evalComparisonValue(c, r)
	return result.value && !result.unknown, err
}

func evalComparisonValue(c *planner.ResolvedComparison, r row) (condResult, error) {
	left, err := evalExpr(c.Left, r)
	if err != nil {
		return condResult{}, err
	}
	right, err := evalExpr(c.Right, r)
	if err != nil {
		return condResult{}, err
	}
	if left == nil || right == nil {
		return condResult{unknown: true}, nil
	}
	cmp, err := compareValues(left, right)
	if err != nil {
		return condResult{}, err
	}
	switch c.Op {
	case planner.OpEq:
		return condResult{value: cmp == 0}, nil
	case planner.OpNeq:
		return condResult{value: cmp != 0}, nil
	case planner.OpLt:
		return condResult{value: cmp < 0}, nil
	case planner.OpGt:
		return condResult{value: cmp > 0}, nil
	case planner.OpLte:
		return condResult{value: cmp <= 0}, nil
	case planner.OpGte:
		return condResult{value: cmp >= 0}, nil
	default:
		return condResult{}, fmt.Errorf("%w: unknown comparison operator %d", ErrUnsupportedCond, c.Op)
	}
}

// evalBinaryCond evaluates AND/OR.
func evalBinaryCond(c *planner.ResolvedBinaryCond, r row) (bool, error) {
	result, err := evalBinaryCondValue(c, r)
	return result.value && !result.unknown, err
}

func evalBinaryCondValue(c *planner.ResolvedBinaryCond, r row) (condResult, error) {
	left, err := evalCondValue(c.Left, r)
	if err != nil {
		return condResult{}, err
	}
	right, err := evalCondValue(c.Right, r)
	if err != nil {
		return condResult{}, err
	}
	switch c.Op {
	case planner.OpAnd:
		if (!left.unknown && !left.value) || (!right.unknown && !right.value) {
			return condResult{value: false}, nil
		}
		if left.unknown || right.unknown {
			return condResult{unknown: true}, nil
		}
		return condResult{value: true}, nil
	case planner.OpOr:
		if (!left.unknown && left.value) || (!right.unknown && right.value) {
			return condResult{value: true}, nil
		}
		if left.unknown || right.unknown {
			return condResult{unknown: true}, nil
		}
		return condResult{value: false}, nil
	default:
		return condResult{}, fmt.Errorf("%w: unknown binary condition operator %d", ErrUnsupportedCond, c.Op)
	}
}

// evalLike evaluates a [NOT] LIKE predicate using SQL LIKE semantics:
// '%' matches any sequence, '_' matches any single character.
func evalLike(c *planner.ResolvedLike, r row) (bool, error) {
	result, err := evalLikeValue(c, r)
	return result.value && !result.unknown, err
}

func evalLikeValue(c *planner.ResolvedLike, r row) (condResult, error) {
	leftVal, err := evalExpr(c.Left, r)
	if err != nil {
		return condResult{}, err
	}
	patternVal, err := evalExpr(c.Pattern, r)
	if err != nil {
		return condResult{}, err
	}
	if leftVal == nil || patternVal == nil {
		return condResult{unknown: true}, nil
	}
	str, ok := leftVal.(string)
	if !ok {
		return condResult{}, fmt.Errorf("%w: LIKE left operand must be a string, got %T", ErrTypeMismatch, leftVal)
	}
	pattern, ok := patternVal.(string)
	if !ok {
		return condResult{}, ErrInvalidLikePattern
	}
	matched := matchLike(str, pattern)
	if c.Negated {
		return condResult{value: !matched}, nil
	}
	return condResult{value: matched}, nil
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
	result, err := evalInValue(c, r)
	return result.value && !result.unknown, err
}

func evalInValue(c *planner.ResolvedIn, r row) (condResult, error) {
	left, err := evalExpr(c.Expr, r)
	if err != nil {
		return condResult{}, err
	}
	if left == nil {
		return condResult{unknown: true}, nil
	}
	found := false
	hasNull := false
	for _, vExpr := range c.Values {
		right, err := evalExpr(vExpr, r)
		if err != nil {
			return condResult{}, err
		}
		if right == nil {
			hasNull = true
			continue
		}
		cmp, err := compareValues(left, right)
		if err != nil {
			return condResult{}, err
		}
		if cmp == 0 {
			found = true
			break
		}
	}
	if !found && hasNull {
		return condResult{unknown: true}, nil
	}
	if c.Negated {
		return condResult{value: !found}, nil
	}
	return condResult{value: found}, nil
}

// evalBetween evaluates a [NOT] BETWEEN predicate.
func evalBetween(c *planner.ResolvedBetween, r row) (bool, error) {
	result, err := evalBetweenValue(c, r)
	return result.value && !result.unknown, err
}

func evalBetweenValue(c *planner.ResolvedBetween, r row) (condResult, error) {
	val, err := evalExpr(c.Expr, r)
	if err != nil {
		return condResult{}, err
	}
	low, err := evalExpr(c.Low, r)
	if err != nil {
		return condResult{}, err
	}
	high, err := evalExpr(c.High, r)
	if err != nil {
		return condResult{}, err
	}
	if val == nil || low == nil || high == nil {
		return condResult{unknown: true}, nil
	}
	cmpLow, err := compareValues(val, low)
	if err != nil {
		return condResult{}, err
	}
	cmpHigh, err := compareValues(val, high)
	if err != nil {
		return condResult{}, err
	}
	between := cmpLow >= 0 && cmpHigh <= 0
	if c.Negated {
		return condResult{value: !between}, nil
	}
	return condResult{value: between}, nil
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
func coerceIntegerResult(value int64, kind ast.DataTypeKind) any {
	switch kind {
	case ast.TypeInt:
		return int32(value)
	case ast.TypeBigInt:
		return value
	default:
		return coerceResultByKind(float64(value), kind)
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
