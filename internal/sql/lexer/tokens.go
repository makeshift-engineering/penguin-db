package lexer

import "fmt"

// TokenType is an integer tag that identifies what kind of lexical unit a
// Token represents. Every terminal in the grammar maps to exactly one
// TokenType constant.
type TokenType int

const (
	// Special
	TOKEN_EOF     TokenType = iota // end of input; always the last token
	TOKEN_ILLEGAL                  // unrecognised character; carries the raw byte

	// Literals
	TOKEN_IDENT
	TOKEN_INTEGER
	TOKEN_FLOAT
	TOKEN_STRING

	// DDL / database keywords
	TOKEN_CREATE
	TOKEN_DATABASE
	TOKEN_USE
	TOKEN_DROP
	TOKEN_IF
	TOKEN_EXISTS
	TOKEN_TABLE
	TOKEN_ALTER
	TOKEN_ADD
	TOKEN_COLUMN
	TOKEN_MODIFY
	TOKEN_RENAME
	TOKEN_TO

	// DML keywords
	TOKEN_SELECT
	TOKEN_DISTINCT
	TOKEN_ALL
	TOKEN_FROM
	TOKEN_WHERE
	TOKEN_AS
	TOKEN_INSERT
	TOKEN_INTO
	TOKEN_VALUES
	TOKEN_UPDATE
	TOKEN_SET
	TOKEN_DELETE

	// JOIN keywords
	TOKEN_JOIN
	TOKEN_INNER
	TOKEN_LEFT
	TOKEN_RIGHT
	TOKEN_FULL
	TOKEN_OUTER
	TOKEN_CROSS
	TOKEN_ON

	// Clause keywords
	TOKEN_GROUP
	TOKEN_BY
	TOKEN_HAVING
	TOKEN_ORDER
	TOKEN_ASC
	TOKEN_DESC
	TOKEN_LIMIT
	TOKEN_OFFSET

	// Constraint / type keywords
	TOKEN_PRIMARY
	TOKEN_KEY
	TOKEN_NOT
	TOKEN_NULL
	TOKEN_DEFAULT
	TOKEN_UNIQUE
	TOKEN_REFERENCES

	// Logical / predicate keywords
	TOKEN_AND
	TOKEN_OR
	TOKEN_TRUE
	TOKEN_FALSE
	TOKEN_LIKE
	TOKEN_IS
	TOKEN_IN
	TOKEN_BETWEEN

	// Data-type keywords
	TOKEN_INT
	TOKEN_BIGINT
	TOKEN_VARCHAR
	TOKEN_BOOLEAN
	TOKEN_TEXT
	TOKEN_TIMESTAMP

	// Comparison operators
	TOKEN_EQ  // =
	TOKEN_NEQ // != or <>
	TOKEN_LT  // <
	TOKEN_GT  // >
	TOKEN_LTE // <=
	TOKEN_GTE // >=

	// Arithmetic operators
	TOKEN_PLUS    // +
	TOKEN_MINUS   // -
	TOKEN_STAR    // *
	TOKEN_SLASH   // /
	TOKEN_PERCENT // %

	// Punctuation
	TOKEN_LPAREN    // (
	TOKEN_RPAREN    // )
	TOKEN_COMMA     // ,
	TOKEN_DOT       // .
	TOKEN_SEMICOLON // ;
)

