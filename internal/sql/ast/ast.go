package ast

import (
	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
)

// Node is the root interface satisfied by every AST node. It provides source
// location information through [diagnostic.Span].
type Node interface {
	Span() diagnostic.Span
	Validate() error
}

// Statement is the sealed interface for top-level SQL statements
// (e.g. SELECT, INSERT, CREATE TABLE). The unexported stmtNode marker
// prevents external packages from implementing Statement.
type Statement interface {
	Node
	stmtNode()
}

// Expression is the sealed interface for value-producing SQL expressions
// (e.g. literals, identifiers, arithmetic operations, function calls).
type Expression interface {
	Node
	exprNode()
}

// Condition is the sealed interface for boolean-valued SQL conditions
// (e.g. comparisons, predicates, AND/OR/NOT combinations).
type Condition interface {
	Node
	condNode()
}

// Clause is the sealed interface for structural components of statements
// that are not standalone statements themselves (e.g. WHERE, ORDER BY,
// column definitions, join clauses).
type Clause interface {
	Node
	clauseNode()
}

// NodeBase provides the [Node] interface implementation via struct embedding.
// It stores the source span and exposes it through [Span]. All category-
// specific bases ([ExprBase], [CondBase], [StmtBase], [ClauseBase]) embed
// NodeBase, so concrete types need only embed the appropriate category base.
type NodeBase struct {
	NodeSpan diagnostic.Span
}

// Span returns the source location range of this node.
func (n *NodeBase) Span() diagnostic.Span { return n.NodeSpan }

// Validate provides a default implementation of the Validate method that returns nil.
// This is overridden by concrete AST nodes that require structural validation.
func (n *NodeBase) Validate() error { return nil }

// ExprBase is the embeddable base for concrete [Expression] types.
type ExprBase struct{ NodeBase }

func (ExprBase) exprNode() {}

// CondBase is the embeddable base for concrete [Condition] types.
type CondBase struct{ NodeBase }

func (CondBase) condNode() {}

// StmtBase is the embeddable base for concrete [Statement] types.
type StmtBase struct{ NodeBase }

func (StmtBase) stmtNode() {}

// ClauseBase is the embeddable base for concrete [Clause] types.
type ClauseBase struct{ NodeBase }

func (ClauseBase) clauseNode() {}

// Program is the top-level AST node representing a complete SQL input.
// It contains one or more semicolon-separated [Statement] nodes.
type Program struct {
	NodeBase
	Statements []Statement
}

func (p *Program) Validate() error {
	for _, stmt := range p.Statements {
		if stmt == nil {
			return ErrNilStatement
		}
		if err := stmt.Validate(); err != nil {
			return err
		}
	}
	return nil
}
