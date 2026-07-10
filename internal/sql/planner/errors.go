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
