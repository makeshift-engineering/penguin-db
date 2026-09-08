package planner

import (
	"errors"
	"fmt"

	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
)

// Planner diagnostic codes occupy the 3000–3999 range, reserved for
// semantic errors by the diagnostic package's own numbering convention.
const (
	// CodeNoActiveDatabase is emitted when an identifier carries no
	// database qualifier and no database is currently active for the
	// session (i.e. no USE statement has run yet).
	CodeNoActiveDatabase diagnostic.Code = 3001

	// CodeUnknownDatabase is emitted when a qualified or session-active
	// database name does not exist in the catalog.
	CodeUnknownDatabase diagnostic.Code = 3002

	// CodeUnknownTable is emitted when a table name does not exist in the
	// resolved database.
	CodeUnknownTable diagnostic.Code = 3003

	// CodeDuplicateTableBinding is emitted when two table references in
	// the same FROM clause resolve to the same name or alias.
	CodeDuplicateTableBinding diagnostic.Code = 3004

	// CodeUnknownTableBinding is emitted when a qualified column
	// reference's table qualifier does not match any table or alias
	// currently in scope.
	CodeUnknownTableBinding diagnostic.Code = 3005

	// CodeUnknownColumn is emitted when a column name does not exist on
	// the qualified table, or on any table in scope for a bare reference.
	CodeUnknownColumn diagnostic.Code = 3006

	// CodeAmbiguousColumn is emitted when a bare column name matches
	// columns on more than one table currently in scope.
	CodeAmbiguousColumn diagnostic.Code = 3007

	// CodeUnsupportedJoinType is emitted for LEFT, RIGHT, and FULL joins,
	// which the grammar accepts but the planner does not support in v1.
	CodeUnsupportedJoinType diagnostic.Code = 3008

	// CodeUnknownFunction is emitted when a function call names anything
	// other than the v1 aggregate set (COUNT, SUM, AVG, MIN, MAX).
	CodeUnknownFunction diagnostic.Code = 3009

	// CodeInvalidFunctionArgs is emitted for a function call with the
	// wrong number of arguments, a star argument on a non-COUNT function,
	// or an argument type an aggregate cannot operate on.
	CodeInvalidFunctionArgs diagnostic.Code = 3010

	// CodeTypeMismatch is emitted when two expressions being compared
	// (via =, !=, <, IN, BETWEEN, ...) have incompatible types.
	CodeTypeMismatch diagnostic.Code = 3011

	// CodeNonNumericOperand is emitted when a unary or binary arithmetic
	// operator is applied to a non-numeric, non-NULL operand.
	CodeNonNumericOperand diagnostic.Code = 3012

	// CodeNonBooleanCondition is emitted when a bare expression used
	// directly as a condition (e.g. in WHERE) is not boolean-typed.
	CodeNonBooleanCondition diagnostic.Code = 3013

	// CodeLiteralOverflow is emitted when an integer or float literal
	// cannot be represented in 64 bits.
	CodeLiteralOverflow diagnostic.Code = 3014

	// CodeNonStringOperand is emitted when LIKE is applied to a
	// non-string, non-NULL operand or pattern.
	CodeNonStringOperand diagnostic.Code = 3015

	// CodeUnsupportedExpression is emitted for an ast.Expression concrete
	// type resolveExpr does not recognise.
	CodeUnsupportedExpression diagnostic.Code = 3016

	// CodeUnsupportedCondition is emitted for an ast.Condition concrete
	// type resolveCond does not recognise.
	CodeUnsupportedCondition diagnostic.Code = 3017

	// CodeDuplicateColumn is emitted when a CREATE TABLE or ALTER TABLE
	// ADD/RENAME would introduce two columns with the same name.
	CodeDuplicateColumn diagnostic.Code = 3018

	// CodeInvalidDefaultValue is emitted when a DEFAULT literal's type is
	// incompatible with its column's declared type, or a sign is applied
	// to a non-numeric DEFAULT value.
	CodeInvalidDefaultValue diagnostic.Code = 3019

	// CodeColumnNotFound is emitted when an ALTER TABLE MODIFY, RENAME
	// COLUMN, or DROP COLUMN names a column that does not exist on the
	// table.
	CodeColumnNotFound diagnostic.Code = 3020

	// CodeCannotDropPKColumn is emitted when ALTER TABLE DROP COLUMN
	// targets a column that is part of the table's primary key.
	CodeCannotDropPKColumn diagnostic.Code = 3021

	// CodeDatabaseExists is emitted for CREATE DATABASE without IF NOT
	// EXISTS when the database already exists.
	CodeDatabaseExists diagnostic.Code = 3022

	// CodeTableExists is emitted for CREATE TABLE without IF NOT EXISTS
	// when the table already exists, and for ALTER TABLE RENAME TO when
	// the target name is already taken.
	CodeTableExists diagnostic.Code = 3023

	// CodeUnsupportedDataType is emitted for a data type the catalog
	// schema cannot yet represent — currently DECIMAL with an explicit
	// precision or scale, since catalog.ColumnMeta has no fields for them.
	CodeUnsupportedDataType diagnostic.Code = 3024

	// CodeUnsupportedConstraint is emitted for a column constraint clause
	// buildColumnMeta does not recognise.
	CodeUnsupportedConstraint diagnostic.Code = 3025

	// CodeUnsupportedAlter is emitted for an ALTER TABLE change that the
	// catalog's validateAlter is known to reject in v1: a new NOT NULL
	// column with no DEFAULT, a new UNIQUE or PRIMARY KEY column, or a
	// MODIFY that changes a column's data type.
	CodeUnsupportedAlter diagnostic.Code = 3026

	// CodeInvalidAlterAction is emitted for an ast.AlterActionKind
	// planAlterSchema does not recognise.
	CodeInvalidAlterAction diagnostic.Code = 3027

	// CodeColumnCountMismatch is emitted when an INSERT VALUES row or
	// INSERT ... SELECT source has a different number of columns than the
	// INSERT's target column list.
	CodeColumnCountMismatch diagnostic.Code = 3028

	// CodeUnknownInsertColumn is emitted when an INSERT's explicit column
	// list names a column that does not exist on the target table.
	CodeUnknownInsertColumn diagnostic.Code = 3029

	// CodeDuplicateInsertColumn is emitted when an INSERT's explicit
	// column list repeats the same column name.
	CodeDuplicateInsertColumn diagnostic.Code = 3030

	// CodeAggregateInWhere is emitted when an aggregate function appears
	// in a WHERE clause, where it is never legal (aggregation happens
	// after filtering, not during).
	CodeAggregateInWhere diagnostic.Code = 3031

	// CodeMissingGroupBy is emitted when a SELECT list or HAVING clause
	// references a column that is neither wrapped in an aggregate
	// function nor listed in GROUP BY — including the case where an
	// aggregate function forces a single implicit group with no GROUP BY
	// clause at all, so no bare column reference can ever be legal.
	CodeMissingGroupBy diagnostic.Code = 3032

	// CodeAggregateWithoutFrom is emitted when an aggregate function or
	// GROUP BY is used in a SELECT with no FROM clause.
	CodeAggregateWithoutFrom diagnostic.Code = 3033

	// CodeWhereWithoutFrom is emitted when a WHERE clause is used in a
	// SELECT with no FROM clause.
	CodeWhereWithoutFrom diagnostic.Code = 3034

	// CodeEmptyStarExpansion is emitted when SELECT * (or table.*)
	// expands to zero columns because the FROM clause has no tables in
	// scope for it to expand against.
	CodeEmptyStarExpansion diagnostic.Code = 3035

	// CodeOrdinalOutOfRange is emitted when ORDER BY <n> names a position
	// outside the SELECT list's column range.
	CodeOrdinalOutOfRange diagnostic.Code = 3036

	// CodeAggregateInJoin is emitted when an aggregate function is used in
	// a join condition.
	CodeAggregateInJoin diagnostic.Code = 3037

	// CodeNonOrderableType is emitted when an ordering operator (<, <=,
	// >, >=) or BETWEEN is applied to a type that only supports equality,
	// such as BOOLEAN.
	CodeNonOrderableType diagnostic.Code = 3038

	// CodeInvalidForeignKey is emitted when a REFERENCES constraint names
	// a table or column that does not exist in the catalog.
	CodeInvalidForeignKey diagnostic.Code = 3039

	// CodeDuplicateSetColumn is emitted when an UPDATE's SET clause
	// assigns the same column more than once.
	CodeDuplicateSetColumn diagnostic.Code = 3040

	// CodeNegativeLimit is emitted when LIMIT or OFFSET is negative.
	CodeNegativeLimit diagnostic.Code = 3041

	// CodeInvalidInsertPlan is a planner-internal assertion failure
	// emitted when an InsertPlan is constructed with both Rows and Source
	// set, or neither set.
	CodeInvalidInsertPlan diagnostic.Code = 3042

	// CodeNestedAggregate is emitted when an aggregate function call is
	// nested inside another aggregate function call, e.g. SUM(COUNT(*)),
	// which is illegal in standard SQL.
	CodeNestedAggregate diagnostic.Code = 3043

	// CodeNullComparison is emitted as a warning when NULL is compared
	// using = or !=, which always evaluates to NULL (unknown) in SQL
	// and is almost certainly a user mistake.
	CodeNullComparison diagnostic.Code = 3044

	// CodeNullInList is emitted as a warning when a NULL literal
	// appears in an IN list. Any comparison with NULL yields UNKNOWN:
	// for IN, a non-matching value evaluates to NULL instead of FALSE;
	// for NOT IN, a non-matching value evaluates to NULL instead of
	// TRUE, silently filtering rows. Removing NULL may change results.
	CodeNullInList diagnostic.Code = 3045

	// CodeNullAggregate is emitted as a warning when an aggregate
	// function is called with a NULL literal argument, e.g. SUM(NULL).
	CodeNullAggregate diagnostic.Code = 3046

	// CodeMalformedAST is emitted when the planner encounters a
	// structurally invalid AST node — e.g. a SelectExpression with
	// neither Expr nor Cond set.
	CodeMalformedAST diagnostic.Code = 3047
)

