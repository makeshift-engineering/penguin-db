package lexer

import (
	"strings"
	"unicode/utf8"

	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

// Lexer tokenizes SQL source text into a stream of utils.Token values.
// It no longer owns the Token or TokenType types — those live in utils so
// that the parser can use them without importing the lexer.
type Lexer struct {
	src    string   // full input
	pos    position // current read cursor
	diag   diagnostic.List
	source *diagnostic.Source
}

// NewLexer creates a Lexer for the given SQL source string.
func NewLexer(name, src string) *Lexer {
	return &Lexer{
		src:    src,
		pos:    position{line: 1, column: 1},
		source: &diagnostic.Source{Name: name, Text: src},
	}
}

// Diagnostics returns all lex-time diagnostics accumulated so far.
func (l *Lexer) Diagnostics() diagnostic.List {
	return l.diag
}

// Tokenize lexes the entire input and returns every token, including the
// final TOKEN_EOF. This is the primary entry point for batch consumers
// (e.g. the parser). Any lex errors are recorded in l.Diagnostics().
func (l *Lexer) Tokenize() []utils.Token {
	var tokens []utils.Token
	for {
		tok := l.NextToken()
		tokens = append(tokens, tok)
		if tok.Type == utils.TOKEN_EOF {
			break
		}
	}
	return tokens
}

// NextToken scans and returns the next token from the source.
// It is retained for streaming use-cases and for internal use by Tokenize.
func (l *Lexer) NextToken() utils.Token {
	l.skipWhitespaceAndComments()

	start := l.pos.snapshot()
	if l.pos.index >= len(l.src) {
		return l.makeToken(utils.TOKEN_EOF, "", start)
	}

	ch := l.peek()

	if isIdentStart(ch) {
		return l.scanIdentifier()
	}
	if isDigit(ch) {
		return l.scanNumber()
	}
	if ch == '.' {
		if next := l.peekNext(); next != 0 && isDigit(next) {
			return l.scanNumber()
		}
		l.advance()
		return l.makeToken(utils.TOKEN_DOT, ".", start)
	}
	if ch == '\'' {
		return l.scanString()
	}

	l.advance()
	switch ch {
	case '(':
		return l.makeToken(utils.TOKEN_LPAREN, "(", start)
	case ')':
		return l.makeToken(utils.TOKEN_RPAREN, ")", start)
	case ',':
		return l.makeToken(utils.TOKEN_COMMA, ",", start)
	case ';':
		return l.makeToken(utils.TOKEN_SEMICOLON, ";", start)
	case '+':
		return l.makeToken(utils.TOKEN_PLUS, "+", start)
	case '-':
		return l.makeToken(utils.TOKEN_MINUS, "-", start)
	case '*':
		return l.makeToken(utils.TOKEN_STAR, "*", start)
	case '/':
		return l.makeToken(utils.TOKEN_SLASH, "/", start)
	case '%':
		return l.makeToken(utils.TOKEN_PERCENT, "%", start)
	case '=':
		return l.makeToken(utils.TOKEN_EQ, "=", start)
	case '<':
		if l.peek() == '=' {
			l.advance()
			return l.makeToken(utils.TOKEN_LTE, "<=", start)
		}
		if l.peek() == '>' {
			l.advance()
			return l.makeToken(utils.TOKEN_NEQ, "<>", start)
		}
		return l.makeToken(utils.TOKEN_LT, "<", start)
	case '>':
		if l.peek() == '=' {
			l.advance()
			return l.makeToken(utils.TOKEN_GTE, ">=", start)
		}
		return l.makeToken(utils.TOKEN_GT, ">", start)
	case '!':
		if l.peek() == '=' {
			l.advance()
			return l.makeToken(utils.TOKEN_NEQ, "!=", start)
		}
		span := diagnostic.Span{Start: start, End: l.pos.snapshot()}
		l.diag.Append(unexpectedChar(span, l.source, '!'))
		return l.makeToken(utils.TOKEN_ILLEGAL, "!", start)
	default:
		span := diagnostic.Span{Start: start, End: l.pos.snapshot()}
		l.diag.Append(unexpectedChar(span, l.source, ch))
		return l.makeToken(utils.TOKEN_ILLEGAL, string(ch), start)
	}
}

// peek returns the current rune under the cursor without advancing.
// Returns 0 if the cursor is at or past the end of input.
func (l *Lexer) peek() rune {
	if l.pos.index >= len(l.src) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.src[l.pos.index:])
	return r
}

// peekNext returns the rune immediately after the current one without
// advancing the cursor. Returns 0 if fewer than two runes remain.
func (l *Lexer) peekNext() rune {
	if l.pos.index >= len(l.src) {
		return 0
	}
	_, size := utf8.DecodeRuneInString(l.src[l.pos.index:])
	if l.pos.index+size >= len(l.src) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.src[l.pos.index+size:])
	return r
}

// advance consumes the current rune, updates the cursor position
// (line/column/offset), and returns the consumed rune. Returns 0 at EOF.
func (l *Lexer) advance() rune {
	if l.pos.index >= len(l.src) {
		return 0
	}
	r, size := utf8.DecodeRuneInString(l.src[l.pos.index:])
	l.pos.advance(r, size)
	return r
}

// makeToken constructs a Token with the given type and literal, spanning
// from start to the current cursor position.
func (l *Lexer) makeToken(typ utils.TokenType, lit string, start diagnostic.Pos) utils.Token {
	return utils.Token{
		Type:    typ,
		Literal: lit,
		Span:    diagnostic.Span{Start: start, End: l.pos.snapshot()},
	}
}

