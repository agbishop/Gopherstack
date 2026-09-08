package odatatable

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

// filter.go implements a hand-written lexer and recursive-descent parser
// for OData's $filter mini-language, modeled on services/dynamodb/expr's
// lexer/parser/evaluator split (see lexer.go, parser.go, ast.go,
// evaluator.go there). eval.go holds the evaluator.
//
// Supported grammar:
//
//	expr       := orExpr
//	orExpr     := andExpr ('or' andExpr)*
//	andExpr    := unary ('and' unary)*
//	unary      := 'not' unary | primary
//	primary    := '(' expr ')' | comparison
//	comparison := operand ('eq'|'ne'|'lt'|'le'|'gt'|'ge') operand
//	operand    := identifier | literal
//	literal    := 'quoted string' (with '' escape) | integer | integer'L' | float | true | false
//	            | datetime'<RFC3339>' | guid'<uuid>' | X'<hex>' | binary'<base64>'

// tokenType enumerates the lexical token kinds $filter expressions use.
type tokenType int

const (
	tEOF tokenType = iota
	tError
	tIdent
	tString
	tInt
	tInt64
	tFloat
	tBool
	tDateTime
	tGUID
	tBinary
	tLParen
	tRParen
	tAnd
	tOr
	tNot
	tEq
	tNe
	tLt
	tLe
	tGt
	tGe
)

// token is one lexical token. enc distinguishes Binary literals' source
// encoding ('x' for X'<hex>', 'b' for binary'<base64>'); it is meaningless
// for every other token type.
type token struct {
	lit string
	typ tokenType
	enc byte
}

// lexer tokenizes a $filter expression string.
type lexer struct {
	input string
	pos   int
}

func newLexer(s string) *lexer { return &lexer{input: s} }

func (l *lexer) next() token {
	l.skipSpace()

	if l.pos >= len(l.input) {
		return token{typ: tEOF}
	}

	ch := l.input[l.pos]

	switch {
	case ch == '(':
		l.pos++

		return token{typ: tLParen, lit: "("}
	case ch == ')':
		l.pos++

		return token{typ: tRParen, lit: ")"}
	case ch == '\'':
		content, ok := l.readQuotedContent()
		if !ok {
			return token{typ: tError, lit: "unterminated string literal"}
		}

		return token{typ: tString, lit: content}
	case ch == '-' || isDigit(ch):
		return l.readNumber()
	case isAlpha(ch):
		return l.readWord()
	default:
		l.pos++

		return token{typ: tError, lit: string(ch)}
	}
}

func (l *lexer) skipSpace() {
	for l.pos < len(l.input) && (l.input[l.pos] == ' ' || l.input[l.pos] == '\t') {
		l.pos++
	}
}

// readQuotedContent consumes a '...'-delimited literal (escaped by doubling:
// a single quote written twice in a row means one literal quote) starting at
// l.pos, which must point at the opening quote. Returns the unescaped
// content and true, or ("", false) if the input ends before a closing quote
// is found.
func (l *lexer) readQuotedContent() (string, bool) {
	if l.pos >= len(l.input) || l.input[l.pos] != '\'' {
		return "", false
	}

	l.pos++

	buf := make([]byte, 0, len(l.input)-l.pos)

	for {
		if l.pos >= len(l.input) {
			return "", false
		}

		c := l.input[l.pos]

		if c == '\'' {
			if l.pos+1 < len(l.input) && l.input[l.pos+1] == '\'' {
				buf = append(buf, '\'')
				l.pos += 2

				continue
			}

			l.pos++

			return string(buf), true
		}

		buf = append(buf, c)
		l.pos++
	}
}

