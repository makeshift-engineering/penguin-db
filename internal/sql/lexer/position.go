package lexer

import "github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"

// position is the internal scanning cursor. Not exported.
type position struct {
	index  int // absolute 0-based byte offset from start of input
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
