package executor

import (
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/planner"
)

func TestEvalUnaryExpr(t *testing.T) {
	tests := []struct {
		op      planner.Op
		operand any
		want    any
		wantErr bool
	}{
		{planner.OpAdd, int32(42), int64(42), false},
		{planner.OpSub, int32(42), int32(-42), false},
		{planner.OpAdd, float64(3.14), float64(3.14), false},
		{planner.OpSub, float64(3.14), float64(-3.14), false},
		{planner.OpSub, nil, nil, false},
		{planner.OpSub, "not-a-number", nil, true},
	}

	for i, tc := range tests {
		var expr planner.ResolvedExpr

		switch v := tc.operand.(type) {
		case int32:
			expr = &planner.ResolvedUnaryExpr{
				ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt},
				Op:               tc.op,
				Operand:          &planner.ResolvedIntLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeInt}, Value: int64(v)},
			}
		case float64:
			expr = &planner.ResolvedUnaryExpr{
				ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeDouble},
				Op:               tc.op,
				Operand:          &planner.ResolvedFloatLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeDouble}, Value: v},
			}
		case string:
			expr = &planner.ResolvedUnaryExpr{
				ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText},
				Op:               tc.op,
				Operand:          &planner.ResolvedStringLiteral{ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeText}, Value: v},
			}
		case nil:
			expr = &planner.ResolvedUnaryExpr{
				ResolvedExprBase: planner.ResolvedExprBase{Type: ast.TypeNull},
				Op:               tc.op,
				Operand:          &planner.ResolvedNullLiteral{},
			}
		}

		got, err := evalUnaryExpr(expr.(*planner.ResolvedUnaryExpr), row{})
		if (err != nil) != tc.wantErr {
			t.Errorf("test %d: evalUnaryExpr() error = %v, wantErr %v", i, err, tc.wantErr)
			continue
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("test %d: evalUnaryExpr() = %v, want %v", i, got, tc.want)
		}
	}
}

func TestEvalComparison(t *testing.T) {
	tests := []struct {
		op    planner.Op
		left  any
		right any
		want  bool
	}{
		{planner.OpEq, int32(1), int32(1), true},
		{planner.OpEq, int32(1), int32(2), false},
		{planner.OpNeq, int32(1), int32(2), true},
		{planner.OpLt, int32(1), int32(2), true},
		{planner.OpLt, int32(2), int32(1), false},
		{planner.OpGt, int32(2), int32(1), true},
		{planner.OpLte, int32(2), int32(2), true},
		{planner.OpGte, int32(2), int32(2), true},
		{planner.OpEq, nil, int32(1), false},
		{planner.OpEq, int32(1), nil, false},
	}

	for i, tc := range tests {
		expr := &planner.ResolvedComparison{
			Op: tc.op,
		}
		if tc.left == nil {
			expr.Left = &planner.ResolvedNullLiteral{}
		} else if v, ok := tc.left.(int32); ok {
			expr.Left = &planner.ResolvedIntLiteral{Value: int64(v)}
		}
		if tc.right == nil {
			expr.Right = &planner.ResolvedNullLiteral{}
		} else if v, ok := tc.right.(int32); ok {
			expr.Right = &planner.ResolvedIntLiteral{Value: int64(v)}
		}

		got, err := evalComparison(expr, row{})
		if err != nil {
			t.Errorf("test %d: evalComparison() unexpected error: %v", i, err)
			continue
		}
		if got != tc.want {
			t.Errorf("test %d: evalComparison() = %v, want %v", i, got, tc.want)
		}
	}
}

