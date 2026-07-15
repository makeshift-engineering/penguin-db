// Package planner turns a parsed *ast.Program into a logical plan tree,
// resolving every name against the catalog and attaching the type
// information execution needs. It reads the catalog but never writes to
// it — DDL execution (Build*Ops, WriteBatch, Apply*) belongs to the
// executor, not here.
package planner

import (
	"fmt"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
)

// Session holds the per-connection state a planning call needs beyond the
// AST itself. ActiveDatabase is the database selected by the most recent
// USE statement, or empty if none has been selected yet.
type Session struct {
	ActiveDatabase string
}

// Planner produces logical plans from parsed SQL statements. It wraps a
// single Catalog and holds no other state, so one Planner can safely
// service many concurrent sessions: every planning call builds its own
// planContext rather than mutating the Planner itself.
type Planner struct {
	catalog *catalog.Catalog
}

// New creates a Planner backed by the given catalog.
func New(cat *catalog.Catalog) *Planner {
	return &Planner{catalog: cat}
}

// Plan produces a logical plan for a single parsed statement. session
// supplies the active database for unqualified references; src is used to
// attach source snippets to diagnostics and may be nil. Diagnostics
// accumulated during planning are returned alongside any error, since a
// single statement can surface more than one problem.
func (p *Planner) Plan(stmt ast.Statement, session Session, src *diagnostic.Source) (Plan, diagnostic.List, error) {
	pc := newPlanContext(p.catalog, session, src)
	plan, err := pc.planStatement(stmt)
	return plan, pc.diag, err
}

// planContext carries the mutable state for a single top-level planning
// call: the diagnostics accumulated so far, the active session, the source
// text (for diagnostic snippets), and a reference to the read-only catalog.
// A fresh planContext is created per call so the Planner itself never
// accumulates state across queries.
type planContext struct {
	catalog *catalog.Catalog
	session Session
	source  *diagnostic.Source
	diag    diagnostic.List
}

// newPlanContext creates a planContext for a single planning call.
func newPlanContext(cat *catalog.Catalog, session Session, src *diagnostic.Source) *planContext {
	return &planContext{
		catalog: cat,
		session: session,
		source:  src,
	}
}

// planStatement dispatches to the planning method for stmt's concrete type.
func (pc *planContext) planStatement(stmt ast.Statement) (Plan, error) {
	switch s := stmt.(type) {
	case *ast.CreateDatabaseStmt:
		return pc.planCreateDatabase(s)
	case *ast.UseDatabaseStmt:
		return pc.planUseDatabase(s)
	case *ast.DropDatabaseStmt:
		return pc.planDropDatabase(s)
	case *ast.CreateTableStmt:
		return pc.planCreateTable(s)
	case *ast.AlterTableStmt:
		return pc.planAlterTable(s)
	case *ast.DropTableStmt:
		return pc.planDropTable(s)
	case *ast.SelectStmt:
		return pc.planSelect(s)
	case *ast.InsertStmt:
		return pc.planInsert(s)
	case *ast.UpdateStmt:
		return pc.planUpdate(s)
	case *ast.DeleteStmt:
		return pc.planDelete(s)
	default:
		return nil, fmt.Errorf("planner: statement type %T is not yet supported", stmt)
	}
}
