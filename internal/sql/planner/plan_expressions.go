package planner

import (
	"math"
	"strconv"
	"strings"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

// resolveExpr resolves an ast.Expression against scope, producing a fully
// type-annotated ResolvedExpr. Resolution errors are recorded as
// diagnostics on pc.
func (pc *planContext) resolveExpr(scope *Scope, expr ast.Expression) (ResolvedExpr, error) {
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		return pc.resolveIntLiteral(e)
	case *ast.FloatLiteral:
		return pc.resolveFloatLiteral(e)
	case *ast.StringLiteral:
		return &ResolvedStringLiteral{ResolvedExprBase: newExprBase(ast.TypeText), Value: e.Value}, nil
	case *ast.BooleanLiteral:
		return &ResolvedBoolLiteral{
			ResolvedExprBase: newExprBase(ast.TypeBoolean),
			Value:            strings.EqualFold(e.Value, "TRUE"),
		}, nil
	case *ast.NullLiteral:
		return &ResolvedNullLiteral{}, nil
	case *ast.Identifier:
		col, err := pc.resolveColumn(scope, e)
		if err != nil {
			return nil, err
		}
		return &ResolvedColumnRef{Column: col}, nil
	case *ast.ParenExpr:
		return pc.resolveExpr(scope, e.Inner)
	case *ast.BinaryExpr:
		return pc.resolveBinaryExpr(scope, e)
	case *ast.UnaryExpr:
		return pc.resolveUnaryExpr(scope, e)
	case *ast.FunctionCall:
		return pc.resolveFunctionCall(scope, e)
	default:
		return nil, pc.errorf(expr.Span(), CodeUnsupportedExpression, "unsupported expression type %T", expr)
	}
}

// resolveSelectExpression resolves the tagged-union ast.SelectExpression —
// used in SELECT list items, function arguments, and INSERT VALUES rows —
// into a single ResolvedExpr. When the Cond side is set (a comparison or
// other boolean predicate appearing where an expression is expected), the
// resolved condition is wrapped in a ResolvedConditionExpr so every caller
// can treat the result uniformly without knowing the union existed.
func (pc *planContext) resolveSelectExpression(scope *Scope, se *ast.SelectExpression) (ResolvedExpr, error) {
	if se.Expr != nil {
		return pc.resolveExpr(scope, se.Expr)
	}
	cond, err := pc.resolveCond(scope, se.Cond)
	if err != nil {
		return nil, err
	}
	return &ResolvedConditionExpr{Cond: cond}, nil
}

func (pc *planContext) resolveIntLiteral(lit *ast.IntegerLiteral) (ResolvedExpr, error) {
	v, err := strconv.ParseInt(lit.Value, 10, 64)
	if err != nil {
		return nil, pc.errorf(lit.Span(), CodeLiteralOverflow, "integer literal %q does not fit in 64 bits", lit.Value)
	}
	t := ast.TypeBigInt
	if v >= math.MinInt32 && v <= math.MaxInt32 {
		t = ast.TypeInt
	}
	return &ResolvedIntLiteral{ResolvedExprBase: newExprBase(t), Value: v}, nil
}

func (pc *planContext) resolveFloatLiteral(lit *ast.FloatLiteral) (ResolvedExpr, error) {
	v, err := strconv.ParseFloat(lit.Value, 64)
	if err != nil {
		return nil, pc.errorf(lit.Span(), CodeLiteralOverflow, "float literal %q is invalid: %v", lit.Value, err)
	}
	return &ResolvedFloatLiteral{ResolvedExprBase: newExprBase(ast.TypeDouble), Value: v}, nil
}

func (pc *planContext) resolveBinaryExpr(scope *Scope, be *ast.BinaryExpr) (ResolvedExpr, error) {
	left, err := pc.resolveExpr(scope, be.Left)
	if err != nil {
		return nil, err
	}
	right, err := pc.resolveExpr(scope, be.Right)
	if err != nil {
		return nil, err
	}
	resultType, ok := arithmeticResultType(left, right)
	if !ok {
		return nil, pc.errorf(
			be.Span(), CodeNonNumericOperand,
			"arithmetic operator %s requires numeric operands, got %s and %s",
			be.Op, exprTypeName(left), exprTypeName(right),
		)
	}
	return &ResolvedBinaryExpr{ResolvedExprBase: newExprBase(resultType), Left: left, Op: be.Op, Right: right}, nil
}

func (pc *planContext) resolveUnaryExpr(scope *Scope, ue *ast.UnaryExpr) (ResolvedExpr, error) {
	if ue.Op == utils.TOKEN_MINUS {
		if lit, ok := ue.Operand.(*ast.IntegerLiteral); ok {
			v, err := strconv.ParseInt("-"+lit.Value, 10, 64)
			if err != nil {
				return nil, pc.errorf(
					ue.Span(),
					CodeLiteralOverflow,
					"integer literal -%q does not fit in 64 bits",
					lit.Value,
				)
			}
			t := ast.TypeBigInt
			if v >= math.MinInt32 && v <= math.MaxInt32 {
				t = ast.TypeInt
			}
			return &ResolvedIntLiteral{
				ResolvedExprBase: newExprBase(t),
				Value:            v,
			}, nil
		}
	}
	operand, err := pc.resolveExpr(scope, ue.Operand)
	if err != nil {
		return nil, err
	}
	resultType, ok := unaryResultType(operand)
	if !ok {
		return nil, pc.errorf(
			ue.Span(), CodeNonNumericOperand,
			"unary %s requires a numeric operand, got %s", ue.Op, exprTypeName(operand),
		)
	}
	return &ResolvedUnaryExpr{ResolvedExprBase: newExprBase(resultType), Op: ue.Op, Operand: operand}, nil
}

func (pc *planContext) resolveFunctionCall(scope *Scope, fc *ast.FunctionCall) (ResolvedExpr, error) {
	name := strings.ToUpper(fc.Name)
	if !aggregateFunctions[name] {
		return nil, pc.errorf(fc.Span(), CodeUnknownFunction, "unknown function %q", fc.Name)
	}

	if fc.Star {
		if name != "COUNT" {
			return nil, pc.errorf(fc.Span(), CodeInvalidFunctionArgs, "%s(*) is not supported; only COUNT(*) is", name)
		}
		return &ResolvedFunctionCall{
			ResolvedExprBase: newExprBase(ast.TypeBigInt),
			Name:             name,
			Distinct:         fc.Distinct,
			Star:             true,
		}, nil
	}

	if len(fc.Args) != 1 {
		return nil, pc.errorf(
			fc.Span(), CodeInvalidFunctionArgs,
			"%s expects exactly one argument, got %d", name, len(fc.Args),
		)
	}

	arg, err := pc.resolveSelectExpression(scope, fc.Args[0])
	if err != nil {
		return nil, err
	}

	resultType, ok := aggregateResultType(name, arg.ResolvedType())
	if !ok {
		return nil, pc.errorf(
			fc.Span(), CodeInvalidFunctionArgs,
			"%s does not support a %s argument", name, exprTypeName(arg),
		)
	}

	return &ResolvedFunctionCall{
		ResolvedExprBase: newExprBase(resultType),
		Name:             name,
		Distinct:         fc.Distinct,
		Args:             []ResolvedExpr{arg},
	}, nil
}
