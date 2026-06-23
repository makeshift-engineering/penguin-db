package diagnostic

import (
	"fmt"
	"strings"
)

// Pos is a single point in the source text.
type Pos struct {
	Line   int // 1-based
	Col    int // 1-based
	Offset int // 0-based byte offset from start of input
}

// Span covers the range [Start, End) in the source.
type Span struct {
	Start Pos
	End   Pos
}

// Source holds the original input text for a single query or file.
// All Diagnostics for the same query share a pointer to the same Source.
type Source struct {
	Name string // filename, or "<query>" for inline SQL
	Text string // full source text
}

// Line returns the Nth source line (1-based) for use in error snippets.
func (s *Source) Line(n int) string {
	if s == nil {
		return ""
	}
	line := 1
	var b strings.Builder
	for _, ch := range s.Text {
		if line == n {
			if ch == '\n' {
				return b.String()
			}
			b.WriteRune(ch)
		} else if ch == '\n' {
			line++
		}
	}
	if line == n {
		return b.String()
	}
	return ""
}

// Severity categorizes the level of diagnostic messages.
type Severity uint8

const (
	SeverityError Severity = iota
	SeverityWarning
	SeverityHint
)

func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	case SeverityHint:
		return "hint"
	default:
		return "unknown"
	}
}

// Code is an opaque numeric error identifier.
// Ranges are assigned per-package by convention:
//
//	1000–1999  lexer
//	2000–2999  parser
//	3000–3999  semantic
type Code int

// Error implements the error interface so Code can act as sentinel errors.
func (c Code) Error() string {
	return fmt.Sprintf("E%d", int(c))
}

// Diagnostic is a single structured compiler message.
type Diagnostic struct {
	Severity Severity
	Code     Code
	Category string
	Span     Span
	Msg      string
	Source   *Source
}

// Error implements the error interface.
// Format: "3:18: error [E1001] Illegal Character: unexpected character '@'"
func (d *Diagnostic) Error() string {
	return fmt.Sprintf("%d:%d: %s [E%d] %s: %s",
		d.Span.Start.Line, d.Span.Start.Col,
		d.Severity, int(d.Code),
		d.Category, d.Msg)
}

// Unwrap allows standard library functions like errors.Is to check for specific Code sentinels.
func (d *Diagnostic) Unwrap() error {
	return d.Code
}

// Snippet returns the offending source line with a caret underline.
// Returns "" if Source is nil.
func (d *Diagnostic) Snippet() string {
	if d.Source == nil {
		return ""
	}
	line := d.Source.Line(d.Span.Start.Line)
	if line == "" {
		return ""
	}

	startCol := d.Span.Start.Col
	if startCol < 1 {
		startCol = 1
	}
	pad := strings.Repeat(" ", startCol-1)
	var width int
	if d.Span.Start.Line != d.Span.End.Line {
		width = len([]rune(line)) - startCol + 1
	} else {
		width = d.Span.End.Col - startCol
	}
	if width < 1 {
		width = 1
	}
	return fmt.Sprintf("  %s\n  %s%s", line, pad, strings.Repeat("^", width))
}

// FullString is the human display: error line + source snippet.
func (d *Diagnostic) FullString() string {
	msg := d.Error()
	if snip := d.Snippet(); snip != "" {
		msg += "\n" + snip
	}
	return msg
}

// List is an ordered collection of Diagnostic values.
// It satisfies the error interface so it can be returned as a single error.
type List []*Diagnostic

func (l List) Error() string {
	if len(l) == 0 {
		return "(no diagnostics)"
	}
	var b strings.Builder
	for i, d := range l {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(d.Error())
	}
	return b.String()
}

// HasErrors returns true if any diagnostic has SeverityError.
func (l List) HasErrors() bool {
	for _, d := range l {
		if d.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Unwrap enables errors.Is to traverse the list (Go 1.20+).
func (l List) Unwrap() []error {
	errs := make([]error, len(l))
	for i, d := range l {
		errs[i] = d
	}
	return errs
}

// Append adds a diagnostic directly to the list.
func (l *List) Append(d *Diagnostic) {
	if d == nil {
		return
	}
	*l = append(*l, d)
}
