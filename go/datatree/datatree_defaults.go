// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree

import (
	"encoding/json"
	"fmt"
	"slices"

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
//
// Matcher operational errors are returned with their original typed cause. On
// error the tree, including nested values and ordering, is unchanged. Predicates
// see the same initial data view as before default application; defaults are not
// reevaluated to a fixed point. Unsupported XPath guards still omit defaults.
func (t *Tree) ApplyDefaults() error {
	candidate := *t
	candidate.roots = cloneNodes(t.roots)
	root := candidate.xroot()
	roots, err := applyDefaultsLevel(root, root, topLevelSchema(t.modules...), candidate.roots)
	if err != nil {
		return err
	}
	t.roots = roots
	return nil
}

// Clone all mutable structure; token byte slices are immutable and every value
// update replaces the token. No candidate mutation can alter the original tree.
func cloneNodes(nodes []*node) []*node {
	out := make([]*node, len(nodes))
	for i, original := range nodes {
		n := *original
		n.children = cloneNodes(original.children)
		n.values = slices.Clone(original.values)
		n.entries = make([][]*node, len(original.entries))
		for j, entry := range original.entries {
			n.entries[j] = cloneNodes(entry)
		}
		out[i] = &n
	}
	return out
}

func applyDefaultsLevel(root, parent *xnode, schema []cambium.SchemaNodeRef, data []*node) ([]*node, error) {
	return appendDefaults(nil, root, parent, schema, indexLevel(data))
}

// appendDefaults appends to out, in schema declaration order, the data present
// at this level with defaults filled below it, plus the absent leaves and
// leaf-lists whose defaults are in use.
func appendDefaults(out []*node, root, parent *xnode, schema []cambium.SchemaNodeRef, present levelIndex) ([]*node, error) {
	for _, sn := range schema {
		if sn.IsChoice() {
			effective, ok := effectiveCase(sn, present)
			for _, c := range choiceCases(sn) {
				if ok && c == effective {
					var err error
					out, err = appendDefaults(out, root, parent, caseSchema(c), present)
					if err != nil {
						return nil, err
					}
				} else {
					out = appendCaseData(out, c, present, ok)
				}
			}
			continue
		}
		if dn := present.node(sn); dn != nil {
			if err := applyDefaultsBelow(root, parent, sn, dn); err != nil {
				return nil, err
			}
			out = append(out, dn)
			continue
		}
		var added *node
		switch {
		case sn.IsLeaf():
			if def, ok := sn.DefaultEntry(); ok {
				active, err := schemaNodeActiveForMissing(root, parent, sn)
				if err != nil {
					return nil, err
				}
				if active {
					added, err = defaultLeafNode(sn, def)
					if err != nil {
						return nil, err
					}
				}
			}
		case sn.IsLeafList():
			if defs := sn.DefaultEntries(); len(defs) > 0 {
				active, err := schemaNodeActiveForMissing(root, parent, sn)
				if err != nil {
					return nil, err
				}
				if active {
					added, err = defaultLeafListNode(sn, defs)
					if err != nil {
						return nil, err
					}
				}
			}
		}
		if added != nil {
			out = append(out, added)
		}
	}
	return out, nil
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
func applyDefaultsBelow(root, parent *xnode, sn cambium.SchemaNodeRef, dn *node) error {
	switch {
	case sn.IsContainer():
		children, err := applyDefaultsLevel(root, firstMatchingChildXNode(parent, sn), levelSchema(sn), dn.children)
		if err != nil {
			return err
		}
		dn.children = children
	case sn.IsList():
		listNodes := matchingChildXNodes(parent, sn)
		for i := range dn.entries {
			var listParent *xnode
			if i < len(listNodes) {
				listParent = listNodes[i]
			}
			filled, err := applyDefaultsLevel(root, listParent, levelSchema(sn), dn.entries[i])
			if err != nil {
				return err
			}
			dn.entries[i] = keysFirst(sn, filled)
		}
	}
	return nil
}

func defaultLeafNode(sn cambium.SchemaNodeRef, def cambium.DefaultValue) (*node, error) {
	n := newNode(sn)
	n.kind = kindLeaf
	value, err := defaultToken(sn, def)
	if err != nil {
		return nil, err
	}
	n.value = value
	return n, nil
}

func defaultLeafListNode(sn cambium.SchemaNodeRef, defs []cambium.DefaultValue) (*node, error) {
	n := newNode(sn)
	n.kind = kindLeafList
	for _, def := range defs {
		value, err := defaultToken(sn, def)
		if err != nil {
			return nil, err
		}
		n.values = append(n.values, value)
	}
	if err := sortSystemOrdered(sn, n); err != nil {
		return nil, err
	}
	return n, nil
}

// defaultToken returns the canonical token of a leaf or leaf-list default,
// resolving its prefixes in the module that declared it.
func defaultToken(sn cambium.SchemaNodeRef, def cambium.DefaultValue) (json.RawMessage, error) {
	ti, _ := sn.LeafType()
	source := def.SourceModule()
	if source.Name() == "" {
		source = sn.Module()
	}
	token, err := defaultValueToken(ti, def.Value(), sn.Module(), schemaScope{module: source})
	if err != nil {
		return nil, fmt.Errorf("%s: default: %w", sn.Path(), err)
	}
	return canonicalLeafToken(sn, token)
}

// defaultValueToken converts a schema default value to its instance-data JSON
// token. It differs from jsonTokenFromText, the XML data-value conversion, only
// in integer notation: a default written in a module may use RFC 7950 section
// 9.2.1 hexadecimal or octal notation, which instance data may not, and is
// instantiated as its canonical decimal value.
func defaultValueToken(ti cambium.TypeInfo, text string, leafModule cambium.Module, scope valueScope) (json.RawMessage, error) {
	switch r := ti.Resolved().(type) {
	case cambium.ResolvedInt:
		if canonical, ok := cambium.CanonicalIntegerDefault(text, r.Kind); ok {
			return jsonTokenFromIntegerText(canonical, r.Kind), nil
		}
	case cambium.ResolvedUnion:
		for _, member := range r.Members() {
			token, err := defaultValueToken(member, text, leafModule, scope)
			if err != nil {
				return nil, err
			}
			var trial []string
			if err := validateLeafValue(member, token, "", leafModule.Name(), &trial); err != nil {
				return nil, err
			}
			if len(trial) == 0 {
				return token, nil
			}
		}
	case cambium.ResolvedLeafRef:
		if rt, ok := r.Realtype(); ok && rt != nil {
			return defaultValueToken(*rt, text, leafModule, scope)
		}
	}
	return jsonTokenFromText(ti, text, leafModule, scope)
}
