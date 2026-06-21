package datatree

import (
	"fmt"
	"strconv"
	"strings"
)

// A small XPath 1.0 front end (lexer + parser) for YANG must/when expressions
// and predicates (O8 slice 4b/4c). The parser returns an error for any construct
// it does not handle; callers treat a parse error as "cannot evaluate" and SKIP
// the check rather than risk a wrong verdict.

// --- AST ---------------------------------------------------------------------

type xExpr interface{ String() string }

type xNumber struct{ v float64 }

func (n *xNumber) String() string { return strconv.FormatFloat(n.v, 'g', -1, 64) }

type xLiteral struct{ s string }

func (l *xLiteral) String() string { return "'" + l.s + "'" }

type xBinary struct {
	op       string
	lhs, rhs xExpr
}

func (b *xBinary) String() string { return fmt.Sprintf("(%s %s %s)", b.op, b.lhs, b.rhs) }

type xNeg struct{ x xExpr }

func (n *xNeg) String() string { return fmt.Sprintf("(neg %s)", n.x) }

type xCall struct {
	name string
	args []xExpr
}

func (c *xCall) String() string {
	parts := make([]string, len(c.args))
	for i, a := range c.args {
		parts[i] = a.String()
	}
	return fmt.Sprintf("%s(%s)", c.name, strings.Join(parts, ","))
}

// xStep is one location-path step: an axis, a node test (a name, "*", or
// "node()"), and zero or more predicate expressions.
type xStep struct {
	axis  string // "child", "parent", "self", "descendant-or-self"
	test  string // local name, "*", or "node()"
	preds []xExpr
}

func (s *xStep) String() string {
	out := s.test
	switch s.axis {
	case "parent":
		out = ".."
	case "self":
		out = "."
	}
	for _, p := range s.preds {
		out += "[" + p.String() + "]"
	}
	return out
}

// xLocation is a location path, optionally absolute, optionally starting from a
// primary expression (e.g. a function result) — though YANG paths are usually
// plain absolute/relative.
type xLocation struct {
	abs   bool
	start xExpr // nil for a pure location path
	steps []*xStep
}

func (l *xLocation) String() string {
	var b strings.Builder
	if l.start != nil {
		b.WriteString(l.start.String())
	}
	if l.abs {
		b.WriteString("/")
	}
	for i, s := range l.steps {
		if i > 0 || (!l.abs && l.start == nil && i == 0) {
			if i > 0 {
				b.WriteString("/")
			}
		}
		b.WriteString(s.String())
	}
	return b.String()
}

// --- lexer -------------------------------------------------------------------

type tokKind int

const (
	tkEOF tokKind = iota
	tkName
	tkFunc // NCName/QName immediately followed by '('
	tkAxis // NCName followed by '::'
	tkNumber
	tkLiteral
	tkSlash  // /
	tkDSlash // //
	tkDotDot // ..
	tkDot    // .
	tkLParen
	tkRParen
	tkLBracket
	tkRBracket
	tkComma
	tkPipe
	tkStarTest   // * as a wildcard node test
	tkStarOp     // * as multiply
	tkOpName     // and / or / div / mod
	tkRelOp      // = != < <= > >=
	tkAddOp      // + -
	tkColonColon // :: (rejected by parser)
	tkAt         // @  (rejected by parser)
	tkDollar     // $  (rejected by parser)
)

type token struct {
	kind tokKind
	text string
}

