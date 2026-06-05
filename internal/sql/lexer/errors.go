package lexer

import "errors"

var (
	// ErrUnexpectedToken is returned when the lexer encounters an unexpected token.
	ErrUnexpectedToken = errors.New("unexpected token")
)
