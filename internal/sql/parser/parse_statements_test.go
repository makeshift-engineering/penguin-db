package parser

import (
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

func TestParse_CreateDatabase(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  *ast.Program
	}{
		{
			name:  "simple",
			input: "CREATE DATABASE mydb;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.CreateDatabaseStmt{Name: "mydb", IfNotExists: false},
				},
			},
		},
		{
			name:  "if not exists",
			input: "CREATE DATABASE IF NOT EXISTS mydb;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.CreateDatabaseStmt{Name: "mydb", IfNotExists: true},
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

func TestParse_UseDatabase(t *testing.T) {
	requireAST(t, "USE mydb;", &ast.Program{
		Statements: []ast.Statement{
			&ast.UseDatabaseStmt{Name: "mydb"},
		},
	})
}

func TestParse_DropDatabase(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  *ast.Program
	}{
		{
			name:  "simple",
			input: "DROP DATABASE mydb;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.DropDatabaseStmt{Name: "mydb", IfExists: false},
				},
			},
		},
		{
			name:  "if exists",
			input: "DROP DATABASE IF EXISTS mydb;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.DropDatabaseStmt{Name: "mydb", IfExists: true},
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

func TestParse_CreateTable(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  *ast.Program
	}{
		{
			name:  "simple with basic types",
			input: "CREATE TABLE t (id INT, val VARCHAR(255), is_active BOOLEAN);",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.CreateTableStmt{
						Table:       &ast.Identifier{Name: "t"},
						IfNotExists: false,
						Columns: []*ast.ColumnDef{
							{Name: "id", Type: &ast.DataType{Kind: ast.TypeInt}},
							{Name: "val", Type: &ast.DataType{Kind: ast.TypeVarchar, VarcharLen: ptr(255)}},
							{Name: "is_active", Type: &ast.DataType{Kind: ast.TypeBoolean}},
						},
					},
				},
			},
		},
		{
			name:  "all data types",
			input: "CREATE TABLE t (a INT, b BIGINT, c VARCHAR(50), d BOOLEAN, e TEXT, f TIMESTAMP, g FLOAT, h DOUBLE, i DECIMAL, j DECIMAL(10), k DECIMAL(10, 2));",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.CreateTableStmt{
						Table: &ast.Identifier{Name: "t"},
						Columns: []*ast.ColumnDef{
							{Name: "a", Type: &ast.DataType{Kind: ast.TypeInt}},
							{Name: "b", Type: &ast.DataType{Kind: ast.TypeBigInt}},
							{Name: "c", Type: &ast.DataType{Kind: ast.TypeVarchar, VarcharLen: ptr(50)}},
							{Name: "d", Type: &ast.DataType{Kind: ast.TypeBoolean}},
							{Name: "e", Type: &ast.DataType{Kind: ast.TypeText}},
							{Name: "f", Type: &ast.DataType{Kind: ast.TypeTimestamp}},
							{Name: "g", Type: &ast.DataType{Kind: ast.TypeFloat}},
							{Name: "h", Type: &ast.DataType{Kind: ast.TypeDouble}},
							{Name: "i", Type: &ast.DataType{Kind: ast.TypeDecimal}},
							{Name: "j", Type: &ast.DataType{Kind: ast.TypeDecimal, DecimalPrec: ptr(10)}},
							{Name: "k", Type: &ast.DataType{Kind: ast.TypeDecimal, DecimalPrec: ptr(10), DecimalScale: ptr(2)}},
						},
					},
				},
			},
		},
		{
			name:  "if not exists with qualified table name",
			input: "CREATE TABLE IF NOT EXISTS public.t (id INT);",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.CreateTableStmt{
						Table:       &ast.Identifier{Name: "t", Qualifier: "public"},
						IfNotExists: true,
						Columns: []*ast.ColumnDef{
							{Name: "id", Type: &ast.DataType{Kind: ast.TypeInt}},
						},
					},
				},
			},
		},
		{
			name:  "all constraint types",
			input: "CREATE TABLE t (id INT PRIMARY KEY, name TEXT UNIQUE NOT NULL, val INT NULL, status VARCHAR(10) DEFAULT 'active', parent_id INT REFERENCES parent(id));",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.CreateTableStmt{
						Table: &ast.Identifier{Name: "t"},
						Columns: []*ast.ColumnDef{
							{
								Name: "id",
								Type: &ast.DataType{Kind: ast.TypeInt},
								Constraints: []ast.Clause{
									&ast.PrimaryKeyConstraint{},
								},
							},
							{
								Name: "name",
								Type: &ast.DataType{Kind: ast.TypeText},
								Constraints: []ast.Clause{
									&ast.UniqueConstraint{},
									&ast.NotNullConstraint{},
								},
							},
							{
								Name: "val",
								Type: &ast.DataType{Kind: ast.TypeInt},
								Constraints: []ast.Clause{
									&ast.NullConstraint{},
								},
							},
							{
								Name: "status",
								Type: &ast.DataType{Kind: ast.TypeVarchar, VarcharLen: ptr(10)},
								Constraints: []ast.Clause{
									&ast.DefaultConstraint{
										Value: &ast.SignedLiteral{
											Negative: false,
											Value:    &ast.StringLiteral{Value: "active"},
										},
									},
								},
							},
							{
								Name: "parent_id",
								Type: &ast.DataType{Kind: ast.TypeInt},
								Constraints: []ast.Clause{
									&ast.ForeignRef{Table: "parent", Column: "id"},
								},
							},
						},
					},
				},
			},
		},
		{
			name:  "default negative integer",
			input: "CREATE TABLE t (val INT DEFAULT -1);",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.CreateTableStmt{
						Table: &ast.Identifier{Name: "t"},
						Columns: []*ast.ColumnDef{
							{
								Name: "val",
								Type: &ast.DataType{Kind: ast.TypeInt},
								Constraints: []ast.Clause{
									&ast.DefaultConstraint{
										Value: &ast.SignedLiteral{
											Negative: true,
											Value:    &ast.IntegerLiteral{Value: "1"},
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
			name:  "default positive integer with plus sign",
			input: "CREATE TABLE t (val INT DEFAULT +42);",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.CreateTableStmt{
						Table: &ast.Identifier{Name: "t"},
						Columns: []*ast.ColumnDef{
							{
								Name: "val",
								Type: &ast.DataType{Kind: ast.TypeInt},
								Constraints: []ast.Clause{
									&ast.DefaultConstraint{
										Value: &ast.SignedLiteral{
											Negative: false,
											Value:    &ast.IntegerLiteral{Value: "42"},
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
			name:  "default float",
			input: "CREATE TABLE t (val FLOAT DEFAULT 3.14);",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.CreateTableStmt{
						Table: &ast.Identifier{Name: "t"},
						Columns: []*ast.ColumnDef{
							{
								Name: "val",
								Type: &ast.DataType{Kind: ast.TypeFloat},
								Constraints: []ast.Clause{
									&ast.DefaultConstraint{
										Value: &ast.SignedLiteral{
											Negative: false,
											Value:    &ast.FloatLiteral{Value: "3.14"},
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
			name:  "default boolean",
			input: "CREATE TABLE t (active BOOLEAN DEFAULT TRUE);",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.CreateTableStmt{
						Table: &ast.Identifier{Name: "t"},
						Columns: []*ast.ColumnDef{
							{
								Name: "active",
								Type: &ast.DataType{Kind: ast.TypeBoolean},
								Constraints: []ast.Clause{
									&ast.DefaultConstraint{
										Value: &ast.SignedLiteral{
											Negative: false,
											Value:    &ast.BooleanLiteral{Value: "TRUE"},
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
			name:  "default null",
			input: "CREATE TABLE t (val INT DEFAULT NULL);",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.CreateTableStmt{
						Table: &ast.Identifier{Name: "t"},
						Columns: []*ast.ColumnDef{
							{
								Name: "val",
								Type: &ast.DataType{Kind: ast.TypeInt},
								Constraints: []ast.Clause{
									&ast.DefaultConstraint{
										Value: &ast.SignedLiteral{
											Negative: false,
											Value:    &ast.NullLiteral{},
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
			name:  "default negative float",
			input: "CREATE TABLE t (val DOUBLE DEFAULT -2.5);",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.CreateTableStmt{
						Table: &ast.Identifier{Name: "t"},
						Columns: []*ast.ColumnDef{
							{
								Name: "val",
								Type: &ast.DataType{Kind: ast.TypeDouble},
								Constraints: []ast.Clause{
									&ast.DefaultConstraint{
										Value: &ast.SignedLiteral{
											Negative: true,
											Value:    &ast.FloatLiteral{Value: "2.5"},
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
			name:  "multiple constraints on one column",
			input: "CREATE TABLE t (id INT PRIMARY KEY NOT NULL DEFAULT 0);",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.CreateTableStmt{
						Table: &ast.Identifier{Name: "t"},
						Columns: []*ast.ColumnDef{
							{
								Name: "id",
								Type: &ast.DataType{Kind: ast.TypeInt},
								Constraints: []ast.Clause{
									&ast.PrimaryKeyConstraint{},
									&ast.NotNullConstraint{},
									&ast.DefaultConstraint{
										Value: &ast.SignedLiteral{
											Negative: false,
											Value:    &ast.IntegerLiteral{Value: "0"},
										},
									},
								},
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

func TestParse_AlterTable(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  *ast.Program
	}{
		{
			name:  "add column with COLUMN keyword",
			input: "ALTER TABLE t ADD COLUMN age INT;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.AlterTableStmt{
						Table: &ast.Identifier{Name: "t"},
						Action: &ast.AlterAction{
							Kind: ast.AlterAdd,
							Column: &ast.ColumnDef{
								Name: "age",
								Type: &ast.DataType{Kind: ast.TypeInt},
							},
						},
					},
				},
			},
		},
		{
			name:  "add column without COLUMN keyword",
			input: "ALTER TABLE t ADD age INT;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.AlterTableStmt{
						Table: &ast.Identifier{Name: "t"},
						Action: &ast.AlterAction{
							Kind: ast.AlterAdd,
							Column: &ast.ColumnDef{
								Name: "age",
								Type: &ast.DataType{Kind: ast.TypeInt},
							},
						},
					},
				},
			},
		},
		{
			name:  "modify column with COLUMN keyword",
			input: "ALTER TABLE t MODIFY COLUMN name VARCHAR(100);",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.AlterTableStmt{
						Table: &ast.Identifier{Name: "t"},
						Action: &ast.AlterAction{
							Kind: ast.AlterModify,
							Column: &ast.ColumnDef{
								Name: "name",
								Type: &ast.DataType{Kind: ast.TypeVarchar, VarcharLen: ptr(100)},
							},
						},
					},
				},
			},
		},
		{
			name:  "modify column without COLUMN keyword",
			input: "ALTER TABLE t MODIFY name VARCHAR(100);",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.AlterTableStmt{
						Table: &ast.Identifier{Name: "t"},
						Action: &ast.AlterAction{
							Kind: ast.AlterModify,
							Column: &ast.ColumnDef{
								Name: "name",
								Type: &ast.DataType{Kind: ast.TypeVarchar, VarcharLen: ptr(100)},
							},
						},
					},
				},
			},
		},
		{
			name:  "rename to",
			input: "ALTER TABLE t RENAME TO new_t;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.AlterTableStmt{
						Table: &ast.Identifier{Name: "t"},
						Action: &ast.AlterAction{
							Kind:    ast.AlterRenameTable,
							NewName: "new_t",
						},
					},
				},
			},
		},
		{
			name:  "rename column",
			input: "ALTER TABLE t RENAME COLUMN old_col TO new_col;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.AlterTableStmt{
						Table: &ast.Identifier{Name: "t"},
						Action: &ast.AlterAction{
							Kind:    ast.AlterRenameColumn,
							OldName: "old_col",
							NewName: "new_col",
						},
					},
				},
			},
		},
		{
			name:  "drop column",
			input: "ALTER TABLE t DROP COLUMN c;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.AlterTableStmt{
						Table: &ast.Identifier{Name: "t"},
						Action: &ast.AlterAction{
							Kind:     ast.AlterDropColumn,
							DropName: "c",
						},
					},
				},
			},
		},
		{
			name:  "alter with qualified table",
			input: "ALTER TABLE mydb.t ADD val TEXT;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.AlterTableStmt{
						Table: &ast.Identifier{Name: "t", Qualifier: "mydb"},
						Action: &ast.AlterAction{
							Kind: ast.AlterAdd,
							Column: &ast.ColumnDef{
								Name: "val",
								Type: &ast.DataType{Kind: ast.TypeText},
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

func TestParse_DropTable(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  *ast.Program
	}{
		{
			name:  "simple",
			input: "DROP TABLE t;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.DropTableStmt{Table: &ast.Identifier{Name: "t"}, IfExists: false},
				},
			},
		},
		{
			name:  "if exists",
			input: "DROP TABLE IF EXISTS public.t;",
			want: &ast.Program{
				Statements: []ast.Statement{
					&ast.DropTableStmt{Table: &ast.Identifier{Name: "t", Qualifier: "public"}, IfExists: true},
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

func TestParse_DDLErrors(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantErr  error
		wantLine int
		wantCol  int
	}{
		{"CREATE DATABASE missing name", "CREATE DATABASE;", CodeUnexpectedToken, 1, 16},
		{"CREATE TABLE missing lparen", "CREATE TABLE t id INT;", CodeUnexpectedToken, 1, 16},
		{"CREATE TABLE missing rparen", "CREATE TABLE t (id INT", CodeUnexpectedToken, 1, 23},
		{"CREATE TABLE invalid type", "CREATE TABLE t (id UNKNOWN_TYPE);", CodeInvalidDataType, 1, 20},
		{"CREATE TABLE invalid constraint", "CREATE TABLE t (id INT NOT TRUE);", CodeUnexpectedToken, 1, 28},
		{"CREATE TABLE missing column name", "CREATE TABLE t (INT);", CodeUnexpectedToken, 1, 17},
		{"DEFAULT plus string", "CREATE TABLE t (val TEXT DEFAULT +'hello');", CodeExpectedExpression, 1, 35},
		{"DEFAULT minus string", "CREATE TABLE t (val TEXT DEFAULT -'hello');", CodeExpectedExpression, 1, 35},
		{"REFERENCES missing table", "CREATE TABLE t (id INT REFERENCES);", CodeUnexpectedToken, 1, 34},
		{"REFERENCES missing lparen", "CREATE TABLE t (id INT REFERENCES parent);", CodeUnexpectedToken, 1, 41},
		{"REFERENCES missing column", "CREATE TABLE t (id INT REFERENCES parent());", CodeUnexpectedToken, 1, 42},
		{"REFERENCES missing rparen", "CREATE TABLE t (id INT REFERENCES parent(id);", CodeUnexpectedToken, 1, 45},
		{"ALTER TABLE missing action", "ALTER TABLE t;", CodeInvalidAlterAction, 1, 14},
		{"ALTER TABLE RENAME missing TO", "ALTER TABLE t RENAME new_t;", CodeInvalidAlterAction, 1, 22},
		{"ALTER TABLE RENAME COLUMN missing TO", "ALTER TABLE t RENAME COLUMN c1 new_c1;", CodeUnexpectedToken, 1, 32},
		{"ALTER TABLE DROP missing COLUMN", "ALTER TABLE t DROP c;", CodeUnexpectedToken, 1, 20},
		{"DROP TABLE missing name", "DROP TABLE;", CodeUnexpectedToken, 1, 11},
		{"CREATE unknown", "CREATE INDEX foo;", CodeMalformedStatement, 1, 8},
		{"DROP unknown", "DROP INDEX foo;", CodeMalformedStatement, 1, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireParseError(t, tt.input, tt.wantErr, tt.wantLine, tt.wantCol)
		})
	}
}

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
		name     string
		input    string
		wantErr  error
		wantLine int
		wantCol  int
	}{
		{"INSERT missing INTO", "INSERT t VALUES (1);", CodeUnexpectedToken, 1, 8},
		{"INSERT missing table", "INSERT INTO VALUES (1);", CodeUnexpectedToken, 1, 13},
		{"INSERT invalid source", "INSERT INTO t CREATE TABLE;", CodeMalformedStatement, 1, 15},
		{"INSERT missing value rparen", "INSERT INTO t VALUES (1, 2;", CodeUnexpectedToken, 1, 27},
		{"INSERT missing values lparen", "INSERT INTO t VALUES 1;", CodeUnexpectedToken, 1, 22},
		{"UPDATE missing SET", "UPDATE t a = 1;", CodeUnexpectedToken, 1, 10},
		{"UPDATE invalid SET item", "UPDATE t SET 1 = 1;", CodeUnexpectedToken, 1, 14},
		{"UPDATE missing equals", "UPDATE t SET a 1;", CodeUnexpectedToken, 1, 16},
		{"DELETE missing FROM", "DELETE t;", CodeUnexpectedToken, 1, 8},
		{"DELETE missing table", "DELETE FROM;", CodeUnexpectedToken, 1, 12},
		{
			name:     "multi-line malformed INSERT",
			input:    "INSERT INTO t\nVALUES (1,\n        2;",
			wantErr:  CodeUnexpectedToken,
			wantLine: 3,
			wantCol:  10,
		},
		{
			name:     "multi-line malformed UPDATE",
			input:    "UPDATE t\nSET a = 1,\n    b  2;",
			wantErr:  CodeUnexpectedToken,
			wantLine: 3,
			wantCol:  8,
		},
		{
			name:     "multi-line malformed DELETE",
			input:    "DELETE\nFROM;",
			wantErr:  CodeUnexpectedToken,
			wantLine: 2,
			wantCol:  5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireParseError(t, tt.input, tt.wantErr, tt.wantLine, tt.wantCol)
		})
	}
}

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
		name     string
		input    string
		wantErr  error
		wantLine int
		wantCol  int
	}{
		{"SELECT missing list", "SELECT FROM t;", CodeExpectedExpression, 1, 8},
		{"SELECT GROUP BY missing BY", "SELECT * FROM t GROUP t;", CodeUnexpectedToken, 1, 23},
		{"SELECT ORDER BY missing BY", "SELECT * FROM t ORDER t;", CodeUnexpectedToken, 1, 23},
		{"SELECT out of order clauses", "SELECT * FROM t LIMIT 10 WHERE id = 1;", CodeUnexpectedToken, 1, 26},
		{"SELECT JOIN missing ON", "SELECT * FROM a JOIN b WHERE a.id = b.id;", CodeUnexpectedToken, 1, 24},
		{"SELECT missing semicolon", "SELECT 1 SELECT 2;", CodeUnexpectedToken, 1, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireParseError(t, tt.input, tt.wantErr, tt.wantLine, tt.wantCol)
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
	requireParseError(t, "BOGUS;", CodeMalformedStatement, 1, 1)
}
