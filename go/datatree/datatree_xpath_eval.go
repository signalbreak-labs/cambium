// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// XPath evaluator over the data tree (O8 slice 4b). Names are resolved to
// namespaces via the context module's prefix map (an unprefixed name is the
// context module's own namespace, per RFC 7950 §6.4.1) — never matched by bare
// local name, so a same-local-name node in another module can't be confused.
// Any unimplemented function or unresolvable prefix returns an error; the
// caller skips the check rather than risk a wrong verdict.

// xnode is an XML-data-model view of a data node, with parent links for the
// parent/ancestor axes.
type xnode struct {
	name      string
	ns        string // namespace URI
	value     string // leaf lexical value; "" for non-leaf
	leaf      bool
	schema    cambium.SchemaNodeRef
	hasSchema bool
	parent    *xnode
	kids      []*xnode
}

// buildXNodes builds the xnode children for a level, parallel to the schema, so
// names carry their namespace.
func buildXNodes(schema []cambium.SchemaNodeRef, data []*node, parent *xnode) []*xnode {
	present := make(map[nodeKey]*node, len(data))
	for _, d := range data {
		present[dataNodeKey(d)] = d
	}
	var out []*xnode
	for _, sn := range schema {
		dn := present[schemaNodeKey(sn)]
		if dn == nil {
			continue
		}
		switch {
		case sn.IsLeaf():
			out = append(out, &xnode{name: sn.Name(), ns: sn.Namespace(), value: xmlTextFromToken(dn.value), leaf: true, schema: sn, hasSchema: true, parent: parent})
		case sn.IsLeafList():
			for _, v := range dn.values {
				out = append(out, &xnode{name: sn.Name(), ns: sn.Namespace(), value: xmlTextFromToken(v), leaf: true, schema: sn, hasSchema: true, parent: parent})
			}
		case sn.IsContainer():
			xn := &xnode{name: sn.Name(), ns: sn.Namespace(), schema: sn, hasSchema: true, parent: parent}
			xn.kids = buildXNodes(childRefs(sn.DataChildren(true)), dn.children, xn)
			out = append(out, xn)
		case sn.IsList():
			for _, entry := range dn.entries {
				xn := &xnode{name: sn.Name(), ns: sn.Namespace(), schema: sn, hasSchema: true, parent: parent}
				xn.kids = buildXNodes(childRefs(sn.DataChildren(true)), entry, xn)
				out = append(out, xn)
			}
		}
	}
	return out
}

func stringValue(n *xnode) string {
	if n.leaf {
		return n.value
	}
	var b strings.Builder
	for _, k := range n.kids {
		b.WriteString(stringValue(k))
	}
	return b.String()
}

// --- values ------------------------------------------------------------------

type xkind int

const (
	kNodeset xkind = iota
	kStr
	kNum
	kBool
)

type xval struct {
	kind xkind
	ns   []*xnode
	s    string
	n    float64
	b    bool
}

func boolVal(b bool) xval   { return xval{kind: kBool, b: b} }
func numVal(n float64) xval { return xval{kind: kNum, n: n} }
func strVal(s string) xval  { return xval{kind: kStr, s: s} }

func (v xval) toBool() bool {
	switch v.kind {
	case kNodeset:
		return len(v.ns) > 0
	case kStr:
		return v.s != ""
	case kNum:
		return v.n != 0 && !math.IsNaN(v.n)
	default:
		return v.b
	}
}

func (v xval) toStr() string {
	switch v.kind {
	case kNodeset:
		if len(v.ns) == 0 {
			return ""
		}
		return stringValue(v.ns[0])
	case kStr:
		return v.s
	case kNum:
		return formatXPathNumber(v.n)
	default:
		if v.b {
			return "true"
		}
		return "false"
	}
}

func (v xval) toNum() float64 {
	switch v.kind {
	case kNum:
		return v.n
	case kBool:
		if v.b {
			return 1
		}
		return 0
	default:
		return parseXPathNumber(v.toStr())
	}
}

func formatXPathNumber(n float64) string {
	if math.IsNaN(n) {
		return "NaN"
	}
	if n == math.Trunc(n) && !math.IsInf(n, 0) {
		return strconv.FormatFloat(n, 'f', -1, 64)
	}
	return strconv.FormatFloat(n, 'g', -1, 64)
}

func parseXPathNumber(s string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return math.NaN()
	}
	return f
}

// --- evaluator ---------------------------------------------------------------

type evaluator struct {
	root         *xnode
	module       cambium.Module
	unprefixedNS string
	excluded     map[cambium.SchemaNodeRef]bool
	current      *xnode
}

type ectx struct {
	node *xnode
	pos  int
	size int
}

