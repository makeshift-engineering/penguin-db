package ast

// DataTypeKind enumerates the SQL data types supported by the grammar.
type DataTypeKind int

const (
	TypeInt       DataTypeKind = iota // INT
	TypeBigInt                        // BIGINT
	TypeVarchar                       // VARCHAR(n)
	TypeBoolean                       // BOOLEAN
	TypeText                          // TEXT
	TypeTimestamp                      // TIMESTAMP
)

// DataType represents a column's SQL data type. VarcharLen is non-nil
// only when Kind is [TypeVarchar].
type DataType struct {
	ClauseBase
	Kind       DataTypeKind
	VarcharLen *int
}

// ColumnDef represents a single column definition within a CREATE TABLE
// statement: Name DataType [Constraints].
type ColumnDef struct {
	ClauseBase
	Name        string
	Type        *DataType
	Constraints *ColumnConstraints
}

// ColumnConstraints holds the optional constraints for a column definition.
// The grammar allows these in any order; duplicate and semantic validation
// is deferred to analysis time.
type ColumnConstraints struct {
	ClauseBase
	PrimaryKey bool
	Unique     bool
	NotNull    bool
	Null       bool
	Default    *SignedLiteral
	References *ForeignRef
}

// SignedLiteral represents a literal value optionally preceded by a sign.
// Used in DEFAULT constraints (e.g. DEFAULT -1, DEFAULT 'hello').
type SignedLiteral struct {
	ClauseBase
	Negative bool
	Value    Expression
}

// ForeignRef represents a column-level foreign key reference:
// REFERENCES Table ( Column ).
type ForeignRef struct {
	ClauseBase
	Table  string
	Column string
}

// AlterActionKind identifies which ALTER TABLE variant is being used.
type AlterActionKind int

const (
	AlterAdd          AlterActionKind = iota // ADD [COLUMN] ColumnDef
	AlterModify                              // MODIFY [COLUMN] ColumnDef
	AlterRenameTable                         // RENAME TO NewName
	AlterRenameColumn                        // RENAME COLUMN OldName TO NewName
	AlterDropColumn                          // DROP COLUMN DropName
)

// AlterAction represents the action clause in an ALTER TABLE statement.
// Only the fields relevant to Kind are populated.
type AlterAction struct {
	ClauseBase
	Kind     AlterActionKind
	Column   *ColumnDef
	OldName  string
	NewName  string
	DropName string
}

// SelectColumn represents one item in a SELECT column list. Exactly one of
// the following is set:
//   - Star=true for a bare *.
//   - QualifiedStar non-nil for table.* or db.table.*.
//   - Expr non-nil for an expression, optionally with an AS Alias.
type SelectColumn struct {
	ClauseBase
	Star          bool
	QualifiedStar *Identifier
	Expr          *SelectExpression
	Alias         string
}

// TableRef represents a table reference in a FROM clause. Either
// Primary+Joins is used (a named table with optional joins) or Paren is
// used (a parenthesized sub-reference). These are mutually exclusive.
type TableRef struct {
	ClauseBase
	Primary *TablePrimary
	Joins   []*JoinClause
	Paren   *TableRef
}

// TablePrimary is a named table with an optional alias: Name [[AS] Alias].
type TablePrimary struct {
	ClauseBase
	Name  *Identifier
	Alias string
}

// JoinType enumerates the supported JOIN variants.
type JoinType int

const (
	JoinInner JoinType = iota // [INNER] JOIN
	JoinLeft                   // LEFT [OUTER] JOIN
	JoinRight                  // RIGHT [OUTER] JOIN
	JoinFull                   // FULL [OUTER] JOIN
	JoinCross                  // CROSS JOIN
)

// JoinClause represents a single JOIN operation chained after a [TablePrimary].
// On is nil for CROSS JOIN, which produces a cartesian product.
type JoinClause struct {
	ClauseBase
	Type  JoinType
	Right *TablePrimary
	On    Condition
}

// WhereClause represents: WHERE Cond.
type WhereClause struct {
	ClauseBase
	Cond Condition
}

// GroupByClause represents: GROUP BY col1, col2, ...
type GroupByClause struct {
	ClauseBase
	Columns []*Identifier
}

// HavingClause represents: HAVING Cond.
type HavingClause struct {
	ClauseBase
	Cond Condition
}

// OrderDirection indicates ascending or descending sort order.
type OrderDirection int

const (
	OrderAsc  OrderDirection = iota // ASC (default)
	OrderDesc                       // DESC
)

// OrderByClause represents: ORDER BY item1, item2, ...
type OrderByClause struct {
	ClauseBase
	Items []*OrderByItem
}

// OrderByItem represents a single ordering term: Expr [ASC|DESC].
type OrderByItem struct {
	ClauseBase
	Expr      Expression
	Direction OrderDirection
}

// LimitClause represents: LIMIT Count [OFFSET Offset].
// Offset is nil when no OFFSET is specified.
type LimitClause struct {
	ClauseBase
	Count  int
	Offset *int
}

// SetItem represents a single assignment in an UPDATE SET clause:
// Column = Value.
type SetItem struct {
	ClauseBase
	Column *Identifier
	Value  Expression
}
