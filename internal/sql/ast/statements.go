package ast

import "errors"

// CreateDatabaseStmt represents: CREATE DATABASE [IF NOT EXISTS] Name.
type CreateDatabaseStmt struct {
	StmtBase
	Name        string
	IfNotExists bool
}

// UseDatabaseStmt represents: USE Name.
type UseDatabaseStmt struct {
	StmtBase
	Name string
}

// DropDatabaseStmt represents: DROP DATABASE [IF EXISTS] Name.
type DropDatabaseStmt struct {
	StmtBase
	Name     string
	IfExists bool
}

// CreateTableStmt represents: CREATE TABLE [IF NOT EXISTS] Table ( Columns... ).
type CreateTableStmt struct {
	StmtBase
	Table       *Identifier
	IfNotExists bool
	Columns     []*ColumnDef
}

// AlterTableStmt represents: ALTER TABLE Table Action.
type AlterTableStmt struct {
	StmtBase
	Table  *Identifier
	Action *AlterAction
}

// DropTableStmt represents: DROP TABLE [IF EXISTS] Table.
type DropTableStmt struct {
	StmtBase
	Table    *Identifier
	IfExists bool
}

// SelectStmt represents a full SELECT query. From is nil when the query
// has no FROM clause (e.g. SELECT 1+1). Distinct and All are mutually
// exclusive; both false means neither modifier was specified.
type SelectStmt struct {
	StmtBase
	Distinct bool
	All      bool
	Columns  []*SelectColumn
	From     []*TableRef
	Where    *WhereClause
	GroupBy  *GroupByClause
	Having   *HavingClause
	OrderBy  *OrderByClause
	Limit    *LimitClause
}

// InsertStmt represents an INSERT statement. Exactly one of Rows or Source
// is non-nil: Rows for INSERT ... VALUES, Source for INSERT ... SELECT.
type InsertStmt struct {
	StmtBase
	Table   *Identifier
	Columns []string
	Rows    [][]*SelectExpression
	Source  *SelectStmt
}

func (i *InsertStmt) Validate() error {
	if (i.Rows == nil) == (i.Source == nil) {
		return errors.New("InsertStmt must have exactly one of Rows or Source set")
	}
	return nil
}

// UpdateStmt represents: UPDATE Table SET assignments [WHERE cond].
type UpdateStmt struct {
	StmtBase
	Table *Identifier
	Set   []*SetItem
	Where *WhereClause
}

// DeleteStmt represents: DELETE FROM Table [WHERE cond].
type DeleteStmt struct {
	StmtBase
	Table *Identifier
	Where *WhereClause
}
