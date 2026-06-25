package sstable

import (
	"encoding/binary"
	"hash/fnv"
	"math"
)

// BloomFilter holds the bit vector, number of hash functions, and total bit count.
type BloomFilter struct {
	bits          []byte
	numHashes     uint8
	totalBitCount int
}

// NewBloomFilter creates a Bloom filter optimized for the given number of keys and bits per key.
func NewBloomFilter(numKeys, bitsPerKey int) *BloomFilter {
	if numKeys < 0 {
		numKeys = 0
	}

	totalBitCount := numKeys * bitsPerKey

	// Ensure even an empty/small filter has a minimum size of 64 bits (8 bytes)
	// to avoid division by zero or extremely poor false positive rates in tiny vectors.
	if totalBitCount < 64 {
		totalBitCount = 64
	}

	// Calculate the optimal number of hash functions (k) for the given bits per key.
	// The formula k = (m/n) * ln(2) minimizes the false positive probability.
	k := int(float64(bitsPerKey) * math.Ln2)

	// Clamp the number of hash functions to a reasonable range.
	// At least 1 hash function is required. A maximum of 30 prevents excessive 
	// CPU usage during hashing while providing diminishing returns beyond that.
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
	if f == nil || f.totalBitCount <= 0 || len(f.bits) == 0 || f.numHashes == 0 {
		return
	}
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
	if f == nil || f.totalBitCount <= 0 || len(f.bits) == 0 || f.numHashes == 0 {
		return false
	}
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

func (f *BloomFilter) Bytes() []byte {
	out := make([]byte, len(f.bits))
	copy(out, f.bits)
	return out
}

func (f *BloomFilter) NumHashes() uint8 {
	return f.numHashes
}

// NewBloomFilterFromBytes reconstructs a BloomFilter from a serialised bit
// vector and hash count. This is used when reading an SSTable to restore the
// filter from the Bloom Block without re-inserting every key.
func NewBloomFilterFromBytes(data []byte, numHashes uint8) *BloomFilter {
	bits := make([]byte, len(data))
	copy(bits, data)

	return &BloomFilter{
		bits:          bits,
		numHashes:     numHashes,
		totalBitCount: len(bits) * 8,
	}
}

// hash is an internal helper to generate the two base hashes using FNV-1a.
func (f *BloomFilter) hash(key []byte) (h1, h2 uint32) {
	hasher := fnv.New32a()

	hasher.Write(key)
	h1 = hasher.Sum32()

	hasher.Reset()

	// Allocate a 5-byte slice: 4 bytes for h1, 1 byte for salt
	h1Bytes := make([]byte, 5)
	binary.LittleEndian.PutUint32(h1Bytes, h1)
	h1Bytes[4] = 0x01 // Arbitrary salt byte

	hasher.Write(h1Bytes)
	h2 = hasher.Sum32()
	
	// If h2 is exactly 0, the combined hash function (h1 + i*h2) would just equal h1 
	// for all iterations, effectively degrading the filter to use only a single hash 
	// function. Setting h2 to 1 prevents this edge case.
	if h2 == 0 {
		h2 = 1
	}

	return h1, h2
}
