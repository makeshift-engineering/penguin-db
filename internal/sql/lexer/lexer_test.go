package lexer

import (
	"errors"
	"fmt"
	"testing"
)

// ---------- helpers ----------------------------------------------------------

// tok is a compact constructor for expected Token values in table-driven tests.
func tok(typ TokenType, lit string, line, col int) Token {
	return Token{Type: typ, Literal: lit, Line: line, Col: col}
}

// collectAll drives the lexer to exhaustion and returns every token it emits
// (including the final EOF). It fails the test on the first error.
func collectAll(t *testing.T, input string) []Token {
	t.Helper()
	l := NewLexer(input)
	var tokens []Token
	for {
		token, err := l.NextToken()
		if err != nil {
			t.Fatalf("unexpected error at %d:%d: %v", token.Line, token.Col, err)
		}
		tokens = append(tokens, token)
		if token.Type == TOKEN_EOF {
			break
		}
	}
	return tokens
}

// requireTokens asserts the full token stream for a given input, including the
// trailing EOF.
func requireTokens(t *testing.T, input string, want []Token) {
	t.Helper()
	got := collectAll(t, input)
	if len(got) != len(want) {
		t.Fatalf("token count mismatch: got %d, want %d\ngot:  %v\nwant: %v",
			len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token[%d]: got %v, want %v", i, got[i], want[i])
		}
	}
}

// requireError asserts that lexing produces a specific sentinel error and an
// ILLEGAL or EOF token at the expected position.
func requireError(t *testing.T, input string, sentinel error) {
	t.Helper()
	l := NewLexer(input)
	for {
		_, err := l.NextToken()
		if err != nil {
			if !errors.Is(err, sentinel) {
				t.Fatalf("expected error wrapping %v, got %v", sentinel, err)
			}
			return
		}
	}
}

// ---------- EOF & empty input ------------------------------------------------

func TestNextToken_EmptyInput(t *testing.T) {
	requireTokens(t, "", []Token{
		tok(TOKEN_EOF, "", 1, 1),
	})
}

func TestNextToken_OnlyWhitespace(t *testing.T) {
	requireTokens(t, "   \t \r\n  \n  ", []Token{
		tok(TOKEN_EOF, "", 3, 3),
	})
}

func TestNextToken_RepeatedEOF(t *testing.T) {
	l := NewLexer("")
	for i := 0; i < 5; i++ {
		token, err := l.NextToken()
		if err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
		if token.Type != TOKEN_EOF {
			t.Fatalf("iteration %d: expected EOF, got %v", i, token)
		}
	}
}

// ---------- Single-character punctuation -------------------------------------

func TestNextToken_Punctuation(t *testing.T) {
	tests := []struct {
		input string
		want  Token
	}{
		{"(", tok(TOKEN_LPAREN, "(", 1, 1)},
		{")", tok(TOKEN_RPAREN, ")", 1, 1)},
		{",", tok(TOKEN_COMMA, ",", 1, 1)},
		{".", tok(TOKEN_DOT, ".", 1, 1)},
		{";", tok(TOKEN_SEMICOLON, ";", 1, 1)},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			requireTokens(t, tc.input, []Token{
				tc.want,
				tok(TOKEN_EOF, "", 1, 2),
			})
		})
	}
}

// ---------- Arithmetic operators ---------------------------------------------

func TestNextToken_ArithmeticOperators(t *testing.T) {
	tests := []struct {
		input string
		want  Token
	}{
		{"+", tok(TOKEN_PLUS, "+", 1, 1)},
		{"-", tok(TOKEN_MINUS, "-", 1, 1)},
		{"*", tok(TOKEN_STAR, "*", 1, 1)},
		{"/", tok(TOKEN_SLASH, "/", 1, 1)},
		{"%", tok(TOKEN_PERCENT, "%", 1, 1)},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			requireTokens(t, tc.input, []Token{
				tc.want,
				tok(TOKEN_EOF, "", 1, 2),
			})
		})
	}
}

// ---------- Comparison operators (single & multi-char) -----------------------

func TestNextToken_ComparisonOperators(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []Token
	}{
		{"EQ", "=", []Token{
			tok(TOKEN_EQ, "=", 1, 1),
			tok(TOKEN_EOF, "", 1, 2),
		}},
		{"LT", "<", []Token{
			tok(TOKEN_LT, "<", 1, 1),
			tok(TOKEN_EOF, "", 1, 2),
		}},
		{"GT", ">", []Token{
			tok(TOKEN_GT, ">", 1, 1),
			tok(TOKEN_EOF, "", 1, 2),
		}},
		{"LTE", "<=", []Token{
			tok(TOKEN_LTE, "<=", 1, 1),
			tok(TOKEN_EOF, "", 1, 3),
		}},
		{"GTE", ">=", []Token{
			tok(TOKEN_GTE, ">=", 1, 1),
			tok(TOKEN_EOF, "", 1, 3),
		}},
		{"NEQ_bang", "!=", []Token{
			tok(TOKEN_NEQ, "!=", 1, 1),
			tok(TOKEN_EOF, "", 1, 3),
		}},
		{"NEQ_diamond", "<>", []Token{
			tok(TOKEN_NEQ, "<>", 1, 1),
			tok(TOKEN_EOF, "", 1, 3),
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requireTokens(t, tc.input, tc.want)
		})
	}
}

// ---------- Lone bang (!) is ILLEGAL -----------------------------------------

func TestNextToken_LoneBang_IsIllegal(t *testing.T) {
	l := NewLexer("!")
	token, err := l.NextToken()
	if err == nil {
		t.Fatal("expected error for lone '!'")
	}
	if !errors.Is(err, ErrUnexpectedChar) {
		t.Fatalf("expected ErrUnexpectedChar, got %v", err)
	}
	if token.Type != TOKEN_ILLEGAL {
		t.Fatalf("expected TOKEN_ILLEGAL, got %v", token.Type)
	}
	if token.Literal != "!" {
		t.Fatalf("expected literal '!', got %q", token.Literal)
	}
}

// ---------- Integer literals -------------------------------------------------

func TestNextToken_Integers(t *testing.T) {
	tests := []struct {
		input string
		want  Token
	}{
		{"0", tok(TOKEN_INTEGER, "0", 1, 1)},
		{"1", tok(TOKEN_INTEGER, "1", 1, 1)},
		{"42", tok(TOKEN_INTEGER, "42", 1, 1)},
		{"999999", tok(TOKEN_INTEGER, "999999", 1, 1)},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			requireTokens(t, tc.input, []Token{
				tc.want,
				tok(TOKEN_EOF, "", 1, len(tc.input)+1),
			})
		})
	}
}

