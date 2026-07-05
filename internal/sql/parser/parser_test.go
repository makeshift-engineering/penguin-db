package parser

import (
	"errors"
	"reflect"
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
	"github.com/makeshift-engineering/penguin-db/internal/sql/lexer"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

// parseForTest runs the lexer and parser on the given input, returning the resulting AST.
func parseForTest(t *testing.T, input string) (*ast.Program, error) {
	t.Helper()
	l := lexer.NewLexer("test", input)
	tokens := l.Tokenize()
	if l.Diagnostics().HasErrors() {
		return nil, l.Diagnostics().AsError()
	}

	p := New(tokens, &diagnostic.Source{Name: "test", Text: input})
	prog, err := p.Parse()
	if err != nil {
		return nil, err
	}
	return prog, nil
}

// requireAST runs the parser on the input and asserts that it matches the expected AST.
// Spans are ignored in the comparison.
func requireAST(t *testing.T, input string, expected *ast.Program) {
	t.Helper()
	actual, err := parseForTest(t, input)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if !deepEqualNoSpan(reflect.ValueOf(expected), reflect.ValueOf(actual)) {
		t.Errorf("AST mismatch for %q\nExpected: %#v\nActual:   %#v", input, expected, actual)
	}
}

// requireParseError runs the parser and asserts that it produces a specific error code
// at a specific starting line and column.
func requireParseError(t *testing.T, input string, expectedErr error, line, col int) {
	t.Helper()
	_, err := parseForTest(t, input)
	if err == nil {
		t.Fatalf("expected error wrapping %v, got none", expectedErr)
	}
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error wrapping %v, got: %v", expectedErr, err)
	}
	var diagList diagnostic.List
	if errors.As(err, &diagList) {
		if len(diagList) == 0 {
			t.Fatalf("expected diagnostics, got empty list")
		}
		first := diagList[0]
		if first.Span.Start.Line != line || first.Span.Start.Col != col {
			t.Errorf("diagnostic location mismatch for %q:\nGot:  line %d, col %d\nWant: line %d, col %d\nDiagnostic: %v",
				input, first.Span.Start.Line, first.Span.Start.Col, line, col, first)
		}
	} else {
		var first *diagnostic.Diagnostic
		if errors.As(err, &first) {
			if first.Span.Start.Line != line || first.Span.Start.Col != col {
				t.Errorf("diagnostic location mismatch for %q:\nGot:  line %d, col %d\nWant: line %d, col %d\nDiagnostic: %v",
					input, first.Span.Start.Line, first.Span.Start.Col, line, col, first)
			}
		} else {
			t.Fatalf("expected error to be *diagnostic.Diagnostic or diagnostic.List, got: %T (%v)", err, err)
		}
	}
}

var spanType = reflect.TypeOf(diagnostic.Span{})

// deepEqualNoSpan performs a recursive deep equality check, ignoring diagnostic Span fields.
func deepEqualNoSpan(v1, v2 reflect.Value) bool {
	if !v1.IsValid() || !v2.IsValid() {
		return v1.IsValid() == v2.IsValid()
	}
	if v1.Type() != v2.Type() {
		return false
	}

	if v1.Type() == spanType {
		return true // skip span comparison
	}

	switch v1.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v1.IsNil() || v2.IsNil() {
			return v1.IsNil() == v2.IsNil()
		}
		return deepEqualNoSpan(v1.Elem(), v2.Elem())
	case reflect.Slice:
		if v1.IsNil() != v2.IsNil() {
			return false
		}
		if v1.Len() != v2.Len() {
			return false
		}
		for i := 0; i < v1.Len(); i++ {
			if !deepEqualNoSpan(v1.Index(i), v2.Index(i)) {
				return false
			}
		}
		return true
	case reflect.Struct:
		for i := 0; i < v1.NumField(); i++ {
			if !deepEqualNoSpan(v1.Field(i), v2.Field(i)) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(v1.Interface(), v2.Interface())
	}
}

// ptr returns a pointer to the given value.
func ptr[T any](v T) *T {
	return &v
}

// TestGrammar_ArithmeticExpressions validates operator-precedence, associativity,
// unary signs, parentheses, and the modulo operator in expression position.
func TestGrammar_ArithmeticExpressions(t *testing.T) {
	t.Run("addition then multiplication: right operand binds tighter (1 + 2 * 3)", func(t *testing.T) {
		requireAST(t, "SELECT 1 + 2 * 3;", exprQuery(&ast.BinaryExpr{
			Left: &ast.IntegerLiteral{Value: "1"},
			Op:   utils.TOKEN_PLUS,
			Right: &ast.BinaryExpr{
				Left:  &ast.IntegerLiteral{Value: "2"},
				Op:    utils.TOKEN_STAR,
				Right: &ast.IntegerLiteral{Value: "3"},
			},
		}))
	})

	t.Run("multiplication then addition: left operand binds tighter (1 * 2 + 3)", func(t *testing.T) {
		requireAST(t, "SELECT 1 * 2 + 3;", exprQuery(&ast.BinaryExpr{
			Left: &ast.BinaryExpr{
				Left:  &ast.IntegerLiteral{Value: "1"},
				Op:    utils.TOKEN_STAR,
				Right: &ast.IntegerLiteral{Value: "2"},
			},
			Op:    utils.TOKEN_PLUS,
			Right: &ast.IntegerLiteral{Value: "3"},
		}))
	})

	t.Run("subtraction left-associativity: 10 - 3 - 2 groups as (10 - 3) - 2", func(t *testing.T) {
		requireAST(t, "SELECT 10 - 3 - 2;", exprQuery(&ast.BinaryExpr{
			Left: &ast.BinaryExpr{
				Left:  &ast.IntegerLiteral{Value: "10"},
				Op:    utils.TOKEN_MINUS,
				Right: &ast.IntegerLiteral{Value: "3"},
			},
			Op:    utils.TOKEN_MINUS,
			Right: &ast.IntegerLiteral{Value: "2"},
		}))
	})

	t.Run("parentheses override multiplication precedence: (1 + 2) * 3", func(t *testing.T) {
		requireAST(t, "SELECT (1 + 2) * 3;", exprQuery(&ast.BinaryExpr{
			Left: &ast.ParenExpr{
				Inner: &ast.BinaryExpr{
					Left:  &ast.IntegerLiteral{Value: "1"},
					Op:    utils.TOKEN_PLUS,
					Right: &ast.IntegerLiteral{Value: "2"},
				},
			},
			Op:    utils.TOKEN_STAR,
			Right: &ast.IntegerLiteral{Value: "3"},
		}))
	})

	t.Run("complex four-operator expression with precedence grouping: a + b*c - d/e", func(t *testing.T) {
		requireAST(t, "SELECT a + b * c - d / e;", exprQuery(&ast.BinaryExpr{
			Left: &ast.BinaryExpr{
				Left: &ast.Identifier{Name: "a"},
				Op:   utils.TOKEN_PLUS,
				Right: &ast.BinaryExpr{
					Left:  &ast.Identifier{Name: "b"},
					Op:    utils.TOKEN_STAR,
					Right: &ast.Identifier{Name: "c"},
				},
			},
			Op: utils.TOKEN_MINUS,
			Right: &ast.BinaryExpr{
				Left:  &ast.Identifier{Name: "d"},
				Op:    utils.TOKEN_SLASH,
				Right: &ast.Identifier{Name: "e"},
			},
		}))
	})

	t.Run("double unary minus on float literal: - - 3.14", func(t *testing.T) {
		requireAST(t, "SELECT - - 3.14;", exprQuery(&ast.UnaryExpr{
			Op: utils.TOKEN_MINUS,
			Operand: &ast.UnaryExpr{
				Op:      utils.TOKEN_MINUS,
				Operand: &ast.FloatLiteral{Value: "3.14"},
			},
		}))
	})

	t.Run("modulo operator: id % 2", func(t *testing.T) {
		requireAST(t, "SELECT id % 2;", exprQuery(&ast.BinaryExpr{
			Left:  &ast.Identifier{Name: "id"},
			Op:    utils.TOKEN_PERCENT,
			Right: &ast.IntegerLiteral{Value: "2"},
		}))
	})
}

// TestGrammar_ArithmeticExpressionErrors validates that malformed expressions
// are rejected at exactly the right token position.
func TestGrammar_ArithmeticExpressionErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
		code  error
		line  int
		col   int
	}{
		{"binary operator with no right-hand operand", "SELECT 1 + ;", CodeExpectedExpression, 1, 12},
		{"trailing multiplicative operator on qualified identifier", "SELECT tbl.col * ;", CodeExpectedExpression, 1, 18},
		{"unmatched opening paren in expression", "SELECT (1 + 2;", CodeUnexpectedToken, 1, 14},
		{"empty parens in expression position", "SELECT ();", CodeExpectedExpression, 1, 9},
		{"keyword where expression expected", "SELECT SELECT;", CodeExpectedExpression, 1, 8},
		{"trailing multiply with no right operand", "SELECT 1 *;", CodeExpectedExpression, 1, 11},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireParseError(t, tt.input, tt.code, tt.line, tt.col)
		})
	}
}

