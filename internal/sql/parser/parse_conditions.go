package parser

import (
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

// parseCondition is the stable public entry point for all condition contexts
// (WHERE, HAVING, JOIN ON, parenthesised conditions).
func (p *Parser) parseCondition() (ast.Condition, error) {
	return p.parseOrCondition()
}

// parseOrCondition parses the lowest-precedence boolean level.
// OrCondition = AndCondition ( 'OR' AndCondition )*
func (p *Parser) parseOrCondition() (ast.Condition, error) {
	start := p.currentStart()

	left, err := p.parseAndCondition()
	if err != nil {
		return nil, err
	}

	for p.check(utils.TOKEN_OR) {
		p.advance()
		right, err := p.parseAndCondition()
		if err != nil {
			return nil, err
		}
		left = &ast.BinaryCondition{
			CondBase: p.condBase(start),
			Left:     left,
			Op:       utils.TOKEN_OR,
			Right:    right,
		}
	}
	return left, nil
}

// parseAndCondition parses the AND level.
// AndCondition = NotCondition ( 'AND' NotCondition )*
func (p *Parser) parseAndCondition() (ast.Condition, error) {
	start := p.currentStart()

	left, err := p.parseNotCondition()
	if err != nil {
		return nil, err
	}

	for p.check(utils.TOKEN_AND) {
		p.advance()
		right, err := p.parseNotCondition()
		if err != nil {
			return nil, err
		}
		left = &ast.BinaryCondition{
			CondBase: p.condBase(start),
			Left:     left,
			Op:       utils.TOKEN_AND,
			Right:    right,
		}
	}
	return left, nil
}

// parseNotCondition handles the NOT prefix (right-recursive).
// NotCondition = ConditionPrimary | 'NOT' NotCondition
func (p *Parser) parseNotCondition() (ast.Condition, error) {
	start := p.currentStart()

	if p.check(utils.TOKEN_NOT) {
		p.advance()
		inner, err := p.parseNotCondition()
		if err != nil {
			return nil, err
		}
		return &ast.NotCondition{CondBase: p.condBase(start), Operand: inner}, nil
	}

	return p.parseConditionPrimary()
}

// parseConditionPrimary is the key disambiguation function.
func (p *Parser) parseConditionPrimary() (ast.Condition, error) {
	start := p.currentStart()

	if p.check(utils.TOKEN_LPAREN) {
		p.advance() // consume '('

		// Always try an arithmetic expression first.
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}

		// Case A: closing ')' immediately — the inner content is an Expression.
		if p.check(utils.TOKEN_RPAREN) {
			p.advance() // consume ')'
			parenExpr := &ast.ParenExpr{ExprBase: p.exprBase(start), Inner: expr}

			if p.isPredicateTailStart() {
				// ( expr ) compOp expr — left side of a predicate
				return p.parsePredicateTail(start, parenExpr)
			}
			// Bare (expr) used as a condition truth value
			return &ast.ExprCondition{CondBase: p.condBase(start), Expr: parenExpr}, nil
		}

		// Case B: something else inside the parens — must be a condition.
		// e.g. (a > 5), (a > 5 AND b < 10)
		innerCond, err := p.parseConditionContinuationFromExpr(start, expr)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(utils.TOKEN_RPAREN); err != nil {
			return nil, err
		}
		return &ast.ParenCondition{CondBase: p.condBase(start), Inner: innerCond}, nil
	}

	// Non-paren: parse expression, check for predicate tail.
	expr, err := p.parseExpression()
	if err != nil {
		return nil, err
	}

	if p.isPredicateTailStart() {
		return p.parsePredicateTail(start, expr)
	}

	// Bare expression used as a condition (e.g. boolean column ref in WHERE).
	return &ast.ExprCondition{CondBase: p.condBase(start), Expr: expr}, nil
}

// parseConditionContinuationFromExpr is called when we have parsed `left` as
// an Expression inside '(…)' but ')' is NOT the next token. The remaining
// tokens inside the parens must form the continuation of a Condition.
func (p *Parser) parseConditionContinuationFromExpr(start diagnostic.Pos, left ast.Expression) (ast.Condition, error) {
	if p.isPredicateTailStart() {
		pred, err := p.parsePredicateTail(start, left)
		if err != nil {
			return nil, err
		}
		// Continue with any AND/OR chain still inside the parens.
		return p.parseOrConditionTailFromLeft(start, pred)
	}

	if p.check(utils.TOKEN_AND) || p.check(utils.TOKEN_OR) {
		leftCond := &ast.ExprCondition{CondBase: p.condBase(start), Expr: left}
		return p.parseOrConditionTailFromLeft(start, leftCond)
	}

	return nil, p.errorf(
		p.current.Span,
		CodeExpectedCondition,
		"expected a comparison operator, IN, BETWEEN, LIKE, IS, AND, or OR inside '('; got %s (%q)",
		p.current.Type, p.current.Literal,
	)
}

// isPredicateTailStart reports whether the current token can begin a predicate
// tail (i.e. the operator that turns an expression into a condition).
func (p *Parser) isPredicateTailStart() bool {
	switch p.current.Type {
	case utils.TOKEN_EQ, utils.TOKEN_NEQ,
		utils.TOKEN_LT, utils.TOKEN_GT, utils.TOKEN_LTE, utils.TOKEN_GTE,
		utils.TOKEN_LIKE, utils.TOKEN_IS, utils.TOKEN_IN, utils.TOKEN_BETWEEN:
		return true
	case utils.TOKEN_NOT:
		switch p.tokens.Peek().Type {
		case utils.TOKEN_LIKE, utils.TOKEN_IN, utils.TOKEN_BETWEEN:
			return true
		}
	}
	return false
}

