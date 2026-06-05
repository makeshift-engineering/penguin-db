package lexer

import "fmt"

// TokenType represents the type of a lexical token.
// The underlying string value doubles as the human-readable name,
// so no separate name map is needed.
type TokenType string

const (
	// Special
	T_INVALID TokenType = "INVALID"
	T_EOF     TokenType = "EOF"

	// Identifiers and literals
	T_IDENT      TokenType = "IDENT"
	T_INT_LIT    TokenType = "INT_LIT"
	T_FLOAT_LIT  TokenType = "FLOAT_LIT"
	T_STRING_LIT TokenType = "STRING_LIT"

	// Arithmetic operators
	T_PLUS     TokenType = "+"
	T_MINUS    TokenType = "-"
	T_ASTERISK TokenType = "*"
	T_SLASH    TokenType = "/"
	T_MODULO   TokenType = "%"

	// Comparison operators
	T_EQUAL         TokenType = "="
	T_NOT_EQUAL     TokenType = "!="
	T_DIAMOND       TokenType = "<>"
	T_LESS          TokenType = "<"
	T_GREATER       TokenType = ">"
	T_LESS_EQUAL    TokenType = "<="
	T_GREATER_EQUAL TokenType = ">="

	// Delimiters
	T_COMMA  TokenType = ","
	T_SEMI   TokenType = ";"
	T_LPAREN TokenType = "("
	T_RPAREN TokenType = ")"

	// Database manipulation
	T_CREATE   TokenType = "CREATE"
	T_DROP     TokenType = "DROP"
	T_DATABASE TokenType = "DATABASE"
	T_USE      TokenType = "USE"

	// Table manipulation
	T_TABLE  TokenType = "TABLE"
	T_ALTER  TokenType = "ALTER"
	T_ADD    TokenType = "ADD"
	T_MODIFY TokenType = "MODIFY"
	T_COLUMN TokenType = "COLUMN"
	T_RENAME TokenType = "RENAME"
	T_TO     TokenType = "TO"

	// Column constraints
	T_PRIMARY TokenType = "PRIMARY"
	T_KEY     TokenType = "KEY"
	T_UNIQUE  TokenType = "UNIQUE"
	T_NOT     TokenType = "NOT"
	T_NULL    TokenType = "NULL"
	T_DEFAULT TokenType = "DEFAULT"

	// Data types
	T_INT       TokenType = "INT"
	T_BIGINT    TokenType = "BIGINT"
	T_VARCHAR   TokenType = "VARCHAR"
	T_BOOLEAN   TokenType = "BOOLEAN"
	T_TEXT      TokenType = "TEXT"
	T_TIMESTAMP TokenType = "TIMESTAMP"

	// SELECT
	T_SELECT TokenType = "SELECT"
	T_FROM   TokenType = "FROM"
	T_WHERE  TokenType = "WHERE"
	T_LIMIT  TokenType = "LIMIT"
	T_AS     TokenType = "AS"

	// INSERT
	T_INSERT TokenType = "INSERT"
	T_INTO   TokenType = "INTO"
	T_VALUES TokenType = "VALUES"

	// UPDATE / DELETE
	T_UPDATE TokenType = "UPDATE"
	T_SET    TokenType = "SET"
	T_DELETE TokenType = "DELETE"

	// Logical operators
	T_AND TokenType = "AND"
	T_OR  TokenType = "OR"

	// Literals
	T_TRUE  TokenType = "TRUE"
	T_FALSE TokenType = "FALSE"
)

// LookupKeyword returns the keyword TokenType for the given identifier string.
// If the string is not a keyword, it returns T_INVALID.
// The caller should pass the uppercased string.
//
// Since TokenType is a string and every keyword const equals its SQL text,
// we simply cast and check via a switch — no extra map or slice needed.
func LookupKeyword(ident string) TokenType {
	switch TokenType(ident) {
	case T_CREATE, T_DROP, T_DATABASE, T_USE,
		T_TABLE, T_ALTER, T_ADD, T_MODIFY, T_COLUMN, T_RENAME, T_TO,
		T_PRIMARY, T_KEY, T_UNIQUE, T_NOT, T_NULL, T_DEFAULT,
		T_INT, T_BIGINT, T_VARCHAR, T_BOOLEAN, T_TEXT, T_TIMESTAMP,
		T_SELECT, T_FROM, T_WHERE, T_LIMIT, T_AS,
		T_INSERT, T_INTO, T_VALUES,
		T_UPDATE, T_SET, T_DELETE,
		T_AND, T_OR,
		T_TRUE, T_FALSE:
		return TokenType(ident)
	default:
		return T_INVALID
	}
}

// Token represents a single lexical token produced by the lexer.
type Token struct {
	Type    TokenType // the type of the token
	Literal string    // the raw text of the token from the source input
	Pos     Position  // the position of the first character of the token
}

// String returns a human-readable representation of the token.
func (t Token) String() string {
	return fmt.Sprintf("%s(%q) at %d:%d", t.Type, t.Literal, t.Pos.Line, t.Pos.Column)
}
