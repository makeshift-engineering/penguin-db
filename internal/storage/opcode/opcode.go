package opcode

// Opcode is the common uint8 type for all write operation codes across
// PenguinDB storage engine components (WAL, MemTable, SSTable, Storage Engine).
//
// All three on-disk formats agree on the same byte values, so no translation
// is required when crossing package boundaries.
type Opcode = uint8

// Canonical opcode values used by every on-disk format in this codebase.
const (
	// OpPut is the opcode for a live insert / update operation (0x00).
	OpPut Opcode = 0x00
	// OpDelete is the opcode for a tombstone deletion operation (0x01).
	OpDelete Opcode = 0x01

	// OpcodePut / OpcodeDelete are package-level aliases kept for
	// readability when used inside package-local constant blocks.
	OpcodePut    = OpPut
	OpcodeDelete = OpDelete
)

// IsDelete returns true when the opcode represents a tombstone deletion.
func IsDelete(opcode Opcode) bool { return opcode == OpcodeDelete }
