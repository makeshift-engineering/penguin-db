package memtable

import (
	"math/rand"
)

type Node struct {
	Key       []byte
	Value     []byte
	IsDeleted bool
	Next      []*Node
}

func NewNode(key, value []byte, level int) *Node {
	return &Node{
		Key:       key,
		Value:     value,
		IsDeleted: false,
		Next:      make([]*Node, level),
	}
}

func randomLevel() int {
	level := 1

	for rand.Float32() < 0.5 && level < MaxLevel {
		level++
	}
	return level
}
