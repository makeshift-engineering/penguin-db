package utils

import (
	"fmt"
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
)

// TestTokenType_String_KnownTypes tests token type string known types.
func TestTokenType_String_KnownTypes(t *testing.T) {
	// Every entry in the tokenTable should be returned by String().
	for i := 0; i < int(tokenTypeSentinel); i++ {
		tt := TokenType(i)
		name := tokenTable[i].name
		t.Run(name, func(t *testing.T) {
			got := tt.String()
			if got != name {
				t.Errorf("TokenType(%d).String() = %q, want %q", int(tt), got, name)
			}
		})
	}
}

// TestTokenType_String_UnknownType tests token type string unknown type.
func TestTokenType_String_UnknownType(t *testing.T) {
	unknown := TokenType(9999)
	got := unknown.String()
	want := fmt.Sprintf("TokenType(%d)", 9999)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestTokenType_String_AllTokenTypesHaveNames tests token type string all token types have names.
func TestTokenType_String_AllTokenTypesHaveNames(t *testing.T) {
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
		// The fallback is "TokenType(<int>)". If we see that, the definition is incomplete.
		if name == fmt.Sprintf("TokenType(%d)", int(tt)) {
			t.Errorf("TokenType %d has no human-readable name in tokenTable", int(tt))
		}
	}
}

// TestToken_String tests token string.
func TestToken_String(t *testing.T) {
	tests := []struct {
		token Token
		want  string
	}{
		{
			Token{
				Type:    TOKEN_SELECT,
				Literal: "SELECT",
				Span: diagnostic.Span{
					Start: diagnostic.Pos{Line: 1, Col: 1},
					End:   diagnostic.Pos{Line: 1, Col: 7},
				},
			},
			`Token{SELECT       "SELECT"  1:1-1:7}`,
		},
		{
			Token{
				Type:    TOKEN_INTEGER,
				Literal: "42",
				Span: diagnostic.Span{
					Start: diagnostic.Pos{Line: 3, Col: 15},
					End:   diagnostic.Pos{Line: 3, Col: 17},
				},
			},
			`Token{INTEGER      "42"  3:15-3:17}`,
		},
		{
			Token{
				Type:    TOKEN_STRING,
				Literal: "hello",
				Span: diagnostic.Span{
					Start: diagnostic.Pos{Line: 1, Col: 10},
					End:   diagnostic.Pos{Line: 1, Col: 17},
				},
			},
			`Token{STRING       "hello"  1:10-1:17}`,
		},
		{
			Token{
				Type:    TOKEN_EOF,
				Literal: "",
				Span: diagnostic.Span{
					Start: diagnostic.Pos{Line: 5, Col: 1},
					End:   diagnostic.Pos{Line: 5, Col: 1},
				},
			},
			`Token{EOF          ""  5:1-5:1}`,
		},
		{
			Token{
				Type:    TOKEN_ILLEGAL,
				Literal: "@",
				Span: diagnostic.Span{
					Start: diagnostic.Pos{Line: 1, Col: 1},
					End:   diagnostic.Pos{Line: 1, Col: 2},
				},
			},
			`Token{ILLEGAL      "@"  1:1-1:2}`,
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

// TestLookupIdent_ReturnsKeywordType tests lookup ident returns keyword type.
func TestLookupIdent_ReturnsKeywordType(t *testing.T) {
	for word, expected := range keywords {
		got := LookupIdent(word)
		if got != expected {
			t.Errorf("lookupIdent(%q) = %v, want %v", word, got, expected)
		}
	}
}

// TestLookupIdent_CaseInsensitive tests lookup ident case insensitive.
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
			got := LookupIdent(tc.input)
			if got != tc.want {
				t.Errorf("lookupIdent(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// TestLookupIdent_ReturnsIdentForNonKeywords tests lookup ident returns ident for non keywords.
func TestLookupIdent_ReturnsIdentForNonKeywords(t *testing.T) {
	nonKeywords := []string{
		"foo", "bar", "my_table", "userId", "x", "_private",
		"selection", "fromage", "orderly", "deleteme",
	}
	for _, word := range nonKeywords {
		got := LookupIdent(word)
		if got != TOKEN_IDENT {
			t.Errorf("lookupIdent(%q) = %v, want TOKEN_IDENT", word, got)
		}
	}
}

// TestTokenType_Classifications tests token type classifications.
func TestTokenType_Classifications(t *testing.T) {
	tests := []struct {
		name       string
		tokenType  TokenType
		isKeyword  bool
		isLiteral  bool
		isOperator bool
		isPunct    bool
		isSpecial  bool
	}{
		{"EOF", TOKEN_EOF, false, false, false, false, true},
		{"ILLEGAL", TOKEN_ILLEGAL, false, false, false, false, true},
		{"IDENT", TOKEN_IDENT, false, true, false, false, false},
		{"INTEGER", TOKEN_INTEGER, false, true, false, false, false},
		{"CREATE", TOKEN_CREATE, true, false, false, false, false},
		{"SELECT", TOKEN_SELECT, true, false, false, false, false},
		{"EQ", TOKEN_EQ, false, false, true, false, false},
		{"PLUS", TOKEN_PLUS, false, false, true, false, false},
		{"LPAREN", TOKEN_LPAREN, false, false, false, true, false},
		{"SEMICOLON", TOKEN_SEMICOLON, false, false, false, true, false},
		{"UNKNOWN", TokenType(9999), false, false, false, false, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.tokenType.IsKeyword(); got != tc.isKeyword {
				t.Errorf("IsKeyword() = %v, want %v", got, tc.isKeyword)
			}
			if got := tc.tokenType.IsLiteral(); got != tc.isLiteral {
				t.Errorf("IsLiteral() = %v, want %v", got, tc.isLiteral)
			}
			if got := tc.tokenType.IsOperator(); got != tc.isOperator {
				t.Errorf("IsOperator() = %v, want %v", got, tc.isOperator)
			}
			if got := tc.tokenType.IsPunct(); got != tc.isPunct {
				t.Errorf("IsPunct() = %v, want %v", got, tc.isPunct)
			}
			if got := tc.tokenType.IsSpecial(); got != tc.isSpecial {
				t.Errorf("IsSpecial() = %v, want %v", got, tc.isSpecial)
			}
		})
	}
}
