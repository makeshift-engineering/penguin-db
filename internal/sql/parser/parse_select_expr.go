package parser

import (
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

// parseSelectExpression parses one column-list or function-argument item that
// may be either an arithmetic Expression or a boolean Condition.
func (p *Parser) parseSelectExpression() (*ast.SelectExpression, error) {
	start := p.currentStart()

	// NOT at the very start can only begin a Condition.
	// (A unary NOT on an expression would require '+'/'-'; NOT is not TOKEN_MINUS.)
	if p.check(utils.TOKEN_NOT) {
		cond, err := p.parseOrCondition()
		if err != nil {
			return nil, err
		}
		return &ast.SelectExpression{
			NodeBase: p.nodeBase(start),
			Cond:     cond,
		}, nil
	}

	// Step 1: parse an arithmetic expression.
	expr, err := p.parseExpression()
	if err != nil {
		return nil, err
	}

	// Step 2: apply the SelectExpression disambiguation.
	return p.selectExpressionFromExpression(start, expr)
}

// selectExpressionFromExpression converts an already-parsed arithmetic
// Expression into a *ast.SelectExpression by inspecting the following token.
//
// Called by parseSelectExpression (after the NOT guard) and by
// parseSelectColumnFromPrimary (when a qualified identifier has been consumed
// before this function is entered).
func (p *Parser) selectExpressionFromExpression(start diagnostic.Pos, expr ast.Expression) (*ast.SelectExpression, error) {
	if p.isPredicateTailStart() {
		cond, err := p.parsePredicateTail(start, expr)
		if err != nil {
			return nil, err
		}
		// Any AND/OR chain that follows the predicate is still part of this
		// SelectExpression (e.g. `price > 10 AND qty < 5` in a column list).
		finalCond, err := p.parseOrConditionTailFromLeft(start, cond)
		if err != nil {
			return nil, err
		}
		return &ast.SelectExpression{
			NodeBase: p.nodeBase(start),
			Cond:     finalCond,
		}, nil
	}

	if p.check(utils.TOKEN_AND) || p.check(utils.TOKEN_OR) {
		leftCond := &ast.ExprCondition{CondBase: p.condBase(start), Expr: expr}
		finalCond, err := p.parseOrConditionTailFromLeft(start, leftCond)
		if err != nil {
			return nil, err
		}
		return &ast.SelectExpression{
			NodeBase: p.nodeBase(start),
			Cond:     finalCond,
		}, nil
	}

	return &ast.SelectExpression{
		NodeBase: p.nodeBase(start),
		Expr:     expr,
	}, nil
}

// isSelectExprStart reports whether the current token can begin a SelectExpression.
// Used by parseValueRow and similar callers that need to know before committing.
func (p *Parser) isSelectExprStart() bool {
	switch p.current.Type {
	case utils.TOKEN_IDENT,
		utils.TOKEN_INTEGER, utils.TOKEN_FLOAT, utils.TOKEN_STRING,
		utils.TOKEN_TRUE, utils.TOKEN_FALSE, utils.TOKEN_NULL,
		utils.TOKEN_LPAREN,
		utils.TOKEN_PLUS, utils.TOKEN_MINUS,
		utils.TOKEN_NOT:
		return true
	}
	return false
}
