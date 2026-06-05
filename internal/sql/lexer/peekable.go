package lexer

// PeekableIterator provides up to 1 element lookahead over a stream of elements.
//
// It is intended for use by tokenizers and parsers to enforce an LL(1) design:
// only one element may be observed ahead of the consumption point. The backing
// stream is a simple function, so no buffering array is needed and arbitrarily
// large inputs are supported with no extra overhead.
//
// This type is not safe for concurrent use.
type PeekableIterator[T any] struct {
	nextFn func() T // produces the next element on demand
	peeked *T       // buffered lookahead element, nil if not yet peeked
	count  int      // number of elements consumed via Next()
}

// NewPeekableIterator creates a PeekableIterator backed by the given function.
// nextFn is called each time a new element is needed.
func NewPeekableIterator[T any](nextFn func() T) *PeekableIterator[T] {
	return &PeekableIterator[T]{nextFn: nextFn}
}

// Peek returns the next element without consuming it.
// Successive calls return the same element until Next() is called.
func (p *PeekableIterator[T]) Peek() T {
	if p.peeked != nil {
		return *p.peeked
	}
	v := p.nextFn()
	p.peeked = &v
	return v
}

// Next consumes and returns the next element.
func (p *PeekableIterator[T]) Next() T {
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
func (p *PeekableIterator[T]) ExpectNextValue(expected T, eq func(a, b T) bool) *T {
	return p.ExpectNextMatches(func(v T) bool { return eq(v, expected) })
}

// ExpectNextMatches consumes and returns the next element if it satisfies the
// predicate. Returns a pointer to the consumed element, or nil otherwise.
// The predicate is called at most once.
func (p *PeekableIterator[T]) ExpectNextMatches(predicate func(T) bool) *T {
	v := p.Peek()
	if predicate(v) {
		p.Next() // consume
		return &v
	}
	return nil
}

// Count returns the number of elements consumed via Next() so far.
func (p *PeekableIterator[T]) Count() int {
	return p.count
}