// TestGrammar_SelectColumnDisambiguation validates the parser's ability to
// correctly handle qualified identifiers (tbl.col) that continue into arithmetic,
// predicate comparisons, or logical conditions in a SELECT column list.
//
// This exercises parseExpressionFromFactor and selectExpressionFromExpression —
// paths only reachable when the leading IDENT . IDENT prefix has already been
// consumed before the remaining tail is parsed.
func TestGrammar_SelectColumnDisambiguation(t *testing.T) {
	t.Run("qualified identifier multiplied by literal (ExpressionFromFactor multiply path)", func(t *testing.T) {
		requireAST(t, "SELECT tbl.col * 5;", exprQuery(&ast.BinaryExpr{
			Left:  &ast.Identifier{Qualifier: "tbl", Name: "col"},
			Op:    utils.TOKEN_STAR,
			Right: &ast.IntegerLiteral{Value: "5"},
		}))
	})

	t.Run("qualified identifier added to literal (ExpressionFromFactor addition path)", func(t *testing.T) {
		requireAST(t, "SELECT tbl.col + 2;", exprQuery(&ast.BinaryExpr{
			Left:  &ast.Identifier{Qualifier: "tbl", Name: "col"},
			Op:    utils.TOKEN_PLUS,
			Right: &ast.IntegerLiteral{Value: "2"},
		}))
	})

	t.Run("qualified identifier compared in select list → wraps into Cond SelectExpression", func(t *testing.T) {
		requireAST(t, "SELECT tbl.col > 5;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					Columns: []*ast.SelectColumn{
						{Expr: &ast.SelectExpression{
							Cond: &ast.ComparisonPredicate{
								Left:  &ast.Identifier{Qualifier: "tbl", Name: "col"},
								Op:    utils.TOKEN_GT,
								Right: &ast.IntegerLiteral{Value: "5"},
							},
						}},
					},
				},
			},
		})
	})

	t.Run("qualified identifier followed by AND → wraps into BinaryCondition SelectExpression", func(t *testing.T) {
		requireAST(t, "SELECT tbl.col AND active;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					Columns: []*ast.SelectColumn{
						{Expr: &ast.SelectExpression{
							Cond: &ast.BinaryCondition{
								Left:  &ast.ExprCondition{Expr: &ast.Identifier{Qualifier: "tbl", Name: "col"}},
								Op:    utils.TOKEN_AND,
								Right: &ast.ExprCondition{Expr: &ast.Identifier{Name: "active"}},
							},
						}},
					},
				},
			},
		})
	})

	t.Run("NOT prefix in select list → wraps into NotCondition SelectExpression", func(t *testing.T) {
		requireAST(t, "SELECT NOT active;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					Columns: []*ast.SelectColumn{
						{Expr: &ast.SelectExpression{
							Cond: &ast.NotCondition{
								Operand: &ast.ExprCondition{Expr: &ast.Identifier{Name: "active"}},
							},
						}},
					},
				},
			},
		})
	})

	t.Run("trailing operator after qualified identifier fails at semicolon", func(t *testing.T) {
		requireParseError(t, "SELECT tbl.col + ;", CodeExpectedExpression, 1, 18)
	})
}

// TestGrammar_FunctionCalls validates all function-call variants:
// zero arguments, star wildcard, DISTINCT, single arithmetic argument,
// and multiple arguments.
func TestGrammar_FunctionCalls(t *testing.T) {
	t.Run("zero-argument function call: NOW()", func(t *testing.T) {
		requireAST(t, "SELECT NOW();", exprQuery(&ast.FunctionCall{Name: "NOW"}))
	})

	t.Run("COUNT(*) wildcard aggregate", func(t *testing.T) {
		requireAST(t, "SELECT COUNT(*);", exprQuery(&ast.FunctionCall{Name: "COUNT", Star: true}))
	})

	t.Run("COUNT(DISTINCT id) distinct aggregate", func(t *testing.T) {
		requireAST(t, "SELECT COUNT(DISTINCT id);", exprQuery(&ast.FunctionCall{
			Name:     "COUNT",
			Distinct: true,
			Args:     []*ast.SelectExpression{{Expr: &ast.Identifier{Name: "id"}}},
		}))
	})

	t.Run("ABS with arithmetic argument: ABS(a - b)", func(t *testing.T) {
		requireAST(t, "SELECT ABS(a - b);", exprQuery(&ast.FunctionCall{
			Name: "ABS",
			Args: []*ast.SelectExpression{
				{Expr: &ast.BinaryExpr{
					Left:  &ast.Identifier{Name: "a"},
					Op:    utils.TOKEN_MINUS,
					Right: &ast.Identifier{Name: "b"},
				}},
			},
		}))
	})

	t.Run("COALESCE with expression and float fallback: COALESCE(price * 0.9, 0.0)", func(t *testing.T) {
		requireAST(t, "SELECT COALESCE(price * 0.9, 0.0);", exprQuery(&ast.FunctionCall{
			Name: "COALESCE",
			Args: []*ast.SelectExpression{
				{Expr: &ast.BinaryExpr{
					Left:  &ast.Identifier{Name: "price"},
					Op:    utils.TOKEN_STAR,
					Right: &ast.FloatLiteral{Value: "0.9"},
				}},
				{Expr: &ast.FloatLiteral{Value: "0.0"}},
			},
		}))
	})

	t.Run("function missing closing paren fails at the token after args", func(t *testing.T) {
		requireParseError(t, "SELECT UPPER(a;", CodeUnexpectedToken, 1, 15)
	})
}

// TestGrammar_Predicates validates every predicate variant:
// IS NULL, IS NOT NULL, LIKE, NOT LIKE, IN, NOT IN, BETWEEN, NOT BETWEEN,
// and all six comparison operators.
func TestGrammar_Predicates(t *testing.T) {
	t.Run("IS NULL predicate", func(t *testing.T) {
		requireAST(t, "DELETE FROM t WHERE email IS NULL;", condQuery(&ast.IsNullPredicate{
			Expr: &ast.Identifier{Name: "email"}, Negated: false,
		}))
	})

	t.Run("IS NOT NULL predicate", func(t *testing.T) {
		requireAST(t, "DELETE FROM t WHERE email IS NOT NULL;", condQuery(&ast.IsNullPredicate{
			Expr: &ast.Identifier{Name: "email"}, Negated: true,
		}))
	})

	t.Run("LIKE predicate", func(t *testing.T) {
		requireAST(t, "DELETE FROM t WHERE name LIKE '%foo%';", condQuery(&ast.LikePredicate{
			Left: &ast.Identifier{Name: "name"}, Pattern: &ast.StringLiteral{Value: "%foo%"}, Negated: false,
		}))
	})

	t.Run("NOT LIKE predicate", func(t *testing.T) {
		requireAST(t, "DELETE FROM t WHERE name NOT LIKE '%bar%';", condQuery(&ast.LikePredicate{
			Left: &ast.Identifier{Name: "name"}, Pattern: &ast.StringLiteral{Value: "%bar%"}, Negated: true,
		}))
	})

	t.Run("IN predicate with string literals", func(t *testing.T) {
		requireAST(t, "DELETE FROM t WHERE status IN ('active', 'pending');", condQuery(&ast.InPredicate{
			Expr:    &ast.Identifier{Name: "status"},
			Values:  []ast.Expression{&ast.StringLiteral{Value: "active"}, &ast.StringLiteral{Value: "pending"}},
			Negated: false,
		}))
	})

	t.Run("NOT IN predicate with integer literals", func(t *testing.T) {
		requireAST(t, "DELETE FROM t WHERE id NOT IN (1, 2, 3);", condQuery(&ast.InPredicate{
			Expr: &ast.Identifier{Name: "id"},
			Values: []ast.Expression{
				&ast.IntegerLiteral{Value: "1"},
				&ast.IntegerLiteral{Value: "2"},
				&ast.IntegerLiteral{Value: "3"},
			},
			Negated: true,
		}))
	})

	t.Run("BETWEEN predicate", func(t *testing.T) {
		requireAST(t, "DELETE FROM t WHERE age BETWEEN 18 AND 65;", condQuery(&ast.BetweenPredicate{
			Expr: &ast.Identifier{Name: "age"}, Low: &ast.IntegerLiteral{Value: "18"},
			High: &ast.IntegerLiteral{Value: "65"}, Negated: false,
		}))
	})

	t.Run("NOT BETWEEN predicate", func(t *testing.T) {
		requireAST(t, "DELETE FROM t WHERE score NOT BETWEEN 0 AND 50;", condQuery(&ast.BetweenPredicate{
			Expr: &ast.Identifier{Name: "score"}, Low: &ast.IntegerLiteral{Value: "0"},
			High: &ast.IntegerLiteral{Value: "50"}, Negated: true,
		}))
	})

	t.Run("all comparison operators (=, !=, <, >, <=, >=)", func(t *testing.T) {
		ops := []struct {
			sql string
			tok utils.TokenType
		}{
			{"DELETE FROM t WHERE a = 1;", utils.TOKEN_EQ},
			{"DELETE FROM t WHERE a != 1;", utils.TOKEN_NEQ},
			{"DELETE FROM t WHERE a < 1;", utils.TOKEN_LT},
			{"DELETE FROM t WHERE a > 1;", utils.TOKEN_GT},
			{"DELETE FROM t WHERE a <= 1;", utils.TOKEN_LTE},
			{"DELETE FROM t WHERE a >= 1;", utils.TOKEN_GTE},
		}
		for _, c := range ops {
			c := c
			t.Run(c.tok.String(), func(t *testing.T) {
				requireAST(t, c.sql, condQuery(&ast.ComparisonPredicate{
					Left: &ast.Identifier{Name: "a"}, Op: c.tok, Right: &ast.IntegerLiteral{Value: "1"},
				}))
			})
		}
	})

	t.Run("LIKE with integer pattern is accepted syntactically", func(t *testing.T) {
		requireAST(t, "DELETE FROM t WHERE name LIKE 123;", condQuery(
			&ast.LikePredicate{
				Left:    &ast.Identifier{Name: "name"},
				Pattern: &ast.IntegerLiteral{Value: "123"},
				Negated: false,
			},
		))
	})
}

