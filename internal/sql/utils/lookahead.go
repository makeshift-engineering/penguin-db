package utils

// LookaheadIterator provides single-element lookahead over an arbitrary stream.
// Used by the parser to inspect the next token without consuming it.
// Not used by the lexer itself.
//
// This type is not safe for concurrent use.
type LookaheadIterator[T any] struct {
	nextFn    func() T // produces the next element on demand
	peeked    T        // buffered lookahead element
	hasPeeked bool     // true if peeked contains a valid buffered element
	consumed  int      // number of elements consumed via Next()
}

// NewLookaheadIterator creates a LookaheadIterator backed by the given function.
// nextFn is called each time a new element is needed.
func NewLookaheadIterator[T any](nextFn func() T) *LookaheadIterator[T] {
	return &LookaheadIterator[T]{nextFn: nextFn}
}

// Peek returns the next element without consuming it.
// Successive calls return the same element until Next() is called.
func (p *LookaheadIterator[T]) Peek() T {
	if p.hasPeeked {
		return p.peeked
	}
	p.peeked = p.nextFn()
	p.hasPeeked = true
	return p.peeked
}

// Next consumes and returns the next element.
func (p *LookaheadIterator[T]) Next() T {
	var v T
	if p.hasPeeked {
		v = p.peeked
		p.hasPeeked = false
	} else {
		v = p.nextFn()
	}
	p.consumed++
	return v
}

// ExpectNextValue consumes and returns the next element if it equals expected.
// Returns the consumed element and true, or zero-value and false if it did not match.
func (p *LookaheadIterator[T]) ExpectNextValue(expected T, eq func(a, b T) bool) (T, bool) {
	return p.ExpectNextMatches(func(v T) bool { return eq(v, expected) })
}

// ExpectNextMatches consumes and returns the next element if it satisfies the
// predicate. Returns the consumed element and true, or zero-value and false otherwise.
// The predicate is called at most once.
func (p *LookaheadIterator[T]) ExpectNextMatches(predicate func(T) bool) (T, bool) {
	v := p.Peek()
	if predicate(v) {
		p.Next() // consume
		return v, true
	}
	var zero T
	return zero, false
}

// Consumed returns the number of elements consumed via Next() so far.
func (p *LookaheadIterator[T]) Consumed() int {
	return p.consumed
}
