package cwquery

import (
	"fmt"
	"strconv"
)

// Parser is a hand-written recursive-descent parser over a pre-tokenized
// query. Pre-tokenizing (see Tokenize) means arbitrary lookahead is just
// slice indexing - no lookahead-buffer bookkeeping needed.
type Parser struct {
	tokens []Token
	pos    int
}

func NewParser(input string) (*Parser, error) {
	tokens, err := Tokenize(input)
	if err != nil {
		return nil, err
	}
	return &Parser{tokens: tokens}, nil
}

func (p *Parser) cur() Token {
	return p.tokens[p.pos]
}

func (p *Parser) advance() Token {
	t := p.cur()
	if p.pos < len(p.tokens)-1 {
		p.pos++
	}
	return t
}

func (p *Parser) expectIdent(kw string) error {
	if !p.cur().IsKeyword(kw) {
		return &ParseError{Pos: p.cur().Pos, Message: fmt.Sprintf("expected %q, got %q", kw, p.cur().Literal)}
	}
	p.advance()
	return nil
}

// ParseQuery is the grammar's entry point: disambiguates a SELECT metric
// query from an ALARM/AND/OR/NOT composite expression by its first
// token - the two forms never share a legal first token, so this is a
// plain LL(1) dispatch, not a real ambiguity.
func ParseQuery(input string) (*Query, error) {
	p, err := NewParser(input)
	if err != nil {
		return nil, err
	}
	q, err := p.parseQuery()
	if err != nil {
		return nil, err
	}
	if p.cur().Type != TokenEOF {
		return nil, &ParseError{Pos: p.cur().Pos, Message: fmt.Sprintf("unexpected trailing input %q", p.cur().Literal)}
	}
	return q, nil
}

func (p *Parser) parseQuery() (*Query, error) {
	cur := p.cur()
	if cur.IsKeyword("SELECT") {
		mq, err := p.parseSelectQuery()
		if err != nil {
			return nil, err
		}
		return &Query{Kind: "metric", Metric: mq}, nil
	}
	if cur.IsKeyword("ALARM") || cur.IsKeyword("NOT") || cur.Type == TokenLParen {
		ce, err := p.parseCompositeExpr()
		if err != nil {
			return nil, err
		}
		return &Query{Kind: "composite", Composite: ce}, nil
	}
	return nil, &ParseError{Pos: cur.Pos, Message: fmt.Sprintf("expected SELECT or ALARM/NOT/'(', got %q", cur.Literal)}
}

// ---- Tiers 1/2/4: SELECT ... FROM ... WHERE ... HAVING ... ----

func (p *Parser) parseSelectQuery() (*MetricQuery, error) {
	if err := p.expectIdent("SELECT"); err != nil {
		return nil, err
	}
	sel, err := p.parseSelectList()
	if err != nil {
		return nil, err
	}
	if err := p.expectIdent("FROM"); err != nil {
		return nil, err
	}
	ns, err := p.parseNamespace()
	if err != nil {
		return nil, err
	}
	if err := p.expectIdent("WHERE"); err != nil {
		return nil, err
	}
	where, err := p.parseWhereClause()
	if err != nil {
		return nil, err
	}
	if err := p.expectIdent("HAVING"); err != nil {
		return nil, err
	}
	having, err := p.parseHavingClause()
	if err != nil {
		return nil, err
	}
	return &MetricQuery{Select: sel, Namespace: ns, Where: where, Having: having}, nil
}

func (p *Parser) parseNamespace() (string, error) {
	left := p.cur()
	if left.Type != TokenIdent {
		return "", &ParseError{Pos: left.Pos, Message: "expected namespace segment"}
	}
	p.advance()
	if p.cur().Type != TokenSlash {
		return "", &ParseError{Pos: p.cur().Pos, Message: "expected '/' in namespace"}
	}
	p.advance()
	right := p.cur()
	if right.Type != TokenIdent {
		return "", &ParseError{Pos: right.Pos, Message: "expected namespace segment after '/'"}
	}
	p.advance()
	return left.Literal + "/" + right.Literal, nil
}

func (p *Parser) parseWhereClause() ([]DimPredicate, error) {
	var preds []DimPredicate
	for {
		dimTok := p.cur()
		if dimTok.Type != TokenIdent {
			return nil, &ParseError{Pos: dimTok.Pos, Message: "expected dimension name"}
		}
		p.advance()
		if p.cur().Type != TokenEq {
			return nil, &ParseError{Pos: p.cur().Pos, Message: "expected '=' in WHERE predicate"}
		}
		p.advance()
		valTok := p.cur()
		if valTok.Type != TokenString {
			return nil, &ParseError{Pos: valTok.Pos, Message: "expected string literal dimension value"}
		}
		p.advance()
		preds = append(preds, DimPredicate{Dimension: dimTok.Literal, Value: valTok.Literal})
		if p.cur().IsKeyword("AND") {
			p.advance()
			continue
		}
		break
	}
	return preds, nil
}

