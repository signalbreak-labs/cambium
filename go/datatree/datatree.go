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
// Slice 1a scope: JSON_IETF (RFC 7951) round-trip of containers, leaves,
// leaf-lists, and lists. Leaf values are preserved verbatim (normalized to
// compact JSON); type-aware validation and encoding are a later slice. choice
// nodes are flattened (RFC 7950 §7.9); anydata/anyxml and operations are not yet
// handled.
package datatree

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// Format selects a data encoding. Only FormatJSONIETF is supported in this slice.
type Format int

const (
	// FormatJSONIETF is RFC 7951 JSON encoding.
	FormatJSONIETF Format = iota
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
	name   string // schema node local name
	module string // name of the module owning this node (for JSON_IETF qualification)
	kind   nodeKind

	value    json.RawMessage   // kindLeaf: the scalar, normalized to compact JSON
	values   []json.RawMessage // kindLeafList: ordered values
	children []*node           // kindContainer: ordered children
	entries  [][]*node         // kindList: each entry is its ordered child nodes
}

// Parse decodes data against schema m into an ordered Tree.
func Parse(m cambium.Module, f Format, data []byte) (*Tree, error) {
	if f != FormatJSONIETF {
		return nil, fmt.Errorf("datatree: unsupported format %d", f)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("datatree: parse root object: %w", err)
	}
	roots, err := parseChildren(m.TopLevel(), raw)
	if err != nil {
		return nil, err
	}
	return &Tree{module: m, roots: roots}, nil
}

// parseChildren builds nodes for the schema children present in raw, in schema
// declaration order. Unknown members (not matching any schema child) are an
// error, so no data is silently dropped.
func parseChildren(children cambium.SchemaChildren, raw map[string]json.RawMessage) ([]*node, error) {
	consumed := make(map[string]bool, len(raw))
	var out []*node
	for sn := range children.Iter() {
		member, val, ok := lookupMember(sn, raw)
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

// lookupMember returns the raw value for a schema node under either its
// module-qualified JSON_IETF name ("module:name") or its bare name.
func lookupMember(sn cambium.SchemaNodeRef, raw map[string]json.RawMessage) (string, json.RawMessage, bool) {
	qualified := sn.Module().Name() + ":" + sn.Name()
	if v, ok := raw[qualified]; ok {
		return qualified, v, true
	}
	if v, ok := raw[sn.Name()]; ok {
		return sn.Name(), v, true
	}
	return "", nil, false
}

func parseNode(sn cambium.SchemaNodeRef, raw json.RawMessage) (*node, error) {
	n := &node{name: sn.Name(), module: sn.Module().Name()}
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
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil, fmt.Errorf("datatree: container %q: %w", sn.Name(), err)
		}
		kids, err := parseChildren(sn.DataChildren(true), obj)
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
			var obj map[string]json.RawMessage
			if err := json.Unmarshal(e, &obj); err != nil {
				return nil, fmt.Errorf("datatree: list %q entry: %w", sn.Name(), err)
			}
			kids, err := parseChildren(sn.DataChildren(true), obj)
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
	if f != FormatJSONIETF {
		return nil, fmt.Errorf("datatree: unsupported format %d", f)
	}
	var b bytes.Buffer
	if err := writeObject(&b, t.roots, ""); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
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
	var elems []json.RawMessage
	if err := json.Unmarshal(raw, &elems); err != nil {
		return nil, err
	}
	return elems, nil
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
