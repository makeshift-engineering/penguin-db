package ast

// DataTypeKind enumerates the SQL data types supported by the grammar.
type DataTypeKind int

const (
	TypeInt       DataTypeKind = iota // INT
	TypeBigInt                        // BIGINT
	TypeVarchar                       // VARCHAR(n)
	TypeBoolean                       // BOOLEAN
	TypeText                          // TEXT
	TypeTimestamp                     // TIMESTAMP
)

// DataType represents a column's SQL data type. VarcharLen is non-nil
// only when Kind is [TypeVarchar].
type DataType struct {
	ClauseBase
	Kind       DataTypeKind
	VarcharLen *int
}

func (d *DataType) Validate() error {
	if d.Kind == TypeVarchar {
		if d.VarcharLen == nil {
			return ErrVarcharLengthRequired
		}
		if *d.VarcharLen <= 0 {
			return ErrVarcharLengthInvalid
		}
	} else {
		if d.VarcharLen != nil {
			return ErrLengthNotSupported
		}
	}
	return nil
}

// ColumnDef represents a single column definition within a CREATE TABLE
// statement: Name DataType [Constraints].
type ColumnDef struct {
	ClauseBase
	Name        string
	Type        *DataType
	Constraints []Clause
}

func (c *ColumnDef) Validate() error {
	if c.Name == "" {
		return ErrEmptyIdentifierName
	}
	if c.Type == nil {
		return ErrNilColumnType
	}
	if err := c.Type.Validate(); err != nil {
		return err
	}
	for _, constr := range c.Constraints {
		if constr == nil {
			return ErrNilClause
		}
		if err := constr.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// PrimaryKeyConstraint represents a PRIMARY KEY constraint.
type PrimaryKeyConstraint struct {
	ClauseBase
}

// UniqueConstraint represents a UNIQUE constraint.
type UniqueConstraint struct {
	ClauseBase
}

// NotNullConstraint represents a NOT NULL constraint.
type NotNullConstraint struct {
	ClauseBase
}

// NullConstraint represents a NULL constraint.
type NullConstraint struct {
	ClauseBase
}

// DefaultConstraint represents a DEFAULT constraint with a signed literal.
type DefaultConstraint struct {
	ClauseBase
	Value *SignedLiteral
}

func (d *DefaultConstraint) Validate() error {
	if d.Value == nil {
		return ErrNilDefaultValue
	}
	return d.Value.Validate()
}

// ReferencesConstraint represents a foreign key REFERENCE constraint.
type ReferencesConstraint struct {
	ClauseBase
	Table  string
	Column string
}

func (r *ReferencesConstraint) Validate() error {
	if r.Table == "" {
		return ErrEmptyReferencesTable
	}
	if r.Column == "" {
		return ErrEmptyReferencesColumn
	}
	return nil
}

// SignedLiteral represents a literal value optionally preceded by a sign.
// Used in DEFAULT constraints (e.g. DEFAULT -1, DEFAULT 'hello').
type SignedLiteral struct {
	ClauseBase
	Negative bool
	Value    Expression
}

func (s *SignedLiteral) Validate() error {
	if s.Value == nil {
		return ErrNilExpression
	}
	return s.Value.Validate()
}

// ForeignRef represents a column-level foreign key reference:
// REFERENCES Table ( Column ).
type ForeignRef struct {
	ClauseBase
	Table  string
	Column string
}

func (f *ForeignRef) Validate() error {
	if f.Table == "" {
		return ErrEmptyForeignTable
	}
	if f.Column == "" {
		return ErrEmptyForeignColumn
	}
	return nil
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

func (a *AlterAction) Validate() error {
	switch a.Kind {
	case AlterAdd, AlterModify:
		if a.Column == nil {
			return ErrNilColumnDefinition
		}
		return a.Column.Validate()
	case AlterRenameTable:
		if a.NewName == "" {
			return ErrEmptyNewTableName
		}
	case AlterRenameColumn:
		if a.OldName == "" || a.NewName == "" {
			return ErrEmptyOldOrNewColumnName
		}
	case AlterDropColumn:
		if a.DropName == "" {
			return ErrEmptyDropColumnName
		}
	default:
		return ErrInvalidAlterAction
	}
	return nil
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

func (s *SelectColumn) Validate() error {
	count := 0
	if s.Star {
		count++
	}
	if s.QualifiedStar != nil {
		count++
	}
	if s.Expr != nil {
		count++
	}
	if count != 1 {
		return ErrInvalidSelectColumn
	}
	if s.QualifiedStar != nil {
		if err := s.QualifiedStar.Validate(); err != nil {
			return err
		}
	}
	if s.Expr != nil {
		if err := s.Expr.Validate(); err != nil {
			return err
		}
	}
	return nil
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

func (t *TableRef) Validate() error {
	if (t.Primary == nil) == (t.Paren == nil) {
		return ErrInvalidTableRef
	}
	if t.Paren != nil {
		if len(t.Joins) > 0 {
			return ErrTableRefParenJoins
		}
		return t.Paren.Validate()
	}
	if err := t.Primary.Validate(); err != nil {
		return err
	}
	for _, join := range t.Joins {
		if join == nil {
			return ErrNilClause
		}
		if err := join.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// TablePrimary is a named table with an optional alias: Name [[AS] Alias].
type TablePrimary struct {
	ClauseBase
	Name  *Identifier
	Alias string
}

func (t *TablePrimary) Validate() error {
	if t.Name == nil {
		return ErrNilIdentifier
	}
	return t.Name.Validate()
}

// JoinType enumerates the supported JOIN variants.
type JoinType int

const (
	JoinInner JoinType = iota // [INNER] JOIN
	JoinLeft                  // LEFT [OUTER] JOIN
	JoinRight                 // RIGHT [OUTER] JOIN
	JoinFull                  // FULL [OUTER] JOIN
	JoinCross                 // CROSS JOIN
)

// JoinClause represents a single JOIN operation chained after a [TablePrimary].
// On is nil for CROSS JOIN, which produces a cartesian product.
type JoinClause struct {
	ClauseBase
	Type  JoinType
	Right *TablePrimary
	On    Condition
}

func (j *JoinClause) Validate() error {
	if j.Right == nil {
		return ErrNilJoinRightTable
	}
	if err := j.Right.Validate(); err != nil {
		return err
	}
	if j.Type == JoinCross {
		if j.On != nil {
			return ErrCrossJoinWithOn
		}
	} else {
		if j.On == nil {
			return ErrNonCrossJoinWithoutOn
		}
		if err := j.On.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// WhereClause represents: WHERE Cond.
type WhereClause struct {
	ClauseBase
	Cond Condition
}

func (w *WhereClause) Validate() error {
	if w.Cond == nil {
		return ErrNilCondition
	}
	return w.Cond.Validate()
}

// GroupByClause represents: GROUP BY col1, col2, ...
type GroupByClause struct {
	ClauseBase
	Columns []*Identifier
}

func (g *GroupByClause) Validate() error {
	if len(g.Columns) == 0 {
		return ErrEmptyGroupBy
	}
	for _, col := range g.Columns {
		if col == nil {
			return ErrNilIdentifier
		}
		if err := col.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// HavingClause represents: HAVING Cond.
type HavingClause struct {
	ClauseBase
	Cond Condition
}

func (h *HavingClause) Validate() error {
	if h.Cond == nil {
		return ErrNilCondition
	}
	return h.Cond.Validate()
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

func (o *OrderByClause) Validate() error {
	if len(o.Items) == 0 {
		return ErrEmptyOrderBy
	}
	for _, item := range o.Items {
		if item == nil {
			return ErrNilClause
		}
		if err := item.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// OrderByItem represents a single ordering term: Expr [ASC|DESC].
type OrderByItem struct {
	ClauseBase
	Expr      Expression
	Direction OrderDirection
}

func (o *OrderByItem) Validate() error {
	if o.Expr == nil {
		return ErrNilExpression
	}
	return o.Expr.Validate()
}

// LimitClause represents: LIMIT Count [OFFSET Offset].
// Offset is nil when no OFFSET is specified.
type LimitClause struct {
	ClauseBase
	Count  int
	Offset *int
}

func (l *LimitClause) Validate() error {
	if l.Count < 0 {
		return ErrNegativeLimitCount
	}
	if l.Offset != nil && *l.Offset < 0 {
		return ErrNegativeLimitOffset
	}
	return nil
}

// SetItem represents a single assignment in an UPDATE SET clause:
// Column = Value.
type SetItem struct {
	ClauseBase
	Column *Identifier
	Value  Expression
}

func (s *SetItem) Validate() error {
	if s.Column == nil {
		return ErrNilIdentifier
	}
	if s.Value == nil {
		return ErrNilExpression
	}
	if err := s.Column.Validate(); err != nil {
		return err
	}
	return s.Value.Validate()
}
