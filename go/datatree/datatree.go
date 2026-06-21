// Package datatree is a pure-Go, cgo-free generic data tree for YANG instance
// data. It parses a document against a Cambium schema into an ordered tree and
// serializes it back in effective schema declaration order, so it requires no
// libyang/cgo for generic data round-tripping.
//
// Order is structural here exactly as in the schema tier: a node's children are
// stored as an ordered slice (never a map as the traversal source), output
// follows schema declaration order (invariant I2), list keys are emitted first
// in key-statement order (I3), and leaf-list / list element order from the input
// is preserved. Input member order is irrelevant — output order comes from the
// schema.
//
// JSON_IETF (RFC 7951) and XML round-trip containers, leaves, leaf-lists, and
// lists. Leaf values are held as JSON tokens, with type-aware validation and XML
// text conversion layered on that representation. choice nodes are flattened
// (RFC 7950 §7.9); anydata/anyxml and operations are not yet handled.
package datatree

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// Format selects a data encoding.
type Format int

const (
	// FormatJSONIETF is RFC 7951 JSON encoding.
	FormatJSONIETF Format = iota
	// FormatXML is RFC 7950 / NETCONF-style XML encoding.
	FormatXML
)

// Tree is an ordered, schema-bound generic data tree.
type Tree struct {
	module cambium.Module
	roots  []*node
}

type nodeKind int

const (
	kindLeaf nodeKind = iota
	kindLeafList
	kindContainer
	kindList
)

// node is one data node. Children/entries are stored in the order they will be
// serialized (schema declaration order, keys first for list entries).
type node struct {
	name      string // schema node local name
	module    string // module name owning this node (for JSON_IETF qualification)
	namespace string // module namespace URI (for XML xmlns)
	kind      nodeKind

	value    json.RawMessage   // kindLeaf: the scalar, normalized to compact JSON
	values   []json.RawMessage // kindLeafList: ordered values
	children []*node           // kindContainer: ordered children
	entries  [][]*node         // kindList: each entry is its ordered child nodes
}

type nodeKey struct {
	module string
	name   string
}

func schemaNodeKey(sn cambium.SchemaNodeRef) nodeKey {
	return nodeKey{module: sn.Module().Name(), name: sn.Name()}
}

func dataNodeKey(n *node) nodeKey {
	if n == nil {
		return nodeKey{}
	}
	return nodeKey{module: n.module, name: n.name}
}

// Parse decodes data against schema m into an ordered Tree.
func Parse(m cambium.Module, f Format, data []byte) (*Tree, error) {
	switch f {
	case FormatJSONIETF:
		raw, err := decodeJSONObject("root", data)
		if err != nil {
			return nil, err
		}
		roots, err := parseChildren(flattenTopLevel(m), raw, "")
		if err != nil {
			return nil, err
		}
		return &Tree{module: m, roots: roots}, nil
	case FormatXML:
		return parseXML(m, data)
	default:
		return nil, fmt.Errorf("datatree: unsupported format %d", f)
	}
}

