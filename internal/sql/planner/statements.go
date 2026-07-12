package planner

import "github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"

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
