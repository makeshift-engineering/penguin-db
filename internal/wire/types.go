package wire

import (
	"fmt"
	"time"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

// PostgreSQL Data Type Object Identifiers (OIDs) and metadata mappings.
const (
	OIDBool      int32 = 16   // Boolean data type
	OIDInt8      int32 = 20   // 8-byte signed integer (BIGINT)
	OIDInt4      int32 = 23   // 4-byte signed integer (INT)
	OIDText      int32 = 25   // Text string / Variable length character
	OIDFloat4    int32 = 700  // Single precision floating-point
	OIDFloat8    int32 = 701  // Double precision floating-point
	OIDTimestamp int32 = 1114 // Timestamp without timezone
	OIDNumeric   int32 = 1700 // Arbitrary precision numeric / decimal
)

// TypeInfo encapsulates PostgreSQL type OID and fixed byte width descriptor.
type TypeInfo struct {
	OID  int32
	Size int16
}

// MapASTTypeToOID maps internal AST data type kinds to standard PostgreSQL OIDs and sizes.
func MapASTTypeToOID(kind ast.DataTypeKind) TypeInfo {
	switch kind {
	case ast.TypeInt:
		return TypeInfo{OID: OIDInt4, Size: 4}
	case ast.TypeBigInt:
		return TypeInfo{OID: OIDInt8, Size: 8}
	case ast.TypeFloat:
		return TypeInfo{OID: OIDFloat4, Size: 4}
	case ast.TypeDouble:
		return TypeInfo{OID: OIDFloat8, Size: 8}
	case ast.TypeDecimal:
		return TypeInfo{OID: OIDNumeric, Size: -1}
	case ast.TypeVarchar, ast.TypeText:
		return TypeInfo{OID: OIDText, Size: -1}
	case ast.TypeBoolean:
		return TypeInfo{OID: OIDBool, Size: 1}
	case ast.TypeTimestamp:
		return TypeInfo{OID: OIDTimestamp, Size: 8}
	default:
		return TypeInfo{OID: OIDText, Size: -1}
	}
}

// FormatColumnValue converts an internal Go evaluation result into a text-format byte slice for wire DataRow transmission.
func FormatColumnValue(val any) ([]byte, bool) {
	if val == nil {
		return nil, true
	}

	switch v := val.(type) {
	case bool:
		if v {
			return []byte("t"), false
		}
		return []byte("f"), false
	case int:
		return []byte(fmt.Sprintf("%d", v)), false
	case int32:
		return []byte(fmt.Sprintf("%d", v)), false
	case int64:
		return []byte(fmt.Sprintf("%d", v)), false
	case float32:
		return []byte(fmt.Sprintf("%g", v)), false
	case float64:
		return []byte(fmt.Sprintf("%g", v)), false
	case string:
		return []byte(v), false
	case time.Time:
		return []byte(v.Format("2006-01-02 15:04:05.999999")), false
	case []byte:
		return v, false
	default:
		return []byte(fmt.Sprintf("%v", v)), false
	}
}