// readNumber reads an integer or floating-point literal, including an
// optional leading '-' and an optional trailing 'L'/'l' Int64 suffix on
// integers.
func (l *lexer) readNumber() token {
	start := l.pos
	if l.input[l.pos] == '-' {
		l.pos++
	}

	for l.pos < len(l.input) && isDigit(l.input[l.pos]) {
		l.pos++
	}

	isFloat := false

	if l.pos < len(l.input) && l.input[l.pos] == '.' && l.pos+1 < len(l.input) && isDigit(l.input[l.pos+1]) {
		isFloat = true
		l.pos++

		for l.pos < len(l.input) && isDigit(l.input[l.pos]) {
			l.pos++
		}
	}

	numStr := l.input[start:l.pos]

	if !isFloat && l.pos < len(l.input) && (l.input[l.pos] == 'L' || l.input[l.pos] == 'l') {
		l.pos++

		return token{typ: tInt64, lit: numStr}
	}

	if isFloat {
		return token{typ: tFloat, lit: numStr}
	}

	return token{typ: tInt, lit: numStr}
}

// readWord reads an identifier-shaped word and classifies it as a keyword
// (and/or/not/eq/ne/lt/le/gt/ge), a boolean literal, a prefixed literal
// (datetime'...'/guid'...'/X'.../binary'...'), or a plain property
// identifier.
func (l *lexer) readWord() token {
	start := l.pos
	for l.pos < len(l.input) && isAlnum(l.input[l.pos]) {
		l.pos++
	}

	word := l.input[start:l.pos]

	if tok, ok := keywordToken(word); ok {
		return tok
	}

	if l.pos < len(l.input) && l.input[l.pos] == '\'' {
		if tok, ok := l.prefixedLiteral(word); ok {
			return tok
		}
	}

	return token{typ: tIdent, lit: word}
}

// keywordToken returns the token for a reserved word (case-sensitive, per
// OData: operator/logical keywords and true/false are lowercase).
func keywordToken(word string) (token, bool) {
	switch word {
	case "and":
		return token{typ: tAnd}, true
	case "or":
		return token{typ: tOr}, true
	case "not":
		return token{typ: tNot}, true
	case "eq":
		return token{typ: tEq}, true
	case "ne":
		return token{typ: tNe}, true
	case "lt":
		return token{typ: tLt}, true
	case "le":
		return token{typ: tLe}, true
	case "gt":
		return token{typ: tGt}, true
	case "ge":
		return token{typ: tGe}, true
	case "true", "false":
		return token{typ: tBool, lit: word}, true
	default:
		return token{}, false
	}
}

// prefixedLiteral handles the datetime'...'/guid'...'/binary'...'/X'...'
// literal forms, which are a keyword word immediately followed by a quoted
// literal with no space.
func (l *lexer) prefixedLiteral(word string) (token, bool) {
	var typ tokenType

	var enc byte

	switch word {
	case "datetime":
		typ = tDateTime
	case "guid":
		typ = tGUID
	case "binary":
		typ, enc = tBinary, 'b'
	case "X":
		typ, enc = tBinary, 'x'
	default:
		return token{}, false
	}

	content, ok := l.readQuotedContent()
	if !ok {
		return token{typ: tError, lit: "unterminated string literal"}, true
	}

	return token{typ: typ, lit: content, enc: enc}, true
}

func isDigit(ch byte) bool { return ch >= '0' && ch <= '9' }
func isAlpha(ch byte) bool {
	return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch == '_'
}
func isAlnum(ch byte) bool { return isAlpha(ch) || isDigit(ch) }

// --- AST ---

// Node is a node in a parsed $filter expression tree.
type Node interface{ isFilterNode() }

type andNode struct{ left, right Node }

func (*andNode) isFilterNode() {}

type orNode struct{ left, right Node }

func (*orNode) isFilterNode() {}

type notNode struct{ expr Node }

func (*notNode) isFilterNode() {}

type cmpNode struct {
	left, right operand
	op          tokenType
}

func (*cmpNode) isFilterNode() {}

// operand is either a property-identifier reference or a typed literal
// value. Exactly one of isIdent or a literal field-set applies, selected by
// litType when !isIdent.
type operand struct {
	ident    string
	strVal   string
	timeVal  time.Time
	bytesVal []byte
	intVal   int64
	floatVal float64
	litType  tokenType
	isIdent  bool
	isInt32  bool
	boolVal  bool
}

