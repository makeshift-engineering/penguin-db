package parser

import (
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

func TestParse_Select(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  *ast.Program
	}{
		{
			name:  "simple select star",
			input: "SELECT * FROM t;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{
							{Star: true},
						},
						From: []*ast.TableRef{
							{
								Primary: &ast.TablePrimary{
									Name: &ast.Identifier{Name: "t"},
								},
							},
						},
					},
				},
			},
		},
		{
			name:  "select without from",
			input: "SELECT 1 + 1;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{
							{
								Expr: &ast.SelectExpression{
									Expr: &ast.BinaryExpr{
										Left:  &ast.IntegerLiteral{Value: "1"},
										Op:    utils.TOKEN_PLUS,
										Right: &ast.IntegerLiteral{Value: "1"},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:  "select distinct columns",
			input: "SELECT DISTINCT id, name AS n FROM t;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Distinct: true,
						Columns: []*ast.SelectColumn{
							{
								Expr: &ast.SelectExpression{
									Expr: &ast.Identifier{Name: "id"},
								},
							},
							{
								Expr: &ast.SelectExpression{
									Expr: &ast.Identifier{Name: "name"},
								},
								Alias: "n",
							},
						},
						From: []*ast.TableRef{
							{
								Primary: &ast.TablePrimary{
									Name: &ast.Identifier{Name: "t"},
								},
							},
						},
					},
				},
			},
		},
		{
			name:  "select all",
			input: "SELECT ALL id FROM t;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						All: true,
						Columns: []*ast.SelectColumn{
							{
								Expr: &ast.SelectExpression{
									Expr: &ast.Identifier{Name: "id"},
								},
							},
						},
						From: []*ast.TableRef{
							{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}},
						},
					},
				},
			},
		},
		{
			name:  "select qualified star",
			input: "SELECT a.* FROM a;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{
							{
								QualifiedStar: &ast.Identifier{Name: "a"},
							},
						},
						From: []*ast.TableRef{
							{
								Primary: &ast.TablePrimary{
									Name: &ast.Identifier{Name: "a"},
								},
							},
						},
					},
				},
			},
		},
		{
			name:  "select two-level qualified star",
			input: "SELECT db.t.* FROM t;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{
							{
								QualifiedStar: &ast.Identifier{Qualifier: "db", Name: "t"},
							},
						},
						From: []*ast.TableRef{
							{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}},
						},
					},
				},
			},
		},
		{
			name:  "select with implicit column alias",
			input: "SELECT id myid FROM t;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{
							{
								Expr:  &ast.SelectExpression{Expr: &ast.Identifier{Name: "id"}},
								Alias: "myid",
							},
						},
						From: []*ast.TableRef{
							{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}},
						},
					},
				},
			},
		},
		{
			name:  "select with joins and where",
			input: "SELECT a.* FROM a INNER JOIN b ON a.id = b.id WHERE a.id > 10;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{
							{
								QualifiedStar: &ast.Identifier{Name: "a"},
							},
						},
						From: []*ast.TableRef{
							{
								Primary: &ast.TablePrimary{
									Name: &ast.Identifier{Name: "a"},
								},
								Joins: []*ast.JoinClause{
									{
										Type: ast.JoinInner,
										Right: &ast.TablePrimary{
											Name: &ast.Identifier{Name: "b"},
										},
										On: &ast.ComparisonPredicate{
											Left:  &ast.Identifier{Name: "id", Qualifier: "a"},
											Op:    utils.TOKEN_EQ,
											Right: &ast.Identifier{Name: "id", Qualifier: "b"},
										},
									},
								},
							},
						},
						Where: &ast.WhereClause{
							Cond: &ast.ComparisonPredicate{
								Left:  &ast.Identifier{Name: "id", Qualifier: "a"},
								Op:    utils.TOKEN_GT,
								Right: &ast.IntegerLiteral{Value: "10"},
							},
						},
					},
				},
			},
		},
		{
			name:  "select with all clauses",
			input: "SELECT id FROM t GROUP BY id HAVING id > 0 ORDER BY id DESC LIMIT 10 OFFSET 5;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{
							{
								Expr: &ast.SelectExpression{
									Expr: &ast.Identifier{Name: "id"},
								},
							},
						},
						From: []*ast.TableRef{
							{
								Primary: &ast.TablePrimary{
									Name: &ast.Identifier{Name: "t"},
								},
							},
						},
						GroupBy: &ast.GroupByClause{
							Columns: []*ast.Identifier{
								{Name: "id"},
							},
						},
						Having: &ast.HavingClause{
							Cond: &ast.ComparisonPredicate{
								Left:  &ast.Identifier{Name: "id"},
								Op:    utils.TOKEN_GT,
								Right: &ast.IntegerLiteral{Value: "0"},
							},
						},
						OrderBy: &ast.OrderByClause{
							Items: []*ast.OrderByItem{
								{
									Expr:      &ast.Identifier{Name: "id"},
									Direction: ast.OrderDesc,
								},
							},
						},
						Limit: &ast.LimitClause{
							Count:  10,
							Offset: ptr(5),
						},
					},
				},
			},
		},
		{
			name:  "select multiple columns with functions",
			input: "SELECT COUNT(*), SUM(val) FROM t;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{
							{
								Expr: &ast.SelectExpression{
									Expr: &ast.FunctionCall{Name: "COUNT", Star: true},
								},
							},
							{
								Expr: &ast.SelectExpression{
									Expr: &ast.FunctionCall{
										Name: "SUM",
										Args: []*ast.SelectExpression{
											{Expr: &ast.Identifier{Name: "val"}},
										},
									},
								},
							},
						},
						From: []*ast.TableRef{
							{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}},
						},
					},
				},
			},
		},
		{
			name:  "select with expression and condition in select list",
			input: "SELECT a > 1 FROM t;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{
							{
								Expr: &ast.SelectExpression{
									Cond: &ast.ComparisonPredicate{
										Left:  &ast.Identifier{Name: "a"},
										Op:    utils.TOKEN_GT,
										Right: &ast.IntegerLiteral{Value: "1"},
									},
								},
							},
						},
						From: []*ast.TableRef{
							{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}},
						},
					},
				},
			},
		},
		{
			name:  "select condition with and or in column list",
			input: "SELECT a = 1 AND b = 2 FROM t;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{
							{
								Expr: &ast.SelectExpression{
									Cond: &ast.BinaryCondition{
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
								},
							},
						},
						From: []*ast.TableRef{
							{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}},
						},
					},
				},
			},
		},
		{
			name:  "select not condition in column list",
			input: "SELECT NOT active FROM t;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{
							{
								Expr: &ast.SelectExpression{
									Cond: &ast.NotCondition{
										Operand: &ast.ExprCondition{
											Expr: &ast.Identifier{Name: "active"},
										},
									},
								},
							},
						},
						From: []*ast.TableRef{
							{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}},
						},
					},
				},
			},
		},
		{
			name:  "select where only",
			input: "SELECT * FROM t WHERE x = 1;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{{Star: true}},
						From: []*ast.TableRef{
							{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}},
						},
						Where: &ast.WhereClause{
							Cond: &ast.ComparisonPredicate{
								Left:  &ast.Identifier{Name: "x"},
								Op:    utils.TOKEN_EQ,
								Right: &ast.IntegerLiteral{Value: "1"},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireAST(t, tt.input, tt.want)
		})
	}
}

