package diagnostic

import (
	"errors"
	"strings"
	"testing"
)

// TestSource_Line tests source line.
func TestSource_Line(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		lineNum  int
		expected string
	}{
		{
			name:     "single line query",
			text:     "SELECT * FROM users",
			lineNum:  1,
			expected: "SELECT * FROM users",
		},
		{
			name:     "multi line query first line",
			text:     "SELECT *\nFROM users\nWHERE id = 1",
			lineNum:  1,
			expected: "SELECT *",
		},
		{
			name:     "multi line query middle line",
			text:     "SELECT *\nFROM users\nWHERE id = 1",
			lineNum:  2,
			expected: "FROM users",
		},
		{
			name:     "multi line query last line",
			text:     "SELECT *\nFROM users\nWHERE id = 1",
			lineNum:  3,
			expected: "WHERE id = 1",
		},
		{
			name:     "empty line handling",
			text:     "first\n\nthird",
			lineNum:  2,
			expected: "",
		},
		{
			name:     "out of bounds line number high",
			text:     "SELECT *",
			lineNum:  2,
			expected: "",
		},
		{
			name:     "out of bounds line number low",
			text:     "SELECT *",
			lineNum:  0,
			expected: "",
		},
		{
			name:     "utf-8 content line",
			text:     "SELECT '🚀'\nFROM space",
			lineNum:  1,
			expected: "SELECT '🚀'",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := &Source{Name: "test", Text: tc.text}
			got := src.Line(tc.lineNum)
			if got != tc.expected {
				t.Errorf("Line(%d) = %q, want %q", tc.lineNum, got, tc.expected)
			}
		})
	}

	t.Run("nil source returns empty string", func(t *testing.T) {
		var src *Source
		got := src.Line(1)
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})
}

// TestSeverity_String tests severity string.
func TestSeverity_String(t *testing.T) {
	tests := []struct {
		severity Severity
		expected string
	}{
		{SeverityError, "error"},
		{SeverityWarning, "warning"},
		{SeverityHint, "hint"},
		{Severity(99), "unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			got := tc.severity.String()
			if got != tc.expected {
				t.Errorf("String() = %q, want %q", got, tc.expected)
			}
		})
	}
}

// TestCode_Error tests code error.
func TestCode_Error(t *testing.T) {
	code := Code(1001)
	expected := "E1001"
	if code.Error() != expected {
		t.Errorf("Code.Error() = %q, want %q", code.Error(), expected)
	}
}

// TestDiagnostic_Error tests diagnostic error.
func TestDiagnostic_Error(t *testing.T) {
	d := &Diagnostic{
		Severity: SeverityError,
		Code:     Code(1001),
		Category: "Illegal Character",
		Span: Span{
			Start: Pos{Line: 1, Col: 5, Offset: 4},
			End:   Pos{Line: 1, Col: 6, Offset: 5},
		},
		Msg:    "unexpected character '@'",
		Source: &Source{Name: "query", Text: "SELECT @ FROM users"},
	}

	expected := "1:5: error [E1001] Illegal Character: unexpected character '@'"
	if d.Error() != expected {
		t.Errorf("Error() = %q, want %q", d.Error(), expected)
	}
}

// TestDiagnostic_Unwrap tests diagnostic unwrap.
func TestDiagnostic_Unwrap(t *testing.T) {
	sentinel := Code(1002)
	d := &Diagnostic{
		Code: sentinel,
	}

	if !errors.Is(d, sentinel) {
		t.Error("errors.Is failed to match diagnostic wrapping Code")
	}
}

