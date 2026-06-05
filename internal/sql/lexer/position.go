package lexer

// Position represents a position in the source input.
// It tracks three values:
//
//   - Index:  absolute character offset from the start of the entire input (0-based).
//     It counts every character (including newlines) and never resets.
//   - Line:   the current line number (1-based). Increments only when a '\n' is encountered.
//   - Column: the position within the current line (1-based). Resets to 1 on every new line.
type Position struct {
	Index  int
	Line   int
	Column int
}

// NewPosition returns a Position initialised to the start of the input.
func NewPosition() Position {
	return Position{
		Index:  0,
		Line:   1,
		Column: 1,
	}
}

// Advance moves the position forward by one character.
// If the character is a newline ('\n'), the line number is incremented
// and the column is reset to 1. Otherwise the column is incremented.
// The absolute index is always incremented by 1.
func (p *Position) Advance(ch rune) {
	p.Index++
	if ch == '\n' {
		p.Line++
		p.Column = 1
	} else {
		p.Column++
	}
}
