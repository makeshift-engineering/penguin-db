package executor

import "errors"

// Sentinel errors returned by the executor.
var (
	// ErrNullPrimaryKey is returned when an INSERT or UPDATE produces a
	// NULL value for a primary key column.
	ErrNullPrimaryKey = errors.New("executor: NULL value in primary key column")

	// ErrNotNullViolation is returned when an INSERT or UPDATE attempts
	// to write NULL into a NOT NULL column.
	ErrNotNullViolation = errors.New("executor: NOT NULL constraint violation")

	// ErrDuplicateKey is returned when an INSERT would overwrite an
	// existing primary key.
	ErrDuplicateKey = errors.New("executor: duplicate primary key")

	// ErrTypeMismatch is returned when a value cannot be converted to the
	// target column type.
	ErrTypeMismatch = errors.New("executor: type mismatch")

	// ErrDivisionByZero is returned when a division or modulo expression
	// encounters a zero denominator.
	ErrDivisionByZero = errors.New("executor: division by zero")

	// ErrUnsupportedPlan is returned when Execute receives a plan type
	// it does not know how to handle.
	ErrUnsupportedPlan = errors.New("executor: unsupported plan type")

	// ErrVarcharTooLong is returned when a string value exceeds the
	// column's declared VARCHAR length.
	ErrVarcharTooLong = errors.New("executor: value exceeds VARCHAR length")

	// ErrUnsupportedExpr is returned when evalExpr encounters an
	// expression type it does not handle.
	ErrUnsupportedExpr = errors.New("executor: unsupported expression type")

	// ErrUnsupportedCond is returned when evalCond encounters a
	// condition type it does not handle.
	ErrUnsupportedCond = errors.New("executor: unsupported condition type")

	// ErrUnsupportedRelNode is returned when the query executor encounters
	// a RelNode type it does not handle.
	ErrUnsupportedRelNode = errors.New("executor: unsupported relational node type")

	// ErrUnsupportedAggFunc is returned when an aggregate function name
	// is not recognized by the executor.
	ErrUnsupportedAggFunc = errors.New("executor: unsupported aggregate function")

	// ErrColumnCountMismatch is returned when the number of values in an
	// INSERT row does not match the number of target columns.
	ErrColumnCountMismatch = errors.New("executor: column count mismatch")

	// ErrInvalidLikePattern is returned when a LIKE pattern operand is
	// not a string.
	ErrInvalidLikePattern = errors.New("executor: LIKE pattern must be a string")
)
