package lexer

import (
	"fmt"

	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
)

// TokenType is an integer tag that identifies what kind of lexical unit a
// Token represents. Every terminal in the grammar maps to exactly one
// TokenType constant.
type TokenType int

//nolint:revive // We prefer ALL_CAPS for token constants
const (
	// Special
	TOKEN_EOF TokenType = iota // end of input; always the last token
	TOKEN_ILLEGAL              // unrecognised character; carries the raw byte

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

	tokenTypeSentinel // always last; never a real token; gives len of enum
)

// tokenClass categorises what role a token plays.
type tokenClass uint8

const (
	classSpecial tokenClass = iota
	classLiteral
	classKeyword
	classOperator
	classPunct
)

type tokenDef struct {
	name  string
	class tokenClass
}

// tokenTable is the single source of truth for display names and classes.
// Indexed by iota value — O(1) lookup.
var tokenTable = [...]tokenDef{
	// Special
	TOKEN_EOF:     {name: "EOF", class: classSpecial},
	TOKEN_ILLEGAL: {name: "ILLEGAL", class: classSpecial},

	// Literals
	TOKEN_IDENT:   {name: "IDENT", class: classLiteral},
	TOKEN_INTEGER: {name: "INTEGER", class: classLiteral},
	TOKEN_FLOAT:   {name: "FLOAT", class: classLiteral},
	TOKEN_STRING:  {name: "STRING", class: classLiteral},

	// DDL keywords
	TOKEN_CREATE:   {name: "CREATE", class: classKeyword},
	TOKEN_DATABASE: {name: "DATABASE", class: classKeyword},
	TOKEN_USE:      {name: "USE", class: classKeyword},
	TOKEN_DROP:     {name: "DROP", class: classKeyword},
	TOKEN_IF:       {name: "IF", class: classKeyword},
	TOKEN_EXISTS:   {name: "EXISTS", class: classKeyword},
	TOKEN_TABLE:    {name: "TABLE", class: classKeyword},
	TOKEN_ALTER:    {name: "ALTER", class: classKeyword},
	TOKEN_ADD:      {name: "ADD", class: classKeyword},
	TOKEN_COLUMN:   {name: "COLUMN", class: classKeyword},
	TOKEN_MODIFY:   {name: "MODIFY", class: classKeyword},
	TOKEN_RENAME:   {name: "RENAME", class: classKeyword},
	TOKEN_TO:       {name: "TO", class: classKeyword},

	// DML keywords
	TOKEN_SELECT:   {name: "SELECT", class: classKeyword},
	TOKEN_DISTINCT: {name: "DISTINCT", class: classKeyword},
	TOKEN_ALL:      {name: "ALL", class: classKeyword},
	TOKEN_FROM:     {name: "FROM", class: classKeyword},
	TOKEN_WHERE:    {name: "WHERE", class: classKeyword},
	TOKEN_AS:       {name: "AS", class: classKeyword},
	TOKEN_INSERT:   {name: "INSERT", class: classKeyword},
	TOKEN_INTO:     {name: "INTO", class: classKeyword},
	TOKEN_VALUES:   {name: "VALUES", class: classKeyword},
	TOKEN_UPDATE:   {name: "UPDATE", class: classKeyword},
	TOKEN_SET:      {name: "SET", class: classKeyword},
	TOKEN_DELETE:   {name: "DELETE", class: classKeyword},

	// JOIN keywords
	TOKEN_JOIN:  {name: "JOIN", class: classKeyword},
	TOKEN_INNER: {name: "INNER", class: classKeyword},
	TOKEN_LEFT:  {name: "LEFT", class: classKeyword},
	TOKEN_RIGHT: {name: "RIGHT", class: classKeyword},
	TOKEN_FULL:  {name: "FULL", class: classKeyword},
	TOKEN_OUTER: {name: "OUTER", class: classKeyword},
	TOKEN_CROSS: {name: "CROSS", class: classKeyword},
	TOKEN_ON:    {name: "ON", class: classKeyword},

	// Clause keywords
	TOKEN_GROUP:  {name: "GROUP", class: classKeyword},
	TOKEN_BY:     {name: "BY", class: classKeyword},
	TOKEN_HAVING: {name: "HAVING", class: classKeyword},
	TOKEN_ORDER:  {name: "ORDER", class: classKeyword},
	TOKEN_ASC:    {name: "ASC", class: classKeyword},
	TOKEN_DESC:   {name: "DESC", class: classKeyword},
	TOKEN_LIMIT:  {name: "LIMIT", class: classKeyword},
	TOKEN_OFFSET: {name: "OFFSET", class: classKeyword},

	// Constraint / type keywords
	TOKEN_PRIMARY:    {name: "PRIMARY", class: classKeyword},
	TOKEN_KEY:        {name: "KEY", class: classKeyword},
	TOKEN_NOT:        {name: "NOT", class: classKeyword},
	TOKEN_NULL:       {name: "NULL", class: classKeyword},
	TOKEN_DEFAULT:    {name: "DEFAULT", class: classKeyword},
	TOKEN_UNIQUE:     {name: "UNIQUE", class: classKeyword},
	TOKEN_REFERENCES: {name: "REFERENCES", class: classKeyword},

	// Logical / predicate keywords
	TOKEN_AND:     {name: "AND", class: classKeyword},
	TOKEN_OR:      {name: "OR", class: classKeyword},
	TOKEN_TRUE:    {name: "TRUE", class: classKeyword},
	TOKEN_FALSE:   {name: "FALSE", class: classKeyword},
	TOKEN_LIKE:    {name: "LIKE", class: classKeyword},
	TOKEN_IS:      {name: "IS", class: classKeyword},
	TOKEN_IN:      {name: "IN", class: classKeyword},
	TOKEN_BETWEEN: {name: "BETWEEN", class: classKeyword},

	// Data-type keywords
	TOKEN_INT:       {name: "INT", class: classKeyword},
	TOKEN_BIGINT:    {name: "BIGINT", class: classKeyword},
	TOKEN_VARCHAR:   {name: "VARCHAR", class: classKeyword},
	TOKEN_BOOLEAN:   {name: "BOOLEAN", class: classKeyword},
	TOKEN_TEXT:      {name: "TEXT", class: classKeyword},
	TOKEN_TIMESTAMP: {name: "TIMESTAMP", class: classKeyword},

	// Comparison operators
	TOKEN_EQ:  {name: "=", class: classOperator},
	TOKEN_NEQ: {name: "!=", class: classOperator},
	TOKEN_LT:  {name: "<", class: classOperator},
	TOKEN_GT:  {name: ">", class: classOperator},
	TOKEN_LTE: {name: "<=", class: classOperator},
	TOKEN_GTE: {name: ">=", class: classOperator},

	// Arithmetic operators
	TOKEN_PLUS:    {name: "+", class: classOperator},
	TOKEN_MINUS:   {name: "-", class: classOperator},
	TOKEN_STAR:    {name: "*", class: classOperator},
	TOKEN_SLASH:   {name: "/", class: classOperator},
	TOKEN_PERCENT: {name: "%", class: classOperator},

	// Punctuation
	TOKEN_LPAREN:    {name: "(", class: classPunct},
	TOKEN_RPAREN:    {name: ")", class: classPunct},
	TOKEN_COMMA:     {name: ",", class: classPunct},
	TOKEN_DOT:       {name: ".", class: classPunct},
	TOKEN_SEMICOLON: {name: ";", class: classPunct},
}

