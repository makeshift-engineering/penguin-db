package parser

import (
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

func TestParse_TableReferences(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  *ast.Program
	}{
		{
			name:  "cross join",
			input: "SELECT * FROM a CROSS JOIN b;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{{Star: true}},
						From: []*ast.TableRef{
							{
								Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "a"}},
								Joins: []*ast.JoinClause{
									{
										Type:  ast.JoinCross,
										Right: &ast.TablePrimary{Name: &ast.Identifier{Name: "b"}},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:  "left outer join",
			input: "SELECT * FROM a LEFT OUTER JOIN b ON a.id = b.id;",
			want: &ast.Program{
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
			},
		},
		{
			name:  "left join without outer",
			input: "SELECT * FROM a LEFT JOIN b ON a.id = b.id;",
			want: &ast.Program{
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
			},
		},
		{
			name:  "right outer join",
			input: "SELECT * FROM a RIGHT OUTER JOIN b ON a.id = b.id;",
			want: &ast.Program{
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
			},
		},
		{
			name:  "right join without outer",
			input: "SELECT * FROM a RIGHT JOIN b ON a.id = b.id;",
			want: &ast.Program{
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
			},
		},
		{
			name:  "full join without outer",
			input: "SELECT * FROM a FULL JOIN b ON a.id = b.id;",
			want: &ast.Program{
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
			},
		},
		{
			name:  "full outer join",
			input: "SELECT * FROM a FULL OUTER JOIN b ON a.id = b.id;",
			want: &ast.Program{
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
			},
		},
		{
			name:  "bare join defaults to inner",
			input: "SELECT * FROM a JOIN b ON a.id = b.id;",
			want: &ast.Program{
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
			},
		},
		{
			name:  "explicit inner join",
			input: "SELECT * FROM a INNER JOIN b ON a.id = b.id;",
			want: &ast.Program{
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
			},
		},
		{
			name:  "multiple chained joins",
			input: "SELECT * FROM a JOIN b ON a.id = b.a_id LEFT JOIN c ON b.id = c.b_id;",
			want: &ast.Program{
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
											Left:  &ast.Identifier{Qualifier: "a", Name: "id"},
											Op:    utils.TOKEN_EQ,
											Right: &ast.Identifier{Qualifier: "b", Name: "a_id"},
										},
									},
									{
										Type:  ast.JoinLeft,
										Right: &ast.TablePrimary{Name: &ast.Identifier{Name: "c"}},
										On: &ast.ComparisonPredicate{
											Left:  &ast.Identifier{Qualifier: "b", Name: "id"},
											Op:    utils.TOKEN_EQ,
											Right: &ast.Identifier{Qualifier: "c", Name: "b_id"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:  "parenthesized table ref",
			input: "SELECT * FROM (a INNER JOIN b ON a.id = b.id);",
			want: &ast.Program{
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
							},
						},
					},
				},
			},
		},
		{
			name:  "parenthesized ref with trailing join",
			input: "SELECT * FROM (a) JOIN b ON a.id = b.id;",
			want: &ast.Program{
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
			},
		},
		{
			name:  "comma separated table refs",
			input: "SELECT * FROM a, b;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{{Star: true}},
						From: []*ast.TableRef{
							{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "a"}}},
							{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "b"}}},
						},
					},
				},
			},
		},
		{
			name:  "table alias with AS",
			input: "SELECT * FROM users AS u;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{{Star: true}},
						From: []*ast.TableRef{
							{Primary: &ast.TablePrimary{
								Name:  &ast.Identifier{Name: "users"},
								Alias: "u",
							}},
						},
					},
				},
			},
		},
		{
			name:  "table implicit alias",
			input: "SELECT * FROM users u;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{{Star: true}},
						From: []*ast.TableRef{
							{Primary: &ast.TablePrimary{
								Name:  &ast.Identifier{Name: "users"},
								Alias: "u",
							}},
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

func TestParse_OrderBy(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  *ast.Program
	}{
		{
			name:  "order by single column default asc",
			input: "SELECT * FROM t ORDER BY id;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{{Star: true}},
						From: []*ast.TableRef{
							{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}},
						},
						OrderBy: &ast.OrderByClause{
							Items: []*ast.OrderByItem{
								{Expr: &ast.Identifier{Name: "id"}, Direction: ast.OrderAsc},
							},
						},
					},
				},
			},
		},
		{
			name:  "order by explicit asc",
			input: "SELECT * FROM t ORDER BY id ASC;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{{Star: true}},
						From: []*ast.TableRef{
							{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}},
						},
						OrderBy: &ast.OrderByClause{
							Items: []*ast.OrderByItem{
								{Expr: &ast.Identifier{Name: "id"}, Direction: ast.OrderAsc},
							},
						},
					},
				},
			},
		},
		{
			name:  "order by desc",
			input: "SELECT * FROM t ORDER BY id DESC;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{{Star: true}},
						From: []*ast.TableRef{
							{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}},
						},
						OrderBy: &ast.OrderByClause{
							Items: []*ast.OrderByItem{
								{Expr: &ast.Identifier{Name: "id"}, Direction: ast.OrderDesc},
							},
						},
					},
				},
			},
		},
		{
			name:  "order by multiple columns",
			input: "SELECT * FROM t ORDER BY name ASC, id DESC;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{{Star: true}},
						From: []*ast.TableRef{
							{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}},
						},
						OrderBy: &ast.OrderByClause{
							Items: []*ast.OrderByItem{
								{Expr: &ast.Identifier{Name: "name"}, Direction: ast.OrderAsc},
								{Expr: &ast.Identifier{Name: "id"}, Direction: ast.OrderDesc},
							},
						},
					},
				},
			},
		},
		{
			name:  "order by expression",
			input: "SELECT * FROM t ORDER BY 1;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{{Star: true}},
						From: []*ast.TableRef{
							{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}},
						},
						OrderBy: &ast.OrderByClause{
							Items: []*ast.OrderByItem{
								{Expr: &ast.IntegerLiteral{Value: "1"}, Direction: ast.OrderAsc},
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

func TestParse_GroupByAndHaving(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  *ast.Program
	}{
		{
			name:  "group by single column",
			input: "SELECT id FROM t GROUP BY id;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{
							{Expr: &ast.SelectExpression{Expr: &ast.Identifier{Name: "id"}}},
						},
						From: []*ast.TableRef{
							{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}},
						},
						GroupBy: &ast.GroupByClause{
							Columns: []*ast.Identifier{{Name: "id"}},
						},
					},
				},
			},
		},
		{
			name:  "group by multiple columns",
			input: "SELECT dept, role FROM t GROUP BY dept, role;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{
							{Expr: &ast.SelectExpression{Expr: &ast.Identifier{Name: "dept"}}},
							{Expr: &ast.SelectExpression{Expr: &ast.Identifier{Name: "role"}}},
						},
						From: []*ast.TableRef{
							{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}},
						},
						GroupBy: &ast.GroupByClause{
							Columns: []*ast.Identifier{{Name: "dept"}, {Name: "role"}},
						},
					},
				},
			},
		},
		{
			name:  "having clause",
			input: "SELECT dept FROM t GROUP BY dept HAVING dept = 'eng';",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{
							{Expr: &ast.SelectExpression{Expr: &ast.Identifier{Name: "dept"}}},
						},
						From: []*ast.TableRef{
							{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}},
						},
						GroupBy: &ast.GroupByClause{
							Columns: []*ast.Identifier{{Name: "dept"}},
						},
						Having: &ast.HavingClause{
							Cond: &ast.ComparisonPredicate{
								Left:  &ast.Identifier{Name: "dept"},
								Op:    utils.TOKEN_EQ,
								Right: &ast.StringLiteral{Value: "eng"},
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

func TestParse_LimitAndOffset(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  *ast.Program
	}{
		{
			name:  "limit only",
			input: "SELECT * FROM t LIMIT 10;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{{Star: true}},
						From: []*ast.TableRef{
							{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}},
						},
						Limit: &ast.LimitClause{
							Count: 10,
						},
					},
				},
			},
		},
		{
			name:  "limit and offset",
			input: "SELECT * FROM t LIMIT 10 OFFSET 5;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{{Star: true}},
						From: []*ast.TableRef{
							{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}},
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
			name:  "limit zero",
			input: "SELECT * FROM t LIMIT 0;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.SelectStmt{
						Columns: []*ast.SelectColumn{{Star: true}},
						From: []*ast.TableRef{
							{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "t"}}},
						},
						Limit: &ast.LimitClause{Count: 0},
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

func TestParse_ClauseErrors(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{"join missing table", "SELECT * FROM a JOIN ;", CodeUnexpectedToken},
		{"join missing ON", "SELECT * FROM a INNER JOIN b;", CodeUnexpectedToken},
		{"limit invalid count", "SELECT * FROM t LIMIT 'a';", CodeUnexpectedToken},
		{"offset missing number", "SELECT * FROM t LIMIT 10 OFFSET ;", CodeUnexpectedToken},
		{"cross join with ON", "SELECT * FROM a CROSS JOIN b ON a.id = b.id;", CodeUnexpectedToken},
		{"group by missing column", "SELECT * FROM t GROUP BY;", CodeUnexpectedToken},
		{"having without group by standalone condition", "SELECT * FROM t HAVING ;", CodeExpectedExpression},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireParseError(t, tt.input, tt.wantErr)
		})
	}
}
