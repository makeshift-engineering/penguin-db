package parser

import (
	"github.com/makeshift-engineering/penguin-db/internal/sql/ast"
	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
	"github.com/makeshift-engineering/penguin-db/internal/sql/utils"
)

// Parser is a single-pass recursive descent parser.
type Parser struct {
	tokens  *utils.LookaheadIterator[utils.Token]
	current utils.Token
	diag    diagnostic.List
	source  *diagnostic.Source
}

// New creates a Parser over the given pre-lexed token slice.
//
// tokens must be the slice produced by lexer.Tokenize() — it must end with a
// TOKEN_EOF entry. The lexer's diagnostic list (lex errors) should be merged
// with the parser's own list by the caller after parsing; the parser itself
// has no reference to the lexer.
//
// One advance() call primes p.current with the first token. The iterator
// lazily buffers the second token on the first Peek() call.
func New(tokens []utils.Token, src *diagnostic.Source) *Parser {
	// Build a closure over the slice that acts as a token source.
	// Once the slice is exhausted the closure returns TOKEN_EOF repeatedly
	// (the lexer already appended one, so this only triggers if the caller
	// reads past it — defensive behaviour only).
	i := 0
	next := func() utils.Token {
		if i >= len(tokens) {
			return utils.Token{Type: utils.TOKEN_EOF}
		}
		tok := tokens[i]
		i++
		return tok
	}

	p := &Parser{
		tokens: utils.NewLookaheadIterator[utils.Token](next),
		source: src,
	}
	p.advance() // prime current with the first token
	return p
}

// advance consumes the next token from the iterator into p.current.
// This is the only place in the parser that reads from the iterator.
func (p *Parser) advance() {
	p.current = p.tokens.Next()
}

// check reports whether the current token has the given type. Does not consume.
func (p *Parser) check(t utils.TokenType) bool {
	return p.current.Type == t
}

// peekIs reports whether the lookahead token has the given type. Does not consume.
func (p *Parser) peekIs(t utils.TokenType) bool {
	return p.tokens.Peek().Type == t
}

// match consumes current and returns true if it matches any of the given types.
// Returns false without consuming if nothing matches.
func (p *Parser) match(types ...utils.TokenType) bool {
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
func (p *Parser) expect(t utils.TokenType) (utils.Token, error) {
	if !p.check(t) {
		return utils.Token{}, p.errorf(
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
// Keywords are intentionally rejected — `CREATE TABLE select (…)` is an error.
func (p *Parser) expectIdent() (string, error) {
	tok, err := p.expect(utils.TOKEN_IDENT)
	if err != nil {
		return "", err
	}
	return tok.Literal, nil
}

// currentStart returns the start position of the current (not-yet-consumed)
// token. Must be called as the very first line of every parse function.
func (p *Parser) currentStart() diagnostic.Pos {
	return p.current.Span.Start
}

// spanFrom builds a [Start, End) Span covering from start through the end
// of the last consumed token. Call after the final advance/expect in a rule.
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

// Parse runs the statement loop until TOKEN_EOF. Errors within a statement
// trigger synchronization so subsequent statements can still be parsed.
// Returns nil and the diagnostic list if any error was found.
func (p *Parser) Parse() (*ast.Program, error) {
	start := p.currentStart()
	prog := &ast.Program{}

	for !p.check(utils.TOKEN_EOF) {
		stmt, err := p.parseStatement()
		if err != nil {
			p.synchronize()
			continue
		}
		prog.Statements = append(prog.Statements, stmt)

		if _, err := p.expect(utils.TOKEN_SEMICOLON); err != nil {
			p.synchronize()
		}
	}

	prog.NodeSpan = p.spanFrom(start)

	if p.diag.HasErrors() {
		return nil, p.diag.AsError()
	}
	return prog, nil
}

// Diagnostics returns the parser-level diagnostic list.
// The caller is responsible for merging this with the lexer's diagnostic list
// to get a complete picture of all errors.
func (p *Parser) Diagnostics() diagnostic.List {
	return p.diag
}

// synchronize discards tokens until a safe recovery boundary:
//   - TOKEN_SEMICOLON  → consumed, return (outer loop won't re-consume it)
//   - Statement keyword → NOT consumed (outer loop will parse the next statement)
//   - TOKEN_EOF        → stop
func (p *Parser) synchronize() {
	for !p.check(utils.TOKEN_EOF) {
		if p.check(utils.TOKEN_SEMICOLON) {
			p.advance()
			return
		}
		switch p.current.Type {
		case utils.TOKEN_SELECT, utils.TOKEN_INSERT, utils.TOKEN_UPDATE,
			utils.TOKEN_DELETE, utils.TOKEN_CREATE, utils.TOKEN_DROP,
			utils.TOKEN_ALTER, utils.TOKEN_USE:
			return
		}
		p.advance()
	}
}
