package wire

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
	"github.com/makeshift-engineering/penguin-db/internal/sql/executor"
	"github.com/makeshift-engineering/penguin-db/internal/sql/lexer"
	"github.com/makeshift-engineering/penguin-db/internal/sql/parser"
	"github.com/makeshift-engineering/penguin-db/internal/sql/planner"
)

// diagCodeToSQLSTATE maps PenguinDB diagnostic codes to PostgreSQL SQLSTATE
// codes. Lexer and parser codes map to 42601 (syntax_error). Planner codes
// are mapped to granular SQLSTATE values matching PostgreSQL conventions.
func diagCodeToSQLSTATE(code diagnostic.Code) string {
	switch code {
	// ── Lexer (1000–1999) ────────────────────────────────────────────────
	case lexer.CodeUnexpectedChar:
		return "42601" // syntax_error
	case lexer.CodeUnterminatedString:
		return "42601"
	case lexer.CodeUnterminatedComment:
		return "42601"

	// ── Parser (2000–2999) ───────────────────────────────────────────────
	case parser.CodeUnexpectedToken,
		parser.CodeUnexpectedEOF,
		parser.CodeInvalidDataType,
		parser.CodeInvalidAlterAction,
		parser.CodeInvalidJoinType,
		parser.CodeExpectedExpression,
		parser.CodeExpectedCondition,
		parser.CodeInvalidIntegerLiteral,
		parser.CodeMalformedStatement:
		return "42601" // syntax_error

	// ── Planner / Semantic (3000–3999) ───────────────────────────────────
	case planner.CodeNoActiveDatabase:
		return "3D000" // invalid_catalog_name
	case planner.CodeUnknownDatabase:
		return "3D000" // invalid_catalog_name
	case planner.CodeUnknownTable:
		return "42P01" // undefined_table
	case planner.CodeDuplicateTableBinding:
		return "42712" // duplicate_alias
	case planner.CodeUnknownTableBinding:
		return "42P01" // undefined_table
	case planner.CodeUnknownColumn:
		return "42703" // undefined_column
	case planner.CodeAmbiguousColumn:
		return "42702" // ambiguous_column
	case planner.CodeUnsupportedJoinType:
		return "0A000" // feature_not_supported
	case planner.CodeUnknownFunction:
		return "42883" // undefined_function
	case planner.CodeInvalidFunctionArgs:
		return "42804" // datatype_mismatch
	case planner.CodeTypeMismatch:
		return "42804" // datatype_mismatch
	case planner.CodeNonNumericOperand:
		return "42804" // datatype_mismatch
	case planner.CodeNonBooleanCondition:
		return "42804" // datatype_mismatch
	case planner.CodeLiteralOverflow:
		return "22003" // numeric_value_out_of_range
	case planner.CodeNonStringOperand:
		return "42804" // datatype_mismatch
	case planner.CodeUnsupportedExpression,
		planner.CodeUnsupportedCondition:
		return "0A000" // feature_not_supported
	case planner.CodeDuplicateColumn:
		return "42701" // duplicate_column
	case planner.CodeInvalidDefaultValue:
		return "22P02" // invalid_text_representation
	case planner.CodeColumnNotFound:
		return "42703" // undefined_column
	case planner.CodeCannotDropPKColumn:
		return "42P16" // invalid_table_definition
	case planner.CodeDatabaseExists:
		return "42P04" // duplicate_database
	case planner.CodeTableExists:
		return "42P07" // duplicate_table
	case planner.CodeUnsupportedDataType:
		return "0A000" // feature_not_supported
	case planner.CodeUnsupportedConstraint:
		return "0A000" // feature_not_supported
	case planner.CodeUnsupportedAlter:
		return "0A000" // feature_not_supported
	case planner.CodeInvalidAlterAction:
		return "42601" // syntax_error (bad ALTER action)
	case planner.CodeColumnCountMismatch:
		return "42601" // syntax_error
	case planner.CodeUnknownInsertColumn:
		return "42703" // undefined_column
	case planner.CodeDuplicateInsertColumn:
		return "42701" // duplicate_column
	case planner.CodeAggregateInWhere:
		return "42803" // grouping_error
	case planner.CodeMissingGroupBy:
		return "42803" // grouping_error
	case planner.CodeAggregateWithoutFrom:
		return "42803" // grouping_error
	case planner.CodeWhereWithoutFrom:
		return "42P10" // invalid_column_reference
	case planner.CodeEmptyStarExpansion:
		return "42P10" // invalid_column_reference
	case planner.CodeOrdinalOutOfRange:
		return "42P10" // invalid_column_reference
	case planner.CodeAggregateInJoin:
		return "42803" // grouping_error
	case planner.CodeNonOrderableType:
		return "42804" // datatype_mismatch
	case planner.CodeInvalidForeignKey:
		return "42830" // invalid_foreign_key
	case planner.CodeDuplicateSetColumn:
		return "42701" // duplicate_column
	case planner.CodeNegativeLimit:
		return "2201W" // invalid_row_count_in_limit_clause
	case planner.CodeInvalidInsertPlan:
		return "XX000" // internal_error
	case planner.CodeNestedAggregate:
		return "42803" // grouping_error
	case planner.CodeNullComparison:
		return "01003" // warning: null_value_eliminated_in_set_function
	case planner.CodeNullInList:
		return "01003"
	case planner.CodeNullAggregate:
		return "01003"
	case planner.CodeMalformedAST:
		return "XX000" // internal_error

	default:
		return "42000" // syntax_error_or_access_rule_violation
	}
}

