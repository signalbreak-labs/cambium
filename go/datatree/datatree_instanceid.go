// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// checkInstanceIdentifier validates a require-instance instance-identifier leaf
// (O8 slice 4c): the value is itself a restricted XPath naming an instance that
// must exist. The value is parsed and evaluated with the XPath engine; if it
// resolves to an empty node-set the instance is missing.
//
// A value in canonical JSON_IETF form is first resolved against the schema and
// evaluated with each node's namespace bound explicitly, so module-name
// qualifiers resolve whatever the leaf's module imports. A value that does not
// resolve against the schema is evaluated as written, with the leaf module's
// import prefixes; anything still unresolvable makes evaluation error out and
// the check is SKIPPED — never a wrong verdict.
func checkInstanceIdentifier(ev *evaluator, n *xnode, out *[]string) {
	if !n.hasSchema || !n.leaf {
		return
	}
	ti, ok := n.schema.LeafType()
	if !ok {
		return
	}
	ii, ok := ti.Resolved().(cambium.ResolvedInstanceIdentifier)
	if !ok || !ii.RequireInstance {
		return
	}
	expr := n.value
	if steps, ok := resolveJSONInstanceIdentifier(n.value, n.schema.Module()); ok {
		if xp, decls, ok := renderInstanceIdentifierPrefixed(steps, true); ok {
			expr = xp
			ev.prefixNS = make(map[string]string, len(decls))
			for _, d := range decls {
				ev.prefixNS[d.prefix] = d.uri
			}
		}
	}
	ast, err := parseXPath(expr)
	if err != nil {
		return // unparseable instance-identifier: skip
	}
	v, err := ev.eval(ast, ectx{node: n, pos: 1, size: 1})
	if err != nil || v.kind != kNodeset {
		return // unresolvable: skip
	}
	if len(v.ns) == 0 {
		*out = append(*out, fmt.Sprintf("%s: instance-identifier %q references a non-existent instance", xnodePath(n), n.value))
	}
}

// Instance-identifier values (RFC 7950 §9.13) are paths to data nodes, and
// their text depends on the encoding: XML qualifies every node name with a
// prefix bound by the document's xmlns declarations, JSON_IETF (RFC 7951
// §6.11) qualifies the first node, and each node whose module differs from its
// parent's, with the module name. datatree keeps the canonical JSON_IETF form
// (as libyang does) and converts at the format boundary by resolving each node
// against the schema. A value that does not resolve is kept as written.

type iidPredKind uint8

const (
	iidPredKey      iidPredKind = iota // [key='value']
	iidPredLeafList                    // [.='value']
	iidPredPos                         // [N]
)

// iidPred is one predicate as written.
type iidPred struct {
	kind  iidPredKind
	qual  string // key qualifier ("" when absent)
	name  string // key name
	value string // unquoted value, or the position digits
}

// iidStep is one path node as written.
type iidStep struct {
	qual  string // XML prefix or JSON module name ("" when absent)
	name  string
	preds []iidPred
}

// parseInstanceIdentifier splits an instance-identifier into its steps,
// keeping qualifiers as written. Whitespace is allowed inside predicates.
func parseInstanceIdentifier(s string) ([]iidStep, bool) {
	p := iidParser{s: s}
	var steps []iidStep
	for p.i < len(p.s) {
		if !p.eat('/') {
			return nil, false
		}
		qual, name, ok := p.qname()
		if !ok {
			return nil, false
		}
		step := iidStep{qual: qual, name: name}
		for p.eat('[') {
			p.skipWS()
			var pred iidPred
			switch {
			case p.eat('.'):
				pred.kind = iidPredLeafList
			case p.i < len(p.s) && p.s[p.i] >= '1' && p.s[p.i] <= '9':
				start := p.i
				for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
					p.i++
				}
				pred.kind, pred.value = iidPredPos, p.s[start:p.i]
			default:
				pred.kind = iidPredKey
				if pred.qual, pred.name, ok = p.qname(); !ok {
					return nil, false
				}
			}
			if pred.kind != iidPredPos {
				p.skipWS()
				if !p.eat('=') {
					return nil, false
				}
				p.skipWS()
				if pred.value, ok = p.quoted(); !ok {
					return nil, false
				}
			}
			p.skipWS()
			if !p.eat(']') {
				return nil, false
			}
			step.preds = append(step.preds, pred)
		}
		steps = append(steps, step)
	}
	return steps, len(steps) > 0
}

