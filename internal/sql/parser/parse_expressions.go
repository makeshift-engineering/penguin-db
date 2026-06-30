package parser

import (
	"strconv"

	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

// parseExpression parses an additive expression (lowest precedence level).
// Expression = Term ( ( '+' | '-' ) Term )*
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
// Term = Factor ( ( '*' | '/' | '%' ) Factor )*
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
//	Factor = Literal
//	       | QualifiedIdentifier        (plain IDENT or table.col)
//	       | FunctionCall               (IDENT followed by '(')
//	       | '(' Expression ')'
//	       | ( '+' | '-' ) Factor
//
// The IDENT disambiguation is the only place where peek is needed:
//   - IDENT + peek '(' → FunctionCall
//   - IDENT + anything else → QualifiedIdentifier
func (p *Parser) parseFactor() (ast.Expression, error) {
	start := p.currentStart()

	switch p.current.Type {
	case utils.TOKEN_PLUS, utils.TOKEN_MINUS:
		op := p.current.Type
		p.advance()
		operand, err := p.parseFactor() // right-recursive: grammar says `( '+' | '-' ) Factor`
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

// parseLiteral parses any scalar literal: integer, float, string, boolean, null.
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
		// The lexer already stripped quotes and decoded escape sequences.
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

// parseNumericLiteral is a restricted variant of parseLiteral that only
// accepts TOKEN_INTEGER or TOKEN_FLOAT. Used after a unary sign in
// parseSignedLiteral, where e.g. DEFAULT -'hello' is invalid.
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
func (p *Parser) parseFunctionCall() (*ast.FunctionCall, error) {
	start := p.currentStart()

	nameTok := p.current
	p.advance() // consume function name (IDENT)
	p.advance() // consume '('

	fc := &ast.FunctionCall{Name: nameTok.Literal}

	switch {
	// Zero-argument call: func()
	case p.check(utils.TOKEN_RPAREN):
		p.advance()

	// Star form: COUNT(*)
	case p.check(utils.TOKEN_STAR):
		p.advance()
		fc.Star = true
		if _, err := p.expect(utils.TOKEN_RPAREN); err != nil {
			return nil, err
		}

	// Argument list, optionally preceded by DISTINCT
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

// parseIntegerLiteralValue expects and consumes a TOKEN_INTEGER, converts its
// literal to int, and returns the value. Used for LIMIT/OFFSET counts and
// VARCHAR lengths — places where the grammar requires a bare integer, not a
// full arithmetic expression.
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

// parseExpressionFromFactor continues parsing an Expression given that the
// Factor-level primary has already been assembled as `factor`. Re-enters at the
// Term level (multiplicative) and then the Expression level (additive).
func (p *Parser) parseExpressionFromFactor(start diagnostic.Pos, factor ast.Expression) (ast.Expression, error) {
	// Term tail: * / %
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

	// Expression tail: + -
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
