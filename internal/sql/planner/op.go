package planner

// Op is the planner's own operator vocabulary. It mirrors the subset of
// lexer tokens that survive into resolved plan trees (arithmetic,
// comparison, logical), but is intentionally separate so the planner
// depends only on its upstream stage (the parser/AST), not on the lexer's
// full token set.
type Op int

const (
	// Comparison operators.
	OpEq  Op = iota // =
	OpNeq           // !=, <>
	OpLt            // <
	OpGt            // >
	OpLte           // <=
	OpGte           // >=

	// Arithmetic operators.
	OpAdd // +
	OpSub // -
	OpMul // *
	OpDiv // /
	OpMod // %

	// Logical operators.
	OpAnd // AND
	OpOr  // OR
)

// String returns a short human-readable representation of the operator,
// suitable for diagnostic messages.
func (o Op) String() string {
	switch o {
	case OpEq:
		return "="
	case OpNeq:
		return "!="
	case OpLt:
		return "<"
	case OpGt:
		return ">"
	case OpLte:
		return "<="
	case OpGte:
		return ">="
	case OpAdd:
		return "+"
	case OpSub:
		return "-"
	case OpMul:
		return "*"
	case OpDiv:
		return "/"
	case OpMod:
		return "%"
	case OpAnd:
		return "AND"
	case OpOr:
		return "OR"
	default:
		return "?"
	}
}
