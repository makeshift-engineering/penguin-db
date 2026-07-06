package ast

import "errors"

var (
	// ErrInvalidSelectExpression is returned when a SelectExpression does not have exactly one of Expr or Cond set.
	ErrInvalidSelectExpression = errors.New("SelectExpression must have exactly one of Expr or Cond set")

	// ErrInvalidInsertStmt is returned when an InsertStmt does not have exactly one of Rows or Source set.
	ErrInvalidInsertStmt = errors.New("InsertStmt must have exactly one of Rows or Source set")

	// ErrNilStatement is returned when a statement in the program is nil.
	ErrNilStatement = errors.New("Statement cannot be nil")

	// ErrNilExpression is returned when a required expression is nil.
	ErrNilExpression = errors.New("Expression cannot be nil")

	// ErrNilCondition is returned when a required condition is nil.
	ErrNilCondition = errors.New("Condition cannot be nil")

	// ErrNilClause is returned when a required clause is nil.
	ErrNilClause = errors.New("Clause cannot be nil")

	// ErrNilIdentifier is returned when a required identifier is nil.
	ErrNilIdentifier = errors.New("Identifier cannot be nil")

	// ErrInvalidBinaryOperator is returned when a BinaryExpr's operator is not a valid arithmetic operator.
	ErrInvalidBinaryOperator = errors.New("invalid binary operator")

	// ErrInvalidUnaryOperator is returned when a UnaryExpr's operator is not a valid unary operator.
	ErrInvalidUnaryOperator = errors.New("invalid unary operator")

	// ErrInvalidComparisonOperator is returned when a ComparisonPredicate's operator is not a valid comparison operator.
	ErrInvalidComparisonOperator = errors.New("invalid comparison operator")

	// ErrInvalidConditionOperator is returned when a BinaryCondition's operator is not a valid logical operator (AND, OR).
	ErrInvalidConditionOperator = errors.New("invalid condition operator (must be AND or OR)")

	// ErrStarFunctionArgs is returned when a function call has Star == true and non-empty arguments.
	ErrStarFunctionArgs = errors.New("functions with Star (e.g. COUNT(*)) must not have arguments")

	// ErrEmptyFunctionName is returned when a function name is empty.
	ErrEmptyFunctionName = errors.New("function name cannot be empty")

	// ErrEmptyIdentifierName is returned when an identifier name is empty.
	ErrEmptyIdentifierName = errors.New("identifier name cannot be empty")

	// ErrEmptyDatabaseName is returned when a database name is empty.
	ErrEmptyDatabaseName = errors.New("database name cannot be empty")

	// ErrVarcharLengthRequired is returned when VARCHAR type has no length specified.
	ErrVarcharLengthRequired = errors.New("VARCHAR length must be specified")

	// ErrVarcharLengthInvalid is returned when VARCHAR type length is less than or equal to zero.
	ErrVarcharLengthInvalid = errors.New("VARCHAR length must be greater than zero")

	// ErrLengthNotSupported is returned when column length is specified on a non-VARCHAR type.
	ErrLengthNotSupported = errors.New("length only supported for VARCHAR")

	// ErrDecimalPrecisionInvalid is returned when DECIMAL precision is less than or equal to zero.
	ErrDecimalPrecisionInvalid = errors.New("DECIMAL precision must be greater than zero")

	// ErrDecimalScaleInvalid is returned when DECIMAL scale is negative or greater than precision.
	ErrDecimalScaleInvalid = errors.New("DECIMAL scale must be greater than or equal to zero and less than or equal to precision")

	// ErrDecimalParamsNotSupported is returned when decimal parameters are specified on a non-DECIMAL type.
	ErrDecimalParamsNotSupported = errors.New("precision and scale only supported for DECIMAL")

	// ErrNilColumnType is returned when a column definition has a nil data type.
	ErrNilColumnType = errors.New("column type cannot be nil")

	// ErrNilDefaultValue is returned when a DEFAULT constraint has a nil value.
	ErrNilDefaultValue = errors.New("default value cannot be nil")

	// ErrEmptyReferencesTable is returned when a References constraint has an empty table name.
	ErrEmptyReferencesTable = errors.New("references table name cannot be empty")

	// ErrEmptyReferencesColumn is returned when a References constraint has an empty column name.
	ErrEmptyReferencesColumn = errors.New("references column name cannot be empty")

	// ErrEmptyForeignTable is returned when a foreign reference has an empty table name.
	ErrEmptyForeignTable = errors.New("foreign table name cannot be empty")

	// ErrEmptyForeignColumn is returned when a foreign reference has an empty column name.
	ErrEmptyForeignColumn = errors.New("foreign column name cannot be empty")

	// ErrNilColumnDefinition is returned when ALTER TABLE ADD/MODIFY has a nil column definition.
	ErrNilColumnDefinition = errors.New("column definition cannot be nil for ADD/MODIFY")

	// ErrEmptyNewTableName is returned when ALTER TABLE RENAME TO has an empty new table name.
	ErrEmptyNewTableName = errors.New("new table name cannot be empty")

	// ErrEmptyOldOrNewColumnName is returned when ALTER TABLE RENAME COLUMN has empty old or new column name.
	ErrEmptyOldOrNewColumnName = errors.New("old and new column names cannot be empty")

	// ErrEmptyDropColumnName is returned when ALTER TABLE DROP COLUMN has an empty column name.
	ErrEmptyDropColumnName = errors.New("drop column name cannot be empty")

	// ErrInvalidAlterAction is returned when an ALTER TABLE action is invalid.
	ErrInvalidAlterAction = errors.New("invalid alter action kind")

	// ErrInvalidSelectColumn is returned when a SelectColumn does not specify exactly one of Star, QualifiedStar, or Expr.
	ErrInvalidSelectColumn = errors.New("SelectColumn must specify exactly one of Star, QualifiedStar, or Expr")

	// ErrInvalidTableRef is returned when a TableRef does not specify exactly one of Primary or Paren.
	ErrInvalidTableRef = errors.New("TableRef must have exactly one of Primary or Paren set")

	// ErrTableRefParenJoins is returned when a parenthesized TableRef has Joins.
	ErrTableRefParenJoins = errors.New("TableRef with Paren cannot have Joins")

	// ErrNilJoinRightTable is returned when a Join clause has a nil right table.
	ErrNilJoinRightTable = errors.New("join right table cannot be nil")

	// ErrCrossJoinWithOn is returned when a CROSS JOIN has an ON condition.
	ErrCrossJoinWithOn = errors.New("CROSS JOIN cannot have an ON condition")

	// ErrNonCrossJoinWithoutOn is returned when an ON condition is missing from a non-cross JOIN.
	ErrNonCrossJoinWithoutOn = errors.New("non-cross JOIN must have an ON condition")

	// ErrEmptyGroupBy is returned when a GROUP BY clause specifies no columns.
	ErrEmptyGroupBy = errors.New("GROUP BY clause must specify at least one column")

	// ErrEmptyOrderBy is returned when an ORDER BY clause specifies no items.
	ErrEmptyOrderBy = errors.New("ORDER BY clause must specify at least one item")

	// ErrNegativeLimitCount is returned when LIMIT count is negative.
	ErrNegativeLimitCount = errors.New("LIMIT count cannot be negative")

	// ErrNegativeLimitOffset is returned when LIMIT offset is negative.
	ErrNegativeLimitOffset = errors.New("LIMIT offset cannot be negative")

	// ErrEmptyCreateTableColumns is returned when CREATE TABLE specifies no columns.
	ErrEmptyCreateTableColumns = errors.New("CREATE TABLE must specify at least one column")

	// ErrNilAlterTableAction is returned when ALTER TABLE action is nil.
	ErrNilAlterTableAction = errors.New("ALTER TABLE action cannot be nil")

	// ErrMutuallyExclusiveSelectModifiers is returned when SELECT specifies both DISTINCT and ALL.
	ErrMutuallyExclusiveSelectModifiers = errors.New("SELECT DISTINCT and SELECT ALL are mutually exclusive")

	// ErrEmptySelectColumns is returned when SELECT specifies no columns.
	ErrEmptySelectColumns = errors.New("SELECT must specify at least one column")

	// ErrEmptyUpdateAssignments is returned when UPDATE specifies no assignments.
	ErrEmptyUpdateAssignments = errors.New("UPDATE must specify at least one assignment")
)
