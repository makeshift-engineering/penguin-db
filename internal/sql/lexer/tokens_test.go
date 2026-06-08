package lexer

import (
	"fmt"
	"testing"
)

// ---------- TokenType.String() -----------------------------------------------

func TestTokenType_String_KnownTypes(t *testing.T) {
	// Every entry in the tokenNames map should be returned by String().
	for tt, name := range tokenNames {
		t.Run(name, func(t *testing.T) {
			got := tt.String()
			if got != name {
				t.Errorf("TokenType(%d).String() = %q, want %q", int(tt), got, name)
			}
		})
	}
}

func TestTokenType_String_UnknownType(t *testing.T) {
	unknown := TokenType(9999)
	got := unknown.String()
	want := fmt.Sprintf("TokenType(%d)", 9999)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTokenType_String_AllTokenTypesHaveNames(t *testing.T) {
	// Walk through the iota range to ensure no gaps in the tokenNames map.
	// This uses the fact that all token types are contiguous iota values
	// from TOKEN_EOF (0) to TOKEN_SEMICOLON.
	allTokenTypes := []TokenType{
		TOKEN_EOF, TOKEN_ILLEGAL,
		TOKEN_IDENT, TOKEN_INTEGER, TOKEN_FLOAT, TOKEN_STRING,
		TOKEN_CREATE, TOKEN_DATABASE, TOKEN_USE, TOKEN_DROP, TOKEN_IF,
		TOKEN_EXISTS, TOKEN_TABLE, TOKEN_ALTER, TOKEN_ADD, TOKEN_COLUMN,
		TOKEN_MODIFY, TOKEN_RENAME, TOKEN_TO,
		TOKEN_SELECT, TOKEN_DISTINCT, TOKEN_ALL, TOKEN_FROM, TOKEN_WHERE,
		TOKEN_AS, TOKEN_INSERT, TOKEN_INTO, TOKEN_VALUES, TOKEN_UPDATE,
		TOKEN_SET, TOKEN_DELETE,
		TOKEN_JOIN, TOKEN_INNER, TOKEN_LEFT, TOKEN_RIGHT, TOKEN_FULL,
		TOKEN_OUTER, TOKEN_CROSS, TOKEN_ON,
		TOKEN_GROUP, TOKEN_BY, TOKEN_HAVING, TOKEN_ORDER, TOKEN_ASC,
		TOKEN_DESC, TOKEN_LIMIT, TOKEN_OFFSET,
		TOKEN_PRIMARY, TOKEN_KEY, TOKEN_NOT, TOKEN_NULL, TOKEN_DEFAULT,
		TOKEN_UNIQUE, TOKEN_REFERENCES,
		TOKEN_AND, TOKEN_OR, TOKEN_TRUE, TOKEN_FALSE, TOKEN_LIKE,
		TOKEN_IS, TOKEN_IN, TOKEN_BETWEEN,
		TOKEN_INT, TOKEN_BIGINT, TOKEN_VARCHAR, TOKEN_BOOLEAN, TOKEN_TEXT,
		TOKEN_TIMESTAMP,
		TOKEN_EQ, TOKEN_NEQ, TOKEN_LT, TOKEN_GT, TOKEN_LTE, TOKEN_GTE,
		TOKEN_PLUS, TOKEN_MINUS, TOKEN_STAR, TOKEN_SLASH, TOKEN_PERCENT,
		TOKEN_LPAREN, TOKEN_RPAREN, TOKEN_COMMA, TOKEN_DOT, TOKEN_SEMICOLON,
	}
	for _, tt := range allTokenTypes {
		name := tt.String()
		// The fallback is "TokenType(<int>)". If we see that, the map is incomplete.
		if name == fmt.Sprintf("TokenType(%d)", int(tt)) {
			t.Errorf("TokenType %d has no human-readable name in tokenNames", int(tt))
		}
	}
}

// ---------- Token.String() ---------------------------------------------------

func TestToken_String(t *testing.T) {
	tests := []struct {
		token Token
		want  string
	}{
		{
			Token{Type: TOKEN_SELECT, Literal: "SELECT", Line: 1, Col: 1},
			`Token{SELECT       "SELECT"  1:1}`,
		},
		{
			Token{Type: TOKEN_INTEGER, Literal: "42", Line: 3, Col: 15},
			`Token{INTEGER      "42"  3:15}`,
		},
		{
			Token{Type: TOKEN_STRING, Literal: "hello", Line: 1, Col: 10},
			`Token{STRING       "hello"  1:10}`,
		},
		{
			Token{Type: TOKEN_EOF, Literal: "", Line: 5, Col: 1},
			`Token{EOF          ""  5:1}`,
		},
		{
			Token{Type: TOKEN_ILLEGAL, Literal: "@", Line: 1, Col: 1},
			`Token{ILLEGAL      "@"  1:1}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			got := tc.token.String()
			if got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

// ---------- lookupIdent (keywords.go) ----------------------------------------

func TestLookupIdent_ReturnsKeywordType(t *testing.T) {
	for word, expected := range keywords {
		got := lookupIdent(word)
		if got != expected {
			t.Errorf("lookupIdent(%q) = %v, want %v", word, got, expected)
		}
	}
}

func TestLookupIdent_CaseInsensitive(t *testing.T) {
	tests := []struct {
		input string
		want  TokenType
	}{
		{"select", TOKEN_SELECT},
		{"SELECT", TOKEN_SELECT},
		{"SeLeCt", TOKEN_SELECT},
		{"from", TOKEN_FROM},
		{"FROM", TOKEN_FROM},
		{"fRoM", TOKEN_FROM},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := lookupIdent(tc.input)
			if got != tc.want {
				t.Errorf("lookupIdent(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestLookupIdent_ReturnsIdentForNonKeywords(t *testing.T) {
	nonKeywords := []string{
		"foo", "bar", "my_table", "userId", "x", "_private",
		"selection", "fromage", "orderly", "deleteme",
	}
	for _, word := range nonKeywords {
		got := lookupIdent(word)
		if got != TOKEN_IDENT {
			t.Errorf("lookupIdent(%q) = %v, want TOKEN_IDENT", word, got)
		}
	}
}