func TestEvalBinaryCond(t *testing.T) {
	tests := []struct {
		op    planner.Op
		left  bool
		right bool
		want  bool
	}{
		{planner.OpAnd, true, true, true},
		{planner.OpAnd, true, false, false},
		{planner.OpAnd, false, true, false},
		{planner.OpAnd, false, false, false},
		{planner.OpOr, true, true, true},
		{planner.OpOr, true, false, true},
		{planner.OpOr, false, true, true},
		{planner.OpOr, false, false, false},
	}

	for i, tc := range tests {
		cond := &planner.ResolvedBinaryCond{
			Op: tc.op,
			Left: &planner.ResolvedExprCond{
				Expr: &planner.ResolvedBoolLiteral{Value: tc.left},
			},
			Right: &planner.ResolvedExprCond{
				Expr: &planner.ResolvedBoolLiteral{Value: tc.right},
			},
		}

		got, err := evalBinaryCond(cond, row{})
		if err != nil {
			t.Errorf("test %d: evalBinaryCond() unexpected error: %v", i, err)
			continue
		}
		if got != tc.want {
			t.Errorf("test %d: evalBinaryCond() = %v, want %v", i, got, tc.want)
		}
	}
}

func TestEvalLike(t *testing.T) {
	tests := []struct {
		left    any
		pattern any
		negated bool
		want    bool
		wantErr bool
	}{
		{"hello", "h%o", false, true, false},
		{"hello", "h%o", true, false, false},
		{"hello", "h_llo", false, true, false},
		{"hello", "x%", false, false, false},
		{nil, "h%o", false, false, false},
		{"hello", nil, false, false, false},
		{123, "1%", false, false, true}, // not a string
	}

	for i, tc := range tests {
		cond := &planner.ResolvedLike{
			Negated: tc.negated,
		}
		if tc.left == nil {
			cond.Left = &planner.ResolvedNullLiteral{}
		} else if v, ok := tc.left.(string); ok {
			cond.Left = &planner.ResolvedStringLiteral{Value: v}
		} else if v, ok := tc.left.(int); ok {
			cond.Left = &planner.ResolvedIntLiteral{Value: int64(v)}
		}
		if tc.pattern == nil {
			cond.Pattern = &planner.ResolvedNullLiteral{}
		} else if v, ok := tc.pattern.(string); ok {
			cond.Pattern = &planner.ResolvedStringLiteral{Value: v}
		}

		got, err := evalLike(cond, row{})
		if (err != nil) != tc.wantErr {
			t.Errorf("test %d: evalLike() error = %v, wantErr %v", i, err, tc.wantErr)
			continue
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("test %d: evalLike() = %v, want %v", i, got, tc.want)
		}
	}
}

func TestEvalIsNull(t *testing.T) {
	tests := []struct {
		expr    planner.ResolvedExpr
		negated bool
		want    bool
	}{
		{&planner.ResolvedNullLiteral{}, false, true},
		{&planner.ResolvedNullLiteral{}, true, false},
		{&planner.ResolvedIntLiteral{Value: int64(1)}, false, false},
		{&planner.ResolvedIntLiteral{Value: int64(1)}, true, true},
	}

	for i, tc := range tests {
		cond := &planner.ResolvedIsNull{
			Expr:    tc.expr,
			Negated: tc.negated,
		}
		got, err := evalIsNull(cond, row{})
		if err != nil {
			t.Errorf("test %d: evalIsNull() unexpected error: %v", i, err)
			continue
		}
		if got != tc.want {
			t.Errorf("test %d: evalIsNull() = %v, want %v", i, got, tc.want)
		}
	}
}

