package utils

import (
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/sql/lexer"
)

// ---------- LookaheadIterator ------------------------------------------------

func TestLookaheadIterator_BasicNextAndPeek(t *testing.T) {
	seq := []int{10, 20, 30}
	idx := 0
	iter := NewLookaheadIterator(func() int {
		v := seq[idx]
		idx++
		return v
	})

	// Peek should return first element without consuming.
	if got := iter.Peek(); got != 10 {
		t.Fatalf("Peek() = %d, want 10", got)
	}
	if got := iter.Peek(); got != 10 {
		t.Fatalf("second Peek() = %d, want 10 (should be idempotent)", got)
	}
	if iter.Consumed() != 0 {
		t.Fatalf("Consumed() = %d after Peek, want 0", iter.Consumed())
	}

	// Next should consume the peeked element.
	if got := iter.Next(); got != 10 {
		t.Fatalf("Next() = %d, want 10", got)
	}
	if iter.Consumed() != 1 {
		t.Fatalf("Consumed() = %d after first Next, want 1", iter.Consumed())
	}

	// Next without prior Peek.
	if got := iter.Next(); got != 20 {
		t.Fatalf("Next() = %d, want 20", got)
	}
	if iter.Consumed() != 2 {
		t.Fatalf("Consumed() = %d, want 2", iter.Consumed())
	}

	// Peek then Next.
	if got := iter.Peek(); got != 30 {
		t.Fatalf("Peek() = %d, want 30", got)
	}
	if got := iter.Next(); got != 30 {
		t.Fatalf("Next() = %d, want 30", got)
	}
	if iter.Consumed() != 3 {
		t.Fatalf("Consumed() = %d, want 3", iter.Consumed())
	}
}

func TestLookaheadIterator_NextWithoutPeek(t *testing.T) {
	calls := 0
	iter := NewLookaheadIterator(func() int {
		calls++
		return calls
	})

	// Calling Next without Peek should call nextFn directly.
	if got := iter.Next(); got != 1 {
		t.Fatalf("Next() = %d, want 1", got)
	}
	if got := iter.Next(); got != 2 {
		t.Fatalf("Next() = %d, want 2", got)
	}
	if calls != 2 {
		t.Fatalf("nextFn called %d times, want 2", calls)
	}
}

func TestLookaheadIterator_PeekDoesNotCallNextFnTwice(t *testing.T) {
	calls := 0
	iter := NewLookaheadIterator(func() string {
		calls++
		return "hello"
	})

	_ = iter.Peek()
	_ = iter.Peek()
	_ = iter.Peek()

	if calls != 1 {
		t.Fatalf("nextFn called %d times, want 1 (Peek should buffer)", calls)
	}
}

func TestLookaheadIterator_Consumed_StartsAtZero(t *testing.T) {
	iter := NewLookaheadIterator(func() int { return 0 })
	if iter.Consumed() != 0 {
		t.Fatalf("Consumed() = %d, want 0", iter.Consumed())
	}
}

func TestLookaheadIterator_Consumed_IncrementedByNext(t *testing.T) {
	iter := NewLookaheadIterator(func() int { return 42 })
	for i := 1; i <= 5; i++ {
		iter.Next()
		if iter.Consumed() != i {
			t.Fatalf("after %d Next calls: Consumed() = %d", i, iter.Consumed())
		}
	}
}

func TestLookaheadIterator_Consumed_NotIncrementedByPeek(t *testing.T) {
	iter := NewLookaheadIterator(func() int { return 1 })
	iter.Peek()
	iter.Peek()
	if iter.Consumed() != 0 {
		t.Fatalf("Peek should not increment Consumed; got %d", iter.Consumed())
	}
}

// ---------- ExpectNextValue --------------------------------------------------

func TestLookaheadIterator_ExpectNextValue_Match(t *testing.T) {
	seq := []int{5, 10, 15}
	idx := 0
	iter := NewLookaheadIterator(func() int {
		v := seq[idx]
		idx++
		return v
	})

	eq := func(a, b int) bool { return a == b }

	result, ok := iter.ExpectNextValue(5, eq)
	if !ok {
		t.Fatal("expected match, got ok=false")
	}
	if result != 5 {
		t.Fatalf("matched value = %d, want 5", result)
	}
	if iter.Consumed() != 1 {
		t.Fatalf("Consumed() = %d, want 1 (match should consume)", iter.Consumed())
	}
}

func TestLookaheadIterator_ExpectNextValue_NoMatch(t *testing.T) {
	seq := []int{5, 10}
	idx := 0
	iter := NewLookaheadIterator(func() int {
		v := seq[idx]
		idx++
		return v
	})

	eq := func(a, b int) bool { return a == b }

	result, ok := iter.ExpectNextValue(999, eq)
	if ok {
		t.Fatalf("expected ok=false for non-match, got ok=true with %d", result)
	}
	if iter.Consumed() != 0 {
		t.Fatalf("Consumed() = %d, want 0 (non-match should not consume)", iter.Consumed())
	}

	// The element should still be available.
	if got := iter.Next(); got != 5 {
		t.Fatalf("Next() after failed expect = %d, want 5", got)
	}
}