// TestGrammar_PredicateErrors validates that every malformed predicate is
// rejected with the correct error code and token position.
func TestGrammar_PredicateErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
		code  error
		line  int
		col   int
	}{
		{"IS followed by non-NULL token", "DELETE FROM t WHERE a IS TRUE;", CodeUnexpectedToken, 1, 26},
		{"IN with empty value list", "DELETE FROM t WHERE a IN ();", CodeExpectedExpression, 1, 27},
		{"IN missing closing paren", "DELETE FROM t WHERE a IN (1, 2;", CodeUnexpectedToken, 1, 31},
		{"NOT followed by unexpected token in predicate position", "DELETE FROM t WHERE a NOT = 1;", CodeUnexpectedToken, 1, 23},
		{"BETWEEN missing AND separator", "DELETE FROM t WHERE a BETWEEN 1 OR 2;", CodeUnexpectedToken, 1, 33},
		{"empty WHERE clause", "DELETE FROM t WHERE ;", CodeExpectedExpression, 1, 21},
		{"missing right operand of comparison", "DELETE FROM t WHERE a = ;", CodeExpectedExpression, 1, 25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireParseError(t, tt.input, tt.code, tt.line, tt.col)
		})
	}
}

// TestGrammar_LogicalOperators validates AND/OR precedence, NOT negation,
// double NOT, and complex mixed condition chains.
func TestGrammar_LogicalOperators(t *testing.T) {
	t.Run("AND has higher precedence than OR: a=1 OR b=2 AND c=3 → a=1 OR (b=2 AND c=3)", func(t *testing.T) {
		requireAST(t, "DELETE FROM t WHERE a = 1 OR b = 2 AND c = 3;",
			condQuery(&ast.BinaryCondition{
				Left: &ast.ComparisonPredicate{
					Left: &ast.Identifier{Name: "a"}, Op: utils.TOKEN_EQ, Right: &ast.IntegerLiteral{Value: "1"},
				},
				Op: utils.TOKEN_OR,
				Right: &ast.BinaryCondition{
					Left: &ast.ComparisonPredicate{
						Left: &ast.Identifier{Name: "b"}, Op: utils.TOKEN_EQ, Right: &ast.IntegerLiteral{Value: "2"},
					},
					Op: utils.TOKEN_AND,
					Right: &ast.ComparisonPredicate{
						Left: &ast.Identifier{Name: "c"}, Op: utils.TOKEN_EQ, Right: &ast.IntegerLiteral{Value: "3"},
					},
				},
			}))
	})

	t.Run("NOT negates a comparison predicate", func(t *testing.T) {
		requireAST(t, "DELETE FROM t WHERE NOT a = 1;", condQuery(&ast.NotCondition{
			Operand: &ast.ComparisonPredicate{
				Left: &ast.Identifier{Name: "a"}, Op: utils.TOKEN_EQ, Right: &ast.IntegerLiteral{Value: "1"},
			},
		}))
	})

	t.Run("NOT NOT double negation is right-recursive", func(t *testing.T) {
		requireAST(t, "DELETE FROM t WHERE NOT NOT active;", condQuery(&ast.NotCondition{
			Operand: &ast.NotCondition{
				Operand: &ast.ExprCondition{Expr: &ast.Identifier{Name: "active"}},
			},
		}))
	})

	t.Run("complex chain: BETWEEN AND IS NOT NULL AND NOT deleted", func(t *testing.T) {
		requireAST(t,
			"DELETE FROM t WHERE age BETWEEN 18 AND 65 AND email IS NOT NULL AND NOT deleted;",
			condQuery(&ast.BinaryCondition{
				Left: &ast.BinaryCondition{
					Left: &ast.BetweenPredicate{
						Expr: &ast.Identifier{Name: "age"},
						Low:  &ast.IntegerLiteral{Value: "18"},
						High: &ast.IntegerLiteral{Value: "65"},
					},
					Op: utils.TOKEN_AND,
					Right: &ast.IsNullPredicate{
						Expr: &ast.Identifier{Name: "email"}, Negated: true,
					},
				},
				Op: utils.TOKEN_AND,
				Right: &ast.NotCondition{
					Operand: &ast.ExprCondition{Expr: &ast.Identifier{Name: "deleted"}},
				},
			}))
	})
}

// TestGrammar_ParenthesizedConditions validates the disambiguation between
// parenthesized arithmetic expressions and parenthesized boolean conditions,
// and all edge cases of the condition-continuation path.
func TestGrammar_ParenthesizedConditions(t *testing.T) {
	t.Run("paren expression as LHS of comparison: (a + 1) > 5", func(t *testing.T) {
		requireAST(t, "DELETE FROM t WHERE (a + 1) > 5;", condQuery(&ast.ComparisonPredicate{
			Left: &ast.ParenExpr{
				Inner: &ast.BinaryExpr{
					Left:  &ast.Identifier{Name: "a"},
					Op:    utils.TOKEN_PLUS,
					Right: &ast.IntegerLiteral{Value: "1"},
				},
			},
			Op:    utils.TOKEN_GT,
			Right: &ast.IntegerLiteral{Value: "5"},
		}))
	})

	t.Run("parenthesized bare logical condition: (active AND ok)", func(t *testing.T) {
		requireAST(t, "DELETE FROM t WHERE (active AND ok);",
			condQuery(&ast.ParenCondition{
				Inner: &ast.BinaryCondition{
					Left:  &ast.ExprCondition{Expr: &ast.Identifier{Name: "active"}},
					Op:    utils.TOKEN_AND,
					Right: &ast.ExprCondition{Expr: &ast.Identifier{Name: "ok"}},
				},
			}))
	})

	t.Run("parenthesized comparison chain: (a > 5 AND b < 10)", func(t *testing.T) {
		requireAST(t, "DELETE FROM t WHERE (a > 5 AND b < 10);",
			condQuery(&ast.ParenCondition{
				Inner: &ast.BinaryCondition{
					Left: &ast.ComparisonPredicate{
						Left: &ast.Identifier{Name: "a"}, Op: utils.TOKEN_GT, Right: &ast.IntegerLiteral{Value: "5"},
					},
					Op: utils.TOKEN_AND,
					Right: &ast.ComparisonPredicate{
						Left: &ast.Identifier{Name: "b"}, Op: utils.TOKEN_LT, Right: &ast.IntegerLiteral{Value: "10"},
					},
				},
			}))
	})

	t.Run("arithmetic on left of comparison with qualified columns: t.a + 5 = t.b", func(t *testing.T) {
		requireAST(t, "DELETE FROM t WHERE t.a + 5 = t.b;", condQuery(&ast.ComparisonPredicate{
			Left: &ast.BinaryExpr{
				Left:  &ast.Identifier{Qualifier: "t", Name: "a"},
				Op:    utils.TOKEN_PLUS,
				Right: &ast.IntegerLiteral{Value: "5"},
			},
			Op:    utils.TOKEN_EQ,
			Right: &ast.Identifier{Qualifier: "t", Name: "b"},
		}))
	})
}

// TestGrammar_ParenthesizedConditionErrors validates errors inside parenthesized
// condition expressions.
func TestGrammar_ParenthesizedConditionErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
		code  error
		line  int
		col   int
	}{
		{"comparison with missing right operand inside paren", "DELETE FROM t WHERE (a = );", CodeExpectedExpression, 1, 26},
		{"bare expression with no condition continuation inside paren", "DELETE FROM t WHERE (a + 5 12);", CodeExpectedCondition, 1, 28},
		{"unclosed parenthesized condition", "DELETE FROM t WHERE (a = 1;", CodeUnexpectedToken, 1, 27},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireParseError(t, tt.input, tt.code, tt.line, tt.col)
		})
	}
}

// TestGrammar_SelectColumns validates all SELECT column-list variants:
// wildcard star, qualified wildcard, explicit alias, implicit alias,
// and NOT LIKE in the select list.
func TestGrammar_SelectColumns(t *testing.T) {
	t.Run("SELECT * (unqualified wildcard)", func(t *testing.T) {
		requireAST(t, "SELECT * FROM orders;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					Columns: []*ast.SelectColumn{{Star: true}},
					From:    []*ast.TableRef{{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "orders"}}}},
				},
			},
		})
	})

	t.Run("SELECT o.* (qualified wildcard) with implicit table alias", func(t *testing.T) {
		requireAST(t, "SELECT o.* FROM orders o;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					Columns: []*ast.SelectColumn{{QualifiedStar: &ast.Identifier{Name: "o"}}},
					From:    []*ast.TableRef{{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "orders"}, Alias: "o"}}},
				},
			},
		})
	})

	t.Run("SELECT expression with explicit AS alias", func(t *testing.T) {
		requireAST(t, "SELECT price * 1.1 AS taxed_price FROM products;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					Columns: []*ast.SelectColumn{
						{
							Expr: &ast.SelectExpression{Expr: &ast.BinaryExpr{
								Left: &ast.Identifier{Name: "price"}, Op: utils.TOKEN_STAR,
								Right: &ast.FloatLiteral{Value: "1.1"},
							}},
							Alias: "taxed_price",
						},
					},
					From: []*ast.TableRef{{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "products"}}}},
				},
			},
		})
	})

	t.Run("SELECT with NOT LIKE predicate in column list with alias", func(t *testing.T) {
		requireAST(t, "SELECT name NOT LIKE '%tmp%' AS is_real FROM t;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					Columns: []*ast.SelectColumn{
						{
							Expr: &ast.SelectExpression{Cond: &ast.LikePredicate{
								Left: &ast.Identifier{Name: "name"}, Pattern: &ast.StringLiteral{Value: "%tmp%"}, Negated: true,
							}},
							Alias: "is_real",
						},
					},
					From: []*ast.TableRef{{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}}},
				},
			},
		})
	})
}

