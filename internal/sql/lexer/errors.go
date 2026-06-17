package lexer

import "github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"

// Lexer error codes occupy the 1000–1999 range.
const (
	ErrUnexpectedChar      diagnostic.Code = 1001
	ErrUnterminatedString  diagnostic.Code = 1002
	ErrUnterminatedComment diagnostic.Code = 1003
)