func (ev *evaluator) resolveName(qname string) (ns, local string, err error) {
	if i := strings.Index(qname, ":"); i >= 0 {
		prefix := qname[:i]
		local = qname[i+1:]
		mod, ok := ev.module.ResolvePrefix(prefix)
		if !ok {
			return "", "", fmt.Errorf("unknown prefix %q", prefix)
		}
		return mod.Namespace(), local, nil
	}
	if ev.unprefixedNS != "" {
		return ev.unprefixedNS, qname, nil
	}
	return ev.module.Namespace(), qname, nil // fallback for callers without a bound context node
}

func (ev *evaluator) matchTest(n *xnode, test string) (bool, error) {
	if test == "*" || test == "node()" {
		return true, nil
	}
	ns, local, err := ev.resolveName(test)
	if err != nil {
		return false, err
	}
	return n.name == local && n.ns == ns, nil
}

func (ev *evaluator) eval(e xExpr, c ectx) (xval, error) {
	switch x := e.(type) {
	case *xNumber:
		return numVal(x.v), nil
	case *xLiteral:
		return strVal(x.s), nil
	case *xNeg:
		v, err := ev.eval(x.x, c)
		if err != nil {
			return xval{}, err
		}
		return numVal(-v.toNum()), nil
	case *xCall:
		return ev.evalCall(x, c)
	case *xLocation:
		ns, err := ev.evalPath(x, c)
		if err != nil {
			return xval{}, err
		}
		return xval{kind: kNodeset, ns: ns}, nil
	case *xBinary:
		return ev.evalBinary(x, c)
	default:
		return xval{}, fmt.Errorf("cannot evaluate %T", e)
	}
}

func (ev *evaluator) evalBinary(x *xBinary, c ectx) (xval, error) {
	switch x.op {
	case "or":
		l, err := ev.eval(x.lhs, c)
		if err != nil {
			return xval{}, err
		}
		if l.toBool() {
			return boolVal(true), nil
		}
		r, err := ev.eval(x.rhs, c)
		if err != nil {
			return xval{}, err
		}
		return boolVal(r.toBool()), nil
	case "and":
		l, err := ev.eval(x.lhs, c)
		if err != nil {
			return xval{}, err
		}
		if !l.toBool() {
			return boolVal(false), nil
		}
		r, err := ev.eval(x.rhs, c)
		if err != nil {
			return xval{}, err
		}
		return boolVal(r.toBool()), nil
	case "|":
		l, err := ev.eval(x.lhs, c)
		if err != nil {
			return xval{}, err
		}
		r, err := ev.eval(x.rhs, c)
		if err != nil {
			return xval{}, err
		}
		if l.kind != kNodeset || r.kind != kNodeset {
			return xval{}, fmt.Errorf("union of non-node-sets")
		}
		return xval{kind: kNodeset, ns: unionNodes(l.ns, r.ns)}, nil
	case "=", "!=", "<", "<=", ">", ">=":
		l, err := ev.eval(x.lhs, c)
		if err != nil {
			return xval{}, err
		}
		r, err := ev.eval(x.rhs, c)
		if err != nil {
			return xval{}, err
		}
		return boolVal(compareValues(x.op, l, r)), nil
	default: // + - mul div mod
		l, err := ev.eval(x.lhs, c)
		if err != nil {
			return xval{}, err
		}
		r, err := ev.eval(x.rhs, c)
		if err != nil {
			return xval{}, err
		}
		return numVal(arith(x.op, l.toNum(), r.toNum())), nil
	}
}

func arith(op string, a, b float64) float64 {
	switch op {
	case "+":
		return a + b
	case "-":
		return a - b
	case "mul":
		return a * b
	case "div":
		return a / b
	case "mod":
		return math.Mod(a, b)
	}
	return math.NaN()
}

