package planner

import (
	"errors"
	"fmt"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

// ResolvedColumn is the bridge artifact between a catalog schema and the
// executor: it carries everything execution needs to read a value out of a
// decoded row without ever consulting the catalog again. Index matches the
// column's position among the table's active (non-dropped) columns, which
// is exactly the order the row codec decodes values in.
type ResolvedColumn struct {
	Database   string
	Table      string
	Name       string
	Index      int
	Type       ast.DataTypeKind
	VarcharLen *int
	Nullable   bool
}

// String returns a fully qualified "db.table.column" representation for
// debugging and logging.
func (c *ResolvedColumn) String() string {
	return fmt.Sprintf("%s.%s.%s", c.Database, c.Table, c.Name)
}

// ResolvedTable snapshots a table's catalog schema at plan time, plus the
// binding name (alias, or the table name itself) it is addressed by within
// a single query's scope.
type ResolvedTable struct {
	Database string
	Table    string
	Binding  string
	Schema   *catalog.TableMeta

	columns []*ResolvedColumn
	byName  map[string]*ResolvedColumn
}

// resolvedColumnFrom builds a ResolvedColumn for a single column at a known
// index within meta. Factored out of newResolvedTable so any code that
// needs a ResolvedColumn without going through a full Scope — INSERT's
// column-list resolution, in particular — can build one the same way
// instead of duplicating the field mapping.
func resolvedColumnFrom(meta *catalog.TableMeta, col catalog.ColumnMeta, index int) *ResolvedColumn {
	return &ResolvedColumn{
		Database:   meta.Database,
		Table:      meta.Name,
		Name:       col.Name,
		Index:      index,
		Type:       col.Type,
		VarcharLen: col.VarcharLen,
		Nullable:   !col.NotNull,
	}
}

// newResolvedTable builds a ResolvedTable from a catalog schema, computing
// the column index and name lookup once so later lookups are O(1).
func newResolvedTable(meta *catalog.TableMeta, binding string) *ResolvedTable {
	active := meta.ActiveColumns()
	columns := make([]*ResolvedColumn, len(active))
	byName := make(map[string]*ResolvedColumn, len(active))

	for i, col := range active {
		rc := resolvedColumnFrom(meta, col, i)
		columns[i] = rc
		byName[col.Name] = rc
	}

	return &ResolvedTable{
		Database: meta.Database,
		Table:    meta.Name,
		Binding:  binding,
		Schema:   meta,
		columns:  columns,
		byName:   byName,
	}
}

// Columns returns every active column of the table, in declaration order.
func (rt *ResolvedTable) Columns() []*ResolvedColumn {
	return rt.columns
}

// findColumn looks up a column by its bare name within this table only.
// Returns nil if the table has no active column with that name.
func (rt *ResolvedTable) findColumn(name string) *ResolvedColumn {
	return rt.byName[name]
}

// Scope is the set of tables visible to column resolution within a single
// query (or, once subqueries exist, a single query level). It is built once
// from a FROM clause and then used to resolve every column reference in the
// SELECT list, WHERE, GROUP BY, HAVING, and ORDER BY.
type Scope struct {
	tables    []*ResolvedTable
	byBinding map[string]*ResolvedTable
	byColumn  map[string][]*ResolvedTable
}

func newScope() *Scope {
	return &Scope{
		byBinding: make(map[string]*ResolvedTable),
		byColumn:  make(map[string][]*ResolvedTable),
	}
}

// addTable registers a resolved table under its binding name. It returns
// ErrDuplicateTableBinding if that name is already taken in this scope —
// e.g. two unaliased references to the same table, or two tables sharing
// an alias.
func (s *Scope) addTable(rt *ResolvedTable) error {
	if _, exists := s.byBinding[rt.Binding]; exists {
		return ErrDuplicateTableBinding
	}
	s.tables = append(s.tables, rt)
	s.byBinding[rt.Binding] = rt
	for _, col := range rt.columns {
		s.byColumn[col.Name] = append(s.byColumn[col.Name], rt)
	}
	return nil
}

// Tables returns every table registered in this scope, in FROM-clause order.
func (s *Scope) Tables() []*ResolvedTable {
	return s.tables
}

// resolveDatabaseName returns the database an identifier resolves against:
// the qualifier itself if one was given, otherwise the session's active
// database. Returns a CodeNoActiveDatabase diagnostic if neither is set.
func (pc *planContext) resolveDatabaseName(qualifier string, span ast.Node) (string, error) {
	if qualifier != "" {
		return qualifier, nil
	}
	if pc.session.ActiveDatabase != "" {
		return pc.session.ActiveDatabase, nil
	}
	return "", pc.errorf(
		span.Span(), CodeNoActiveDatabase,
		"no database selected: qualify the name or run USE first",
	)
}

// resolveTableIdentifier resolves a table-position identifier — as used in
// FROM, INSERT/UPDATE/DELETE targets, and CREATE/ALTER/DROP TABLE — against
// the catalog. id.Qualifier, when present, names the database; when absent,
// the session's active database is used.
func (pc *planContext) resolveTableIdentifier(id *ast.Identifier) (*catalog.TableMeta, string, error) {
	db, err := pc.resolveDatabaseName(id.Qualifier, id)
	if err != nil {
		return nil, "", err
	}

	meta, err := pc.catalog.GetTable(db, id.Name)
	if err != nil {
		switch {
		case errors.Is(err, catalog.ErrDatabaseNotFound):
			return nil, "", pc.errorf(id.Span(), CodeUnknownDatabase, "database %q does not exist", db)
		case errors.Is(err, catalog.ErrTableNotFound):
			return nil, "", pc.errorf(id.Span(), CodeUnknownTable, "table %q does not exist in database %q", id.Name, db)
		default:
			return nil, "", pc.errorf(id.Span(), CodeUnknownTable, "resolving table %q: %v", id.Name, err)
		}
	}
	return meta, db, nil
}

// resolveColumn resolves a column-position identifier against scope. The
// grammar's QualifiedIdentifier is at most two parts (Identifier ['.'
// Identifier]), so a column reference's qualifier — when present — always
// names a table or alias in scope, never a database; database-qualified
// names only occur in table position (see resolveTableIdentifier).
func (pc *planContext) resolveColumn(scope *Scope, id *ast.Identifier) (*ResolvedColumn, error) {
	if id.Qualifier != "" {
		table, ok := scope.byBinding[id.Qualifier]
		if !ok {
			return nil, pc.errorf(id.Span(), CodeUnknownTableBinding, "unknown table or alias %q", id.Qualifier)
		}
		col := table.findColumn(id.Name)
		if col == nil {
			return nil, pc.errorf(id.Span(), CodeUnknownColumn, "column %q not found on %q", id.Name, id.Qualifier)
		}
		return col, nil
	}

	candidates := scope.byColumn[id.Name]
	switch len(candidates) {
	case 0:
		return nil, pc.errorf(id.Span(), CodeUnknownColumn, "unknown column %q", id.Name)
	case 1:
		return candidates[0].findColumn(id.Name), nil
	default:
		return nil, pc.errorf(
			id.Span(), CodeAmbiguousColumn,
			"column %q is ambiguous between %d tables in this query; qualify it", id.Name, len(candidates),
		)
	}
}
