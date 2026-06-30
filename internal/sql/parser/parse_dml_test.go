package parser

import (
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

func TestParse_Insert(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  *ast.Program
	}{
		{
			name:  "simple values",
			input: "INSERT INTO t VALUES (1, 'a');",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.InsertStmt{
						Table: &ast.Identifier{Name: "t"},
						Rows: [][]*ast.SelectExpression{
							{
								{Expr: &ast.IntegerLiteral{Value: "1"}},
								{Expr: &ast.StringLiteral{Value: "a"}},
							},
						},
					},
				},
			},
		},
		{
			name:  "with columns and multiple rows",
			input: "INSERT INTO t (id, name) VALUES (1, 'a'), (2, 'b');",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.InsertStmt{
						Table:   &ast.Identifier{Name: "t"},
						Columns: []string{"id", "name"},
						Rows: [][]*ast.SelectExpression{
							{
								{Expr: &ast.IntegerLiteral{Value: "1"}},
								{Expr: &ast.StringLiteral{Value: "a"}},
							},
							{
								{Expr: &ast.IntegerLiteral{Value: "2"}},
								{Expr: &ast.StringLiteral{Value: "b"}},
							},
						},
					},
				},
			},
		},
		{
			name:  "insert select",
			input: "INSERT INTO t SELECT * FROM other_t;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.InsertStmt{
						Table: &ast.Identifier{Name: "t"},
						Source: &ast.SelectStmt{
							Columns: []*ast.SelectColumn{
								{Star: true},
							},
							From: []*ast.TableRef{
								{
									Primary: &ast.TablePrimary{
										Name: &ast.Identifier{Name: "other_t"},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:  "insert with qualified table",
			input: "INSERT INTO mydb.t VALUES (1);",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.InsertStmt{
						Table: &ast.Identifier{Name: "t", Qualifier: "mydb"},
						Rows: [][]*ast.SelectExpression{
							{
								{Expr: &ast.IntegerLiteral{Value: "1"}},
							},
						},
					},
				},
			},
		},
		{
			name:  "insert with null and boolean values",
			input: "INSERT INTO t VALUES (NULL, TRUE, FALSE);",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.InsertStmt{
						Table: &ast.Identifier{Name: "t"},
						Rows: [][]*ast.SelectExpression{
							{
								{Expr: &ast.NullLiteral{}},
								{Expr: &ast.BooleanLiteral{Value: "TRUE"}},
								{Expr: &ast.BooleanLiteral{Value: "FALSE"}},
							},
						},
					},
				},
			},
		},
		{
			name:  "insert with expression value",
			input: "INSERT INTO t VALUES (1 + 2);",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.InsertStmt{
						Table: &ast.Identifier{Name: "t"},
						Rows: [][]*ast.SelectExpression{
							{
								{Expr: &ast.BinaryExpr{
									Left:  &ast.IntegerLiteral{Value: "1"},
									Op:    utils.TOKEN_PLUS,
									Right: &ast.IntegerLiteral{Value: "2"},
								}},
							},
						},
					},
				},
			},
		},
		{
			name:  "insert select with columns",
			input: "INSERT INTO t (id) SELECT id FROM other_t;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.InsertStmt{
						Table:   &ast.Identifier{Name: "t"},
						Columns: []string{"id"},
						Source: &ast.SelectStmt{
							Columns: []*ast.SelectColumn{
								{Expr: &ast.SelectExpression{Expr: &ast.Identifier{Name: "id"}}},
							},
							From: []*ast.TableRef{
								{Primary: &ast.TablePrimary{Name: &ast.Identifier{Name: "other_t"}}},
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

func TestParse_Update(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  *ast.Program
	}{
		{
			name:  "simple",
			input: "UPDATE t SET a = 1;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.UpdateStmt{
						Table: &ast.Identifier{Name: "t"},
						Set: []*ast.SetItem{
							{
								Column: &ast.Identifier{Name: "a"},
								Value:  &ast.IntegerLiteral{Value: "1"},
							},
						},
					},
				},
			},
		},
		{
			name:  "multiple set and where",
			input: "UPDATE t SET a = 1, b = 'foo' WHERE id = 42;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.UpdateStmt{
						Table: &ast.Identifier{Name: "t"},
						Set: []*ast.SetItem{
							{
								Column: &ast.Identifier{Name: "a"},
								Value:  &ast.IntegerLiteral{Value: "1"},
							},
							{
								Column: &ast.Identifier{Name: "b"},
								Value:  &ast.StringLiteral{Value: "foo"},
							},
						},
						Where: &ast.WhereClause{
							Cond: &ast.ComparisonPredicate{
								Left:  &ast.Identifier{Name: "id"},
								Op:    utils.TOKEN_EQ,
								Right: &ast.IntegerLiteral{Value: "42"},
							},
						},
					},
				},
			},
		},
		{
			name:  "update with null value",
			input: "UPDATE t SET a = NULL;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.UpdateStmt{
						Table: &ast.Identifier{Name: "t"},
						Set: []*ast.SetItem{
							{
								Column: &ast.Identifier{Name: "a"},
								Value:  &ast.NullLiteral{},
							},
						},
					},
				},
			},
		},
		{
			name:  "update with expression value",
			input: "UPDATE t SET count = count + 1 WHERE id = 1;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.UpdateStmt{
						Table: &ast.Identifier{Name: "t"},
						Set: []*ast.SetItem{
							{
								Column: &ast.Identifier{Name: "count"},
								Value: &ast.BinaryExpr{
									Left:  &ast.Identifier{Name: "count"},
									Op:    utils.TOKEN_PLUS,
									Right: &ast.IntegerLiteral{Value: "1"},
								},
							},
						},
						Where: &ast.WhereClause{
							Cond: &ast.ComparisonPredicate{
								Left:  &ast.Identifier{Name: "id"},
								Op:    utils.TOKEN_EQ,
								Right: &ast.IntegerLiteral{Value: "1"},
							},
						},
					},
				},
			},
		},
		{
			name:  "update with qualified table",
			input: "UPDATE mydb.t SET a = 1;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.UpdateStmt{
						Table: &ast.Identifier{Name: "t", Qualifier: "mydb"},
						Set: []*ast.SetItem{
							{
								Column: &ast.Identifier{Name: "a"},
								Value:  &ast.IntegerLiteral{Value: "1"},
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

func TestParse_Delete(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  *ast.Program
	}{
		{
			name:  "simple",
			input: "DELETE FROM t;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.DeleteStmt{
						Table: &ast.Identifier{Name: "t"},
					},
				},
			},
		},
		{
			name:  "with where",
			input: "DELETE FROM t WHERE id = 1;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.DeleteStmt{
						Table: &ast.Identifier{Name: "t"},
						Where: &ast.WhereClause{
							Cond: &ast.ComparisonPredicate{
								Left:  &ast.Identifier{Name: "id"},
								Op:    utils.TOKEN_EQ,
								Right: &ast.IntegerLiteral{Value: "1"},
							},
						},
					},
				},
			},
		},
		{
			name:  "with complex where",
			input: "DELETE FROM t WHERE id > 10 AND active = TRUE;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.DeleteStmt{
						Table: &ast.Identifier{Name: "t"},
						Where: &ast.WhereClause{
							Cond: &ast.BinaryCondition{
								Left: &ast.ComparisonPredicate{
									Left:  &ast.Identifier{Name: "id"},
									Op:    utils.TOKEN_GT,
									Right: &ast.IntegerLiteral{Value: "10"},
								},
								Op: utils.TOKEN_AND,
								Right: &ast.ComparisonPredicate{
									Left:  &ast.Identifier{Name: "active"},
									Op:    utils.TOKEN_EQ,
									Right: &ast.BooleanLiteral{Value: "TRUE"},
								},
							},
						},
					},
				},
			},
		},
		{
			name:  "delete with qualified table",
			input: "DELETE FROM mydb.t;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.DeleteStmt{
						Table: &ast.Identifier{Name: "t", Qualifier: "mydb"},
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

func TestParse_DMLErrors(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		// INSERT errors
		{"INSERT missing INTO", "INSERT t VALUES (1);", CodeUnexpectedToken},
		{"INSERT missing table", "INSERT INTO VALUES (1);", CodeUnexpectedToken},
		{"INSERT invalid source", "INSERT INTO t CREATE TABLE;", CodeMalformedStatement},
		{"INSERT missing value rparen", "INSERT INTO t VALUES (1, 2;", CodeUnexpectedToken},
		{"INSERT missing values lparen", "INSERT INTO t VALUES 1;", CodeUnexpectedToken},

		// UPDATE errors
		{"UPDATE missing SET", "UPDATE t a = 1;", CodeUnexpectedToken},
		{"UPDATE invalid SET item", "UPDATE t SET 1 = 1;", CodeUnexpectedToken},
		{"UPDATE missing equals", "UPDATE t SET a 1;", CodeUnexpectedToken},

		// DELETE errors
		{"DELETE missing FROM", "DELETE t;", CodeUnexpectedToken},
		{"DELETE missing table", "DELETE FROM;", CodeUnexpectedToken},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireParseError(t, tt.input, tt.wantErr)
		})
	}
}
