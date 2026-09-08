// Package executor translates validated planner.Plan trees into concrete
// KV operations, bridging the gap between the relational intent the planner
// produces and the key-value storage the bridge layer provides.
//
// The executor works directly against the kv.KV interface rather than gRPC,
// matching the existing bridge architecture. DDL operations use the
// catalog.Build*Ops / kv.WriteBatch / catalog.Apply* pipeline. DML and
// query operations use prefix scans, the codec for row serialization, and
// the encoding package for key construction.
package executor

import "github.com/makeshift-engineering/penguin-db/internal/sql/ast"

// ResultType classifies the kind of result an execution produces.
type ResultType int

const (
	// ResultDDL is returned for schema-modifying statements
	// (CREATE/DROP/ALTER DATABASE/TABLE, USE).
	ResultDDL ResultType = iota

	// ResultDML is returned for data-modifying statements
	// (INSERT, UPDATE, DELETE).
	ResultDML

	// ResultQuery is returned for data-reading statements (SELECT).
	ResultQuery
)

// Result is the unified return type for all statement executions. Exactly
// which fields are populated depends on Type:
//
//   - ResultDDL:   Message is set; RowsAffected, Columns, Rows are zero/nil.
//   - ResultDML:   RowsAffected is set; Columns, Rows are nil.
//   - ResultQuery: Columns and Rows are set; RowsAffected is zero.
//
// SessionUpdate is non-nil only when the executor needs the caller to
// update connection-level state (currently just USE database).
type Result struct {
	Type          ResultType
	RowsAffected  int64
	Columns       []ColumnInfo
	Rows          [][]any
	Message       string
	SessionUpdate *SessionUpdate
}

// ColumnInfo describes a single output column of a query result.
type ColumnInfo struct {
	Name string
	Type ast.DataTypeKind
}

// SessionUpdate signals that the caller should update session state.
// Currently only ActiveDatabase is used (for USE statements).
type SessionUpdate struct {
	ActiveDatabase string
}
