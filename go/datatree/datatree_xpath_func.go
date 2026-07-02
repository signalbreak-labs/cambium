// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/internal/xsdregex"
)

// Core XPath 1.0 function library. Functions outside this set return an error so
// the caller skips the check. deref() is intentionally not implemented yet and
// therefore skips rather than mis-evaluates.
func (ev *evaluator) evalCall(x *xCall, c ectx) (xval, error) {
	args := make([]xval, len(x.args))
	for i, a := range x.args {
		v, err := ev.eval(a, c)
		if err != nil {
			return xval{}, err
		}
		args[i] = v
	}
	arity := func(want int) error {
		if len(args) != want {
			return fmt.Errorf("%s() expects %d argument(s), got %d", x.name, want, len(args))
		}
		return nil
	}

	switch x.name {
	case "true":
		return boolVal(true), arity(0)
	case "false":
		return boolVal(false), arity(0)
	case "not":
		if err := arity(1); err != nil {
			return xval{}, err
		}
		return boolVal(!args[0].toBool()), nil
	case "boolean":
		if err := arity(1); err != nil {
			return xval{}, err
		}
		return boolVal(args[0].toBool()), nil
	case "string":
		if len(args) == 0 {
			return strVal(stringValue(c.node)), nil
		}
		return strVal(args[0].toStr()), arity(1)
	case "number":
		if len(args) == 0 {
			return numVal(parseXPathNumber(stringValue(c.node))), nil
		}
		return numVal(args[0].toNum()), arity(1)
	case "count":
		if err := arity(1); err != nil {
			return xval{}, err
		}
		if args[0].kind != kNodeset {
			return xval{}, fmt.Errorf("count() requires a node-set")
		}
		return numVal(float64(len(args[0].ns))), nil
	case "sum":
		if err := arity(1); err != nil {
			return xval{}, err
		}
		if args[0].kind != kNodeset {
			return xval{}, fmt.Errorf("sum() requires a node-set")
		}
		total := 0.0
		for _, n := range args[0].ns {
			total += parseXPathNumber(stringValue(n))
		}
		return numVal(total), nil
	case "position":
		return numVal(float64(c.pos)), arity(0)
	case "last":
		return numVal(float64(c.size)), arity(0)
	case "contains":
		if err := arity(2); err != nil {
			return xval{}, err
		}
		return boolVal(strings.Contains(args[0].toStr(), args[1].toStr())), nil
	case "starts-with":
		if err := arity(2); err != nil {
			return xval{}, err
		}
		return boolVal(strings.HasPrefix(args[0].toStr(), args[1].toStr())), nil
	case "string-length":
		s := stringValue(c.node)
		if len(args) == 1 {
			s = args[0].toStr()
		} else if len(args) > 1 {
			return xval{}, fmt.Errorf("string-length() takes at most 1 argument")
		}
		return numVal(float64(len([]rune(s)))), nil
	case "concat":
		if len(args) < 2 {
			return xval{}, fmt.Errorf("concat() needs at least 2 arguments")
		}
		var b strings.Builder
		for _, a := range args {
			b.WriteString(a.toStr())
		}
		return strVal(b.String()), nil
	case "substring":
		if len(args) < 2 || len(args) > 3 {
			return xval{}, fmt.Errorf("substring() takes 2 or 3 arguments")
		}
		return strVal(xpathSubstring(args)), nil
	case "floor":
		if err := arity(1); err != nil {
			return xval{}, err
		}
		return numVal(math.Floor(args[0].toNum())), nil
	case "ceiling":
		if err := arity(1); err != nil {
			return xval{}, err
		}
		return numVal(math.Ceil(args[0].toNum())), nil
	case "round":
		if err := arity(1); err != nil {
			return xval{}, err
		}
		return numVal(xpathRound(args[0].toNum())), nil
	case "current":
		if err := arity(0); err != nil {
			return xval{}, err
		}
		if ev.current == nil {
			return xval{kind: kNodeset}, nil
		}
		return xval{kind: kNodeset, ns: []*xnode{ev.current}}, nil
	case "local-name":
		if len(args) > 1 {
			return xval{}, fmt.Errorf("local-name() takes at most 1 argument")
		}
		var n *xnode
		switch {
		case len(args) == 0:
			n = c.node
		case args[0].kind == kNodeset && len(args[0].ns) > 0:
			n = args[0].ns[0]
		case args[0].kind != kNodeset:
			return xval{}, fmt.Errorf("local-name() requires a node-set")
		}
		if n == nil {
			return strVal(""), nil
		}
		return strVal(n.name), nil
	case "re-match":
		if err := arity(2); err != nil {
			return xval{}, err
		}
		pattern := args[1].toStr()
		if unsupported := xsdregex.UnsupportedNativeSyntax(pattern); unsupported != "" {
			return xval{}, fmt.Errorf("re-match() unsupported regex syntax: %s", unsupported)
		}
		re, err := regexp.Compile("^(?:" + xsdregex.NativePattern(pattern) + ")$")
		if err != nil {
			return xval{}, fmt.Errorf("re-match() regex compile: %w", err)
		}
		return boolVal(re.MatchString(args[0].toStr())), nil
	case "bit-is-set":
		if err := arity(2); err != nil {
			return xval{}, err
		}
		want := args[1].toStr()
		for _, bit := range strings.Fields(args[0].toStr()) {
			if bit == want {
				return boolVal(true), nil
			}
		}
		return boolVal(false), nil
	case "derived-from":
		if err := arity(2); err != nil {
			return xval{}, err
		}
		return ev.evalDerivedFrom(args[0], args[1].toStr(), false)
	case "derived-from-or-self":
		if err := arity(2); err != nil {
			return xval{}, err
		}
		return ev.evalDerivedFrom(args[0], args[1].toStr(), true)
	default:
		return xval{}, fmt.Errorf("unsupported function %q", x.name)
	}
}