// tokenNames is built from tokenTable for backwards compatibility/tests.
var tokenNames = map[TokenType]string{}

func init() {
	for i := 0; i < int(tokenTypeSentinel); i++ {
		def := tokenTable[i]
		if def.name == "" {
			panic(fmt.Sprintf("lexer: TokenType %d has no entry in tokenTable", i))
		}
		tokenNames[TokenType(i)] = def.name
	}
}

// String returns the human-readable name of a TokenType.
func (t TokenType) String() string {
	if i := int(t); i >= 0 && i < len(tokenTable) {
		if name := tokenTable[i].name; name != "" {
			return name
		}
	}
	return fmt.Sprintf("TokenType(%d)", int(t))
}

// IsKeyword returns true if the token is a keyword.
func (t TokenType) IsKeyword() bool {
	return t.class() == classKeyword
}

// IsLiteral returns true if the token is a literal (IDENT, INTEGER, FLOAT, STRING).
func (t TokenType) IsLiteral() bool {
	return t.class() == classLiteral
}

// IsOperator returns true if the token is an operator.
func (t TokenType) IsOperator() bool {
	return t.class() == classOperator
}

// IsPunct returns true if the token is punctuation.
func (t TokenType) IsPunct() bool {
	return t.class() == classPunct
}

// IsSpecial returns true if the token is special (EOF, ILLEGAL).
func (t TokenType) IsSpecial() bool {
	return t.class() == classSpecial
}

func (t TokenType) class() tokenClass {
	if i := int(t); i >= 0 && i < len(tokenTable) {
		return tokenTable[i].class
	}
	return classSpecial
}

// Token is a single lexical unit produced by the Lexer.
type Token struct {
	Type    TokenType       // what kind of token this is
	Literal string          // raw source text (string tokens have quotes stripped and escapes decoded)
	Span    diagnostic.Span // start and end range of this token
}

func (t Token) String() string {
	return fmt.Sprintf("Token{%-12s %q  %d:%d-%d:%d}",
		t.Type, t.Literal,
		t.Span.Start.Line, t.Span.Start.Col,
		t.Span.End.Line, t.Span.End.Col)
}

// position is the internal scanning cursor. Not exported.
type position struct {
	index  int // absolute 0-based index from start of input
	line   int // 1-based line number
	column int // 1-based column number
}

// advance updates the cursor after consuming ch.
func (p *position) advance(ch rune, size int) {
	p.index += size
	if ch == '\n' {
		p.line++
		p.column = 1
	} else {
		p.column++
	}
}

// snapshot captures the current cursor as an immutable Pos.
func (p position) snapshot() diagnostic.Pos {
	return diagnostic.Pos{Line: p.line, Col: p.column, Offset: p.index}
}