// TestGrammar_SelectColumnErrors validates errors in the SELECT column list.
func TestGrammar_SelectColumnErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
		code  error
		line  int
		col   int
	}{
		{"SELECT with no column list fails at FROM keyword", "SELECT FROM t;", CodeExpectedExpression, 1, 8},
		{"SELECT DISTINCT missing column list fails at FROM", "SELECT DISTINCT FROM t;", CodeExpectedExpression, 1, 17},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireParseError(t, tt.input, tt.code, tt.line, tt.col)
		})
	}
}

// TestGrammar_Joins validates all supported JOIN variants and their ON conditions.
func TestGrammar_Joins(t *testing.T) {
	t.Run("INNER JOIN ON qualified-column equality", func(t *testing.T) {
		requireAST(t, "SELECT o.id FROM orders o INNER JOIN customers c ON o.customer_id = c.id;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					Columns: []*ast.SelectColumn{
						{Expr: &ast.SelectExpression{Expr: &ast.Identifier{Qualifier: "o", Name: "id"}}},
					},
					From: []*ast.TableRef{
						{
							Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "orders"}, Alias: "o"},
							Joins: []*ast.JoinClause{
								{
									Type:  ast.JoinInner,
									Right: &ast.TablePrimary{Name: &ast.Identifier{Name: "customers"}, Alias: "c"},
									On: &ast.ComparisonPredicate{
										Left:  &ast.Identifier{Qualifier: "o", Name: "customer_id"},
										Op:    utils.TOKEN_EQ,
										Right: &ast.Identifier{Qualifier: "c", Name: "id"},
									},
								},
							},
						},
					},
				},
			},
		})
	})

	t.Run("LEFT OUTER JOIN ON", func(t *testing.T) {
		requireAST(t, "SELECT a.id FROM a LEFT OUTER JOIN b ON a.id = b.id;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					Columns: []*ast.SelectColumn{
						{Expr: &ast.SelectExpression{Expr: &ast.Identifier{Qualifier: "a", Name: "id"}}},
					},
					From: []*ast.TableRef{
						{
							Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "a"}},
							Joins: []*ast.JoinClause{
								{
									Type:  ast.JoinLeft,
									Right: &ast.TablePrimary{Name: &ast.Identifier{Name: "b"}},
									On: &ast.ComparisonPredicate{
										Left:  &ast.Identifier{Qualifier: "a", Name: "id"},
										Op:    utils.TOKEN_EQ,
										Right: &ast.Identifier{Qualifier: "b", Name: "id"},
									},
								},
							},
						},
					},
				},
			},
		})
	})

	t.Run("CROSS JOIN produces no ON clause", func(t *testing.T) {
		requireAST(t, "SELECT * FROM a CROSS JOIN b;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					Columns: []*ast.SelectColumn{{Star: true}},
					From: []*ast.TableRef{
						{
							Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "a"}},
							Joins: []*ast.JoinClause{
								{Type: ast.JoinCross, Right: &ast.TablePrimary{Name: &ast.Identifier{Name: "b"}}},
							},
						},
					},
				},
			},
		})
	})

	t.Run("JOIN missing ON clause fails at the next clause keyword", func(t *testing.T) {
		requireParseError(t, "SELECT * FROM a JOIN b WHERE a.id = b.id;", CodeUnexpectedToken, 1, 24)
	})
}

// TestGrammar_SelectPipeline validates the full SELECT pipeline:
// WHERE with complex conditions, GROUP BY, HAVING with aggregate,
// ORDER BY DESC, and LIMIT with OFFSET.
func TestGrammar_SelectPipeline(t *testing.T) {
	t.Run("WHERE with BETWEEN AND IS NOT NULL AND NOT chain", func(t *testing.T) {
		requireAST(t,
			"SELECT id FROM users WHERE age BETWEEN 18 AND 65 AND email IS NOT NULL AND NOT deleted;",
			&ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{
							{Expr: &ast.SelectExpression{Expr: &ast.Identifier{Name: "id"}}},
						},
						From: []*ast.TableRef{{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "users"}}}},
						Where: &ast.WhereClause{
							Cond: &ast.BinaryCondition{
								Left: &ast.BinaryCondition{
									Left: &ast.BetweenPredicate{
										Expr: &ast.Identifier{Name: "age"},
										Low:  &ast.IntegerLiteral{Value: "18"},
										High: &ast.IntegerLiteral{Value: "65"},
									},
									Op:    utils.TOKEN_AND,
									Right: &ast.IsNullPredicate{Expr: &ast.Identifier{Name: "email"}, Negated: true},
								},
								Op: utils.TOKEN_AND,
								Right: &ast.NotCondition{
									Operand: &ast.ExprCondition{Expr: &ast.Identifier{Name: "deleted"}},
								},
							},
						},
					},
				},
			})
	})

	t.Run("GROUP BY + HAVING aggregate + ORDER BY DESC + LIMIT OFFSET", func(t *testing.T) {
		requireAST(t,
			"SELECT dept, COUNT(*) AS cnt FROM employees GROUP BY dept HAVING COUNT(*) > 5 ORDER BY cnt DESC LIMIT 10 OFFSET 20;",
			&ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{
							{Expr: &ast.SelectExpression{Expr: &ast.Identifier{Name: "dept"}}},
							{Expr: &ast.SelectExpression{Expr: &ast.FunctionCall{Name: "COUNT", Star: true}}, Alias: "cnt"},
						},
						From:    []*ast.TableRef{{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "employees"}}}},
						GroupBy: &ast.GroupByClause{Columns: []*ast.Identifier{{Name: "dept"}}},
						Having: &ast.HavingClause{
							Cond: &ast.ComparisonPredicate{
								Left:  &ast.FunctionCall{Name: "COUNT", Star: true},
								Op:    utils.TOKEN_GT,
								Right: &ast.IntegerLiteral{Value: "5"},
							},
						},
						OrderBy: &ast.OrderByClause{
							Items: []*ast.OrderByItem{
								{Expr: &ast.Identifier{Name: "cnt"}, Direction: ast.OrderDesc},
							},
						},
						Limit: &ast.LimitClause{Count: 10, Offset: ptr(20)},
					},
				},
			})
	})
}

// TestGrammar_SelectPipelineErrors validates that out-of-order or malformed
// SELECT pipeline clauses are rejected correctly.
func TestGrammar_SelectPipelineErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
		code  error
		line  int
		col   int
	}{
		{"LIMIT before WHERE is rejected at WHERE keyword", "SELECT * FROM t LIMIT 10 WHERE id = 1;", CodeUnexpectedToken, 1, 26},
		{"GROUP BY missing BY fails at non-BY token", "SELECT * FROM t GROUP t;", CodeUnexpectedToken, 1, 23},
		{"ORDER BY missing BY fails at non-BY token", "SELECT * FROM t ORDER t;", CodeUnexpectedToken, 1, 23},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireParseError(t, tt.input, tt.code, tt.line, tt.col)
		})
	}
}

// TestGrammar_Update validates UPDATE statements with single and multiple SET
// items, SET expressions, and a compound WHERE condition.
func TestGrammar_Update(t *testing.T) {
	t.Run("multiple SET items including expression, with compound WHERE", func(t *testing.T) {
		requireAST(t,
			"UPDATE orders SET status = 'shipped', qty = qty - 1 WHERE id = 42 AND active = TRUE;",
			&ast.Program{
				Statements: []ast.Statement{
					&ast.UpdateStmt{
						Table: &ast.Identifier{Name: "orders"},
						Set: []*ast.SetItem{
							{Column: &ast.Identifier{Name: "status"}, Value: &ast.StringLiteral{Value: "shipped"}},
							{Column: &ast.Identifier{Name: "qty"}, Value: &ast.BinaryExpr{
								Left:  &ast.Identifier{Name: "qty"},
								Op:    utils.TOKEN_MINUS,
								Right: &ast.IntegerLiteral{Value: "1"},
							}},
						},
						Where: &ast.WhereClause{
							Cond: &ast.BinaryCondition{
								Left: &ast.ComparisonPredicate{
									Left: &ast.Identifier{Name: "id"}, Op: utils.TOKEN_EQ, Right: &ast.IntegerLiteral{Value: "42"},
								},
								Op: utils.TOKEN_AND,
								Right: &ast.ComparisonPredicate{
									Left: &ast.Identifier{Name: "active"}, Op: utils.TOKEN_EQ, Right: &ast.BooleanLiteral{Value: "TRUE"},
								},
							},
						},
					},
				},
			})
	})
}

// TestGrammar_UpdateErrors validates UPDATE syntax errors.
func TestGrammar_UpdateErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
		code  error
		line  int
		col   int
	}{
		{"missing SET keyword", "UPDATE t a = 1;", CodeUnexpectedToken, 1, 10},
		{"non-identifier column in SET", "UPDATE t SET 1 = 1;", CodeUnexpectedToken, 1, 14},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireParseError(t, tt.input, tt.code, tt.line, tt.col)
		})
	}
}

