// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree

import "github.com/signalbreak-labs/cambium/go/cambium"

// choice/case semantics (RFC 7950 §7.9). Data is stored flattened: a case's
// data nodes sit directly among their data parent's children, in schema
// declaration order. Validation and ApplyDefaults instead walk the schema with
// choice and case nodes kept, and derive from the data present at a level
// which case of each choice is in effect.

// levelSchema returns the schema children of a data node with choice and case
// nodes kept, in effective declaration order.
func levelSchema(sn cambium.SchemaNodeRef) []cambium.SchemaNodeRef {
	return childRefs(sn.DataChildren(false))
}

// topLevelSchema returns the module's top-level schema nodes with choice and
// case nodes kept, in effective declaration order.
func topLevelSchema(m cambium.Module) []cambium.SchemaNodeRef {
	return childRefs(m.TopLevel())
}

// levelIndex looks up the data nodes present at one level by schema identity.
// It is a lookup cache only; traversal always follows the schema.
type levelIndex map[nodeKey]*node

func indexLevel(data []*node) levelIndex {
	present := make(levelIndex, len(data))
	for _, d := range data {
		present[dataNodeKey(d)] = d
	}
	return present
}

func (p levelIndex) node(sn cambium.SchemaNodeRef) *node {
	return p[schemaNodeKey(sn)]
}

// choiceCases returns a choice's cases in declaration order. A case is either
// a case node or a shorthand case: a data node or choice directly under the
// choice, which is its own case of the same name (RFC 7950 §7.9.2). The
// schema wraps most shorthand cases in an implicit case node, but not all
// (for example not under a top-level choice), so both forms occur.
func choiceCases(choice cambium.SchemaNodeRef) []cambium.SchemaNodeRef {
	return childRefs(choice.Children())
}

// caseSchema returns the schema nodes of a case (see choiceCases), nested
// choices kept.
func caseSchema(c cambium.SchemaNodeRef) []cambium.SchemaNodeRef {
	if c.IsCase() {
		return levelSchema(c)
	}
	return []cambium.SchemaNodeRef{c}
}

// caseHas reports whether some data node of case c (through nested choices)
// is present at this level and satisfies has.
func caseHas(c cambium.SchemaNodeRef, present levelIndex, has func(*node) bool) bool {
	for _, sn := range appendFlattened(nil, c) {
		if n := present.node(sn); n != nil && has(n) {
			return true
		}
	}
	return false
}

// instantiated reports whether n holds at least one data node instance: an
// empty JSON array for a leaf-list or list creates none.
func instantiated(n *node) bool {
	switch n.kind {
	case kindLeafList:
		return len(n.values) > 0
	case kindList:
		return len(n.entries) > 0
	default:
		return true
	}
}

// carriesData reports whether n is more than an empty non-presence container.
// A non-presence container has no meaning of its own (RFC 7950 §7.5.1), so
// one holding no data does not select its case; libyang deletes it as an
// implicit default node.
func carriesData(n *node) bool {
	if !instantiated(n) {
		return false
	}
	if n.kind != kindContainer || n.schema.IsPresenceContainer() {
		return true
	}
	for _, c := range n.children {
		if carriesData(c) {
			return true
		}
	}
	return false
}

// dataCase returns the case of choice selected by the data at this level: the
// first case, in declaration order, with a node that carries data.
func dataCase(choice cambium.SchemaNodeRef, present levelIndex) (cambium.SchemaNodeRef, bool) {
	for _, c := range choiceCases(choice) {
		if caseHas(c, present, carriesData) {
			return c, true
		}
	}
	return cambium.SchemaNodeRef{}, false
}

// effectiveCase returns the case of choice whose default values are in use
// (RFC 7950 §7.6.1, §7.9.3): the case selected by the data or, when no case
// has data, the choice's default case.
func effectiveCase(choice cambium.SchemaNodeRef, present levelIndex) (cambium.SchemaNodeRef, bool) {
	if c, ok := dataCase(choice, present); ok {
		return c, true
	}
	name, ok := choice.DefaultValue()
	if !ok {
		return cambium.SchemaNodeRef{}, false
	}
	for _, c := range choiceCases(choice) {
		if c.Name() == name {
			return c, true
		}
	}
	return cambium.SchemaNodeRef{}, false
}

// choiceActive reports whether the when conditions guarding a choice hold. The
// schema records the choice's own when and an augment's when on the choice
// node; a uses or an enclosing choice or case passes its when only to data
// nodes, so those are read from one of the choice's data nodes, keeping the
// ones inherited from a subtree rooted at or above the choice. Every one of
// them is evaluated with the choice's closest data ancestor as context.
func choiceActive(root, parent *xnode, choice cambium.SchemaNodeRef) bool {
	whens := choice.Whens()
	if data := appendFlattened(nil, choice); len(data) > 0 {
		above := map[cambium.SchemaNodeRef]bool{choice: true}
		for _, a := range choice.Ancestors() {
			above[a] = true
		}
		for _, w := range data[0].Whens() {
			for _, r := range w.ExcludedSubtreeRoots() {
				if above[r] {
					whens = append(whens, w)
					break
				}
			}
		}
	}
	for _, w := range whens {
		contextNode := missingConstraintContext(parent, nil, max(w.ContextAncestorDepth(), 1))
		if contextNode == nil {
			return false
		}
		ev := constraintEvaluator(root, contextNode, contextNode, w.SourceModule(), choice.Module(), w.ExcludedSubtreeRoots())
		ok, err := evalConstraint(ev, w.Expression(), ectx{node: contextNode, pos: 1, size: 1})
		if err != nil || !ok {
			return false
		}
	}
	return true
}
