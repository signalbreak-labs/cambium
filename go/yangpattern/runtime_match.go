// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package yangpattern

import (
	"errors"
	"slices"
	"unicode/utf8"
)

// MatchString evaluates with default match limits, independently of constructor policy.
func (p *Program) MatchString(value string) (bool, error) {
	return p.MatchStringWithLimits(value, MatchLimits{})
}

// MatchStringWithLimits returns an exact boolean or an operational error. It never
// reparses source XSD. All mutable value-dependent state belongs to this call.
func (p *Program) MatchStringWithLimits(value string, limits MatchLimits) (bool, error) {
	result, _, err := p.match(value, limits)
	return result, err
}
func (p *Program) match(value string, limits MatchLimits) (matched bool, route string, err error) {
	if p == nil || len(p.data.Nodes) == 0 {
		return false, "", &ConfigurationError{"uninitialized program"}
	}
	l := limits.defaults()
	count := uint64(utf8.RuneCountInString(value)) // #nosec G115 -- RuneCountInString is nonnegative and bounded by len(value).
	root := p.data.Root
	if p.minimum[root] > count {
		return false, "length", nil
	}
	if count == 0 {
		return p.nullable[root], "empty", nil
	}
	n := p.data.Nodes[root]
	class := root
	lo, hi := uint64(1), uint64(1)
	inf := false
	if n.Op == OpRepeat {
		class = n.Children[0]
		lo, hi, inf = p.bounds[root].lower, p.bounds[root].upper, n.Unbounded
	}
	if p.data.Nodes[class].Op == OpClass {
		if err := limitError("match", "direct", "bytes", l.Bytes, 128); err != nil {
			return false, "direct", err
		}
		if err := limitError("match", "direct", "work", l.Work, 1); err != nil {
			return false, "direct", err
		}
		if count < lo || (!inf && count > hi) {
			return false, "direct", nil
		}
		for _, r := range value {
			if !containsRune(p.data.Nodes[class].Ranges, r) {
				return false, "direct", nil
			}
		}
		return true, "direct", nil
	}
	var ordinaryErr error
	if p.native != nil {
		ordinaryErr = nativeEligible(uint64(len(p.nativeSource)), p.nativeInstructions, count, l)
		if ordinaryErr == nil {
			return p.native.MatchString(value), "native", nil
		}
	}
	a, rootID := p.specialize(count, l)

	// Stable cache hits perform no structural work; long values have no rune cap.
	state := rootID
	for _, r := range value {
		if a.b.err != nil {
			break
		}
		symbol := a.symbol(r)
		state = a.derive(state, symbol)
		if a.b.err != nil {
			break
		}
		if state == 0 {
			return false, "derivative", nil
		}
	}
	if a.b.err == nil {
		return a.nodes[state].nullable, "derivative", nil
	}
	derivativeErr := a.b.err
	// Release the speculative arena before constructing fallback state.
	a = nil
	fallback, err := p.specializedProgram(count, l)
	if err != nil {
		return false, "fallback", errors.Join(derivativeErr, err)
	}
	source, estimate, err := fallback.nativeExpression(true, l)
	if err != nil {
		return false, "fallback", errors.Join(derivativeErr, err)
	}
	if source == p.nativeSource && ordinaryErr != nil {
		return false, "fallback", errors.Join(derivativeErr, ordinaryErr)
	}
	if err = nativeEligible(uint64(len(source)), estimate, count, l); err != nil {
		return false, "fallback", errors.Join(derivativeErr, err)
	}
	instructions, err := nativeInstructionCount(source)
	if err != nil {
		return false, "fallback", err
	}
	if err = nativeEligible(uint64(len(source)), instructions, count, l); err != nil {
		return false, "fallback", errors.Join(derivativeErr, err)
	}
	re, err := fallback.compileNative(source)
	if err != nil {
		return false, "fallback", err
	}
	return re.MatchString(value), "fallback", nil
}
func containsRune(ranges []RuneRange, r rune) bool {
	i, _ := slices.BinarySearchFunc(ranges, r, func(v RuneRange, r rune) int {
		if v.Hi < r {
			return -1
		}
		if v.Lo > r {
			return 1
		}
		return 0
	})
	return i < len(ranges) && ranges[i].Lo <= r && ranges[i].Hi >= r
}

// Specialization only clips counts against this candidate's length. The portable
// metadata is not changed or exported from this private evaluator.
func (p *Program) specialize(count uint64, l MatchLimits) (scratch *arena, rootID int) {
	a := newArena(l, p)
	memo := make(map[uint32]int)
	if !a.b.charge(mulSat(uint64(len(p.data.Nodes)), 64), uint64(len(p.data.Nodes))) {
		return a, 0
	}
	var build func(uint32) int
	build = func(id uint32) int {
		if a.b.err != nil {
			return 0
		}
		if out, ok := memo[id]; ok {
			return out
		}
		if p.minimum[id] > count {
			return 0
		}
		n := p.data.Nodes[id]
		result := 0
		switch n.Op {
		case OpEmpty:
		case OpEpsilon:
			result = 1
		case OpClass:
			result = a.put(residual{op: OpClass, a: int(id)})
		case OpAlternate:
			result = a.alt(build(n.Children[0]), build(n.Children[1]))
		case OpConcat:
			result = a.seq(build(n.Children[0]), build(n.Children[1]))
		case OpRepeat:
			child := n.Children[0]
			maximum := count / p.minimum[child]
			lo := p.bounds[id].lower
			hi := -1
			if !n.Unbounded && p.bounds[id].upper < maximum {
				hi = int(p.bounds[id].upper) // #nosec G115 -- maximum <= rune count <= len(value), an int.
			}
			result = a.rep(build(child), int(lo), hi) // #nosec G115 -- minimum-length check bounds lo by rune count.
		}
		memo[id] = result
		return result
	}
	root := build(p.data.Root)
	return a, root
}
func (p *Program) specializedProgram(count uint64, l MatchLimits) (*Program, error) {
	if err := limitError("match", "fallback", "bytes", l.Bytes, mulSat(uint64(len(p.data.Nodes)), 256)); err != nil {
		return nil, err
	}
	if err := limitError("match", "fallback", "work", l.Work, uint64(len(p.data.Nodes))); err != nil {
		return nil, err
	}
	d := p.data
	d.Nodes = slices.Clone(d.Nodes)
	for i := range d.Nodes {
		n := &d.Nodes[i]
		if p.minimum[i] > count {
			*n = NodeData{Op: OpEmpty}
			continue
		}
		if n.Op == OpRepeat {
			child := n.Children[0]
			maximum := count / p.minimum[child]
			if n.Unbounded || p.bounds[i].upper >= maximum {
				n.Unbounded = true
				n.Max = ""
			}
		}
	}
	return &Program{data: d, bounds: p.bounds}, nil
}
