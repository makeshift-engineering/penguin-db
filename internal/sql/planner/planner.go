// Package planner turns a parsed *ast.Program into a logical plan tree,
// resolving every name against the catalog and attaching the type
// information execution needs. It reads the catalog but never writes to
// it — DDL execution (Build*Ops, WriteBatch, Apply*) belongs to the
// executor, not here.
package planner

import (
	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
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