// tokenNames provides a human-readable label for each TokenType; used by
// String() and by test failure messages.
var tokenNames = map[TokenType]string{
	TOKEN_EOF:        "EOF",
	TOKEN_ILLEGAL:    "ILLEGAL",
	TOKEN_IDENT:      "IDENT",
	TOKEN_INTEGER:    "INTEGER",
	TOKEN_FLOAT:      "FLOAT",
	TOKEN_STRING:     "STRING",
	TOKEN_CREATE:     "CREATE",
	TOKEN_DATABASE:   "DATABASE",
	TOKEN_USE:        "USE",
	TOKEN_DROP:       "DROP",
	TOKEN_IF:         "IF",
	TOKEN_EXISTS:     "EXISTS",
	TOKEN_TABLE:      "TABLE",
	TOKEN_ALTER:      "ALTER",
	TOKEN_ADD:        "ADD",
	TOKEN_COLUMN:     "COLUMN",
	TOKEN_MODIFY:     "MODIFY",
	TOKEN_RENAME:     "RENAME",
	TOKEN_TO:         "TO",
	TOKEN_SELECT:     "SELECT",
	TOKEN_DISTINCT:   "DISTINCT",
	TOKEN_ALL:        "ALL",
	TOKEN_FROM:       "FROM",
	TOKEN_WHERE:      "WHERE",
	TOKEN_AS:         "AS",
	TOKEN_INSERT:     "INSERT",
	TOKEN_INTO:       "INTO",
	TOKEN_VALUES:     "VALUES",
	TOKEN_UPDATE:     "UPDATE",
	TOKEN_SET:        "SET",
	TOKEN_DELETE:     "DELETE",
	TOKEN_JOIN:       "JOIN",
	TOKEN_INNER:      "INNER",
	TOKEN_LEFT:       "LEFT",
	TOKEN_RIGHT:      "RIGHT",
	TOKEN_FULL:       "FULL",
	TOKEN_OUTER:      "OUTER",
	TOKEN_CROSS:      "CROSS",
	TOKEN_ON:         "ON",
	TOKEN_GROUP:      "GROUP",
	TOKEN_BY:         "BY",
	TOKEN_HAVING:     "HAVING",
	TOKEN_ORDER:      "ORDER",
	TOKEN_ASC:        "ASC",
	TOKEN_DESC:       "DESC",
	TOKEN_LIMIT:      "LIMIT",
	TOKEN_OFFSET:     "OFFSET",
	TOKEN_PRIMARY:    "PRIMARY",
	TOKEN_KEY:        "KEY",
	TOKEN_NOT:        "NOT",
	TOKEN_NULL:       "NULL",
	TOKEN_DEFAULT:    "DEFAULT",
	TOKEN_UNIQUE:     "UNIQUE",
	TOKEN_REFERENCES: "REFERENCES",
	TOKEN_AND:        "AND",
	TOKEN_OR:         "OR",
	TOKEN_TRUE:       "TRUE",
	TOKEN_FALSE:      "FALSE",
	TOKEN_LIKE:       "LIKE",
	TOKEN_IS:         "IS",
	TOKEN_IN:         "IN",
	TOKEN_BETWEEN:    "BETWEEN",
	TOKEN_INT:        "INT",
	TOKEN_BIGINT:     "BIGINT",
	TOKEN_VARCHAR:    "VARCHAR",
	TOKEN_BOOLEAN:    "BOOLEAN",
	TOKEN_TEXT:       "TEXT",
	TOKEN_TIMESTAMP:  "TIMESTAMP",
	TOKEN_EQ:         "=",
	TOKEN_NEQ:        "!=",
	TOKEN_LT:         "<",
	TOKEN_GT:         ">",
	TOKEN_LTE:        "<=",
	TOKEN_GTE:        ">=",
	TOKEN_PLUS:       "+",
	TOKEN_MINUS:      "-",
	TOKEN_STAR:       "*",
	TOKEN_SLASH:      "/",
	TOKEN_PERCENT:    "%",
	TOKEN_LPAREN:     "(",
	TOKEN_RPAREN:     ")",
	TOKEN_COMMA:      ",",
	TOKEN_DOT:        ".",
	TOKEN_SEMICOLON:  ";",
}

// String returns the human-readable name of a TokenType.
func (t TokenType) String() string {
	if s, ok := tokenNames[t]; ok {
		return s
	}
	return fmt.Sprintf("TokenType(%d)", int(t))
}

// Token is a single lexical unit produced by the Lexer.
type Token struct {
	Type    TokenType // what kind of token this is
	Literal string    // raw source text (string tokens have quotes stripped and
	//                   escapes decoded; keywords preserve their original casing)
	Line int // 1-based line number of the token's first character
	Col  int // 1-based column number of the token's first character
}

func (t Token) String() string {
	return fmt.Sprintf("Token{%-12s %q  %d:%d}", t.Type, t.Literal, t.Line, t.Col)
}
