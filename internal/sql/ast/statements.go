package ast

// CreateDatabaseStmt represents: CREATE DATABASE [IF NOT EXISTS] Name.
type CreateDatabaseStmt struct {
	StmtBase
	Name        string
	IfNotExists bool
}

func (c *CreateDatabaseStmt) Validate() error {
	if c.Name == "" {
		return ErrEmptyDatabaseName
	}
	return nil
}

// UseDatabaseStmt represents: USE Name.
type UseDatabaseStmt struct {
	StmtBase
	Name string
}

func (u *UseDatabaseStmt) Validate() error {
	if u.Name == "" {
		return ErrEmptyDatabaseName
	}
	return nil
}

// DropDatabaseStmt represents: DROP DATABASE [IF EXISTS] Name.
type DropDatabaseStmt struct {
	StmtBase
	Name     string
	IfExists bool
}

func (d *DropDatabaseStmt) Validate() error {
	if d.Name == "" {
		return ErrEmptyDatabaseName
	}
	return nil
}

// CreateTableStmt represents: CREATE TABLE [IF NOT EXISTS] Table ( Columns... ).
type CreateTableStmt struct {
	StmtBase
	Table       *Identifier
	IfNotExists bool
	Columns     []*ColumnDef
}

func (c *CreateTableStmt) Validate() error {
	if c.Table == nil {
		return ErrNilIdentifier
	}
	if err := c.Table.Validate(); err != nil {
		return err
	}
	if len(c.Columns) == 0 {
		return ErrEmptyCreateTableColumns
	}
	for _, col := range c.Columns {
		if col == nil {
			return ErrNilClause
		}
		if err := col.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// AlterTableStmt represents: ALTER TABLE Table Action.
type AlterTableStmt struct {
	StmtBase
	Table  *Identifier
	Action *AlterAction
}

func (a *AlterTableStmt) Validate() error {
	if a.Table == nil {
		return ErrNilIdentifier
	}
	if a.Action == nil {
		return ErrNilAlterTableAction
	}
	if err := a.Table.Validate(); err != nil {
		return err
	}
	return a.Action.Validate()
}

// DropTableStmt represents: DROP TABLE [IF EXISTS] Table.
type DropTableStmt struct {
	StmtBase
	Table    *Identifier
	IfExists bool
}

func (d *DropTableStmt) Validate() error {
	if d.Table == nil {
		return ErrNilIdentifier
	}
	return d.Table.Validate()
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

func (s *SelectStmt) Validate() error {
	if s.Distinct && s.All {
		return ErrMutuallyExclusiveSelectModifiers
	}
	if len(s.Columns) == 0 {
		return ErrEmptySelectColumns
	}
	for _, col := range s.Columns {
		if col == nil {
			return ErrNilClause
		}
		if err := col.Validate(); err != nil {
			return err
		}
	}
	for _, table := range s.From {
		if table == nil {
			return ErrNilClause
		}
		if err := table.Validate(); err != nil {
			return err
		}
	}
	if s.Where != nil {
		if err := s.Where.Validate(); err != nil {
			return err
		}
	}
	if s.GroupBy != nil {
		if err := s.GroupBy.Validate(); err != nil {
			return err
		}
	}
	if s.Having != nil {
		if err := s.Having.Validate(); err != nil {
			return err
		}
	}
	if s.OrderBy != nil {
		if err := s.OrderBy.Validate(); err != nil {
			return err
		}
	}
	if s.Limit != nil {
		if err := s.Limit.Validate(); err != nil {
			return err
		}
	}
	return nil
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
	if i.Table == nil {
		return ErrNilIdentifier
	}
	if err := i.Table.Validate(); err != nil {
		return err
	}
	if (len(i.Rows) == 0) == (i.Source == nil) {
		return ErrInvalidInsertStmt
	}
	if i.Source != nil {
		return i.Source.Validate()
	}
	for _, row := range i.Rows {
		for _, val := range row {
			if val == nil {
				return ErrNilExpression
			}
			if err := val.Validate(); err != nil {
				return err
			}
		}
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

func (u *UpdateStmt) Validate() error {
	if u.Table == nil {
		return ErrNilIdentifier
	}
	if err := u.Table.Validate(); err != nil {
		return err
	}
	if len(u.Set) == 0 {
		return ErrEmptyUpdateAssignments
	}
	for _, assignment := range u.Set {
		if assignment == nil {
			return ErrNilClause
		}
		if err := assignment.Validate(); err != nil {
			return err
		}
	}
	if u.Where != nil {
		return u.Where.Validate()
	}
	return nil
}

// DeleteStmt represents: DELETE FROM Table [WHERE cond].
type DeleteStmt struct {
	StmtBase
	Table *Identifier
	Where *WhereClause
}

func (d *DeleteStmt) Validate() error {
	if d.Table == nil {
		return ErrNilIdentifier
	}
	if err := d.Table.Validate(); err != nil {
		return err
	}
	if d.Where != nil {
		return d.Where.Validate()
	}
	return nil
}
