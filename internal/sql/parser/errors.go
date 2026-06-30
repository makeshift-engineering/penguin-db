package parser

import (
	"fmt"

	"github.com/makeshift-engineering/penguin-db/internal/sql/diagnostic"
)

// Parser diagnostic codes occupy the 2000–2999 range.
// Each constant identifies a distinct class of syntax error.
const (
	// CodeUnexpectedToken is emitted when expect() or a branch check finds
	// a token that does not match what the grammar requires at that position.
	CodeUnexpectedToken diagnostic.Code = 2001

	// CodeUnexpectedEOF is emitted when TOKEN_EOF is encountered mid-rule.
	CodeUnexpectedEOF diagnostic.Code = 2002

	// CodeInvalidDataType is emitted when a data-type keyword is required but
	// the current token is not one of INT, BIGINT, VARCHAR, BOOLEAN, TEXT, TIMESTAMP.
	CodeInvalidDataType diagnostic.Code = 2003

	// CodeInvalidAlterAction is emitted when the token following ALTER TABLE <name>
	// is not ADD, MODIFY, RENAME, or DROP.
	CodeInvalidAlterAction diagnostic.Code = 2004

	// CodeInvalidJoinType is emitted when a JOIN keyword sequence cannot be
	// matched to any recognised join variant.
	CodeInvalidJoinType diagnostic.Code = 2005

	// CodeExpectedExpression is emitted when parseFactor cannot find any token
	// that can start an arithmetic expression.
	CodeExpectedExpression diagnostic.Code = 2006

	// CodeExpectedCondition is emitted when a condition continuation is expected
	// inside parentheses but no predicate tail or AND/OR is found.
	CodeExpectedCondition diagnostic.Code = 2007

	// CodeInvalidIntegerLiteral is emitted when an integer literal token cannot
	// be converted to int (overflow, leading sign already consumed, etc.).
	CodeInvalidIntegerLiteral diagnostic.Code = 2008

	// CodeMalformedStatement is emitted when the current token cannot start
	// any known statement, or when CREATE / DROP is not followed by DATABASE or TABLE.
	CodeMalformedStatement diagnostic.Code = 2009
)

// errorf creates a Diagnostic with SeverityError, appends it to the parser's
// diagnostic list, and returns it as an error value.
// span : the source range of the offending token(s)
// code : one of the Code constants above
// format : printf-style message template
// args : format arguments
func (p *Parser) errorf(span diagnostic.Span, code diagnostic.Code, format string, args ...any) error {
	d := &diagnostic.Diagnostic{
		Severity: diagnostic.SeverityError,
		Code:     code,
		Category: "Syntax Error",
		Span:     span,
		Msg:      fmt.Sprintf(format, args...),
		Source:   p.source,
	}
	p.diag.Append(d)
	return d
}
