package lexer

import (
	"errors"
	"fmt"
)

// Sentinel errors — compare with errors.Is, never inspect the message string.
var (
	ErrUnexpectedChar      = errors.New("unexpected character")
	ErrUnterminatedString  = errors.New("unterminated string literal")
	ErrUnterminatedComment = errors.New("unterminated block comment")
)

// LexError wraps a sentinel with the source location and a message.
type LexError struct {
	Err  error
	Line int
	Col  int
	Msg  string
}

func (e *LexError) Error() string {
	return fmt.Sprintf("%d:%d: %s: %s", e.Line, e.Col, e.Err.Error(), e.Msg)
}

// Unwrap lets errors.Is / errors.As traverse to the sentinel.
func (e *LexError) Unwrap() error { return e.Err }

// lexErr — Msg now carries only the specific detail, not a repetition of the sentinel.
func lexErr(sentinel error, line, col int, format string, args ...any) *LexError {
	return &LexError{
		Err:  sentinel,
		Line: line,
		Col:  col,
		Msg:  fmt.Sprintf(format, args...),
	}
}
