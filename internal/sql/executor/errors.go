package executor

import (
	"errors"
	"strings"

	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
	"github.com/makeshift-engineering/penguin-db/internal/sql/planner"
	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
)

// Executor diagnostic codes occupy the 4000–4999 range.
const (
	CodeNullPrimaryKey      diagnostic.Code = 4001
	CodeNotNullViolation    diagnostic.Code = 4002
	CodeDuplicateKey        diagnostic.Code = 4003
	CodeTypeMismatch        diagnostic.Code = 4004
	CodeDivisionByZero      diagnostic.Code = 4005
	CodeUnsupportedPlan              diagnostic.Code = 4006
	CodeVarcharTooLong               diagnostic.Code = 4007
	CodeUnsupportedExpression        diagnostic.Code = 4008
	CodeUnsupportedExpr              diagnostic.Code = 4008 // backward-compatibility alias
	CodeUnsupportedCondition         diagnostic.Code = 4009
	CodeUnsupportedCond              diagnostic.Code = 4009 // backward-compatibility alias
	CodeUnsupportedRelationalNode    diagnostic.Code = 4010
	CodeUnsupportedRelNode           diagnostic.Code = 4010 // backward-compatibility alias
	CodeUnsupportedAggregateFunction diagnostic.Code = 4011
	CodeUnsupportedAggFunc           diagnostic.Code = 4011 // backward-compatibility alias
	CodeColumnCountMismatch          diagnostic.Code = 4012
	CodeInvalidLikePattern           diagnostic.Code = 4013
	CodeNumericOverflow              diagnostic.Code = 4014
)

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

	// ErrNumericOverflow is returned when arithmetic exceeds the supported
	// range of its integer result type.
	ErrNumericOverflow = errors.New("executor: numeric overflow")

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

// SpannedError binds an execution error to a source diagnostic.Span.
type SpannedError struct {
	Err  error
	Span diagnostic.Span
}

func (e *SpannedError) Error() string {
	return e.Err.Error()
}

func (e *SpannedError) Unwrap() error {
	return e.Err
}

func (e *SpannedError) ErrorSpan() diagnostic.Span {
	return e.Span
}

// withSpan attaches a span to err unless err already carries a valid span.
func withSpan(err error, span diagnostic.Span) error {
	if err == nil || span.Start.Line <= 0 {
		return err
	}
	type spanned interface {
		ErrorSpan() diagnostic.Span
	}
	var sp spanned
	if errors.As(err, &sp) && sp.ErrorSpan().Start.Line > 0 {
		return err
	}
	return &SpannedError{Err: err, Span: span}
}

// ExecutionDiagnosticError wraps an executor error with structured compiler diagnostics,
// including line, column, source code snippet, and carets (^^^^).
type ExecutionDiagnosticError struct {
	Diagnostic *diagnostic.Diagnostic
	Err        error
}

func (e *ExecutionDiagnosticError) Error() string {
	if e.Diagnostic != nil {
		return e.Diagnostic.FullString()
	}
	return e.Err.Error()
}

func (e *ExecutionDiagnosticError) Unwrap() error {
	return e.Err
}

func (e *ExecutionDiagnosticError) As(target any) bool {
	if d, ok := target.(**diagnostic.Diagnostic); ok {
		*d = e.Diagnostic
		return true
	}
	return errors.As(e.Err, target)
}

func (e *ExecutionDiagnosticError) ErrorSpan() diagnostic.Span {
	if e.Diagnostic != nil {
		return e.Diagnostic.Span
	}
	return diagnostic.Span{}
}