// ErrDuplicateTableBinding is returned by Scope.addTable when a binding
// name is already registered. Callers that have span information wrap this
// into a Diagnostic via errorf; Scope itself carries no source positions.
var ErrDuplicateTableBinding = errors.New("planner: duplicate table name or alias in FROM clause")

// errorf creates a Diagnostic with SeverityError, appends it to the
// planContext's diagnostic list, and returns it as an error value. This
// mirrors (*parser.Parser).errorf so the two packages read the same way.
func (pc *planContext) errorf(span diagnostic.Span, code diagnostic.Code, format string, args ...any) error {
	d := &diagnostic.Diagnostic{
		Severity: diagnostic.SeverityError,
		Code:     code,
		Category: "Semantic Error",
		Span:     span,
		Msg:      fmt.Sprintf(format, args...),
		Source:   pc.source,
	}
	pc.diag.Append(d)
	return d
}

// warnf creates a Diagnostic with SeverityWarning and appends it to the
// planContext's diagnostic list. Unlike errorf it does not return an error
// — warnings are advisory and never block planning.
func (pc *planContext) warnf(span diagnostic.Span, code diagnostic.Code, format string, args ...any) {
	d := &diagnostic.Diagnostic{
		Severity: diagnostic.SeverityWarning,
		Code:     code,
		Category: "Semantic Warning",
		Span:     span,
		Msg:      fmt.Sprintf(format, args...),
		Source:   pc.source,
	}
	pc.diag.Append(d)
}
