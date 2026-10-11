package planner

import "github.com/makeshift-engineering/penguin-db/internal/sql/utils"

// tokenToOp translates a lexer token type to the planner's own Op enum.
// It is called at the AST→planner boundary — where resolution reads an
// operator from an AST node and stores it in a resolved plan node — and
// nowhere else.
func tokenToOp(t utils.TokenType) Op {
	switch t {
	case utils.TOKEN_EQ:
		return OpEq
	case utils.TOKEN_NEQ:
		return OpNeq
	case utils.TOKEN_LT:
		return OpLt
	case utils.TOKEN_GT:
		return OpGt
	case utils.TOKEN_LTE:
		return OpLte
	case utils.TOKEN_GTE:
		return OpGte
	case utils.TOKEN_PLUS:
		return OpAdd
	case utils.TOKEN_MINUS:
		return OpSub
	case utils.TOKEN_STAR:
		return OpMul
	case utils.TOKEN_SLASH:
		return OpDiv
	case utils.TOKEN_PERCENT:
		return OpMod
	case utils.TOKEN_AND:
		return OpAnd
	case utils.TOKEN_OR:
		return OpOr
	default:
		return -1
	}
}

// isOrderingOp reports whether op is a strict ordering operator that
// requires orderable operands, as opposed to an equality operator.
func isOrderingOp(op Op) bool {
	return op == OpLt || op == OpGt || op == OpLte || op == OpGte
}
