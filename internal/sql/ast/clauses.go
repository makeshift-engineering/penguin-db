package ast

type DataTypeKind int

const (
	TypeInt DataTypeKind = iota
	TypeBigInt
	TypeVarchar
	TypeBoolean
	TypeText
	TypeTimestamp
)

type DataType struct {
	ClauseBase
	Kind       DataTypeKind
	VarcharLen *int
}

type ColumnDef struct {
	ClauseBase
	Name        string
	Type        *DataType
	Constraints *ColumnConstraints
}

type ColumnConstraints struct {
	ClauseBase
	PrimaryKey bool
	Unique     bool
	NotNull    bool
	Null       bool
	Default    *SignedLiteral
	References *ForeignRef
}

type SignedLiteral struct {
	ClauseBase
	Negative bool
	Value    Expression
}

type ForeignRef struct {
	ClauseBase
	Table  string
	Column string
}

type AlterActionKind int

const (
	AlterAdd AlterActionKind = iota
	AlterModify
	AlterRenameTable
	AlterRenameColumn
	AlterDropColumn
)

type AlterAction struct {
	ClauseBase
	Kind     AlterActionKind
	Column   *ColumnDef
	OldName  string
	NewName  string
	DropName string
}

type SelectColumn struct {
	ClauseBase
	Star          bool
	QualifiedStar *Identifier
	Expr          *SelectExpression
	Alias         string
}

type TableRef struct {
	ClauseBase
	Primary *TablePrimary
	Joins   []*JoinClause
	Paren   *TableRef
}

type TablePrimary struct {
	ClauseBase
	Name  *Identifier
	Alias string
}

type JoinType int

const (
	JoinInner JoinType = iota
	JoinLeft
	JoinRight
	JoinFull
	JoinCross
)

type JoinClause struct {
	ClauseBase
	Type  JoinType
	Right *TablePrimary
	On    Condition
}

type WhereClause struct {
	ClauseBase
	Cond Condition
}

type GroupByClause struct {
	ClauseBase
	Columns []*Identifier
}

type HavingClause struct {
	ClauseBase
	Cond Condition
}

type OrderDirection int

const (
	OrderAsc OrderDirection = iota
	OrderDesc
)

type OrderByClause struct {
	ClauseBase
	Items []*OrderByItem
}

type OrderByItem struct {
	ClauseBase
	Expr      Expression
	Direction OrderDirection
}

type LimitClause struct {
	ClauseBase
	Count  int
	Offset *int
}

type SetItem struct {
	ClauseBase
	Column *Identifier
	Value  Expression
}