// TestGrammar_Insert validates INSERT statements with column list, multiple
// value rows, NULL/boolean values, and INSERT ... SELECT.
func TestGrammar_Insert(t *testing.T) {
	t.Run("INSERT with column list and multiple value rows containing NULL and booleans", func(t *testing.T) {
		requireAST(t,
			"INSERT INTO t (id, name, active) VALUES (1, 'alice', TRUE), (2, NULL, FALSE);",
			&ast.Program{
				Statements: []ast.Statement{
					&ast.InsertStmt{
						Table:   &ast.Identifier{Name: "t"},
						Columns: []string{"id", "name", "active"},
						Rows: [][]*ast.SelectExpression{
							{
								{Expr: &ast.IntegerLiteral{Value: "1"}},
								{Expr: &ast.StringLiteral{Value: "alice"}},
								{Expr: &ast.BooleanLiteral{Value: "TRUE"}},
							},
							{
								{Expr: &ast.IntegerLiteral{Value: "2"}},
								{Expr: &ast.NullLiteral{}},
								{Expr: &ast.BooleanLiteral{Value: "FALSE"}},
							},
						},
					},
				},
			})
	})
}

// TestGrammar_InsertErrors validates INSERT syntax errors.
func TestGrammar_InsertErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
		code  error
		line  int
		col   int
	}{
		{"missing INTO keyword", "INSERT t VALUES (1);", CodeUnexpectedToken, 1, 8},
		{"missing closing paren in VALUES", "INSERT INTO t VALUES (1, 2;", CodeUnexpectedToken, 1, 27},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireParseError(t, tt.input, tt.code, tt.line, tt.col)
		})
	}
}

// TestGrammar_Delete validates DELETE errors (success cases are tested implicitly
// through condQuery helpers across the other test functions).
func TestGrammar_Delete(t *testing.T) {
	t.Run("missing FROM keyword fails at the table name token", func(t *testing.T) {
		requireParseError(t, "DELETE t;", CodeUnexpectedToken, 1, 8)
	})
}

// TestGrammar_CreateTable validates CREATE TABLE with every data type,
// every column constraint kind, and the IF NOT EXISTS guard.
func TestGrammar_CreateTable(t *testing.T) {
	t.Run("all data types and all column constraint kinds", func(t *testing.T) {
		requireAST(t,
			`CREATE TABLE orders (
				id          INT PRIMARY KEY NOT NULL,
				ref         BIGINT UNIQUE,
				label       VARCHAR(64) DEFAULT 'pending',
				qty         DECIMAL(10, 2) DEFAULT -1.5,
				flag        BOOLEAN DEFAULT TRUE,
				created_at  TIMESTAMP,
				score       FLOAT,
				ratio       DOUBLE,
				note        TEXT NULL,
				customer_id INT REFERENCES customers(id)
			);`,
			&ast.Program{
				Statements: []ast.Statement{
					&ast.CreateTableStmt{
						Table: &ast.Identifier{Name: "orders"},
						Columns: []*ast.ColumnDef{
							{Name: "id", Type: &ast.DataType{Kind: ast.TypeInt}, Constraints: []ast.Clause{
								&ast.PrimaryKeyConstraint{}, &ast.NotNullConstraint{},
							}},
							{Name: "ref", Type: &ast.DataType{Kind: ast.TypeBigInt}, Constraints: []ast.Clause{
								&ast.UniqueConstraint{},
							}},
							{Name: "label", Type: &ast.DataType{Kind: ast.TypeVarchar, VarcharLen: ptr(64)}, Constraints: []ast.Clause{
								&ast.DefaultConstraint{Value: &ast.SignedLiteral{Value: &ast.StringLiteral{Value: "pending"}}},
							}},
							{Name: "qty", Type: &ast.DataType{Kind: ast.TypeDecimal, DecimalPrec: ptr(10), DecimalScale: ptr(2)}, Constraints: []ast.Clause{
								&ast.DefaultConstraint{Value: &ast.SignedLiteral{Negative: true, Value: &ast.FloatLiteral{Value: "1.5"}}},
							}},
							{Name: "flag", Type: &ast.DataType{Kind: ast.TypeBoolean}, Constraints: []ast.Clause{
								&ast.DefaultConstraint{Value: &ast.SignedLiteral{Value: &ast.BooleanLiteral{Value: "TRUE"}}},
							}},
							{Name: "created_at", Type: &ast.DataType{Kind: ast.TypeTimestamp}},
							{Name: "score", Type: &ast.DataType{Kind: ast.TypeFloat}},
							{Name: "ratio", Type: &ast.DataType{Kind: ast.TypeDouble}},
							{Name: "note", Type: &ast.DataType{Kind: ast.TypeText}, Constraints: []ast.Clause{
								&ast.NullConstraint{},
							}},
							{Name: "customer_id", Type: &ast.DataType{Kind: ast.TypeInt}, Constraints: []ast.Clause{
								&ast.ForeignRef{Table: "customers", Column: "id"},
							}},
						},
					},
				},
			})
	})
}

// TestGrammar_DDLErrors validates errors in DDL statements: CREATE TABLE,
// ALTER TABLE, DROP TABLE, and unrecognised statement keywords.
func TestGrammar_DDLErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
		code  error
		line  int
		col   int
	}{
		{"CREATE TABLE with unknown column type", "CREATE TABLE t (id NCHAR);", CodeInvalidDataType, 1, 20},
		{"CREATE TABLE NOT followed by non-NULL", "CREATE TABLE t (id INT NOT TRUE);", CodeUnexpectedToken, 1, 28},
		{"DEFAULT signed string is invalid (only numeric types can be signed)", "CREATE TABLE t (v TEXT DEFAULT +'nope');", CodeExpectedExpression, 1, 33},
		{"REFERENCES missing table name", "CREATE TABLE t (id INT REFERENCES);", CodeUnexpectedToken, 1, 34},
		{"ALTER TABLE with unknown action keyword", "ALTER TABLE t;", CodeInvalidAlterAction, 1, 14},
		{"ALTER TABLE RENAME missing TO", "ALTER TABLE t RENAME new_t;", CodeInvalidAlterAction, 1, 22},
		{"ALTER TABLE DROP missing COLUMN keyword", "ALTER TABLE t DROP c;", CodeUnexpectedToken, 1, 20},
		{"DROP TABLE missing table name", "DROP TABLE;", CodeUnexpectedToken, 1, 11},
		{"completely unknown keyword at statement start", "BOGUS;", CodeMalformedStatement, 1, 1},
		{"CREATE followed by unrecognised object type", "CREATE INDEX foo;", CodeMalformedStatement, 1, 8},
		{"DROP followed by unrecognised object type", "DROP INDEX foo;", CodeMalformedStatement, 1, 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireParseError(t, tt.input, tt.code, tt.line, tt.col)
		})
	}
}

// TestGrammar_DatabaseDDL validates CREATE DATABASE, DROP DATABASE, and USE
// in all their variants (plain, IF NOT EXISTS, IF EXISTS), including syntax errors.
func TestGrammar_DatabaseDDL(t *testing.T) {
	t.Run("CREATE DATABASE simple", func(t *testing.T) {
		requireAST(t, "CREATE DATABASE mydb;", &ast.Program{
			Statements: []ast.Statement{
				&ast.CreateDatabaseStmt{Name: "mydb", IfNotExists: false},
			},
		})
	})

	t.Run("CREATE DATABASE IF NOT EXISTS", func(t *testing.T) {
		requireAST(t, "CREATE DATABASE IF NOT EXISTS mydb;", &ast.Program{
			Statements: []ast.Statement{
				&ast.CreateDatabaseStmt{Name: "mydb", IfNotExists: true},
			},
		})
	})

	t.Run("USE database", func(t *testing.T) {
		requireAST(t, "USE mydb;", &ast.Program{
			Statements: []ast.Statement{
				&ast.UseDatabaseStmt{Name: "mydb"},
			},
		})
	})

	t.Run("DROP DATABASE simple", func(t *testing.T) {
		requireAST(t, "DROP DATABASE mydb;", &ast.Program{
			Statements: []ast.Statement{
				&ast.DropDatabaseStmt{Name: "mydb", IfExists: false},
			},
		})
	})

	t.Run("DROP DATABASE IF EXISTS", func(t *testing.T) {
		requireAST(t, "DROP DATABASE IF EXISTS mydb;", &ast.Program{
			Statements: []ast.Statement{
				&ast.DropDatabaseStmt{Name: "mydb", IfExists: true},
			},
		})
	})

	t.Run("CREATE DATABASE missing name fails at semicolon", func(t *testing.T) {
		requireParseError(t, "CREATE DATABASE;", CodeUnexpectedToken, 1, 16)
	})

	t.Run("CREATE DATABASE IF EXISTS is invalid (wrong guard)", func(t *testing.T) {
		requireParseError(t, "CREATE DATABASE IF EXISTS mydb;", CodeUnexpectedToken, 1, 20)
	})
}