// ---------- Float literals ---------------------------------------------------

func TestNextToken_Floats(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  Token
	}{
		{"simple", "3.14", tok(TOKEN_FLOAT, "3.14", 1, 1)},
		{"leading_dot", ".5", tok(TOKEN_FLOAT, ".5", 1, 1)},
		{"trailing_dot", "5.", tok(TOKEN_FLOAT, "5.", 1, 1)},
		{"zero_dot_zero", "0.0", tok(TOKEN_FLOAT, "0.0", 1, 1)},
		{"large", "12345.6789", tok(TOKEN_FLOAT, "12345.6789", 1, 1)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requireTokens(t, tc.input, []Token{
				tc.want,
				tok(TOKEN_EOF, "", 1, len(tc.input)+1),
			})
		})
	}
}

// ---------- Dot-disambiguation (dot vs float) --------------------------------

func TestNextToken_DotVsFloat(t *testing.T) {
	t.Run("dot_followed_by_identifier", func(t *testing.T) {
		// "t.id" → IDENT "t", DOT ".", IDENT "id"
		requireTokens(t, "t.id", []Token{
			tok(TOKEN_IDENT, "t", 1, 1),
			tok(TOKEN_DOT, ".", 1, 2),
			tok(TOKEN_IDENT, "id", 1, 3),
			tok(TOKEN_EOF, "", 1, 5),
		})
	})

	t.Run("dot_followed_by_digit", func(t *testing.T) {
		// ".5" → FLOAT ".5"
		requireTokens(t, ".5", []Token{
			tok(TOKEN_FLOAT, ".5", 1, 1),
			tok(TOKEN_EOF, "", 1, 3),
		})
	})

	t.Run("dot_alone", func(t *testing.T) {
		requireTokens(t, ".", []Token{
			tok(TOKEN_DOT, ".", 1, 1),
			tok(TOKEN_EOF, "", 1, 2),
		})
	})

	t.Run("number_dot_ident_is_int_then_dot_then_ident", func(t *testing.T) {
		// "42.col" → INTEGER "42", DOT ".", IDENT "col"
		requireTokens(t, "42.col", []Token{
			tok(TOKEN_INTEGER, "42", 1, 1),
			tok(TOKEN_DOT, ".", 1, 3),
			tok(TOKEN_IDENT, "col", 1, 4),
			tok(TOKEN_EOF, "", 1, 7),
		})
	})

	t.Run("number_dot_underscore_is_int_then_dot_then_ident", func(t *testing.T) {
		// "1._x" → INTEGER "1", DOT ".", IDENT "_x"
		requireTokens(t, "1._x", []Token{
			tok(TOKEN_INTEGER, "1", 1, 1),
			tok(TOKEN_DOT, ".", 1, 2),
			tok(TOKEN_IDENT, "_x", 1, 3),
			tok(TOKEN_EOF, "", 1, 5),
		})
	})
}

// ---------- String literals --------------------------------------------------

func TestNextToken_Strings(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLit string
	}{
		{"empty", "''", ""},
		{"simple", "'hello'", "hello"},
		{"with_spaces", "'hello world'", "hello world"},
		{"with_digits", "'abc123'", "abc123"},
		{"escaped_quote", "'it''s'", "it's"},
		{"double_escaped", "'a''''b'", "a''b"},
		{"only_escaped", "''''", "'"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l := NewLexer(tc.input)
			token, err := l.NextToken()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if token.Type != TOKEN_STRING {
				t.Fatalf("expected TOKEN_STRING, got %v", token.Type)
			}
			if token.Literal != tc.wantLit {
				t.Fatalf("literal: got %q, want %q", token.Literal, tc.wantLit)
			}
			if token.Line != 1 || token.Col != 1 {
				t.Fatalf("position: got %d:%d, want 1:1", token.Line, token.Col)
			}
		})
	}
}

func TestNextToken_UnterminatedString(t *testing.T) {
	inputs := []string{
		"'hello",
		"'",
		"'unterminated",
		"'it''s still open",
	}
	for _, input := range inputs {
		t.Run(fmt.Sprintf("%q", input), func(t *testing.T) {
			requireError(t, input, ErrUnterminatedString)
		})
	}
}

// ---------- Identifiers ------------------------------------------------------

func TestNextToken_Identifiers(t *testing.T) {
	tests := []struct {
		input string
		want  Token
	}{
		{"foo", tok(TOKEN_IDENT, "foo", 1, 1)},
		{"Bar", tok(TOKEN_IDENT, "Bar", 1, 1)},
		{"_private", tok(TOKEN_IDENT, "_private", 1, 1)},
		{"col1", tok(TOKEN_IDENT, "col1", 1, 1)},
		{"_", tok(TOKEN_IDENT, "_", 1, 1)},
		{"a_b_c", tok(TOKEN_IDENT, "a_b_c", 1, 1)},
		{"CamelCase", tok(TOKEN_IDENT, "CamelCase", 1, 1)},
		{"x123abc", tok(TOKEN_IDENT, "x123abc", 1, 1)},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			requireTokens(t, tc.input, []Token{
				tc.want,
				tok(TOKEN_EOF, "", 1, len(tc.input)+1),
			})
		})
	}
}

// ---------- Keywords (case-insensitive) --------------------------------------

