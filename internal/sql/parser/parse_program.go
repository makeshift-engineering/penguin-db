package parser

import (
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

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
