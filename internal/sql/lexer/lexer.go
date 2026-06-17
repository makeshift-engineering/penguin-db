package lexer

import "github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"

// Position represents a position in the source input.
// It tracks three values:
//
//   - Index:  absolute character offset from the start of the entire input (0-based).
//     It counts every character (including newlines) and never resets.
//   - Line:   the current line number (1-based). Increments only when a '\n' is encountered.
//   - Column: the position within the current line (1-based). Resets to 1 on every new line.
type position struct {
	index  int
	line   int
	column int
}

// Lexer tokenizes SQL source text into a stream of Tokens
type Lexer struct {
	src  []rune   // full input as runes
	pos  position // current read cursor(line, col, index)
	Diag diagnostic.List
}

// NewLexer creates a Lexer fro the given SQL source string
func NewLexer(src string) *Lexer {
	return &Lexer{
		src: []rune(src),
		pos: position{0, 1, 1},
	}
}

func (l *Lexer) addError(code diagnostic.Code, line, col int, format string, args ...any) diagnostic.Diagnostic {
	diag := diagnostic.New(code, line, col, format, args...)
	l.Diag = append(l.Diag, diag)
	return diag
}

func (l *Lexer) peek() rune {
	if l.pos.index >= len(l.src) {
		return 0
	}
	return l.src[l.pos.index]
}

func (l *Lexer) peekNext() rune {
	if l.pos.index+1 >= len(l.src) {
		return 0
	}
	return l.src[l.pos.index+1]
}

func (l *Lexer) advance() rune {
	if l.pos.index >= len(l.src) {
		return 0
	}
	ch := l.src[l.pos.index]
	l.pos.index++
	if ch == '\n' {
		l.pos.line++
		l.pos.column = 1
	} else {
		l.pos.column++
	}
	return ch
}

// skipLineComment discards everything from the current position to end-of-line.
// Precondition: the two leading '-' characters have already been consumed.
func (l *Lexer) skipLineComment() {
	for l.pos.index < len(l.src) && l.src[l.pos.index] != '\n' {
		l.advance()
	}
}

// skipBlockComment discards everything up to and including the closing */.
// Precondition: the opening /* has already been consumed.
func (l *Lexer) skipBlockComment(openLine, openCol int) error {
	for l.pos.index < len(l.src) {
		if l.peek() == '*' && l.peekNext() == '/' {
			l.advance() // *
			l.advance() // /
			return nil
		}
		l.advance()
	}
	// End of input without finding */
	return l.addError(ErrUnterminatedComment, l.pos.line, l.pos.column,
		"expected '*/' to close '/*' opened at %d:%d", openLine, openCol)
}

// skipWhitespaceAndComments returns an error only for an unterminated block comment.
// All other skipped content (whitespace, line comments) is infallible.
func (l *Lexer) skipWhitespaceAndComments() error {
	for l.pos.index < len(l.src) {
		ch := l.src[l.pos.index]
		switch {
		case ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n':
			l.advance()

		case ch == '-' && l.peekNext() == '-':
			l.advance()
			l.advance()
			l.skipLineComment()

		case ch == '/' && l.peekNext() == '*':
			openLine, openCol := l.pos.line, l.pos.column
			l.advance()
			l.advance()
			if err := l.skipBlockComment(openLine, openCol); err != nil {
				return err
			}

		default:
			return nil
		}
	}
	return nil
}

// makeToken is a convenience to build a Token with the given fields.
func (l *Lexer) makeToken(typ TokenType, lit string, line, col int) Token {
	return Token{Type: typ, Literal: lit, Line: line, Col: col}
}

// scanIdentifier reads a keyword or user identifier.
// Precondition: peek() is a letter.
func (l *Lexer) scanIdentifier() Token {
	startLine, startCol := l.pos.line, l.pos.column
	start := l.pos.index
	for l.pos.index < len(l.src) {
		ch := l.src[l.pos.index]
		if isIdentPart(ch) {
			l.advance()
		} else {
			break
		}
	}
	lit := string(l.src[start:l.pos.index])
	typ := lookupIdent(lit) // keyword or TOKEN_IDENT
	return l.makeToken(typ, lit, startLine, startCol)
}

func isIdentStart(ch rune) bool { return isLetter(ch) || ch == '_' }
func isIdentPart(ch rune) bool  { return isLetter(ch) || isDigit(ch) || ch == '_' }