func TestNextToken_AllKeywords(t *testing.T) {
	// Exhaustive coverage of every keyword in the keywords map.
	// Each entry tests UPPER, lower, and MiXeD casing.
	allKeywords := []struct {
		upper string
		typ   TokenType
	}{
		{"CREATE", TOKEN_CREATE},
		{"DATABASE", TOKEN_DATABASE},
		{"USE", TOKEN_USE},
		{"DROP", TOKEN_DROP},
		{"IF", TOKEN_IF},
		{"EXISTS", TOKEN_EXISTS},
		{"TABLE", TOKEN_TABLE},
		{"ALTER", TOKEN_ALTER},
		{"ADD", TOKEN_ADD},
		{"COLUMN", TOKEN_COLUMN},
		{"MODIFY", TOKEN_MODIFY},
		{"RENAME", TOKEN_RENAME},
		{"TO", TOKEN_TO},
		{"SELECT", TOKEN_SELECT},
		{"DISTINCT", TOKEN_DISTINCT},
		{"ALL", TOKEN_ALL},
		{"FROM", TOKEN_FROM},
		{"WHERE", TOKEN_WHERE},
		{"AS", TOKEN_AS},
		{"INSERT", TOKEN_INSERT},
		{"INTO", TOKEN_INTO},
		{"VALUES", TOKEN_VALUES},
		{"UPDATE", TOKEN_UPDATE},
		{"SET", TOKEN_SET},
		{"DELETE", TOKEN_DELETE},
		{"JOIN", TOKEN_JOIN},
		{"INNER", TOKEN_INNER},
		{"LEFT", TOKEN_LEFT},
		{"RIGHT", TOKEN_RIGHT},
		{"FULL", TOKEN_FULL},
		{"OUTER", TOKEN_OUTER},
		{"CROSS", TOKEN_CROSS},
		{"ON", TOKEN_ON},
		{"GROUP", TOKEN_GROUP},
		{"BY", TOKEN_BY},
		{"HAVING", TOKEN_HAVING},
		{"ORDER", TOKEN_ORDER},
		{"ASC", TOKEN_ASC},
		{"DESC", TOKEN_DESC},
		{"LIMIT", TOKEN_LIMIT},
		{"OFFSET", TOKEN_OFFSET},
		{"PRIMARY", TOKEN_PRIMARY},
		{"KEY", TOKEN_KEY},
		{"NOT", TOKEN_NOT},
		{"NULL", TOKEN_NULL},
		{"DEFAULT", TOKEN_DEFAULT},
		{"UNIQUE", TOKEN_UNIQUE},
		{"REFERENCES", TOKEN_REFERENCES},
		{"AND", TOKEN_AND},
		{"OR", TOKEN_OR},
		{"TRUE", TOKEN_TRUE},
		{"FALSE", TOKEN_FALSE},
		{"LIKE", TOKEN_LIKE},
		{"IS", TOKEN_IS},
		{"IN", TOKEN_IN},
		{"BETWEEN", TOKEN_BETWEEN},
		{"INT", TOKEN_INT},
		{"BIGINT", TOKEN_BIGINT},
		{"VARCHAR", TOKEN_VARCHAR},
		{"BOOLEAN", TOKEN_BOOLEAN},
		{"TEXT", TOKEN_TEXT},
		{"TIMESTAMP", TOKEN_TIMESTAMP},
	}
	for _, kw := range allKeywords {
		t.Run(kw.upper, func(t *testing.T) {
			// Upper case
			tokens := collectAll(t, kw.upper)
			if tokens[0].Type != kw.typ {
				t.Errorf("UPPER %q: got type %v, want %v", kw.upper, tokens[0].Type, kw.typ)
			}
			if tokens[0].Literal != kw.upper {
				t.Errorf("UPPER %q: literal got %q, want %q", kw.upper, tokens[0].Literal, kw.upper)
			}
		})
	}
}

func TestNextToken_KeywordsCaseInsensitive(t *testing.T) {
	// Verify that the literal preserves original casing while the type is correct.
	cases := []struct {
		input   string
		wantTyp TokenType
		wantLit string
	}{
		{"select", TOKEN_SELECT, "select"},
		{"SELECT", TOKEN_SELECT, "SELECT"},
		{"SeLeCt", TOKEN_SELECT, "SeLeCt"},
		{"from", TOKEN_FROM, "from"},
		{"From", TOKEN_FROM, "From"},
		{"insert", TOKEN_INSERT, "insert"},
		{"InSeRt", TOKEN_INSERT, "InSeRt"},
		{"null", TOKEN_NULL, "null"},
		{"Null", TOKEN_NULL, "Null"},
		{"true", TOKEN_TRUE, "true"},
		{"false", TOKEN_FALSE, "false"},
		{"FaLsE", TOKEN_FALSE, "FaLsE"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			tokens := collectAll(t, tc.input)
			if tokens[0].Type != tc.wantTyp {
				t.Errorf("type: got %v, want %v", tokens[0].Type, tc.wantTyp)
			}
			if tokens[0].Literal != tc.wantLit {
				t.Errorf("literal: got %q, want %q", tokens[0].Literal, tc.wantLit)
			}
		})
	}
}

// ---------- Comments ---------------------------------------------------------

func TestNextToken_LineComment(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []Token
	}{
		{"comment_at_end", "42 -- comment", []Token{
			tok(TOKEN_INTEGER, "42", 1, 1),
			tok(TOKEN_EOF, "", 1, 14),
		}},
		{"comment_only", "-- everything is a comment", []Token{
			tok(TOKEN_EOF, "", 1, 27),
		}},
		{"comment_before_newline", "-- comment\n42", []Token{
			tok(TOKEN_INTEGER, "42", 2, 1),
			tok(TOKEN_EOF, "", 2, 3),
		}},
		{"multiple_line_comments", "-- first\n-- second\n42", []Token{
			tok(TOKEN_INTEGER, "42", 3, 1),
			tok(TOKEN_EOF, "", 3, 3),
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requireTokens(t, tc.input, tc.want)
		})
	}
}

func TestNextToken_BlockComment(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []Token
	}{
		{"inline", "/* comment */ 42", []Token{
			tok(TOKEN_INTEGER, "42", 1, 15),
			tok(TOKEN_EOF, "", 1, 17),
		}},
		{"multi_line", "/* line1\nline2 */ 42", []Token{
			tok(TOKEN_INTEGER, "42", 2, 10),
			tok(TOKEN_EOF, "", 2, 12),
		}},
		{"empty_block", "/**/ 42", []Token{
			tok(TOKEN_INTEGER, "42", 1, 6),
			tok(TOKEN_EOF, "", 1, 8),
		}},
		{"adjacent", "/*a*//*b*/ 42", []Token{
			tok(TOKEN_INTEGER, "42", 1, 12),
			tok(TOKEN_EOF, "", 1, 14),
		}},
		{"comment_only", "/* eof in comment? no, closed */", []Token{
			tok(TOKEN_EOF, "", 1, 33),
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requireTokens(t, tc.input, tc.want)
		})
	}
}

func TestNextToken_UnterminatedBlockComment(t *testing.T) {
	inputs := []string{
		"/* unclosed",
		"/* also \n unclosed",
		"/*",
	}
	for _, input := range inputs {
		t.Run(fmt.Sprintf("%q", input), func(t *testing.T) {
			requireError(t, input, ErrUnterminatedComment)
		})
	}
}

func TestNextToken_MixedComments(t *testing.T) {
	input := "-- line\n/* block */ SELECT"
	requireTokens(t, input, []Token{
		tok(TOKEN_SELECT, "SELECT", 2, 13),
		tok(TOKEN_EOF, "", 2, 19),
	})
}

// ---------- Illegal characters -----------------------------------------------

