package memtable

import (
	"bytes"
	"sync"
	"sync/atomic"
)

const (
	MaxLevel = 12
)

type SkipList struct {
	headNode           *Node
	highestActiveLevel int
	currentSizeBytes   int64
	mutex              sync.RWMutex
}

func NewSkipList() *SkipList {
	return &SkipList{
		headNode:           NewNode(nil, nil, MaxLevel),
		highestActiveLevel: 1,
		currentSizeBytes:   0,
	}
}

func (skipList *SkipList) Get(key []byte) ([]byte, error) {
	skipList.mutex.RLock()
	defer skipList.mutex.RUnlock()

	currentNode := skipList.headNode

	for levelIndex := skipList.highestActiveLevel - 1; levelIndex >= 0; levelIndex-- {
		for currentNode.Next[levelIndex] != nil && bytes.Compare(currentNode.Next[levelIndex].Key, key) < 0 {
			currentNode = currentNode.Next[levelIndex]
		}
	}

	currentNode = currentNode.Next[0]

	if currentNode != nil && bytes.Equal(currentNode.Key, key) {
		if currentNode.IsDeleted {
			return nil, ErrKeyNotFound
		}
		return currentNode.Value, nil
	}

	return nil, ErrKeyNotFound
}

func (skipList *SkipList) Put(key, value []byte) error {
	skipList.mutex.Lock()
	defer skipList.mutex.Unlock()

	predecessorNodes := make([]*Node, MaxLevel)
	currentNode := skipList.headNode

	for levelIndex := skipList.highestActiveLevel - 1; levelIndex >= 0; levelIndex-- {
		for currentNode.Next[levelIndex] != nil && bytes.Compare(currentNode.Next[levelIndex].Key, key) < 0 {
			currentNode = currentNode.Next[levelIndex]
		}
		predecessorNodes[levelIndex] = currentNode
	}

	currentNode = currentNode.Next[0]

	if currentNode != nil && bytes.Equal(currentNode.Key, key) {
		sizeDifference := len(value) - len(currentNode.Value)
		atomic.AddInt64(&skipList.currentSizeBytes, int64(sizeDifference))

		currentNode.Value = value
		currentNode.IsDeleted = false
		return nil
	}

	newNodeHeight := randomLevel()

	if newNodeHeight > skipList.highestActiveLevel {
		for levelIndex := skipList.highestActiveLevel; levelIndex < newNodeHeight; levelIndex++ {
			predecessorNodes[levelIndex] = skipList.headNode
		}
		skipList.highestActiveLevel = newNodeHeight
	}

	newNode := NewNode(key, value, newNodeHeight)

	for levelIndex := 0; levelIndex < newNodeHeight; levelIndex++ {
		newNode.Next[levelIndex] = predecessorNodes[levelIndex].Next[levelIndex]
		predecessorNodes[levelIndex].Next[levelIndex] = newNode
	}

	atomic.AddInt64(&skipList.currentSizeBytes, int64(len(key)+len(value)))

	return nil
}

func (skipList *SkipList) Delete(key []byte) error {
	skipList.mutex.Lock()
	defer skipList.mutex.Unlock()

	currentNode := skipList.headNode

	for levelIndex := skipList.highestActiveLevel - 1; levelIndex >= 0; levelIndex-- {
		for currentNode.Next[levelIndex] != nil && bytes.Compare(currentNode.Next[levelIndex].Key, key) < 0 {
			currentNode = currentNode.Next[levelIndex]
		}
	}

	currentNode = currentNode.Next[0]

	if currentNode != nil && bytes.Equal(currentNode.Key, key) {
		if !currentNode.IsDeleted {
			atomic.AddInt64(&skipList.currentSizeBytes, int64(-len(currentNode.Value)))

			currentNode.IsDeleted = true
			currentNode.Value = nil
		}
		return nil
	}

	return nil
}
