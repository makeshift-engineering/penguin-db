package parser

import (
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

// condQuery is a helper to construct a DELETE statement AST containing a single WHERE condition.
func condQuery(cond ast.Condition) *ast.Program {
	return &ast.Program{
		Statements: []ast.Statement{
			&ast.DeleteStmt{
				Table: &ast.Identifier{Name: "t"},
				Where: &ast.WhereClause{
					Cond: cond,
				},
			},
		},
	}
}

func TestParse_Conditions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  *ast.Program
	}{

		{
			name:  "comparison eq",
			input: "DELETE FROM t WHERE a = 1;",
			want: condQuery(&ast.ComparisonPredicate{
				Left:  &ast.Identifier{Name: "a"},
				Op:    utils.TOKEN_EQ,
				Right: &ast.IntegerLiteral{Value: "1"},
			}),
		},
		{
			name:  "comparison neq",
			input: "DELETE FROM t WHERE a != 1;",
			want: condQuery(&ast.ComparisonPredicate{
				Left:  &ast.Identifier{Name: "a"},
				Op:    utils.TOKEN_NEQ,
				Right: &ast.IntegerLiteral{Value: "1"},
			}),
		},
		{
			name:  "comparison lt",
			input: "DELETE FROM t WHERE a < 10;",
			want: condQuery(&ast.ComparisonPredicate{
				Left:  &ast.Identifier{Name: "a"},
				Op:    utils.TOKEN_LT,
				Right: &ast.IntegerLiteral{Value: "10"},
			}),
		},
		{
			name:  "comparison gt",
			input: "DELETE FROM t WHERE a > 10;",
			want: condQuery(&ast.ComparisonPredicate{
				Left:  &ast.Identifier{Name: "a"},
				Op:    utils.TOKEN_GT,
				Right: &ast.IntegerLiteral{Value: "10"},
			}),
		},
		{
			name:  "comparison lte",
			input: "DELETE FROM t WHERE a <= 10;",
			want: condQuery(&ast.ComparisonPredicate{
				Left:  &ast.Identifier{Name: "a"},
				Op:    utils.TOKEN_LTE,
				Right: &ast.IntegerLiteral{Value: "10"},
			}),
		},
		{
			name:  "comparison gte",
			input: "DELETE FROM t WHERE a >= 10;",
			want: condQuery(&ast.ComparisonPredicate{
				Left:  &ast.Identifier{Name: "a"},
				Op:    utils.TOKEN_GTE,
				Right: &ast.IntegerLiteral{Value: "10"},
			}),
		},

		{
			name:  "logical and",
			input: "DELETE FROM t WHERE a = 1 AND b = 2;",
			want: condQuery(&ast.BinaryCondition{
				Left: &ast.ComparisonPredicate{
					Left:  &ast.Identifier{Name: "a"},
					Op:    utils.TOKEN_EQ,
					Right: &ast.IntegerLiteral{Value: "1"},
				},
				Op: utils.TOKEN_AND,
				Right: &ast.ComparisonPredicate{
					Left:  &ast.Identifier{Name: "b"},
					Op:    utils.TOKEN_EQ,
					Right: &ast.IntegerLiteral{Value: "2"},
				},
			}),
		},
		{
			name:  "logical or with precedence (AND binds tighter)",
			input: "DELETE FROM t WHERE a = 1 OR b = 2 AND c = 3;",
			want: condQuery(&ast.BinaryCondition{
				Left: &ast.ComparisonPredicate{
					Left:  &ast.Identifier{Name: "a"},
					Op:    utils.TOKEN_EQ,
					Right: &ast.IntegerLiteral{Value: "1"},
				},
				Op: utils.TOKEN_OR,
				Right: &ast.BinaryCondition{
					Left: &ast.ComparisonPredicate{
						Left:  &ast.Identifier{Name: "b"},
						Op:    utils.TOKEN_EQ,
						Right: &ast.IntegerLiteral{Value: "2"},
					},
					Op: utils.TOKEN_AND,
					Right: &ast.ComparisonPredicate{
						Left:  &ast.Identifier{Name: "c"},
						Op:    utils.TOKEN_EQ,
						Right: &ast.IntegerLiteral{Value: "3"},
					},
				},
			}),
		},
		{
			name:  "multiple and",
			input: "DELETE FROM t WHERE a = 1 AND b = 2 AND c = 3;",
			want: condQuery(&ast.BinaryCondition{
				Left: &ast.BinaryCondition{
					Left: &ast.ComparisonPredicate{
						Left:  &ast.Identifier{Name: "a"},
						Op:    utils.TOKEN_EQ,
						Right: &ast.IntegerLiteral{Value: "1"},
					},
					Op: utils.TOKEN_AND,
					Right: &ast.ComparisonPredicate{
						Left:  &ast.Identifier{Name: "b"},
						Op:    utils.TOKEN_EQ,
						Right: &ast.IntegerLiteral{Value: "2"},
					},
				},
				Op: utils.TOKEN_AND,
				Right: &ast.ComparisonPredicate{
					Left:  &ast.Identifier{Name: "c"},
					Op:    utils.TOKEN_EQ,
					Right: &ast.IntegerLiteral{Value: "3"},
				},
			}),
		},
		{
			name:  "multiple or",
			input: "DELETE FROM t WHERE a = 1 OR b = 2 OR c = 3;",
			want: condQuery(&ast.BinaryCondition{
				Left: &ast.BinaryCondition{
					Left: &ast.ComparisonPredicate{
						Left:  &ast.Identifier{Name: "a"},
						Op:    utils.TOKEN_EQ,
						Right: &ast.IntegerLiteral{Value: "1"},
					},
					Op: utils.TOKEN_OR,
					Right: &ast.ComparisonPredicate{
						Left:  &ast.Identifier{Name: "b"},
						Op:    utils.TOKEN_EQ,
						Right: &ast.IntegerLiteral{Value: "2"},
					},
				},
				Op: utils.TOKEN_OR,
				Right: &ast.ComparisonPredicate{
					Left:  &ast.Identifier{Name: "c"},
					Op:    utils.TOKEN_EQ,
					Right: &ast.IntegerLiteral{Value: "3"},
				},
			}),
		},

		{
			name:  "not condition",
			input: "DELETE FROM t WHERE NOT a = 1;",
			want: condQuery(&ast.NotCondition{
				Operand: &ast.ComparisonPredicate{
					Left:  &ast.Identifier{Name: "a"},
					Op:    utils.TOKEN_EQ,
					Right: &ast.IntegerLiteral{Value: "1"},
				},
			}),
		},
		{
			name:  "double not",
			input: "DELETE FROM t WHERE NOT NOT a = 1;",
			want: condQuery(&ast.NotCondition{
				Operand: &ast.NotCondition{
					Operand: &ast.ComparisonPredicate{
						Left:  &ast.Identifier{Name: "a"},
						Op:    utils.TOKEN_EQ,
						Right: &ast.IntegerLiteral{Value: "1"},
					},
				},
			}),
		},

		{
			name:  "is null",
			input: "DELETE FROM t WHERE a IS NULL;",
			want: condQuery(&ast.IsNullPredicate{
				Expr:    &ast.Identifier{Name: "a"},
				Negated: false,
			}),
		},
		{
			name:  "is not null",
			input: "DELETE FROM t WHERE a IS NOT NULL;",
			want: condQuery(&ast.IsNullPredicate{
				Expr:    &ast.Identifier{Name: "a"},
				Negated: true,
			}),
		},

		{
			name:  "like",
			input: "DELETE FROM t WHERE a LIKE '%foo%';",
			want: condQuery(&ast.LikePredicate{
				Left:    &ast.Identifier{Name: "a"},
				Pattern: &ast.StringLiteral{Value: "%foo%"},
				Negated: false,
			}),
		},
		{
			name:  "not like",
			input: "DELETE FROM t WHERE a NOT LIKE '%foo%';",
			want: condQuery(&ast.LikePredicate{
				Left:    &ast.Identifier{Name: "a"},
				Pattern: &ast.StringLiteral{Value: "%foo%"},
				Negated: true,
			}),
		},

		{
			name:  "in list",
			input: "DELETE FROM t WHERE a IN (1, 2, 3);",
			want: condQuery(&ast.InPredicate{
				Expr: &ast.Identifier{Name: "a"},
				Values: []ast.Expression{
					&ast.IntegerLiteral{Value: "1"},
					&ast.IntegerLiteral{Value: "2"},
					&ast.IntegerLiteral{Value: "3"},
				},
				Negated: false,
			}),
		},
		{
			name:  "not in list",
			input: "DELETE FROM t WHERE a NOT IN (1);",
			want: condQuery(&ast.InPredicate{
				Expr: &ast.Identifier{Name: "a"},
				Values: []ast.Expression{
					&ast.IntegerLiteral{Value: "1"},
				},
				Negated: true,
			}),
		},
		{
			name:  "in list with strings",
			input: "DELETE FROM t WHERE status IN ('active', 'pending');",
			want: condQuery(&ast.InPredicate{
				Expr: &ast.Identifier{Name: "status"},
				Values: []ast.Expression{
					&ast.StringLiteral{Value: "active"},
					&ast.StringLiteral{Value: "pending"},
				},
				Negated: false,
			}),
		},

		{
			name:  "between",
			input: "DELETE FROM t WHERE a BETWEEN 1 AND 10;",
			want: condQuery(&ast.BetweenPredicate{
				Expr:    &ast.Identifier{Name: "a"},
				Low:     &ast.IntegerLiteral{Value: "1"},
				High:    &ast.IntegerLiteral{Value: "10"},
				Negated: false,
			}),
		},
		{
			name:  "not between",
			input: "DELETE FROM t WHERE a NOT BETWEEN 1 AND 10;",
			want: condQuery(&ast.BetweenPredicate{
				Expr:    &ast.Identifier{Name: "a"},
				Low:     &ast.IntegerLiteral{Value: "1"},
				High:    &ast.IntegerLiteral{Value: "10"},
				Negated: true,
			}),
		},

		{
			name:  "parenthesized condition overrides precedence",
			input: "DELETE FROM t WHERE (a = 1 OR b = 2) AND c = 3;",
			want: condQuery(&ast.BinaryCondition{
				Left: &ast.ParenCondition{
					Inner: &ast.BinaryCondition{
						Left: &ast.ComparisonPredicate{
							Left:  &ast.Identifier{Name: "a"},
							Op:    utils.TOKEN_EQ,
							Right: &ast.IntegerLiteral{Value: "1"},
						},
						Op: utils.TOKEN_OR,
						Right: &ast.ComparisonPredicate{
							Left:  &ast.Identifier{Name: "b"},
							Op:    utils.TOKEN_EQ,
							Right: &ast.IntegerLiteral{Value: "2"},
						},
					},
				},
				Op: utils.TOKEN_AND,
				Right: &ast.ComparisonPredicate{
					Left:  &ast.Identifier{Name: "c"},
					Op:    utils.TOKEN_EQ,
					Right: &ast.IntegerLiteral{Value: "3"},
				},
			}),
		},
		{
			name:  "nested parenthesized comparison",
			input: "DELETE FROM t WHERE (a > 5);",
			want: condQuery(&ast.ParenCondition{
				Inner: &ast.ComparisonPredicate{
					Left:  &ast.Identifier{Name: "a"},
					Op:    utils.TOKEN_GT,
					Right: &ast.IntegerLiteral{Value: "5"},
				},
			}),
		},

		{
			name:  "bare expression as condition",
			input: "DELETE FROM t WHERE active;",
			want: condQuery(&ast.ExprCondition{
				Expr: &ast.Identifier{Name: "active"},
			}),
		},

		{
			name:  "comparison with arithmetic on left",
			input: "DELETE FROM t WHERE a + 1 = 10;",
			want: condQuery(&ast.ComparisonPredicate{
				Left: &ast.BinaryExpr{
					Left:  &ast.Identifier{Name: "a"},
					Op:    utils.TOKEN_PLUS,
					Right: &ast.IntegerLiteral{Value: "1"},
				},
				Op:    utils.TOKEN_EQ,
				Right: &ast.IntegerLiteral{Value: "10"},
			}),
		},
		{
			name:  "comparison with qualified identifiers",
			input: "DELETE FROM t WHERE t.a = t.b;",
			want: condQuery(&ast.ComparisonPredicate{
				Left:  &ast.Identifier{Qualifier: "t", Name: "a"},
				Op:    utils.TOKEN_EQ,
				Right: &ast.Identifier{Qualifier: "t", Name: "b"},
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireAST(t, tt.input, tt.want)
		})
	}
}

func TestParse_ConditionErrors(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantErr  error
		wantLine int
		wantCol  int
	}{
		{"incomplete comparison", "DELETE FROM t WHERE a = ;", CodeExpectedExpression, 1, 25},
		{"is without null", "DELETE FROM t WHERE a IS TRUE;", CodeUnexpectedToken, 1, 26},
		{"in missing rparen", "DELETE FROM t WHERE a IN (1, 2;", CodeUnexpectedToken, 1, 31},
		{"between missing and", "DELETE FROM t WHERE a BETWEEN 1 OR 2;", CodeUnexpectedToken, 1, 33},
		{"not missing predicate", "DELETE FROM t WHERE a NOT = 1;", CodeUnexpectedToken, 1, 23},
		{"incomplete parenthesis", "DELETE FROM t WHERE (a = 1;", CodeUnexpectedToken, 1, 27},
		{"empty where clause", "DELETE FROM t WHERE ;", CodeExpectedExpression, 1, 21},
		{"in empty list", "DELETE FROM t WHERE a IN ();", CodeExpectedExpression, 1, 27},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireParseError(t, tt.input, tt.wantErr, tt.wantLine, tt.wantCol)
		})
	}
}
