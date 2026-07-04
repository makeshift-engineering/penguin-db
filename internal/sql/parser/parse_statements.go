// Package parser implements a single-pass recursive descent SQL parser.
// This file covers rules for AST Statement / Program nodes, including:
// - Statement dispatching (SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, ALTER, USE)
// - Database DDL (CREATE DATABASE, DROP DATABASE, USE)
// - Table DDL (CREATE TABLE, DROP TABLE, ALTER TABLE)
// - DML (INSERT, UPDATE, DELETE)
// - Query (SELECT)
package parser

import (
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

// Grammar Rule: Statement = CreateStatement
//
//	| DropStatement
//	| AlterTableStatement
//	| UseDatabaseStatement
//	| SelectStatement
//	| InsertStatement
//	| UpdateStatement
//	| DeleteStatement
//
// parseStatement inspects the current token and dispatches to the
// appropriate statement-level parse function.
func (p *Parser) parseStatement() (ast.Statement, error) {
	switch p.current.Type {
	case utils.TOKEN_CREATE:
		return p.parseCreateStatement()
	case utils.TOKEN_DROP:
		return p.parseDropStatement()
	case utils.TOKEN_ALTER:
		return p.parseAlterTableStatement()
	case utils.TOKEN_USE:
		return p.parseUseDatabaseStatement()
	case utils.TOKEN_SELECT:
		return p.parseSelectStatement()
	case utils.TOKEN_INSERT:
		return p.parseInsertStatement()
	case utils.TOKEN_UPDATE:
		return p.parseUpdateStatement()
	case utils.TOKEN_DELETE:
		return p.parseDeleteStatement()
	default:
		return nil, p.errorf(
			p.current.Span,
			CodeMalformedStatement,
			"unexpected token %q: expected a statement keyword",
			p.current.Literal,
		)
	}
}

// Grammar Rule: CreateStatement = 'CREATE' ( 'DATABASE' CreateDatabaseStmt | 'TABLE' CreateTableStmt )
// parseCreateStatement peeks at the token following CREATE and dispatches.
func (p *Parser) parseCreateStatement() (ast.Statement, error) {
	switch p.tokens.Peek().Type {
	case utils.TOKEN_DATABASE:
		return p.parseCreateDatabaseStatement()
	case utils.TOKEN_TABLE:
		return p.parseCreateTableStatement()
	default:
		p.advance() // consume CREATE so the span points at the bad token
		return nil, p.errorf(
			p.current.Span,
			CodeMalformedStatement,
			"expected DATABASE or TABLE after CREATE, got %s (%q)",
			p.current.Type, p.current.Literal,
		)
	}
}

// Grammar Rule: DropStatement = 'DROP' ( 'DATABASE' DropDatabaseStmt | 'TABLE' DropTableStmt )
// parseDropStatement peeks at the token following DROP and dispatches.
func (p *Parser) parseDropStatement() (ast.Statement, error) {
	switch p.tokens.Peek().Type {
	case utils.TOKEN_DATABASE:
		return p.parseDropDatabaseStatement()
	case utils.TOKEN_TABLE:
		return p.parseDropTableStatement()
	default:
		p.advance()
		return nil, p.errorf(
			p.current.Span,
			CodeMalformedStatement,
			"expected DATABASE or TABLE after DROP, got %s (%q)",
			p.current.Type, p.current.Literal,
		)
	}
}

// Grammar Rule: CreateDatabaseStmt = 'CREATE' 'DATABASE' [ 'IF' 'NOT' 'EXISTS' ] Identifier
// parseCreateDatabaseStatement handles: CREATE DATABASE [IF NOT EXISTS] Identifier
func (p *Parser) parseCreateDatabaseStatement() (*ast.CreateDatabaseStmt, error) {
	start := p.currentStart()
	if _, err := p.expect(utils.TOKEN_CREATE); err != nil {
		return nil, err
	}
	if _, err := p.expect(utils.TOKEN_DATABASE); err != nil {
		return nil, err
	}

	ifNotExists, err := p.parseIfNotExists()
	if err != nil {
		return nil, err
	}

	name, err := p.expectIdent()
	if err != nil {
		return nil, err
	}

	return &ast.CreateDatabaseStmt{
		StmtBase:    p.stmtBase(start),
		Name:        name,
		IfNotExists: ifNotExists,
	}, nil
}

// Grammar Rule: UseDatabaseStmt = 'USE' Identifier
// parseUseDatabaseStatement handles: USE Identifier
func (p *Parser) parseUseDatabaseStatement() (*ast.UseDatabaseStmt, error) {
	start := p.currentStart()
	if _, err := p.expect(utils.TOKEN_USE); err != nil {
		return nil, err
	}

	name, err := p.expectIdent()
	if err != nil {
		return nil, err
	}

	return &ast.UseDatabaseStmt{
		StmtBase: p.stmtBase(start),
		Name:     name,
	}, nil
}

// Grammar Rule: DropDatabaseStmt = 'DROP' 'DATABASE' [ 'IF' 'EXISTS' ] Identifier
// parseDropDatabaseStatement handles: DROP DATABASE [IF EXISTS] Identifier
func (p *Parser) parseDropDatabaseStatement() (*ast.DropDatabaseStmt, error) {
	start := p.currentStart()
	if _, err := p.expect(utils.TOKEN_DROP); err != nil {
		return nil, err
	}
	if _, err := p.expect(utils.TOKEN_DATABASE); err != nil {
		return nil, err
	}

	ifExists, err := p.parseIfExists()
	if err != nil {
		return nil, err
	}

	name, err := p.expectIdent()
	if err != nil {
		return nil, err
	}

	return &ast.DropDatabaseStmt{
		StmtBase: p.stmtBase(start),
		Name:     name,
		IfExists: ifExists,
	}, nil
}

// Grammar Rule: CreateTableStmt = 'CREATE' 'TABLE' [ 'IF' 'NOT' 'EXISTS' ] QualifiedIdentifier '(' ColumnDefinitions ')'
// parseCreateTableStatement handles: CREATE TABLE [IF NOT EXISTS] QualifiedIdentifier '(' ColumnDefinition (',' ColumnDefinition)* ')'
func (p *Parser) parseCreateTableStatement() (*ast.CreateTableStmt, error) {
	start := p.currentStart()
	if _, err := p.expect(utils.TOKEN_CREATE); err != nil {
		return nil, err
	}
	if _, err := p.expect(utils.TOKEN_TABLE); err != nil {
		return nil, err
	}

	ifNotExists, err := p.parseIfNotExists()
	if err != nil {
		return nil, err
	}

	table, err := p.parseQualifiedIdentifier()
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(utils.TOKEN_LPAREN); err != nil {
		return nil, err
	}

	cols, err := p.parseColumnDefinitions()
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(utils.TOKEN_RPAREN); err != nil {
		return nil, err
	}

	return &ast.CreateTableStmt{
		StmtBase:    p.stmtBase(start),
		Table:       table,
		IfNotExists: ifNotExists,
		Columns:     cols,
	}, nil
}

// Grammar Rule: AlterTableStmt = 'ALTER' 'TABLE' QualifiedIdentifier AlterAction
// parseAlterTableStatement handles: ALTER TABLE QualifiedIdentifier AlterAction
func (p *Parser) parseAlterTableStatement() (*ast.AlterTableStmt, error) {
	start := p.currentStart()
	if _, err := p.expect(utils.TOKEN_ALTER); err != nil {
		return nil, err
	}

	if _, err := p.expect(utils.TOKEN_TABLE); err != nil {
		return nil, err
	}

	table, err := p.parseQualifiedIdentifier()
	if err != nil {
		return nil, err
	}

	action, err := p.parseAlterAction()
	if err != nil {
		return nil, err
	}

	return &ast.AlterTableStmt{
		StmtBase: p.stmtBase(start),
		Table:    table,
		Action:   action,
	}, nil
}

// Grammar Rule: DropTableStmt = 'DROP' 'TABLE' [ 'IF' 'EXISTS' ] QualifiedIdentifier
// parseDropTableStatement handles: DROP TABLE [IF EXISTS] QualifiedIdentifier
func (p *Parser) parseDropTableStatement() (*ast.DropTableStmt, error) {
	start := p.currentStart()
	if _, err := p.expect(utils.TOKEN_DROP); err != nil {
		return nil, err
	}
	if _, err := p.expect(utils.TOKEN_TABLE); err != nil {
		return nil, err
	}

	ifExists, err := p.parseIfExists()
	if err != nil {
		return nil, err
	}

	table, err := p.parseQualifiedIdentifier()
	if err != nil {
		return nil, err
	}

	return &ast.DropTableStmt{
		StmtBase: p.stmtBase(start),
		Table:    table,
		IfExists: ifExists,
	}, nil
}

// Grammar Rule: IfNotExists = [ 'IF' 'NOT' 'EXISTS' ]
// parseIfNotExists consumes [IF NOT EXISTS] and returns the flag.
func (p *Parser) parseIfNotExists() (bool, error) {
	if !p.check(utils.TOKEN_IF) {
		return false, nil
	}
	p.advance() // IF
	if _, err := p.expect(utils.TOKEN_NOT); err != nil {
		return false, err
	}
	if _, err := p.expect(utils.TOKEN_EXISTS); err != nil {
		return false, err
	}
	return true, nil
}

// Grammar Rule: IfExists = [ 'IF' 'EXISTS' ]
// parseIfExists consumes [IF EXISTS] and returns the flag.
func (p *Parser) parseIfExists() (bool, error) {
	if !p.check(utils.TOKEN_IF) {
		return false, nil
	}
	p.advance() // IF
	if _, err := p.expect(utils.TOKEN_EXISTS); err != nil {
		return false, err
	}
	return true, nil
}

// Grammar Rule: InsertStmt = 'INSERT' 'INTO' QualifiedIdentifier [ '(' IdentifierList ')' ] ( 'VALUES' ValueRowList | SelectStatement )
// parseInsertStatement handles INSERT INTO statement.
func (p *Parser) parseInsertStatement() (*ast.InsertStmt, error) {
	start := p.currentStart()
	if _, err := p.expect(utils.TOKEN_INSERT); err != nil {
		return nil, err
	}

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

// Grammar Rule: ValueRow = '(' Expression ( ',' Expression )* ')'
// parseValueRow parses one row of INSERT VALUES.
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

// Grammar Rule: UpdateStmt = 'UPDATE' QualifiedIdentifier 'SET' SetItemList [ WhereClause ]
// parseUpdateStatement handles UPDATE statement.
func (p *Parser) parseUpdateStatement() (*ast.UpdateStmt, error) {
	start := p.currentStart()
	if _, err := p.expect(utils.TOKEN_UPDATE); err != nil {
		return nil, err
	}

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

// Grammar Rule: DeleteStmt = 'DELETE' 'FROM' QualifiedIdentifier [ WhereClause ]
// parseDeleteStatement handles DELETE FROM statement.
func (p *Parser) parseDeleteStatement() (*ast.DeleteStmt, error) {
	start := p.currentStart()
	if _, err := p.expect(utils.TOKEN_DELETE); err != nil {
		return nil, err
	}

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

// Grammar Rule: SelectStmt = 'SELECT' [ 'DISTINCT' | 'ALL' ] SelectList
//
//	[ 'FROM' TableReferences ]
//	[ WhereClause ]
//	[ GroupByClause ]
//	[ HavingClause ]
//	[ OrderByClause ]
//	[ LimitClause ]
//
// parseSelectStatement handles the full SELECT syntax.
func (p *Parser) parseSelectStatement() (*ast.SelectStmt, error) {
	start := p.currentStart()
	if _, err := p.expect(utils.TOKEN_SELECT); err != nil {
		return nil, err
	}

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