func lexXPath(input string) ([]token, error) {
	var toks []token
	rs := []rune(input)
	i := 0
	prevAllowsOperand := func() bool {
		// Per XPath 1.0 §3.7: a '*' / NCName-operator is an operand position
		// (wildcard / name) when there is no preceding token or it is @, ::, (,
		// [, , or an operator.
		if len(toks) == 0 {
			return true
		}
		switch toks[len(toks)-1].kind {
		case tkAt, tkColonColon, tkLParen, tkLBracket, tkComma,
			tkStarOp, tkOpName, tkRelOp, tkAddOp, tkSlash, tkDSlash, tkPipe:
			return true
		}
		return false
	}
	for i < len(rs) {
		c := rs[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '\'' || c == '"':
			j := i + 1
			for j < len(rs) && rs[j] != c {
				j++
			}
			if j >= len(rs) {
				return nil, fmt.Errorf("unterminated string literal")
			}
			toks = append(toks, token{tkLiteral, string(rs[i+1 : j])})
			i = j + 1
		case c >= '0' && c <= '9' || (c == '.' && i+1 < len(rs) && rs[i+1] >= '0' && rs[i+1] <= '9'):
			j := i
			for j < len(rs) && (rs[j] >= '0' && rs[j] <= '9' || rs[j] == '.') {
				j++
			}
			toks = append(toks, token{tkNumber, string(rs[i:j])})
			i = j
		case c == '/':
			if i+1 < len(rs) && rs[i+1] == '/' {
				toks = append(toks, token{tkDSlash, "//"})
				i += 2
			} else {
				toks = append(toks, token{tkSlash, "/"})
				i++
			}
		case c == '.':
			if i+1 < len(rs) && rs[i+1] == '.' {
				toks = append(toks, token{tkDotDot, ".."})
				i += 2
			} else {
				toks = append(toks, token{tkDot, "."})
				i++
			}
		case c == '(':
			toks = append(toks, token{tkLParen, "("})
			i++
		case c == ')':
			toks = append(toks, token{tkRParen, ")"})
			i++
		case c == '[':
			toks = append(toks, token{tkLBracket, "["})
			i++
		case c == ']':
			toks = append(toks, token{tkRBracket, "]"})
			i++
		case c == ',':
			toks = append(toks, token{tkComma, ","})
			i++
		case c == '|':
			toks = append(toks, token{tkPipe, "|"})
			i++
		case c == '@':
			toks = append(toks, token{tkAt, "@"})
			i++
		case c == '$':
			toks = append(toks, token{tkDollar, "$"})
			i++
		case c == '+' || c == '-':
			toks = append(toks, token{tkAddOp, string(c)})
			i++
		case c == '=':
			toks = append(toks, token{tkRelOp, "="})
			i++
		case c == '!':
			if i+1 < len(rs) && rs[i+1] == '=' {
				toks = append(toks, token{tkRelOp, "!="})
				i += 2
			} else {
				return nil, fmt.Errorf("unexpected '!'")
			}
		case c == '<' || c == '>':
			op := string(c)
			i++
			if i < len(rs) && rs[i] == '=' {
				op += "="
				i++
			}
			toks = append(toks, token{tkRelOp, op})
		case c == '*':
			if prevAllowsOperand() {
				toks = append(toks, token{tkStarTest, "*"})
			} else {
				toks = append(toks, token{tkStarOp, "*"})
			}
			i++
		case c == ':' && i+1 < len(rs) && rs[i+1] == ':':
			toks = append(toks, token{tkColonColon, "::"})
			i += 2
		case isNameStart(c):
			j := i
			for j < len(rs) && isNameChar(rs[j]) {
				j++
			}
			name := string(rs[i:j])
			i = j
			// QName: NCName ':' NCName (but not '::').
			if i+1 < len(rs) && rs[i] == ':' && rs[i+1] != ':' && isNameStart(rs[i+1]) {
				k := i + 1
				for k < len(rs) && isNameChar(rs[k]) {
					k++
				}
				name = name + ":" + string(rs[i+1:k])
				i = k
			}
			toks = append(toks, classifyName(name, rs, i, prevAllowsOperand()))
		default:
			return nil, fmt.Errorf("unexpected character %q", string(c))
		}
	}
	toks = append(toks, token{tkEOF, ""})
	return toks, nil
}

func classifyName(name string, rs []rune, i int, operandPos bool) token {
	// Peek the next non-space rune.
	k := i
	for k < len(rs) && (rs[k] == ' ' || rs[k] == '\t' || rs[k] == '\n' || rs[k] == '\r') {
		k++
	}
	if k < len(rs) && rs[k] == '(' {
		return token{tkFunc, name}
	}
	if k+1 < len(rs) && rs[k] == ':' && rs[k+1] == ':' {
		return token{tkAxis, name}
	}
	if !operandPos {
		switch name {
		case "and", "or", "div", "mod":
			return token{tkOpName, name}
		}
	}
	return token{tkName, name}
}

func isNameStart(c rune) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isNameChar(c rune) bool {
	return isNameStart(c) || (c >= '0' && c <= '9') || c == '-' || c == '.'
}

// --- parser ------------------------------------------------------------------

type xparser struct {
	toks []token
	pos  int
}

