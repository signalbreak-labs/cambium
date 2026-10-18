// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree

import (
	"strings"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// checkUnique enforces a list's unique statements (RFC 7950 §7.8.3): the
// combined values of the referenced leaves, including leaves with default
// values, must differ between all entries in which every referenced leaf
// exists or has a default value in use. Entries are compared in list order and
// each duplicate entry is reported against the first entry it repeats.
func (v *validator) checkUnique(list cambium.SchemaNodeRef, dn *node, listNodes []*xnode, path string) error {
	if len(dn.entries) < 2 {
		return nil
	}
	for _, u := range list.UniqueConstraints() {
		leafs := u.Leafs()
		names := make([]string, len(leafs))
		for i, leaf := range leafs {
			names[i] = pathBelow(list, leaf)
		}
		seen := make(map[string]int, len(dn.entries))
		for i, entry := range dn.entries {
			var entryNode *xnode
			if i < len(listNodes) {
				entryNode = listNodes[i]
			}
			tuple, ok, err := v.uniqueTuple(list, leafs, entry, entryNode)
			if err != nil {
				return err
			}
			if !ok {
				continue // an entry missing a referenced leaf is not constrained
			}
			if first, dup := seen[tuple]; dup {
				v.report("%s[%d]: unique constraint %q is violated: same values as %s[%d]", path, i, strings.Join(names, " "), path, first)
				continue
			}
			seen[tuple] = i
		}
	}
	return nil
}

// uniqueTuple returns the comparison key of an entry's unique leaf values, or
// false when one of the leaves has no value in the entry.
func (v *validator) uniqueTuple(list cambium.SchemaNodeRef, leafs []cambium.SchemaNodeRef, entry []*node, entryNode *xnode) (value string, present bool, err error) {
	var b strings.Builder
	for _, leaf := range leafs {
		value, ok, err := v.entryLeafValue(list, leaf, entry, entryNode)
		if err != nil {
			return "", false, err
		}
		if !ok {
			return "", false, nil
		}
		b.WriteString(value)
		b.WriteByte(0)
	}
	return b.String(), true, nil
}

// entryLeafValue returns the comparison key of a descendant leaf's value in a
// list entry: its value when present, otherwise its default when that is in
// use (RFC 7950 §7.6.1) — every container on the way exists or is a
// non-presence one, every case on the way is the effective case of its
// choice, and the when conditions of the absent nodes hold.
func (v *validator) entryLeafValue(list, leaf cambium.SchemaNodeRef, entry []*node, entryNode *xnode) (value string, present bool, err error) {
	data, parent := entry, entryNode
	chain := schemaChainBelow(list, leaf)
	for i, sn := range chain {
		if sn.IsChoice() {
			// The next node on the way is one of the choice's cases (see
			// choiceCases); it must be the one in effect.
			if c, ok := effectiveCase(sn, indexLevel(data)); !ok || i+1 == len(chain) || c != chain[i+1] {
				return "", false, nil
			}
			continue
		}
		if sn.IsCase() {
			continue
		}
		var n *node
		for _, d := range data {
			if dataNodeKey(d) == schemaNodeKey(sn) {
				n = d
				break
			}
		}
		if n != nil {
			if sn.IsLeaf() {
				value, err := leafValueKey(sn, n.value)
				return value, true, err
			}
			data, parent = n.children, firstMatchingChildXNode(parent, sn)
			continue
		}
		active, err := schemaNodeActiveForMissing(v.root, parent, sn)
		if err != nil {
			return "", false, err
		}
		if !active {
			return "", false, nil
		}
		if sn.IsLeaf() {
			def, ok := sn.DefaultEntry()
			if !ok {
				return "", false, nil
			}
			token, err := defaultToken(sn, def)
			if err != nil {
				return "", false, err
			}
			value, err := leafValueKey(sn, token)
			return value, true, err
		}
		if !sn.IsContainer() || sn.IsPresenceContainer() {
			return "", false, nil
		}
		data, parent = nil, dummyMissingNode(parent, sn)
	}
	return "", false, nil
}

// schemaChainBelow returns the schema nodes from just below ancestor down to
// n (choice and case nodes included), outermost first.
func schemaChainBelow(ancestor, n cambium.SchemaNodeRef) []cambium.SchemaNodeRef {
	var rev []cambium.SchemaNodeRef
	for cur := n; cur != ancestor; {
		rev = append(rev, cur)
		p, ok := cur.Parent()
		if !ok {
			return nil
		}
		cur = p
	}
	out := make([]cambium.SchemaNodeRef, len(rev))
	for i, sn := range rev {
		out[len(rev)-1-i] = sn
	}
	return out
}

// pathBelow renders n's schema path relative to ancestor ("c/v"), as a unique
// statement names it.
func pathBelow(ancestor, n cambium.SchemaNodeRef) string {
	chain := schemaChainBelow(ancestor, n)
	names := make([]string, len(chain))
	for i, sn := range chain {
		names[i] = sn.Name()
	}
	return strings.Join(names, "/")
}