func isLetter(ch rune) bool { return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') }
func isDigit(ch rune) bool  { return ch >= '0' && ch <= '9' }

func (l *Lexer) scanNumber() Token {
	startLine, startCol := l.pos.line, l.pos.column
	start := l.pos.index
	isFloat := false

	// Leading '.' case
	if l.peek() == '.' {
		isFloat = true
		l.advance()
	}

	// Digits consumption
	for l.pos.index < len(l.src) && isDigit((l.src[l.pos.index])) {
		l.advance()
	}

	// Decimal check
	if !isFloat && l.peek() == '.' {
		nextCh := l.peekNext()
		if nextCh == 0 || isDigit(nextCh) || (!isLetter(nextCh) && nextCh != '_') {
			isFloat = true
			l.advance()
			for l.pos.index < len(l.src) && isDigit((l.src[l.pos.index])) {
				l.advance()
			}
		}
	}

	lit := string(l.src[start:l.pos.index])
	if isFloat {
		return l.makeToken(TOKEN_FLOAT, lit, startLine, startCol)
	}
	return l.makeToken(TOKEN_INTEGER, lit, startLine, startCol)
}

func (l *Lexer) scanString() (Token, error) {
	startLine, startCol := l.pos.line, l.pos.column
	l.advance() // consume opening '

	var buf []rune
	for {
		if l.pos.index >= len(l.src) {
			return l.makeToken(TOKEN_ILLEGAL, string(buf), startLine, startCol),
				l.addError(ErrUnterminatedString, l.pos.line, l.pos.column,
					"expected closing ' (string opened at %d:%d)", startLine, startCol)
		}

		ch := l.advance()
		if ch == '\'' {
			if l.peek() == '\'' { // '' is the SQL escape for a literal single-quote
				l.advance()
				buf = append(buf, '\'')
			} else {
				break // normal close
			}
		} else {
			buf = append(buf, ch)
		}
	}

	return l.makeToken(TOKEN_STRING, string(buf), startLine, startCol), nil
}

func (l *Lexer) NextToken() (Token, error) {
	if err := l.skipWhitespaceAndComments(); err != nil {
		return l.makeToken(TOKEN_EOF, "", l.pos.line, l.pos.column), err
	}

	if l.pos.index >= len(l.src) {
		return l.makeToken(TOKEN_EOF, "", l.pos.line, l.pos.column), nil
	}

	startLine, startCol := l.pos.line, l.pos.column
	ch := l.peek()

	if isIdentStart(ch) {
		return l.scanIdentifier(), nil
	}
	if isDigit(ch) {
		return l.scanNumber(), nil
	}
	if ch == '.' {
		if next := l.peekNext(); next != 0 && isDigit(next) {
			return l.scanNumber(), nil
		}
		l.advance()
		return l.makeToken(TOKEN_DOT, ".", startLine, startCol), nil
	}
	if ch == '\'' {
		return l.scanString()
	}

	l.advance()
	switch ch {
	case '(':
		return l.makeToken(TOKEN_LPAREN, "(", startLine, startCol), nil
	case ')':
		return l.makeToken(TOKEN_RPAREN, ")", startLine, startCol), nil
	case ',':
		return l.makeToken(TOKEN_COMMA, ",", startLine, startCol), nil
	case ';':
		return l.makeToken(TOKEN_SEMICOLON, ";", startLine, startCol), nil
	case '+':
		return l.makeToken(TOKEN_PLUS, "+", startLine, startCol), nil
	case '-':
		return l.makeToken(TOKEN_MINUS, "-", startLine, startCol), nil
	case '*':
		return l.makeToken(TOKEN_STAR, "*", startLine, startCol), nil
	case '/':
		return l.makeToken(TOKEN_SLASH, "/", startLine, startCol), nil
	case '%':
		return l.makeToken(TOKEN_PERCENT, "%", startLine, startCol), nil
	case '=':
		return l.makeToken(TOKEN_EQ, "=", startLine, startCol), nil
	// ── Multi-character operators ──
	case '<':
		if l.peek() == '=' {
			l.advance()
			return l.makeToken(TOKEN_LTE, "<=", startLine, startCol), nil
		}
		if l.peek() == '>' {
			l.advance()
			return l.makeToken(TOKEN_NEQ, "<>", startLine, startCol), nil
		}
		return l.makeToken(TOKEN_LT, "<", startLine, startCol), nil
	case '>':
		if l.peek() == '=' {
			l.advance()
			return l.makeToken(TOKEN_GTE, ">=", startLine, startCol), nil
		}
		return l.makeToken(TOKEN_GT, ">", startLine, startCol), nil
	case '!':
		if l.peek() == '=' {
			l.advance()
			return l.makeToken(TOKEN_NEQ, "!=", startLine, startCol), nil
		}
		return l.makeToken(TOKEN_ILLEGAL, "!", startLine, startCol), l.addError(ErrUnexpectedChar, startLine, startCol, "'!'; did you mean '!='?")

	default:
		return l.makeToken(TOKEN_ILLEGAL, string(ch), startLine, startCol), l.addError(ErrUnexpectedChar, startLine, startCol, "%q", ch)
	}
}
