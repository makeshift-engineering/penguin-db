package parser

import (
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

// parseCreateDatabaseStatement handles:
// CREATE DATABASE [IF NOT EXISTS] Identifier
func (p *Parser) parseCreateDatabaseStatement() (*ast.CreateDatabaseStmt, error) {
	start := p.currentStart()
	p.advance() // CREATE
	p.advance() // DATABASE

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

// parseUseDatabaseStatement handles:
// USE Identifier
func (p *Parser) parseUseDatabaseStatement() (*ast.UseDatabaseStmt, error) {
	start := p.currentStart()
	p.advance() // USE

	name, err := p.expectIdent()
	if err != nil {
		return nil, err
	}

	return &ast.UseDatabaseStmt{
		StmtBase: p.stmtBase(start),
		Name:     name,
	}, nil
}

// parseDropDatabaseStatement handles:
// DROP DATABASE [IF EXISTS] Identifier
func (p *Parser) parseDropDatabaseStatement() (*ast.DropDatabaseStmt, error) {
	start := p.currentStart()
	p.advance() // DROP
	p.advance() // DATABASE

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

// parseCreateTableStatement handles:
// CREATE TABLE [IF NOT EXISTS] QualifiedIdentifier '(' ColumnDefinition (',' ColumnDefinition)* ')'
func (p *Parser) parseCreateTableStatement() (*ast.CreateTableStmt, error) {
	start := p.currentStart()
	p.advance() // CREATE
	p.advance() // TABLE

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

// parseAlterTableStatement handles:
// ALTER TABLE QualifiedIdentifier AlterAction
func (p *Parser) parseAlterTableStatement() (*ast.AlterTableStmt, error) {
	start := p.currentStart()
	p.advance() // ALTER

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

// parseAlterAction handles the four ALTER TABLE action variants:
//
//	ADD    [COLUMN] ColumnDefinition
//	MODIFY [COLUMN] ColumnDefinition
//	RENAME TO NewName
//	RENAME COLUMN OldName TO NewName
//	DROP   COLUMN ColumnName
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

// parseDropTableStatement handles:
// DROP TABLE [IF EXISTS] QualifiedIdentifier
func (p *Parser) parseDropTableStatement() (*ast.DropTableStmt, error) {
	start := p.currentStart()
	p.advance() // DROP
	p.advance() // TABLE

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

// parseColumnDefinitions parses a comma-separated list of column definitions.
// Called after the opening '(' of a CREATE TABLE statement.
// ColumnDefinitions = ColumnDefinition ( ',' ColumnDefinition )*
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

// parseColumnDefinition parses a single column definition:
// ColumnDefinition = Identifier DataType ColumnConstraint*
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

// isConstraintStart reports whether the current token can begin a column
// constraint. This is the FIRST set of the ColumnConstraint rule.
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
		// Should be unreachable if isConstraintStart is correct.
		return nil, p.errorf(
			p.current.Span,
			CodeUnexpectedToken,
			"expected column constraint keyword, got %s (%q)",
			p.current.Type, p.current.Literal,
		)
	}
}

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

// parseQualifiedIdentifier parses a simple or dot-qualified name.
func (p *Parser) parseQualifiedIdentifier() (*ast.Identifier, error) {
	start := p.currentStart()

	nameTok, err := p.expect(utils.TOKEN_IDENT)
	if err != nil {
		return nil, err
	}

	name := nameTok.Literal
	qualifier := ""

	// Only consume the dot if it is followed by another IDENT (not '*' or EOF).
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
