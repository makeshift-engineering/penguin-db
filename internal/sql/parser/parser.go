package parser

import (
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
	"github.com/makeshift-engineering/penguin-db/internal/sql/lexer"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

// Parser is a single-pass recursive descent parser.
type Parser struct {
	tokens  *utils.LookaheadIterator[lexer.Token]
	lex     *lexer.Lexer
	current lexer.Token
	diag    diagnostic.List
	source  *diagnostic.Source
}

// New creates a Parser for the token stream produced by l.
func New(l *lexer.Lexer, src *diagnostic.Source) *Parser {
	p := &Parser{
		tokens: utils.NewLookaheadIterator(l.NextToken),
		lex:    l,
		source: src,
	}
	p.advance()
	return p
}

// advance consumes the next token from the stream, storing it in p.current.
func (p *Parser) advance() {
	p.current = p.tokens.Next()
}

// check reports whether the current token has the given type.
// Does not consume.
func (p *Parser) check(t lexer.TokenType) bool {
	return p.current.Type == t
}

// peekIs reports whether the lookahead token (the token after current)
// has the given type. Does not consume either token.
func (p *Parser) peekIs(t lexer.TokenType) bool {
	return p.tokens.Peek().Type == t
}

// match consumes current and returns true if it matches any of the given types.
// Returns false without consuming if nothing matches.
func (p *Parser) match(types ...lexer.TokenType) bool {
	for _, t := range types {
		if p.check(t) {
			p.advance()
			return true
		}
	}
	return false
}

// expect consumes and returns current if its type matches t.
// On a mismatch it records an UnexpectedToken diagnostic and returns an error.
func (p *Parser) expect(t lexer.TokenType) (lexer.Token, error) {
	if !p.check(t) {
		return lexer.Token{}, p.errorf(
			p.current.Span,
			CodeUnexpectedToken,
			"expected %s, got %s (%q)",
			t, p.current.Type, p.current.Literal,
		)
	}
	tok := p.current
	p.advance()
	return tok, nil
}

// expectIdent consumes and returns the Literal of current if it is TOKEN_IDENT.
func (p *Parser) expectIdent() (string, error) {
	tok, err := p.expect(lexer.TOKEN_IDENT)
	if err != nil {
		return "", err
	}
	return tok.Literal, nil
}

// currentStart returns the start position of the current (not-yet-consumed)
// token. Call this as the very first line of every parse function.
func (p *Parser) currentStart() diagnostic.Pos {
	return p.current.Span.Start
}

// spanFrom builds a [Start, End) Span from the given start position to the
// end of the last consumed token. Call after the final consume in a rule.
func (p *Parser) spanFrom(start diagnostic.Pos) diagnostic.Span {
	return diagnostic.Span{Start: start, End: p.current.Span.End}
}

func (p *Parser) nodeBase(start diagnostic.Pos) ast.NodeBase {
	return ast.NodeBase{NodeSpan: p.spanFrom(start)}
}

func (p *Parser) stmtBase(start diagnostic.Pos) ast.StmtBase {
	return ast.StmtBase{NodeBase: p.nodeBase(start)}
}

func (p *Parser) exprBase(start diagnostic.Pos) ast.ExprBase {
	return ast.ExprBase{NodeBase: p.nodeBase(start)}
}

func (p *Parser) condBase(start diagnostic.Pos) ast.CondBase {
	return ast.CondBase{NodeBase: p.nodeBase(start)}
}

func (p *Parser) clauseBase(start diagnostic.Pos) ast.ClauseBase {
	return ast.ClauseBase{NodeBase: p.nodeBase(start)}
}

// Parse is the public entry point. It runs the program loop, collecting
// statements until EOF. On a statement error it synchronizes to the next
// boundary and continues, accumulating all diagnostics. Returns nil and the
// diagnostic list if any errors were found.
func (p *Parser) Parse() (*ast.Program, error) {
	start := p.currentStart()
	prog := &ast.Program{}

	for !p.check(lexer.TOKEN_EOF) {
		stmt, err := p.parseStatement()
		if err != nil {
			p.synchronize()
			continue
		}
		prog.Statements = append(prog.Statements, stmt)

		if _, err := p.expect(lexer.TOKEN_SEMICOLON); err != nil {
			p.synchronize()
		}
	}

	prog.NodeSpan = p.spanFrom(start)

	if p.Diagnostics().HasErrors() {
		return nil, p.Diagnostics().AsError()
	}
	return prog, nil
}

// Diagnostics returns the combined lexer + parser diagnostic list.
// Lexer errors appear first. Safe to call before or after Parse.
func (p *Parser) Diagnostics() diagnostic.List {
	lexDiag := p.lex.Diagnostics()
	all := make(diagnostic.List, 0, len(lexDiag)+len(p.diag))
	all = append(all, lexDiag...)
	all = append(all, p.diag...)
	return all
}

// synchronize discards tokens until a safe recovery point
func (p *Parser) synchronize() {
	for !p.check(lexer.TOKEN_EOF) {
		if p.check(lexer.TOKEN_SEMICOLON) {
			p.advance()
			return
		}
		switch p.current.Type {
		case lexer.TOKEN_SELECT, lexer.TOKEN_INSERT, lexer.TOKEN_UPDATE,
			lexer.TOKEN_DELETE, lexer.TOKEN_CREATE, lexer.TOKEN_DROP,
			lexer.TOKEN_ALTER, lexer.TOKEN_USE:
			return
		}
		p.advance()
	}
}