// TestGrammar_TableDDLGuards validates IF NOT EXISTS, IF EXISTS guards and
// qualified names on CREATE TABLE and DROP TABLE, plus DECIMAL precision and VARCHAR scale error cases.
func TestGrammar_TableDDLGuards(t *testing.T) {
	t.Run("CREATE TABLE IF NOT EXISTS with qualified name", func(t *testing.T) {
		requireAST(t, "CREATE TABLE IF NOT EXISTS mydb.orders (id INT);", &ast.Program{
			Statements: []ast.Statement{
				&ast.CreateTableStmt{
					Table:       &ast.Identifier{Qualifier: "mydb", Name: "orders"},
					IfNotExists: true,
					Columns: []*ast.ColumnDef{
						{Name: "id", Type: &ast.DataType{Kind: ast.TypeInt}},
					},
				},
			},
		})
	})

	t.Run("DROP TABLE IF EXISTS qualified name", func(t *testing.T) {
		requireAST(t, "DROP TABLE IF EXISTS mydb.orders;", &ast.Program{
			Statements: []ast.Statement{
				&ast.DropTableStmt{
					Table:    &ast.Identifier{Qualifier: "mydb", Name: "orders"},
					IfExists: true,
				},
			},
		})
	})

	t.Run("DECIMAL with only precision (no scale)", func(t *testing.T) {
		requireAST(t, "CREATE TABLE t (v DECIMAL(10));", &ast.Program{
			Statements: []ast.Statement{
				&ast.CreateTableStmt{
					Table: &ast.Identifier{Name: "t"},
					Columns: []*ast.ColumnDef{
						{Name: "v", Type: &ast.DataType{Kind: ast.TypeDecimal, DecimalPrec: ptr(10)}},
					},
				},
			},
		})
	})

	t.Run("DECIMAL with no precision parens", func(t *testing.T) {
		requireAST(t, "CREATE TABLE t (v DECIMAL);", &ast.Program{
			Statements: []ast.Statement{
				&ast.CreateTableStmt{
					Table: &ast.Identifier{Name: "t"},
					Columns: []*ast.ColumnDef{
						{Name: "v", Type: &ast.DataType{Kind: ast.TypeDecimal}},
					},
				},
			},
		})
	})

	t.Run("VARCHAR missing length paren fails at the token after VARCHAR", func(t *testing.T) {
		requireParseError(t, "CREATE TABLE t (v VARCHAR);", CodeUnexpectedToken, 1, 26)
	})

	t.Run("VARCHAR missing lparen fails at the numeric literal", func(t *testing.T) {
		requireParseError(t, "CREATE TABLE t (v VARCHAR 10);", CodeUnexpectedToken, 1, 27)
	})

	t.Run("DECIMAL missing rparen fails at semicolon", func(t *testing.T) {
		requireParseError(t, "CREATE TABLE t (v DECIMAL(10);", CodeUnexpectedToken, 1, 30)
	})

	t.Run("DECIMAL missing scale rparen fails at semicolon", func(t *testing.T) {
		requireParseError(t, "CREATE TABLE t (v DECIMAL(10, 2);", CodeUnexpectedToken, 1, 33)
	})
}

// TestGrammar_AlterTable validates all five ALTER TABLE action variants,
// both with and without the optional COLUMN keyword where applicable, plus qualified table names.
func TestGrammar_AlterTable(t *testing.T) {
	t.Run("ADD COLUMN with COLUMN keyword", func(t *testing.T) {
		requireAST(t, "ALTER TABLE t ADD COLUMN age INT;", &ast.Program{
			Statements: []ast.Statement{
				&ast.AlterTableStmt{
					Table: &ast.Identifier{Name: "t"},
					Action: &ast.AlterAction{Kind: ast.AlterAdd,
						Column: &ast.ColumnDef{Name: "age", Type: &ast.DataType{Kind: ast.TypeInt}},
					},
				},
			},
		})
	})

	t.Run("ADD without COLUMN keyword", func(t *testing.T) {
		requireAST(t, "ALTER TABLE t ADD age INT;", &ast.Program{
			Statements: []ast.Statement{
				&ast.AlterTableStmt{
					Table: &ast.Identifier{Name: "t"},
					Action: &ast.AlterAction{Kind: ast.AlterAdd,
						Column: &ast.ColumnDef{Name: "age", Type: &ast.DataType{Kind: ast.TypeInt}},
					},
				},
			},
		})
	})

	t.Run("MODIFY COLUMN with COLUMN keyword", func(t *testing.T) {
		requireAST(t, "ALTER TABLE t MODIFY COLUMN name VARCHAR(100);", &ast.Program{
			Statements: []ast.Statement{
				&ast.AlterTableStmt{
					Table: &ast.Identifier{Name: "t"},
					Action: &ast.AlterAction{Kind: ast.AlterModify,
						Column: &ast.ColumnDef{Name: "name", Type: &ast.DataType{Kind: ast.TypeVarchar, VarcharLen: ptr(100)}},
					},
				},
			},
		})
	})

	t.Run("MODIFY without COLUMN keyword", func(t *testing.T) {
		requireAST(t, "ALTER TABLE t MODIFY name VARCHAR(50);", &ast.Program{
			Statements: []ast.Statement{
				&ast.AlterTableStmt{
					Table: &ast.Identifier{Name: "t"},
					Action: &ast.AlterAction{Kind: ast.AlterModify,
						Column: &ast.ColumnDef{Name: "name", Type: &ast.DataType{Kind: ast.TypeVarchar, VarcharLen: ptr(50)}},
					},
				},
			},
		})
	})

	t.Run("RENAME TO (table rename)", func(t *testing.T) {
		requireAST(t, "ALTER TABLE t RENAME TO new_t;", &ast.Program{
			Statements: []ast.Statement{
				&ast.AlterTableStmt{
					Table:  &ast.Identifier{Name: "t"},
					Action: &ast.AlterAction{Kind: ast.AlterRenameTable, NewName: "new_t"},
				},
			},
		})
	})

	t.Run("RENAME COLUMN old TO new", func(t *testing.T) {
		requireAST(t, "ALTER TABLE t RENAME COLUMN old_col TO new_col;", &ast.Program{
			Statements: []ast.Statement{
				&ast.AlterTableStmt{
					Table:  &ast.Identifier{Name: "t"},
					Action: &ast.AlterAction{Kind: ast.AlterRenameColumn, OldName: "old_col", NewName: "new_col"},
				},
			},
		})
	})

	t.Run("DROP COLUMN", func(t *testing.T) {
		requireAST(t, "ALTER TABLE t DROP COLUMN c;", &ast.Program{
			Statements: []ast.Statement{
				&ast.AlterTableStmt{
					Table:  &ast.Identifier{Name: "t"},
					Action: &ast.AlterAction{Kind: ast.AlterDropColumn, DropName: "c"},
				},
			},
		})
	})

	t.Run("ALTER with qualified table name", func(t *testing.T) {
		requireAST(t, "ALTER TABLE mydb.t ADD val TEXT;", &ast.Program{
			Statements: []ast.Statement{
				&ast.AlterTableStmt{
					Table: &ast.Identifier{Qualifier: "mydb", Name: "t"},
					Action: &ast.AlterAction{Kind: ast.AlterAdd,
						Column: &ast.ColumnDef{Name: "val", Type: &ast.DataType{Kind: ast.TypeText}},
					},
				},
			},
		})
	})
}

// TestGrammar_SelectModifiers validates SELECT DISTINCT, SELECT ALL, and bare
// SELECT (no FROM clause).
func TestGrammar_SelectModifiers(t *testing.T) {
	t.Run("SELECT DISTINCT produces Distinct=true", func(t *testing.T) {
		requireAST(t, "SELECT DISTINCT id FROM t;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					Distinct: true,
					Columns: []*ast.SelectColumn{
						{Expr: &ast.SelectExpression{Expr: &ast.Identifier{Name: "id"}}},
					},
					From: []*ast.TableRef{{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}}},
				},
			},
		})
	})

	t.Run("SELECT ALL produces All=true", func(t *testing.T) {
		requireAST(t, "SELECT ALL id FROM t;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					All: true,
					Columns: []*ast.SelectColumn{
						{Expr: &ast.SelectExpression{Expr: &ast.Identifier{Name: "id"}}},
					},
					From: []*ast.TableRef{{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}}},
				},
			},
		})
	})

	t.Run("bare SELECT with no FROM clause", func(t *testing.T) {
		requireAST(t, "SELECT 42;", exprQuery(&ast.IntegerLiteral{Value: "42"}))
	})
}

// TestGrammar_FromClauseVariants validates comma-separated (implicit cross-join)
// FROM lists, implicit column aliases (no AS), two-level qualified wildcard, and
// explicit AS table alias.
func TestGrammar_FromClauseVariants(t *testing.T) {
	t.Run("multi-table FROM (implicit cross-join via comma)", func(t *testing.T) {
		requireAST(t, "SELECT * FROM a, b;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					Columns: []*ast.SelectColumn{{Star: true}},
					From: []*ast.TableRef{
						{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "a"}}},
						{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "b"}}},
					},
				},
			},
		})
	})

	t.Run("implicit alias on column expression (no AS keyword)", func(t *testing.T) {
		requireAST(t, "SELECT id myid FROM t;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					Columns: []*ast.SelectColumn{
						{Expr: &ast.SelectExpression{Expr: &ast.Identifier{Name: "id"}}, Alias: "myid"},
					},
					From: []*ast.TableRef{{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}}},
				},
			},
		})
	})

	t.Run("two-level qualified wildcard: db.table.*", func(t *testing.T) {
		requireAST(t, "SELECT mydb.orders.* FROM mydb.orders;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					Columns: []*ast.SelectColumn{
						{QualifiedStar: &ast.Identifier{Qualifier: "mydb", Name: "orders"}},
					},
					From: []*ast.TableRef{
						{Primary: &ast.TablePrimary{Name: &ast.Identifier{Qualifier: "mydb", Name: "orders"}}},
					},
				},
			},
		})
	})

	t.Run("explicit AS alias on table reference", func(t *testing.T) {
		requireAST(t, "SELECT a.id FROM orders AS a;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					Columns: []*ast.SelectColumn{
						{Expr: &ast.SelectExpression{Expr: &ast.Identifier{Qualifier: "a", Name: "id"}}},
					},
					From: []*ast.TableRef{
						{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "orders"}, Alias: "a"}},
					},
				},
			},
		})
	})

	t.Run("IDENT DOT unexpected token in SELECT list fails at that token", func(t *testing.T) {
		requireParseError(t, "SELECT t.123 FROM t;", CodeUnexpectedToken, 1, 9)
	})
}

