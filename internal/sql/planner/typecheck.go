package planner

import "github.com/makeshift-engineering/penguin-db/internal/sql/ast"

// numericRank orders numeric types from narrowest to widest, used to pick
// the promoted result type of an arithmetic expression. Types absent from
// this map are not numeric.
var numericRank = map[ast.DataTypeKind]int{
	ast.TypeInt:     0,
	ast.TypeBigInt:  1,
	ast.TypeFloat:   2,
	ast.TypeDouble:  3,
	ast.TypeDecimal: 4,
}

func isNumericType(t ast.DataTypeKind) bool {
	_, ok := numericRank[t]
	return ok
}

func isStringType(t ast.DataTypeKind) bool {
	return t == ast.TypeVarchar || t == ast.TypeText
}

// isNullExpr reports whether e is a resolved NULL literal. NULL is
// compatible with every type and every operator, so callers must check this
// before consulting ResolvedType.
func isNullExpr(e ResolvedExpr) bool {
	_, ok := e.(*ResolvedNullLiteral)
	return ok
}

// typeName returns a short human-readable name for a data type, used in
// diagnostic messages.
func typeName(t ast.DataTypeKind) string {
	switch t {
	case ast.TypeInt:
		return "INT"
	case ast.TypeBigInt:
		return "BIGINT"
	case ast.TypeVarchar:
		return "VARCHAR"
	case ast.TypeBoolean:
		return "BOOLEAN"
	case ast.TypeText:
		return "TEXT"
	case ast.TypeTimestamp:
		return "TIMESTAMP"
	case ast.TypeFloat:
		return "FLOAT"
	case ast.TypeDouble:
		return "DOUBLE"
	case ast.TypeDecimal:
		return "DECIMAL"
	default:
		return "UNKNOWN"
	}
}

// exprTypeName is typeName for a resolved expression, reporting "NULL"
// instead of the meaningless placeholder type NULL literals carry.
func exprTypeName(e ResolvedExpr) string {
	if isNullExpr(e) {
		return "NULL"
	}
	return typeName(e.ResolvedType())
}

// typesCompatible reports whether two data types may appear on either side
// of a comparison: numeric types are mutually compatible with each other,
// string types are mutually compatible with each other, and every other
// type (boolean, timestamp) is only compatible with itself.
func typesCompatible(a, b ast.DataTypeKind) bool {
	if isNumericType(a) && isNumericType(b) {
		return true
	}
	if isStringType(a) && isStringType(b) {
		return true
	}
	return a == b
}

// exprsCompatible is typesCompatible for two resolved expressions, treating
// NULL as compatible with anything.
func exprsCompatible(a, b ResolvedExpr) bool {
	if isNullExpr(a) || isNullExpr(b) {
		return true
	}
	return typesCompatible(a.ResolvedType(), b.ResolvedType())
}

// arithmeticResultType computes the promoted result type of a binary
// arithmetic expression: the wider of the two operand types by
// numericRank, so DECIMAL always wins. NULL takes on the other operand's
// type; if both operands are NULL the returned type is arbitrary (the
// value is NULL regardless). ok is false if either non-NULL operand is not
// numeric.
func arithmeticResultType(left, right ResolvedExpr) (result ast.DataTypeKind, ok bool) {
	leftNull, rightNull := isNullExpr(left), isNullExpr(right)
	switch {
	case leftNull && rightNull:
		return ast.TypeInt, true
	case leftNull:
		t := right.ResolvedType()
		return t, isNumericType(t)
	case rightNull:
		t := left.ResolvedType()
		return t, isNumericType(t)
	}

	lt, rt := left.ResolvedType(), right.ResolvedType()
	lRank, lOK := numericRank[lt]
	rRank, rOK := numericRank[rt]
	if !lOK || !rOK {
		return 0, false
	}
	if lRank >= rRank {
		return lt, true
	}
	return rt, true
}

// unaryResultType validates that a unary +/- operand is numeric (or NULL)
// and returns its type unchanged.
func unaryResultType(operand ResolvedExpr) (result ast.DataTypeKind, ok bool) {
	if isNullExpr(operand) {
		return ast.TypeInt, true
	}
	t := operand.ResolvedType()
	return t, isNumericType(t)
}

// aggregateFunctions is the set of function names the planner recognises.
var aggregateFunctions = map[string]bool{
	"COUNT": true,
	"SUM":   true,
	"AVG":   true,
	"MIN":   true,
	"MAX":   true,
}

// aggregateResultType computes an aggregate function's result type from its
// single argument's type. argType is ignored for COUNT (including
// COUNT(*), which never resolves an argument at all). ok is false if
// argType is unsupported for the given function — SUM and AVG require a
// numeric argument.
func aggregateResultType(name string, argType ast.DataTypeKind) (result ast.DataTypeKind, ok bool) {
	switch name {
	case "COUNT":
		return ast.TypeBigInt, true
	case "SUM":
		if !isNumericType(argType) {
			return 0, false
		}
		if numericRank[argType] >= numericRank[ast.TypeFloat] {
			return argType, true // FLOAT/DOUBLE/DECIMAL: preserve precision
		}
		return ast.TypeBigInt, true // INT/BIGINT: widen to avoid overflow
	case "AVG":
		if !isNumericType(argType) {
			return 0, false
		}
		if argType == ast.TypeDecimal {
			return ast.TypeDecimal, true
		}
		return ast.TypeDouble, true
	case "MIN", "MAX":
		return argType, true // any orderable type passes through unchanged
	default:
		return 0, false
	}
}