func TestEvalIn(t *testing.T) {
	tests := []struct {
		left    any
		values  []any
		negated bool
		want    bool
	}{
		{int32(2), []any{int32(1), int32(2), int32(3)}, false, true},
		{int32(4), []any{int32(1), int32(2), int32(3)}, false, false},
		{int32(2), []any{int32(1), int32(2), int32(3)}, true, false},
		{nil, []any{int32(1)}, false, false},
		{int32(1), []any{int32(2), nil}, false, false},
	}

	for i, tc := range tests {
		cond := &planner.ResolvedIn{
			Negated: tc.negated,
		}
		if tc.left == nil {
			cond.Expr = &planner.ResolvedNullLiteral{}
		} else if v, ok := tc.left.(int32); ok {
			cond.Expr = &planner.ResolvedIntLiteral{Value: int64(v)}
		}
		for _, v := range tc.values {
			if v == nil {
				cond.Values = append(cond.Values, &planner.ResolvedNullLiteral{})
			} else if vInt, ok := v.(int32); ok {
				cond.Values = append(cond.Values, &planner.ResolvedIntLiteral{Value: int64(vInt)})
			}
		}

		got, err := evalIn(cond, row{})
		if err != nil {
			t.Errorf("test %d: evalIn() unexpected error: %v", i, err)
			continue
		}
		if got != tc.want {
			t.Errorf("test %d: evalIn() = %v, want %v", i, got, tc.want)
		}
	}
}

func TestEvalBetween(t *testing.T) {
	tests := []struct {
		val     any
		low     any
		high    any
		negated bool
		want    bool
	}{
		{int32(5), int32(1), int32(10), false, true},
		{int32(0), int32(1), int32(10), false, false},
		{int32(11), int32(1), int32(10), false, false},
		{int32(5), int32(1), int32(10), true, false},
		{nil, int32(1), int32(10), false, false},
		{int32(5), nil, int32(10), false, false},
		{int32(5), int32(1), nil, false, false},
	}

	for i, tc := range tests {
		cond := &planner.ResolvedBetween{
			Negated: tc.negated,
		}
		if tc.val == nil {
			cond.Expr = &planner.ResolvedNullLiteral{}
		} else if v, ok := tc.val.(int32); ok {
			cond.Expr = &planner.ResolvedIntLiteral{Value: int64(v)}
		}
		if tc.low == nil {
			cond.Low = &planner.ResolvedNullLiteral{}
		} else if v, ok := tc.low.(int32); ok {
			cond.Low = &planner.ResolvedIntLiteral{Value: int64(v)}
		}
		if tc.high == nil {
			cond.High = &planner.ResolvedNullLiteral{}
		} else if v, ok := tc.high.(int32); ok {
			cond.High = &planner.ResolvedIntLiteral{Value: int64(v)}
		}

		got, err := evalBetween(cond, row{})
		if err != nil {
			t.Errorf("test %d: evalBetween() unexpected error: %v", i, err)
			continue
		}
		if got != tc.want {
			t.Errorf("test %d: evalBetween() = %v, want %v", i, got, tc.want)
		}
	}
}

func TestEvalCond(t *testing.T) {
	// A basic test to hit the switch cases not covered by other tests.
	tests := []struct {
		cond planner.ResolvedCond
		want bool
	}{
		{
			&planner.ResolvedNotCond{
				Operand: &planner.ResolvedExprCond{Expr: &planner.ResolvedBoolLiteral{Value: true}},
			},
			false,
		},
		{
			&planner.ResolvedExprCond{Expr: &planner.ResolvedNullLiteral{}},
			false,
		},
	}

	for i, tc := range tests {
		got, err := evalCond(tc.cond, row{})
		if err != nil {
			t.Errorf("test %d: evalCond() unexpected error: %v", i, err)
			continue
		}
		if got != tc.want {
			t.Errorf("test %d: evalCond() = %v, want %v", i, got, tc.want)
		}
	}
}

func TestCoerceResultByKind(t *testing.T) {
	tests := []struct {
		val  float64
		kind ast.DataTypeKind
		want any
	}{
		{1.5, ast.TypeInt, int32(1)},
		{1.5, ast.TypeBigInt, int64(1)},
		{1.5, ast.TypeFloat, float32(1.5)},
		{1.5, ast.TypeDouble, float64(1.5)},
		{1.5, ast.TypeDecimal, float64(1.5)},
	}

	for i, tc := range tests {
		got := coerceResultByKind(tc.val, tc.kind)
		if got != tc.want {
			t.Errorf("test %d: coerceResultByKind() = %v, want %v", i, got, tc.want)
		}
	}
}
