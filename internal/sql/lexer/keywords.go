package lexer

import "strings"

// keywords is built from tokenTable at init time.
// No manual sync needed — keywords updates itself.
var keywords map[string]TokenType

func init() {
	keywords = make(map[string]TokenType, int(tokenTypeSentinel))
	for i := 0; i < int(tokenTypeSentinel); i++ {
		def := tokenTable[i]
		if def.class == classKeyword {
			keywords[def.name] = TokenType(i)
		}
	}
}

// lookupIdent returns the keyword TokenType for s if it is a reserved word,
// or TOKEN_IDENT if it is a plain user-defined name.
// The comparison is case-insensitive: "select", "SELECT", and "SeLeCt" all
// resolve to TOKEN_SELECT.
func lookupIdent(s string) TokenType {
	if tt, ok := keywords[strings.ToUpper(s)]; ok {
		return tt
	}
	return TOKEN_IDENT
}
