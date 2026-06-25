package ast_test

import (
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
	"github.com/makeshift-engineering/penguin-db/internal/sql/lexer"
)

var (
	_ ast.Expression = (*ast.IntegerLiteral)(nil)
	_ ast.Expression = (*ast.FloatLiteral)(nil)
	_ ast.Expression = (*ast.StringLiteral)(nil)
	_ ast.Expression = (*ast.BooleanLiteral)(nil)
	_ ast.Expression = (*ast.NullLiteral)(nil)
	_ ast.Expression = (*ast.Identifier)(nil)
	_ ast.Expression = (*ast.BinaryExpr)(nil)
	_ ast.Expression = (*ast.UnaryExpr)(nil)
	_ ast.Expression = (*ast.ParenExpr)(nil)
	_ ast.Expression = (*ast.FunctionCall)(nil)
)

var (
	_ ast.Condition = (*ast.BinaryCondition)(nil)
	_ ast.Condition = (*ast.NotCondition)(nil)
	_ ast.Condition = (*ast.ComparisonPredicate)(nil)
	_ ast.Condition = (*ast.LikePredicate)(nil)
	_ ast.Condition = (*ast.IsNullPredicate)(nil)
	_ ast.Condition = (*ast.InPredicate)(nil)
	_ ast.Condition = (*ast.BetweenPredicate)(nil)
	_ ast.Condition = (*ast.ParenCondition)(nil)
	_ ast.Condition = (*ast.ExprCondition)(nil)
)

var (
	_ ast.Statement = (*ast.CreateDatabaseStmt)(nil)
	_ ast.Statement = (*ast.UseDatabaseStmt)(nil)
	_ ast.Statement = (*ast.DropDatabaseStmt)(nil)
	_ ast.Statement = (*ast.CreateTableStmt)(nil)
	_ ast.Statement = (*ast.AlterTableStmt)(nil)
	_ ast.Statement = (*ast.DropTableStmt)(nil)
	_ ast.Statement = (*ast.SelectStmt)(nil)
	_ ast.Statement = (*ast.InsertStmt)(nil)
	_ ast.Statement = (*ast.UpdateStmt)(nil)
	_ ast.Statement = (*ast.DeleteStmt)(nil)
)

var (
	_ ast.Clause = (*ast.DataType)(nil)
	_ ast.Clause = (*ast.ColumnDef)(nil)
	_ ast.Clause = (*ast.ColumnConstraints)(nil)
	_ ast.Clause = (*ast.SignedLiteral)(nil)
	_ ast.Clause = (*ast.ForeignRef)(nil)
	_ ast.Clause = (*ast.AlterAction)(nil)
	_ ast.Clause = (*ast.SelectColumn)(nil)
	_ ast.Clause = (*ast.TableRef)(nil)
	_ ast.Clause = (*ast.TablePrimary)(nil)
	_ ast.Clause = (*ast.JoinClause)(nil)
	_ ast.Clause = (*ast.WhereClause)(nil)
	_ ast.Clause = (*ast.GroupByClause)(nil)
	_ ast.Clause = (*ast.HavingClause)(nil)
	_ ast.Clause = (*ast.OrderByClause)(nil)
	_ ast.Clause = (*ast.OrderByItem)(nil)
	_ ast.Clause = (*ast.LimitClause)(nil)
	_ ast.Clause = (*ast.SetItem)(nil)
)

var _ ast.Node = (*ast.Program)(nil)
var _ ast.Node = (*ast.SelectExpression)(nil)

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
		{"BooleanLiteral", &ast.BooleanLiteral{ExprBase: eb(span), Value: true}},
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
		{"ColumnConstraints", &ast.ColumnConstraints{ClauseBase: clb(span)}},
		{"SignedLiteral", &ast.SignedLiteral{ClauseBase: clb(span)}},
		{"ForeignRef", &ast.ForeignRef{ClauseBase: clb(span)}},
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

func TestCondition_TypeSwitchCoverage(t *testing.T) {
	conditions := []ast.Condition{
		&ast.BinaryCondition{},
		&ast.NotCondition{},
		&ast.ComparisonPredicate{},
		&ast.LikePredicate{},
		&ast.IsNullPredicate{},
		&ast.InPredicate{},
		&ast.BetweenPredicate{},
		&ast.ParenCondition{},
		&ast.ExprCondition{},
	}

	for _, c := range conditions {
		switch c.(type) {
		case *ast.BinaryCondition:
		case *ast.NotCondition:
		case *ast.ComparisonPredicate:
		case *ast.LikePredicate:
		case *ast.IsNullPredicate:
		case *ast.InPredicate:
		case *ast.BetweenPredicate:
		case *ast.ParenCondition:
		case *ast.ExprCondition:
		default:
			t.Errorf("unhandled Condition type: %T", c)
		}
	}
}

func TestStatement_TypeSwitchCoverage(t *testing.T) {
	stmts := []ast.Statement{
		&ast.CreateDatabaseStmt{},
		&ast.UseDatabaseStmt{},
		&ast.DropDatabaseStmt{},
		&ast.CreateTableStmt{},
		&ast.AlterTableStmt{},
		&ast.DropTableStmt{},
		&ast.SelectStmt{},
		&ast.InsertStmt{},
		&ast.UpdateStmt{},
		&ast.DeleteStmt{},
	}

	for _, s := range stmts {
		switch s.(type) {
		case *ast.CreateDatabaseStmt:
		case *ast.UseDatabaseStmt:
		case *ast.DropDatabaseStmt:
		case *ast.CreateTableStmt:
		case *ast.AlterTableStmt:
		case *ast.DropTableStmt:
		case *ast.SelectStmt:
		case *ast.InsertStmt:
		case *ast.UpdateStmt:
		case *ast.DeleteStmt:
		default:
			t.Errorf("unhandled Statement type: %T", s)
		}
	}
}

func TestExpression_TypeSwitchCoverage(t *testing.T) {
	exprs := []ast.Expression{
		&ast.IntegerLiteral{},
		&ast.FloatLiteral{},
		&ast.StringLiteral{},
		&ast.BooleanLiteral{},
		&ast.NullLiteral{},
		&ast.Identifier{},
		&ast.BinaryExpr{},
		&ast.UnaryExpr{},
		&ast.ParenExpr{},
		&ast.FunctionCall{},
	}

	for _, e := range exprs {
		switch e.(type) {
		case *ast.IntegerLiteral:
		case *ast.FloatLiteral:
		case *ast.StringLiteral:
		case *ast.BooleanLiteral:
		case *ast.NullLiteral:
		case *ast.Identifier:
		case *ast.BinaryExpr:
		case *ast.UnaryExpr:
		case *ast.ParenExpr:
		case *ast.FunctionCall:
		default:
			t.Errorf("unhandled Expression type: %T", e)
		}
	}
}
