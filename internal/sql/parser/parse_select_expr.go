package parser

import (
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
	"github.com/makeshift-engineering/penguin-db/internal/sql/lexer"
)

// parseSelectExpression parses one column-list or function-argument item that
// may be either an arithmetic Expression or a boolean Condition.
func (p *Parser) parseSelectExpression() (*ast.SelectExpression, error) {
	start := p.currentStart()

	// NOT at the very start can only begin a Condition.
	// (A unary NOT on an expression would require '+'/'-'; NOT is not TOKEN_MINUS.)
	if p.check(lexer.TOKEN_NOT) {
		cond, err := p.parseOrCondition()
		if err != nil {
			return nil, err
		}
		return &ast.SelectExpression{
			NodeBase: p.nodeBase(start),
			Cond:     cond,
		}, nil
	}

	expr, err := p.parseExpression()
	if err != nil {
		return nil, err
	}

	return p.selectExpressionFromExpression(start, expr)
}

// selectExpressionFromExpression converts an already-parsed arithmetic
// Expression into a *ast.SelectExpression by inspecting the following token.
func (p *Parser) selectExpressionFromExpression(start diagnostic.Pos, expr ast.Expression) (*ast.SelectExpression, error) {
	// ── Predicate tail: expression becomes left side of a condition ──────────
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

	if p.check(lexer.TOKEN_AND) || p.check(lexer.TOKEN_OR) {
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
	case lexer.TOKEN_IDENT,
		lexer.TOKEN_INTEGER, lexer.TOKEN_FLOAT, lexer.TOKEN_STRING,
		lexer.TOKEN_TRUE, lexer.TOKEN_FALSE, lexer.TOKEN_NULL,
		lexer.TOKEN_LPAREN,
		lexer.TOKEN_PLUS, lexer.TOKEN_MINUS,
		lexer.TOKEN_NOT:
		return true
	}
	return false
}
