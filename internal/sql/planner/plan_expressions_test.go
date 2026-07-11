package planner

import (
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

// userScope builds a single-table scope over shop.users, aliased "u", for
// tests that only need one table in play.
func userScope(t *testing.T, pc *planContext) *Scope {
	t.Helper()
	scope := pc.buildScope([]*ast.TableRef{tableRef(primary(ident("users"), "u"))})
	if pc.diag.HasErrors() {
		t.Fatalf("unexpected diagnostics building scope: %v", pc.diag)
	}
	return scope
}

func intLit(v string) *ast.IntegerLiteral  { return &ast.IntegerLiteral{Value: v} }
func floatLit(v string) *ast.FloatLiteral  { return &ast.FloatLiteral{Value: v} }
func strLit(v string) *ast.StringLiteral   { return &ast.StringLiteral{Value: v} }
func boolLit(v string) *ast.BooleanLiteral { return &ast.BooleanLiteral{Value: v} }

func selExpr(e ast.Expression) *ast.SelectExpression { return &ast.SelectExpression{Expr: e} }
func selCond(c ast.Condition) *ast.SelectExpression  { return &ast.SelectExpression{Cond: c} }

func TestResolveExpr_IntLiteral_Small(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	got, err := pc.resolveExpr(newScope(), intLit("42"))
	if err != nil {
		t.Fatalf("resolveExpr: %v", err)
	}
	lit := got.(*ResolvedIntLiteral)
	if lit.Value != 42 || lit.Type != ast.TypeInt {
		t.Errorf("expected {42, TypeInt}, got %+v", lit)
	}
}

func TestResolveExpr_IntLiteral_Large(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	got, err := pc.resolveExpr(newScope(), intLit("9999999999"))
	if err != nil {
		t.Fatalf("resolveExpr: %v", err)
	}
	lit := got.(*ResolvedIntLiteral)
	if lit.Type != ast.TypeBigInt {
		t.Errorf("expected TypeBigInt for a value beyond int32 range, got %v", lit.Type)
	}
}

func TestResolveExpr_IntLiteral_Overflow(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	_, err := pc.resolveExpr(newScope(), intLit("99999999999999999999999999"))
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeLiteralOverflow {
		t.Errorf("expected CodeLiteralOverflow, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestResolveExpr_FloatLiteral(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	got, err := pc.resolveExpr(newScope(), floatLit("3.14"))
	if err != nil {
		t.Fatalf("resolveExpr: %v", err)
	}
	lit := got.(*ResolvedFloatLiteral)
	if lit.Value != 3.14 || lit.Type != ast.TypeDouble {
		t.Errorf("expected {3.14, TypeDouble}, got %+v", lit)
	}
}

func TestResolveExpr_StringLiteral(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	got, err := pc.resolveExpr(newScope(), strLit("hello"))
	if err != nil {
		t.Fatalf("resolveExpr: %v", err)
	}
	lit := got.(*ResolvedStringLiteral)
	if lit.Value != "hello" || lit.Type != ast.TypeText {
		t.Errorf("expected {hello, TypeText}, got %+v", lit)
	}
}

func TestResolveExpr_BoolLiteral_CaseInsensitive(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	got, err := pc.resolveExpr(newScope(), boolLit("true"))
	if err != nil {
		t.Fatalf("resolveExpr: %v", err)
	}
	if !got.(*ResolvedBoolLiteral).Value {
		t.Error("expected lowercase 'true' to resolve to Value=true")
	}
}

func TestResolveExpr_NullLiteral(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	got, err := pc.resolveExpr(newScope(), &ast.NullLiteral{})
	if err != nil {
		t.Fatalf("resolveExpr: %v", err)
	}
	if !isNullExpr(got) {
		t.Error("expected a *ResolvedNullLiteral")
	}
}

func TestResolveExpr_Identifier(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	scope := userScope(t, pc)

	got, err := pc.resolveExpr(scope, qualifiedIdent("u", "name"))
	if err != nil {
		t.Fatalf("resolveExpr: %v", err)
	}
	ref := got.(*ResolvedColumnRef)
	if ref.Column.Name != "name" || ref.ResolvedType() != ast.TypeVarchar {
		t.Errorf("unexpected column ref: %+v", ref)
	}
}

func TestResolveExpr_ParenUnwraps(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	got, err := pc.resolveExpr(newScope(), &ast.ParenExpr{Inner: intLit("5")})
	if err != nil {
		t.Fatalf("resolveExpr: %v", err)
	}
	if _, ok := got.(*ResolvedIntLiteral); !ok {
		t.Errorf("expected paren to unwrap directly to *ResolvedIntLiteral, got %T", got)
	}
}

// --- arithmetic promotion -----------------------------------------------

func TestResolveExpr_BinaryExpr_IntPlusInt(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	got, err := pc.resolveExpr(newScope(), &ast.BinaryExpr{Left: intLit("1"), Op: utils.TOKEN_PLUS, Right: intLit("2")})
	if err != nil {
		t.Fatalf("resolveExpr: %v", err)
	}
	if got.ResolvedType() != ast.TypeInt {
		t.Errorf("expected TypeInt, got %v", got.ResolvedType())
	}
}

func TestResolveExpr_BinaryExpr_IntPlusFloat_PromotesToDouble(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	got, err := pc.resolveExpr(newScope(), &ast.BinaryExpr{Left: intLit("1"), Op: utils.TOKEN_PLUS, Right: floatLit("2.5")})
	if err != nil {
		t.Fatalf("resolveExpr: %v", err)
	}
	if got.ResolvedType() != ast.TypeDouble {
		t.Errorf("expected TypeDouble (wider operand wins), got %v", got.ResolvedType())
	}
}

func TestResolveExpr_BinaryExpr_NullPlusInt_TakesOtherSide(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	got, err := pc.resolveExpr(newScope(), &ast.BinaryExpr{Left: &ast.NullLiteral{}, Op: utils.TOKEN_PLUS, Right: intLit("1")})
	if err != nil {
		t.Fatalf("resolveExpr: %v", err)
	}
	if got.ResolvedType() != ast.TypeInt {
		t.Errorf("expected TypeInt from the non-NULL side, got %v", got.ResolvedType())
	}
}

func TestResolveExpr_BinaryExpr_StringPlusInt_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	_, err := pc.resolveExpr(newScope(), &ast.BinaryExpr{Left: strLit("a"), Op: utils.TOKEN_PLUS, Right: intLit("1")})
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeNonNumericOperand {
		t.Errorf("expected CodeNonNumericOperand, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestResolveExpr_UnaryExpr_Numeric(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	got, err := pc.resolveExpr(newScope(), &ast.UnaryExpr{Op: utils.TOKEN_MINUS, Operand: intLit("5")})
	if err != nil {
		t.Fatalf("resolveExpr: %v", err)
	}
	if got.ResolvedType() != ast.TypeInt {
		t.Errorf("expected TypeInt, got %v", got.ResolvedType())
	}
}

func TestResolveExpr_UnaryExpr_NonNumeric_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	_, err := pc.resolveExpr(newScope(), &ast.UnaryExpr{Op: utils.TOKEN_MINUS, Operand: strLit("a")})
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeNonNumericOperand {
		t.Errorf("expected CodeNonNumericOperand, got err=%v diag=%+v", err, pc.diag)
	}
}

// --- function calls -------------------------------------------------

func TestResolveExpr_CountStar(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	got, err := pc.resolveExpr(newScope(), &ast.FunctionCall{Name: "COUNT", Star: true})
	if err != nil {
		t.Fatalf("resolveExpr: %v", err)
	}
	fc := got.(*ResolvedFunctionCall)
	if !fc.Star || fc.Type != ast.TypeBigInt {
		t.Errorf("expected COUNT(*) -> {Star:true, TypeBigInt}, got %+v", fc)
	}
}

func TestResolveExpr_SumInt_WidensToBigInt(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	scope := userScope(t, pc)
	got, err := pc.resolveExpr(scope, &ast.FunctionCall{Name: "sum", Args: []*ast.SelectExpression{selExpr(qualifiedIdent("u", "id"))}})
	if err != nil {
		t.Fatalf("resolveExpr: %v", err)
	}
	if got.ResolvedType() != ast.TypeBigInt {
		t.Errorf("expected SUM(int) to widen to TypeBigInt, got %v", got.ResolvedType())
	}
}

func TestResolveExpr_AvgInt_ProducesDouble(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	scope := userScope(t, pc)
	got, err := pc.resolveExpr(scope, &ast.FunctionCall{Name: "AVG", Args: []*ast.SelectExpression{selExpr(qualifiedIdent("u", "id"))}})
	if err != nil {
		t.Fatalf("resolveExpr: %v", err)
	}
	if got.ResolvedType() != ast.TypeDouble {
		t.Errorf("expected AVG(int) -> TypeDouble, got %v", got.ResolvedType())
	}
}

func TestResolveExpr_MinPassesThroughType(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	scope := userScope(t, pc)
	got, err := pc.resolveExpr(scope, &ast.FunctionCall{Name: "MIN", Args: []*ast.SelectExpression{selExpr(qualifiedIdent("u", "name"))}})
	if err != nil {
		t.Fatalf("resolveExpr: %v", err)
	}
	if got.ResolvedType() != ast.TypeVarchar {
		t.Errorf("expected MIN(varchar) to pass through as TypeVarchar, got %v", got.ResolvedType())
	}
}

func TestResolveExpr_SumString_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{ActiveDatabase: "shop"}, nil)
	scope := userScope(t, pc)
	_, err := pc.resolveExpr(scope, &ast.FunctionCall{Name: "SUM", Args: []*ast.SelectExpression{selExpr(qualifiedIdent("u", "name"))}})
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeInvalidFunctionArgs {
		t.Errorf("expected CodeInvalidFunctionArgs, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestResolveExpr_UnknownFunction_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	_, err := pc.resolveExpr(newScope(), &ast.FunctionCall{Name: "MEDIAN", Args: []*ast.SelectExpression{selExpr(intLit("1"))}})
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeUnknownFunction {
		t.Errorf("expected CodeUnknownFunction, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestResolveExpr_WrongArgCount_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	_, err := pc.resolveExpr(newScope(), &ast.FunctionCall{Name: "SUM", Args: []*ast.SelectExpression{selExpr(intLit("1")), selExpr(intLit("2"))}})
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeInvalidFunctionArgs {
		t.Errorf("expected CodeInvalidFunctionArgs, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestResolveExpr_StarOnNonCount_Errors(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	_, err := pc.resolveExpr(newScope(), &ast.FunctionCall{Name: "SUM", Star: true})
	if err == nil || pc.diag[len(pc.diag)-1].Code != CodeInvalidFunctionArgs {
		t.Errorf("expected CodeInvalidFunctionArgs, got err=%v diag=%+v", err, pc.diag)
	}
}

func TestResolveSelectExpression_ExprSide(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	got, err := pc.resolveSelectExpression(newScope(), selExpr(intLit("1")))
	if err != nil {
		t.Fatalf("resolveSelectExpression: %v", err)
	}
	if _, ok := got.(*ResolvedIntLiteral); !ok {
		t.Errorf("expected *ResolvedIntLiteral, got %T", got)
	}
}

func TestResolveSelectExpression_CondSide_WrapsAsBoolean(t *testing.T) {
	pc := newPlanContext(testCatalog(), Session{}, nil)
	cond := &ast.ComparisonPredicate{Left: intLit("1"), Op: utils.TOKEN_EQ, Right: intLit("1")}
	got, err := pc.resolveSelectExpression(newScope(), selCond(cond))
	if err != nil {
		t.Fatalf("resolveSelectExpression: %v", err)
	}
	wrapped, ok := got.(*ResolvedConditionExpr)
	if !ok {
		t.Fatalf("expected *ResolvedConditionExpr, got %T", got)
	}
	if wrapped.ResolvedType() != ast.TypeBoolean {
		t.Errorf("expected TypeBoolean, got %v", wrapped.ResolvedType())
	}
	if _, ok := wrapped.Cond.(*ResolvedComparison); !ok {
		t.Errorf("expected wrapped condition to be *ResolvedComparison, got %T", wrapped.Cond)
	}
}
