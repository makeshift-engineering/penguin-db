package parser

import (
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
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
		// CREATE DATABASE errors
		{"CREATE DATABASE missing name", "CREATE DATABASE;", CodeUnexpectedToken, 1, 16},

		// CREATE TABLE errors
		{"CREATE TABLE missing lparen", "CREATE TABLE t id INT;", CodeUnexpectedToken, 1, 16},
		{"CREATE TABLE missing rparen", "CREATE TABLE t (id INT", CodeUnexpectedToken, 1, 23},
		{"CREATE TABLE invalid type", "CREATE TABLE t (id UNKNOWN_TYPE);", CodeInvalidDataType, 1, 20},
		{"CREATE TABLE invalid constraint", "CREATE TABLE t (id INT NOT TRUE);", CodeUnexpectedToken, 1, 28},
		{"CREATE TABLE missing column name", "CREATE TABLE t (INT);", CodeUnexpectedToken, 1, 17},

		// DEFAULT with + sign rejects non-numeric
		{"DEFAULT plus string", "CREATE TABLE t (val TEXT DEFAULT +'hello');", CodeExpectedExpression, 1, 35},
		{"DEFAULT minus string", "CREATE TABLE t (val TEXT DEFAULT -'hello');", CodeExpectedExpression, 1, 35},

		// REFERENCES errors
		{"REFERENCES missing table", "CREATE TABLE t (id INT REFERENCES);", CodeUnexpectedToken, 1, 34},
		{"REFERENCES missing lparen", "CREATE TABLE t (id INT REFERENCES parent);", CodeUnexpectedToken, 1, 41},
		{"REFERENCES missing column", "CREATE TABLE t (id INT REFERENCES parent());", CodeUnexpectedToken, 1, 42},
		{"REFERENCES missing rparen", "CREATE TABLE t (id INT REFERENCES parent(id);", CodeUnexpectedToken, 1, 45},

		// ALTER TABLE errors
		{"ALTER TABLE missing action", "ALTER TABLE t;", CodeInvalidAlterAction, 1, 14},
		{"ALTER TABLE RENAME missing TO", "ALTER TABLE t RENAME new_t;", CodeInvalidAlterAction, 1, 22},
		{"ALTER TABLE RENAME COLUMN missing TO", "ALTER TABLE t RENAME COLUMN c1 new_c1;", CodeUnexpectedToken, 1, 32},
		{"ALTER TABLE DROP missing COLUMN", "ALTER TABLE t DROP c;", CodeUnexpectedToken, 1, 20},

		// DROP TABLE errors
		{"DROP TABLE missing name", "DROP TABLE;", CodeUnexpectedToken, 1, 11},

		// CREATE/DROP bad target
		{"CREATE unknown", "CREATE INDEX foo;", CodeMalformedStatement, 1, 8},
		{"DROP unknown", "DROP INDEX foo;", CodeMalformedStatement, 1, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireParseError(t, tt.input, tt.wantErr, tt.wantLine, tt.wantCol)
		})
	}
}