// parseSelectList is the one genuinely ambiguous production: a bare
// leading identifier could be a tier-1 metric name or the first operand of
// a tier-2 math expression. Resolved by peeking one token past it.
func (p *Parser) parseSelectList() (SelectClause, error) {
	if p.cur().Type == TokenLParen {
		expr, err := p.parseMathExpr()
		if err != nil {
			return SelectClause{}, err
		}
		return p.finishMathSelect(expr)
	}

	identTok := p.cur()
	if identTok.Type != TokenIdent {
		return SelectClause{}, &ParseError{Pos: identTok.Pos, Message: "expected metric name or expression in SELECT"}
	}
	p.advance()

	switch p.cur().Type {
	case TokenLParen:
		// stat-call: p99(Latency)
		p.advance()
		metricTok := p.cur()
		if metricTok.Type != TokenIdent {
			return SelectClause{}, &ParseError{Pos: metricTok.Pos, Message: "expected metric name inside stat call"}
		}
		p.advance()
		if p.cur().Type != TokenRParen {
			return SelectClause{}, &ParseError{Pos: p.cur().Pos, Message: "expected ')' after stat call argument"}
		}
		p.advance()
		return SelectClause{StatCall: &StatCallExpr{Stat: identTok.Literal, Metric: metricTok.Literal}}, nil

	case TokenPlus, TokenMinus, TokenStar, TokenSlash:
		left := Expr(MetricRef{Name: identTok.Literal})
		expr, err := p.continueMathExpr(left)
		if err != nil {
			return SelectClause{}, err
		}
		return p.finishMathSelect(expr)

	case TokenIdent:
		if p.cur().IsKeyword("FROM") {
			name := identTok.Literal
			return SelectClause{BareMetric: &name}, nil
		}
		return SelectClause{}, &ParseError{Pos: p.cur().Pos, Message: fmt.Sprintf("unexpected identifier %q after metric name in SELECT", p.cur().Literal)}

	default:
		return SelectClause{}, &ParseError{Pos: p.cur().Pos, Message: fmt.Sprintf("unexpected token %q after metric name in SELECT", p.cur().Literal)}
	}
}

func (p *Parser) finishMathSelect(expr Expr) (SelectClause, error) {
	if err := p.expectIdent("AS"); err != nil {
		return SelectClause{}, err
	}
	aliasTok := p.cur()
	if aliasTok.Type != TokenIdent {
		return SelectClause{}, &ParseError{Pos: aliasTok.Pos, Message: "expected alias identifier after AS"}
	}
	p.advance()
	return SelectClause{MathExpr: expr, Alias: aliasTok.Literal}, nil
}

func (p *Parser) parseHavingClause() (HavingClause, error) {
	refTok := p.cur()
	if refTok.Type != TokenIdent {
		return HavingClause{}, &ParseError{Pos: refTok.Pos, Message: "expected value reference in HAVING"}
	}
	p.advance()

	cur := p.cur()
	switch cur.Type {
	case TokenGt, TokenGte, TokenLt, TokenLte, TokenEq, TokenNeq:
		p.advance()
		numTok := p.cur()
		if numTok.Type != TokenNumber {
			return HavingClause{}, &ParseError{Pos: numTok.Pos, Message: "expected number after comparator"}
		}
		p.advance()
		v, err := strconv.ParseFloat(numTok.Literal, 64)
		if err != nil {
			return HavingClause{}, &ParseError{Pos: numTok.Pos, Message: fmt.Sprintf("invalid number %q", numTok.Literal)}
		}
		return HavingClause{ValueRef: refTok.Literal, Threshold: &ThresholdPred{Comparator: cur.Literal, Value: v}}, nil

	case TokenIdent:
		var direction string
		switch {
		case cur.IsKeyword("OUTSIDE"):
			direction = "OUTSIDE"
		case cur.IsKeyword("ABOVE"):
			direction = "ABOVE"
		case cur.IsKeyword("BELOW"):
			direction = "BELOW"
		default:
			return HavingClause{}, &ParseError{Pos: cur.Pos, Message: fmt.Sprintf("expected comparator or OUTSIDE/ABOVE/BELOW, got %q", cur.Literal)}
		}
		p.advance()
		if err := p.expectIdent("BAND"); err != nil {
			return HavingClause{}, err
		}
		if p.cur().Type != TokenLParen {
			return HavingClause{}, &ParseError{Pos: p.cur().Pos, Message: "expected '(' after BAND"}
		}
		p.advance()
		numTok := p.cur()
		if numTok.Type != TokenNumber {
			return HavingClause{}, &ParseError{Pos: numTok.Pos, Message: "expected number inside BAND(...)"}
		}
		p.advance()
		n, err := strconv.ParseFloat(numTok.Literal, 64)
		if err != nil {
			return HavingClause{}, &ParseError{Pos: numTok.Pos, Message: fmt.Sprintf("invalid number %q", numTok.Literal)}
		}
		if p.cur().Type != TokenRParen {
			return HavingClause{}, &ParseError{Pos: p.cur().Pos, Message: "expected ')' after BAND(n"}
		}
		p.advance()
		return HavingClause{ValueRef: refTok.Literal, Band: &BandPred{Direction: direction, N: n}}, nil
	}

	return HavingClause{}, &ParseError{Pos: cur.Pos, Message: fmt.Sprintf("expected comparator or OUTSIDE/ABOVE/BELOW after value reference, got %q", cur.Literal)}
}