type iidParser struct {
	s string
	i int
}

func (p *iidParser) eat(c byte) bool {
	if p.i < len(p.s) && p.s[p.i] == c {
		p.i++
		return true
	}
	return false
}

func (p *iidParser) skipWS() {
	for p.i < len(p.s) && strings.IndexByte(" \t\n\r", p.s[p.i]) >= 0 {
		p.i++
	}
}

// qname reads [qualifier ":"] identifier.
func (p *iidParser) qname() (qual, name string, ok bool) {
	if name, ok = p.ident(); !ok {
		return "", "", false
	}
	if p.eat(':') {
		qual = name
		if name, ok = p.ident(); !ok {
			return "", "", false
		}
	}
	return qual, name, true
}

// ident reads a YANG identifier: [A-Za-z_][A-Za-z0-9_.-]*.
func (p *iidParser) ident() (string, bool) {
	start := p.i
	for p.i < len(p.s) {
		c := p.s[p.i]
		letter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_'
		if !letter && (p.i == start || (c < '0' || c > '9') && c != '-' && c != '.') {
			break
		}
		p.i++
	}
	return p.s[start:p.i], p.i > start
}

func (p *iidParser) quoted() (string, bool) {
	if p.i >= len(p.s) || (p.s[p.i] != '\'' && p.s[p.i] != '"') {
		return "", false
	}
	q := p.s[p.i]
	end := strings.IndexByte(p.s[p.i+1:], q)
	if end < 0 {
		return "", false
	}
	v := p.s[p.i+1 : p.i+1+end]
	p.i += end + 2
	return v, true
}

// iidNode is a path node resolved against the schema.
type iidNode struct {
	node  cambium.SchemaNodeRef
	preds []iidValuePred
}

// iidValuePred is a predicate whose value is held as a canonical JSON token of
// the key (or leaf-list) type.
type iidValuePred struct {
	kind  iidPredKind
	leaf  cambium.SchemaNodeRef // the key leaf, or the leaf-list itself
	token json.RawMessage
	pos   string
}

// iidNames abstracts how a format qualifies names and writes predicate values.
type iidNames interface {
	// matches reports whether schema node sn is the one written with qualifier
	// qual; parent is the module of the preceding path node (zero at the root).
	matches(sn cambium.SchemaNodeRef, qual string, parent cambium.Module) bool
	// token converts a predicate value as written to a JSON token of type ti.
	token(ti cambium.TypeInfo, text string, leafModule cambium.Module) json.RawMessage
}

// lexicalNames: XML (and YANG schema) form, where each qualifier is a prefix
// bound in scope.
type lexicalNames struct{ scope valueScope }

func (l lexicalNames) matches(sn cambium.SchemaNodeRef, qual string, _ cambium.Module) bool {
	ns, ok := l.scope.namespace(qual)
	return ok && ns == sn.Namespace()
}

func (l lexicalNames) token(ti cambium.TypeInfo, text string, leafModule cambium.Module) json.RawMessage {
	return jsonTokenFromText(ti, text, leafModule, l.scope)
}

// jsonNames: JSON_IETF form, where a qualifier is a module name and an absent
// one is inherited from the parent node.
type jsonNames struct{ modules []cambium.Module }

func (j jsonNames) matches(sn cambium.SchemaNodeRef, qual string, parent cambium.Module) bool {
	if qual == "" {
		qual = parent.Name()
	}
	return qual != "" && sn.Module().Name() == qual
}

func (j jsonNames) token(ti cambium.TypeInfo, text string, leafModule cambium.Module) json.RawMessage {
	return jsonTokenFromText(ti, text, leafModule, moduleNameScope{modules: j.modules, local: leafModule})
}

// moduleNameScope resolves JSON_IETF module-name qualifiers (identityref values
// inside predicates) among known modules; "" is the local module.
type moduleNameScope struct {
	modules []cambium.Module
	local   cambium.Module
}

func (s moduleNameScope) namespace(name string) (string, bool) {
	if name == "" {
		return s.local.Namespace(), true
	}
	for _, m := range s.modules {
		if m.Name() == name {
			return m.Namespace(), true
		}
	}
	return "", false
}

