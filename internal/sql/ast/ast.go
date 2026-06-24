package ast

import "github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"

type Node interface {
	Span() diagnostic.Span
}

type Statement interface {
	Node
	stmtNode()
}

type Expression interface {
	Node
	exprNode()
}

type Condition interface {
	Node
	condNode()
}

type Clause interface {
	Node
	clauseNode()
}

type NodeBase struct {
	NodeSpan diagnostic.Span
}

func (n *NodeBase) Span() diagnostic.Span { return n.NodeSpan }

type ExprBase struct{ NodeBase }

func (ExprBase) exprNode() {}

type CondBase struct{ NodeBase }

func (CondBase) condNode() {}

type StmtBase struct{ NodeBase }

func (StmtBase) stmtNode() {}

type ClauseBase struct{ NodeBase }

func (ClauseBase) clauseNode() {}

type Program struct {
	NodeBase
	Statements []Statement
}
