package sstable

import (
	"encoding/binary"
	"hash/fnv"
	"math"
)

// Filter holds the bit vector, number of hash functions, and total bit count.
type BloomFilter struct {
	bits          []byte
	numHashes     uint8
	totalBitCount int
}

// New creates a Bloom filter optimized for the given number of keys and bits per key.
func NewBloomFilter(numKeys int, bitsPerKey int) *BloomFilter {
	if numKeys < 0 {
		numKeys = 0
	}

	totalBitCount := numKeys * bitsPerKey

	// Ensure even an empty/small filter has a minimum size to avoid /0 or tiny vectors
	if totalBitCount < 64 {
		totalBitCount = 64
	}

	k := int(float64(bitsPerKey) * math.Ln2)

	if k < 1 {
		k = 1
	} else if k > 30 {
		k = 30
	}

	byteLength := (totalBitCount + 7) / 8
	bits := make([]byte, byteLength)

	return &BloomFilter{
		bits:          bits,
		numHashes:     uint8(k),
		totalBitCount: totalBitCount,
	}
}

// Add inserts a key into the Bloom filter.
func (f *BloomFilter) Add(key []byte) {
	baseHash1, baseHash2 := f.hash(key)

	for i := uint8(0); i < f.numHashes; i++ {
		targetBitIndex := (baseHash1 + uint32(i)*baseHash2) % uint32(f.totalBitCount)

		byteIndex := targetBitIndex / 8
		bitPosition := targetBitIndex % 8

		bitMask := byte(1 << bitPosition)

		f.bits[byteIndex] |= bitMask
	}
}

// MayContain returns true if the key might be in the filter, false if it is definitely absent.
func (f *BloomFilter) MayContain(key []byte) bool {
	baseHash1, baseHash2 := f.hash(key)

	for i := uint8(0); i < f.numHashes; i++ {
		targetBitIndex := (baseHash1 + uint32(i)*baseHash2) % uint32(f.totalBitCount)

		byteIndex := targetBitIndex / 8
		bitPosition := targetBitIndex % 8

		bitMask := byte(1 << bitPosition)

		if f.bits[byteIndex]&bitMask == 0 {
			return false
		}
	}

	return true
}

func (bf *BloomFilter) Bytes() []byte {
	return bf.bits
}

func (bf *BloomFilter) NumHashes() uint8 {
	return bf.numHashes
}

// hash is an internal helper to generate the two base hashes using FNV-1a.
func (f *BloomFilter) hash(key []byte) (uint32, uint32) {
	hasher := fnv.New32a()

	hasher.Write(key)
	h1 := hasher.Sum32()

	hasher.Reset()

	// Allocate a 5-byte slice: 4 bytes for h1, 1 byte for salt
	h1Bytes := make([]byte, 5)
	binary.LittleEndian.PutUint32(h1Bytes, h1)
	h1Bytes[4] = 0x01 // Arbitrary salt byte

	hasher.Write(h1Bytes)
	h2 := hasher.Sum32()

	return h1, h2
}