func errorCodeForExecutorError(err error) diagnostic.Code {
	switch {
	case errors.Is(err, ErrNullPrimaryKey):
		return CodeNullPrimaryKey
	case errors.Is(err, ErrNotNullViolation):
		return CodeNotNullViolation
	case errors.Is(err, ErrDuplicateKey):
		return CodeDuplicateKey
	case errors.Is(err, ErrDivisionByZero):
		return CodeDivisionByZero
	case errors.Is(err, ErrNumericOverflow):
		return CodeNumericOverflow
	case errors.Is(err, ErrVarcharTooLong):
		return CodeVarcharTooLong
	case errors.Is(err, ErrTypeMismatch):
		return CodeTypeMismatch
	case errors.Is(err, ErrColumnCountMismatch):
		return CodeColumnCountMismatch
	case errors.Is(err, ErrInvalidLikePattern):
		return CodeInvalidLikePattern
	case errors.Is(err, ErrUnsupportedPlan):
		return CodeUnsupportedPlan
	case errors.Is(err, ErrUnsupportedExpr):
		return CodeUnsupportedExpression
	case errors.Is(err, ErrUnsupportedCond):
		return CodeUnsupportedCondition
	case errors.Is(err, ErrUnsupportedRelNode):
		return CodeUnsupportedRelNode
	case errors.Is(err, ErrUnsupportedAggFunc):
		return CodeUnsupportedAggregateFunction
	case errors.Is(err, catalog.ErrDatabaseExists):
		return planner.CodeDuplicateDatabase
	case errors.Is(err, catalog.ErrTableExists):
		return planner.CodeDuplicateTable
	case errors.Is(err, catalog.ErrDatabaseNotFound), errors.Is(err, catalog.ErrCatalogDBMissing):
		return planner.CodeUnknownDatabase
	case errors.Is(err, catalog.ErrTableNotFound):
		return planner.CodeUnknownTable
	case errors.Is(err, catalog.ErrColumnNotFound), errors.Is(err, catalog.ErrPKColumnNotFound):
		return planner.CodeUnknownAlterColumn
	case errors.Is(err, catalog.ErrDuplicateColumn):
		return planner.CodeDuplicateColumn
	case errors.Is(err, catalog.ErrCannotDropPKColumn), errors.Is(err, catalog.ErrSchemaViolation):
		return planner.CodeCannotDropPKColumn
	case errors.Is(err, catalog.ErrUnsupportedAlter):
		return planner.CodeUnsupportedAlter
	default:
		return diagnostic.Code(4999)
	}
}

func cleanErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	for {
		trimmed := msg
		if strings.HasPrefix(trimmed, "executor: evaluating projection item ") {
			rem := trimmed[len("executor: evaluating projection item "):]
			if idx := strings.Index(rem, ": "); idx != -1 {
				trimmed = rem[idx+2:]
			}
		}
		for _, prefix := range []string{
			"executor: evaluating INSERT value: ",
			"executor: evaluating SET value: ",
			"executor: evaluating WHERE: ",
			"executor: evaluating WHERE for UPDATE: ",
			"executor: evaluating WHERE for DELETE: ",
			"executor: converting value for column ",
			"executor: converting updated value for column ",
			"executor: writing INSERT ... SELECT: ",
			"executor: executing INSERT ... SELECT source: ",
			"executor: writing INSERT: ",
			"executor: writing DELETE: ",
			"executor: writing UPDATE: ",
			"executor: writing CREATE DATABASE: ",
			"executor: writing DROP DATABASE: ",
			"executor: writing CREATE TABLE: ",
			"executor: writing DROP TABLE: ",
			"executor: writing ALTER TABLE: ",
			"executor: writing RENAME TABLE: ",
			"executor: building CREATE DATABASE ops: ",
			"executor: building DROP DATABASE ops: ",
			"executor: building CREATE TABLE ops: ",
			"executor: building DROP TABLE ops: ",
			"executor: building ALTER TABLE ops: ",
			"executor: building RENAME TABLE ops: ",
			"executor: checking PK existence: ",
			"executor: getting PK types: ",
			"executor: scanning for UPDATE: ",
			"executor: scanning for DELETE: ",
			"executor: encoding row: ",
			"executor: decoding row: ",
			"executor: ",
			"catalog: ",
			"kv: ",
			"planner: ",
		} {
			trimmed = strings.TrimPrefix(trimmed, prefix)
		}
		if trimmed == msg {
			break
		}
		msg = trimmed
	}
	msg = strings.ReplaceAll(msg, ": executor: ", ": ")
	msg = strings.TrimPrefix(msg, "executor: ")
	return msg
}

func enrichExecutorError(err error, plan planner.Plan) error {
	if err == nil {
		return nil
	}
	var existingDiag *diagnostic.Diagnostic
	if errors.As(err, &existingDiag) && existingDiag != nil {
		return err
	}

	var span diagnostic.Span
	type spanned interface {
		ErrorSpan() diagnostic.Span
	}
	var sp spanned
	if errors.As(err, &sp) {
		span = sp.ErrorSpan()
	}
	if span.Start.Line <= 0 && plan != nil {
		span = plan.PlanSpan()
	}
	if span.Start.Line <= 0 {
		return err
	}

	var src *diagnostic.Source
	if plan != nil {
		src = plan.PlanSource()
	}

	code := errorCodeForExecutorError(err)
	msg := cleanErrorMessage(err)

	diag := &diagnostic.Diagnostic{
		Severity: diagnostic.SeverityError,
		Code:     code,
		Category: "Runtime Error",
		Span:     span,
		Msg:      msg,
		Source:   src,
	}

	return &ExecutionDiagnosticError{
		Diagnostic: diag,
		Err:        err,
	}
}
