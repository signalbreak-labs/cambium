// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree

import (
	"encoding/json"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// ApplyDefaults fills in absent leaves and leaf-lists that carry a schema
// default with that default value, in effective schema declaration order (RFC
// 6243 report-all, scoped to subtrees that are present). It recurses into
// present containers and list entries; absent containers and lists are not
// materialized. Existing values are never overwritten, and it is idempotent.
// At the top level it fills the defaults of every module the tree is bound to
// (see ParseModules).
//
// Inside a choice, defaults are filled only in the case whose data exists or,
// when no case has data, in the choice's default case (RFC 7950 §7.6.1,
// §7.9.3), recursively through nested choices. Once a case is in effect, an
// empty non-presence container of another case is dropped, as libyang does:
// it holds no data and would otherwise count as data of a second case.
//
// Order is preserved: each level is rebuilt in schema declaration order, list
// entries keep their keys-first ordering (I3), and ordered-by system
// leaf-list defaults are put in canonical order (I2).
func (t *Tree) ApplyDefaults() {
	root := t.xroot()
	t.roots = applyDefaultsLevel(root, root, topLevelSchema(t.modules...), t.roots)
}

func applyDefaultsLevel(root, parent *xnode, schema []cambium.SchemaNodeRef, data []*node) []*node {
	return appendDefaults(nil, root, parent, schema, indexLevel(data))
}

// appendDefaults appends to out, in schema declaration order, the data present
// at this level with defaults filled below it, plus the absent leaves and
// leaf-lists whose defaults are in use.
func appendDefaults(out []*node, root, parent *xnode, schema []cambium.SchemaNodeRef, present levelIndex) []*node {
	for _, sn := range schema {
		if sn.IsChoice() {
			effective, ok := effectiveCase(sn, present)
			for _, c := range choiceCases(sn) {
				if ok && c == effective {
					out = appendDefaults(out, root, parent, caseSchema(c), present)
				} else {
					out = appendCaseData(out, c, present, ok)
				}
			}
			continue
		}
		if dn := present.node(sn); dn != nil {
			applyDefaultsBelow(root, parent, sn, dn)
			out = append(out, dn)
			continue
		}
		switch {
		case sn.IsLeaf():
			if def, ok := sn.DefaultEntry(); ok && schemaNodeActiveForMissing(root, parent, sn) {
				out = append(out, defaultLeafNode(sn, def))
			}
		case sn.IsLeafList():
			if defs := sn.DefaultEntries(); len(defs) > 0 && schemaNodeActiveForMissing(root, parent, sn) {
				out = append(out, defaultLeafListNode(sn, defs))
			}
		}
	}
	return out
}

// appendCaseData appends the data present in a case that is not in effect,
// unchanged. When another case is in effect (dropEmpty), a node that carries
// no data is dropped.
func appendCaseData(out []*node, c cambium.SchemaNodeRef, present levelIndex, dropEmpty bool) []*node {
	for _, sn := range appendFlattened(nil, c) {
		if dn := present.node(sn); dn != nil && (!dropEmpty || carriesData(dn)) {
			out = append(out, dn)
		}
	}
	return out
}

// applyDefaultsBelow fills defaults inside a present container or in each
// entry of a present list.
func applyDefaultsBelow(root, parent *xnode, sn cambium.SchemaNodeRef, dn *node) {
	switch {
	case sn.IsContainer():
		dn.children = applyDefaultsLevel(root, firstMatchingChildXNode(parent, sn), levelSchema(sn), dn.children)
	case sn.IsList():
		listNodes := matchingChildXNodes(parent, sn)
		for i := range dn.entries {
			var listParent *xnode
			if i < len(listNodes) {
				listParent = listNodes[i]
			}
			filled := applyDefaultsLevel(root, listParent, levelSchema(sn), dn.entries[i])
			dn.entries[i] = keysFirst(sn, filled)
		}
	}
}

func defaultLeafNode(sn cambium.SchemaNodeRef, def cambium.DefaultValue) *node {
	n := newNode(sn)
	n.kind = kindLeaf
	n.value = defaultToken(sn, def)
	return n
}

func defaultLeafListNode(sn cambium.SchemaNodeRef, defs []cambium.DefaultValue) *node {
	n := newNode(sn)
	n.kind = kindLeafList
	for _, def := range defs {
		n.values = append(n.values, defaultToken(sn, def))
	}
	sortSystemOrdered(sn, n)
	return n
}

// defaultToken returns the canonical token of a leaf or leaf-list default,
// resolving its prefixes in the module that declared it.
func defaultToken(sn cambium.SchemaNodeRef, def cambium.DefaultValue) json.RawMessage {
	ti, _ := sn.LeafType()
	source := def.SourceModule()
	if source.Name() == "" {
		source = sn.Module()
	}
	return canonicalLeafToken(sn, jsonTokenFromText(ti, def.Value(), sn.Module(), schemaScope{module: source}))
}
