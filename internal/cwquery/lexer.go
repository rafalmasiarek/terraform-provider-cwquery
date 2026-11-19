package cwquery

import (
	"fmt"
	"strings"
	"unicode"
)

// Lexer scans a cwquery string into Tokens one at a time. It has no
// knowledge of the grammar - keyword recognition happens later, in the
// parser, via Token.IsKeyword.
type Lexer struct {
	input []rune
	pos   int
}

func NewLexer(input string) *Lexer {
	return &Lexer{input: []rune(input)}
}

func (l *Lexer) peek() rune {
	if l.pos >= len(l.input) {
		return 0
	}
	return l.input[l.pos]
}

func (l *Lexer) peekAt(offset int) rune {
	p := l.pos + offset
	if p >= len(l.input) {
		return 0
	}
	return l.input[p]
}

func (l *Lexer) advance() rune {
	r := l.peek()
	l.pos++
	return r
}

func (l *Lexer) skipWhitespace() {
	for {
		switch l.peek() {
		case ' ', '\t', '\n', '\r':
			l.advance()
		default:
			return
		}
	}
}

// Next returns the next token, advancing past it. Returns a TokenEOF token
// (never an error) once the input is exhausted.
func (l *Lexer) Next() (Token, error) {
	l.skipWhitespace()
	start := l.pos
	r := l.peek()

	if r == 0 {
		return Token{Type: TokenEOF, Pos: start}, nil
	}

	switch {
	case unicode.IsLetter(r) || r == '_':
		for unicode.IsLetter(l.peek()) || unicode.IsDigit(l.peek()) || l.peek() == '_' {
			l.advance()
		}
		return Token{Type: TokenIdent, Literal: string(l.input[start:l.pos]), Pos: start}, nil

	case unicode.IsDigit(r):
		for unicode.IsDigit(l.peek()) {
			l.advance()
		}
		if l.peek() == '.' && unicode.IsDigit(l.peekAt(1)) {
			l.advance()
			for unicode.IsDigit(l.peek()) {
				l.advance()
			}
		}
		if l.peek() == 'e' || l.peek() == 'E' {
			save := l.pos
			l.advance()
			if l.peek() == '+' || l.peek() == '-' {
				l.advance()
			}
			if unicode.IsDigit(l.peek()) {
				for unicode.IsDigit(l.peek()) {
					l.advance()
				}
			} else {
				l.pos = save // not a real exponent (e.g. a trailing bare 'e') - back off
			}
		}
		return Token{Type: TokenNumber, Literal: string(l.input[start:l.pos]), Pos: start}, nil

	case r == '\'' || r == '"':
		quote := r
		l.advance()
		var sb strings.Builder
		for {
			c := l.peek()
			if c == 0 {
				return Token{}, &ParseError{Pos: start, Message: "unterminated string literal"}
			}
			if c == quote {
				l.advance()
				break
			}
			sb.WriteRune(c)
			l.advance()
		}
		return Token{Type: TokenString, Literal: sb.String(), Pos: start}, nil

	case r == '/':
		l.advance()
		return Token{Type: TokenSlash, Literal: "/", Pos: start}, nil
	case r == '+':
		l.advance()
		return Token{Type: TokenPlus, Literal: "+", Pos: start}, nil
	case r == '-':
		l.advance()
		return Token{Type: TokenMinus, Literal: "-", Pos: start}, nil
	case r == '*':
		l.advance()
		return Token{Type: TokenStar, Literal: "*", Pos: start}, nil
	case r == '(':
		l.advance()
		return Token{Type: TokenLParen, Literal: "(", Pos: start}, nil
	case r == ')':
		l.advance()
		return Token{Type: TokenRParen, Literal: ")", Pos: start}, nil
	case r == ',':
		l.advance()
		return Token{Type: TokenComma, Literal: ",", Pos: start}, nil
	case r == '=':
		l.advance()
		return Token{Type: TokenEq, Literal: "=", Pos: start}, nil
	case r == '!':
		l.advance()
		if l.peek() == '=' {
			l.advance()
			return Token{Type: TokenNeq, Literal: "!=", Pos: start}, nil
		}
		return Token{}, &ParseError{Pos: start, Message: "unexpected character '!'"}
	case r == '>':
		l.advance()
		if l.peek() == '=' {
			l.advance()
			return Token{Type: TokenGte, Literal: ">=", Pos: start}, nil
		}
		return Token{Type: TokenGt, Literal: ">", Pos: start}, nil
	case r == '<':
		l.advance()
		if l.peek() == '=' {
			l.advance()
			return Token{Type: TokenLte, Literal: "<=", Pos: start}, nil
		}
		return Token{Type: TokenLt, Literal: "<", Pos: start}, nil
	}

	return Token{}, &ParseError{Pos: start, Message: fmt.Sprintf("unexpected character %q", r)}
}

// Tokenize lexes the entire input up front (queries are short, so there's
// no benefit to streaming) into a slice ending in exactly one TokenEOF -
// this lets Parser use plain slice indexing with arbitrary lookahead
// instead of a hand-rolled lookahead buffer.
func Tokenize(input string) ([]Token, error) {
	lex := NewLexer(input)
	var tokens []Token
	for {
		tok, err := lex.Next()
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, tok)
		if tok.Type == TokenEOF {
			break
		}
	}
	return tokens, nil
}