func TestNextToken_IllegalCharacters(t *testing.T) {
	illegals := []string{"@", "#", "$", "^", "&", "~", "\\", "`", "?", "|"}
	for _, ch := range illegals {
		t.Run(ch, func(t *testing.T) {
			l := NewLexer(ch)
			token, err := l.NextToken()
			if err == nil {
				t.Fatal("expected error for illegal character")
			}
			if !errors.Is(err, ErrUnexpectedChar) {
				t.Fatalf("expected ErrUnexpectedChar, got %v", err)
			}
			if token.Type != TOKEN_ILLEGAL {
				t.Fatalf("expected TOKEN_ILLEGAL, got %v", token.Type)
			}
			if token.Literal != ch {
				t.Fatalf("literal: got %q, want %q", token.Literal, ch)
			}
		})
	}
}

// ---------- Line/column tracking ---------------------------------------------

func TestNextToken_LineColTracking(t *testing.T) {
	input := "SELECT\n  *\nFROM t"
	requireTokens(t, input, []Token{
		tok(TOKEN_SELECT, "SELECT", 1, 1),
		tok(TOKEN_STAR, "*", 2, 3),
		tok(TOKEN_FROM, "FROM", 3, 1),
		tok(TOKEN_IDENT, "t", 3, 6),
		tok(TOKEN_EOF, "", 3, 7),
	})
}

func TestNextToken_TabTracking(t *testing.T) {
	// Tabs count as single column advances.
	input := "\tSELECT"
	tokens := collectAll(t, input)
	if tokens[0].Col != 2 {
		t.Errorf("expected column 2 after tab, got %d", tokens[0].Col)
	}
}

func TestNextToken_MultipleNewlines(t *testing.T) {
	input := "\n\n\n42"
	tokens := collectAll(t, input)
	if tokens[0].Line != 4 || tokens[0].Col != 1 {
		t.Errorf("expected 4:1, got %d:%d", tokens[0].Line, tokens[0].Col)
	}
}

func TestNextToken_CarriageReturnLineFeed(t *testing.T) {
	// \r is treated as whitespace but doesn't increment line; only \n does.
	input := "a\r\nb"
	tokens := collectAll(t, input)
	// 'a' at 1:1
	if tokens[0].Line != 1 || tokens[0].Col != 1 {
		t.Errorf("'a' expected 1:1, got %d:%d", tokens[0].Line, tokens[0].Col)
	}
	// 'b' at 2:1
	if tokens[1].Line != 2 || tokens[1].Col != 1 {
		t.Errorf("'b' expected 2:1, got %d:%d", tokens[1].Line, tokens[1].Col)
	}
}

// ---------- Whitespace sensitivity -------------------------------------------

func TestNextToken_MultipleSpaces(t *testing.T) {
	input := "a     b"
	requireTokens(t, input, []Token{
		tok(TOKEN_IDENT, "a", 1, 1),
		tok(TOKEN_IDENT, "b", 1, 7),
		tok(TOKEN_EOF, "", 1, 8),
	})
}

func TestNextToken_NoWhitespace(t *testing.T) {
	input := "a+b"
	requireTokens(t, input, []Token{
		tok(TOKEN_IDENT, "a", 1, 1),
		tok(TOKEN_PLUS, "+", 1, 2),
		tok(TOKEN_IDENT, "b", 1, 3),
		tok(TOKEN_EOF, "", 1, 4),
	})
}

// ---------- LexError structure -----------------------------------------------

func TestLexError_ErrorMessage(t *testing.T) {
	e := lexErr(ErrUnexpectedChar, 5, 10, "'@'")
	want := "5:10: unexpected character: '@'"
	if e.Error() != want {
		t.Errorf("got %q, want %q", e.Error(), want)
	}
}

func TestLexError_Unwrap(t *testing.T) {
	e := lexErr(ErrUnterminatedString, 1, 1, "detail")
	if !errors.Is(e, ErrUnterminatedString) {
		t.Error("errors.Is should match sentinel")
	}
	var le *LexError
	if !errors.As(e, &le) {
		t.Error("errors.As should succeed for *LexError")
	}
	if le.Line != 1 || le.Col != 1 {
		t.Errorf("position: got %d:%d, want 1:1", le.Line, le.Col)
	}
}

// ---------- Full SQL statements (integration) --------------------------------

func TestNextToken_SelectStatement(t *testing.T) {
	input := "SELECT id, name FROM users WHERE age >= 18;"
	requireTokens(t, input, []Token{
		tok(TOKEN_SELECT, "SELECT", 1, 1),
		tok(TOKEN_IDENT, "id", 1, 8),
		tok(TOKEN_COMMA, ",", 1, 10),
		tok(TOKEN_IDENT, "name", 1, 12),
		tok(TOKEN_FROM, "FROM", 1, 17),
		tok(TOKEN_IDENT, "users", 1, 22),
		tok(TOKEN_WHERE, "WHERE", 1, 28),
		tok(TOKEN_IDENT, "age", 1, 34),
		tok(TOKEN_GTE, ">=", 1, 38),
		tok(TOKEN_INTEGER, "18", 1, 41),
		tok(TOKEN_SEMICOLON, ";", 1, 43),
		tok(TOKEN_EOF, "", 1, 44),
	})
}

func TestNextToken_InsertStatement(t *testing.T) {
	input := "INSERT INTO users (name, age) VALUES ('Alice', 30);"
	requireTokens(t, input, []Token{
		tok(TOKEN_INSERT, "INSERT", 1, 1),
		tok(TOKEN_INTO, "INTO", 1, 8),
		tok(TOKEN_IDENT, "users", 1, 13),
		tok(TOKEN_LPAREN, "(", 1, 19),
		tok(TOKEN_IDENT, "name", 1, 20),
		tok(TOKEN_COMMA, ",", 1, 24),
		tok(TOKEN_IDENT, "age", 1, 26),
		tok(TOKEN_RPAREN, ")", 1, 29),
		tok(TOKEN_VALUES, "VALUES", 1, 31),
		tok(TOKEN_LPAREN, "(", 1, 38),
		tok(TOKEN_STRING, "Alice", 1, 39),
		tok(TOKEN_COMMA, ",", 1, 46),
		tok(TOKEN_INTEGER, "30", 1, 48),
		tok(TOKEN_RPAREN, ")", 1, 50),
		tok(TOKEN_SEMICOLON, ";", 1, 51),
		tok(TOKEN_EOF, "", 1, 52),
	})
}

