// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree

import (
	"fmt"
	"strings"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// ValidationError aggregates the structural validation violations found by
// Tree.Validate, in document/schema order.
type ValidationError struct {
	Violations []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("datatree: %d validation violation(s): %s",
		len(e.Violations), strings.Join(e.Violations, "; "))
}

// Validate checks: mandatory leaves present, list / leaf-list min- and
// max-elements, leaf-list value uniqueness, list keys present in every entry,
// list key uniqueness, per-leaf value types, leafref instance existence, and
// must/when XPath constraints. It returns a *ValidationError listing every
// violation, or nil if the tree is valid.
//
// must/when expressions and leafref paths that use constructs outside the
// supported XPath subset (unimplemented functions, explicit axes, unresolved
// prefixes) are SKIPPED rather than mis-evaluated — the engine never produces a
// wrong verdict, it under-claims coverage. instance-identifier resolution uses
// the same skip-on-unsupported rule.
func (t *Tree) Validate() error {
	var violations []string
	root := t.xroot()
	validateLevel(root, root, flattenTopLevel(t.module), [][]*node{t.roots}, "", &violations)
	t.checkMustWhen(&violations)
	if len(violations) == 0 {
		return nil
	}
	return &ValidationError{Violations: violations}
}

// validateLevel walks the schema children in declaration order and checks each
// against the data present at this level. Driving from the schema (not the data)
// is what surfaces absent-but-required nodes.
func validateLevel(root, parent *xnode, schema []cambium.SchemaNodeRef, ancestors [][]*node, path string, out *[]string) {
	data := ancestors[len(ancestors)-1]
	present := make(map[nodeKey]*node, len(data))
	for _, d := range data {
		present[dataNodeKey(d)] = d
	}
	for _, sn := range schema {
		childPath := path + "/" + sn.Name()
		dn := present[schemaNodeKey(sn)]
		if dn == nil {
			active := schemaNodeActiveForMissing(root, parent, sn)
			if active && sn.IsMandatory() {
				*out = append(*out, fmt.Sprintf("missing mandatory node %s", childPath))
			}
			if minE, ok := sn.MinElements(); active && ok && minE > 0 {
				*out = append(*out, fmt.Sprintf("%s has 0 entries, fewer than min-elements %d", childPath, minE))
			}
			continue
		}
		switch {
		case sn.IsList():
			checkElements(sn, len(dn.entries), childPath, out)
			checkListKeys(sn, dn, childPath, out)
			listNodes := matchingChildXNodes(parent, sn)
			for i, entry := range dn.entries {
				var listParent *xnode
				if i < len(listNodes) {
					listParent = listNodes[i]
				}
				validateLevel(root, listParent, childRefs(sn.DataChildren(true)), appendFrame(ancestors, entry), fmt.Sprintf("%s[%d]", childPath, i), out)
			}
		case sn.IsLeafList():
			checkElements(sn, len(dn.values), childPath, out)
			checkLeafListUnique(dn, childPath, out)
			if ti, ok := sn.LeafType(); ok {
				for i, v := range dn.values {
					validateLeafValue(ti, v, fmt.Sprintf("%s[%d]", childPath, i), sn.Module().Name(), out)
				}
			}
		case sn.IsContainer():
			validateLevel(root, firstMatchingChildXNode(parent, sn), childRefs(sn.DataChildren(true)), appendFrame(ancestors, dn.children), childPath, out)
		case sn.IsLeaf():
			if ti, ok := sn.LeafType(); ok {
				validateLeafValue(ti, dn.value, childPath, sn.Module().Name(), out)
			}
			checkLeafRefInstance(sn, string(dn.value), ancestors, childPath, out)
		}
	}
}

func (t *Tree) xroot() *xnode {
	root := &xnode{}
	root.kids = buildXNodes(flattenTopLevel(t.module), t.roots, root)
	return root
}

func schemaNodeActiveForMissing(root, parent *xnode, sn cambium.SchemaNodeRef) bool {
	whens := sn.Whens()
	if len(whens) == 0 {
		return true
	}
	dummy := dummyMissingNode(parent, sn)
	for _, w := range whens {
		contextNode := missingConstraintContext(parent, dummy, w.ContextAncestorDepth())
		if contextNode == nil {
			return false
		}
		ev := constraintEvaluator(root, contextNode, contextNode, w.SourceModule(), sn.Module(), w.ExcludedSubtreeRoots())
		ok, err := evalConstraint(ev, w.Expression(), ectx{node: contextNode, pos: 1, size: 1})
		if err != nil || !ok {
			return false
		}
	}
	return true
}

func dummyMissingNode(parent *xnode, sn cambium.SchemaNodeRef) *xnode {
	return &xnode{name: sn.Name(), ns: sn.Namespace(), leaf: sn.IsLeaf() || sn.IsLeafList(), schema: sn, hasSchema: true, parent: parent}
}

func missingConstraintContext(parent, dummy *xnode, ancestorDepth int) *xnode {
	if ancestorDepth == 0 {
		return dummy
	}
	n := parent
	for i := 1; i < ancestorDepth && n != nil; i++ {
		n = n.parent
	}
	return n
}

func firstMatchingChildXNode(parent *xnode, sn cambium.SchemaNodeRef) *xnode {
	nodes := matchingChildXNodes(parent, sn)
	if len(nodes) == 0 {
		return nil
	}
	return nodes[0]
}

func matchingChildXNodes(parent *xnode, sn cambium.SchemaNodeRef) []*xnode {
	if parent == nil {
		return nil
	}
	var out []*xnode
	for _, child := range parent.kids {
		if child.hasSchema && child.schema == sn {
			out = append(out, child)
		}
	}
	return out
}

// appendFrame returns a new ancestor chain with frame appended, copying so
// sibling recursions never alias the same backing array.
func appendFrame(ancestors [][]*node, frame []*node) [][]*node {
	out := make([][]*node, len(ancestors)+1)
	copy(out, ancestors)
	out[len(ancestors)] = frame
	return out
}

func checkElements(sn cambium.SchemaNodeRef, count int, path string, out *[]string) {
	if minE, ok := sn.MinElements(); ok && int64(count) < int64(minE) {
		*out = append(*out, fmt.Sprintf("%s has %d entries, fewer than min-elements %d", path, count, minE))
	}
	if maxE, ok := sn.MaxElements(); ok && int64(count) > int64(maxE) {
		*out = append(*out, fmt.Sprintf("%s has %d entries, more than max-elements %d", path, count, maxE))
	}
}

// checkListKeys verifies every list entry carries all key leaves and that the
// key tuples are unique across entries (invariant-neutral: key order in the
// tuple follows key-statement order, and entries are not reordered).
func checkListKeys(sn cambium.SchemaNodeRef, dn *node, path string, out *[]string) {
	keys := sn.KeyNames()
	if len(keys) == 0 {
		return
	}
	seen := make(map[string]bool, len(dn.entries))
	for i, entry := range dn.entries {
		byName := make(map[string]*node, len(entry))
		for _, e := range entry {
			byName[e.name] = e
		}
		tuple := make([]string, 0, len(keys))
		complete := true
		for _, k := range keys {
			kn := byName[k]
			if kn == nil {
				*out = append(*out, fmt.Sprintf("%s[%d] is missing key leaf %q", path, i, k))
				complete = false
				continue
			}
			tuple = append(tuple, string(kn.value))
		}
		if !complete {
			continue
		}
		joined := strings.Join(tuple, "\x00")
		if seen[joined] {
			*out = append(*out, fmt.Sprintf("%s has a duplicate key %v", path, tuple))
		}
		seen[joined] = true
	}
}

func checkLeafListUnique(dn *node, path string, out *[]string) {
	seen := make(map[string]bool, len(dn.values))
	for i, v := range dn.values {
		value := string(v)
		if seen[value] {
			*out = append(*out, fmt.Sprintf("%s[%d] has a duplicate leaf-list value", path, i))
			continue
		}
		seen[value] = true
	}
}
