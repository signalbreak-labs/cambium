// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package yangpattern

import (
	"math/bits"
	"slices"
)

type residual struct {
	op           Op
	a, b, lo, hi int
	nullable     bool
}
type transition struct{ node, symbol int }
type arena struct {
	nodes       []residual
	intern      map[residual]int
	transitions map[transition]int
	endpoints   []rune
	program     *Program
	limits      MatchLimits
	b           ledger
	depth       uint64
}

func newArena(l MatchLimits, p *Program) *arena {
	a := &arena{limits: l, program: p, b: ledger{phase: "match", stage: "derivative", byteLimit: l.Bytes, workLimit: l.Work}}
	if !a.b.charge(1024, 1) {
		return a
	}
	a.intern = make(map[residual]int)
	a.transitions = make(map[transition]int)
	a.put(residual{op: OpEmpty})
	a.put(residual{op: OpEpsilon, nullable: true})
	var nr uint64
	for _, n := range p.data.Nodes {
		nr = addSat(nr, uint64(len(n.Ranges)))
	}
	if !a.b.charge(mulSat(nr, 16), mulSat(nr, uint64(bits.Len64(nr))+1)) { // #nosec G115 -- bit length is in [0,64].
		return a
	}
	a.endpoints = make([]rune, 0, int(nr*2))
	for _, n := range p.data.Nodes {
		for _, r := range n.Ranges {
			a.endpoints = append(a.endpoints, r.Lo, r.Hi+1)
		}
	}
	slices.Sort(a.endpoints)
	a.endpoints = slices.Compact(a.endpoints)
	return a
}
func (a *arena) symbol(r rune) int {
	i, found := slices.BinarySearch(a.endpoints, r)
	if found {
		return i
	}
	return i - 1
}
func (a *arena) enter() bool {
	if !a.b.charge(0, 1) {
		return false
	}
	a.depth++
	if a.depth > 512 {
		a.b.err = &ResourceError{"match", "derivative", "recursion depth", 512, a.depth}
		a.depth--
		return false
	}
	return true
}
func (a *arena) put(n residual) int {
	if !a.b.charge(0, 1) {
		return 0
	}
	if id, ok := a.intern[n]; ok {
		return id
	}
	if err := limitError("match", "derivative", "nodes", a.limits.Nodes, uint64(len(a.nodes)+1)); err != nil {
		a.b.err = err
		return 0
	}
	if !a.b.charge(256, 1) {
		return 0
	}
	id := len(a.nodes)
	a.nodes = append(a.nodes, n)
	a.intern[n] = id
	return id
}
func (a *arena) seq(x, y int) int {
	if !a.enter() {
		return 0
	}
	defer func() { a.depth-- }()
	if x == 0 || y == 0 {
		return 0
	}
	if x == 1 {
		return y
	}
	if y == 1 {
		return x
	}
	n := a.nodes[x]
	if n.op == OpConcat {
		return a.seq(n.a, a.seq(n.b, y))
	}
	if n.op == OpAlternate {
		return a.alt(a.seq(n.a, y), a.seq(n.b, y))
	}
	return a.put(residual{op: OpConcat, a: x, b: y, nullable: n.nullable && a.nodes[y].nullable})
}
func (a *arena) rep(x, lo, hi int) int {
	if !a.b.charge(0, 1) {
		return 0
	}
	if hi == 0 || x == 1 {
		return 1
	}
	if x == 0 {
		if lo == 0 {
			return 1
		}
		return 0
	}
	if lo == 1 && hi == 1 {
		return x
	}
	return a.put(residual{op: OpRepeat, a: x, lo: lo, hi: hi, nullable: lo == 0})
}
func (a *arena) alt(x, y int) int {
	if !a.enter() {
		return 0
	}
	defer func() { a.depth-- }()
	if x == 0 || x == y {
		return y
	}
	if y == 0 {
		return x
	}
	// Two passes reserve the complete temporary operand vector before allocation.
	count := 0
	var visit func(int, func(int))
	visit = func(id int, leaf func(int)) {
		if !a.enter() {
			return
		}
		defer func() { a.depth-- }()
		n := a.nodes[id]
		if n.op == OpAlternate {
			visit(n.a, leaf)
			visit(n.b, leaf)
		} else {
			leaf(id)
		}
	}
	visit(x, func(int) { count++ })
	visit(y, func(int) { count++ })
	if !a.b.charge(mulSat(uint64(count), 16), mulSat(uint64(count), uint64(bits.Len(uint(count)))+1)) { // #nosec G115 -- bit length is in [0,64].
		return 0
	}
	terms := make([]int, 0, count)
	collect := func(id int) { terms = append(terms, id) }
	visit(x, collect)
	visit(y, collect)
	if a.b.err != nil {
		return 0
	}
	slices.Sort(terms)
	terms = slices.Compact(terms)
	for changed := true; changed && a.b.err == nil; {
		changed = false
		for i := 0; i < len(terms) && !changed; i++ {
			for j := i + 1; j < len(terms); j++ {
				if !a.b.charge(0, 1) {
					return 0
				}
				left, right := a.nodes[terms[i]], a.nodes[terms[j]]
				merged := 0
				ok := false
				if left.op == OpConcat && right.op == OpConcat && left.a == right.a {
					merged = a.seq(left.a, a.alt(left.b, right.b))
					ok = true
				} else if left.op == OpRepeat && right.op == OpRepeat && left.a == right.a {
					lh, rh := left.hi, right.hi
					if lh < 0 {
						lh = int(^uint(0) >> 1)
					}
					if rh < 0 {
						rh = int(^uint(0) >> 1)
					}
					// Compare adjacent intervals without overflowing an unbounded upper end.
					if (left.lo <= rh || left.lo-1 == rh) && (right.lo <= lh || right.lo-1 == lh) {
						hi := max(left.hi, right.hi)
						if left.hi < 0 || right.hi < 0 {
							hi = -1
						}
						merged = a.rep(left.a, min(left.lo, right.lo), hi)
						ok = true
					}
				}
				if ok {
					terms[i] = merged
					terms = slices.Delete(terms, j, j+1)
					changed = true
					break
				}
			}
		}
	}
	if a.b.err != nil {
		return 0
	}
	slices.Sort(terms)
	terms = slices.Compact(terms)
	// Balanced alternatives keep recursive evaluation proportional to log arity.
	for len(terms) > 1 {
		next := terms[:0]
		for i := 0; i < len(terms); i += 2 {
			if i+1 == len(terms) {
				next = append(next, terms[i])
				continue
			}
			x, y := terms[i], terms[i+1]
			next = append(next, a.put(residual{op: OpAlternate, a: x, b: y, nullable: a.nodes[x].nullable || a.nodes[y].nullable}))
		}
		terms = next
	}
	return terms[0]
}
func (a *arena) derive(id, symbol int) int {
	key := transition{id, symbol}
	if out, ok := a.transitions[key]; ok {
		return out
	}
	if !a.enter() {
		return 0
	}
	defer func() { a.depth-- }()
	n := a.nodes[id]
	result := 0
	switch n.op {
	case OpEmpty, OpEpsilon:
	case OpClass:
		if symbol >= 0 && containsRune(a.program.data.Nodes[n.a].Ranges, a.endpoints[symbol]) {
			result = 1
		}
	case OpAlternate:
		result = a.alt(a.derive(n.a, symbol), a.derive(n.b, symbol))
	case OpConcat:
		result = a.seq(a.derive(n.a, symbol), n.b)
		if a.nodes[n.a].nullable {
			result = a.alt(result, a.derive(n.b, symbol))
		}
	case OpRepeat:
		upper := n.hi
		if upper >= 0 {
			upper--
		}
		result = a.seq(a.derive(n.a, symbol), a.rep(n.a, max(0, n.lo-1), upper))
	}
	if a.b.err != nil {
		return 0
	}
	if err := limitError("match", "derivative", "transitions", a.limits.Transitions, uint64(len(a.transitions)+1)); err != nil {
		a.b.err = err
		return 0
	}
	if !a.b.charge(96, 1) {
		return 0
	}
	a.transitions[key] = result
	return result
}
