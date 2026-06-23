package lexer

import (
	"testing"
)

// TestPosition_AdvanceAndSnapshot tests position advance and snapshot.
func TestPosition_AdvanceAndSnapshot(t *testing.T) {
	p := position{line: 1, column: 1}

	p.advance('S', 1)
	snap1 := p.snapshot()
	if snap1.Line != 1 || snap1.Col != 2 || snap1.Offset != 1 {
		t.Errorf("Snap 1: got %d:%d (offset %d), want 1:2 (offset 1)", snap1.Line, snap1.Col, snap1.Offset)
	}

	p.advance('\n', 1)
	snap2 := p.snapshot()
	if snap2.Line != 2 || snap2.Col != 1 || snap2.Offset != 2 {
		t.Errorf("Snap 2: got %d:%d (offset %d), want 2:1 (offset 2)", snap2.Line, snap2.Col, snap2.Offset)
	}

	p.advance('🚀', 4)
	snap3 := p.snapshot()
	if snap3.Line != 2 || snap3.Col != 2 || snap3.Offset != 6 {
		t.Errorf("Snap 3: got %d:%d (offset %d), want 2:2 (offset 6)", snap3.Line, snap3.Col, snap3.Offset)
	}

	p.advance('x', 1)
	snap4 := p.snapshot()
	if snap4.Line != 2 || snap4.Col != 3 || snap4.Offset != 7 {
		t.Errorf("Snap 4: got %d:%d (offset %d), want 2:3 (offset 7)", snap4.Line, snap4.Col, snap4.Offset)
	}
}
