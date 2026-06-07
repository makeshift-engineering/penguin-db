package lexer

// LookaheadIterator provides single-element lookahead over an arbitrary stream.
// Used by the parser to inspect the next token without consuming it.
// Not used by the lexer itself.
//
// This type is not safe for concurrent use.
type LookaheadIterator[T any] struct {
	nextFn func() T // produces the next element on demand
	peeked *T       // buffered lookahead element, nil if not yet peeked
	count  int      // number of elements consumed via Next()
}

// NewLookaheadIterator creates a LookaheadIterator backed by the given function.
// nextFn is called each time a new element is needed.
func NewLookaheadIterator[T any](nextFn func() T) *LookaheadIterator[T] {
	return &LookaheadIterator[T]{nextFn: nextFn}
}

// Peek returns the next element without consuming it.
// Successive calls return the same element until Next() is called.
func (p *LookaheadIterator[T]) Peek() T {
	if p.peeked != nil {
		return *p.peeked
	}
	v := p.nextFn()
	p.peeked = &v
	return v
}

// Next consumes and returns the next element.
func (p *LookaheadIterator[T]) Next() T {
	var v T
	if p.peeked != nil {
		v = *p.peeked
		p.peeked = nil
	} else {
		v = p.nextFn()
	}
	p.count++
	return v
}

// ExpectNextValue consumes and returns the next element if it equals expected.
// Returns a pointer to the consumed element, or nil if it did not match.
func (p *LookaheadIterator[T]) ExpectNextValue(expected T, eq func(a, b T) bool) *T {
	return p.ExpectNextMatches(func(v T) bool { return eq(v, expected) })
}

// ExpectNextMatches consumes and returns the next element if it satisfies the
// predicate. Returns a pointer to the consumed element, or nil otherwise.
// The predicate is called at most once.
func (p *LookaheadIterator[T]) ExpectNextMatches(predicate func(T) bool) *T {
	v := p.Peek()
	if predicate(v) {
		p.Next() // consume
		return &v
	}
	return nil
}

// Count returns the number of elements consumed via Next() so far.
func (p *LookaheadIterator[T]) Count() int {
	return p.count
}