// TestDiagnostic_Snippet tests diagnostic snippet.
func TestDiagnostic_Snippet(t *testing.T) {
	src := &Source{
		Name: "query",
		Text: "SELECT * FROM users\nWHERE id = 1;",
	}

	t.Run("single character span", func(t *testing.T) {
		d := &Diagnostic{
			Span: Span{
				Start: Pos{Line: 1, Col: 8, Offset: 7},
				End:   Pos{Line: 1, Col: 9, Offset: 8},
			},
			Source: src,
		}
		expected := "  SELECT * FROM users\n         ^"
		got := d.Snippet()
		if got != expected {
			t.Errorf("Snippet() = %q, want %q", got, expected)
		}
	})

	t.Run("multi character span", func(t *testing.T) {
		d := &Diagnostic{
			Span: Span{
				Start: Pos{Line: 1, Col: 15, Offset: 14},
				End:   Pos{Line: 1, Col: 20, Offset: 19},
			},
			Source: src,
		}
		expected := "  SELECT * FROM users\n                ^^^^^"
		got := d.Snippet()
		if got != expected {
			t.Errorf("Snippet() = %q, want %q", got, expected)
		}
	})

	t.Run("multi-line span (unterminated literal)", func(t *testing.T) {
		d := &Diagnostic{
			Span: Span{
				Start: Pos{Line: 1, Col: 10, Offset: 9},
				End:   Pos{Line: 2, Col: 5, Offset: 24},
			},
			Source: src,
		}

		expected := "  SELECT * FROM users\n           ^^^^^^^^^^"
		got := d.Snippet()
		if got != expected {
			t.Errorf("Snippet() = %q, want %q", got, expected)
		}
	})

	t.Run("defaults to width 1 if end <= start", func(t *testing.T) {
		d := &Diagnostic{
			Span: Span{
				Start: Pos{Line: 2, Col: 7, Offset: 26},
				End:   Pos{Line: 2, Col: 7, Offset: 26},
			},
			Source: src,
		}
		expected := "  WHERE id = 1;\n        ^"
		got := d.Snippet()
		if got != expected {
			t.Errorf("Snippet() = %q, want %q", got, expected)
		}
	})

	t.Run("empty string when source is nil", func(t *testing.T) {
		d := &Diagnostic{
			Span: Span{
				Start: Pos{Line: 1, Col: 1},
				End:   Pos{Line: 1, Col: 2},
			},
			Source: nil,
		}
		if d.Snippet() != "" {
			t.Errorf("expected empty string, got %q", d.Snippet())
		}
	})

	t.Run("empty string when line number out of bounds", func(t *testing.T) {
		d := &Diagnostic{
			Span: Span{
				Start: Pos{Line: 99, Col: 1},
				End:   Pos{Line: 99, Col: 2},
			},
			Source: src,
		}
		if d.Snippet() != "" {
			t.Errorf("expected empty string, got %q", d.Snippet())
		}
	})
}

// TestDiagnostic_FullString tests diagnostic full string.
func TestDiagnostic_FullString(t *testing.T) {
	d := &Diagnostic{
		Severity: SeverityError,
		Code:     Code(1001),
		Category: "Illegal Character",
		Span: Span{
			Start: Pos{Line: 1, Col: 8, Offset: 7},
			End:   Pos{Line: 1, Col: 9, Offset: 8},
		},
		Msg:    "unexpected character '@'",
		Source: &Source{Name: "query", Text: "SELECT @ FROM users"},
	}

	expectedParts := []string{
		"1:8: error [E1001] Illegal Character: unexpected character '@'",
		"  SELECT @ FROM users",
		"         ^",
	}
	expected := strings.Join(expectedParts, "\n")
	got := d.FullString()
	if got != expected {
		t.Errorf("FullString() = %q, want %q", got, expected)
	}
}

// TestList tests list.
func TestList(t *testing.T) {
	d1 := &Diagnostic{
		Severity: SeverityError,
		Code:     Code(1001),
		Category: "Illegal Character",
		Span:     Span{Start: Pos{Line: 1, Col: 1}},
		Msg:      "unexpected '@'",
	}
	d2 := &Diagnostic{
		Severity: SeverityWarning,
		Code:     Code(2001),
		Category: "Parsing Warning",
		Span:     Span{Start: Pos{Line: 2, Col: 5}},
		Msg:      "deprecated syntax",
	}

	t.Run("empty list representation", func(t *testing.T) {
		var l List
		if l.Error() != "(no diagnostics)" {
			t.Errorf("empty list Error() = %q", l.Error())
		}
	})

	t.Run("multiple errors display formatting", func(t *testing.T) {
		var l List
		l.Append(d1)
		l.Append(d2)

		expected := d1.Error() + "\n" + d2.Error()
		if l.Error() != expected {
			t.Errorf("Error() = %q, want %q", l.Error(), expected)
		}
	})

	t.Run("HasErrors check", func(t *testing.T) {
		var l List
		if l.HasErrors() {
			t.Error("empty list should have no errors")
		}

		l.Append(d2) // warning only
		if l.HasErrors() {
			t.Error("list with only warnings should have no errors")
		}

		l.Append(d1) // error
		if !l.HasErrors() {
			t.Error("list with errors should report true")
		}
	})

	t.Run("Unwrap and errors.Is list traversal", func(t *testing.T) {
		var l List
		l.Append(d1)
		l.Append(d2)

		if !errors.Is(l, Code(1001)) {
			t.Error("errors.Is failed to find E1001 in List")
		}
		if !errors.Is(l, Code(2001)) {
			t.Error("errors.Is failed to find E2001 in List")
		}
		if errors.Is(l, Code(9999)) {
			t.Error("errors.Is matched non-existent E9999")
		}
	})
}
