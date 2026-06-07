package lexer

import "strings"

// keywords maps the canonical (upper-case) spelling of every reserved word to
// its TokenType. The lookup is always done on the upper-cased form of whatever
// the source contained, giving the grammar its case-insensitive keyword
// semantics while leaving Token.Literal in its original casing.
var keywords = map[string]TokenType{
	// DDL / database
	"CREATE":   TOKEN_CREATE,
	"DATABASE": TOKEN_DATABASE,
	"USE":      TOKEN_USE,
	"DROP":     TOKEN_DROP,
	"IF":       TOKEN_IF,
	"EXISTS":   TOKEN_EXISTS,

	// Table DDL
	"TABLE":  TOKEN_TABLE,
	"ALTER":  TOKEN_ALTER,
	"ADD":    TOKEN_ADD,
	"COLUMN": TOKEN_COLUMN,
	"MODIFY": TOKEN_MODIFY,
	"RENAME": TOKEN_RENAME,
	"TO":     TOKEN_TO,

	// DML
	"SELECT":   TOKEN_SELECT,
	"DISTINCT": TOKEN_DISTINCT,
	"ALL":      TOKEN_ALL,
	"FROM":     TOKEN_FROM,
	"WHERE":    TOKEN_WHERE,
	"AS":       TOKEN_AS,
	"INSERT":   TOKEN_INSERT,
	"INTO":     TOKEN_INTO,
	"VALUES":   TOKEN_VALUES,
	"UPDATE":   TOKEN_UPDATE,
	"SET":      TOKEN_SET,
	"DELETE":   TOKEN_DELETE,

	// JOIN
	"JOIN":  TOKEN_JOIN,
	"INNER": TOKEN_INNER,
	"LEFT":  TOKEN_LEFT,
	"RIGHT": TOKEN_RIGHT,
	"FULL":  TOKEN_FULL,
	"OUTER": TOKEN_OUTER,
	"CROSS": TOKEN_CROSS,
	"ON":    TOKEN_ON,

	// Clauses
	"GROUP":  TOKEN_GROUP,
	"BY":     TOKEN_BY,
	"HAVING": TOKEN_HAVING,
	"ORDER":  TOKEN_ORDER,
	"ASC":    TOKEN_ASC,
	"DESC":   TOKEN_DESC,
	"LIMIT":  TOKEN_LIMIT,
	"OFFSET": TOKEN_OFFSET,

	// Constraints
	"PRIMARY":    TOKEN_PRIMARY,
	"KEY":        TOKEN_KEY,
	"NOT":        TOKEN_NOT,
	"NULL":       TOKEN_NULL,
	"DEFAULT":    TOKEN_DEFAULT,
	"UNIQUE":     TOKEN_UNIQUE,
	"REFERENCES": TOKEN_REFERENCES,

	// Logical / predicates
	"AND":     TOKEN_AND,
	"OR":      TOKEN_OR,
	"TRUE":    TOKEN_TRUE,
	"FALSE":   TOKEN_FALSE,
	"LIKE":    TOKEN_LIKE,
	"IS":      TOKEN_IS,
	"IN":      TOKEN_IN,
	"BETWEEN": TOKEN_BETWEEN,

	// Data types
	"INT":       TOKEN_INT,
	"BIGINT":    TOKEN_BIGINT,
	"VARCHAR":   TOKEN_VARCHAR,
	"BOOLEAN":   TOKEN_BOOLEAN,
	"TEXT":      TOKEN_TEXT,
	"TIMESTAMP": TOKEN_TIMESTAMP,
}

// lookupIdent returns the keyword TokenType for s if it is a reserved word,
// or TOKEN_IDENT if it is a plain user-defined name.
// The comparison is case-insensitive: "select", "SELECT", and "SeLeCt" all
// resolve to TOKEN_SELECT.
func lookupIdent(s string) TokenType {
	if tt, ok := keywords[strings.ToUpper(s)]; ok {
		return tt
	}
	return TOKEN_IDENT
}
