package utils

import "strings"

// keywords maps upper-cased identifier text to its reserved TokenType.
// Built automatically from tokenTable at init time — no manual sync needed.
var keywords map[string]TokenType

func init() {
	keywords = make(map[string]TokenType, int(tokenTypeSentinel))
	for i := range int(tokenTypeSentinel) {
		def := tokenTable[i]
		if def.class == classKeyword {
			keywords[def.name] = TokenType(i)
		}
	}
}

// LookupIdent maps an identifier string to its TokenType.
// If s matches a reserved keyword (case-insensitively), that keyword's
// TokenType is returned. Otherwise TOKEN_IDENT is returned.
//
// This function is intended for use by the lexer's scanIdentifier method.
// It lives in utils so that the lexer can call it without defining TokenType
// itself.
func LookupIdent(s string) TokenType {
	if tt, ok := keywords[strings.ToUpper(s)]; ok {
		return tt
	}
	return TOKEN_IDENT
}
