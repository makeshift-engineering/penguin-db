package planner

import (
	"testing"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
)

// typedExpr returns a ResolvedExpr with the given type. For TypeNull it
// returns a *ResolvedNullLiteral so isNullExpr recognises it.
func typedExpr(t ast.DataTypeKind) ResolvedExpr {
	if t == ast.TypeNull {
		return &ResolvedNullLiteral{}
	}
	return &ResolvedIntLiteral{ResolvedExprBase: newExprBase(t)}
}

func TestIsNumericType(t *testing.T) {
	tests := []struct {
		name string
		typ  ast.DataTypeKind
		want bool
	}{
		{"INT", ast.TypeInt, true},
		{"BIGINT", ast.TypeBigInt, true},
		{"FLOAT", ast.TypeFloat, true},
		{"DOUBLE", ast.TypeDouble, true},
		{"DECIMAL", ast.TypeDecimal, true},
		{"VARCHAR", ast.TypeVarchar, false},
		{"TEXT", ast.TypeText, false},
		{"BOOLEAN", ast.TypeBoolean, false},
		{"TIMESTAMP", ast.TypeTimestamp, false},
		{"NULL", ast.TypeNull, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isNumericType(tt.typ); got != tt.want {
				t.Errorf("isNumericType(%v) = %v, want %v", tt.typ, got, tt.want)
			}
		})
	}
}

func TestIsStringType(t *testing.T) {
	tests := []struct {
		name string
		typ  ast.DataTypeKind
		want bool
	}{
		{"VARCHAR", ast.TypeVarchar, true},
		{"TEXT", ast.TypeText, true},
		{"INT", ast.TypeInt, false},
		{"BOOLEAN", ast.TypeBoolean, false},
		{"TIMESTAMP", ast.TypeTimestamp, false},
		{"NULL", ast.TypeNull, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isStringType(tt.typ); got != tt.want {
				t.Errorf("isStringType(%v) = %v, want %v", tt.typ, got, tt.want)
			}
		})
	}
}

func TestIsNullExpr(t *testing.T) {
	if !isNullExpr(&ResolvedNullLiteral{}) {
		t.Error("expected isNullExpr to return true for *ResolvedNullLiteral")
	}
	if isNullExpr(&ResolvedIntLiteral{ResolvedExprBase: newExprBase(ast.TypeInt)}) {
		t.Error("expected isNullExpr to return false for *ResolvedIntLiteral")
	}
}

func TestTypeName(t *testing.T) {
	tests := []struct {
		typ  ast.DataTypeKind
		want string
	}{
		{ast.TypeInt, "INT"},
		{ast.TypeBigInt, "BIGINT"},
		{ast.TypeVarchar, "VARCHAR"},
		{ast.TypeBoolean, "BOOLEAN"},
		{ast.TypeText, "TEXT"},
		{ast.TypeTimestamp, "TIMESTAMP"},
		{ast.TypeFloat, "FLOAT"},
		{ast.TypeDouble, "DOUBLE"},
		{ast.TypeDecimal, "DECIMAL"},
		{ast.TypeNull, "NULL"},
		{ast.DataTypeKind(9999), "UNKNOWN"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := typeName(tt.typ); got != tt.want {
				t.Errorf("typeName(%v) = %q, want %q", tt.typ, got, tt.want)
			}
		})
	}
}

func TestExprTypeName(t *testing.T) {
	if got := exprTypeName(&ResolvedNullLiteral{}); got != "NULL" {
		t.Errorf("exprTypeName(NULL literal) = %q, want %q", got, "NULL")
	}
	if got := exprTypeName(typedExpr(ast.TypeInt)); got != "INT" {
		t.Errorf("exprTypeName(INT expr) = %q, want %q", got, "INT")
	}
}

