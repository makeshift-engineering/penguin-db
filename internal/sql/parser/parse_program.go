package parser

import (
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/lexer"
)

// parseStatement dispatches to the correct statement parser based on the
// current token. This is the only function that looks at the statement-level
// keyword; every sub-parser assumes its leading keyword is still in p.current
// when called.
func (p *Parser) parseStatement() (ast.Statement, error) {
	switch p.current.Type {
	case lexer.TOKEN_CREATE:
		return p.parseCreateStatement()
	case lexer.TOKEN_DROP:
		return p.parseDropStatement()
	case lexer.TOKEN_ALTER:
		return p.parseAlterTableStatement()
	case lexer.TOKEN_USE:
		return p.parseUseDatabaseStatement()
	case lexer.TOKEN_SELECT:
		return p.parseSelectStatement()
	case lexer.TOKEN_INSERT:
		return p.parseInsertStatement()
	case lexer.TOKEN_UPDATE:
		return p.parseUpdateStatement()
	case lexer.TOKEN_DELETE:
		return p.parseDeleteStatement()
	default:
		return nil, p.errorf(
			p.current.Span,
			CodeMalformedStatement,
			"unexpected token %q: expected a statement keyword (SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, ALTER, USE)",
			p.current.Literal,
		)
	}
}

// parseCreateStatement reads the token after CREATE (still in peek at this
// point) to decide which CREATE variant to dispatch to.
func (p *Parser) parseCreateStatement() (ast.Statement, error) {
	// peek holds the token immediately after CREATE
	switch p.tokens.Peek().Type {
	case lexer.TOKEN_DATABASE:
		return p.parseCreateDatabaseStatement()
	case lexer.TOKEN_TABLE:
		return p.parseCreateTableStatement()
	default:
		// Consume CREATE so the error span points at the unexpected token.
		p.advance()
		return nil, p.errorf(
			p.current.Span,
			CodeMalformedStatement,
			"expected DATABASE or TABLE after CREATE, got %s (%q)",
			p.current.Type, p.current.Literal,
		)
	}
}

// parseDropStatement uses the same peek-ahead technique as parseCreateStatement.
func (p *Parser) parseDropStatement() (ast.Statement, error) {
	switch p.tokens.Peek().Type {
	case lexer.TOKEN_DATABASE:
		return p.parseDropDatabaseStatement()
	case lexer.TOKEN_TABLE:
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
