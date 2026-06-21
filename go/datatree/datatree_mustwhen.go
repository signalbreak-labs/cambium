package datatree

import (
	"fmt"
	"strings"
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
		ev := &evaluator{root: root, module: n.schema.Module(), current: n}
		c := ectx{node: n, pos: 1, size: 1}
		for _, w := range n.schema.Whens() {
			if ok, err := evalConstraint(ev, w.Expression(), c); err == nil && !ok {
				*out = append(*out, fmt.Sprintf("%s: when condition %q is not satisfied", xnodePath(n), w.Expression()))
			}
		}
		for _, m := range n.schema.Musts() {
			if ok, err := evalConstraint(ev, m.Expression(), c); err == nil && !ok {
				*out = append(*out, fmt.Sprintf("%s: %s", xnodePath(n), mustMessage(m)))
			}
		}
	}
	for _, k := range n.kids {
		walkMustWhen(root, k, out)
	}
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