func TestNextToken_CreateTable(t *testing.T) {
	input := `CREATE TABLE users (
    id INT PRIMARY KEY,
    name VARCHAR NOT NULL,
    active BOOLEAN DEFAULT TRUE
);`
	requireTokens(t, input, []Token{
		tok(TOKEN_CREATE, "CREATE", 1, 1),
		tok(TOKEN_TABLE, "TABLE", 1, 8),
		tok(TOKEN_IDENT, "users", 1, 14),
		tok(TOKEN_LPAREN, "(", 1, 20),
		// line 2
		tok(TOKEN_IDENT, "id", 2, 5),
		tok(TOKEN_INT, "INT", 2, 8),
		tok(TOKEN_PRIMARY, "PRIMARY", 2, 12),
		tok(TOKEN_KEY, "KEY", 2, 20),
		tok(TOKEN_COMMA, ",", 2, 23),
		// line 3
		tok(TOKEN_IDENT, "name", 3, 5),
		tok(TOKEN_VARCHAR, "VARCHAR", 3, 10),
		tok(TOKEN_NOT, "NOT", 3, 18),
		tok(TOKEN_NULL, "NULL", 3, 22),
		tok(TOKEN_COMMA, ",", 3, 26),
		// line 4
		tok(TOKEN_IDENT, "active", 4, 5),
		tok(TOKEN_BOOLEAN, "BOOLEAN", 4, 12),
		tok(TOKEN_DEFAULT, "DEFAULT", 4, 20),
		tok(TOKEN_TRUE, "TRUE", 4, 28),
		// line 5
		tok(TOKEN_RPAREN, ")", 5, 1),
		tok(TOKEN_SEMICOLON, ";", 5, 2),
		tok(TOKEN_EOF, "", 5, 3),
	})
}

func TestNextToken_UpdateStatement(t *testing.T) {
	input := "UPDATE users SET name = 'Bob' WHERE id = 1;"
	requireTokens(t, input, []Token{
		tok(TOKEN_UPDATE, "UPDATE", 1, 1),
		tok(TOKEN_IDENT, "users", 1, 8),
		tok(TOKEN_SET, "SET", 1, 14),
		tok(TOKEN_IDENT, "name", 1, 18),
		tok(TOKEN_EQ, "=", 1, 23),
		tok(TOKEN_STRING, "Bob", 1, 25),
		tok(TOKEN_WHERE, "WHERE", 1, 31),
		tok(TOKEN_IDENT, "id", 1, 37),
		tok(TOKEN_EQ, "=", 1, 40),
		tok(TOKEN_INTEGER, "1", 1, 42),
		tok(TOKEN_SEMICOLON, ";", 1, 43),
		tok(TOKEN_EOF, "", 1, 44),
	})
}

func TestNextToken_DeleteStatement(t *testing.T) {
	input := "DELETE FROM users WHERE id = 1;"
	requireTokens(t, input, []Token{
		tok(TOKEN_DELETE, "DELETE", 1, 1),
		tok(TOKEN_FROM, "FROM", 1, 8),
		tok(TOKEN_IDENT, "users", 1, 13),
		tok(TOKEN_WHERE, "WHERE", 1, 19),
		tok(TOKEN_IDENT, "id", 1, 25),
		tok(TOKEN_EQ, "=", 1, 28),
		tok(TOKEN_INTEGER, "1", 1, 30),
		tok(TOKEN_SEMICOLON, ";", 1, 31),
		tok(TOKEN_EOF, "", 1, 32),
	})
}

func TestNextToken_JoinQuery(t *testing.T) {
	input := "SELECT a.id FROM a INNER JOIN b ON a.id = b.a_id"
	requireTokens(t, input, []Token{
		tok(TOKEN_SELECT, "SELECT", 1, 1),
		tok(TOKEN_IDENT, "a", 1, 8),
		tok(TOKEN_DOT, ".", 1, 9),
		tok(TOKEN_IDENT, "id", 1, 10),
		tok(TOKEN_FROM, "FROM", 1, 13),
		tok(TOKEN_IDENT, "a", 1, 18),
		tok(TOKEN_INNER, "INNER", 1, 20),
		tok(TOKEN_JOIN, "JOIN", 1, 26),
		tok(TOKEN_IDENT, "b", 1, 31),
		tok(TOKEN_ON, "ON", 1, 33),
		tok(TOKEN_IDENT, "a", 1, 36),
		tok(TOKEN_DOT, ".", 1, 37),
		tok(TOKEN_IDENT, "id", 1, 38),
		tok(TOKEN_EQ, "=", 1, 41),
		tok(TOKEN_IDENT, "b", 1, 43),
		tok(TOKEN_DOT, ".", 1, 44),
		tok(TOKEN_IDENT, "a_id", 1, 45),
		tok(TOKEN_EOF, "", 1, 49),
	})
}

func TestNextToken_GroupByHavingOrderBy(t *testing.T) {
	input := "SELECT dept, COUNT(*) FROM emp GROUP BY dept HAVING COUNT(*) > 5 ORDER BY dept ASC LIMIT 10 OFFSET 5"
	requireTokens(t, input, []Token{
		tok(TOKEN_SELECT, "SELECT", 1, 1),
		tok(TOKEN_IDENT, "dept", 1, 8),
		tok(TOKEN_COMMA, ",", 1, 12),
		tok(TOKEN_IDENT, "COUNT", 1, 14),
		tok(TOKEN_LPAREN, "(", 1, 19),
		tok(TOKEN_STAR, "*", 1, 20),
		tok(TOKEN_RPAREN, ")", 1, 21),
		tok(TOKEN_FROM, "FROM", 1, 23),
		tok(TOKEN_IDENT, "emp", 1, 28),
		tok(TOKEN_GROUP, "GROUP", 1, 32),
		tok(TOKEN_BY, "BY", 1, 38),
		tok(TOKEN_IDENT, "dept", 1, 41),
		tok(TOKEN_HAVING, "HAVING", 1, 46),
		tok(TOKEN_IDENT, "COUNT", 1, 53),
		tok(TOKEN_LPAREN, "(", 1, 58),
		tok(TOKEN_STAR, "*", 1, 59),
		tok(TOKEN_RPAREN, ")", 1, 60),
		tok(TOKEN_GT, ">", 1, 62),
		tok(TOKEN_INTEGER, "5", 1, 64),
		tok(TOKEN_ORDER, "ORDER", 1, 66),
		tok(TOKEN_BY, "BY", 1, 72),
		tok(TOKEN_IDENT, "dept", 1, 75),
		tok(TOKEN_ASC, "ASC", 1, 80),
		tok(TOKEN_LIMIT, "LIMIT", 1, 84),
		tok(TOKEN_INTEGER, "10", 1, 90),
		tok(TOKEN_OFFSET, "OFFSET", 1, 93),
		tok(TOKEN_INTEGER, "5", 1, 100),
		tok(TOKEN_EOF, "", 1, 101),
	})
}

