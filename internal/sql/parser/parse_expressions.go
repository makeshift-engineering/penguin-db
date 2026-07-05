// Package parser implements a single-pass recursive descent SQL parser.
// This file covers rules for AST Expression nodes, including:
// - Arithmetic operations (+, -, *, /, %)
// - Unary signs (+, -)
// - Literals (numbers, strings, booleans, NULL)
// - Identifiers (simple or dot-qualified column references)
// - Function calls (e.g. COUNT(*), AVG(price))
// - SelectExpression (disambiguation wrapper for expressions/conditions)
package parser

import (
	"strconv"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

// parseExpression parses an additive expression (lowest precedence level).
//
//	Expression = Term ( ( '+' | '-' ) Term )*
func (p *Parser) parseExpression() (ast.Expression, error) {
	start := p.currentStart()

	left, err := p.parseTerm()
	if err != nil {
		return nil, err
	}

	for p.check(utils.TOKEN_PLUS) || p.check(utils.TOKEN_MINUS) {
		op := p.current.Type
		p.advance()

		right, err := p.parseTerm()
		if err != nil {
			return nil, err
		}

		left = &ast.BinaryExpr{
			ExprBase: p.exprBase(start),
			Left:     left,
			Op:       op,
			Right:    right,
		}
	}

	return left, nil
}

// parseTerm parses a multiplicative expression.
//
//	Term = Factor ( ( '*' | '/' | '%' ) Factor )*
func (p *Parser) parseTerm() (ast.Expression, error) {
	start := p.currentStart()

	left, err := p.parseFactor()
	if err != nil {
		return nil, err
	}

	for p.check(utils.TOKEN_STAR) || p.check(utils.TOKEN_SLASH) || p.check(utils.TOKEN_PERCENT) {
		op := p.current.Type
		p.advance()

		right, err := p.parseFactor()
		if err != nil {
			return nil, err
		}

		left = &ast.BinaryExpr{
			ExprBase: p.exprBase(start),
			Left:     left,
			Op:       op,
			Right:    right,
		}
	}

	return left, nil
}

// parseFactor parses the primary level of an expression.
//
//	Factor = Literal | QualifiedIdentifier | FunctionCall | '(' Expression ')' | ( '+' | '-' ) Factor
func (p *Parser) parseFactor() (ast.Expression, error) {
	start := p.currentStart()

	switch p.current.Type {
	case utils.TOKEN_PLUS, utils.TOKEN_MINUS:
		op := p.current.Type
		p.advance()
		operand, err := p.parseFactor() // right-recursive
		if err != nil {
			return nil, err
		}
		return &ast.UnaryExpr{
			ExprBase: p.exprBase(start),
			Op:       op,
			Operand:  operand,
		}, nil

	case utils.TOKEN_LPAREN:
		p.advance() // consume '('
		inner, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(utils.TOKEN_RPAREN); err != nil {
			return nil, err
		}
		return &ast.ParenExpr{ExprBase: p.exprBase(start), Inner: inner}, nil

	case utils.TOKEN_IDENT:
		if p.peekIs(utils.TOKEN_LPAREN) {
			return p.parseFunctionCall()
		}
		return p.parseQualifiedIdentifier()

	default:
		return p.parseLiteral()
	}
}

// parseLiteral parses any scalar literal.
//
//	Literal = IntegerLiteral | FloatLiteral | StringLiteral | BooleanLiteral | 'NULL'
func (p *Parser) parseLiteral() (ast.Expression, error) {
	start := p.currentStart()

	switch p.current.Type {
	case utils.TOKEN_INTEGER:
		lit := p.current.Literal
		p.advance()
		return &ast.IntegerLiteral{ExprBase: p.exprBase(start), Value: lit}, nil

	case utils.TOKEN_FLOAT:
		lit := p.current.Literal
		p.advance()
		return &ast.FloatLiteral{ExprBase: p.exprBase(start), Value: lit}, nil

	case utils.TOKEN_STRING:
		lit := p.current.Literal
		p.advance()
		return &ast.StringLiteral{ExprBase: p.exprBase(start), Value: lit}, nil

	case utils.TOKEN_TRUE, utils.TOKEN_FALSE:
		lit := p.current.Literal
		p.advance()
		return &ast.BooleanLiteral{ExprBase: p.exprBase(start), Value: lit}, nil

	case utils.TOKEN_NULL:
		p.advance()
		return &ast.NullLiteral{ExprBase: p.exprBase(start)}, nil

	default:
		return nil, p.errorf(
			p.current.Span,
			CodeExpectedExpression,
			"expected an expression (literal, identifier, or '('), got %s (%q)",
			p.current.Type, p.current.Literal,
		)
	}
}

// parseNumericLiteral is a restricted variant of parseLiteral that only accepts TOKEN_INTEGER or TOKEN_FLOAT.
//
//	NumericLiteral = IntegerLiteral | FloatLiteral
func (p *Parser) parseNumericLiteral() (ast.Expression, error) {
	start := p.currentStart()

	switch p.current.Type {
	case utils.TOKEN_INTEGER:
		lit := p.current.Literal
		p.advance()
		return &ast.IntegerLiteral{ExprBase: p.exprBase(start), Value: lit}, nil

	case utils.TOKEN_FLOAT:
		lit := p.current.Literal
		p.advance()
		return &ast.FloatLiteral{ExprBase: p.exprBase(start), Value: lit}, nil

	default:
		return nil, p.errorf(
			p.current.Span,
			CodeExpectedExpression,
			"expected a numeric literal after sign, got %s (%q)",
			p.current.Type, p.current.Literal,
		)
	}
}

// parseFunctionCall parses a SQL function invocation.
//
//	FunctionCall = Identifier '(' [ 'DISTINCT' ] '*' | [ Expression ( ',' Expression )* ] ')'
func (p *Parser) parseFunctionCall() (*ast.FunctionCall, error) {
	start := p.currentStart()

	nameTok, err := p.expect(utils.TOKEN_IDENT)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(utils.TOKEN_LPAREN); err != nil {
		return nil, err
	}

	fc := &ast.FunctionCall{Name: nameTok.Literal}

	switch {
	case p.check(utils.TOKEN_RPAREN):
		p.advance()

	case p.check(utils.TOKEN_STAR):
		p.advance()
		fc.Star = true
		if _, err := p.expect(utils.TOKEN_RPAREN); err != nil {
			return nil, err
		}

	default:
		if p.match(utils.TOKEN_DISTINCT) {
			fc.Distinct = true
		}

		arg, err := p.parseSelectExpression()
		if err != nil {
			return nil, err
		}
		fc.Args = append(fc.Args, arg)

		for p.match(utils.TOKEN_COMMA) {
			arg, err = p.parseSelectExpression()
			if err != nil {
				return nil, err
			}
			fc.Args = append(fc.Args, arg)
		}

		if _, err := p.expect(utils.TOKEN_RPAREN); err != nil {
			return nil, err
		}
	}

	fc.ExprBase = p.exprBase(start)
	return fc, nil
}

// parseIntegerLiteralValue expects and consumes a TOKEN_INTEGER, converts its literal to int.
//
//	IntegerLiteralValue = Integer
func (p *Parser) parseIntegerLiteralValue() (int, error) {
	tok, err := p.expect(utils.TOKEN_INTEGER)
	if err != nil {
		return 0, err
	}
	n, convErr := strconv.Atoi(tok.Literal)
	if convErr != nil {
		return 0, p.errorf(
			tok.Span,
			CodeInvalidIntegerLiteral,
			"cannot parse %q as integer: %v", tok.Literal, convErr,
		)
	}
	return n, nil
}

// parseExpressionFromFactor continues parsing an Expression given that the Factor-level primary has already been assembled.
//
//	ExpressionFromFactor = Factor (TermTail | ExprTail)
func (p *Parser) parseExpressionFromFactor(start diagnostic.Pos, factor ast.Expression) (ast.Expression, error) {
	left := factor
	for p.check(utils.TOKEN_STAR) || p.check(utils.TOKEN_SLASH) || p.check(utils.TOKEN_PERCENT) {
		op := p.current.Type
		p.advance()
		right, err := p.parseFactor()
		if err != nil {
			return nil, err
		}
		left = &ast.BinaryExpr{ExprBase: p.exprBase(start), Left: left, Op: op, Right: right}
	}

	for p.check(utils.TOKEN_PLUS) || p.check(utils.TOKEN_MINUS) {
		op := p.current.Type
		p.advance()
		right, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		left = &ast.BinaryExpr{ExprBase: p.exprBase(start), Left: left, Op: op, Right: right}
	}

	return left, nil
}

// parseQualifiedIdentifier parses a simple or dot-qualified name.
//
//	QualifiedIdentifier = Identifier [ '.' Identifier ]
func (p *Parser) parseQualifiedIdentifier() (*ast.Identifier, error) {
	start := p.currentStart()

	nameTok, err := p.expect(utils.TOKEN_IDENT)
	if err != nil {
		return nil, err
	}

	name := nameTok.Literal
	qualifier := ""

	if p.check(utils.TOKEN_DOT) && p.peekIs(utils.TOKEN_IDENT) {
		p.advance() // consume '.'
		qualTok, err := p.expect(utils.TOKEN_IDENT)
		if err != nil {
			return nil, err
		}
		qualifier = name
		name = qualTok.Literal
	}

	return &ast.Identifier{
		ExprBase:  p.exprBase(start),
		Name:      name,
		Qualifier: qualifier,
	}, nil
}

// parseSelectExpression parses one column-list or function-argument item that
// may be either an arithmetic Expression or a boolean Condition.
//
//	SelectExpression = [ 'NOT' ] OrCondition | Expression [ PredicateTail | OrConditionTail ]
func (p *Parser) parseSelectExpression() (*ast.SelectExpression, error) {
	start := p.currentStart()

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

	expr, err := p.parseExpression()
	if err != nil {
		return nil, err
	}

	return p.selectExpressionFromExpression(start, expr)
}

// selectExpressionFromExpression converts an already-parsed arithmetic Expression into a *ast.SelectExpression.
//
//	selectExpressionFromExpression = Expression [ PredicateTail | OrConditionTail ]
func (p *Parser) selectExpressionFromExpression(start diagnostic.Pos, expr ast.Expression) (*ast.SelectExpression, error) {
	if p.isPredicateTailStart() {
		cond, err := p.parsePredicateTail(start, expr)
		if err != nil {
			return nil, err
		}
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