func TestParse_SelectErrors(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{"SELECT missing list", "SELECT FROM t;", CodeExpectedExpression},
		{"SELECT GROUP BY missing BY", "SELECT * FROM t GROUP t;", CodeUnexpectedToken},
		{"SELECT ORDER BY missing BY", "SELECT * FROM t ORDER t;", CodeUnexpectedToken},
		{"SELECT out of order clauses", "SELECT * FROM t LIMIT 10 WHERE id = 1;", CodeUnexpectedToken},
		{"SELECT JOIN missing ON", "SELECT * FROM a JOIN b WHERE a.id = b.id;", CodeUnexpectedToken},
		{"SELECT missing semicolon", "SELECT 1 SELECT 2;", CodeUnexpectedToken},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireParseError(t, tt.input, tt.wantErr)
		})
	}
}

func TestParse_MultipleStatements(t *testing.T) {
	input := "CREATE DATABASE mydb; USE mydb; DROP DATABASE mydb;"
	want := &ast.Program{
		Statements: []ast.Statement{
			&ast.CreateDatabaseStmt{Name: "mydb"},
			&ast.UseDatabaseStmt{Name: "mydb"},
			&ast.DropDatabaseStmt{Name: "mydb"},
		},
	}
	requireAST(t, input, want)
}

func TestParse_EmptyInput(t *testing.T) {
	requireAST(t, "", &ast.Program{})
}

func TestParse_UnknownStatementKeyword(t *testing.T) {
	requireParseError(t, "BOGUS;", CodeMalformedStatement)
}