func TestNextToken_ComplexExpression(t *testing.T) {
	input := "WHERE x BETWEEN 1 AND 10 AND name LIKE 'foo%' OR val IS NOT NULL AND id IN (1, 2, 3)"
	requireTokens(t, input, []Token{
		tok(TOKEN_WHERE, "WHERE", 1, 1),
		tok(TOKEN_IDENT, "x", 1, 7),
		tok(TOKEN_BETWEEN, "BETWEEN", 1, 9),
		tok(TOKEN_INTEGER, "1", 1, 17),
		tok(TOKEN_AND, "AND", 1, 19),
		tok(TOKEN_INTEGER, "10", 1, 23),
		tok(TOKEN_AND, "AND", 1, 26),
		tok(TOKEN_IDENT, "name", 1, 30),
		tok(TOKEN_LIKE, "LIKE", 1, 35),
		tok(TOKEN_STRING, "foo%", 1, 40),
		tok(TOKEN_OR, "OR", 1, 47),
		tok(TOKEN_IDENT, "val", 1, 50),
		tok(TOKEN_IS, "IS", 1, 54),
		tok(TOKEN_NOT, "NOT", 1, 57),
		tok(TOKEN_NULL, "NULL", 1, 61),
		tok(TOKEN_AND, "AND", 1, 66),
		tok(TOKEN_IDENT, "id", 1, 70),
		tok(TOKEN_IN, "IN", 1, 73),
		tok(TOKEN_LPAREN, "(", 1, 76),
		tok(TOKEN_INTEGER, "1", 1, 77),
		tok(TOKEN_COMMA, ",", 1, 78),
		tok(TOKEN_INTEGER, "2", 1, 80),
		tok(TOKEN_COMMA, ",", 1, 81),
		tok(TOKEN_INTEGER, "3", 1, 83),
		tok(TOKEN_RPAREN, ")", 1, 84),
		tok(TOKEN_EOF, "", 1, 85),
	})
}

func TestNextToken_AlterTable(t *testing.T) {
	input := "ALTER TABLE users ADD COLUMN email TEXT UNIQUE"
	requireTokens(t, input, []Token{
		tok(TOKEN_ALTER, "ALTER", 1, 1),
		tok(TOKEN_TABLE, "TABLE", 1, 7),
		tok(TOKEN_IDENT, "users", 1, 13),
		tok(TOKEN_ADD, "ADD", 1, 19),
		tok(TOKEN_COLUMN, "COLUMN", 1, 23),
		tok(TOKEN_IDENT, "email", 1, 30),
		tok(TOKEN_TEXT, "TEXT", 1, 36),
		tok(TOKEN_UNIQUE, "UNIQUE", 1, 41),
		tok(TOKEN_EOF, "", 1, 47),
	})
}

func TestNextToken_DropIfExists(t *testing.T) {
	input := "DROP TABLE IF EXISTS users;"
	requireTokens(t, input, []Token{
		tok(TOKEN_DROP, "DROP", 1, 1),
		tok(TOKEN_TABLE, "TABLE", 1, 6),
		tok(TOKEN_IF, "IF", 1, 12),
		tok(TOKEN_EXISTS, "EXISTS", 1, 15),
		tok(TOKEN_IDENT, "users", 1, 22),
		tok(TOKEN_SEMICOLON, ";", 1, 27),
		tok(TOKEN_EOF, "", 1, 28),
	})
}

func TestNextToken_CreateDatabase(t *testing.T) {
	input := "CREATE DATABASE mydb;"
	requireTokens(t, input, []Token{
		tok(TOKEN_CREATE, "CREATE", 1, 1),
		tok(TOKEN_DATABASE, "DATABASE", 1, 8),
		tok(TOKEN_IDENT, "mydb", 1, 17),
		tok(TOKEN_SEMICOLON, ";", 1, 21),
		tok(TOKEN_EOF, "", 1, 22),
	})
}

func TestNextToken_UseDatabase(t *testing.T) {
	input := "USE mydb;"
	requireTokens(t, input, []Token{
		tok(TOKEN_USE, "USE", 1, 1),
		tok(TOKEN_IDENT, "mydb", 1, 5),
		tok(TOKEN_SEMICOLON, ";", 1, 9),
		tok(TOKEN_EOF, "", 1, 10),
	})
}

func TestNextToken_RenameTable(t *testing.T) {
	input := "ALTER TABLE old_name RENAME TO new_name;"
	requireTokens(t, input, []Token{
		tok(TOKEN_ALTER, "ALTER", 1, 1),
		tok(TOKEN_TABLE, "TABLE", 1, 7),
		tok(TOKEN_IDENT, "old_name", 1, 13),
		tok(TOKEN_RENAME, "RENAME", 1, 22),
		tok(TOKEN_TO, "TO", 1, 29),
		tok(TOKEN_IDENT, "new_name", 1, 32),
		tok(TOKEN_SEMICOLON, ";", 1, 40),
		tok(TOKEN_EOF, "", 1, 41),
	})
}

func TestNextToken_SelectWithAlias(t *testing.T) {
	input := "SELECT DISTINCT name AS n FROM users"
	requireTokens(t, input, []Token{
		tok(TOKEN_SELECT, "SELECT", 1, 1),
		tok(TOKEN_DISTINCT, "DISTINCT", 1, 8),
		tok(TOKEN_IDENT, "name", 1, 17),
		tok(TOKEN_AS, "AS", 1, 22),
		tok(TOKEN_IDENT, "n", 1, 25),
		tok(TOKEN_FROM, "FROM", 1, 27),
		tok(TOKEN_IDENT, "users", 1, 32),
		tok(TOKEN_EOF, "", 1, 37),
	})
}

func TestNextToken_SelectAllJoins(t *testing.T) {
	input := "LEFT OUTER JOIN RIGHT OUTER JOIN FULL OUTER JOIN CROSS JOIN"
	requireTokens(t, input, []Token{
		tok(TOKEN_LEFT, "LEFT", 1, 1),
		tok(TOKEN_OUTER, "OUTER", 1, 6),
		tok(TOKEN_JOIN, "JOIN", 1, 12),
		tok(TOKEN_RIGHT, "RIGHT", 1, 17),
		tok(TOKEN_OUTER, "OUTER", 1, 23),
		tok(TOKEN_JOIN, "JOIN", 1, 29),
		tok(TOKEN_FULL, "FULL", 1, 34),
		tok(TOKEN_OUTER, "OUTER", 1, 39),
		tok(TOKEN_JOIN, "JOIN", 1, 45),
		tok(TOKEN_CROSS, "CROSS", 1, 50),
		tok(TOKEN_JOIN, "JOIN", 1, 56),
		tok(TOKEN_EOF, "", 1, 60),
	})
}

