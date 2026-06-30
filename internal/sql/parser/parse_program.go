package parser

import (
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

// parseStatement inspects the current token and dispatches to the
// appropriate statement-level parse function (DDL, DML, or USE). It returns a
// [CodeMalformedStatement] error when the current token is not a recognised
// statement keyword.
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

// parseCreateStatement peeks at the token following CREATE and dispatches to
// either parseCreateDatabaseStatement or parseCreateTableStatement.
// If the lookahead is neither DATABASE nor TABLE, the CREATE token is consumed
// first so the error span points at the unexpected token rather than at CREATE.
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

// parseDropStatement peeks at the token following DROP and dispatches to
// either parseDropDatabaseStatement or parseDropTableStatement.
// If the lookahead is neither DATABASE nor TABLE, the DROP token is consumed
// first so the error span points at the unexpected token rather than at DROP.
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