// parsePredicateTail takes the already-parsed `left` Expression and constructs
// the appropriate Condition node based on the predicate operator that follows.
func (p *Parser) parsePredicateTail(start diagnostic.Pos, left ast.Expression) (ast.Condition, error) {
	switch p.current.Type {
	case utils.TOKEN_EQ, utils.TOKEN_NEQ,
		utils.TOKEN_LT, utils.TOKEN_GT, utils.TOKEN_LTE, utils.TOKEN_GTE:
		op := p.current.Type
		p.advance()
		right, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		return &ast.ComparisonPredicate{
			CondBase: p.condBase(start),
			Left:     left,
			Op:       op,
			Right:    right,
		}, nil

	case utils.TOKEN_LIKE:
		p.advance()
		pattern, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		return &ast.LikePredicate{
			CondBase: p.condBase(start),
			Left:     left,
			Pattern:  pattern,
			Negated:  false,
		}, nil

	case utils.TOKEN_IS:
		p.advance()
		negated := p.match(utils.TOKEN_NOT)
		if _, err := p.expect(utils.TOKEN_NULL); err != nil {
			return nil, err
		}
		return &ast.IsNullPredicate{
			CondBase: p.condBase(start),
			Expr:     left,
			Negated:  negated,
		}, nil

	case utils.TOKEN_IN:
		p.advance()
		vals, err := p.parseParenExpressionList()
		if err != nil {
			return nil, err
		}
		return &ast.InPredicate{
			CondBase: p.condBase(start),
			Expr:     left,
			Values:   vals,
			Negated:  false,
		}, nil

	case utils.TOKEN_BETWEEN:
		p.advance()
		lo, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(utils.TOKEN_AND); err != nil {
			return nil, err
		}
		hi, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		return &ast.BetweenPredicate{
			CondBase: p.condBase(start),
			Expr:     left,
			Low:      lo,
			High:     hi,
			Negated:  false,
		}, nil

	case utils.TOKEN_NOT:
		p.advance() // consume NOT
		switch p.current.Type {
		case utils.TOKEN_LIKE:
			p.advance()
			pattern, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			return &ast.LikePredicate{
				CondBase: p.condBase(start),
				Left:     left,
				Pattern:  pattern,
				Negated:  true,
			}, nil

		case utils.TOKEN_IN:
			p.advance()
			vals, err := p.parseParenExpressionList()
			if err != nil {
				return nil, err
			}
			return &ast.InPredicate{
				CondBase: p.condBase(start),
				Expr:     left,
				Values:   vals,
				Negated:  true,
			}, nil

		case utils.TOKEN_BETWEEN:
			p.advance()
			lo, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(utils.TOKEN_AND); err != nil {
				return nil, err
			}
			hi, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			return &ast.BetweenPredicate{
				CondBase: p.condBase(start),
				Expr:     left,
				Low:      lo,
				High:     hi,
				Negated:  true,
			}, nil

		default:
			return nil, p.errorf(
				p.current.Span,
				CodeExpectedCondition,
				"expected LIKE, IN, or BETWEEN after NOT; got %s (%q)",
				p.current.Type, p.current.Literal,
			)
		}

	default:
		// Unreachable if isPredicateTailStart() was checked before calling.
		return nil, p.errorf(
			p.current.Span,
			CodeExpectedCondition,
			"expected predicate operator, got %s (%q)",
			p.current.Type, p.current.Literal,
		)
	}
}

// parseOrConditionTailFromLeft continues an OR chain starting from `left`.
// AND binds tighter than OR, so each right-side operand first collects any
// pending AND terms before the OR node is built.
func (p *Parser) parseOrConditionTailFromLeft(start diagnostic.Pos, left ast.Condition) (ast.Condition, error) {
	left, err := p.parseAndConditionTailFromLeft(start, left)
	if err != nil {
		return nil, err
	}

	for p.check(utils.TOKEN_OR) {
		p.advance()
		right, err := p.parseAndCondition()
		if err != nil {
			return nil, err
		}
		left = &ast.BinaryCondition{
			CondBase: p.condBase(start),
			Left:     left,
			Op:       utils.TOKEN_OR,
			Right:    right,
		}
	}
	return left, nil
}

// parseAndConditionTailFromLeft continues an AND chain starting from `left`.
func (p *Parser) parseAndConditionTailFromLeft(start diagnostic.Pos, left ast.Condition) (ast.Condition, error) {
	for p.check(utils.TOKEN_AND) {
		p.advance()
		right, err := p.parseNotCondition()
		if err != nil {
			return nil, err
		}
		left = &ast.BinaryCondition{
			CondBase: p.condBase(start),
			Left:     left,
			Op:       utils.TOKEN_AND,
			Right:    right,
		}
	}
	return left, nil
}

// parseParenExpressionList parses '(' Expression ( ',' Expression )* ')'.
// Used by IN predicates.
func (p *Parser) parseParenExpressionList() ([]ast.Expression, error) {
	if _, err := p.expect(utils.TOKEN_LPAREN); err != nil {
		return nil, err
	}

	first, err := p.parseExpression()
	if err != nil {
		return nil, err
	}
	vals := []ast.Expression{first}

	for p.match(utils.TOKEN_COMMA) {
		v, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		vals = append(vals, v)
	}

	if _, err := p.expect(utils.TOKEN_RPAREN); err != nil {
		return nil, err
	}
	return vals, nil
}