// --- Parser ---

// maxFilterDepth bounds recursive-descent recursion so a deeply nested (or
// adversarial) $filter string fails with a parse error instead of
// overflowing the stack -- an unbounded recursive-descent parser is a
// stack-overflow DoS.
const maxFilterDepth = 100

// Parser is a recursive-descent parser for $filter expressions.
type Parser struct {
	lx  *lexer
	cur token
}

// NewParser creates a Parser over a $filter expression string.
func NewParser(s string) *Parser {
	p := &Parser{lx: newLexer(s)}
	p.advance()

	return p
}

func (p *Parser) advance() { p.cur = p.lx.next() }

// ParseFilter parses a complete $filter expression string into a Node tree.
// Returns ErrFilterParse (wrapped with detail) on any malformed input,
// ErrFilterTooDeep if nesting exceeds maxFilterDepth, and never panics.
func ParseFilter(s string) (Node, error) {
	p := NewParser(s)

	node, err := p.parseOr(0)
	if err != nil {
		return nil, err
	}

	if p.cur.typ == tError {
		return nil, fmt.Errorf("%w: %s", ErrFilterParse, p.cur.lit)
	}

	if p.cur.typ != tEOF {
		return nil, fmt.Errorf("%w: unexpected trailing input %q", ErrFilterParse, p.cur.lit)
	}

	return node, nil
}

func checkDepth(depth int) error {
	if depth > maxFilterDepth {
		return ErrFilterTooDeep
	}

	return nil
}

// parseOr, parseAnd, parseUnary, and parsePrimary form one precedence-
// climbing layer of recursive descent per grammar rule (see this file's top
// doc comment), not one nesting level each: "Age eq 1" with no parentheses
// or "not" anywhere still passes through all four on its way to
// parseComparison. depth must therefore be threaded through UNCHANGED across
// these routine same-level calls, and incremented ONLY at the two places
// genuine nesting actually happens -- parseUnary's "not" branch and
// parsePrimary's "(...)" branch, both of which recurse back into a
// lower-precedence rule. Incrementing depth on every layer instead meant
// maxFilterDepth=100 was actually reached by roughly 25 nested parentheses,
// not 100 -- a bound that doesn't mean what its name says is barely better
// than no bound at all.
func (p *Parser) parseOr(depth int) (Node, error) {
	if err := checkDepth(depth); err != nil {
		return nil, err
	}

	left, err := p.parseAnd(depth)
	if err != nil {
		return nil, err
	}

	for p.cur.typ == tOr {
		p.advance()

		right, andErr := p.parseAnd(depth)
		if andErr != nil {
			return nil, andErr
		}

		left = &orNode{left: left, right: right}
	}

	return left, nil
}

func (p *Parser) parseAnd(depth int) (Node, error) {
	if err := checkDepth(depth); err != nil {
		return nil, err
	}

	left, err := p.parseUnary(depth)
	if err != nil {
		return nil, err
	}

	for p.cur.typ == tAnd {
		p.advance()

		right, unaryErr := p.parseUnary(depth)
		if unaryErr != nil {
			return nil, unaryErr
		}

		left = &andNode{left: left, right: right}
	}

	return left, nil
}

func (p *Parser) parseUnary(depth int) (Node, error) {
	if err := checkDepth(depth); err != nil {
		return nil, err
	}

	if p.cur.typ == tNot {
		p.advance()

		// Genuine recursion: "not" wraps another full unary expression, so
		// this is one real nesting level deeper.
		inner, err := p.parseUnary(depth + 1)
		if err != nil {
			return nil, err
		}

		return &notNode{expr: inner}, nil
	}

	return p.parsePrimary(depth)
}

