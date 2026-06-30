package parser

import (
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

// parseSelectStatement handles the full SELECT syntax:
//
//	SELECT [DISTINCT | ALL] SelectList
//	       [FROM TableReference (',' TableReference)*]
//	       [WHERE Condition]
//	       [GROUP BY QualifiedIdentifier (',' QualifiedIdentifier)*]
//	       [HAVING Condition]
//	       [ORDER BY OrderByItem (',' OrderByItem)*]
//	       [LIMIT Integer [OFFSET Integer]]
//
// Clause ordering is enforced by checking each keyword in the exact grammar
// order. An out-of-order clause (e.g. LIMIT before WHERE) produces an
// "expected ';'" error at the right position, pointing directly at the misplaced
// keyword.
func (p *Parser) parseSelectStatement() (*ast.SelectStmt, error) {
	start := p.currentStart()
	p.advance() // SELECT

	distinct, all := false, false
	switch {
	case p.match(utils.TOKEN_DISTINCT):
		distinct = true
	case p.match(utils.TOKEN_ALL):
		all = true
	}

	cols, err := p.parseSelectList()
	if err != nil {
		return nil, err
	}

	var from []*ast.TableRef
	if p.match(utils.TOKEN_FROM) {
		from, err = p.parseTableReferences()
		if err != nil {
			return nil, err
		}
	}

	var where *ast.WhereClause
	if p.check(utils.TOKEN_WHERE) {
		where, err = p.parseWhereClause()
		if err != nil {
			return nil, err
		}
	}

	var groupBy *ast.GroupByClause
	if p.check(utils.TOKEN_GROUP) {
		groupBy, err = p.parseGroupByClause()
		if err != nil {
			return nil, err
		}
	}

	var having *ast.HavingClause
	if p.check(utils.TOKEN_HAVING) {
		having, err = p.parseHavingClause()
		if err != nil {
			return nil, err
		}
	}

	var orderBy *ast.OrderByClause
	if p.check(utils.TOKEN_ORDER) {
		orderBy, err = p.parseOrderByClause()
		if err != nil {
			return nil, err
		}
	}

	var limit *ast.LimitClause
	if p.check(utils.TOKEN_LIMIT) {
		limit, err = p.parseLimitClause()
		if err != nil {
			return nil, err
		}
	}

	return &ast.SelectStmt{
		StmtBase: p.stmtBase(start),
		Distinct: distinct,
		All:      all,
		Columns:  cols,
		From:     from,
		Where:    where,
		GroupBy:  groupBy,
		Having:   having,
		OrderBy:  orderBy,
		Limit:    limit,
	}, nil
}

// parseSelectList parses a comma-separated list of SelectColumns.
func (p *Parser) parseSelectList() ([]*ast.SelectColumn, error) {
	col, err := p.parseSelectColumn()
	if err != nil {
		return nil, err
	}
	cols := []*ast.SelectColumn{col}

	for p.match(utils.TOKEN_COMMA) {
		col, err = p.parseSelectColumn()
		if err != nil {
			return nil, err
		}
		cols = append(cols, col)
	}
	return cols, nil
}

// parseSelectColumn parses one item from the SELECT column list.
func (p *Parser) parseSelectColumn() (*ast.SelectColumn, error) {
	start := p.currentStart()

	if p.check(utils.TOKEN_STAR) {
		p.advance()
		return &ast.SelectColumn{ClauseBase: p.clauseBase(start), Star: true}, nil
	}

	if p.check(utils.TOKEN_IDENT) && p.peekIs(utils.TOKEN_DOT) {
		// Save and consume the first identifier.
		firstTok := p.current
		p.advance() // consume IDENT
		// current == '.', peek == ???

		switch p.tokens.Peek().Type {
		case utils.TOKEN_STAR:
			// Pattern: IDENT '.' '*' : single-level qualified wildcard (table.*)
			p.advance() // consume '.'
			p.advance() // consume '*'
			ident := &ast.Identifier{
				ExprBase: p.exprBase(start),
				Name:     firstTok.Literal,
			}
			return &ast.SelectColumn{
				ClauseBase:    p.clauseBase(start),
				QualifiedStar: ident,
			}, nil

		case utils.TOKEN_IDENT:
			// Pattern: IDENT '.' IDENT — could be db.table.* or table.col[…]
			p.advance() // consume '.'
			secondTok := p.current
			p.advance() // consume second IDENT

			if p.check(utils.TOKEN_DOT) && p.peekIs(utils.TOKEN_STAR) {
				// Pattern: IDENT '.' IDENT '.' '*' :  two-level wildcard (db.table.*)
				p.advance() // consume '.'
				p.advance() // consume '*'
				ident := &ast.Identifier{
					ExprBase:  p.exprBase(start),
					Name:      secondTok.Literal,
					Qualifier: firstTok.Literal,
				}
				return &ast.SelectColumn{
					ClauseBase:    p.clauseBase(start),
					QualifiedStar: ident,
				}, nil
			}

			// Pattern: IDENT '.' IDENT [not followed by '.'  '*']
			qualIdent := &ast.Identifier{
				ExprBase:  p.exprBase(start),
				Name:      secondTok.Literal,
				Qualifier: firstTok.Literal,
			}
			return p.parseSelectColumnFromPrimary(start, qualIdent)

		default:
			// IDENT '.' <something unexpected>
			return nil, p.errorf(
				p.tokens.Peek().Span,
				CodeUnexpectedToken,
				"expected identifier or '*' after '.', got %s",
				p.tokens.Peek().Type,
			)
		}
	}

	selExpr, err := p.parseSelectExpression()
	if err != nil {
		return nil, err
	}

	alias, err := p.parseOptionalAlias()
	if err != nil {
		return nil, err
	}

	return &ast.SelectColumn{
		ClauseBase: p.clauseBase(start),
		Expr:       selExpr,
		Alias:      alias,
	}, nil
}

// parseSelectColumnFromPrimary finishes a SelectColumn when the caller has
// already consumed and reconstructed `primary` as the Factor-level expression.
// It re-enters at the Term/Expression tail levels, applies SelectExpression
// disambiguation, and collects the optional alias.
func (p *Parser) parseSelectColumnFromPrimary(start diagnostic.Pos, primary *ast.Identifier) (*ast.SelectColumn, error) {
	// Continue expression parsing from the already-consumed Factor.
	expr, err := p.parseExpressionFromFactor(start, primary)
	if err != nil {
		return nil, err
	}

	// Apply SelectExpression disambiguation (predicate tail, AND/OR, or plain expr).
	selExpr, err := p.selectExpressionFromExpression(start, expr)
	if err != nil {
		return nil, err
	}

	alias, err := p.parseOptionalAlias()
	if err != nil {
		return nil, err
	}

	return &ast.SelectColumn{
		ClauseBase: p.clauseBase(start),
		Expr:       selExpr,
		Alias:      alias,
	}, nil
}

// parseOptionalAlias consumes ['AS'] Identifier if present and returns the alias
// string. Returns "" if no alias follows.
func (p *Parser) parseOptionalAlias() (string, error) {
	if p.match(utils.TOKEN_AS) {
		return p.expectIdent()
	}
	// Implicit alias: a bare IDENT that is not a clause keyword.
	// Only TOKEN_IDENT qualifies — keywords like WHERE, JOIN, etc. are not aliases.
	if p.check(utils.TOKEN_IDENT) {
		alias := p.current.Literal
		p.advance()
		return alias, nil
	}
	return "", nil
}