// resolveInstanceIdentifier resolves parsed steps against the schema reachable
// from leafModule: the top-level nodes of leafModule and the modules it imports
// (transitively), then each node's data children, augmentations included.
func resolveInstanceIdentifier(steps []iidStep, leafModule cambium.Module, names iidNames) ([]iidNode, bool) {
	var candidates []cambium.SchemaNodeRef
	for _, m := range moduleClosure(leafModule) {
		candidates = append(candidates, flattenTopLevel(m)...)
	}
	var parent cambium.Module
	out := make([]iidNode, 0, len(steps))
	for _, st := range steps {
		node, ok := findIIDNode(candidates, st.name, st.qual, parent, names)
		if !ok {
			return nil, false
		}
		resolved := iidNode{node: node}
		for _, p := range st.preds {
			pred := iidValuePred{kind: p.kind, pos: p.value}
			switch p.kind {
			case iidPredKey:
				if !node.IsList() {
					return nil, false
				}
				if pred.leaf, ok = findIIDNode(childRefs(node.ListKeys()), p.name, p.qual, node.Module(), names); !ok {
					return nil, false
				}
			case iidPredLeafList:
				if !node.IsLeafList() {
					return nil, false
				}
				pred.leaf = node
			}
			if p.kind != iidPredPos {
				ti, ok := pred.leaf.LeafType()
				if !ok {
					return nil, false
				}
				pred.token = canonicalToken(ti, names.token(ti, p.value, pred.leaf.Module()), pred.leaf.Module())
			}
			resolved.preds = append(resolved.preds, pred)
		}
		out = append(out, resolved)
		parent = node.Module()
		candidates = childRefs(node.DataChildren(true))
	}
	return out, true
}

func findIIDNode(candidates []cambium.SchemaNodeRef, name, qual string, parent cambium.Module, names iidNames) (cambium.SchemaNodeRef, bool) {
	for _, c := range candidates {
		if c.Name() == name && names.matches(c, qual, parent) {
			return c, true
		}
	}
	return cambium.SchemaNodeRef{}, false
}

// moduleClosure returns m and every module it imports, transitively, in
// breadth-first import order.
func moduleClosure(m cambium.Module) []cambium.Module {
	out := []cambium.Module{m}
	seen := map[string]bool{m.Name(): true}
	for i := 0; i < len(out); i++ {
		for _, imp := range out[i].Imports() {
			dep, ok := out[i].ResolvePrefix(imp.Prefix)
			if !ok || seen[dep.Name()] {
				continue
			}
			seen[dep.Name()] = true
			out = append(out, dep)
		}
	}
	return out
}

// resolveJSONInstanceIdentifier resolves a value in JSON_IETF form.
func resolveJSONInstanceIdentifier(value string, leafModule cambium.Module) ([]iidNode, bool) {
	steps, ok := parseInstanceIdentifier(value)
	if !ok {
		return nil, false
	}
	return resolveInstanceIdentifier(steps, leafModule, jsonNames{modules: moduleClosure(leafModule)})
}

// instanceIdentifierFromLexical converts an XML (or schema) instance-identifier,
// whose prefixes resolve in scope, to canonical JSON_IETF form.
func instanceIdentifierFromLexical(value string, leafModule cambium.Module, scope valueScope) (string, bool) {
	steps, ok := parseInstanceIdentifier(strings.TrimSpace(value))
	if !ok {
		return "", false
	}
	nodes, ok := resolveInstanceIdentifier(steps, leafModule, lexicalNames{scope: scope})
	if !ok {
		return "", false
	}
	return renderInstanceIdentifierJSON(nodes)
}

// canonicalInstanceIdentifier returns a JSON_IETF instance-identifier in
// canonical form.
func canonicalInstanceIdentifier(value string, leafModule cambium.Module) (string, bool) {
	nodes, ok := resolveJSONInstanceIdentifier(value, leafModule)
	if !ok {
		return "", false
	}
	return renderInstanceIdentifierJSON(nodes)
}

// instanceIdentifierXML converts a JSON_IETF instance-identifier to XML form:
// every node and key qualified with its module's own prefix, each such module
// declared on the element, in order of first use (as libyang prints it).
func instanceIdentifierXML(value string, leafModule cambium.Module) (string, []xmlNSDecl, bool) {
	nodes, ok := resolveJSONInstanceIdentifier(value, leafModule)
	if !ok {
		return "", nil, false
	}
	return renderInstanceIdentifierPrefixed(nodes, false)
}

