// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package yangpattern

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
	"unicode/utf8"

	"github.com/signalbreak-labs/cambium/go/internal/xsdregex"
)

// Compile parses a complete XSD expression, normalizing arbitrary decimal counts.
func Compile(source string) (*Program, error) { return CompileWithLimits(source, CompileLimits{}) }

// CompileWithLimits applies a compilation policy; source is retained verbatim.
func CompileWithLimits(source string, limits CompileLimits) (*Program, error) {
	l := limits.defaults()
	if err := limitError("compile", "parse", "source bytes", l.SourceBytes, uint64(len(source))); err != nil {
		return nil, err
	}
	if !utf8.ValidString(source) {
		return nil, &ConfigurationError{"source is not UTF-8"}
	}
	c := compiler{limits: l, b: ledger{phase: "compile", stage: "parse", byteLimit: l.Bytes, workLimit: l.Work}, intern: make(map[string]uint32)}
	if !c.b.charge(mulSat(uint64(len(source)), 8), uint64(len(source))) {
		return nil, c.b.err
	}
	c.source = []rune(source)
	c.put(NodeData{Op: OpEmpty})
	c.put(NodeData{Op: OpEpsilon})
	root := c.expression(1)
	if c.err == nil && c.pos != len(c.source) {
		c.fail("unexpected closing group")
	}
	if c.b.err != nil {
		return nil, c.b.err
	}
	if c.err != nil {
		return nil, c.err
	}
	d := ProgramData{Version: FormatVersion, Source: source, Nodes: c.nodes, Root: root}
	// Validation/copying is mandatory constructor work, charged from the same
	// compilation allowance, rather than receiving a second full budget.
	if !c.b.charge(1, 1) {
		return nil, c.b.err
	}
	remainingBytes, remainingWork := l.Bytes-c.b.bytes+1, l.Work-c.b.work+1
	p, err := RestoreWithLimits(d, RestoreLimits{Bytes: remainingBytes, Nodes: l.Nodes, Edges: l.Edges, Depth: l.Depth, Digits: l.Digits, Ranges: l.Ranges, Work: remainingWork})
	if e, ok := errors.AsType[*ResourceError](err); ok {
		resource := *e
		resource.Phase = "compile"
		if resource.Limit == "bytes" {
			resource.Allowance = l.Bytes
			resource.Observed = addSat(resource.Observed, c.b.bytes-1)
		}
		if resource.Limit == "work" {
			resource.Allowance = l.Work
			resource.Observed = addSat(resource.Observed, c.b.work-1)
		}
		return nil, &resource
	}
	return p, err
}

type compiler struct {
	source                []rune
	pos                   int
	limits                CompileLimits
	b                     ledger
	nodes                 []NodeData
	nullable              []bool
	intern                map[string]uint32
	edges, digits, ranges uint64
	err                   error
}

