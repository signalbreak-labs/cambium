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

// Validate checks: mandatory leaves and choices present, list / leaf-list
// min- and max-elements, choice/case exclusivity, leaf-list value uniqueness
// (configuration data only), list keys present in every entry, list key and
// unique-statement uniqueness, per-leaf value types, leafref instance
// existence, and must/when XPath constraints. It returns a *ValidationError
// listing every violation, or nil if the tree is valid.
//
// Constraints follow the data tree RFC 7950 describes: only the case of a
// choice whose data exists is validated, and a non-presence container exists
// whenever its parent does, so the constraints below an absent one still apply
// (libyang instantiates it). Values are compared in canonical form.
//
// must/when expressions and leafref paths that use constructs outside the
// supported XPath subset (unimplemented functions, explicit axes, unresolved
// prefixes) are SKIPPED rather than mis-evaluated — the engine never produces a
// wrong verdict, it under-claims coverage. instance-identifier resolution uses
// the same skip-on-unsupported rule.
func (t *Tree) Validate() error {
	var violations []string
	root := t.xroot()
	v := validator{root: root, out: &violations}
	v.level(root, topLevelSchema(t.module), [][]*node{t.roots}, "")
	t.checkMustWhen(&violations)
	if len(violations) == 0 {
		return nil
	}
	return &ValidationError{Violations: violations}
}

// validator walks the schema in declaration order and checks it against the
// data. Driving from the schema (not the data) is what surfaces
// absent-but-required nodes.
type validator struct {
	root *xnode
	out  *[]string
}

func (v *validator) report(format string, args ...any) {
	*v.out = append(*v.out, fmt.Sprintf(format, args...))
}

// level validates the data of one level (the last ancestors frame) against
// schema, the level's children with choice and case nodes kept. parent is the
// level's XPath node: a detached placeholder when the level is an absent
// non-presence container.
func (v *validator) level(parent *xnode, schema []cambium.SchemaNodeRef, ancestors [][]*node, path string) {
	v.nodes(parent, schema, indexLevel(ancestors[len(ancestors)-1]), ancestors, path)
}

func (v *validator) nodes(parent *xnode, schema []cambium.SchemaNodeRef, present levelIndex, ancestors [][]*node, path string) {
	for _, sn := range schema {
		if sn.IsChoice() {
			v.choice(parent, sn, present, ancestors, path)
			continue
		}
		childPath := path + "/" + sn.Name()
		dn := present.node(sn)
		if dn == nil {
			v.missing(parent, sn, ancestors, childPath)
			continue
		}
		switch {
		case sn.IsList():
			checkElements(sn, len(dn.entries), childPath, v.out)
			checkListKeys(sn, dn, childPath, v.out)
			listNodes := matchingChildXNodes(parent, sn)
			v.checkUnique(sn, dn, listNodes, childPath)
			for i, entry := range dn.entries {
				var listParent *xnode
				if i < len(listNodes) {
					listParent = listNodes[i]
				}
				v.level(listParent, levelSchema(sn), appendFrame(ancestors, entry), fmt.Sprintf("%s[%d]", childPath, i))
			}
		case sn.IsLeafList():
			checkElements(sn, len(dn.values), childPath, v.out)
			if sn.RepresentsConfigurationData() {
				// RFC 7950 §7.7.1: state leaf-lists may repeat a value.
				checkLeafListUnique(dn, childPath, v.out)
			}
			if ti, ok := sn.LeafType(); ok {
				for i, value := range dn.values {
					valuePath := fmt.Sprintf("%s[%d]", childPath, i)
					validateLeafValue(ti, value, valuePath, sn.Module().Name(), v.out)
					checkLeafRefInstance(sn, value, ancestors, valuePath, v.out)
				}
			}
		case sn.IsContainer():
			v.level(firstMatchingChildXNode(parent, sn), levelSchema(sn), appendFrame(ancestors, dn.children), childPath)
		case sn.IsLeaf():
			if ti, ok := sn.LeafType(); ok {
				validateLeafValue(ti, dn.value, childPath, sn.Module().Name(), v.out)
			}
			checkLeafRefInstance(sn, dn.value, ancestors, childPath, v.out)
		}
	}
}

// missing checks an absent node whose when conditions (if any) hold: it must
// not be mandatory or require elements, and an absent non-presence container
// is validated as an empty one.
func (v *validator) missing(parent *xnode, sn cambium.SchemaNodeRef, ancestors [][]*node, path string) {
	if !schemaNodeActiveForMissing(v.root, parent, sn) {
		return
	}
	if sn.IsMandatory() {
		v.report("missing mandatory node %s", path)
	}
	if minE, ok := sn.MinElements(); ok && minE > 0 {
		v.report("%s has 0 entries, fewer than min-elements %d", path, minE)
	}
	if sn.IsContainer() && !sn.IsPresenceContainer() {
		v.level(dummyMissingNode(parent, sn), levelSchema(sn), appendFrame(ancestors, nil), path)
	}
}

// choice checks that at most one case has data and that a mandatory choice
// has one, then validates the selected case only: constraints inside the
// other cases do not apply (RFC 7950 §7.9). With data in several cases, the
// first is validated, as libyang does.
func (v *validator) choice(parent *xnode, choice cambium.SchemaNodeRef, present levelIndex, ancestors [][]*node, path string) {
	var withData []cambium.SchemaNodeRef
	for _, c := range choiceCases(choice) {
		if caseHas(c, present, instantiated) {
			withData = append(withData, c)
		}
	}
	if len(withData) > 1 {
		v.report("%s: data for both cases %q and %q of choice %q exist", displayPath(path), withData[0].Name(), withData[1].Name(), choice.Name())
	}
	selected, ok := dataCase(choice, present)
	if !ok {
		if choice.IsMandatory() && choiceActive(v.root, parent, choice) {
			v.report("missing mandatory choice %s/%s", path, choice.Name())
		}
		return
	}
	v.nodes(parent, caseSchema(selected), present, ancestors, path)
}

func displayPath(path string) string {
	if path == "" {
		return "/"
	}
	return path
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
	keys := childRefs(sn.ListKeys())
	if len(keys) == 0 {
		return
	}
	seen := make(map[string]bool, len(dn.entries))
	for i, entry := range dn.entries {
		// Keys are looked up by qualified (module, name) identity: a same-named
		// leaf augmented in from another module is not the key.
		byKey := make(map[nodeKey]*node, len(entry))
		for _, e := range entry {
			byKey[dataNodeKey(e)] = e
		}
		tuple := make([]string, 0, len(keys))
		complete := true
		for _, k := range keys {
			kn := byKey[schemaNodeKey(k)]
			if kn == nil {
				*out = append(*out, fmt.Sprintf("%s[%d] is missing key leaf %q", path, i, k.Name()))
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