func TestLookaheadIterator_ExpectNextValue_ConsecutiveMatches(t *testing.T) {
	seq := []int{1, 2, 3}
	idx := 0
	iter := NewLookaheadIterator(func() int {
		v := seq[idx]
		idx++
		return v
	})

	eq := func(a, b int) bool { return a == b }

	for i, expected := range seq {
		result, ok := iter.ExpectNextValue(expected, eq)
		if !ok {
			t.Fatalf("step %d: expected match for %d, got ok=false", i, expected)
		}
		if result != expected {
			t.Fatalf("step %d: got %d, want %d", i, result, expected)
		}
	}
	if iter.Consumed() != 3 {
		t.Fatalf("Consumed() = %d, want 3", iter.Consumed())
	}
}

func TestLookaheadIterator_ExpectNextValue_FailThenSucceed(t *testing.T) {
	seq := []int{1, 2}
	idx := 0
	iter := NewLookaheadIterator(func() int {
		v := seq[idx]
		idx++
		return v
	})

	eq := func(a, b int) bool { return a == b }

	// Fail: looking for 2, but next is 1.
	if _, ok := iter.ExpectNextValue(2, eq); ok {
		t.Fatal("expected ok=false, got ok=true")
	}
	// Succeed: looking for 1, and next is 1.
	if r, ok := iter.ExpectNextValue(1, eq); !ok {
		t.Fatal("expected match for 1, got ok=false")
	} else if r != 1 {
		t.Fatalf("got %d, want 1", r)
	}
}

// ---------- ExpectNextMatches ------------------------------------------------

func TestLookaheadIterator_ExpectNextMatches_PredicateTrue(t *testing.T) {
	iter := NewLookaheadIterator(func() int { return 42 })

	result, ok := iter.ExpectNextMatches(func(v int) bool { return v > 0 })
	if !ok {
		t.Fatal("expected match, got ok=false")
	}
	if result != 42 {
		t.Fatalf("matched = %d, want 42", result)
	}
	if iter.Consumed() != 1 {
		t.Fatalf("Consumed() = %d, want 1", iter.Consumed())
	}
}

func TestLookaheadIterator_ExpectNextMatches_PredicateFalse(t *testing.T) {
	iter := NewLookaheadIterator(func() int { return 42 })

	_, ok := iter.ExpectNextMatches(func(v int) bool { return v < 0 })
	if ok {
		t.Fatal("expected ok=false, got ok=true")
	}
	if iter.Consumed() != 0 {
		t.Fatalf("Consumed() = %d, want 0 (no consume on mismatch)", iter.Consumed())
	}
}

func TestLookaheadIterator_ExpectNextMatches_PredicateCalledOnce(t *testing.T) {
	iter := NewLookaheadIterator(func() int { return 1 })
	calls := 0
	_, _ = iter.ExpectNextMatches(func(v int) bool {
		calls++
		return false
	})
	if calls != 1 {
		t.Fatalf("predicate called %d times, want 1", calls)
	}
}

func TestLookaheadIterator_ExpectNextMatches_DoesNotConsumeOnMismatch(t *testing.T) {
	seq := []string{"hello", "world"}
	idx := 0
	iter := NewLookaheadIterator(func() string {
		v := seq[idx]
		idx++
		return v
	})

	// Mismatch.
	_, ok := iter.ExpectNextMatches(func(v string) bool { return v == "world" })
	if ok {
		t.Fatal("expected ok=false, got ok=true")
	}

	// "hello" should still be there.
	got := iter.Next()
	if got != "hello" {
		t.Fatalf("Next() = %q, want 'hello' (should not have been consumed)", got)
	}
}

// ---------- Generic type support (strings) -----------------------------------

func TestLookaheadIterator_WithStrings(t *testing.T) {
	words := []string{"SELECT", "FROM", "WHERE"}
	idx := 0
	iter := NewLookaheadIterator(func() string {
		v := words[idx]
		idx++
		return v
	})

	if got := iter.Peek(); got != "SELECT" {
		t.Fatalf("Peek() = %q, want 'SELECT'", got)
	}
	if got := iter.Next(); got != "SELECT" {
		t.Fatalf("Next() = %q, want 'SELECT'", got)
	}
	if got := iter.Next(); got != "FROM" {
		t.Fatalf("Next() = %q, want 'FROM'", got)
	}
	if got := iter.Peek(); got != "WHERE" {
		t.Fatalf("Peek() = %q, want 'WHERE'", got)
	}
	if got := iter.Next(); got != "WHERE" {
		t.Fatalf("Next() = %q, want 'WHERE'", got)
	}
	if iter.Consumed() != 3 {
		t.Fatalf("Consumed() = %d, want 3", iter.Consumed())
	}
}

