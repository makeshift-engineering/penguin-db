package parser

import (
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

// exprQuery is a helper to construct a SELECT statement AST containing a single expression.
func exprQuery(expr ast.Expression) *ast.Program {
	return &ast.Program{
		Statements: []ast.Statement{
			&ast.SelectStmt{
				Columns: []*ast.SelectColumn{
					{
						Expr: &ast.SelectExpression{
							Expr: expr,
						},
					},
				},
			},
		},
	}
}

func TestParse_Expressions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  *ast.Program
	}{

		{
			name:  "integer literal",
			input: "SELECT 42;",
			want:  exprQuery(&ast.IntegerLiteral{Value: "42"}),
		},
		{
			name:  "float literal",
			input: "SELECT 3.14;",
			want:  exprQuery(&ast.FloatLiteral{Value: "3.14"}),
		},
		{
			name:  "string literal",
			input: "SELECT 'hello';",
			want:  exprQuery(&ast.StringLiteral{Value: "hello"}),
		},
		{
			name:  "boolean literal true",
			input: "SELECT TRUE;",
			want:  exprQuery(&ast.BooleanLiteral{Value: "TRUE"}),
		},
		{
			name:  "boolean literal false",
			input: "SELECT FALSE;",
			want:  exprQuery(&ast.BooleanLiteral{Value: "FALSE"}),
		},
		{
			name:  "null literal",
			input: "SELECT NULL;",
			want:  exprQuery(&ast.NullLiteral{}),
		},

		{
			name:  "identifier",
			input: "SELECT col_name;",
			want:  exprQuery(&ast.Identifier{Name: "col_name"}),
		},
		{
			name:  "qualified identifier",
			input: "SELECT my_table.col;",
			want:  exprQuery(&ast.Identifier{Qualifier: "my_table", Name: "col"}),
		},

		{
			name:  "binary plus",
			input: "SELECT 1 + 2;",
			want: exprQuery(&ast.BinaryExpr{
				Left:  &ast.IntegerLiteral{Value: "1"},
				Op:    utils.TOKEN_PLUS,
				Right: &ast.IntegerLiteral{Value: "2"},
			}),
		},
		{
			name:  "binary minus",
			input: "SELECT 5 - 3;",
			want: exprQuery(&ast.BinaryExpr{
				Left:  &ast.IntegerLiteral{Value: "5"},
				Op:    utils.TOKEN_MINUS,
				Right: &ast.IntegerLiteral{Value: "3"},
			}),
		},

		{
			name:  "binary multiply",
			input: "SELECT 2 * 3;",
			want: exprQuery(&ast.BinaryExpr{
				Left:  &ast.IntegerLiteral{Value: "2"},
				Op:    utils.TOKEN_STAR,
				Right: &ast.IntegerLiteral{Value: "3"},
			}),
		},
		{
			name:  "binary divide",
			input: "SELECT 10 / 2;",
			want: exprQuery(&ast.BinaryExpr{
				Left:  &ast.IntegerLiteral{Value: "10"},
				Op:    utils.TOKEN_SLASH,
				Right: &ast.IntegerLiteral{Value: "2"},
			}),
		},
		{
			name:  "binary modulo",
			input: "SELECT 10 % 3;",
			want: exprQuery(&ast.BinaryExpr{
				Left:  &ast.IntegerLiteral{Value: "10"},
				Op:    utils.TOKEN_PERCENT,
				Right: &ast.IntegerLiteral{Value: "3"},
			}),
		},

		{
			name:  "precedence multiply before add",
			input: "SELECT 1 + 2 * 3;",
			want: exprQuery(&ast.BinaryExpr{
				Left: &ast.IntegerLiteral{Value: "1"},
				Op:   utils.TOKEN_PLUS,
				Right: &ast.BinaryExpr{
					Left:  &ast.IntegerLiteral{Value: "2"},
					Op:    utils.TOKEN_STAR,
					Right: &ast.IntegerLiteral{Value: "3"},
				},
			}),
		},
		{
			name:  "precedence multiply then add",
			input: "SELECT 1 * 2 + 3;",
			want: exprQuery(&ast.BinaryExpr{
				Left: &ast.BinaryExpr{
					Left:  &ast.IntegerLiteral{Value: "1"},
					Op:    utils.TOKEN_STAR,
					Right: &ast.IntegerLiteral{Value: "2"},
				},
				Op:    utils.TOKEN_PLUS,
				Right: &ast.IntegerLiteral{Value: "3"},
			}),
		},
		{
			name:  "left associativity of addition",
			input: "SELECT 1 + 2 + 3;",
			want: exprQuery(&ast.BinaryExpr{
				Left: &ast.BinaryExpr{
					Left:  &ast.IntegerLiteral{Value: "1"},
					Op:    utils.TOKEN_PLUS,
					Right: &ast.IntegerLiteral{Value: "2"},
				},
				Op:    utils.TOKEN_PLUS,
				Right: &ast.IntegerLiteral{Value: "3"},
			}),
		},
		{
			name:  "left associativity of multiplication",
			input: "SELECT 2 * 3 * 4;",
			want: exprQuery(&ast.BinaryExpr{
				Left: &ast.BinaryExpr{
					Left:  &ast.IntegerLiteral{Value: "2"},
					Op:    utils.TOKEN_STAR,
					Right: &ast.IntegerLiteral{Value: "3"},
				},
				Op:    utils.TOKEN_STAR,
				Right: &ast.IntegerLiteral{Value: "4"},
			}),
		},

		{
			name:  "parentheses override precedence",
			input: "SELECT (1 + 2) * 3;",
			want: exprQuery(&ast.BinaryExpr{
				Left: &ast.ParenExpr{
					Inner: &ast.BinaryExpr{
						Left:  &ast.IntegerLiteral{Value: "1"},
						Op:    utils.TOKEN_PLUS,
						Right: &ast.IntegerLiteral{Value: "2"},
					},
				},
				Op:    utils.TOKEN_STAR,
				Right: &ast.IntegerLiteral{Value: "3"},
			}),
		},
		{
			name:  "nested parentheses",
			input: "SELECT ((1 + 2));",
			want: exprQuery(&ast.ParenExpr{
				Inner: &ast.ParenExpr{
					Inner: &ast.BinaryExpr{
						Left:  &ast.IntegerLiteral{Value: "1"},
						Op:    utils.TOKEN_PLUS,
						Right: &ast.IntegerLiteral{Value: "2"},
					},
				},
			}),
		},

		{
			name:  "unary minus",
			input: "SELECT - 5;",
			want: exprQuery(&ast.UnaryExpr{
				Op:      utils.TOKEN_MINUS,
				Operand: &ast.IntegerLiteral{Value: "5"},
			}),
		},
		{
			name:  "unary plus",
			input: "SELECT + 5;",
			want: exprQuery(&ast.UnaryExpr{
				Op:      utils.TOKEN_PLUS,
				Operand: &ast.IntegerLiteral{Value: "5"},
			}),
		},
		{
			name:  "double unary minus",
			input: "SELECT - - 5;",
			want: exprQuery(&ast.UnaryExpr{
				Op: utils.TOKEN_MINUS,
				Operand: &ast.UnaryExpr{
					Op:      utils.TOKEN_MINUS,
					Operand: &ast.IntegerLiteral{Value: "5"},
				},
			}),
		},
		{
			name:  "unary minus in expression",
			input: "SELECT 1 + - 2;",
			want: exprQuery(&ast.BinaryExpr{
				Left: &ast.IntegerLiteral{Value: "1"},
				Op:   utils.TOKEN_PLUS,
				Right: &ast.UnaryExpr{
					Op:      utils.TOKEN_MINUS,
					Operand: &ast.IntegerLiteral{Value: "2"},
				},
			}),
		},

		{
			name:  "function zero args",
			input: "SELECT NOW();",
			want: exprQuery(&ast.FunctionCall{
				Name: "NOW",
			}),
		},
		{
			name:  "function one arg",
			input: "SELECT UPPER(name);",
			want: exprQuery(&ast.FunctionCall{
				Name: "UPPER",
				Args: []*ast.SelectExpression{
					{Expr: &ast.Identifier{Name: "name"}},
				},
			}),
		},
		{
			name:  "function multiple args",
			input: "SELECT COALESCE(a, 'default');",
			want: exprQuery(&ast.FunctionCall{
				Name: "COALESCE",
				Args: []*ast.SelectExpression{
					{Expr: &ast.Identifier{Name: "a"}},
					{Expr: &ast.StringLiteral{Value: "default"}},
				},
			}),
		},
		{
			name:  "function star",
			input: "SELECT COUNT(*);",
			want: exprQuery(&ast.FunctionCall{
				Name: "COUNT",
				Star: true,
			}),
		},
		{
			name:  "function distinct",
			input: "SELECT COUNT(DISTINCT id);",
			want: exprQuery(&ast.FunctionCall{
				Name:     "COUNT",
				Distinct: true,
				Args: []*ast.SelectExpression{
					{Expr: &ast.Identifier{Name: "id"}},
				},
			}),
		},
		{
			name:  "function with expression arg",
			input: "SELECT ABS(a - b);",
			want: exprQuery(&ast.FunctionCall{
				Name: "ABS",
				Args: []*ast.SelectExpression{
					{Expr: &ast.BinaryExpr{
						Left:  &ast.Identifier{Name: "a"},
						Op:    utils.TOKEN_MINUS,
						Right: &ast.Identifier{Name: "b"},
					}},
				},
			}),
		},

		{
			name:  "mixed operators",
			input: "SELECT a + b * c - d / e;",
			want: exprQuery(&ast.BinaryExpr{
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
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireAST(t, tt.input, tt.want)
		})
	}
}

func TestParse_ExpressionErrors(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{"missing right operand", "SELECT 1 + ;", CodeExpectedExpression},
		{"unmatched paren", "SELECT (1 + 2;", CodeUnexpectedToken},
		{"function missing rparen", "SELECT UPPER(a;", CodeUnexpectedToken},
		{"invalid expression", "SELECT SELECT;", CodeExpectedExpression},
		{"empty paren", "SELECT ();", CodeExpectedExpression},
		{"trailing operator", "SELECT 1 *;", CodeExpectedExpression},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireParseError(t, tt.input, tt.wantErr)
		})
	}
}