// parseChildren builds nodes for the schema children present in raw, in schema
// declaration order. Unknown members (not matching any schema child) are an
// error, so no data is silently dropped.
func parseChildren(children []cambium.SchemaNodeRef, raw map[string]json.RawMessage, parentModule string) ([]*node, error) {
	consumed := make(map[string]bool, len(raw))
	var out []*node
	for _, sn := range children {
		member, val, ok := lookupMember(sn, raw, parentModule)
		if !ok {
			continue
		}
		consumed[member] = true
		n, err := parseNode(sn, val)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	for key := range raw {
		if !consumed[key] {
			return nil, fmt.Errorf("datatree: unknown member %q (no matching schema node)", key)
		}
	}
	return out, nil
}

// lookupMember returns the raw value for a schema node under its
// module-qualified JSON_IETF name ("module:name"), or under its bare local name
// only when the node is in the same module as the enclosing data node. Root
// members are therefore always qualified, and augmentation/module-change children
// cannot be accidentally matched by bare local-name collisions.
func lookupMember(sn cambium.SchemaNodeRef, raw map[string]json.RawMessage, parentModule string) (string, json.RawMessage, bool) {
	qualified := sn.Module().Name() + ":" + sn.Name()
	if v, ok := raw[qualified]; ok {
		return qualified, v, true
	}
	if parentModule != "" && sn.Module().Name() == parentModule {
		v, ok := raw[sn.Name()]
		if !ok {
			return "", nil, false
		}
		return sn.Name(), v, true
	}
	return "", nil, false
}

// flattenTopLevel returns the module's top-level data nodes with choice/case
// nodes flattened away (their data children spliced in at the choice's
// position), matching how SchemaNodeRef.DataChildren(true) treats nested levels.
// TopLevel() alone does not flatten, so without this a leaf inside a top-level
// choice would be unreachable.
func flattenTopLevel(m cambium.Module) []cambium.SchemaNodeRef {
	var out []cambium.SchemaNodeRef
	for n := range m.TopLevel().Iter() {
		out = appendFlattened(out, n)
	}
	return out
}

func appendFlattened(out []cambium.SchemaNodeRef, n cambium.SchemaNodeRef) []cambium.SchemaNodeRef {
	if n.IsChoice() || n.IsCase() {
		for c := range n.DataChildren(true).Iter() {
			out = appendFlattened(out, c)
		}
		return out
	}
	return append(out, n)
}

func childRefs(children cambium.SchemaChildren) []cambium.SchemaNodeRef {
	var out []cambium.SchemaNodeRef
	for n := range children.Iter() {
		out = append(out, n)
	}
	return out
}

func parseNode(sn cambium.SchemaNodeRef, raw json.RawMessage) (*node, error) {
	n := &node{name: sn.Name(), module: sn.Module().Name(), namespace: sn.Namespace()}
	switch {
	case sn.IsLeaf():
		n.kind = kindLeaf
		v, err := compactJSON(raw)
		if err != nil {
			return nil, fmt.Errorf("datatree: leaf %q: %w", sn.Name(), err)
		}
		n.value = v
	case sn.IsLeafList():
		n.kind = kindLeafList
		elems, err := splitArray(raw)
		if err != nil {
			return nil, fmt.Errorf("datatree: leaf-list %q: %w", sn.Name(), err)
		}
		for _, e := range elems {
			v, err := compactJSON(e)
			if err != nil {
				return nil, fmt.Errorf("datatree: leaf-list %q element: %w", sn.Name(), err)
			}
			n.values = append(n.values, v)
		}
	case sn.IsContainer():
		n.kind = kindContainer
		obj, err := decodeJSONObject(fmt.Sprintf("container %q", sn.Name()), raw)
		if err != nil {
			return nil, err
		}
		kids, err := parseChildren(childRefs(sn.DataChildren(true)), obj, sn.Module().Name())
		if err != nil {
			return nil, err
		}
		n.children = kids
	case sn.IsList():
		n.kind = kindList
		elems, err := splitArray(raw)
		if err != nil {
			return nil, fmt.Errorf("datatree: list %q: %w", sn.Name(), err)
		}
		for _, e := range elems {
			obj, err := decodeJSONObject(fmt.Sprintf("list %q entry", sn.Name()), e)
			if err != nil {
				return nil, err
			}
			kids, err := parseChildren(childRefs(sn.DataChildren(true)), obj, sn.Module().Name())
			if err != nil {
				return nil, err
			}
			n.entries = append(n.entries, keysFirst(sn, kids))
		}
	default:
		return nil, fmt.Errorf("datatree: unsupported node kind for %q in this slice", sn.Name())
	}
	return n, nil
}

// keysFirst reorders a list entry's child nodes so the key leaves come first in
// key-statement order (invariant I3), followed by the remaining children in
// their original (declaration) order.
func keysFirst(list cambium.SchemaNodeRef, kids []*node) []*node {
	keys := list.KeyNames()
	if len(keys) == 0 {
		return kids
	}
	rank := make(map[string]int, len(keys))
	for i, k := range keys {
		rank[k] = i
	}
	ordered := make([]*node, len(kids))
	copy(ordered, kids)
	sort.SliceStable(ordered, func(i, j int) bool {
		ri, iok := rank[ordered[i].name]
		rj, jok := rank[ordered[j].name]
		switch {
		case iok && jok:
			return ri < rj // both keys: key-statement order
		case iok != jok:
			return iok // keys before non-keys
		default:
			return false // non-keys keep declaration order (stable sort)
		}
	})
	return ordered
}

// Serialize encodes the tree in schema declaration order.
func (t *Tree) Serialize(f Format) ([]byte, error) {
	switch f {
	case FormatJSONIETF:
		var b bytes.Buffer
		if err := writeObject(&b, t.roots, ""); err != nil {
			return nil, err
		}
		return b.Bytes(), nil
	case FormatXML:
		return t.serializeXML()
	default:
		return nil, fmt.Errorf("datatree: unsupported format %d", f)
	}
}

// writeObject emits an ordered JSON object. parentModule is the module of the
// enclosing node ("" at the root, which forces every top-level member to be
// module-qualified per RFC 7951 §4).
func writeObject(b *bytes.Buffer, nodes []*node, parentModule string) error {
	b.WriteByte('{')
	for i, n := range nodes {
		if i > 0 {
			b.WriteByte(',')
		}
		writeMemberName(b, n, parentModule)
		b.WriteByte(':')
		if err := writeValue(b, n); err != nil {
			return err
		}
	}
	b.WriteByte('}')
	return nil
}

func writeMemberName(b *bytes.Buffer, n *node, parentModule string) {
	name := n.name
	if n.module != parentModule {
		name = n.module + ":" + n.name
	}
	b.WriteString(strconvQuote(name))
}

func writeValue(b *bytes.Buffer, n *node) error {
	switch n.kind {
	case kindLeaf:
		b.Write(n.value)
	case kindLeafList:
		b.WriteByte('[')
		for i, v := range n.values {
			if i > 0 {
				b.WriteByte(',')
			}
			b.Write(v)
		}
		b.WriteByte(']')
	case kindContainer:
		return writeObject(b, n.children, n.module)
	case kindList:
		b.WriteByte('[')
		for i, entry := range n.entries {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeObject(b, entry, n.module); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	}
	return nil
}

func compactJSON(raw json.RawMessage) (json.RawMessage, error) {
	var b bytes.Buffer
	if err := json.Compact(&b, raw); err != nil {
		return nil, err
	}
	return json.RawMessage(b.Bytes()), nil
}

func splitArray(raw json.RawMessage) ([]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, fmt.Errorf("expected JSON array")
	}
	var elems []json.RawMessage
	if err := json.Unmarshal(trimmed, &elems); err != nil {
		return nil, err
	}
	return elems, nil
}

func decodeJSONObject(context string, raw []byte) (map[string]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("datatree: %s: %w", context, err)
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, fmt.Errorf("datatree: %s: expected JSON object", context)
	}
	obj := make(map[string]json.RawMessage)
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("datatree: %s: read member name: %w", context, err)
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("datatree: %s: expected JSON object member name", context)
		}
		if _, exists := obj[key]; exists {
			return nil, fmt.Errorf("datatree: %s: duplicate member %q", context, key)
		}
		var val json.RawMessage
		if err := dec.Decode(&val); err != nil {
			return nil, fmt.Errorf("datatree: %s member %q: %w", context, key, err)
		}
		obj[key] = val
	}
	tok, err = dec.Token()
	if err != nil {
		return nil, fmt.Errorf("datatree: %s: close object: %w", context, err)
	}
	delim, ok = tok.(json.Delim)
	if !ok || delim != '}' {
		return nil, fmt.Errorf("datatree: %s: expected end of JSON object", context)
	}
	if tok, err = dec.Token(); err != io.EOF {
		if err != nil {
			return nil, fmt.Errorf("datatree: %s: trailing data: %w", context, err)
		}
		return nil, fmt.Errorf("datatree: %s: unexpected trailing JSON token %v", context, tok)
	}
	return obj, nil
}

// strconvQuote JSON-quotes a member name. Member names are YANG identifiers
// (optionally module-qualified), so they need no escaping, but we route through
// the JSON encoder to stay correct if that ever changes.
func strconvQuote(s string) string {
	if !strings.ContainsAny(s, `"\`+"\n\r\t") {
		return `"` + s + `"`
	}
	encoded, _ := json.Marshal(s)
	return string(encoded)
}