// severityToWire converts a diagnostic.Severity to the PostgreSQL wire
// severity string used in ErrorResponse and NoticeResponse packets.
func severityToWire(s diagnostic.Severity) string {
	switch s {
	case diagnostic.SeverityError:
		return "ERROR"
	case diagnostic.SeverityWarning:
		return "WARNING"
	case diagnostic.SeverityHint:
		return "NOTICE"
	default:
		return "ERROR"
	}
}

// diagToWireError converts a single diagnostic.Diagnostic into a WireError
// with the correct SQLSTATE code and, when a source snippet is available,
// fills the Detail and Position fields so psql can render a caret pointer.
func diagToWireError(d *diagnostic.Diagnostic) *WireError {
	msg := fmt.Sprintf("[E%d] %s: %s", int(d.Code), d.Category, d.Msg)
	if d.Span.Start.Line > 0 && d.Span.Start.Col > 0 {
		msg = fmt.Sprintf("%d:%d: [E%d] %s: %s", d.Span.Start.Line, d.Span.Start.Col, int(d.Code), d.Category, d.Msg)
	}

	we := &WireError{
		Severity: severityToWire(d.Severity),
		Code:     diagCodeToSQLSTATE(d.Code),
		Message:  msg,
	}

	// PostgreSQL Position is a 1-based byte offset from the start of the
	// query string. psql uses it to render the caret on the correct line.
	if d.Span.Start.Offset >= 0 && d.Span.Start.Line > 0 {
		we.Position = d.Span.Start.Offset + 1
	}

	// Include the source snippet as Detail for clients that display it.
	if snip := d.Snippet(); snip != "" {
		we.Detail = snip
	}

	return we
}

// diagListToWireError selects the first error-severity diagnostic from the
// list and converts it into a WireError. If additional errors exist, a
// count is appended to the Detail field.
func diagListToWireError(dl diagnostic.List) *WireError {
	var first *diagnostic.Diagnostic
	var errorCount int
	for _, d := range dl {
		if d.Severity == diagnostic.SeverityError {
			errorCount++
			if first == nil {
				first = d
			}
		}
	}
	if first == nil {
		return nil
	}

	we := diagToWireError(first)
	if errorCount > 1 {
		extra := fmt.Sprintf("\n... and %d more error(s)", errorCount-1)
		we.Detail += extra
	}
	return we
}