// renderInstanceIdentifierJSON writes the canonical JSON_IETF form: the module
// name on the first node and wherever it changes, bare key names, values
// single-quoted unless they contain a single quote.
func renderInstanceIdentifierJSON(nodes []iidNode) (string, bool) {
	var b strings.Builder
	prev := ""
	for _, n := range nodes {
		b.WriteByte('/')
		if m := n.node.Module().Name(); m != prev {
			b.WriteString(m)
			b.WriteByte(':')
			prev = m
		}
		b.WriteString(n.node.Name())
		for _, p := range n.preds {
			value := ""
			if p.kind != iidPredPos {
				value = xmlTextFromToken(p.token)
				if id, ok := identityValue(p.leaf, p.token); ok {
					value = id.Module().Name() + ":" + id.Name() // canonical identityref is qualified
				}
			}
			if !writeIIDPred(&b, p, "", value) {
				return "", false
			}
		}
	}
	return b.String(), true
}

// renderInstanceIdentifierPrefixed writes the XML form, returning the prefix
// declarations it uses. With forXPath set, predicate values are written as the
// XPath engine sees leaf values (their JSON_IETF text) rather than in XML form.
func renderInstanceIdentifierPrefixed(nodes []iidNode, forXPath bool) (string, []xmlNSDecl, bool) {
	var decls []xmlNSDecl
	conflict := false
	prefix := func(m cambium.Module) string {
		for _, d := range decls {
			if d.prefix == m.Prefix() {
				conflict = conflict || d.uri != m.Namespace()
				return d.prefix
			}
		}
		decls = append(decls, xmlNSDecl{prefix: m.Prefix(), uri: m.Namespace()})
		return m.Prefix()
	}
	var b strings.Builder
	for _, n := range nodes {
		b.WriteByte('/')
		b.WriteString(prefix(n.node.Module()))
		b.WriteByte(':')
		b.WriteString(n.node.Name())
		for _, p := range n.preds {
			value := ""
			if p.kind != iidPredPos {
				value = xmlTextFromToken(p.token)
				if id, ok := identityValue(p.leaf, p.token); ok && !forXPath {
					value = prefix(id.Module()) + ":" + id.Name()
				}
			}
			keyPrefix := ""
			if p.kind == iidPredKey {
				keyPrefix = prefix(p.leaf.Module())
			}
			if !writeIIDPred(&b, p, keyPrefix, value) {
				return "", nil, false
			}
		}
	}
	if conflict {
		return "", nil, false // two modules share a prefix: no single-element form
	}
	return b.String(), decls, true
}

func writeIIDPred(b *strings.Builder, p iidValuePred, keyPrefix, value string) bool {
	b.WriteByte('[')
	switch p.kind {
	case iidPredPos:
		b.WriteString(p.pos)
		b.WriteByte(']')
		return true
	case iidPredLeafList:
		b.WriteByte('.')
	default:
		if keyPrefix != "" {
			b.WriteString(keyPrefix)
			b.WriteByte(':')
		}
		b.WriteString(p.leaf.Name())
	}
	quote := byte('\'')
	if strings.IndexByte(value, '\'') >= 0 {
		if strings.IndexByte(value, '"') >= 0 {
			return false // not expressible as an XPath literal
		}
		quote = '"'
	}
	b.WriteByte('=')
	b.WriteByte(quote)
	b.WriteString(value)
	b.WriteByte(quote)
	b.WriteByte(']')
	return true
}

// identityValue reports the identity a leaf value names when the value's
// effective type (through unions and leafrefs) is identityref.
func identityValue(leaf cambium.SchemaNodeRef, raw json.RawMessage) (cambium.Identity, bool) {
	ti, ok := leaf.LeafType()
	if !ok {
		return cambium.Identity{}, false
	}
	return identityOfType(ti, raw, leaf.Module())
}

func identityOfType(ti cambium.TypeInfo, raw json.RawMessage, leafModule cambium.Module) (cambium.Identity, bool) {
	switch r := ti.Resolved().(type) {
	case cambium.ResolvedIdentityRef:
		return identityFromJSON(raw, r, leafModule)
	case cambium.ResolvedLeafRef:
		if rt, ok := r.Realtype(); ok && rt != nil {
			return identityOfType(*rt, raw, leafModule)
		}
	case cambium.ResolvedUnion:
		for _, member := range r.Members() {
			var trial []string
			validateLeafValue(member, raw, "", leafModule.Name(), &trial)
			if len(trial) == 0 {
				return identityOfType(member, raw, leafModule)
			}
		}
	}
	return cambium.Identity{}, false
}
