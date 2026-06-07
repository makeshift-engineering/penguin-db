package lexer

// Lexer tokenizes SQL source text into a stream of Tokens
type Lexer struct {
	src []rune   // full input as runes
	pos Position // current read cursor(line, col, index)
}

// NewLexer creates a Lexer fro the given SQL source string
func NewLexer(src string) *Lexer {
	return &Lexer{
		src: []rune(src),
		pos: NewPosition(),
	}
}

func (l *Lexer) peek() rune {
	if l.pos.Index >= len(l.src) {
		return 0
	}
	return l.src[l.pos.Index]
}

func (l *Lexer) advance() rune {
	ch := l.src[l.pos.Index]
	l.pos.Advance(ch)
	return ch
}

// skipWhitespace consumes spaces, tabs, \r, \n
func (l *Lexer) skipWhitespace() {
	for l.pos.Index < len(l.src) {
		ch := l.src[l.pos.Index]
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			l.pos.Advance(ch)
		} else {
			break
		}
	}
}

// makeToken is a convenience to build a Token with the given fields.
func (l *Lexer) makeToken(typ TokenType, lit string, line, col int) Token {
	return Token{Type: typ, Literal: lit, Line: line, Col: col}
}

// scanIdentifier reads a keyword or user identifier.
// Precondition: peek() is a letter.
func (l *Lexer) scanIdentifier() Token {
	startLine, startCol := l.pos.Line, l.pos.Column
	start := l.pos.Index
	for l.pos.Index < len(l.src) {
		ch := l.src[l.pos.Index]
		if isLetter(ch) || isDigit(ch) || ch == '_' {
			l.pos.Advance(ch)
		} else {
			break
		}
	}
	lit := string(l.src[start:l.pos.Index])
	typ := lookupIdent(lit) // keyword or TOKEN_IDENT
	return l.makeToken(typ, lit, startLine, startCol)
}

func isLetter(ch rune) bool { return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') }
func isDigit(ch rune) bool  { return ch >= '0' && ch <= '9' }

func (l *Lexer) scanNumber() Token {
	startLine, startCol := l.pos.Line, l.pos.Column
	start := l.pos.Index
	isFloat := false

	// Leading '.' case
	if l.peek() == '.' {
		isFloat = true
		l.advance()
	}

	// Digits consumption
	for l.pos.Index < len(l.src) && isDigit((l.src[l.pos.Index])) {
		l.advance()
	}

	// Decimal check
	if !isFloat && l.peek() == '.' {
		nextIdx := l.pos.Index + 1
		if nextIdx >= len(l.src) || isDigit(l.src[nextIdx]) || !isLetter(l.src[nextIdx]) && l.src[nextIdx] != '_' {
			isFloat = true
			l.advance()
			for l.pos.Index < len(l.src) && isDigit((l.src[l.pos.Index])) {
				l.advance()
			}
		}
	}

	lit := string(l.src[start:l.pos.Index])
	if isFloat {
		return l.makeToken(TOKEN_FLOAT, lit, startLine, startCol)
	}
	return l.makeToken(TOKEN_INTEGER, lit, startLine, startCol)
}

func (l *Lexer) scanString() Token {
	startLine, startCol := l.pos.Line, l.pos.Column
	l.advance()

	var buf []rune
	for {
		if l.pos.Index >= len(l.src) {
			return l.makeToken(TOKEN_ILLEGAL, string(buf), startLine, startCol)
		}
		ch := l.advance()
		if ch == '\'' {
			if l.peek() == '\'' {
				l.advance()
				buf = append(buf, '\'')
			} else {
				break
			}
		} else {
			buf = append(buf, ch)
		}
	}

	return l.makeToken(TOKEN_STRING, string(buf), startLine, startCol)
}

func (l *Lexer) NextToken() Token {
	l.skipWhitespace()

	if l.pos.Index >= len(l.src) {
		return l.makeToken(TOKEN_EOF, "", l.pos.Line, l.pos.Column)
	}

	startLine, startCol := l.pos.Line, l.pos.Column
	ch := l.peek()

	// Identifier or Keyword
	if isLetter(ch) {
		return l.scanIdentifier()
	}

	// Number
	if isDigit(ch) {
		return l.scanNumber()
	}

	if ch == '.' {
		nextIdx := l.pos.Index + 1
		if nextIdx < len(l.src) && isDigit(l.src[nextIdx]) {
			return l.scanNumber()
		}
		l.advance()
		return l.makeToken(TOKEN_DOT, ".", startLine, startCol)
	}

	// String literal
	if ch == '\'' {
		return l.scanString()
	}

	// Single character
	l.advance()
	switch ch {
	case '(':
		return l.makeToken(TOKEN_LPAREN, "(", startLine, startCol)
	case ')':
		return l.makeToken(TOKEN_RPAREN, ")", startLine, startCol)
	case ',':
		return l.makeToken(TOKEN_COMMA, ",", startLine, startCol)
	case ';':
		return l.makeToken(TOKEN_SEMICOLON, ";", startLine, startCol)
	case '+':
		return l.makeToken(TOKEN_PLUS, "+", startLine, startCol)
	case '-':
		return l.makeToken(TOKEN_MINUS, "-", startLine, startCol)
	case '*':
		return l.makeToken(TOKEN_STAR, "*", startLine, startCol)
	case '/':
		return l.makeToken(TOKEN_SLASH, "/", startLine, startCol)
	case '%':
		return l.makeToken(TOKEN_PERCENT, "%", startLine, startCol)
	case '=':
		return l.makeToken(TOKEN_EQ, "=", startLine, startCol)
	// ── Multi-character operators ──
	case '<':
		if l.peek() == '=' {
			l.advance()
			return l.makeToken(TOKEN_LTE, "<=", startLine, startCol)
		}
		if l.peek() == '>' {
			l.advance()
			return l.makeToken(TOKEN_NEQ, "<>", startLine, startCol)
		}
		return l.makeToken(TOKEN_LT, "<", startLine, startCol)
	case '>':
		if l.peek() == '=' {
			l.advance()
			return l.makeToken(TOKEN_GTE, ">=", startLine, startCol)
		}
		return l.makeToken(TOKEN_GT, ">", startLine, startCol)
	case '!':
		if l.peek() == '=' {
			l.advance()
			return l.makeToken(TOKEN_NEQ, "!=", startLine, startCol)
		}
		return l.makeToken(TOKEN_ILLEGAL, "!", startLine, startCol)
	default:
		return l.makeToken(TOKEN_ILLEGAL, string(ch), startLine, startCol)
	}
}
