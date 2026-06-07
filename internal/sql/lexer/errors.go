package lexer

import "errors"

var (
	// ErrUnexpectedToken is returned when the lexer encounters an unexpected token.
	ErrUnexpectedToken = errors.New("unexpected token")

	// ErrUnexpectedEOF is returned when the lexer encounters the end of input.
	ErrUnexpectedEOF = errors.New("unexpected EOF")

	// ErrUnterminatedString is returned when the lexer encounters an unterminated string literal.
	ErrUnterminatedString = errors.New("unterminated string")

	// ErrUnterminatedBlockComment is returned when the lexer encounters an unterminated block comment.
	ErrUnterminatedBlockComment = errors.New("unterminated block comment")
)