func parseXPath(input string) (xExpr, error) {
	toks, err := lexXPath(input)
	if err != nil {
		return nil, err
	}
	p := &xparser{toks: toks}
	e, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.cur().kind != tkEOF {
		return nil, fmt.Errorf("trailing tokens at %q", p.cur().text)
	}
	return e, nil
}

func (p *xparser) cur() token  { return p.toks[p.pos] }
func (p *xparser) next() token { t := p.toks[p.pos]; p.pos++; return t }

func (p *xparser) parseOr() (xExpr, error) {
	lhs, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.cur().kind == tkOpName && p.cur().text == "or" {
		p.next()
		rhs, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		lhs = &xBinary{"or", lhs, rhs}
	}
	return lhs, nil
}

func (p *xparser) parseAnd() (xExpr, error) {
	lhs, err := p.parseEquality()
	if err != nil {
		return nil, err
	}
	for p.cur().kind == tkOpName && p.cur().text == "and" {
		p.next()
		rhs, err := p.parseEquality()
		if err != nil {
			return nil, err
		}
		lhs = &xBinary{"and", lhs, rhs}
	}
	return lhs, nil
}

func (p *xparser) parseEquality() (xExpr, error) {
	lhs, err := p.parseRelational()
	if err != nil {
		return nil, err
	}
	for p.cur().kind == tkRelOp && (p.cur().text == "=" || p.cur().text == "!=") {
		op := p.next().text
		rhs, err := p.parseRelational()
		if err != nil {
			return nil, err
		}
		lhs = &xBinary{op, lhs, rhs}
	}
	return lhs, nil
}

func (p *xparser) parseRelational() (xExpr, error) {
	lhs, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}
	for p.cur().kind == tkRelOp && p.cur().text != "=" && p.cur().text != "!=" {
		op := p.next().text
		rhs, err := p.parseAdditive()
		if err != nil {
			return nil, err
		}
		lhs = &xBinary{op, lhs, rhs}
	}
	return lhs, nil
}

func (p *xparser) parseAdditive() (xExpr, error) {
	lhs, err := p.parseMultiplicative()
	if err != nil {
		return nil, err
	}
	for p.cur().kind == tkAddOp {
		op := p.next().text
		rhs, err := p.parseMultiplicative()
		if err != nil {
			return nil, err
		}
		lhs = &xBinary{op, lhs, rhs}
	}
	return lhs, nil
}

func (p *xparser) parseMultiplicative() (xExpr, error) {
	lhs, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.cur().kind == tkStarOp || (p.cur().kind == tkOpName && (p.cur().text == "div" || p.cur().text == "mod")) {
		op := p.next().text
		if op == "*" {
			op = "mul"
		}
		rhs, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		lhs = &xBinary{op, lhs, rhs}
	}
	return lhs, nil
}

func (p *xparser) parseUnary() (xExpr, error) {
	if p.cur().kind == tkAddOp && p.cur().text == "-" {
		p.next()
		x, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &xNeg{x}, nil
	}
	return p.parseUnion()
}

func (p *xparser) parseUnion() (xExpr, error) {
	lhs, err := p.parsePath()
	if err != nil {
		return nil, err
	}
	for p.cur().kind == tkPipe {
		p.next()
		rhs, err := p.parsePath()
		if err != nil {
			return nil, err
		}
		lhs = &xBinary{"|", lhs, rhs}
	}
	return lhs, nil
}

func (p *xparser) parsePath() (xExpr, error) {
	switch p.cur().kind {
	case tkNumber, tkLiteral, tkFunc, tkLParen:
		primary, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		// optional trailing location steps (e.g. deref(.)/x)
		if p.cur().kind == tkSlash || p.cur().kind == tkDSlash {
			steps, err := p.parseSteps()
			if err != nil {
				return nil, err
			}
			return &xLocation{start: primary, steps: steps}, nil
		}
		return primary, nil
	default:
		return p.parseLocationPath()
	}
}

func (p *xparser) parseLocationPath() (xExpr, error) {
	loc := &xLocation{}
	if p.cur().kind == tkSlash {
		loc.abs = true
		p.next()
		// "/" alone is the root; only continue if a step follows.
		if isStepStart(p.cur().kind) {
			steps, err := p.parseRelativeSteps()
			if err != nil {
				return nil, err
			}
			loc.steps = steps
		}
		return loc, nil
	}
	if p.cur().kind == tkDSlash {
		loc.abs = true
		p.next()
		loc.steps = append(loc.steps, &xStep{axis: "descendant-or-self", test: "node()"})
		steps, err := p.parseRelativeSteps()
		if err != nil {
			return nil, err
		}
		loc.steps = append(loc.steps, steps...)
		return loc, nil
	}
	steps, err := p.parseRelativeSteps()
	if err != nil {
		return nil, err
	}
	loc.steps = steps
	return loc, nil
}

