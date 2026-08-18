package planner

import (
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

func cmp(left ast.Expression, op utils.TokenType, right ast.Expression) *ast.ComparisonPredicate {
	return &ast.ComparisonPredicate{Left: left, Op: op, Right: right}
}

func TestResolveCond_Comparison_Compatible(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	got, err := pc.resolveCond(newScope(), cmp(intLit("1"), utils.TOKEN_EQ, intLit("2")))
	if err != nil {
		t.Fatalf("resolveCond: %v", err)
	}
	if _, ok := got.(*ResolvedComparison); !ok {
		t.Errorf("expected *ResolvedComparison, got %T", got)
	}
}

func TestResolveCond_Comparison_Incompatible(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	_, err := pc.resolveCond(newScope(), cmp(intLit("1"), utils.TOKEN_EQ, strLit("a")))
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeTypeMismatch {
		t.Errorf("expected CodeTypeMismatch, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestResolveCond_Comparison_NullAlwaysCompatible(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	_, err := pc.resolveCond(newScope(), cmp(&ast.NullLiteral{}, utils.TOKEN_EQ, strLit("a")))
	if err != nil {
		t.Fatalf("expected NULL to be compatible with any type, got %v", err)
	}
}

func TestResolveCond_Like_Strings(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	_, err := pc.resolveCond(newScope(), &ast.LikePredicate{Left: strLit("abc"), Pattern: strLit("a%")})
	if err != nil {
		t.Fatalf("resolveCond: %v", err)
	}
}

func TestResolveCond_Like_NonString_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	_, err := pc.resolveCond(newScope(), &ast.LikePredicate{Left: intLit("1"), Pattern: strLit("a%")})
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeNonStringOperand {
		t.Errorf("expected CodeNonStringOperand, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestResolveCond_IsNull_AnyType(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	scope := userScope(t, pc)
	_, err := pc.resolveCond(scope, &ast.IsNullPredicate{Expr: qualifiedIdent("u", "name")})
	if err != nil {
		t.Fatalf("resolveCond: %v", err)
	}
}

func TestResolveCond_In_AllCompatible(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	got, err := pc.resolveCond(newScope(), &ast.InPredicate{
		Expr:   intLit("1"),
		Values: []ast.Expression{intLit("1"), intLit("2"), intLit("3")},
	})
	if err != nil {
		t.Fatalf("resolveCond: %v", err)
	}
	in := got.(*ResolvedIn)
	if len(in.Values) != 3 {
		t.Errorf("expected 3 resolved values, got %d", len(in.Values))
	}
}

func TestResolveCond_In_OneIncompatible_ReportsAndErrors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	_, err := pc.resolveCond(newScope(), &ast.InPredicate{
		Expr:   intLit("1"),
		Values: []ast.Expression{intLit("1"), strLit("nope"), intLit("3")},
	})
	if err == nil {
		t.Fatal("expected an error from the incompatible value")
	}
	if !pc.diag.HasErrors() || pc.diag[0].Code != CodeTypeMismatch {
		t.Errorf("expected a CodeTypeMismatch diagnostic, got %+v", pc.diag)
	}
}

func TestResolveCond_Between_Compatible(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	_, err := pc.resolveCond(newScope(), &ast.BetweenPredicate{Expr: intLit("5"), Low: intLit("1"), High: intLit("10")})
	if err != nil {
		t.Fatalf("resolveCond: %v", err)
	}
}

func TestResolveCond_Between_HighIncompatible_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	_, err := pc.resolveCond(newScope(), &ast.BetweenPredicate{Expr: intLit("5"), Low: intLit("1"), High: strLit("z")})
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeTypeMismatch {
		t.Errorf("expected CodeTypeMismatch, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestResolveCond_BinaryCond_AndOr(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	got, err := pc.resolveCond(newScope(), &ast.BinaryCondition{
		Left:  cmp(intLit("1"), utils.TOKEN_EQ, intLit("1")),
		Op:    utils.TOKEN_AND,
		Right: cmp(intLit("2"), utils.TOKEN_EQ, intLit("2")),
	})
	if err != nil {
		t.Fatalf("resolveCond: %v", err)
	}
	if _, ok := got.(*ResolvedBinaryCond); !ok {
		t.Errorf("expected *ResolvedBinaryCond, got %T", got)
	}
}

func TestResolveCond_Not(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	got, err := pc.resolveCond(newScope(), &ast.NotCondition{Operand: cmp(intLit("1"), utils.TOKEN_EQ, intLit("1"))})
	if err != nil {
		t.Fatalf("resolveCond: %v", err)
	}
	if _, ok := got.(*ResolvedNotCond); !ok {
		t.Errorf("expected *ResolvedNotCond, got %T", got)
	}
}

func TestResolveCond_ParenUnwraps(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	got, err := pc.resolveCond(newScope(), &ast.ParenCondition{Inner: cmp(intLit("1"), utils.TOKEN_EQ, intLit("1"))})
	if err != nil {
		t.Fatalf("resolveCond: %v", err)
	}
	if _, ok := got.(*ResolvedComparison); !ok {
		t.Errorf("expected paren to unwrap directly to *ResolvedComparison, got %T", got)
	}
}

func TestResolveCond_ExprCond_Boolean(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	_, err := pc.resolveCond(newScope(), &ast.ExprCondition{Expr: boolLit("TRUE")})
	if err != nil {
		t.Fatalf("resolveCond: %v", err)
	}
}

func TestResolveCond_ExprCond_NonBoolean_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	_, err := pc.resolveCond(newScope(), &ast.ExprCondition{Expr: intLit("1")})
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeNonBooleanCondition {
		t.Errorf("expected CodeNonBooleanCondition, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestResolveCond_BoolLessThan_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	_, err := pc.resolveCond(newScope(), cmp(boolLit("TRUE"), utils.TOKEN_LT, boolLit("FALSE")))
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeNonOrderableType {
		t.Errorf("expected CodeNonOrderableType, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestResolveCond_BoolEquals_Succeeds(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	_, err := pc.resolveCond(newScope(), cmp(boolLit("TRUE"), utils.TOKEN_EQ, boolLit("FALSE")))
	if err != nil {
		t.Fatalf("expected BOOLEAN = BOOLEAN to succeed, got %v", err)
	}
}

func TestResolveCond_BoolGreaterThanOrEqual_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	_, err := pc.resolveCond(newScope(), cmp(boolLit("TRUE"), utils.TOKEN_GTE, boolLit("FALSE")))
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeNonOrderableType {
		t.Errorf("expected CodeNonOrderableType, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestResolveCond_Between_BoolOperand_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	_, err := pc.resolveCond(newScope(), &ast.BetweenPredicate{Expr: boolLit("TRUE"), Low: boolLit("FALSE"), High: boolLit("TRUE")})
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeNonOrderableType {
		t.Errorf("expected CodeNonOrderableType, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestResolveCond_BoolLessThanNull_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	_, err := pc.resolveCond(newScope(), cmp(boolLit("TRUE"), utils.TOKEN_LT, &ast.NullLiteral{}))
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeNonOrderableType {
		t.Errorf("expected CodeNonOrderableType for TRUE < NULL, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestResolveCond_Between_BoolOperandNullBounds_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	_, err := pc.resolveCond(newScope(), &ast.BetweenPredicate{Expr: boolLit("TRUE"), Low: &ast.NullLiteral{}, High: &ast.NullLiteral{}})
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeNonOrderableType {
		t.Errorf("expected CodeNonOrderableType for TRUE BETWEEN NULL AND NULL, got err=%v diag=%+v", err, pc.diag)
	}
}