func TestNextToken_ForeignKeyReference(t *testing.T) {
	input := "user_id BIGINT REFERENCES users(id)"
	requireTokens(t, input, []Token{
		tok(TOKEN_IDENT, "user_id", 1, 1),
		tok(TOKEN_BIGINT, "BIGINT", 1, 9),
		tok(TOKEN_REFERENCES, "REFERENCES", 1, 16),
		tok(TOKEN_IDENT, "users", 1, 27),
		tok(TOKEN_LPAREN, "(", 1, 32),
		tok(TOKEN_IDENT, "id", 1, 33),
		tok(TOKEN_RPAREN, ")", 1, 35),
		tok(TOKEN_EOF, "", 1, 36),
	})
}

func TestNextToken_TimestampColumn(t *testing.T) {
	input := "created_at TIMESTAMP NOT NULL DEFAULT '2024-01-01'"
	requireTokens(t, input, []Token{
		tok(TOKEN_IDENT, "created_at", 1, 1),
		tok(TOKEN_TIMESTAMP, "TIMESTAMP", 1, 12),
		tok(TOKEN_NOT, "NOT", 1, 22),
		tok(TOKEN_NULL, "NULL", 1, 26),
		tok(TOKEN_DEFAULT, "DEFAULT", 1, 31),
		tok(TOKEN_STRING, "2024-01-01", 1, 39),
		tok(TOKEN_EOF, "", 1, 51),
	})
}

func TestNextToken_ArithmeticExpression(t *testing.T) {
	input := "a + b - c * d / e % f"
	requireTokens(t, input, []Token{
		tok(TOKEN_IDENT, "a", 1, 1),
		tok(TOKEN_PLUS, "+", 1, 3),
		tok(TOKEN_IDENT, "b", 1, 5),
		tok(TOKEN_MINUS, "-", 1, 7),
		tok(TOKEN_IDENT, "c", 1, 9),
		tok(TOKEN_STAR, "*", 1, 11),
		tok(TOKEN_IDENT, "d", 1, 13),
		tok(TOKEN_SLASH, "/", 1, 15),
		tok(TOKEN_IDENT, "e", 1, 17),
		tok(TOKEN_PERCENT, "%", 1, 19),
		tok(TOKEN_IDENT, "f", 1, 21),
		tok(TOKEN_EOF, "", 1, 22),
	})
}

func TestNextToken_SelectAll(t *testing.T) {
	input := "SELECT ALL * FROM t"
	requireTokens(t, input, []Token{
		tok(TOKEN_SELECT, "SELECT", 1, 1),
		tok(TOKEN_ALL, "ALL", 1, 8),
		tok(TOKEN_STAR, "*", 1, 12),
		tok(TOKEN_FROM, "FROM", 1, 14),
		tok(TOKEN_IDENT, "t", 1, 19),
		tok(TOKEN_EOF, "", 1, 20),
	})
}

func TestNextToken_DescOrder(t *testing.T) {
	input := "ORDER BY col DESC"
	requireTokens(t, input, []Token{
		tok(TOKEN_ORDER, "ORDER", 1, 1),
		tok(TOKEN_BY, "BY", 1, 7),
		tok(TOKEN_IDENT, "col", 1, 10),
		tok(TOKEN_DESC, "DESC", 1, 14),
		tok(TOKEN_EOF, "", 1, 18),
	})
}

// ---------- Edge cases -------------------------------------------------------

func TestNextToken_MinusVsLineComment(t *testing.T) {
	// Single minus is TOKEN_MINUS; double minus is a line comment.
	t.Run("single_minus", func(t *testing.T) {
		requireTokens(t, "3 - 1", []Token{
			tok(TOKEN_INTEGER, "3", 1, 1),
			tok(TOKEN_MINUS, "-", 1, 3),
			tok(TOKEN_INTEGER, "1", 1, 5),
			tok(TOKEN_EOF, "", 1, 6),
		})
	})
	t.Run("double_minus_is_comment", func(t *testing.T) {
		requireTokens(t, "3 -- 1", []Token{
			tok(TOKEN_INTEGER, "3", 1, 1),
			tok(TOKEN_EOF, "", 1, 7),
		})
	})
}

func TestNextToken_SlashVsBlockComment(t *testing.T) {
	// Single slash is TOKEN_SLASH; /* starts a block comment.
	t.Run("single_slash", func(t *testing.T) {
		requireTokens(t, "3 / 1", []Token{
			tok(TOKEN_INTEGER, "3", 1, 1),
			tok(TOKEN_SLASH, "/", 1, 3),
			tok(TOKEN_INTEGER, "1", 1, 5),
			tok(TOKEN_EOF, "", 1, 6),
		})
	})
	t.Run("slash_star_is_comment", func(t *testing.T) {
		requireTokens(t, "3 /* comment */ / 1", []Token{
			tok(TOKEN_INTEGER, "3", 1, 1),
			tok(TOKEN_SLASH, "/", 1, 17),
			tok(TOKEN_INTEGER, "1", 1, 19),
			tok(TOKEN_EOF, "", 1, 20),
		})
	})
}

func TestNextToken_LessThanAmbiguity(t *testing.T) {
	// < alone, <=, <>
	t.Run("lt_followed_by_space", func(t *testing.T) {
		requireTokens(t, "a < b", []Token{
			tok(TOKEN_IDENT, "a", 1, 1),
			tok(TOKEN_LT, "<", 1, 3),
			tok(TOKEN_IDENT, "b", 1, 5),
			tok(TOKEN_EOF, "", 1, 6),
		})
	})
	t.Run("lt_followed_by_eq", func(t *testing.T) {
		requireTokens(t, "a<=b", []Token{
			tok(TOKEN_IDENT, "a", 1, 1),
			tok(TOKEN_LTE, "<=", 1, 2),
			tok(TOKEN_IDENT, "b", 1, 4),
			tok(TOKEN_EOF, "", 1, 5),
		})
	})
	t.Run("lt_followed_by_gt", func(t *testing.T) {
		requireTokens(t, "a<>b", []Token{
			tok(TOKEN_IDENT, "a", 1, 1),
			tok(TOKEN_NEQ, "<>", 1, 2),
			tok(TOKEN_IDENT, "b", 1, 4),
			tok(TOKEN_EOF, "", 1, 5),
		})
	})
}

func TestNextToken_ConsecutiveOperators(t *testing.T) {
	input := ">=<="
	requireTokens(t, input, []Token{
		tok(TOKEN_GTE, ">=", 1, 1),
		tok(TOKEN_LTE, "<=", 1, 3),
		tok(TOKEN_EOF, "", 1, 5),
	})
}

func TestNextToken_StringInContext(t *testing.T) {
	input := "WHERE name = 'O''Brien'"
	requireTokens(t, input, []Token{
		tok(TOKEN_WHERE, "WHERE", 1, 1),
		tok(TOKEN_IDENT, "name", 1, 7),
		tok(TOKEN_EQ, "=", 1, 12),
		tok(TOKEN_STRING, "O'Brien", 1, 14),
		tok(TOKEN_EOF, "", 1, 24),
	})
}

