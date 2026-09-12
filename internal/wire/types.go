package wire

import (
	"fmt"
	"time"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

const (
	OIDBool      int32 = 16   // BOOL
	OIDInt8      int32 = 20   // INT8 / BIGINT
	OIDInt4      int32 = 23   // INT4 / INT
	OIDText      int32 = 25   // TEXT / VARCHAR
	OIDFloat4    int32 = 700  // FLOAT4 / FLOAT
	OIDFloat8    int32 = 701  // FLOAT8 / DOUBLE
	OIDTimestamp int32 = 1114 // TIMESTAMP
	OIDNumeric   int32 = 1700 // NUMERIC / DECIMAL
)

type TypeInfo struct {
	OID  int32
	Size int16
}

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
