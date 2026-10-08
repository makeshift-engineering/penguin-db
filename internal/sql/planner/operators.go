package planner

import (
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

// RelNode is the sealed interface for a node in a SELECT statement's
// relational operator tree — the internal recursive structure a QueryPlan
// wraps. This is distinct from Plan: Plan is the top-level result of
// planning one statement, RelNode is what that result is built from for a
// SELECT specifically.
type RelNode interface {
	relNode()
}

// RelNodeBase is embedded in every RelNode implementation to satisfy the
// interface's marker method.
type RelNodeBase struct{}

func (*RelNodeBase) relNode() {}

// ScanNode reads every row of a single resolved table.
type ScanNode struct {
	RelNodeBase
	Table *ResolvedTable
}

// JoinNode combines two inputs. Type is always ast.JoinInner or
// ast.JoinCross — LEFT, RIGHT, and FULL are rejected during FROM-clause
// planning before a JoinNode is ever constructed. On is nil for a CROSS
// JOIN. Right is a general RelNode rather than a bare ScanNode: within one
// FROM-clause table reference's own Joins list the right side is always a
// single table, but chaining separate comma-separated FROM items together
// (an implicit CROSS JOIN) needs to join two arbitrary subtrees, so the
// node is kept general rather than needing two similar shapes.
type JoinNode struct {
	RelNodeBase
	Left  RelNode
	Right RelNode
	Type  ast.JoinType
	On    ResolvedCond
}

// FilterNode keeps only rows from Input for which Cond is true. It's used
// for both WHERE (wrapping a scan/join tree) and HAVING (wrapping an
// AggregateNode).
type FilterNode struct {
	RelNodeBase
	Input RelNode
	Cond  ResolvedCond
}

// ProjectItem is one computed output slot: a resolved expression plus the
// display name it's known by, either an explicit AS alias or one derived
// from the expression itself (see defaultAlias). It's used both for a
// plain SELECT list (ProjectNode.Items) and for an aggregate SELECT list
// (AggregateNode.Aggregates) — the shape is identical either way.
type ProjectItem struct {
	Expr  ResolvedExpr
	Alias string
}

// ProjectNode evaluates Items against each row of Input, producing the
// final SELECT-list output. Used for non-aggregate queries only — an
// aggregate query's projection is AggregateNode.Aggregates instead, since
// grouping and projecting the aggregate SELECT list happen together in a
// single pass over grouped rows.
type ProjectNode struct {
	RelNodeBase
	Input RelNode
	Items []ProjectItem
}

// AggregateNode groups Input's rows by GroupBy and evaluates Aggregates
// once per group. GroupBy is empty when the query has aggregate functions
// but no explicit GROUP BY clause — SQL's "single implicit group" case —
// in which case Aggregates may only contain aggregate function calls and
// literal constants, never bare column references (see aggregate.go).
type AggregateNode struct {
	RelNodeBase
	Input      RelNode
	GroupBy    []ResolvedExpr
	Aggregates []ProjectItem
}

// DistinctNode deduplicates Input's rows.
type DistinctNode struct {
	RelNodeBase
	Input RelNode
}

// SortItem is one ORDER BY key.
type SortItem struct {
	Expr      ResolvedExpr
	Direction ast.OrderDirection
}

// SortNode orders Input's rows by Items, in priority order.
type SortNode struct {
	RelNodeBase
	Input RelNode
	Items []SortItem
}

// LimitNode caps Input to at most Count rows, after skipping Offset of
// them. Offset is 0 when no OFFSET was specified. Both values must be
// non-negative; the planner validates this before constructing the node.
type LimitNode struct {
	RelNodeBase
	Input  RelNode
	Count  int64
	Offset int64
}