// parseSteps parses the leading separator(s) then relative steps (used after a
// primary expr).
func (p *xparser) parseSteps() ([]*xStep, error) {
	var steps []*xStep
	if p.cur().kind == tkDSlash {
		p.next()
		steps = append(steps, &xStep{axis: "descendant-or-self", test: "node()"})
	} else {
		p.next() // tkSlash
	}
	rest, err := p.parseRelativeSteps()
	if err != nil {
		return nil, err
	}
	return append(steps, rest...), nil
}

func (p *xparser) parseRelativeSteps() ([]*xStep, error) {
	var steps []*xStep
	s, err := p.parseStep()
	if err != nil {
		return nil, err
	}
	steps = append(steps, s)
	for p.cur().kind == tkSlash || p.cur().kind == tkDSlash {
		if p.cur().kind == tkDSlash {
			p.next()
			steps = append(steps, &xStep{axis: "descendant-or-self", test: "node()"})
		} else {
			p.next()
		}
		s, err := p.parseStep()
		if err != nil {
			return nil, err
		}
		steps = append(steps, s)
	}
	return steps, nil
}

func isStepStart(k tokKind) bool {
	switch k {
	case tkName, tkStarTest, tkDot, tkDotDot, tkFunc:
		return true
	}
	return false
}

func (p *xparser) parseStep() (*xStep, error) {
	switch p.cur().kind {
	case tkDot:
		p.next()
		return &xStep{axis: "self", test: "node()"}, nil
	case tkDotDot:
		p.next()
		return &xStep{axis: "parent", test: "node()"}, nil
	case tkName:
		name := p.next().text
		return p.finishStep("child", name)
	case tkStarTest:
		p.next()
		return p.finishStep("child", "*")
	case tkFunc:
		// node() / text() node-type test
		fn := p.next().text
		if p.cur().kind != tkLParen {
			return nil, fmt.Errorf("expected '(' after %q", fn)
		}
		p.next()
		if p.cur().kind != tkRParen {
			return nil, fmt.Errorf("node-type test %q with arguments unsupported", fn)
		}
		p.next()
		if fn != "node" {
			return nil, fmt.Errorf("node-type test %q unsupported", fn)
		}
		return p.finishStep("child", "node()")
	default:
		return nil, fmt.Errorf("unexpected token %q in step", p.cur().text)
	}
}

func (p *xparser) finishStep(axis, test string) (*xStep, error) {
	s := &xStep{axis: axis, test: test}
	for p.cur().kind == tkLBracket {
		p.next()
		pred, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.cur().kind != tkRBracket {
			return nil, fmt.Errorf("expected ']'")
		}
		p.next()
		s.preds = append(s.preds, pred)
	}
	return s, nil
}

func (p *xparser) parsePrimary() (xExpr, error) {
	switch p.cur().kind {
	case tkNumber:
		v, err := strconv.ParseFloat(p.next().text, 64)
		if err != nil {
			return nil, err
		}
		return &xNumber{v}, nil
	case tkLiteral:
		return &xLiteral{p.next().text}, nil
	case tkLParen:
		p.next()
		e, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.cur().kind != tkRParen {
			return nil, fmt.Errorf("expected ')'")
		}
		p.next()
		return e, nil
	case tkFunc:
		return p.parseCall()
	default:
		return nil, fmt.Errorf("unexpected token %q", p.cur().text)
	}
}

func (p *xparser) parseCall() (xExpr, error) {
	name := p.next().text
	if p.cur().kind != tkLParen {
		return nil, fmt.Errorf("expected '(' after function %q", name)
	}
	p.next()
	call := &xCall{name: name}
	if p.cur().kind != tkRParen {
		for {
			arg, err := p.parseOr()
			if err != nil {
				return nil, err
			}
			call.args = append(call.args, arg)
			if p.cur().kind == tkComma {
				p.next()
				continue
			}
			break
		}
	}
	if p.cur().kind != tkRParen {
		return nil, fmt.Errorf("expected ')' closing %q", name)
	}
	p.next()
	return call, nil
}
