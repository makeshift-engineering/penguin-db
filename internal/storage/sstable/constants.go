package sstable

// Data entry field sizes
const (
	keyLenSize      = 2
	valueLenSize    = 4
	opcodeSize      = 1
	entryHeaderSize = keyLenSize + valueLenSize + opcodeSize
)

// Data entry field offsets
const (
	keyLenOffset   = 0
	valueLenOffset = keyLenOffset + keyLenSize
	opcodeOffset   = valueLenOffset + valueLenSize
	keyDataOffset  = opcodeOffset + opcodeSize
)

// Index entry field sizes
const (
	indexKeyLenSize      = 2
	indexOffsetSize      = 8
	indexEntryHeaderSize = indexKeyLenSize + indexOffsetSize
)

// Index entry field offsets
const (
	indexKeyLenOffset  = 0
	indexOffsetOffset  = indexKeyLenOffset + indexKeyLenSize
	indexKeyDataOffset = indexOffsetOffset + indexOffsetSize
)

// Footer field sizes
const (
	footerIndexOffsetSize           = 8
	footerBloomOffsetSize           = 8
	footerBloomNumHashesSize        = 1
	footerEntryCountSize            = 4
	footerMagicSize                 = 4
	footerSize                      = footerIndexOffsetSize + footerBloomOffsetSize + footerBloomNumHashesSize + footerEntryCountSize + footerMagicSize
	magicNumber              uint32 = 0x50454E47 // "PENG"
)

// Footer field offsets
const (
	footerIndexOffsetOffset    = 0
	footerBloomOffsetOffset    = footerIndexOffsetOffset + footerIndexOffsetSize
	footerBloomNumHashesOffset = footerBloomOffsetOffset + footerBloomOffsetSize
	footerEntryCountOffset     = footerBloomNumHashesOffset + footerBloomNumHashesSize
	footerMagicOffset          = footerEntryCountOffset + footerEntryCountSize
)

// Opcodes
const (
	OpcodePut    uint8 = 0x00
	OpcodeDelete uint8 = 0x01
)