// ---- Metric-math expressions (standard precedence climbing) ----

func (p *Parser) parseMathExpr() (Expr, error) {
	left, err := p.parseMathTerm()
	if err != nil {
		return nil, err
	}
	return p.continueMathExprFromTerm(left)
}

// continueMathExpr continues parsing given that `left` is an
// already-consumed leading factor (used when parseSelectList had to
// consume one identifier before discovering it was the start of a math
// expression, not a bare metric name).
func (p *Parser) continueMathExpr(left Expr) (Expr, error) {
	left, err := p.continueMathTerm(left)
	if err != nil {
		return nil, err
	}
	return p.continueMathExprFromTerm(left)
}

func (p *Parser) continueMathExprFromTerm(left Expr) (Expr, error) {
	for p.cur().Type == TokenPlus || p.cur().Type == TokenMinus {
		op := p.advance().Literal
		right, err := p.parseMathTerm()
		if err != nil {
			return nil, err
		}
		left = BinaryExpr{Op: op, Left: left, Right: right}
	}
	return left, nil
}

func (p *Parser) parseMathTerm() (Expr, error) {
	left, err := p.parseMathFactor()
	if err != nil {
		return nil, err
	}
	return p.continueMathTerm(left)
}

func (p *Parser) continueMathTerm(left Expr) (Expr, error) {
	for p.cur().Type == TokenStar || p.cur().Type == TokenSlash {
		op := p.advance().Literal
		right, err := p.parseMathFactor()
		if err != nil {
			return nil, err
		}
		left = BinaryExpr{Op: op, Left: left, Right: right}
	}
	return left, nil
}

func (p *Parser) parseMathFactor() (Expr, error) {
	tok := p.cur()
	switch tok.Type {
	case TokenNumber:
		p.advance()
		v, err := strconv.ParseFloat(tok.Literal, 64)
		if err != nil {
			return nil, &ParseError{Pos: tok.Pos, Message: fmt.Sprintf("invalid number %q", tok.Literal)}
		}
		return NumberLit{Value: v}, nil
	case TokenIdent:
		p.advance()
		return MetricRef{Name: tok.Literal}, nil
	case TokenLParen:
		p.advance()
		expr, err := p.parseMathExpr()
		if err != nil {
			return nil, err
		}
		if p.cur().Type != TokenRParen {
			return nil, &ParseError{Pos: p.cur().Pos, Message: "expected ')'"}
		}
		p.advance()
		return expr, nil
	}
	return nil, &ParseError{Pos: tok.Pos, Message: fmt.Sprintf("unexpected token %q in expression", tok.Literal)}
}

// ---- Tier 3: ALARM/AND/OR/NOT composite expressions ----

func (p *Parser) parseCompositeExpr() (CompositeExpr, error) {
	left, err := p.parseCompositeTerm()
	if err != nil {
		return nil, err
	}
	for p.cur().IsKeyword("OR") {
		p.advance()
		right, err := p.parseCompositeTerm()
		if err != nil {
			return nil, err
		}
		left = OrExpr{Left: left, Right: right}
	}
	return left, nil
}

func (p *Parser) parseCompositeTerm() (CompositeExpr, error) {
	left, err := p.parseCompositeFactor()
	if err != nil {
		return nil, err
	}
	for p.cur().IsKeyword("AND") {
		p.advance()
		right, err := p.parseCompositeFactor()
		if err != nil {
			return nil, err
		}
		left = AndExpr{Left: left, Right: right}
	}
	return left, nil
}

func (p *Parser) parseCompositeFactor() (CompositeExpr, error) {
	cur := p.cur()
	switch {
	case cur.IsKeyword("NOT"):
		p.advance()
		inner, err := p.parseCompositeFactor()
		if err != nil {
			return nil, err
		}
		return NotExpr{X: inner}, nil
	case cur.Type == TokenLParen:
		p.advance()
		inner, err := p.parseCompositeExpr()
		if err != nil {
			return nil, err
		}
		if p.cur().Type != TokenRParen {
			return nil, &ParseError{Pos: p.cur().Pos, Message: "expected ')'"}
		}
		p.advance()
		return inner, nil
	case cur.IsKeyword("ALARM"):
		p.advance()
		if p.cur().Type != TokenLParen {
			return nil, &ParseError{Pos: p.cur().Pos, Message: "expected '(' after ALARM"}
		}
		p.advance()
		nameTok := p.cur()
		if nameTok.Type != TokenString {
			return nil, &ParseError{Pos: nameTok.Pos, Message: "expected string literal alarm name inside ALARM(...)"}
		}
		p.advance()
		if p.cur().Type != TokenRParen {
			return nil, &ParseError{Pos: p.cur().Pos, Message: "expected ')' after ALARM(name"}
		}
		p.advance()
		return AlarmRef{Name: nameTok.Literal}, nil
	}
	return nil, &ParseError{Pos: cur.Pos, Message: fmt.Sprintf("expected ALARM(...), NOT, or '(', got %q", cur.Literal)}
}
