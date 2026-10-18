// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree

import (
	"errors"
	"fmt"
	"strings"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// must/when validation (O8 slice 4b). Builds an XML-data-model view of the tree,
// then evaluates each present node's when and must constraints with the XPath
// evaluator. A constraint whose expression cannot be parsed or evaluated (an
// unsupported construct or function) is SKIPPED — never reported — so the engine
// never produces a wrong verdict. Supported matcher operational failures are
// returned rather than classified as unsupported.
func (t *Tree) checkMustWhen(out *[]string) error {
	root := &xnode{}
	root.kids = buildXNodes(flattenTopLevel(t.modules...), t.roots, root)
	return walkMustWhen(root, root, out)
}

func walkMustWhen(root, n *xnode, out *[]string) error {
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
			disposition, err := constraintDisposition(ev, w.Expression(), c)
			if err != nil {
				return fmt.Errorf("%s: when %q: %w", xnodePath(n), w.Expression(), err)
			}
			if disposition == predicateFalse {
				*out = append(*out, fmt.Sprintf("%s: when condition %q is not satisfied", xnodePath(n), w.Expression()))
			}
		}
		c := ectx{node: n, pos: 1, size: 1}
		for _, m := range n.schema.Musts() {
			ev := constraintEvaluator(root, n, n, m.SourceModule(), n.schema.Module(), nil)
			disposition, err := constraintDisposition(ev, m.Expression(), c)
			if err != nil {
				return fmt.Errorf("%s: must %q: %w", xnodePath(n), m.Expression(), err)
			}
			if disposition == predicateFalse {
				*out = append(*out, fmt.Sprintf("%s: %s", xnodePath(n), mustMessage(m)))
			}
		}
		if err := checkInstanceIdentifier(constraintEvaluator(root, n, n, n.schema.Module(), n.schema.Module(), nil), n, out); err != nil {
			return fmt.Errorf("%s: %w", xnodePath(n), err)
		}
	}
	for _, k := range n.kids {
		if err := walkMustWhen(root, k, out); err != nil {
			return err
		}
	}
	return nil
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
// parse or evaluation error is returned to the predicate-disposition boundary.
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

// patternEvaluationError distinguishes supported matcher failures from legacy
// unsupported XPath expressions. Its cause remains available through errors.As.
type patternEvaluationError struct{ err error }

func (e *patternEvaluationError) Error() string { return e.err.Error() }
func (e *patternEvaluationError) Unwrap() error { return e.err }

type predicateResult uint8

const (
	predicateFalse predicateResult = iota
	predicateTrue
	legacyUnsupported
)

// constraintDisposition is the sole compatibility boundary for predicates.
// Only matcher-origin errors escape; unrelated unsupported XPath keeps its
// historical present-node skip / absent-node inactive disposition.
func constraintDisposition(ev *evaluator, expr string, c ectx) (predicateResult, error) {
	ok, err := evalConstraint(ev, expr, c)
	if err != nil {
		if _, matcher := errors.AsType[*patternEvaluationError](err); matcher {
			return predicateFalse, err
		}
		return legacyUnsupported, nil
	}
	if ok {
		return predicateTrue, nil
	}
	return predicateFalse, nil
}
