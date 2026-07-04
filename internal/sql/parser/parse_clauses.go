// This file covers rules for AST Clause nodes, including:
// - Table references and aliases
// - JOIN clauses (CROSS, INNER, LEFT, RIGHT, FULL)
// - WHERE, GROUP BY, HAVING, ORDER BY, and LIMIT clauses
// - Column definitions and constraints (PRIMARY KEY, UNIQUE, NOT NULL, DEFAULT, REFERENCES)
// - ALTER action clauses
// - UPDATE SET assignment clauses
// - SELECT column list elements
package parser

import (
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

// Grammar Rule: TableReferences = TableReference ( ',' TableReference )*
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

// Grammar Rule: TableReference = TablePrimary JoinClause* | '(' TableReference ')' JoinClause*
// parseTableReference parses one table reference.
func (p *Parser) parseTableReference() (*ast.TableRef, error) {
	start := p.currentStart()

	// Parenthesised table reference or named table with optional joins.
	// Both paths fall through to the join loop below so that trailing
	// JOINs after e.g. (t1) JOIN t2 ON ... are not dropped.
	var ref *ast.TableRef
	if p.check(utils.TOKEN_LPAREN) {
		p.advance() // consume '('
		inner, err := p.parseTableReference()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(utils.TOKEN_RPAREN); err != nil {
			return nil, err
		}
		ref = &ast.TableRef{ClauseBase: p.clauseBase(start), Paren: inner}
	} else {
		primary, err := p.parseTablePrimary()
		if err != nil {
			return nil, err
		}
		ref = &ast.TableRef{ClauseBase: p.clauseBase(start), Primary: primary}
	}

	// Collect any trailing JOINs (applies to both parenthesised and named refs).
	for p.isJoinStart() {
		join, err := p.parseJoinClause()
		if err != nil {
			return nil, err
		}
		ref.Joins = append(ref.Joins, join)
	}

	return ref, nil
}

// Grammar Rule: TablePrimary = QualifiedIdentifier [ [ 'AS' ] Identifier ]
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

// Grammar Rule: isJoinStart = 'CROSS' | 'INNER' | 'LEFT' | 'RIGHT' | 'FULL' | 'JOIN'
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

// Grammar Rule: JoinClause = [ 'CROSS' | 'INNER' | 'LEFT' [ 'OUTER' ] | 'RIGHT' [ 'OUTER' ] | 'FULL' [ 'OUTER' ] ] 'JOIN' TablePrimary [ 'ON' Condition ]
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

// Grammar Rule: WhereClause = 'WHERE' Condition
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

// Grammar Rule: GroupByClause = 'GROUP' 'BY' QualifiedIdentifier ( ',' QualifiedIdentifier )*
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

// Grammar Rule: HavingClause = 'HAVING' Condition
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

// Grammar Rule: OrderByClause = 'ORDER' 'BY' OrderByItem ( ',' OrderByItem )*
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

// Grammar Rule: OrderByItem = Expression [ 'ASC' | 'DESC' ]
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

// Grammar Rule: LimitClause = 'LIMIT' Integer [ 'OFFSET' Integer ]
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

// Grammar Rule: SetItem = QualifiedIdentifier '=' Expression
// parseSetItem parses one assignment in an UPDATE SET clause:
// SetItem = QualifiedIdentifier '=' Expression
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

// Grammar Rule: AlterAction = 'ADD' [ 'COLUMN' ] ColumnDef
//
//	| 'MODIFY' [ 'COLUMN' ] ColumnDef
//	| 'RENAME' 'TO' Identifier
//	| 'RENAME' 'COLUMN' Identifier 'TO' Identifier
//	| 'DROP' 'COLUMN' Identifier
//
// parseAlterAction handles the four ALTER TABLE action variants.
func (p *Parser) parseAlterAction() (*ast.AlterAction, error) {
	start := p.currentStart()

	switch p.current.Type {
	case utils.TOKEN_ADD, utils.TOKEN_MODIFY:
		kind := ast.AlterAdd
		if p.check(utils.TOKEN_MODIFY) {
			kind = ast.AlterModify
		}
		p.advance()                 // ADD or MODIFY
		p.match(utils.TOKEN_COLUMN) // optional COLUMN keyword

		col, err := p.parseColumnDefinition()
		if err != nil {
			return nil, err
		}
		return &ast.AlterAction{
			ClauseBase: p.clauseBase(start),
			Kind:       kind,
			Column:     col,
		}, nil

	case utils.TOKEN_RENAME:
		p.advance() // RENAME
		switch p.current.Type {
		case utils.TOKEN_TO:
			// RENAME TO <new_table_name>
			p.advance() // TO
			newName, err := p.expectIdent()
			if err != nil {
				return nil, err
			}
			return &ast.AlterAction{
				ClauseBase: p.clauseBase(start),
				Kind:       ast.AlterRenameTable,
				NewName:    newName,
			}, nil

		case utils.TOKEN_COLUMN:
			// RENAME COLUMN <old> TO <new>
			p.advance() // COLUMN
			oldName, err := p.expectIdent()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(utils.TOKEN_TO); err != nil {
				return nil, err
			}
			newName, err := p.expectIdent()
			if err != nil {
				return nil, err
			}
			return &ast.AlterAction{
				ClauseBase: p.clauseBase(start),
				Kind:       ast.AlterRenameColumn,
				OldName:    oldName,
				NewName:    newName,
			}, nil

		default:
			return nil, p.errorf(
				p.current.Span,
				CodeInvalidAlterAction,
				"expected TO or COLUMN after RENAME, got %s (%q)",
				p.current.Type, p.current.Literal,
			)
		}

	case utils.TOKEN_DROP:
		p.advance() // DROP
		if _, err := p.expect(utils.TOKEN_COLUMN); err != nil {
			return nil, err
		}
		dropName, err := p.expectIdent()
		if err != nil {
			return nil, err
		}
		return &ast.AlterAction{
			ClauseBase: p.clauseBase(start),
			Kind:       ast.AlterDropColumn,
			DropName:   dropName,
		}, nil

	default:
		return nil, p.errorf(
			p.current.Span,
			CodeInvalidAlterAction,
			"expected ADD, MODIFY, RENAME, or DROP after ALTER TABLE <name>, got %s (%q)",
			p.current.Type, p.current.Literal,
		)
	}
}

// Grammar Rule: ColumnDefinitions = ColumnDef ( ',' ColumnDef )*
// parseColumnDefinitions parses a comma-separated list of column definitions.
// Called after the opening '(' of a CREATE TABLE statement.
func (p *Parser) parseColumnDefinitions() ([]*ast.ColumnDef, error) {
	col, err := p.parseColumnDefinition()
	if err != nil {
		return nil, err
	}
	cols := []*ast.ColumnDef{col}

	for p.match(utils.TOKEN_COMMA) {
		col, err = p.parseColumnDefinition()
		if err != nil {
			return nil, err
		}
		cols = append(cols, col)
	}
	return cols, nil
}

// Grammar Rule: ColumnDef = Identifier DataType ColumnConstraint*
// parseColumnDefinition parses a single column definition.
func (p *Parser) parseColumnDefinition() (*ast.ColumnDef, error) {
	start := p.currentStart()

	name, err := p.expectIdent()
	if err != nil {
		return nil, err
	}

	dt, err := p.parseDataType()
	if err != nil {
		return nil, err
	}

	constraints, err := p.parseColumnConstraints()
	if err != nil {
		return nil, err
	}

	return &ast.ColumnDef{
		ClauseBase:  p.clauseBase(start),
		Name:        name,
		Type:        dt,
		Constraints: constraints,
	}, nil
}

// Grammar Rule: ColumnConstraints = ColumnConstraint*
// parseColumnConstraints collects zero or more column-level constraints.
// Stops as soon as the current token is not a constraint-starting keyword.
func (p *Parser) parseColumnConstraints() ([]ast.Clause, error) {
	var constraints []ast.Clause
	for p.isConstraintStart() {
		c, err := p.parseColumnConstraint()
		if err != nil {
			return nil, err
		}
		constraints = append(constraints, c)
	}
	return constraints, nil
}

// Grammar Rule: isConstraintStart = 'PRIMARY' | 'UNIQUE' | 'NOT' | 'NULL' | 'DEFAULT' | 'REFERENCES'
// isConstraintStart reports whether the current token can begin a column constraint.
func (p *Parser) isConstraintStart() bool {
	switch p.current.Type {
	case utils.TOKEN_PRIMARY,
		utils.TOKEN_UNIQUE,
		utils.TOKEN_NOT,
		utils.TOKEN_NULL,
		utils.TOKEN_DEFAULT,
		utils.TOKEN_REFERENCES:
		return true
	}
	return false
}

// Grammar Rule: ColumnConstraint = 'PRIMARY' 'KEY'
//
//	| 'UNIQUE'
//	| 'NOT' 'NULL'
//	| 'NULL'
//	| 'DEFAULT' SignedLiteral
//	| 'REFERENCES' Identifier '(' Identifier ')'
//
// parseColumnConstraint parses a single column constraint.
func (p *Parser) parseColumnConstraint() (ast.Clause, error) {
	start := p.currentStart()

	switch p.current.Type {
	case utils.TOKEN_PRIMARY:
		p.advance() // PRIMARY
		if _, err := p.expect(utils.TOKEN_KEY); err != nil {
			return nil, err
		}
		return &ast.PrimaryKeyConstraint{ClauseBase: p.clauseBase(start)}, nil

	case utils.TOKEN_UNIQUE:
		p.advance() // UNIQUE
		return &ast.UniqueConstraint{ClauseBase: p.clauseBase(start)}, nil

	case utils.TOKEN_NOT:
		p.advance() // NOT
		if _, err := p.expect(utils.TOKEN_NULL); err != nil {
			return nil, err
		}
		return &ast.NotNullConstraint{ClauseBase: p.clauseBase(start)}, nil

	case utils.TOKEN_NULL:
		p.advance() // NULL
		return &ast.NullConstraint{ClauseBase: p.clauseBase(start)}, nil

	case utils.TOKEN_DEFAULT:
		p.advance() // DEFAULT
		lit, err := p.parseSignedLiteral()
		if err != nil {
			return nil, err
		}
		return &ast.DefaultConstraint{ClauseBase: p.clauseBase(start), Value: lit}, nil

	case utils.TOKEN_REFERENCES:
		p.advance() // REFERENCES
		table, err := p.expectIdent()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(utils.TOKEN_LPAREN); err != nil {
			return nil, err
		}
		col, err := p.expectIdent()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(utils.TOKEN_RPAREN); err != nil {
			return nil, err
		}
		return &ast.ForeignRef{
			ClauseBase: p.clauseBase(start),
			Table:      table,
			Column:     col,
		}, nil

	default:
		return nil, p.errorf(
			p.current.Span,
			CodeUnexpectedToken,
			"expected column constraint keyword, got %s (%q)",
			p.current.Type, p.current.Literal,
		)
	}
}

// Grammar Rule: DataType = 'INT' | 'BIGINT' | 'BOOLEAN' | 'TEXT' | 'TIMESTAMP' | 'FLOAT' | 'DOUBLE'
//
//	| 'DECIMAL' [ '(' Integer [ ',' Integer ] ')' ]
//	| 'VARCHAR' '(' Integer ')'
//
// parseDataType parses a SQL data type.
func (p *Parser) parseDataType() (*ast.DataType, error) {
	start := p.currentStart()

	switch p.current.Type {
	case utils.TOKEN_INT:
		p.advance()
		return &ast.DataType{ClauseBase: p.clauseBase(start), Kind: ast.TypeInt}, nil

	case utils.TOKEN_BIGINT:
		p.advance()
		return &ast.DataType{ClauseBase: p.clauseBase(start), Kind: ast.TypeBigInt}, nil

	case utils.TOKEN_BOOLEAN:
		p.advance()
		return &ast.DataType{ClauseBase: p.clauseBase(start), Kind: ast.TypeBoolean}, nil

	case utils.TOKEN_TEXT:
		p.advance()
		return &ast.DataType{ClauseBase: p.clauseBase(start), Kind: ast.TypeText}, nil

	case utils.TOKEN_TIMESTAMP:
		p.advance()
		return &ast.DataType{ClauseBase: p.clauseBase(start), Kind: ast.TypeTimestamp}, nil

	case utils.TOKEN_FLOAT_TYPE:
		p.advance()
		return &ast.DataType{ClauseBase: p.clauseBase(start), Kind: ast.TypeFloat}, nil

	case utils.TOKEN_DOUBLE:
		p.advance()
		return &ast.DataType{ClauseBase: p.clauseBase(start), Kind: ast.TypeDouble}, nil

	case utils.TOKEN_DECIMAL:
		p.advance() // DECIMAL
		var prec, scale *int
		if p.match(utils.TOKEN_LPAREN) {
			pVal, err := p.parseIntegerLiteralValue()
			if err != nil {
				return nil, err
			}
			prec = &pVal
			if p.match(utils.TOKEN_COMMA) {
				sVal, err := p.parseIntegerLiteralValue()
				if err != nil {
					return nil, err
				}
				scale = &sVal
			}
			if _, err := p.expect(utils.TOKEN_RPAREN); err != nil {
				return nil, err
			}
		}
		return &ast.DataType{
			ClauseBase:   p.clauseBase(start),
			Kind:         ast.TypeDecimal,
			DecimalPrec:  prec,
			DecimalScale: scale,
		}, nil

	case utils.TOKEN_VARCHAR:
		p.advance() // VARCHAR
		if _, err := p.expect(utils.TOKEN_LPAREN); err != nil {
			return nil, err
		}
		n, err := p.parseIntegerLiteralValue()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(utils.TOKEN_RPAREN); err != nil {
			return nil, err
		}
		return &ast.DataType{
			ClauseBase: p.clauseBase(start),
			Kind:       ast.TypeVarchar,
			VarcharLen: &n,
		}, nil

	default:
		return nil, p.errorf(
			p.current.Span,
			CodeInvalidDataType,
			"expected a data type (INT, BIGINT, VARCHAR, BOOLEAN, TEXT, TIMESTAMP, FLOAT, DOUBLE, DECIMAL), got %s (%q)",
			p.current.Type, p.current.Literal,
		)
	}
}

// Grammar Rule: SignedLiteral = [ '+' | '-' ] Literal
// parseSignedLiteral parses an optional sign followed by a literal value.
// Used for DEFAULT constraint values.
func (p *Parser) parseSignedLiteral() (*ast.SignedLiteral, error) {
	start := p.currentStart()
	negative := false
	signed := false

	if p.check(utils.TOKEN_MINUS) {
		p.advance()
		negative = true
		signed = true
	} else if p.check(utils.TOKEN_PLUS) {
		p.advance()
		signed = true
	}

	// After a sign only numeric literals are valid.
	var val ast.Expression
	var err error
	if signed {
		val, err = p.parseNumericLiteral()
	} else {
		val, err = p.parseLiteral()
	}
	if err != nil {
		return nil, err
	}

	return &ast.SignedLiteral{
		ClauseBase: p.clauseBase(start),
		Negative:   negative,
		Value:      val,
	}, nil
}

// Grammar Rule: SelectList = SelectColumn ( ',' SelectColumn )*
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

// Grammar Rule: SelectColumn = '*'
//
//	| Identifier '.' '*'
//	| Identifier '.' Identifier '.' '*'
//	| SelectExpression [ [ 'AS' ] Identifier ]
//
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

// Grammar Rule: SelectColumnFromPrimary = Identifier (TermTail | ExprTail) [ [ 'AS' ] Identifier ]
// parseSelectColumnFromPrimary finishes a SelectColumn when the caller has
// already consumed and reconstructed `primary` as the Factor-level expression.
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

// Grammar Rule: OptionalAlias = [ 'AS' ] Identifier
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