// ---------- Integration: LookaheadIterator wrapping the Lexer ----------------

func TestLookaheadIterator_WithLexer(t *testing.T) {
	l := lexer.NewLexer("test", "SELECT * FROM t;")
	iter := NewLookaheadIterator(func() lexer.Token {
		return l.NextToken()
	})

	// Peek should give SELECT.
	peeked := iter.Peek()
	if peeked.Type != lexer.TOKEN_SELECT {
		t.Fatalf("Peek() type = %v, want SELECT", peeked.Type)
	}

	// Next should consume the same SELECT.
	got := iter.Next()
	if got.Type != lexer.TOKEN_SELECT {
		t.Fatalf("Next() type = %v, want SELECT", got.Type)
	}

	// Next → STAR.
	got = iter.Next()
	if got.Type != lexer.TOKEN_STAR {
		t.Fatalf("Next() type = %v, want STAR", got.Type)
	}

	// Peek → FROM.
	peeked = iter.Peek()
	if peeked.Type != lexer.TOKEN_FROM {
		t.Fatalf("Peek() type = %v, want FROM", peeked.Type)
	}

	// ExpectNextValue should match FROM.
	eq := func(a, b lexer.Token) bool { return a.Type == b.Type }
	result, ok := iter.ExpectNextValue(lexer.Token{Type: lexer.TOKEN_FROM}, eq)
	if !ok {
		t.Fatal("expected FROM to match, got ok=false")
	}
	if result.Literal != "FROM" {
		t.Fatalf("matched literal = %q, want 'FROM'", result.Literal)
	}

	// ExpectNextMatches for an identifier.
	result, ok = iter.ExpectNextMatches(func(tok lexer.Token) bool {
		return tok.Type == lexer.TOKEN_IDENT
	})
	if !ok {
		t.Fatal("expected IDENT match, got ok=false")
	}
	if result.Literal != "t" {
		t.Fatalf("matched literal = %q, want 't'", result.Literal)
	}

	// SEMICOLON.
	got = iter.Next()
	if got.Type != lexer.TOKEN_SEMICOLON {
		t.Fatalf("Next() type = %v, want SEMICOLON", got.Type)
	}

	// EOF.
	got = iter.Next()
	if got.Type != lexer.TOKEN_EOF {
		t.Fatalf("Next() type = %v, want EOF", got.Type)
	}

	if iter.Consumed() != 6 {
		t.Fatalf("Consumed() = %d, want 6", iter.Consumed())
	}
}

func TestLookaheadIterator_ExpectNextValue_NoMatchDoesNotAdvanceLexer(t *testing.T) {
	l := lexer.NewLexer("test", "SELECT FROM")
	iter := NewLookaheadIterator(func() lexer.Token {
		return l.NextToken()
	})

	eq := func(a, b lexer.Token) bool { return a.Type == b.Type }

	// Try to match FROM, but next is SELECT — should fail.
	_, ok := iter.ExpectNextValue(lexer.Token{Type: lexer.TOKEN_FROM}, eq)
	if ok {
		t.Fatal("expected ok=false, got ok=true")
	}

	// SELECT should still be the next token.
	got := iter.Next()
	if got.Type != lexer.TOKEN_SELECT {
		t.Fatalf("Next() after failed expect = %v, want SELECT", got.Type)
	}
}

// ---------- Edge: struct types with LookaheadIterator ------------------------

type testPair struct {
	key   string
	value int
}

func TestLookaheadIterator_WithStructs(t *testing.T) {
	pairs := []testPair{
		{"a", 1},
		{"b", 2},
		{"c", 3},
	}
	idx := 0
	iter := NewLookaheadIterator(func() testPair {
		v := pairs[idx]
		idx++
		return v
	})

	// Peek.
	peeked := iter.Peek()
	if peeked.key != "a" || peeked.value != 1 {
		t.Fatalf("Peek() = %+v, want {a 1}", peeked)
	}

	// ExpectNextMatches with struct field check.
	result, ok := iter.ExpectNextMatches(func(p testPair) bool {
		return p.key == "a"
	})
	if !ok {
		t.Fatal("expected match, got ok=false")
	}
	if result.key != "a" || result.value != 1 {
		t.Fatalf("matched = %+v, want {a 1}", result)
	}

	// Next.
	got := iter.Next()
	if got.key != "b" {
		t.Fatalf("Next() = %+v, want key='b'", got)
	}

	if iter.Consumed() != 2 {
		t.Fatalf("Consumed() = %d, want 2", iter.Consumed())
	}
}