func unionNodes(a, b []*xnode) []*xnode {
	seen := make(map[*xnode]bool, len(a)+len(b))
	var out []*xnode
	for _, n := range append(append([]*xnode{}, a...), b...) {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

// --- comparison (XPath 1.0 §3.4) ---------------------------------------------

func compareValues(op string, a, b xval) bool {
	if a.kind == kNodeset || b.kind == kNodeset {
		return compareNodeset(op, a, b)
	}
	switch op {
	case "=", "!=":
		var eq bool
		switch {
		case a.kind == kBool || b.kind == kBool:
			eq = a.toBool() == b.toBool()
		case a.kind == kNum || b.kind == kNum:
			eq = a.toNum() == b.toNum()
		default:
			eq = a.toStr() == b.toStr()
		}
		if op == "=" {
			return eq
		}
		return !eq
	default:
		return numCompare(op, a.toNum(), b.toNum())
	}
}

func compareNodeset(op string, a, b xval) bool {
	// Gather the comparison operands from each side.
	if a.kind == kNodeset && b.kind == kNodeset {
		for _, na := range a.ns {
			for _, nb := range b.ns {
				if cmpStrings(op, stringValue(na), stringValue(nb)) {
					return true
				}
			}
		}
		return false
	}
	ns, other := a, b
	swapped := false
	if b.kind == kNodeset {
		ns, other = b, a
		swapped = true
	}
	// node-set vs boolean: convert the node-set to a boolean (not existential).
	if other.kind == kBool {
		return boolEq(op, len(ns.ns) > 0, other.b)
	}
	for _, n := range ns.ns {
		sv := stringValue(n)
		var res bool
		switch other.kind {
		case kNum:
			lo, ro := parseXPathNumber(sv), other.n
			if swapped {
				lo, ro = ro, lo
			}
			res = applyNumOp(op, lo, ro)
		default: // string
			lo, ro := sv, other.s
			if swapped {
				lo, ro = ro, lo
			}
			res = cmpStrings(op, lo, ro)
		}
		if res {
			return true
		}
	}
	return false
}

func boolEq(op string, a, b bool) bool {
	switch op {
	case "=":
		return a == b
	case "!=":
		return a != b
	}
	return false
}

func cmpStrings(op, a, b string) bool {
	switch op {
	case "=":
		return a == b
	case "!=":
		return a != b
	default:
		return applyNumOp(op, parseXPathNumber(a), parseXPathNumber(b))
	}
}

func applyNumOp(op string, a, b float64) bool {
	switch op {
	case "=":
		return a == b
	case "!=":
		return a != b
	default:
		return numCompare(op, a, b)
	}
}

func numCompare(op string, a, b float64) bool {
	switch op {
	case "<":
		return a < b
	case "<=":
		return a <= b
	case ">":
		return a > b
	case ">=":
		return a >= b
	}
	return false
}

// --- location paths ----------------------------------------------------------

func (ev *evaluator) evalPath(loc *xLocation, c ectx) ([]*xnode, error) {
	var cur []*xnode
	switch {
	case loc.start != nil:
		v, err := ev.eval(loc.start, c)
		if err != nil {
			return nil, err
		}
		if v.kind != kNodeset {
			return nil, fmt.Errorf("location step on non-node-set")
		}
		cur = v.ns
	case loc.abs:
		cur = []*xnode{ev.root}
	default:
		cur = []*xnode{c.node}
	}
	for _, step := range loc.steps {
		next, err := ev.evalStep(step, cur)
		if err != nil {
			return nil, err
		}
		cur = next
	}
	return cur, nil
}

func (ev *evaluator) evalStep(step *xStep, cur []*xnode) ([]*xnode, error) {
	seen := make(map[*xnode]bool)
	var cand []*xnode
	add := func(n *xnode) error {
		if n == nil || seen[n] || ev.isExcluded(n) {
			return nil
		}
		ok, err := ev.matchTest(n, step.test)
		if err != nil {
			return err
		}
		if ok {
			seen[n] = true
			cand = append(cand, n)
		}
		return nil
	}
	for _, ctxn := range cur {
		switch step.axis {
		case "self":
			if err := add(ctxn); err != nil {
				return nil, err
			}
		case "parent":
			if err := add(ctxn.parent); err != nil {
				return nil, err
			}
		case "child":
			for _, k := range ctxn.kids {
				if err := add(k); err != nil {
					return nil, err
				}
			}
		case "descendant-or-self":
			var walk func(n *xnode) error
			walk = func(n *xnode) error {
				if err := add(n); err != nil {
					return err
				}
				for _, k := range n.kids {
					if err := walk(k); err != nil {
						return err
					}
				}
				return nil
			}
			if err := walk(ctxn); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("unsupported axis %q", step.axis)
		}
	}
	return ev.applyPredicates(cand, step.preds)
}

func (ev *evaluator) isExcluded(n *xnode) bool {
	if ev == nil || len(ev.excluded) == 0 || n == nil || !n.hasSchema {
		return false
	}
	if ev.excluded[n.schema] {
		return true
	}
	for _, ancestor := range n.schema.Ancestors() {
		if ev.excluded[ancestor] {
			return true
		}
	}
	return false
}

func (ev *evaluator) applyPredicates(nodes []*xnode, preds []xExpr) ([]*xnode, error) {
	for _, pred := range preds {
		var kept []*xnode
		size := len(nodes)
		for i, n := range nodes {
			v, err := ev.eval(pred, ectx{node: n, pos: i + 1, size: size})
			if err != nil {
				return nil, err
			}
			keep := v.toBool()
			if v.kind == kNum {
				keep = v.n == float64(i+1) // positional predicate
			}
			if keep {
				kept = append(kept, n)
			}
		}
		nodes = kept
	}
	return nodes, nil
}
