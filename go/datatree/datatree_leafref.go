package datatree

import (
	"fmt"
	"strings"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// Leafref instance-existence checking (O8 slice 4a). When a leaf's type is a
// leafref with require-instance true, its value must equal some value of the
// leaf(s) the leafref path points at, within the same data tree.
//
// Safety rule: this resolver supports only location paths made of name steps
// (absolute "/m:a/m:b" or relative "../x"); descending lists branches the
// node-set. Any path using predicates ("[...]") or functions ("current()",
// "deref()") is treated as UNSUPPORTED and the check is SKIPPED — never reported
// as a violation. The engine under-claims coverage rather than risk a wrong
// verdict.

// checkLeafRefInstance reports a violation only when the leafref path is fully
// resolvable and the value is definitely absent from the target instances.
func checkLeafRefInstance(sn cambium.SchemaNodeRef, value string, ancestors [][]*node, path string, out *[]string) {
	ti, ok := sn.LeafType()
	if !ok {
		return
	}
	lr, ok := ti.Resolved().(cambium.ResolvedLeafRef)
	if !ok || !lr.RequireInstance() {
		return
	}
	expr, ok := lr.Path()
	if !ok {
		return
	}
	sourceModule := lr.SourceModule()
	if sourceModule.Name() == "" {
		sourceModule = sn.Module()
	}
	targets, supported := resolveLeafRefTargets(expr, ancestors, sourceModule)
	if !supported {
		return // unsupported path construct: skip, do not false-reject
	}
	want := value
	for _, tv := range targets {
		if tv == want {
			return
		}
	}
	*out = append(*out, fmt.Sprintf("%s: leafref value %s has no matching instance at path %q", path, want, expr))
}

// resolveLeafRefTargets resolves a leafref path to the set of target leaf values
// reachable in the data tree. supported=false means the path used a construct
// outside the name-step subset and the caller must skip the check.
func resolveLeafRefTargets(pathExpr string, ancestors [][]*node, module cambium.Module) (values []string, supported bool) {
	expr := strings.TrimSpace(pathExpr)
	if expr == "" || strings.ContainsAny(expr, "[]()") {
		return nil, false
	}
	var start []*node
	switch {
	case strings.HasPrefix(expr, "/"):
		start = ancestors[0] // absolute: from the data root siblings
		expr = strings.TrimLeft(expr, "/")
	default:
		up := 0
		for strings.HasPrefix(expr, "../") {
			up++
			expr = expr[len("../"):]
		}
		if up == 0 {
			return nil, false // descendant-of-leaf path: nothing to match, skip
		}
		idx := len(ancestors) - up
		if idx < 0 {
			return nil, false // path climbs above the root
		}
		start = ancestors[idx]
	}
	steps, ok := splitLeafRefSteps(expr, module)
	if !ok {
		return nil, false
	}
	if len(steps) == 0 {
		return nil, false
	}
	return navigateLeafRef([][]*node{start}, steps)
}

type leafRefStep struct {
	module string
	name   string
}

// navigateLeafRef walks name steps across a set of sibling frames, branching at
// lists, and collects the terminal leaf / leaf-list values.
func navigateLeafRef(frames [][]*node, steps []leafRefStep) ([]string, bool) {
	for si, step := range steps {
		last := si == len(steps)-1
		if last {
			var values []string
			for _, frame := range frames {
				n := findByLeafRefStep(frame, step)
				if n == nil {
					continue
				}
				switch n.kind {
				case kindLeaf:
					values = append(values, string(n.value))
				case kindLeafList:
					for _, v := range n.values {
						values = append(values, string(v))
					}
				default:
					return nil, false // leafref target must be a leaf or leaf-list
				}
			}
			return values, true
		}
		var next [][]*node
		for _, frame := range frames {
			n := findByLeafRefStep(frame, step)
			if n == nil {
				continue
			}
			switch n.kind {
			case kindContainer:
				next = append(next, n.children)
			case kindList:
				next = append(next, n.entries...)
			default:
				return nil, false // cannot descend through a leaf mid-path
			}
		}
		frames = next
	}
	return nil, true
}

func splitLeafRefSteps(rest string, module cambium.Module) ([]leafRefStep, bool) {
	var steps []leafRefStep
	for _, s := range strings.Split(rest, "/") {
		if s == "" {
			continue
		}
		mod := module
		if i := strings.LastIndex(s, ":"); i >= 0 {
			var ok bool
			mod, ok = module.ResolvePrefix(s[:i])
			if !ok {
				return nil, false
			}
			s = s[i+1:]
		}
		if s == "" {
			return nil, false
		}
		steps = append(steps, leafRefStep{module: mod.Name(), name: s})
	}
	return steps, true
}

func findByLeafRefStep(nodes []*node, step leafRefStep) *node {
	for _, n := range nodes {
		if n.module == step.module && n.name == step.name {
			return n
		}
	}
	return nil
}
