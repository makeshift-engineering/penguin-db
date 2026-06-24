package ast

type CreateDatabaseStmt struct {
	StmtBase
	Name        string
	IfNotExists bool
}

type UseDatabaseStmt struct {
	StmtBase
	Name string
}

type DropDatabaseStmt struct {
	StmtBase
	Name     string
	IfExists bool
}

type CreateTableStmt struct {
	StmtBase
	Table       *Identifier
	IfNotExists bool
	Columns     []*ColumnDef
}

type AlterTableStmt struct {
	StmtBase
	Table  *Identifier
	Action *AlterAction
}

type DropTableStmt struct {
	StmtBase
	Table    *Identifier
	IfExists bool
}

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

type InsertStmt struct {
	StmtBase
	Table   *Identifier
	Columns []string
	Rows    [][]*SelectExpression
	Source  *SelectStmt
}

type UpdateStmt struct {
	StmtBase
	Table *Identifier
	Set   []*SetItem
	Where *WhereClause
}

type DeleteStmt struct {
	StmtBase
	Table *Identifier
	Where *WhereClause
}
