package lexer

import (
	"fmt"

	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
)

// Lexer diagnostic codes occupy the 1000–1999 range.
const (
	CodeUnexpectedChar      diagnostic.Code = 1001
	CodeUnterminatedString  diagnostic.Code = 1002
	CodeUnterminatedComment diagnostic.Code = 1003
)

// unexpectedChar creates a diagnostic for an unexpected character.
func unexpectedChar(span diagnostic.Span, src *diagnostic.Source, ch rune) *diagnostic.Diagnostic {
	return &diagnostic.Diagnostic{
		Severity: diagnostic.SeverityError,
		Code:     CodeUnexpectedChar,
		Category: "Illegal Character",
		Span:     span,
		Msg:      fmt.Sprintf("unexpected character %q", ch),
		Source:   src,
	}
}

// unterminatedString creates a diagnostic for an unterminated string literal.
func unterminatedString(span diagnostic.Span, src *diagnostic.Source) *diagnostic.Diagnostic {
	return &diagnostic.Diagnostic{
		Severity: diagnostic.SeverityError,
		Code:     CodeUnterminatedString,
		Category: "Unterminated String",
		Span:     span,
		Msg:      fmt.Sprintf("string literal opened at %d:%d was never closed", span.Start.Line, span.Start.Col),
		Source:   src,
	}
}

// unterminatedComment creates a diagnostic for an unterminated block comment.
func unterminatedComment(span diagnostic.Span, src *diagnostic.Source) *diagnostic.Diagnostic {
	return &diagnostic.Diagnostic{
		Severity: diagnostic.SeverityError,
		Code:     CodeUnterminatedComment,
		Category: "Unterminated Comment",
		Span:     span,
		Msg:      fmt.Sprintf("block comment opened at %d:%d was never closed", span.Start.Line, span.Start.Col),
		Source:   src,
	}
}
