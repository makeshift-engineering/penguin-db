package parser

import (
	"errors"
	"reflect"
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
	"github.com/makeshift-engineering/penguin-db/internal/sql/lexer"
)

// parseForTest runs the lexer and parser on the given input, returning the resulting AST.
func parseForTest(t *testing.T, input string) (*ast.Program, error) {
	t.Helper()
	l := lexer.NewLexer("test", input)
	tokens := l.Tokenize()
	if l.Diagnostics().HasErrors() {
		return nil, l.Diagnostics().AsError()
	}

	p := New(tokens, &diagnostic.Source{Name: "test", Text: input})
	prog, err := p.Parse()
	if err != nil {
		return nil, err
	}
	return prog, nil
}

// requireAST runs the parser on the input and asserts that it matches the expected AST.
// Spans are ignored in the comparison.
func requireAST(t *testing.T, input string, expected *ast.Program) {
	t.Helper()
	actual, err := parseForTest(t, input)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if !deepEqualNoSpan(reflect.ValueOf(expected), reflect.ValueOf(actual)) {
		t.Errorf("AST mismatch for %q\nExpected: %#v\nActual:   %#v", input, expected, actual)
	}
}

// requireParseError runs the parser and asserts that it produces a specific error.
func requireParseError(t *testing.T, input string, expectedErr error) {
	t.Helper()
	_, err := parseForTest(t, input)
	if err == nil {
		t.Fatalf("expected error wrapping %v, got none", expectedErr)
	}
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error wrapping %v, got: %v", expectedErr, err)
	}
}

var spanType = reflect.TypeOf(diagnostic.Span{})

func deepEqualNoSpan(v1, v2 reflect.Value) bool {
	if !v1.IsValid() || !v2.IsValid() {
		return v1.IsValid() == v2.IsValid()
	}
	if v1.Type() != v2.Type() {
		return false
	}

	if v1.Type() == spanType {
		return true // skip span comparison
	}

	switch v1.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v1.IsNil() || v2.IsNil() {
			return v1.IsNil() == v2.IsNil()
		}
		return deepEqualNoSpan(v1.Elem(), v2.Elem())
	case reflect.Slice:
		if v1.IsNil() != v2.IsNil() {
			return false
		}
		if v1.Len() != v2.Len() {
			return false
		}
		for i := 0; i < v1.Len(); i++ {
			if !deepEqualNoSpan(v1.Index(i), v2.Index(i)) {
				return false
			}
		}
		return true
	case reflect.Struct:
		for i := 0; i < v1.NumField(); i++ {
			if !deepEqualNoSpan(v1.Field(i), v2.Field(i)) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(v1.Interface(), v2.Interface())
	}
}

func ptr[T any](v T) *T {
	return &v
}