// execErrToWireError converts an executor runtime error into a WireError
// with an appropriate SQLSTATE code. Internal details that are unsafe for
// end-user display are sanitised.
func execErrToWireError(err error) *WireError {
	we := &WireError{Severity: "ERROR"}

	switch {
	case errors.Is(err, executor.ErrDuplicateKey):
		we.Code = "23505" // unique_violation
		we.Message = fmt.Sprintf("[E%d] duplicate primary key: %s", int(executor.CodeDuplicateKey), cleanExecMessage(err))
	case errors.Is(err, executor.ErrNotNullViolation):
		we.Code = "23502" // not_null_violation
		we.Message = fmt.Sprintf("[E%d] NOT NULL constraint violation: %s", int(executor.CodeNotNullViolation), cleanExecMessage(err))
	case errors.Is(err, executor.ErrNullPrimaryKey):
		we.Code = "23502" // not_null_violation
		we.Message = fmt.Sprintf("[E%d] primary key cannot be NULL: %s", int(executor.CodeNullPrimaryKey), cleanExecMessage(err))
	case errors.Is(err, executor.ErrDivisionByZero):
		we.Code = "22012" // division_by_zero
		we.Message = fmt.Sprintf("[E%d] %s", int(executor.CodeDivisionByZero), cleanExecMessage(err))
	case errors.Is(err, executor.ErrVarcharTooLong):
		we.Code = "22001" // string_data_right_truncation
		we.Message = fmt.Sprintf("[E%d] %s", int(executor.CodeVarcharTooLong), cleanExecMessage(err))
	case errors.Is(err, executor.ErrTypeMismatch):
		we.Code = "42804" // datatype_mismatch
		we.Message = fmt.Sprintf("[E%d] %s", int(executor.CodeTypeMismatch), cleanExecMessage(err))
	case errors.Is(err, executor.ErrColumnCountMismatch):
		we.Code = "42611" // invalid_column_definition
		we.Message = fmt.Sprintf("[E%d] %s", int(executor.CodeColumnCountMismatch), cleanExecMessage(err))
	case errors.Is(err, executor.ErrInvalidLikePattern):
		we.Code = "22025" // invalid_escape_sequence
		we.Message = fmt.Sprintf("[E%d] %s", int(executor.CodeInvalidLikePattern), cleanExecMessage(err))
	case errors.Is(err, catalog.ErrDatabaseExists):
		we.Code = "42P04" // duplicate_database
		we.Message = cleanExecMessage(err)
	case errors.Is(err, catalog.ErrTableExists):
		we.Code = "42P07" // duplicate_table
		we.Message = cleanExecMessage(err)
	case errors.Is(err, catalog.ErrDatabaseNotFound),
		errors.Is(err, catalog.ErrCatalogDBMissing):
		we.Code = "3D000" // invalid_catalog_name
		we.Message = cleanExecMessage(err)
	case errors.Is(err, catalog.ErrTableNotFound):
		we.Code = "42P01" // undefined_table
		we.Message = cleanExecMessage(err)
	case errors.Is(err, catalog.ErrColumnNotFound),
		errors.Is(err, catalog.ErrPKColumnNotFound):
		we.Code = "42703" // undefined_column
		we.Message = cleanExecMessage(err)
	case errors.Is(err, catalog.ErrDuplicateColumn):
		we.Code = "42701" // duplicate_column
		we.Message = cleanExecMessage(err)
	case errors.Is(err, catalog.ErrCannotDropPKColumn),
		errors.Is(err, catalog.ErrSchemaViolation):
		we.Code = "42P16" // invalid_table_definition
		we.Message = cleanExecMessage(err)
	case errors.Is(err, catalog.ErrUnsupportedAlter):
		we.Code = "0A000" // feature_not_supported
		we.Message = cleanExecMessage(err)
	case errors.Is(err, context.DeadlineExceeded):
		we.Code = "57014" // query_canceled
		we.Message = "query execution timed out"
	case errors.Is(err, context.Canceled):
		we.Code = "57014" // query_canceled
		we.Message = "query execution canceled"
	default:
		we.Code = "XX000" // internal_error
		we.Message = sanitiseInternalError(err)
	}

	return we
}

// cleanExecMessage strips internal prefixes from wrapped error messages to
// produce a cleaner user-facing string.
func cleanExecMessage(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	for {
		trimmed := msg
		for _, prefix := range []string{
			"executor: evaluating INSERT value: ",
			"executor: evaluating SET value: ",
			"executor: converting value for column ",
			"executor: converting updated value for column ",
			"executor: writing INSERT: ",
			"executor: writing DELETE: ",
			"executor: writing UPDATE: ",
			"executor: writing INSERT ... SELECT: ",
			"executor: executing INSERT ... SELECT source: ",
			"executor: checking PK existence: ",
			"executor: getting PK types: ",
			"executor: scanning for UPDATE: ",
			"executor: scanning for DELETE: ",
			"executor: encoding row: ",
			"executor: decoding row: ",
			"executor: ",
			"catalog: ",
			"kv: ",
		} {
			trimmed = strings.TrimPrefix(trimmed, prefix)
		}
		if trimmed == msg {
			break
		}
		msg = trimmed
	}
	return msg
}

// sanitiseInternalError returns a generic message for unexpected errors,
// preventing internal implementation details from leaking to the client.
func sanitiseInternalError(err error) string {
	if err == nil {
		return "internal server error"
	}

	msg := cleanExecMessage(err)

	// If the message contains gRPC error traces, sanitize them
	if strings.Contains(msg, "rpc error:") || strings.Contains(msg, "grpc.") {
		if strings.Contains(msg, "unavailable") || strings.Contains(msg, "connection refused") {
			return "storage server unavailable"
		}
		return "storage service error"
	}

	// Check for well-known user-safe error messages.
	for _, safe := range []string{
		"storage server unavailable",
		"precondition failed",
		"invalid argument",
		"internal storage error",
	} {
		if strings.Contains(msg, safe) {
			return msg
		}
	}

	return "internal server error"
}
