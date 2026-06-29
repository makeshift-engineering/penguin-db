package parser

import (
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

// parseTableReferences parses a comma-separated list of TableReference nodes.
// Called after the FROM keyword has been consumed by parseSelectStatement.
func (p *Parser) parseTableReferences() ([]*ast.TableRef, error) {
	ref, err := p.parseTableReference()
	if err != nil {
		return nil, err
	}
	refs := []*ast.TableRef{ref}

	for p.match(utils.TOKEN_COMMA) {
		ref, err = p.parseTableReference()
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

// parseTableReference parses one table reference.
func (p *Parser) parseTableReference() (*ast.TableRef, error) {
	start := p.currentStart()

	// Parenthesised table reference
	if p.check(utils.TOKEN_LPAREN) {
		p.advance() // consume '('
		inner, err := p.parseTableReference()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(utils.TOKEN_RPAREN); err != nil {
			return nil, err
		}
		return &ast.TableRef{ClauseBase: p.clauseBase(start), Paren: inner}, nil
	}

	// Named table with optional joins
	primary, err := p.parseTablePrimary()
	if err != nil {
		return nil, err
	}

	var joins []*ast.JoinClause
	for p.isJoinStart() {
		join, err := p.parseJoinClause()
		if err != nil {
			return nil, err
		}
		joins = append(joins, join)
	}

	return &ast.TableRef{
		ClauseBase: p.clauseBase(start),
		Primary:    primary,
		Joins:      joins,
	}, nil
}

// parseTablePrimary parses a named table with an optional alias.
func (p *Parser) parseTablePrimary() (*ast.TablePrimary, error) {
	start := p.currentStart()

	name, err := p.parseQualifiedIdentifier()
	if err != nil {
		return nil, err
	}

	var alias string
	if p.match(utils.TOKEN_AS) {
		alias, err = p.expectIdent()
		if err != nil {
			return nil, err
		}
	} else if p.check(utils.TOKEN_IDENT) {
		// Implicit alias: bare IDENT not preceded by AS.
		// We only accept TOKEN_IDENT here — keywords are not identifiers.
		alias = p.current.Literal
		p.advance()
	}

	return &ast.TablePrimary{
		ClauseBase: p.clauseBase(start),
		Name:       name,
		Alias:      alias,
	}, nil
}

// isJoinStart reports whether the current token begins a JoinClause.
func (p *Parser) isJoinStart() bool {
	switch p.current.Type {
	case utils.TOKEN_JOIN,
		utils.TOKEN_INNER,
		utils.TOKEN_LEFT,
		utils.TOKEN_RIGHT,
		utils.TOKEN_FULL,
		utils.TOKEN_CROSS:
		return true
	}
	return false
}

// parseJoinClause parses one JOIN operation.
func (p *Parser) parseJoinClause() (*ast.JoinClause, error) {
	start := p.currentStart()

	// Determine join type and consume qualifier keyword(s).
	joinType := ast.JoinInner // default for bare JOIN
	switch p.current.Type {
	case utils.TOKEN_CROSS:
		p.advance() // CROSS
		joinType = ast.JoinCross

	case utils.TOKEN_INNER:
		p.advance() // INNER
		joinType = ast.JoinInner

	case utils.TOKEN_LEFT:
		p.advance()                // LEFT
		p.match(utils.TOKEN_OUTER) // optional OUTER
		joinType = ast.JoinLeft

	case utils.TOKEN_RIGHT:
		p.advance()
		p.match(utils.TOKEN_OUTER)
		joinType = ast.JoinRight

	case utils.TOKEN_FULL:
		p.advance()
		p.match(utils.TOKEN_OUTER)
		joinType = ast.JoinFull

	case utils.TOKEN_JOIN:
		// bare JOIN — type already set to JoinInner, nothing to consume yet
	}

	// Every variant requires the JOIN keyword.
	if _, err := p.expect(utils.TOKEN_JOIN); err != nil {
		return nil, err
	}

	right, err := p.parseTablePrimary()
	if err != nil {
		return nil, err
	}

	// CROSS JOIN has no ON condition.
	if joinType == ast.JoinCross {
		return &ast.JoinClause{
			ClauseBase: p.clauseBase(start),
			Type:       joinType,
			Right:      right,
		}, nil
	}

	if _, err := p.expect(utils.TOKEN_ON); err != nil {
		return nil, err
	}

	on, err := p.parseCondition()
	if err != nil {
		return nil, err
	}

	return &ast.JoinClause{
		ClauseBase: p.clauseBase(start),
		Type:       joinType,
		Right:      right,
		On:         on,
	}, nil
}

// parseWhereClause handles: WHERE Condition
// Precondition: current == TOKEN_WHERE.
func (p *Parser) parseWhereClause() (*ast.WhereClause, error) {
	start := p.currentStart()
	p.advance() // WHERE

	cond, err := p.parseCondition()
	if err != nil {
		return nil, err
	}

	return &ast.WhereClause{ClauseBase: p.clauseBase(start), Cond: cond}, nil
}

// parseGroupByClause handles: GROUP BY QualifiedIdentifier (',' QualifiedIdentifier)*
// Precondition: current == TOKEN_GROUP.
func (p *Parser) parseGroupByClause() (*ast.GroupByClause, error) {
	start := p.currentStart()
	p.advance() // GROUP

	if _, err := p.expect(utils.TOKEN_BY); err != nil {
		return nil, err
	}

	col, err := p.parseQualifiedIdentifier()
	if err != nil {
		return nil, err
	}
	cols := []*ast.Identifier{col}

	for p.match(utils.TOKEN_COMMA) {
		col, err = p.parseQualifiedIdentifier()
		if err != nil {
			return nil, err
		}
		cols = append(cols, col)
	}

	return &ast.GroupByClause{ClauseBase: p.clauseBase(start), Columns: cols}, nil
}

// parseHavingClause handles: HAVING Condition
// Precondition: current == TOKEN_HAVING.
func (p *Parser) parseHavingClause() (*ast.HavingClause, error) {
	start := p.currentStart()
	p.advance() // HAVING

	cond, err := p.parseCondition()
	if err != nil {
		return nil, err
	}

	return &ast.HavingClause{ClauseBase: p.clauseBase(start), Cond: cond}, nil
}

// parseOrderByClause handles: ORDER BY OrderByItem (',' OrderByItem)*
// Precondition: current == TOKEN_ORDER.
func (p *Parser) parseOrderByClause() (*ast.OrderByClause, error) {
	start := p.currentStart()
	p.advance() // ORDER

	if _, err := p.expect(utils.TOKEN_BY); err != nil {
		return nil, err
	}

	item, err := p.parseOrderByItem()
	if err != nil {
		return nil, err
	}
	items := []*ast.OrderByItem{item}

	for p.match(utils.TOKEN_COMMA) {
		item, err = p.parseOrderByItem()
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return &ast.OrderByClause{ClauseBase: p.clauseBase(start), Items: items}, nil
}

// parseOrderByItem parses one ordering term: Expression ['ASC' | 'DESC']
// The grammar accepts any Expression (not just an identifier), so ORDER BY 1
// (positional) and ORDER BY a + b (computed) are both valid.
func (p *Parser) parseOrderByItem() (*ast.OrderByItem, error) {
	start := p.currentStart()

	expr, err := p.parseExpression()
	if err != nil {
		return nil, err
	}

	dir := ast.OrderAsc
	switch {
	case p.match(utils.TOKEN_DESC):
		dir = ast.OrderDesc
	case p.match(utils.TOKEN_ASC):
		// already the default; consumed but direction unchanged
	}

	return &ast.OrderByItem{
		ClauseBase: p.clauseBase(start),
		Expr:       expr,
		Direction:  dir,
	}, nil
}

// parseLimitClause handles: LIMIT Integer ['OFFSET' Integer]
// Precondition: current == TOKEN_LIMIT.
func (p *Parser) parseLimitClause() (*ast.LimitClause, error) {
	start := p.currentStart()
	p.advance() // LIMIT

	count, err := p.parseIntegerLiteralValue()
	if err != nil {
		return nil, err
	}

	var offset *int
	if p.match(utils.TOKEN_OFFSET) {
		n, err := p.parseIntegerLiteralValue()
		if err != nil {
			return nil, err
		}
		offset = &n
	}

	return &ast.LimitClause{
		ClauseBase: p.clauseBase(start),
		Count:      count,
		Offset:     offset,
	}, nil
}
