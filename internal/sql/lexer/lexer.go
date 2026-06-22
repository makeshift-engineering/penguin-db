package lexer

import (
	"strings"
	"unicode/utf8"

	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
)

// Lexer tokenizes SQL source text into a stream of Tokens.
type Lexer struct {
	src    string             // full input as a string
	pos    position           // current read cursor (line, col, index)
	diag   diagnostic.List    // unexported diagnostics list
	source *diagnostic.Source // source reference for diagnostics
}

// NewLexer creates a Lexer for the given SQL source string.
func NewLexer(name, src string) *Lexer {
	return &Lexer{
		src:    src,
		pos:    position{line: 1, column: 1},
		source: &diagnostic.Source{Name: name, Text: src},
	}
}

// Diagnostics returns the list of collected diagnostics.
func (l *Lexer) Diagnostics() diagnostic.List {
	return l.diag
}

func (l *Lexer) peek() rune {
	if l.pos.index >= len(l.src) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.src[l.pos.index:])
	return r
}

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

func (l *Lexer) advance() rune {
	if l.pos.index >= len(l.src) {
		return 0
	}
	r, size := utf8.DecodeRuneInString(l.src[l.pos.index:])
	l.pos.advance(r, size)
	return r
}

// skipLineComment discards everything from the current position to end-of-line.
func (l *Lexer) skipLineComment() {
	for l.peek() != 0 && l.peek() != '\n' {
		l.advance()
	}
}

// skipBlockComment discards everything up to and including the closing */.
func (l *Lexer) skipBlockComment(start diagnostic.Pos) bool {
	for l.pos.index < len(l.src) {
		if l.peek() == '*' && l.peekNext() == '/' {
			l.advance() // *
			l.advance() // /
			return true
		}
		l.advance()
	}
	// End of input without finding */
	end := l.pos.snapshot()
	span := diagnostic.Span{Start: start, End: end}
	l.diag.Append(unterminatedComment(span, l.source, start))
	return false
}

// skipWhitespaceAndComments returns false if an unterminated block comment is encountered.
func (l *Lexer) skipWhitespaceAndComments() bool {
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
				return false
			}

		default:
			return true
		}
	}
	return true
}

// makeToken is a convenience to build a Token with the given fields.
func (l *Lexer) makeToken(typ TokenType, lit string, start diagnostic.Pos) Token {
	return Token{
		Type:    typ,
		Literal: lit,
		Span: diagnostic.Span{
			Start: start,
			End:   l.pos.snapshot(),
		},
	}
}

// scanIdentifier reads a keyword or user identifier.
func (l *Lexer) scanIdentifier() Token {
	start := l.pos.snapshot()
	startIndex := l.pos.index
	for l.pos.index < len(l.src) {
		ch := l.peek()
		if isIdentPart(ch) {
			l.advance()
		} else {
			break
		}
	}
	lit := l.src[startIndex:l.pos.index]
	typ := lookupIdent(lit) // keyword or TOKEN_IDENT
	return l.makeToken(typ, lit, start)
}

func isIdentStart(ch rune) bool { return isLetter(ch) || ch == '_' }
func isIdentPart(ch rune) bool  { return isLetter(ch) || isDigit(ch) || ch == '_' }

func isLetter(ch rune) bool { return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') }
func isDigit(ch rune) bool  { return ch >= '0' && ch <= '9' }

func (l *Lexer) scanDigits() {
	for isDigit(l.peek()) {
		l.advance()
	}
}

func (l *Lexer) scanNumber() Token {
	start := l.pos.snapshot()
	startIndex := l.pos.index
	isFloat := false

	// Leading '.' case
	if l.peek() == '.' {
		isFloat = true
		l.advance()
	}

	// Digits consumption
	l.scanDigits()

	// Decimal check
	if !isFloat && l.peek() == '.' {
		nextCh := l.peekNext()
		if isDigit(nextCh) || (!isLetter(nextCh) && nextCh != '_') {
			isFloat = true
			l.advance()
			l.scanDigits()
		}
	}

	lit := l.src[startIndex:l.pos.index]
	if isFloat {
		return l.makeToken(TOKEN_FLOAT, lit, start)
	}
	return l.makeToken(TOKEN_INTEGER, lit, start)
}

func (l *Lexer) scanString() Token {
	start := l.pos.snapshot()
	l.advance() // consume opening '

	var buf strings.Builder
	for {
		if l.pos.index >= len(l.src) {
			end := l.pos.snapshot()
			span := diagnostic.Span{Start: start, End: end}
			l.diag.Append(unterminatedString(span, l.source, start))
			return l.makeToken(TOKEN_ILLEGAL, buf.String(), start)
		}

		ch := l.advance()
		if ch == '\'' {
			if l.peek() == '\'' { // '' is the SQL escape for a literal single-quote
				l.advance()
				buf.WriteByte('\'')
			} else {
				break // normal close
			}
		} else {
			buf.WriteRune(ch)
		}
	}

	return l.makeToken(TOKEN_STRING, buf.String(), start)
}

// NextToken scans and returns the next token.
func (l *Lexer) NextToken() Token {
	start := l.pos.snapshot()
	if !l.skipWhitespaceAndComments() {
		return l.makeToken(TOKEN_ILLEGAL, "", start)
	}

	start = l.pos.snapshot()
	if l.pos.index >= len(l.src) {
		return l.makeToken(TOKEN_EOF, "", start)
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
		return l.makeToken(TOKEN_DOT, ".", start)
	}
	if ch == '\'' {
		return l.scanString()
	}

	l.advance()
	switch ch {
	case '(':
		return l.makeToken(TOKEN_LPAREN, "(", start)
	case ')':
		return l.makeToken(TOKEN_RPAREN, ")", start)
	case ',':
		return l.makeToken(TOKEN_COMMA, ",", start)
	case ';':
		return l.makeToken(TOKEN_SEMICOLON, ";", start)
	case '+':
		return l.makeToken(TOKEN_PLUS, "+", start)
	case '-':
		return l.makeToken(TOKEN_MINUS, "-", start)
	case '*':
		return l.makeToken(TOKEN_STAR, "*", start)
	case '/':
		return l.makeToken(TOKEN_SLASH, "/", start)
	case '%':
		return l.makeToken(TOKEN_PERCENT, "%", start)
	case '=':
		return l.makeToken(TOKEN_EQ, "=", start)
	// ── Multi-character operators ──
	case '<':
		if l.peek() == '=' {
			l.advance()
			return l.makeToken(TOKEN_LTE, "<=", start)
		}
		if l.peek() == '>' {
			l.advance()
			return l.makeToken(TOKEN_NEQ, "<>", start)
		}
		return l.makeToken(TOKEN_LT, "<", start)
	case '>':
		if l.peek() == '=' {
			l.advance()
			return l.makeToken(TOKEN_GTE, ">=", start)
		}
		return l.makeToken(TOKEN_GT, ">", start)
	case '!':
		if l.peek() == '=' {
			l.advance()
			return l.makeToken(TOKEN_NEQ, "!=", start)
		}
		span := diagnostic.Span{Start: start, End: l.pos.snapshot()}
		l.diag.Append(unexpectedChar(span, l.source, '!'))
		return l.makeToken(TOKEN_ILLEGAL, "!", start)

	default:
		span := diagnostic.Span{Start: start, End: l.pos.snapshot()}
		l.diag.Append(unexpectedChar(span, l.source, ch))
		return l.makeToken(TOKEN_ILLEGAL, string(ch), start)
	}
}