func TestTypesCompatible(t *testing.T) {
	tests := []struct {
		name string
		a, b ast.DataTypeKind
		want bool
	}{
		// numeric × numeric
		{"INT_INT", ast.TypeInt, ast.TypeInt, true},
		{"INT_BIGINT", ast.TypeInt, ast.TypeBigInt, true},
		{"FLOAT_DECIMAL", ast.TypeFloat, ast.TypeDecimal, true},
		{"DOUBLE_INT", ast.TypeDouble, ast.TypeInt, true},
		// string × string
		{"VARCHAR_TEXT", ast.TypeVarchar, ast.TypeText, true},
		{"TEXT_VARCHAR", ast.TypeText, ast.TypeVarchar, true},
		// same type
		{"BOOLEAN_BOOLEAN", ast.TypeBoolean, ast.TypeBoolean, true},
		{"TIMESTAMP_TIMESTAMP", ast.TypeTimestamp, ast.TypeTimestamp, true},
		// cross-family incompatible
		{"INT_VARCHAR", ast.TypeInt, ast.TypeVarchar, false},
		{"TEXT_BOOLEAN", ast.TypeText, ast.TypeBoolean, false},
		{"BOOLEAN_TIMESTAMP", ast.TypeBoolean, ast.TypeTimestamp, false},
		{"TIMESTAMP_INT", ast.TypeTimestamp, ast.TypeInt, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := typesCompatible(tt.a, tt.b); got != tt.want {
				t.Errorf("typesCompatible(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestIsOrderableType(t *testing.T) {
	tests := []struct {
		name string
		typ  ast.DataTypeKind
		want bool
	}{
		{"INT", ast.TypeInt, true},
		{"BIGINT", ast.TypeBigInt, true},
		{"FLOAT", ast.TypeFloat, true},
		{"DOUBLE", ast.TypeDouble, true},
		{"DECIMAL", ast.TypeDecimal, true},
		{"VARCHAR", ast.TypeVarchar, true},
		{"TEXT", ast.TypeText, true},
		{"TIMESTAMP", ast.TypeTimestamp, true},
		{"BOOLEAN", ast.TypeBoolean, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isOrderableType(tt.typ); got != tt.want {
				t.Errorf("isOrderableType(%v) = %v, want %v", tt.typ, got, tt.want)
			}
		})
	}
}

func TestTypesOrderable(t *testing.T) {
	tests := []struct {
		name string
		a, b ast.DataTypeKind
		want bool
	}{
		// compatible + orderable
		{"INT_BIGINT", ast.TypeInt, ast.TypeBigInt, true},
		{"VARCHAR_TEXT", ast.TypeVarchar, ast.TypeText, true},
		{"TIMESTAMP_TIMESTAMP", ast.TypeTimestamp, ast.TypeTimestamp, true},
		// BOOLEAN excluded
		{"BOOLEAN_BOOLEAN", ast.TypeBoolean, ast.TypeBoolean, false},
		// compatible but one side boolean (not both compatible-typed anyway)
		{"INT_BOOLEAN", ast.TypeInt, ast.TypeBoolean, false},
		// incompatible
		{"INT_VARCHAR", ast.TypeInt, ast.TypeVarchar, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := typesOrderable(tt.a, tt.b); got != tt.want {
				t.Errorf("typesOrderable(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestExprsCompatible(t *testing.T) {
	// NULL is compatible with anything.
	if !exprsCompatible(typedExpr(ast.TypeNull), typedExpr(ast.TypeInt)) {
		t.Error("NULL should be compatible with INT")
	}
	if !exprsCompatible(typedExpr(ast.TypeVarchar), typedExpr(ast.TypeNull)) {
		t.Error("VARCHAR should be compatible with NULL")
	}
	if !exprsCompatible(typedExpr(ast.TypeNull), typedExpr(ast.TypeNull)) {
		t.Error("NULL should be compatible with NULL")
	}
	// Same family.
	if !exprsCompatible(typedExpr(ast.TypeInt), typedExpr(ast.TypeDouble)) {
		t.Error("INT should be compatible with DOUBLE")
	}
	// Cross family.
	if exprsCompatible(typedExpr(ast.TypeInt), typedExpr(ast.TypeVarchar)) {
		t.Error("INT should not be compatible with VARCHAR")
	}
}

func TestArithmeticResultType(t *testing.T) {
	tests := []struct {
		name     string
		left     ResolvedExpr
		right    ResolvedExpr
		wantType ast.DataTypeKind
		wantOK   bool
	}{
		// wider numeric wins
		{"INT_INT", typedExpr(ast.TypeInt), typedExpr(ast.TypeInt), ast.TypeInt, true},
		{"INT_BIGINT", typedExpr(ast.TypeInt), typedExpr(ast.TypeBigInt), ast.TypeBigInt, true},
		{"BIGINT_INT", typedExpr(ast.TypeBigInt), typedExpr(ast.TypeInt), ast.TypeBigInt, true},
		{"INT_FLOAT", typedExpr(ast.TypeInt), typedExpr(ast.TypeFloat), ast.TypeFloat, true},
		{"FLOAT_DOUBLE", typedExpr(ast.TypeFloat), typedExpr(ast.TypeDouble), ast.TypeDouble, true},
		{"INT_DECIMAL", typedExpr(ast.TypeInt), typedExpr(ast.TypeDecimal), ast.TypeDecimal, true},
		{"DOUBLE_DECIMAL", typedExpr(ast.TypeDouble), typedExpr(ast.TypeDecimal), ast.TypeDecimal, true},
		// NULL handling
		{"NULL_INT", typedExpr(ast.TypeNull), typedExpr(ast.TypeInt), ast.TypeInt, true},
		{"FLOAT_NULL", typedExpr(ast.TypeFloat), typedExpr(ast.TypeNull), ast.TypeFloat, true},
		{"NULL_NULL", typedExpr(ast.TypeNull), typedExpr(ast.TypeNull), ast.TypeInt, true},
		// non-numeric rejection
		{"VARCHAR_INT", typedExpr(ast.TypeVarchar), typedExpr(ast.TypeInt), 0, false},
		{"INT_BOOLEAN", typedExpr(ast.TypeInt), typedExpr(ast.TypeBoolean), 0, false},
		{"TEXT_TEXT", typedExpr(ast.TypeText), typedExpr(ast.TypeText), 0, false},
		// NULL + non-numeric
		{"NULL_VARCHAR", typedExpr(ast.TypeNull), typedExpr(ast.TypeVarchar), ast.TypeVarchar, false},
		{"BOOLEAN_NULL", typedExpr(ast.TypeBoolean), typedExpr(ast.TypeNull), ast.TypeBoolean, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := arithmeticResultType(tt.left, tt.right)
			if ok != tt.wantOK {
				t.Fatalf("arithmeticResultType ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.wantType {
				t.Errorf("arithmeticResultType type = %v, want %v", got, tt.wantType)
			}
		})
	}
}

func TestUnaryResultType(t *testing.T) {
	tests := []struct {
		name     string
		operand  ResolvedExpr
		wantType ast.DataTypeKind
		wantOK   bool
	}{
		{"INT", typedExpr(ast.TypeInt), ast.TypeInt, true},
		{"BIGINT", typedExpr(ast.TypeBigInt), ast.TypeBigInt, true},
		{"FLOAT", typedExpr(ast.TypeFloat), ast.TypeFloat, true},
		{"DOUBLE", typedExpr(ast.TypeDouble), ast.TypeDouble, true},
		{"DECIMAL", typedExpr(ast.TypeDecimal), ast.TypeDecimal, true},
		{"NULL", typedExpr(ast.TypeNull), ast.TypeInt, true},
		{"VARCHAR", typedExpr(ast.TypeVarchar), ast.TypeVarchar, false},
		{"BOOLEAN", typedExpr(ast.TypeBoolean), ast.TypeBoolean, false},
		{"TIMESTAMP", typedExpr(ast.TypeTimestamp), ast.TypeTimestamp, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := unaryResultType(tt.operand)
			if ok != tt.wantOK {
				t.Fatalf("unaryResultType ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.wantType {
				t.Errorf("unaryResultType type = %v, want %v", got, tt.wantType)
			}
		})
	}
}

func TestAggregateResultType(t *testing.T) {
	tests := []struct {
		name     string
		fn       string
		argType  ast.DataTypeKind
		wantType ast.DataTypeKind
		wantOK   bool
	}{
		// COUNT always BIGINT, regardless of argument type
		{"COUNT_INT", "COUNT", ast.TypeInt, ast.TypeBigInt, true},
		{"COUNT_VARCHAR", "COUNT", ast.TypeVarchar, ast.TypeBigInt, true},
		{"COUNT_BOOLEAN", "COUNT", ast.TypeBoolean, ast.TypeBigInt, true},

		// SUM: INT/BIGINT → BIGINT, floating/decimal pass through
		{"SUM_INT", "SUM", ast.TypeInt, ast.TypeBigInt, true},
		{"SUM_BIGINT", "SUM", ast.TypeBigInt, ast.TypeBigInt, true},
		{"SUM_FLOAT", "SUM", ast.TypeFloat, ast.TypeFloat, true},
		{"SUM_DOUBLE", "SUM", ast.TypeDouble, ast.TypeDouble, true},
		{"SUM_DECIMAL", "SUM", ast.TypeDecimal, ast.TypeDecimal, true},
		{"SUM_VARCHAR", "SUM", ast.TypeVarchar, 0, false},
		{"SUM_BOOLEAN", "SUM", ast.TypeBoolean, 0, false},

		// AVG: DECIMAL → DECIMAL, everything else numeric → DOUBLE
		{"AVG_INT", "AVG", ast.TypeInt, ast.TypeDouble, true},
		{"AVG_BIGINT", "AVG", ast.TypeBigInt, ast.TypeDouble, true},
		{"AVG_FLOAT", "AVG", ast.TypeFloat, ast.TypeDouble, true},
		{"AVG_DOUBLE", "AVG", ast.TypeDouble, ast.TypeDouble, true},
		{"AVG_DECIMAL", "AVG", ast.TypeDecimal, ast.TypeDecimal, true},
		{"AVG_TEXT", "AVG", ast.TypeText, 0, false},
		{"AVG_TIMESTAMP", "AVG", ast.TypeTimestamp, 0, false},

		// MIN/MAX: orderable type passes through unchanged
		{"MIN_INT", "MIN", ast.TypeInt, ast.TypeInt, true},
		{"MIN_VARCHAR", "MIN", ast.TypeVarchar, ast.TypeVarchar, true},
		{"MIN_TIMESTAMP", "MIN", ast.TypeTimestamp, ast.TypeTimestamp, true},
		{"MAX_DOUBLE", "MAX", ast.TypeDouble, ast.TypeDouble, true},
		{"MAX_TEXT", "MAX", ast.TypeText, ast.TypeText, true},
		// BOOLEAN is not orderable
		{"MIN_BOOLEAN", "MIN", ast.TypeBoolean, 0, false},
		{"MAX_BOOLEAN", "MAX", ast.TypeBoolean, 0, false},

		// unknown function
		{"UNKNOWN_FN", "FOOBAR", ast.TypeInt, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := aggregateResultType(tt.fn, tt.argType)
			if ok != tt.wantOK {
				t.Fatalf("aggregateResultType(%q, %v) ok = %v, want %v", tt.fn, tt.argType, ok, tt.wantOK)
			}
			if ok && got != tt.wantType {
				t.Errorf("aggregateResultType(%q, %v) type = %v, want %v", tt.fn, tt.argType, got, tt.wantType)
			}
		})
	}
}
