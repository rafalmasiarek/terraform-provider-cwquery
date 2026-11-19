package cwquery

import "strings"

// TokenType enumerates the lexical categories cwquery's grammar needs.
// Keywords (SELECT, FROM, AND, ALARM, ...) are never a distinct token
// type - they share the identifier charset with metric/dimension names, so
// they're recognized by value (see Token.IsKeyword), not by the lexer.
type TokenType int

const (
	TokenEOF TokenType = iota
	TokenIdent
	TokenNumber
	TokenString
	TokenSlash
	TokenEq
	TokenNeq
	TokenGt
	TokenGte
	TokenLt
	TokenLte
	TokenPlus
	TokenMinus
	TokenStar
	TokenLParen
	TokenRParen
	TokenComma
)

// Token is one lexical unit. Pos is a byte offset into the original query
// string, carried through to ParseError so error messages can point at the
// exact offending position.
type Token struct {
	Type    TokenType
	Literal string
	Pos     int
}

// IsKeyword reports whether this token is an identifier matching kw,
// case-insensitively.
func (t Token) IsKeyword(kw string) bool {
	return t.Type == TokenIdent && strings.EqualFold(t.Literal, kw)
}
