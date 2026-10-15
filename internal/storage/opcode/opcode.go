package opcode

import "fmt"

// Opcode is the distinct uint8 type for all write operation codes across
// PenguinDB on-disk storage engine formats (WAL, SSTable).
type Opcode uint8

// Canonical opcode values used by every on-disk format in this codebase.
const (
	// OpPut is the opcode for a live insert / update operation (0x00).
	OpPut Opcode = 0x00
	// OpDelete is the opcode for a tombstone deletion operation (0x01).
	OpDelete Opcode = 0x01
)

// IsValid reports whether the opcode is a recognized canonical operation.
func (op Opcode) IsValid() bool {
	return op == OpPut || op == OpDelete
}

// IsDeleted reports whether the opcode represents a tombstone deletion.
func (op Opcode) IsDeleted() bool {
	return op == OpDelete
}

// Byte returns the raw byte representation of the opcode.
func (op Opcode) Byte() byte {
	return byte(op)
}

// String returns a human-readable representation of the opcode.
func (op Opcode) String() string {
	switch op {
	case OpPut:
		return "PUT"
	case OpDelete:
		return "DELETE"
	default:
		return fmt.Sprintf("UNKNOWN(0x%02X)", uint8(op))
	}
}

// IsDeleted returns true when the opcode represents a tombstone deletion.
func IsDeleted(op Opcode) bool {
	return op.IsDeleted()
}
