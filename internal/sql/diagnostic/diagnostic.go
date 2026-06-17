package diagnostic

import (
	"fmt"
	"strings"
)

// Code is an opaque numeric error identifier.
// Ranges are assigned per-package by convention:
//
//	1000–1999  lexer
//	2000–2999  parser
//	3000–3999  semantic
type Code int

// Error implements the error interface so Code can act as sentinel errors.
func (c Code) Error() string {
	return fmt.Sprintf("E%d", c)
}

// Diagnostic is a single structured error with source location.
// Both lexer and parser produce these; a caller collects them
// in a List and handles them uniformly.
type Diagnostic struct {
	Code Code
	Line int // 1-based
	Col  int // 1-based
	Msg  string
}

func (d Diagnostic) Error() string {
	return fmt.Sprintf("%d:%d: E%d: %s", d.Line, d.Col, int(d.Code), d.Msg)
}

// Unwrap allows standard library functions like errors.Is to check for specific Code sentinels.
func (d Diagnostic) Unwrap() error {
	return d.Code
}

// New constructs a Diagnostic. Convenience for call sites.
func New(code Code, line, col int, format string, args ...any) Diagnostic {
	return Diagnostic{
		Code: code,
		Line: line,
		Col:  col,
		Msg:  fmt.Sprintf(format, args...),
	}
}

// List is an ordered collection of Diagnostic values.
// It satisfies the error interface so it can be returned as a single error.
type List []Diagnostic

func (l List) Error() string {
	var b strings.Builder
	for i, d := range l {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(d.Error())
	}
	return b.String()
}

func (l List) HasErrors() bool { return len(l) > 0 }

// Append adds a new diagnostic to the list.
func (l *List) Append(code Code, line, col int, format string, args ...any) {
	*l = append(*l, New(code, line, col, format, args...))
}
