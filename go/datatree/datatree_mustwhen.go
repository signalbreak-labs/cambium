// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree

import (
	"fmt"
	"strings"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// must/when validation (O8 slice 4b). Builds an XML-data-model view of the tree,
// then evaluates each present node's when and must constraints with the XPath
// evaluator. A constraint whose expression cannot be parsed or evaluated (an
// unsupported construct or function) is SKIPPED — never reported — so the engine
// never produces a wrong verdict.
func (t *Tree) checkMustWhen(out *[]string) {
	root := &xnode{}
	root.kids = buildXNodes(flattenTopLevel(t.module), t.roots, root)
	walkMustWhen(root, root, out)
}

func walkMustWhen(root, n *xnode, out *[]string) {
	if n.hasSchema {
		for _, w := range n.schema.Whens() {
			contextNode := constraintContextNode(n, w.ContextAncestorDepth())
			if contextNode == nil {
				continue
			}
			if w.ContextAncestorDepth() == 0 {
				contextNode = dummyConstraintNode(n)
			}
			ev := constraintEvaluator(root, contextNode, contextNode, w.SourceModule(), n.schema.Module(), w.ExcludedSubtreeRoots())
			c := ectx{node: contextNode, pos: 1, size: 1}
			if ok, err := evalConstraint(ev, w.Expression(), c); err == nil && !ok {
				*out = append(*out, fmt.Sprintf("%s: when condition %q is not satisfied", xnodePath(n), w.Expression()))
			}
		}
		c := ectx{node: n, pos: 1, size: 1}
		for _, m := range n.schema.Musts() {
			ev := constraintEvaluator(root, n, n, m.SourceModule(), n.schema.Module(), nil)
			if ok, err := evalConstraint(ev, m.Expression(), c); err == nil && !ok {
				*out = append(*out, fmt.Sprintf("%s: %s", xnodePath(n), mustMessage(m)))
			}
		}
		checkInstanceIdentifier(constraintEvaluator(root, n, n, n.schema.Module(), n.schema.Module(), nil), n, out)
	}
	for _, k := range n.kids {
		walkMustWhen(root, k, out)
	}
}

func constraintContextNode(n *xnode, ancestorDepth int) *xnode {
	for i := 0; i < ancestorDepth && n != nil; i++ {
		n = n.parent
	}
	return n
}

func dummyConstraintNode(n *xnode) *xnode {
	if n == nil {
		return nil
	}
	return &xnode{name: n.name, ns: n.ns, leaf: n.leaf, schema: n.schema, hasSchema: n.hasSchema, parent: n.parent}
}

func constraintEvaluator(root, contextNode, current *xnode, source, fallback cambium.Module, excluded []cambium.SchemaNodeRef) *evaluator {
	if source.Name() == "" {
		source = fallback
	}
	var unprefixedNS string
	if contextNode != nil {
		unprefixedNS = contextNode.ns
	}
	return &evaluator{root: root, module: source, unprefixedNS: unprefixedNS, excluded: excludedMap(excluded), current: current}
}

func excludedMap(roots []cambium.SchemaNodeRef) map[cambium.SchemaNodeRef]bool {
	if len(roots) == 0 {
		return nil
	}
	out := make(map[cambium.SchemaNodeRef]bool, len(roots))
	for _, root := range roots {
		out[root] = true
	}
	return out
}

// evalConstraint parses and evaluates a constraint expression to a boolean. Any
// parse or evaluation error is returned so the caller skips the check.
func evalConstraint(ev *evaluator, expr string, c ectx) (bool, error) {
	ast, err := parseXPath(expr)
	if err != nil {
		return false, err
	}
	v, err := ev.eval(ast, c)
	if err != nil {
		return false, err
	}
	return v.toBool(), nil
}

func mustMessage(m mustConstraintLike) string {
	if msg, ok := m.ErrorMessage(); ok && msg != "" {
		return msg
	}
	return fmt.Sprintf("must condition %q is not satisfied", m.Expression())
}

// mustConstraintLike captures the cambium.MustConstraint accessors used here.
type mustConstraintLike interface {
	Expression() string
	ErrorMessage() (string, bool)
}

func xnodePath(n *xnode) string {
	var parts []string
	for cur := n; cur != nil && cur.name != ""; cur = cur.parent {
		parts = append([]string{cur.name}, parts...)
	}
	return "/" + strings.Join(parts, "/")
}
