package parser

import (
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

// parseInsertStatement handles:
//
//	INSERT INTO QualifiedIdentifier
//	  ['(' Identifier (',' Identifier)* ')']
//	  ( VALUES ValueRow (',' ValueRow)* | SelectStatement )
//
// Column-list disambiguation: after INSERT INTO <table>, a '(' followed by an
// IDENT is always a column list — the only valid alternative is VALUES or SELECT,
// neither of which starts with '('. So if current is '(' and peek is IDENT, it
// is the column list.
func (p *Parser) parseInsertStatement() (*ast.InsertStmt, error) {
	start := p.currentStart()
	p.advance() // INSERT

	if _, err := p.expect(utils.TOKEN_INTO); err != nil {
		return nil, err
	}

	table, err := p.parseQualifiedIdentifier()
	if err != nil {
		return nil, err
	}

	// Optional column list: '(' Identifier (',' Identifier)* ')'
	var cols []string
	if p.check(utils.TOKEN_LPAREN) && p.peekIs(utils.TOKEN_IDENT) {
		p.advance() // consume '('

		col, err := p.expectIdent()
		if err != nil {
			return nil, err
		}
		cols = append(cols, col)

		for p.match(utils.TOKEN_COMMA) {
			col, err = p.expectIdent()
			if err != nil {
				return nil, err
			}
			cols = append(cols, col)
		}

		if _, err := p.expect(utils.TOKEN_RPAREN); err != nil {
			return nil, err
		}
	}

	// VALUES or SELECT
	switch p.current.Type {
	case utils.TOKEN_VALUES:
		p.advance() // VALUES

		row, err := p.parseValueRow()
		if err != nil {
			return nil, err
		}
		rows := [][]*ast.SelectExpression{row}

		for p.match(utils.TOKEN_COMMA) {
			row, err = p.parseValueRow()
			if err != nil {
				return nil, err
			}
			rows = append(rows, row)
		}

		return &ast.InsertStmt{
			StmtBase: p.stmtBase(start),
			Table:    table,
			Columns:  cols,
			Rows:     rows,
		}, nil

	case utils.TOKEN_SELECT:
		source, err := p.parseSelectStatement()
		if err != nil {
			return nil, err
		}
		return &ast.InsertStmt{
			StmtBase: p.stmtBase(start),
			Table:    table,
			Columns:  cols,
			Source:   source,
		}, nil

	default:
		return nil, p.errorf(
			p.current.Span,
			CodeMalformedStatement,
			"expected VALUES or SELECT after INSERT INTO <table> [(<cols>)], got %s (%q)",
			p.current.Type, p.current.Literal,
		)
	}
}

// parseValueRow parses one row of INSERT VALUES:
//
//	ValueRow = '(' Expression (',' Expression)* ')'
//
// The grammar specifies Expression (not SelectExpression) inside VALUE rows —
// conditions are not valid insert values. Each Expression is wrapped in an
// ast.SelectExpression with Expr set, because InsertStmt.Rows is [][]*SelectExpression.
func (p *Parser) parseValueRow() ([]*ast.SelectExpression, error) {
	if _, err := p.expect(utils.TOKEN_LPAREN); err != nil {
		return nil, err
	}

	first, err := p.parseExpression()
	if err != nil {
		return nil, err
	}
	vals := []*ast.SelectExpression{
		{NodeBase: ast.NodeBase{NodeSpan: first.Span()}, Expr: first},
	}

	for p.match(utils.TOKEN_COMMA) {
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		vals = append(vals, &ast.SelectExpression{
			NodeBase: ast.NodeBase{NodeSpan: expr.Span()},
			Expr:     expr,
		})
	}

	if _, err := p.expect(utils.TOKEN_RPAREN); err != nil {
		return nil, err
	}
	return vals, nil
}

// parseUpdateStatement handles:
//
//	UPDATE QualifiedIdentifier SET SetItem (',' SetItem)* [WHERE Condition]
func (p *Parser) parseUpdateStatement() (*ast.UpdateStmt, error) {
	start := p.currentStart()
	p.advance() // UPDATE

	table, err := p.parseQualifiedIdentifier()
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(utils.TOKEN_SET); err != nil {
		return nil, err
	}

	item, err := p.parseSetItem()
	if err != nil {
		return nil, err
	}
	items := []*ast.SetItem{item}

	for p.match(utils.TOKEN_COMMA) {
		item, err = p.parseSetItem()
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	var where *ast.WhereClause
	if p.check(utils.TOKEN_WHERE) {
		where, err = p.parseWhereClause()
		if err != nil {
			return nil, err
		}
	}

	return &ast.UpdateStmt{
		StmtBase: p.stmtBase(start),
		Table:    table,
		Set:      items,
		Where:    where,
	}, nil
}

// parseSetItem parses one assignment in an UPDATE SET clause:
//
//	SetItem = QualifiedIdentifier '=' Expression
func (p *Parser) parseSetItem() (*ast.SetItem, error) {
	start := p.currentStart()

	col, err := p.parseQualifiedIdentifier()
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(utils.TOKEN_EQ); err != nil {
		return nil, err
	}

	val, err := p.parseExpression()
	if err != nil {
		return nil, err
	}

	return &ast.SetItem{
		ClauseBase: p.clauseBase(start),
		Column:     col,
		Value:      val,
	}, nil
}

// parseDeleteStatement handles:
//
//	DELETE FROM QualifiedIdentifier [WHERE Condition]
func (p *Parser) parseDeleteStatement() (*ast.DeleteStmt, error) {
	start := p.currentStart()
	p.advance() // DELETE

	if _, err := p.expect(utils.TOKEN_FROM); err != nil {
		return nil, err
	}

	table, err := p.parseQualifiedIdentifier()
	if err != nil {
		return nil, err
	}

	var where *ast.WhereClause
	if p.check(utils.TOKEN_WHERE) {
		var err error
		where, err = p.parseWhereClause()
		if err != nil {
			return nil, err
		}
	}

	return &ast.DeleteStmt{
		StmtBase: p.stmtBase(start),
		Table:    table,
		Where:    where,
	}, nil
}