// TestGrammar_JoinVariants covers the remaining JOIN types:
// bare JOIN (INNER), LEFT without OUTER, RIGHT OUTER JOIN, FULL OUTER JOIN,
// and parenthesised table references with joins.
func TestGrammar_JoinVariants(t *testing.T) {
	t.Run("bare JOIN treated as INNER JOIN", func(t *testing.T) {
		requireAST(t, "SELECT * FROM a JOIN b ON a.id = b.id;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					Columns: []*ast.SelectColumn{{Star: true}},
					From: []*ast.TableRef{
						{
							Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "a"}},
							Joins: []*ast.JoinClause{
								{
									Type:  ast.JoinInner,
									Right: &ast.TablePrimary{Name: &ast.Identifier{Name: "b"}},
									On: &ast.ComparisonPredicate{
										Left: &ast.Identifier{Qualifier: "a", Name: "id"}, Op: utils.TOKEN_EQ,
										Right: &ast.Identifier{Qualifier: "b", Name: "id"},
									},
								},
							},
						},
					},
				},
			},
		})
	})

	t.Run("LEFT JOIN without OUTER keyword", func(t *testing.T) {
		requireAST(t, "SELECT * FROM a LEFT JOIN b ON a.id = b.id;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					Columns: []*ast.SelectColumn{{Star: true}},
					From: []*ast.TableRef{
						{
							Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "a"}},
							Joins: []*ast.JoinClause{
								{
									Type:  ast.JoinLeft,
									Right: &ast.TablePrimary{Name: &ast.Identifier{Name: "b"}},
									On: &ast.ComparisonPredicate{
										Left: &ast.Identifier{Qualifier: "a", Name: "id"}, Op: utils.TOKEN_EQ,
										Right: &ast.Identifier{Qualifier: "b", Name: "id"},
									},
								},
							},
						},
					},
				},
			},
		})
	})

	t.Run("RIGHT OUTER JOIN ON", func(t *testing.T) {
		requireAST(t, "SELECT * FROM a RIGHT OUTER JOIN b ON a.id = b.id;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					Columns: []*ast.SelectColumn{{Star: true}},
					From: []*ast.TableRef{
						{
							Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "a"}},
							Joins: []*ast.JoinClause{
								{
									Type:  ast.JoinRight,
									Right: &ast.TablePrimary{Name: &ast.Identifier{Name: "b"}},
									On: &ast.ComparisonPredicate{
										Left: &ast.Identifier{Qualifier: "a", Name: "id"}, Op: utils.TOKEN_EQ,
										Right: &ast.Identifier{Qualifier: "b", Name: "id"},
									},
								},
							},
						},
					},
				},
			},
		})
	})

	t.Run("FULL OUTER JOIN ON", func(t *testing.T) {
		requireAST(t, "SELECT * FROM a FULL OUTER JOIN b ON a.id = b.id;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					Columns: []*ast.SelectColumn{{Star: true}},
					From: []*ast.TableRef{
						{
							Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "a"}},
							Joins: []*ast.JoinClause{
								{
									Type:  ast.JoinFull,
									Right: &ast.TablePrimary{Name: &ast.Identifier{Name: "b"}},
									On: &ast.ComparisonPredicate{
										Left: &ast.Identifier{Qualifier: "a", Name: "id"}, Op: utils.TOKEN_EQ,
										Right: &ast.Identifier{Qualifier: "b", Name: "id"},
									},
								},
							},
						},
					},
				},
			},
		})
	})

	t.Run("parenthesised table reference with JOIN", func(t *testing.T) {
		requireAST(t, "SELECT * FROM (a) JOIN b ON a.id = b.id;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					Columns: []*ast.SelectColumn{{Star: true}},
					From: []*ast.TableRef{
						{
							Paren: &ast.TableRef{
								Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "a"}},
							},
							Joins: []*ast.JoinClause{
								{
									Type:  ast.JoinInner,
									Right: &ast.TablePrimary{Name: &ast.Identifier{Name: "b"}},
									On: &ast.ComparisonPredicate{
										Left:  &ast.Identifier{Qualifier: "a", Name: "id"},
										Op:    utils.TOKEN_EQ,
										Right: &ast.Identifier{Qualifier: "b", Name: "id"},
									},
								},
							},
						},
					},
				},
			},
		})
	})

	t.Run("nested parenthesised JOIN reference with JOIN", func(t *testing.T) {
		requireAST(t, "SELECT * FROM (a JOIN b ON a.id = b.id) JOIN c ON a.id = c.id;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					Columns: []*ast.SelectColumn{{Star: true}},
					From: []*ast.TableRef{
						{
							Paren: &ast.TableRef{
								Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "a"}},
								Joins: []*ast.JoinClause{
									{
										Type:  ast.JoinInner,
										Right: &ast.TablePrimary{Name: &ast.Identifier{Name: "b"}},
										On: &ast.ComparisonPredicate{
											Left:  &ast.Identifier{Qualifier: "a", Name: "id"},
											Op:    utils.TOKEN_EQ,
											Right: &ast.Identifier{Qualifier: "b", Name: "id"},
										},
									},
								},
							},
							Joins: []*ast.JoinClause{
								{
									Type:  ast.JoinInner,
									Right: &ast.TablePrimary{Name: &ast.Identifier{Name: "c"}},
									On: &ast.ComparisonPredicate{
										Left:  &ast.Identifier{Qualifier: "a", Name: "id"},
										Op:    utils.TOKEN_EQ,
										Right: &ast.Identifier{Qualifier: "c", Name: "id"},
									},
								},
							},
						},
					},
				},
			},
		})
	})
}

// TestGrammar_OrderGroupLimit validates explicit ASC, multiple ORDER BY items,
// positional ORDER BY, expression in ORDER BY, LIMIT without OFFSET, and
// multi-column GROUP BY.
func TestGrammar_OrderGroupLimit(t *testing.T) {
	t.Run("ORDER BY explicit ASC direction", func(t *testing.T) {
		requireAST(t, "SELECT * FROM t ORDER BY id ASC;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					Columns: []*ast.SelectColumn{{Star: true}},
					From:    []*ast.TableRef{{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}}},
					OrderBy: &ast.OrderByClause{
						Items: []*ast.OrderByItem{
							{Expr: &ast.Identifier{Name: "id"}, Direction: ast.OrderAsc},
						},
					},
				},
			},
		})
	})

	t.Run("ORDER BY multiple comma-separated items: dept ASC, salary DESC", func(t *testing.T) {
		requireAST(t, "SELECT * FROM t ORDER BY dept ASC, salary DESC;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					Columns: []*ast.SelectColumn{{Star: true}},
					From:    []*ast.TableRef{{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}}},
					OrderBy: &ast.OrderByClause{
						Items: []*ast.OrderByItem{
							{Expr: &ast.Identifier{Name: "dept"}, Direction: ast.OrderAsc},
							{Expr: &ast.Identifier{Name: "salary"}, Direction: ast.OrderDesc},
						},
					},
				},
			},
		})
	})

	t.Run("ORDER BY positional integer literal", func(t *testing.T) {
		requireAST(t, "SELECT id, name FROM t ORDER BY 2;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					Columns: []*ast.SelectColumn{
						{Expr: &ast.SelectExpression{Expr: &ast.Identifier{Name: "id"}}},
						{Expr: &ast.SelectExpression{Expr: &ast.Identifier{Name: "name"}}},
					},
					From: []*ast.TableRef{{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}}},
					OrderBy: &ast.OrderByClause{
						Items: []*ast.OrderByItem{{Expr: &ast.IntegerLiteral{Value: "2"}}},
					},
				},
			},
		})
	})

	t.Run("ORDER BY arithmetic expression: a + b DESC", func(t *testing.T) {
		requireAST(t, "SELECT * FROM t ORDER BY a + b DESC;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					Columns: []*ast.SelectColumn{{Star: true}},
					From:    []*ast.TableRef{{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}}},
					OrderBy: &ast.OrderByClause{
						Items: []*ast.OrderByItem{
							{Expr: &ast.BinaryExpr{
								Left: &ast.Identifier{Name: "a"}, Op: utils.TOKEN_PLUS,
								Right: &ast.Identifier{Name: "b"},
							}, Direction: ast.OrderDesc},
						},
					},
				},
			},
		})
	})

	t.Run("LIMIT without OFFSET", func(t *testing.T) {
		requireAST(t, "SELECT * FROM t LIMIT 5;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					Columns: []*ast.SelectColumn{{Star: true}},
					From:    []*ast.TableRef{{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}}},
					Limit:   &ast.LimitClause{Count: 5},
				},
			},
		})
	})

	t.Run("GROUP BY with multiple comma-separated columns", func(t *testing.T) {
		requireAST(t, "SELECT * FROM t GROUP BY dept, region;", &ast.Program{
			Statements: []ast.Statement{
				&ast.SelectStmt{
					Columns: []*ast.SelectColumn{{Star: true}},
					From:    []*ast.TableRef{{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}}},
					GroupBy: &ast.GroupByClause{
						Columns: []*ast.Identifier{{Name: "dept"}, {Name: "region"}},
					},
				},
			},
		})
	})
}