func (c *compiler) fail(detail string) {
	if c.err == nil {
		c.err = &ConfigurationError{fmt.Sprintf("at rune %d: %s", c.pos, detail)}
	}
}
func (c *compiler) tick() bool { return c.err == nil && c.b.charge(0, 1) }
func (c *compiler) put(n NodeData) uint32 {
	if !c.tick() {
		return 0
	}
	size := uint64(512 + len(n.Children)*32 + len(n.Ranges)*128 + 4*(len(n.Min)+len(n.Max)))
	if !c.b.charge(size, uint64(len(n.Children)+len(n.Ranges)+len(n.Min)+len(n.Max))) {
		return 0
	}
	key := fmt.Sprintf("%d/%v/%v/%s/%s/%t", n.Op, n.Children, n.Ranges, n.Min, n.Max, n.Unbounded)
	if id, ok := c.intern[key]; ok {
		return id
	}
	c.edges += uint64(len(n.Children))
	c.digits += uint64(len(n.Min) + len(n.Max))
	c.ranges += uint64(len(n.Ranges))
	for _, v := range []struct {
		name          string
		limit, actual uint64
	}{{"nodes", c.limits.Nodes, uint64(len(c.nodes) + 1)}, {"edges", c.limits.Edges, c.edges}, {"digits", c.limits.Digits, c.digits}, {"ranges", c.limits.Ranges, c.ranges}} {
		if err := limitError("compile", "normalize", v.name, v.limit, v.actual); err != nil {
			c.b.err = err
			return 0
		}
	}
	nullable := n.Op == OpEpsilon || (n.Op == OpRepeat && n.Min == "0")
	if n.Op == OpAlternate {
		nullable = c.nullable[n.Children[0]] || c.nullable[n.Children[1]]
	}
	if n.Op == OpConcat {
		nullable = c.nullable[n.Children[0]] && c.nullable[n.Children[1]]
	}
	if uint64(len(c.nodes)) > uint64(^uint32(0)) {
		c.b.err = &ResourceError{"compile", "normalize", "node indexes", uint64(^uint32(0)), uint64(len(c.nodes))}
		return 0
	}
	id := uint32(len(c.nodes)) // #nosec G115 -- explicit uint32 bound above.
	c.nodes = append(c.nodes, n)
	c.nullable = append(c.nullable, nullable)
	c.intern[key] = id
	return id
}
func (c *compiler) binary(op Op, a, b uint32) uint32 {
	if op == OpAlternate {
		if a == 0 || a == b {
			return b
		}
		if b == 0 {
			return a
		}
		if a > b {
			a, b = b, a
		}
	}
	if op == OpConcat {
		if a == 0 || b == 0 {
			return 0
		}
		if a == 1 {
			return b
		}
		if b == 1 {
			return a
		}
	}
	return c.put(NodeData{Op: op, Children: []uint32{a, b}})
}
func (c *compiler) nonempty(id uint32) uint32 {
	if !c.tick() || !c.nullable[id] {
		return id
	}
	n := c.nodes[id]
	switch n.Op {
	case OpEpsilon:
		return 0
	case OpAlternate:
		return c.binary(OpAlternate, c.nonempty(n.Children[0]), c.nonempty(n.Children[1]))
	case OpConcat:
		a, b := n.Children[0], n.Children[1]
		r := c.binary(OpConcat, c.nonempty(a), b)
		if c.nullable[a] {
			r = c.binary(OpAlternate, r, c.nonempty(b))
		}
		return r
	case OpRepeat:
		return c.repeat(n.Children[0], "1", n.Max, n.Unbounded)
	}
	return id
}
func (c *compiler) multiply(a, b string) string {
	if !c.b.charge(uint64(len(a)+len(b))*4, mulSat(uint64(len(a)), uint64(len(b)))) {
		return "0"
	}
	if err := limitError("compile", "normalize", "digits", c.limits.Digits, uint64(len(a)+len(b))); err != nil {
		c.b.err = err
		return "0"
	}
	var x, y big.Int
	x.SetString(a, 10)
	y.SetString(b, 10)
	return x.Mul(&x, &y).String()
}
func (c *compiler) repeat(id uint32, lo, hi string, unbounded bool) uint32 {
	if !c.tick() {
		return 0
	}
	if (!unbounded && hi == "0") || id == 1 {
		return 1
	}
	if id == 0 {
		if lo == "0" {
			return 1
		}
		return 0
	}
	n := c.nodes[id]
	if n.Op == OpRepeat && ((!unbounded && lo == hi) || n.Min == "0") {
		lower := c.multiply(n.Min, lo)
		upper := ""
		inf := n.Unbounded || unbounded
		if !inf {
			upper = c.multiply(n.Max, hi)
		}
		return c.repeat(n.Children[0], lower, upper, inf)
	}
	if c.nullable[id] {
		id = c.nonempty(id)
		lo = "0"
	}
	if id == 0 {
		return 1
	}
	if lo == "1" && hi == "1" && !unbounded {
		return id
	}
	return c.put(NodeData{Op: OpRepeat, Children: []uint32{id}, Min: lo, Max: hi, Unbounded: unbounded})
}
func (c *compiler) expression(depth uint64) uint32 {
	if err := limitError("compile", "parse", "depth", c.limits.Depth, depth); err != nil {
		c.b.err = err
		return 0
	}
	result := c.sequence(depth)
	for c.pos < len(c.source) && c.source[c.pos] == '|' && c.tick() {
		c.pos++
		result = c.binary(OpAlternate, result, c.sequence(depth))
	}
	return result
}
func (c *compiler) sequence(depth uint64) uint32 {
	// Build a balanced concatenation to keep ordinary long literals shallow.
	var terms []uint32
	for c.pos < len(c.source) && c.source[c.pos] != ')' && c.source[c.pos] != '|' && c.tick() {
		id := c.atom(depth)
		if c.err != nil || c.b.err != nil {
			return 0
		}
		if c.pos < len(c.source) {
			switch c.source[c.pos] {
			case '*':
				c.pos++
				id = c.repeat(id, "0", "", true)
			case '+':
				c.pos++
				id = c.repeat(id, "1", "", true)
			case '?':
				c.pos++
				id = c.repeat(id, "0", "1", false)
			case '{':
				lo, hi, inf, ok := c.count()
				if ok {
					id = c.repeat(id, lo, hi, inf)
				} else {
					c.fail("malformed quantifier")
				}
			}
		}
		if !c.b.charge(8, 1) {
			return 0
		}
		terms = append(terms, id)
	}
	if len(terms) == 0 {
		return 1
	}
	for len(terms) > 1 {
		out := terms[:0]
		for i := 0; i < len(terms); i += 2 {
			if i+1 == len(terms) {
				out = append(out, terms[i])
			} else {
				out = append(out, c.binary(OpConcat, terms[i], terms[i+1]))
			}
		}
		terms = out
	}
	return terms[0]
}
func (c *compiler) atom(depth uint64) uint32 {
	start := c.pos
	r := c.source[c.pos]
	c.pos++
	if r == '(' {
		id := c.expression(depth + 1)
		if c.pos >= len(c.source) || c.source[c.pos] != ')' {
			c.fail("unterminated group")
			return 0
		}
		c.pos++
		return id
	}
	if strings.ContainsRune("*+?}]", r) {
		c.fail("quantifier without atom")
		return 0
	}
	if r == '{' {
		c.fail("quantifier without atom")
		return 0
	}
	if r == '\\' {
		c.scanEscape()
	}
	if r == '[' {
		nesting := uint64(1)
		for c.pos < len(c.source) && nesting > 0 && c.tick() {
			ch := c.source[c.pos]
			c.pos++
			switch ch {
			case '\\':
				c.scanEscape()
			case '[':
				nesting++
				if err := limitError("compile", "parse", "depth", c.limits.Depth, depth+nesting); err != nil {
					c.b.err = err
					return 0
				}
			case ']':
				nesting--
			}
		}
		if nesting != 0 {
			c.fail("unterminated character class")
			return 0
		}
	}
	atom := c.source[start:c.pos]
	// Reserve conservative temporary storage before the existing atom translator
	// expands Unicode tables or class subtraction. Numerical counts never enter it.
	bound := uint64(len(atom) + 4)
	for i, ch := range atom {
		if ch == '[' && i+1 < len(atom) && atom[i+1] == ':' {
			// Reserve fixed parser/AST overhead for one ASCII POSIX class atom.
			bound = addSat(bound, 128)
		}
		if ch == '\\' && i+1 < len(atom) && strings.ContainsRune("pPdDwW", atom[i+1]) {
			bound = addSat(bound, 8192)
		}
		if ch == '\\' && i+1 < len(atom) && strings.ContainsRune("iIcCsS", atom[i+1]) {
			bound = addSat(bound, 64)
		}
	}
	if !c.b.charge(mulSat(bound, 64), mulSat(bound, uint64(len(atom)+1))) {
		return 0
	}
	ranges, ok := xsdregex.AtomRanges(atom)
	if !ok {
		c.fail("invalid character atom")
		return 0
	}
	if len(ranges) == 0 {
		return 0
	}
	out := make([]RuneRange, len(ranges))
	for i, r := range ranges {
		out[i] = RuneRange{r[0], r[1]}
	}
	return c.put(NodeData{Op: OpClass, Ranges: out})
}
func (c *compiler) scanEscape() {
	if c.pos >= len(c.source) {
		c.fail("trailing escape")
		return
	}
	r := c.source[c.pos]
	c.pos++
	if r == 'p' || r == 'P' {
		if c.pos >= len(c.source) || c.source[c.pos] != '{' {
			c.fail("invalid category escape")
			return
		}
		c.pos++
		for c.pos < len(c.source) && c.source[c.pos] != '}' {
			c.pos++
		}
		if c.pos == len(c.source) {
			c.fail("unterminated category escape")
			return
		}
		c.pos++
	}
}
func (c *compiler) count() (lo, hi string, inf, ok bool) {
	start := c.pos
	i := start + 1
	read := func() string {
		begin := i
		for i < len(c.source) && c.source[i] >= '0' && c.source[i] <= '9' {
			i++
		}
		if begin == i {
			return ""
		}
		digits := uint64(i) - uint64(begin) // #nosec G115 -- both are nonnegative source indexes.
		if err := limitError("compile", "parse", "digits", c.limits.Digits, digits); err != nil {
			c.b.err = err
			return ""
		}
		if !c.b.charge(digits, digits) {
			return ""
		}
		s := strings.TrimLeft(string(c.source[begin:i]), "0")
		if s == "" {
			return "0"
		}
		return s
	}
	lo = read()
	if lo == "" || i == len(c.source) {
		return "", "", false, false
	}
	hi = lo
	if c.source[i] == ',' {
		i++
		hi = read()
		inf = hi == ""
	}
	if i >= len(c.source) || c.source[i] != '}' {
		return "", "", false, false
	}
	c.pos = i + 1
	if !inf && decimalCompare(lo, hi) > 0 {
		c.fail("reversed quantifier range")
	}
	if err := limitError("compile", "parse", "digits", c.limits.Digits, uint64(i)-uint64(start)); err != nil { // #nosec G115 -- source positions satisfy 0 <= start < i.
		c.b.err = err
	}
	return lo, hi, inf, true
}

// GoRegexp returns a full-string canonical native expression. Counts outside
// Go's representable syntax return an error even when MatchString can evaluate.
func (p *Program) GoRegexp() (string, error) {
	if p == nil || len(p.data.Nodes) == 0 {
		return "", &ConfigurationError{"uninitialized program"}
	}
	s, _, err := p.nativeExpression(false, MatchLimits{}.defaults())
	if err != nil {
		if e, ok := errors.AsType[*ResourceError](err); ok {
			resource := *e
			resource.Phase = "compile"
			return "", &resource
		}
		return "", err
	}
	if _, err := p.compileNative(s); err != nil {
		return "", err
	}
	return s, nil
}
