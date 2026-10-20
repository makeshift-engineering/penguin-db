package wire

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/bridge/catalog"
	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
	"github.com/makeshift-engineering/penguin-db/internal/sql/executor"
	"github.com/makeshift-engineering/penguin-db/internal/sql/lexer"
	"github.com/makeshift-engineering/penguin-db/internal/sql/parser"
	"github.com/makeshift-engineering/penguin-db/internal/sql/planner"
)

func TestDiagCodeToSQLSTATE(t *testing.T) {
	tests := []struct {
		code diagnostic.Code
		want string
	}{
		{lexer.CodeUnexpectedChar, "42601"},
		{lexer.CodeUnterminatedString, "42601"},
		{parser.CodeUnexpectedToken, "42601"},
		{parser.CodeExpectedExpression, "42601"},
		{planner.CodeUnknownTable, "42P01"},
		{planner.CodeUnknownColumn, "42703"},
		{planner.CodeAmbiguousColumn, "42702"},
		{planner.CodeNoActiveDatabase, "3D000"},
		{planner.CodeTypeMismatch, "42804"},
		{planner.CodeDatabaseExists, "42P04"},
		{planner.CodeTableExists, "42P07"},
		{planner.CodeAggregateInWhere, "42803"},
		{planner.CodeLiteralOverflow, "22003"},
		{planner.CodeNegativeLimit, "2201W"},
		{diagnostic.Code(99999), "42000"},
	}

	for _, tt := range tests {
		got := diagCodeToSQLSTATE(tt.code)
		if got != tt.want {
			t.Errorf("diagCodeToSQLSTATE(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestDiagToWireError(t *testing.T) {
	src := &diagnostic.Source{
		Name: "query",
		Text: "SELECT 42abc FROM users;",
	}
	d := &diagnostic.Diagnostic{
		Severity: diagnostic.SeverityError,
		Code:     parser.CodeUnexpectedToken,
		Category: "Syntax Error",
		Span: diagnostic.Span{
			Start: diagnostic.Pos{Line: 1, Col: 8, Offset: 7},
			End:   diagnostic.Pos{Line: 1, Col: 13, Offset: 12},
		},
		Msg:    `expected IDENTIFIER, got INTEGER "42"`,
		Source: src,
	}

	we := diagToWireError(d)
	if we.Severity != "ERROR" {
		t.Errorf("expected Severity ERROR, got %q", we.Severity)
	}
	if we.Code != "42601" {
		t.Errorf("expected Code 42601, got %q", we.Code)
	}
	expectedMsg := `1:8: [E2001] Syntax Error: expected IDENTIFIER, got INTEGER "42"`
	if we.Message != expectedMsg {
		t.Errorf("expected Message %q, got %q", expectedMsg, we.Message)
	}
	if we.Position != 8 { // 1-based offset: 7 + 1
		t.Errorf("expected Position 8, got %d", we.Position)
	}
	if !strings.Contains(we.Detail, "^") {
		t.Errorf("expected Detail to contain caret underline, got:\n%s", we.Detail)
	}
}

func TestDiagListToWireError_Multiple(t *testing.T) {
	src := &diagnostic.Source{Name: "query", Text: "SELECT foo, bar"}
	dl := diagnostic.List{
		&diagnostic.Diagnostic{
			Severity: diagnostic.SeverityError,
			Code:     planner.CodeUnknownColumn,
			Category: "Semantic Error",
			Span:     diagnostic.Span{Start: diagnostic.Pos{Line: 1, Col: 8, Offset: 7}},
			Msg:      `column "foo" does not exist`,
			Source:   src,
		},
		&diagnostic.Diagnostic{
			Severity: diagnostic.SeverityError,
			Code:     planner.CodeUnknownColumn,
			Category: "Semantic Error",
			Span:     diagnostic.Span{Start: diagnostic.Pos{Line: 1, Col: 13, Offset: 12}},
			Msg:      `column "bar" does not exist`,
			Source:   src,
		},
	}

	we := diagListToWireError(dl)
	if we == nil {
		t.Fatal("expected non-nil WireError")
	}
	if we.Code != "42703" {
		t.Errorf("expected Code 42703, got %q", we.Code)
	}
	if !strings.Contains(we.Detail, "1 more error(s)") {
		t.Errorf("expected Detail to mention additional errors, got:\n%s", we.Detail)
	}
}

func TestExecErrToWireError(t *testing.T) {
	t.Run("duplicate key", func(t *testing.T) {
		err := fmt.Errorf("executor: writing INSERT: %w: key already exists in \"mydb\".\"users\"", executor.ErrDuplicateKey)
		we := execErrToWireError(err)
		if we.Code != "23505" {
			t.Errorf("expected Code 23505, got %q", we.Code)
		}
		if !strings.Contains(we.Message, "[E4003]") || !strings.Contains(we.Message, "duplicate primary key") {
			t.Errorf("unexpected Message: %q", we.Message)
		}
		if strings.Contains(we.Message, "executor: ") {
			t.Errorf("message should not contain internal prefixes: %q", we.Message)
		}
	})

	t.Run("not null violation", func(t *testing.T) {
		err := fmt.Errorf("executor: %w: column \"email\"", executor.ErrNotNullViolation)
		we := execErrToWireError(err)
		if we.Code != "23502" {
			t.Errorf("expected Code 23502, got %q", we.Code)
		}
		if !strings.Contains(we.Message, "[E4002]") {
			t.Errorf("expected [E4002] in Message: %q", we.Message)
		}
	})

	t.Run("catalog table not found", func(t *testing.T) {
		err := catalog.ErrTableNotFound
		we := execErrToWireError(err)
		if we.Code != "42P01" {
			t.Errorf("expected Code 42P01, got %q", we.Code)
		}
	})

	t.Run("grpc error sanitization", func(t *testing.T) {
		err := errors.New("executor: writing INSERT: rpc error: code = Internal desc = panic: nil pointer dereference")
		we := execErrToWireError(err)
		if we.Code != "XX000" {
			t.Errorf("expected Code XX000, got %q", we.Code)
		}
		if strings.Contains(we.Message, "rpc error") || strings.Contains(we.Message, "nil pointer") {
			t.Errorf("gRPC trace leaked: %q", we.Message)
		}
		if we.Message != "storage service error" {
			t.Errorf("expected 'storage service error', got %q", we.Message)
		}
	})
}

func TestEncodeRichErrorResponse(t *testing.T) {
	we := &WireError{
		Severity: "ERROR",
		Code:     "42601",
		Message:  `1:8: [E2001] Syntax Error: expected IDENTIFIER, got INTEGER "42"`,
		Detail:   "  SELECT 42abc FROM users;\n         ^^",
		Hint:     "check your query syntax",
		Position: 8,
	}

	raw := encodeRichErrorResponse(we)
	if len(raw) < 5 {
		t.Fatalf("packet too short: %d", len(raw))
	}
	if raw[0] != MessageErrorResponse {
		t.Fatalf("expected MessageErrorResponse 'E', got %c", raw[0])
	}

	// Parse fields
	fields := make(map[byte]string)
	r := bytes.NewReader(raw[5:]) // skip 'E' and 4-byte length
	for r.Len() > 0 {
		tag, err := r.ReadByte()
		if err != nil || tag == 0 {
			break
		}
		var val bytes.Buffer
		for {
			b, err := r.ReadByte()
			if err != nil || b == 0 {
				break
			}
			val.WriteByte(b)
		}
		fields[tag] = val.String()
	}

	if fields['S'] != "ERROR" {
		t.Errorf("expected S=ERROR, got %q", fields['S'])
	}
	if fields['C'] != "42601" {
		t.Errorf("expected C=42601, got %q", fields['C'])
	}
	if fields['M'] != we.Message {
		t.Errorf("expected M=%q, got %q", we.Message, fields['M'])
	}
	if fields['D'] != we.Detail {
		t.Errorf("expected D=%q, got %q", we.Detail, fields['D'])
	}
	if fields['H'] != we.Hint {
		t.Errorf("expected H=%q, got %q", we.Hint, fields['H'])
	}
	if fields['P'] != "8" {
		t.Errorf("expected P=8, got %q", fields['P'])
	}
}