func (ev *evaluator) evalDerivedFrom(value xval, targetQName string, orSelf bool) (xval, error) {
	if value.kind != kNodeset {
		return xval{}, fmt.Errorf("derived-from() requires a node-set")
	}
	target, ok, err := ev.identityForQName(targetQName)
	if err != nil {
		return xval{}, err
	}
	if !ok {
		return boolVal(false), nil
	}
	for _, n := range value.ns {
		actual, ok := identityRefIdentity(n)
		if ok && identityDerivedFrom(actual, target, orSelf) {
			return boolVal(true), nil
		}
	}
	return boolVal(false), nil
}

func (ev *evaluator) identityForQName(qname string) (cambium.Identity, bool, error) {
	prefix, local, prefixed := strings.Cut(qname, ":")
	if !prefixed {
		local = qname
		prefix = ""
	}
	mod, ok := ev.module.ResolvePrefix(prefix)
	if !ok {
		return cambium.Identity{}, false, fmt.Errorf("unknown identity prefix %q", prefix)
	}
	id, ok := mod.Identity(local)
	return id, ok, nil
}

func identityRefIdentity(n *xnode) (cambium.Identity, bool) {
	if n == nil || !n.hasSchema || !n.leaf {
		return cambium.Identity{}, false
	}
	ti, ok := n.schema.LeafType()
	if !ok {
		return cambium.Identity{}, false
	}
	resolved, ok := identityRefType(ti)
	if !ok {
		return cambium.Identity{}, false
	}
	value := n.value
	leafModule := n.schema.Module().Name()
	for _, base := range resolved.Bases() {
		if id, ok := findIdentityRefValue(base, value, leafModule); ok {
			return id, true
		}
	}
	return cambium.Identity{}, false
}

func identityRefType(ti cambium.TypeInfo) (cambium.ResolvedIdentityRef, bool) {
	switch r := ti.Resolved().(type) {
	case cambium.ResolvedIdentityRef:
		return r, true
	case cambium.ResolvedLeafRef:
		if rt, ok := r.Realtype(); ok && rt != nil {
			return identityRefType(*rt)
		}
	}
	return cambium.ResolvedIdentityRef{}, false
}

func findIdentityRefValue(id cambium.Identity, value, leafModule string) (cambium.Identity, bool) {
	if identityRefValueMatches(id, value, leafModule) {
		return id, true
	}
	for _, derived := range id.Derived() {
		if found, ok := findIdentityRefValue(derived, value, leafModule); ok {
			return found, true
		}
	}
	return cambium.Identity{}, false
}

func identityRefValueMatches(id cambium.Identity, value, leafModule string) bool {
	if strings.Contains(value, ":") {
		return value == identityQName(id)
	}
	return id.Module().Name() == leafModule && id.Name() == value
}

func identityDerivedFrom(actual, target cambium.Identity, orSelf bool) bool {
	if identityQName(actual) == identityQName(target) {
		return orSelf
	}
	for _, derived := range target.Derived() {
		if identityQName(actual) == identityQName(derived) || identityDerivedFrom(actual, derived, true) {
			return true
		}
	}
	return false
}

func identityQName(id cambium.Identity) string {
	return id.Module().Name() + ":" + id.Name()
}

// xpathRound implements XPath round(): round half towards positive infinity.
func xpathRound(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	return math.Floor(x + 0.5)
}

func xpathSubstring(args []xval) string {
	rs := []rune(args[0].toStr())
	from := xpathRound(args[1].toNum())
	to := math.Inf(1)
	if len(args) == 3 {
		to = xpathRound(args[1].toNum() + args[2].toNum())
	}
	var b strings.Builder
	for i, r := range rs {
		pos := float64(i + 1)
		if pos >= from && pos < to {
			b.WriteRune(r)
		}
	}
	return b.String()
}