// TestGrammar_InsertSelectAndQualified validates INSERT … SELECT (both with
// and without a column list) and DML against qualified table names, including INSERT syntax errors.
func TestGrammar_InsertSelectAndQualified(t *testing.T) {
	t.Run("INSERT INTO t SELECT * FROM other (no column list)", func(t *testing.T) {
		requireAST(t, "INSERT INTO t SELECT * FROM other;", &ast.Program{
			Statements: []ast.Statement{
				&ast.InsertStmt{
					Table: &ast.Identifier{Name: "t"},
					Source: &ast.SelectStmt{
						Columns: []*ast.SelectColumn{{Star: true}},
						From:    []*ast.TableRef{{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "other"}}}},
					},
				},
			},
		})
	})

	t.Run("INSERT INTO t (id) SELECT id FROM other (with column list)", func(t *testing.T) {
		requireAST(t, "INSERT INTO t (id) SELECT id FROM other;", &ast.Program{
			Statements: []ast.Statement{
				&ast.InsertStmt{
					Table:   &ast.Identifier{Name: "t"},
					Columns: []string{"id"},
					Source: &ast.SelectStmt{
						Columns: []*ast.SelectColumn{
							{Expr: &ast.SelectExpression{Expr: &ast.Identifier{Name: "id"}}},
						},
						From: []*ast.TableRef{{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "other"}}}},
					},
				},
			},
		})
	})

	t.Run("DELETE FROM qualified table with WHERE", func(t *testing.T) {
		requireAST(t, "DELETE FROM mydb.orders WHERE id = 1;", &ast.Program{
			Statements: []ast.Statement{
				&ast.DeleteStmt{
					Table: &ast.Identifier{Qualifier: "mydb", Name: "orders"},
					Where: &ast.WhereClause{
						Cond: &ast.ComparisonPredicate{
							Left: &ast.Identifier{Name: "id"}, Op: utils.TOKEN_EQ,
							Right: &ast.IntegerLiteral{Value: "1"},
						},
					},
				},
			},
		})
	})

	t.Run("DELETE FROM table with no WHERE clause", func(t *testing.T) {
		requireAST(t, "DELETE FROM t;", &ast.Program{
			Statements: []ast.Statement{
				&ast.DeleteStmt{Table: &ast.Identifier{Name: "t"}},
			},
		})
	})

	t.Run("UPDATE with qualified table and no WHERE", func(t *testing.T) {
		requireAST(t, "UPDATE mydb.t SET val = 0;", &ast.Program{
			Statements: []ast.Statement{
				&ast.UpdateStmt{
					Table: &ast.Identifier{Qualifier: "mydb", Name: "t"},
					Set: []*ast.SetItem{
						{Column: &ast.Identifier{Name: "val"}, Value: &ast.IntegerLiteral{Value: "0"}},
					},
				},
			},
		})
	})

	t.Run("INSERT followed by neither VALUES nor SELECT fails with CodeMalformedStatement", func(t *testing.T) {
		requireParseError(t, "INSERT INTO t UPDATE;", CodeMalformedStatement, 1, 15)
	})
}

// TestGrammar_ConditionEdgeCases covers paths in parseConditionPrimary that were
// not exercised before: bare ExprCondition, three-level AND/OR chains, BETWEEN
// and IN with expression values, and (paren-expr) with a predicate tail.
func TestGrammar_ConditionEdgeCases(t *testing.T) {
	t.Run("bare boolean column in WHERE is an ExprCondition", func(t *testing.T) {
		requireAST(t, "DELETE FROM t WHERE active;", condQuery(
			&ast.ExprCondition{Expr: &ast.Identifier{Name: "active"}},
		))
	})

	t.Run("(expr) alone in WHERE position becomes ExprCondition wrapping ParenExpr", func(t *testing.T) {
		requireAST(t, "DELETE FROM t WHERE (active);", condQuery(
			&ast.ExprCondition{Expr: &ast.ParenExpr{Inner: &ast.Identifier{Name: "active"}}},
		))
	})

	t.Run("three-level AND chain is left-associative: (a AND b) AND c", func(t *testing.T) {
		requireAST(t, "DELETE FROM t WHERE a AND b AND c;", condQuery(
			&ast.BinaryCondition{
				Left: &ast.BinaryCondition{
					Left:  &ast.ExprCondition{Expr: &ast.Identifier{Name: "a"}},
					Op:    utils.TOKEN_AND,
					Right: &ast.ExprCondition{Expr: &ast.Identifier{Name: "b"}},
				},
				Op:    utils.TOKEN_AND,
				Right: &ast.ExprCondition{Expr: &ast.Identifier{Name: "c"}},
			},
		))
	})

	t.Run("three-level OR chain is left-associative: (a OR b) OR c", func(t *testing.T) {
		requireAST(t, "DELETE FROM t WHERE a OR b OR c;", condQuery(
			&ast.BinaryCondition{
				Left: &ast.BinaryCondition{
					Left:  &ast.ExprCondition{Expr: &ast.Identifier{Name: "a"}},
					Op:    utils.TOKEN_OR,
					Right: &ast.ExprCondition{Expr: &ast.Identifier{Name: "b"}},
				},
				Op:    utils.TOKEN_OR,
				Right: &ast.ExprCondition{Expr: &ast.Identifier{Name: "c"}},
			},
		))
	})

	t.Run("BETWEEN with arithmetic bound expressions", func(t *testing.T) {
		requireAST(t, "DELETE FROM t WHERE price BETWEEN cost + 1 AND cost * 2;", condQuery(
			&ast.BetweenPredicate{
				Expr: &ast.Identifier{Name: "price"},
				Low: &ast.BinaryExpr{
					Left: &ast.Identifier{Name: "cost"}, Op: utils.TOKEN_PLUS,
					Right: &ast.IntegerLiteral{Value: "1"},
				},
				High: &ast.BinaryExpr{
					Left: &ast.Identifier{Name: "cost"}, Op: utils.TOKEN_STAR,
					Right: &ast.IntegerLiteral{Value: "2"},
				},
			},
		))
	})

	t.Run("IN with arithmetic expression values", func(t *testing.T) {
		requireAST(t, "DELETE FROM t WHERE id IN (a + 1, b - 2);", condQuery(
			&ast.InPredicate{
				Expr: &ast.Identifier{Name: "id"},
				Values: []ast.Expression{
					&ast.BinaryExpr{Left: &ast.Identifier{Name: "a"}, Op: utils.TOKEN_PLUS, Right: &ast.IntegerLiteral{Value: "1"}},
					&ast.BinaryExpr{Left: &ast.Identifier{Name: "b"}, Op: utils.TOKEN_MINUS, Right: &ast.IntegerLiteral{Value: "2"}},
				},
			},
		))
	})

	t.Run("(paren-expr) LHS with BETWEEN predicate tail: (a + 1) BETWEEN 5 AND 10", func(t *testing.T) {
		requireAST(t, "DELETE FROM t WHERE (a + 1) BETWEEN 5 AND 10;", condQuery(
			&ast.BetweenPredicate{
				Expr: &ast.ParenExpr{
					Inner: &ast.BinaryExpr{
						Left: &ast.Identifier{Name: "a"}, Op: utils.TOKEN_PLUS,
						Right: &ast.IntegerLiteral{Value: "1"},
					},
				},
				Low:  &ast.IntegerLiteral{Value: "5"},
				High: &ast.IntegerLiteral{Value: "10"},
			},
		))
	})
}

// TestGrammar_ExpressionLiterals validates all scalar literal types and the
// unary plus sign, division operator, and float literal in expression position.
func TestGrammar_ExpressionLiterals(t *testing.T) {
	t.Run("unary plus sign is parsed as UnaryExpr", func(t *testing.T) {
		requireAST(t, "SELECT + 5;", exprQuery(&ast.UnaryExpr{
			Op:      utils.TOKEN_PLUS,
			Operand: &ast.IntegerLiteral{Value: "5"},
		}))
	})

	t.Run("NULL literal in SELECT expression", func(t *testing.T) {
		requireAST(t, "SELECT NULL;", exprQuery(&ast.NullLiteral{}))
	})

	t.Run("TRUE literal in SELECT expression", func(t *testing.T) {
		requireAST(t, "SELECT TRUE;", exprQuery(&ast.BooleanLiteral{Value: "TRUE"}))
	})

	t.Run("FALSE literal in SELECT expression", func(t *testing.T) {
		requireAST(t, "SELECT FALSE;", exprQuery(&ast.BooleanLiteral{Value: "FALSE"}))
	})

	t.Run("string literal in SELECT expression", func(t *testing.T) {
		requireAST(t, "SELECT 'hello';", exprQuery(&ast.StringLiteral{Value: "hello"}))
	})

	t.Run("float literal in SELECT expression", func(t *testing.T) {
		requireAST(t, "SELECT 3.14;", exprQuery(&ast.FloatLiteral{Value: "3.14"}))
	})

	t.Run("division operator: a / b", func(t *testing.T) {
		requireAST(t, "SELECT a / b;", exprQuery(&ast.BinaryExpr{
			Left:  &ast.Identifier{Name: "a"},
			Op:    utils.TOKEN_SLASH,
			Right: &ast.Identifier{Name: "b"},
		}))
	})
}

// TestGrammar_MultiStatement validates that multiple semicolon-separated
// SQL statements are collected into a single *ast.Program correctly.
func TestGrammar_MultiStatement(t *testing.T) {
	t.Run("two statements in one program", func(t *testing.T) {
		requireAST(t,
			"USE mydb; DELETE FROM t;",
			&ast.Program{
				Statements: []ast.Statement{
					&ast.UseDatabaseStmt{Name: "mydb"},
					&ast.DeleteStmt{Table: &ast.Identifier{Name: "t"}},
				},
			})
	})

	t.Run("three statements of different kinds", func(t *testing.T) {
		requireAST(t,
			"CREATE DATABASE d; USE d; DROP DATABASE d;",
			&ast.Program{
				Statements: []ast.Statement{
					&ast.CreateDatabaseStmt{Name: "d"},
					&ast.UseDatabaseStmt{Name: "d"},
					&ast.DropDatabaseStmt{Name: "d"},
				},
			})
	})
}