func (p *Parser) parsePrimary(depth int) (Node, error) {
	if err := checkDepth(depth); err != nil {
		return nil, err
	}

	if p.cur.typ == tLParen {
		p.advance()

		// Genuine recursion: entering "(...)" starts a whole new
		// lowest-precedence sub-expression, so this is one real nesting
		// level deeper -- see the doc comment above parseOr.
		node, err := p.parseOr(depth + 1)
		if err != nil {
			return nil, err
		}

		if p.cur.typ != tRParen {
			return nil, fmt.Errorf("%w: expected ) got %q", ErrFilterParse, p.cur.lit)
		}

		p.advance()

		return node, nil
	}

	return p.parseComparison()
}

func (p *Parser) parseComparison() (Node, error) {
	left, err := p.parseOperand()
	if err != nil {
		return nil, err
	}

	if !isCompareOp(p.cur.typ) {
		return nil, fmt.Errorf("%w: expected comparison operator, got %q", ErrFilterParse, p.cur.lit)
	}

	op := p.cur.typ
	p.advance()

	right, err := p.parseOperand()
	if err != nil {
		return nil, err
	}

	return &cmpNode{left: left, right: right, op: op}, nil
}

func isCompareOp(t tokenType) bool {
	switch t {
	case tEq, tNe, tLt, tLe, tGt, tGe:
		return true
	default:
		return false
	}
}

//nolint:cyclop // straightforward per-token-type dispatch; splitting would obscure it
func (p *Parser) parseOperand() (operand, error) {
	tok := p.cur

	switch tok.typ {
	case tIdent:
		p.advance()

		return operand{isIdent: true, ident: tok.lit}, nil
	case tString:
		p.advance()

		return operand{litType: tString, strVal: tok.lit}, nil
	case tInt:
		p.advance()

		n, err := strconv.ParseInt(tok.lit, 10, 32)
		if err != nil {
			return operand{}, fmt.Errorf("%w: invalid integer literal %q", ErrFilterParse, tok.lit)
		}

		return operand{litType: tInt, isInt32: true, intVal: n}, nil
	case tInt64:
		p.advance()

		n, err := strconv.ParseInt(tok.lit, 10, 64)
		if err != nil {
			return operand{}, fmt.Errorf("%w: invalid Int64 literal %q", ErrFilterParse, tok.lit)
		}

		return operand{litType: tInt64, intVal: n}, nil
	case tFloat:
		p.advance()

		f, err := strconv.ParseFloat(tok.lit, 64)
		if err != nil {
			return operand{}, fmt.Errorf("%w: invalid float literal %q", ErrFilterParse, tok.lit)
		}

		return operand{litType: tFloat, floatVal: f}, nil
	case tBool:
		p.advance()

		return operand{litType: tBool, boolVal: tok.lit == "true"}, nil
	case tDateTime:
		p.advance()

		t, err := time.Parse(time.RFC3339Nano, tok.lit)
		if err != nil {
			return operand{}, fmt.Errorf("%w: invalid datetime literal %q", ErrFilterParse, tok.lit)
		}

		return operand{litType: tDateTime, timeVal: t}, nil
	case tGUID:
		p.advance()

		return operand{litType: tGUID, strVal: tok.lit}, nil
	case tBinary:
		p.advance()

		b, err := decodeBinaryLiteral(tok)
		if err != nil {
			return operand{}, err
		}

		return operand{litType: tBinary, bytesVal: b}, nil
	case tError:
		return operand{}, fmt.Errorf("%w: %s", ErrFilterParse, tok.lit)
	default:
		return operand{}, fmt.Errorf("%w: unexpected token %q", ErrFilterParse, tok.lit)
	}
}

func decodeBinaryLiteral(tok token) ([]byte, error) {
	if tok.enc == 'x' {
		b, err := hex.DecodeString(tok.lit)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid hex binary literal %q", ErrFilterParse, tok.lit)
		}

		return b, nil
	}

	b, err := base64.StdEncoding.DecodeString(tok.lit)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid base64 binary literal %q", ErrFilterParse, tok.lit)
	}

	return b, nil
}