// skipLineComment consumes characters until a newline or EOF, discarding
// the body of a SQL line comment (-- ...).
func (l *Lexer) skipLineComment() {
	for l.peek() != 0 && l.peek() != '\n' {
		l.advance()
	}
}

// skipBlockComment consumes characters until the closing */ delimiter.
// Returns true if the comment was properly closed, false if EOF was reached
// first (in which case an unterminated-comment diagnostic is recorded).
func (l *Lexer) skipBlockComment(start diagnostic.Pos) bool {
	for l.pos.index < len(l.src) {
		if l.peek() == '*' && l.peekNext() == '/' {
			l.advance()
			l.advance()
			return true
		}
		l.advance()
	}
	end := l.pos.snapshot()
	l.diag.Append(unterminatedComment(diagnostic.Span{Start: start, End: end}, l.source))
	return false
}

// skipWhitespaceAndComments advances past runs of whitespace, line comments
// (-- ...), and block comments (/* ... */). It stops when a non-whitespace,
// non-comment character is found or at EOF.
func (l *Lexer) skipWhitespaceAndComments() {
	for l.pos.index < len(l.src) {
		ch := l.peek()
		switch {
		case ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n':
			l.advance()
		case ch == '-' && l.peekNext() == '-':
			l.advance()
			l.advance()
			l.skipLineComment()
		case ch == '/' && l.peekNext() == '*':
			start := l.pos.snapshot()
			l.advance()
			l.advance()
			if !l.skipBlockComment(start) {
				return
			}
		default:
			return
		}
	}
}

// scanIdentifier consumes an identifier or keyword starting at the current
// position. The literal is passed through utils.LookupIdent to distinguish
// keywords from ordinary identifiers.
func (l *Lexer) scanIdentifier() utils.Token {
	start := l.pos.snapshot()
	startIndex := l.pos.index
	for l.pos.index < len(l.src) && isIdentPart(l.peek()) {
		l.advance()
	}
	lit := l.src[startIndex:l.pos.index]
	typ := utils.LookupIdent(lit) // delegates keyword detection to utils
	return l.makeToken(typ, lit, start)
}

// scanDigits consumes a contiguous run of ASCII decimal digits.
func (l *Lexer) scanDigits() {
	for isDigit(l.peek()) {
		l.advance()
	}
}

// scanNumber consumes an integer or floating-point numeric literal,
// including optional leading dot (.5), trailing dot (5.), and scientific
// notation (1.5e-3). It disambiguates "42.col" (integer + dot + ident)
// from "42.5" (float) by peeking at the character after the dot.
func (l *Lexer) scanNumber() utils.Token {
	start := l.pos.snapshot()
	startIndex := l.pos.index
	isFloat := false

	if l.peek() == '.' {
		isFloat = true
		l.advance()
	}

	l.scanDigits()

	if !isFloat && l.peek() == '.' {
		nextCh := l.peekNext()
		if isDigit(nextCh) || (!isLetter(nextCh) && nextCh != '_') {
			isFloat = true
			l.advance()
			l.scanDigits()
		}
	}

	if l.peek() == 'e' || l.peek() == 'E' {
		hasExpDigits := false
		nextCh := l.peekNext()
		if nextCh == '+' || nextCh == '-' {
			if l.pos.index+2 < len(l.src) && isDigit(rune(l.src[l.pos.index+2])) {
				hasExpDigits = true
			}
		} else if isDigit(nextCh) {
			hasExpDigits = true
		}
		if hasExpDigits {
			isFloat = true
			l.advance()
			if l.peek() == '+' || l.peek() == '-' {
				l.advance()
			}
			l.scanDigits()
		}
	}

	lit := l.src[startIndex:l.pos.index]
	if isFloat {
		return l.makeToken(utils.TOKEN_FLOAT, lit, start)
	}
	return l.makeToken(utils.TOKEN_INTEGER, lit, start)
}

// scanString consumes a single-quoted SQL string literal, handling
// escaped quotes (”) as a single quote character. Records an
// unterminated-string diagnostic if EOF is reached before the closing quote.
func (l *Lexer) scanString() utils.Token {
	start := l.pos.snapshot()
	l.advance() // consume opening '

	var buf strings.Builder
	for {
		if l.pos.index >= len(l.src) {
			end := l.pos.snapshot()
			l.diag.Append(unterminatedString(diagnostic.Span{Start: start, End: end}, l.source))
			return l.makeToken(utils.TOKEN_ILLEGAL, buf.String(), start)
		}
		ch := l.advance()
		if ch == '\'' {
			if l.peek() == '\'' {
				l.advance()
				buf.WriteByte('\'')
			} else {
				break
			}
		} else {
			buf.WriteRune(ch)
		}
	}
	return l.makeToken(utils.TOKEN_STRING, buf.String(), start)
}

// isIdentStart reports whether ch can begin a SQL identifier (letter or underscore).
func isIdentStart(ch rune) bool { return isLetter(ch) || ch == '_' }

// isIdentPart reports whether ch can appear after the first character of a SQL identifier.
func isIdentPart(ch rune) bool { return isLetter(ch) || isDigit(ch) || ch == '_' }

// isLetter reports whether ch is an ASCII letter (a-z, A-Z).
func isLetter(ch rune) bool { return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') }

// isDigit reports whether ch is an ASCII decimal digit (0-9).
func isDigit(ch rune) bool { return ch >= '0' && ch <= '9' }