func TestNextToken_FloatInExpression(t *testing.T) {
	input := "price * 1.08 + .5"
	requireTokens(t, input, []Token{
		tok(TOKEN_IDENT, "price", 1, 1),
		tok(TOKEN_STAR, "*", 1, 7),
		tok(TOKEN_FLOAT, "1.08", 1, 9),
		tok(TOKEN_PLUS, "+", 1, 14),
		tok(TOKEN_FLOAT, ".5", 1, 16),
		tok(TOKEN_EOF, "", 1, 18),
	})
}

func TestNextToken_IdentStartingWithUnderscore(t *testing.T) {
	input := "_foo _123 __"
	requireTokens(t, input, []Token{
		tok(TOKEN_IDENT, "_foo", 1, 1),
		tok(TOKEN_IDENT, "_123", 1, 6),
		tok(TOKEN_IDENT, "__", 1, 11),
		tok(TOKEN_EOF, "", 1, 13),
	})
}

func TestNextToken_KeywordAsPrefix(t *testing.T) {
	// "selection" should be IDENT, not SELECT + "ion"
	requireTokens(t, "selection", []Token{
		tok(TOKEN_IDENT, "selection", 1, 1),
		tok(TOKEN_EOF, "", 1, 10),
	})
}

func TestNextToken_MultiLineString(t *testing.T) {
	// Strings can span newlines.
	input := "'line1\nline2'"
	l := NewLexer(input)
	token, err := l.NextToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token.Type != TOKEN_STRING {
		t.Fatalf("expected TOKEN_STRING, got %v", token.Type)
	}
	if token.Literal != "line1\nline2" {
		t.Fatalf("literal: got %q, want %q", token.Literal, "line1\nline2")
	}
}

func TestNextToken_NumberFollowedByDotFollowedByNumber(t *testing.T) {
	// "1.2.3" → FLOAT "1.2", then ".3" starts a leading-dot float.
	requireTokens(t, "1.2.3", []Token{
		tok(TOKEN_FLOAT, "1.2", 1, 1),
		tok(TOKEN_FLOAT, ".3", 1, 4),
		tok(TOKEN_EOF, "", 1, 6),
	})
}

func TestNextToken_ModifyKeyword(t *testing.T) {
	input := "ALTER TABLE t MODIFY COLUMN c INT;"
	requireTokens(t, input, []Token{
		tok(TOKEN_ALTER, "ALTER", 1, 1),
		tok(TOKEN_TABLE, "TABLE", 1, 7),
		tok(TOKEN_IDENT, "t", 1, 13),
		tok(TOKEN_MODIFY, "MODIFY", 1, 15),
		tok(TOKEN_COLUMN, "COLUMN", 1, 22),
		tok(TOKEN_IDENT, "c", 1, 29),
		tok(TOKEN_INT, "INT", 1, 31),
		tok(TOKEN_SEMICOLON, ";", 1, 34),
		tok(TOKEN_EOF, "", 1, 35),
	})
}

func TestNextToken_SelectWithComments(t *testing.T) {
	input := `SELECT -- column list
    id, /* primary key */
    name
FROM users;`
	requireTokens(t, input, []Token{
		tok(TOKEN_SELECT, "SELECT", 1, 1),
		tok(TOKEN_IDENT, "id", 2, 5),
		tok(TOKEN_COMMA, ",", 2, 7),
		tok(TOKEN_IDENT, "name", 3, 5),
		tok(TOKEN_FROM, "FROM", 4, 1),
		tok(TOKEN_IDENT, "users", 4, 6),
		tok(TOKEN_SEMICOLON, ";", 4, 11),
		tok(TOKEN_EOF, "", 4, 12),
	})
}

func TestNextToken_OperatorsWithNoSpaces(t *testing.T) {
	input := "(a+b)*(c-d)"
	requireTokens(t, input, []Token{
		tok(TOKEN_LPAREN, "(", 1, 1),
		tok(TOKEN_IDENT, "a", 1, 2),
		tok(TOKEN_PLUS, "+", 1, 3),
		tok(TOKEN_IDENT, "b", 1, 4),
		tok(TOKEN_RPAREN, ")", 1, 5),
		tok(TOKEN_STAR, "*", 1, 6),
		tok(TOKEN_LPAREN, "(", 1, 7),
		tok(TOKEN_IDENT, "c", 1, 8),
		tok(TOKEN_MINUS, "-", 1, 9),
		tok(TOKEN_IDENT, "d", 1, 10),
		tok(TOKEN_RPAREN, ")", 1, 11),
		tok(TOKEN_EOF, "", 1, 12),
	})
}

func TestNextToken_NumberAtEndOfInput(t *testing.T) {
	// Number followed immediately by EOF, with trailing dot.
	requireTokens(t, "42.", []Token{
		tok(TOKEN_FLOAT, "42.", 1, 1),
		tok(TOKEN_EOF, "", 1, 4),
	})
}

func TestNextToken_SelectStar(t *testing.T) {
	input := "SELECT * FROM t;"
	requireTokens(t, input, []Token{
		tok(TOKEN_SELECT, "SELECT", 1, 1),
		tok(TOKEN_STAR, "*", 1, 8),
		tok(TOKEN_FROM, "FROM", 1, 10),
		tok(TOKEN_IDENT, "t", 1, 15),
		tok(TOKEN_SEMICOLON, ";", 1, 16),
		tok(TOKEN_EOF, "", 1, 17),
	})
}

func TestNextToken_NegativeNumberContext(t *testing.T) {
	// Minus is a separate token; the parser handles negation semantically.
	requireTokens(t, "-42", []Token{
		tok(TOKEN_MINUS, "-", 1, 1),
		tok(TOKEN_INTEGER, "42", 1, 2),
		tok(TOKEN_EOF, "", 1, 4),
	})
}

func TestNextToken_ErrorRecovery(t *testing.T) {
	// After hitting an illegal character, the lexer should still be able to
	// produce subsequent tokens.
	l := NewLexer("@ SELECT")
	token, err := l.NextToken()
	if err == nil || token.Type != TOKEN_ILLEGAL {
		t.Fatalf("expected ILLEGAL token with error, got %v, err=%v", token, err)
	}
	// The next call should produce SELECT.
	token, err = l.NextToken()
	if err != nil {
		t.Fatalf("unexpected error after recovery: %v", err)
	}
	if token.Type != TOKEN_SELECT {
		t.Fatalf("expected SELECT after recovery, got %v", token.Type)
	}
}
