package ast_test

import (
	"errors"
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
	"github.com/makeshift-engineering/penguin-db/internal/sql/lexer"
)

var _ ast.Node = (*ast.Program)(nil)

// helpers to build base structs with a span
func eb(s diagnostic.Span) ast.ExprBase { return ast.ExprBase{NodeBase: ast.NodeBase{NodeSpan: s}} }
func cb(s diagnostic.Span) ast.CondBase { return ast.CondBase{NodeBase: ast.NodeBase{NodeSpan: s}} }
func sb(s diagnostic.Span) ast.StmtBase { return ast.StmtBase{NodeBase: ast.NodeBase{NodeSpan: s}} }
func clb(s diagnostic.Span) ast.ClauseBase {
	return ast.ClauseBase{NodeBase: ast.NodeBase{NodeSpan: s}}
}
func nb(s diagnostic.Span) ast.NodeBase { return ast.NodeBase{NodeSpan: s} }

func TestSpan_ReturnsStoredSpan(t *testing.T) {
	span := diagnostic.Span{
		Start: diagnostic.Pos{Line: 1, Col: 1, Offset: 0},
		End:   diagnostic.Pos{Line: 1, Col: 10, Offset: 9},
	}

	tests := []struct {
		name string
		node ast.Node
	}{
		{"IntegerLiteral", &ast.IntegerLiteral{ExprBase: eb(span), Value: "42"}},
		{"FloatLiteral", &ast.FloatLiteral{ExprBase: eb(span), Value: "3.14"}},
		{"StringLiteral", &ast.StringLiteral{ExprBase: eb(span), Value: "hello"}},
		{"BooleanLiteral", &ast.BooleanLiteral{ExprBase: eb(span), Value: "TRUE"}},
		{"NullLiteral", &ast.NullLiteral{ExprBase: eb(span)}},
		{"Identifier", &ast.Identifier{ExprBase: eb(span), Name: "id"}},
		{"BinaryExpr", &ast.BinaryExpr{ExprBase: eb(span), Op: lexer.TOKEN_PLUS}},
		{"UnaryExpr", &ast.UnaryExpr{ExprBase: eb(span), Op: lexer.TOKEN_MINUS}},
		{"ParenExpr", &ast.ParenExpr{ExprBase: eb(span)}},
		{"FunctionCall", &ast.FunctionCall{ExprBase: eb(span), Name: "COUNT"}},

		{"BinaryCondition", &ast.BinaryCondition{CondBase: cb(span), Op: lexer.TOKEN_AND}},
		{"NotCondition", &ast.NotCondition{CondBase: cb(span)}},
		{"ComparisonPredicate", &ast.ComparisonPredicate{CondBase: cb(span), Op: lexer.TOKEN_EQ}},
		{"LikePredicate", &ast.LikePredicate{CondBase: cb(span)}},
		{"IsNullPredicate", &ast.IsNullPredicate{CondBase: cb(span)}},
		{"InPredicate", &ast.InPredicate{CondBase: cb(span)}},
		{"BetweenPredicate", &ast.BetweenPredicate{CondBase: cb(span)}},
		{"ParenCondition", &ast.ParenCondition{CondBase: cb(span)}},
		{"ExprCondition", &ast.ExprCondition{CondBase: cb(span)}},

		{"CreateDatabaseStmt", &ast.CreateDatabaseStmt{StmtBase: sb(span), Name: "db"}},
		{"UseDatabaseStmt", &ast.UseDatabaseStmt{StmtBase: sb(span), Name: "db"}},
		{"DropDatabaseStmt", &ast.DropDatabaseStmt{StmtBase: sb(span), Name: "db"}},
		{"CreateTableStmt", &ast.CreateTableStmt{StmtBase: sb(span)}},
		{"AlterTableStmt", &ast.AlterTableStmt{StmtBase: sb(span)}},
		{"DropTableStmt", &ast.DropTableStmt{StmtBase: sb(span)}},
		{"SelectStmt", &ast.SelectStmt{StmtBase: sb(span)}},
		{"InsertStmt", &ast.InsertStmt{StmtBase: sb(span)}},
		{"UpdateStmt", &ast.UpdateStmt{StmtBase: sb(span)}},
		{"DeleteStmt", &ast.DeleteStmt{StmtBase: sb(span)}},

		{"ColumnDef", &ast.ColumnDef{ClauseBase: clb(span)}},
		{"DataType", &ast.DataType{ClauseBase: clb(span)}},
		{"SignedLiteral", &ast.SignedLiteral{ClauseBase: clb(span)}},
		{"PrimaryKeyConstraint", &ast.PrimaryKeyConstraint{ClauseBase: clb(span)}},
		{"UniqueConstraint", &ast.UniqueConstraint{ClauseBase: clb(span)}},
		{"NotNullConstraint", &ast.NotNullConstraint{ClauseBase: clb(span)}},
		{"NullConstraint", &ast.NullConstraint{ClauseBase: clb(span)}},
		{"DefaultConstraint", &ast.DefaultConstraint{ClauseBase: clb(span)}},
		{"ReferencesConstraint", &ast.ReferencesConstraint{ClauseBase: clb(span)}},
		{"AlterAction", &ast.AlterAction{ClauseBase: clb(span)}},
		{"SelectColumn", &ast.SelectColumn{ClauseBase: clb(span)}},
		{"TableRef", &ast.TableRef{ClauseBase: clb(span)}},
		{"TablePrimary", &ast.TablePrimary{ClauseBase: clb(span)}},
		{"JoinClause", &ast.JoinClause{ClauseBase: clb(span)}},
		{"WhereClause", &ast.WhereClause{ClauseBase: clb(span)}},
		{"GroupByClause", &ast.GroupByClause{ClauseBase: clb(span)}},
		{"HavingClause", &ast.HavingClause{ClauseBase: clb(span)}},
		{"OrderByClause", &ast.OrderByClause{ClauseBase: clb(span)}},
		{"OrderByItem", &ast.OrderByItem{ClauseBase: clb(span)}},
		{"LimitClause", &ast.LimitClause{ClauseBase: clb(span)}},
		{"SetItem", &ast.SetItem{ClauseBase: clb(span)}},

		{"Program", &ast.Program{NodeBase: nb(span)}},
		{"SelectExpression", &ast.SelectExpression{NodeBase: nb(span)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.node.Span()
			if got != span {
				t.Errorf("Span() = %v, want %v", got, span)
			}
		})
	}
}

func TestValidation(t *testing.T) {
	tests := []struct {
		name    string
		node    ast.Node
		wantErr error
	}{
		{
			name: "Program valid",
			node: &ast.Program{
				Statements: []ast.Statement{
					&ast.UseDatabaseStmt{Name: "db"},
				},
			},
			wantErr: nil,
		},
		{
			name: "Program nil statement",
			node: &ast.Program{
				Statements: []ast.Statement{nil},
			},
			wantErr: ast.ErrNilStatement,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.node.Validate()
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
