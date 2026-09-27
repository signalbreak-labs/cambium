// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree

import (
	"encoding/json"
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
// Values are compared as values, not spellings (see valueKey).
func checkLeafRefInstance(sn cambium.SchemaNodeRef, value json.RawMessage, ancestors [][]*node, path string, out *[]string) {
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
	found, supported := leafRefTargetExists(expr, ancestors, sourceModule, valueKey(ti, value, sn.Module().Name()))
	if !supported || found {
		return // an unsupported path construct is skipped, never false-rejected
	}
	*out = append(*out, fmt.Sprintf("%s: leafref value %s has no matching instance at path %q", path, value, expr))
}

// leafRefTargetExists resolves a leafref path in the data tree and reports
// whether one of the leaf or leaf-list values it reaches is the value whose
// comparison key (see valueKey) is want. supported=false means the path used a
// construct outside the name-step subset and the caller must skip the check.
func leafRefTargetExists(pathExpr string, ancestors [][]*node, module cambium.Module, want string) (found, supported bool) {
	expr := strings.TrimSpace(pathExpr)
	if expr == "" || strings.ContainsAny(expr, "[]()") {
		return false, false
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
			return false, false // descendant-of-leaf path: nothing to match, skip
		}
		idx := len(ancestors) - up
		if idx < 0 {
			return false, false // path climbs above the root
		}
		start = ancestors[idx]
	}
	steps, ok := splitLeafRefSteps(expr, module)
	if !ok {
		return false, false
	}
	if len(steps) == 0 {
		return false, false
	}
	return navigateLeafRef([][]*node{start}, steps, want)
}

type leafRefStep struct {
	module string
	name   string
}

// navigateLeafRef walks name steps depth-first across a set of sibling frames,
// branching at lists, until a terminal leaf / leaf-list value matches want.
// Without a match it visits every target, so an unsupported shape anywhere on
// the path is still reported.
func navigateLeafRef(frames [][]*node, steps []leafRefStep, want string) (found, supported bool) {
	step, last := steps[0], len(steps) == 1
	for _, frame := range frames {
		n := findByLeafRefStep(frame, step)
		if n == nil {
			continue
		}
		var next [][]*node
		switch {
		case last && n.kind == kindLeaf:
			if leafRefValueMatches(n, n.value, want) {
				return true, true
			}
			continue
		case last && n.kind == kindLeafList:
			for _, v := range n.values {
				if leafRefValueMatches(n, v, want) {
					return true, true
				}
			}
			continue
		case last:
			return false, false // leafref target must be a leaf or leaf-list
		case n.kind == kindContainer:
			next = [][]*node{n.children}
		case n.kind == kindList:
			next = n.entries
		default:
			return false, false // cannot descend through a leaf mid-path
		}
		if found, supported := navigateLeafRef(next, steps[1:], want); found || !supported {
			return found, supported
		}
	}
	return false, true
}

// leafRefValueMatches reports whether value, held by the leaf or leaf-list n,
// is the value whose comparison key is want. Equal tokens are the same value;
// different tokens can only be when n's type may name an identity, so only
// then is the key computed.
func leafRefValueMatches(n *node, value json.RawMessage, want string) bool {
	if string(value) == want {
		return true
	}
	ti, ok := n.schema.LeafType()
	return ok && valueKeyQualifies(ti) && valueKey(ti, value, n.module) == want
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
