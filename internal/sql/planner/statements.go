package planner

import (
	"fmt"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

// Plan is the sealed interface for every top-level plan node — the output
// of planning a single ast.Statement.
type Plan interface {
	planNode()
}

// PlanBase is embedded in every Plan implementation to satisfy the
// interface's marker method.
type PlanBase struct{}

func (*PlanBase) planNode() {}

// CreateDatabasePlan plans a CREATE DATABASE statement. BuildCreateDatabaseOps
// performs no existence check of its own (see its doc comment), so this
// planner-side check is the only safety net CREATE DATABASE gets against
// silently overwriting an existing entry.
type CreateDatabasePlan struct {
	PlanBase
	Name string
	// NoOp is true when IF NOT EXISTS was specified and the database
	// already exists; the executor should report success without doing
	// anything.
	NoOp bool
}

// UseDatabasePlan plans a USE statement.
type UseDatabasePlan struct {
	PlanBase
	Name string
}

// DropDatabasePlan plans a DROP DATABASE statement. Tables holds every
// table currently in the database — BuildDropDatabaseOps needs each one's
// schema to delete both its catalog entry and its sequence-counter key.
type DropDatabasePlan struct {
	PlanBase
	Name   string
	Tables []*catalog.TableMeta
	// NoOp is true when IF EXISTS was specified and the database does not
	// exist.
	NoOp bool
}

// CreateTablePlan plans a CREATE TABLE statement. Schema is nil when NoOp
// is true (IF NOT EXISTS specified, table already exists).
type CreateTablePlan struct {
	PlanBase
	Database string
	Table    string
	Schema   *catalog.TableMeta
	NoOp     bool
}

// DropTablePlan plans a DROP TABLE statement.
type DropTablePlan struct {
	PlanBase
	Database string
	Table    string
	// NoOp is true when IF EXISTS was specified and the table does not
	// exist.
	NoOp bool
}

// AlterTablePlan plans every ALTER TABLE action except a table rename:
// ADD/MODIFY/DROP COLUMN and RENAME COLUMN. OldSchema and NewSchema are the
// full before/after table metadata, since that's what
// catalog.BuildAlterTableOps actually diffs internally — not an abstract
// action descriptor.
type AlterTablePlan struct {
	PlanBase
	OldSchema *catalog.TableMeta
	NewSchema *catalog.TableMeta
}

// RenameTablePlan plans an ALTER TABLE ... RENAME TO statement. Unlike
// every other alter action, a table rename goes through
// catalog.BuildRenameTableOps, not BuildAlterTableOps: it deletes one KV
// key and inserts another rather than diffing a schema, and it needs the
// live sequence-counter bytes for the table, which only the executor can
// read — the planner has no KV access, only catalog metadata. Schema is
// the table's current metadata as of planning time; the executor supplies
// it, the fresh sequence value, and NewName to BuildRenameTableOps.
type RenameTablePlan struct {
	PlanBase
	Database string
	OldName  string
	NewName  string
	Schema   *catalog.TableMeta
}

// OutputColumn describes one column a QueryPlan produces: a display name
// and the type its values will have. Unlike ResolvedColumn, it doesn't
// point at a physical table column — a computed expression, an aggregate
// result, or an alias has no single backing column to point at.
type OutputColumn struct {
	Name string
	Type ast.DataTypeKind
}

// String returns a human-readable representation of the column for
// debugging and logging.
func (c OutputColumn) String() string {
	return fmt.Sprintf("%s:%s", c.Name, typeName(c.Type))
}

// QueryPlan is the top-level Plan produced for a SELECT statement. Root is
// always non-nil: for a FROM-less query (e.g. SELECT 1+1), Root is a
// ProjectNode whose Input is nil — the executor should treat a nil
// ProjectNode.Input as "evaluate the projection once against a single
// implicit row."
type QueryPlan struct {
	PlanBase
	Root    RelNode
	Columns []OutputColumn
}

// InsertPlan plans an INSERT statement. Exactly one of Rows/Source is
// non-nil, matching the shape of ast.InsertStmt. Columns is the ordered
// list of target columns each VALUES row or each Source output column maps
// to positionally — defaulted to every active column in declaration order
// when the statement had no explicit column list.
type InsertPlan struct {
	PlanBase
	Database string
	Table    string
	Schema   *catalog.TableMeta
	Columns  []*ResolvedColumn
	Rows     [][]ResolvedExpr
	Source   *QueryPlan
}

// Assignment is one SET item in an UPDATE statement.
type Assignment struct {
	Column *ResolvedColumn
	Value  ResolvedExpr
}

// UpdatePlan plans an UPDATE statement. Where is nil when the statement has
// no WHERE clause, meaning every row in the table is updated.
type UpdatePlan struct {
	PlanBase
	Database    string
	Table       string
	Schema      *catalog.TableMeta
	Assignments []Assignment
	Where       ResolvedCond
}

// DeletePlan plans a DELETE statement. Where is nil when the statement has
// no WHERE clause, meaning every row in the table is deleted.
type DeletePlan struct {
	PlanBase
	Database string
	Table    string
	Schema   *catalog.TableMeta
	Where    ResolvedCond
}

