package memtable

import (
	"math/rand"
)

// node is an internal element of the skip list. Each node stores a key-value pair
// along with a deletion marker and a tower of forward pointers, one per level the
// node participates in. The height of the tower is determined at insertion time by
// randomLevel and never changes afterwards.
//
// node is intentionally unexported; callers interact with the skip list exclusively
// through SkipList and Iterator.
type node struct {
	key       []byte
	value     []byte
	isDeleted bool
	next      []*node
}

// newNode allocates and returns a new node with the supplied key, value and tower
// height. The isDeleted field defaults to false and all forward pointers default
// to nil, matching their zero values.
func newNode(key, value []byte, level int) *node {
	return &node{
		key:   key,
		value: value,
		next:  make([]*node, level),
	}
}

// randomLevel determines the height of a newly inserted node's tower using a
// geometric distribution with a promotion probability of 0.5. The returned level
// is always in the range [1, MaxLevel].
func randomLevel() int {
	level := 1
	for rand.Float32() < 0.5 && level < MaxLevel {
		level++
	}
	return level
}
