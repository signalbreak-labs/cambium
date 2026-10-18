// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

// Package yangpattern implements bounded, full-string XML Schema pattern matching.
// Inversion belongs to the caller, and must only be applied after a nil error.
package yangpattern

import (
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// FormatVersion is the portable semantic representation version.
const FormatVersion uint32 = 1

// Op identifies a portable language node.
type Op uint8

// Portable language operations.
const (
	OpEmpty Op = iota
	OpEpsilon
	OpClass
	OpAlternate
	OpConcat
	OpRepeat
)

// RuneRange is an inclusive Unicode scalar range. Surrogate code points are excluded.
type RuneRange struct{ Lo, Hi rune }

// NodeData is a portable language node. Only repetition nodes have bounds;
// Unbounded requires an empty Max. Children are ordered node indexes.
type NodeData struct {
	Op        Op
	Children  []uint32
	Ranges    []RuneRange
	Min, Max  string
	Unbounded bool
}

// ProgramData contains no native-engine state or policy. Bounds are canonical
// decimal strings, and repeat children must be nonnullable. Source is diagnostic.
type ProgramData struct {
	Version uint32
	Source  string
	Nodes   []NodeData
	Root    uint32
}

type repetitionBounds struct{ lower, upper uint64 }

// Program is immutable after construction and safe for concurrent matching.
// Its zero value is invalid; construct it with Compile or Restore.
type Program struct {
	data               ProgramData
	nullable           []bool
	minimum            []uint64
	bounds             []repetitionBounds
	native             *regexp.Regexp
	nativeSource       string
	nativeInstructions uint64
}

// Export returns a deep copy of the original portable program.
func (p *Program) Export() ProgramData {
	if p == nil {
		return ProgramData{}
	}
	return copyData(p.data)
}
func copyData(d ProgramData) ProgramData {
	out := d
	out.Source = strings.Clone(d.Source)
	out.Nodes = slices.Clone(d.Nodes)
	for i := range out.Nodes {
		n := &out.Nodes[i]
		n.Children = slices.Clone(n.Children)
		n.Ranges = slices.Clone(n.Ranges)
		n.Min = strings.Clone(n.Min)
		n.Max = strings.Clone(n.Max)
	}
	return out
}

// Restore validates and copies portable metadata using the default restore policy.
func Restore(d ProgramData) (*Program, error) { return RestoreWithLimits(d, RestoreLimits{}) }

// RestoreWithLimits checks all metadata, including unreachable nodes, before copying.
func RestoreWithLimits(d ProgramData, limits RestoreLimits) (*Program, error) {
	l := limits.defaults()
	b := ledger{phase: "restore", stage: "validate", byteLimit: l.Bytes, workLimit: l.Work}
	if d.Version != FormatVersion || len(d.Nodes) == 0 || uint64(d.Root) >= uint64(len(d.Nodes)) {
		return nil, &ConfigurationError{"invalid version, root, or empty node table"}
	}
	if err := limitError("restore", "validate", "nodes", l.Nodes, uint64(len(d.Nodes))); err != nil {
		return nil, err
	}
	if !b.charge(addSat(uint64(len(d.Source)), mulSat(uint64(len(d.Nodes)), 272)), uint64(len(d.Nodes))) {
		return nil, b.err
	}
	var edges, ranges, digits uint64
	for _, n := range d.Nodes {
		edges = addSat(edges, uint64(len(n.Children)))
		ranges = addSat(ranges, uint64(len(n.Ranges)))
		digits = addSat(digits, uint64(len(n.Min)+len(n.Max)))
		for _, c := range []struct {
			name        string
			max, actual uint64
		}{{"edges", l.Edges, edges}, {"ranges", l.Ranges, ranges}, {"digits", l.Digits, digits}} {
			if err := limitError("restore", "validate", c.name, c.max, c.actual); err != nil {
				return nil, err
			}
		}
		if !b.charge(addSat(mulSat(uint64(len(n.Children)), 8), addSat(mulSat(uint64(len(n.Ranges)), 16), uint64(len(n.Min)+len(n.Max)))), addSat(uint64(len(n.Children)+len(n.Ranges)), mulSat(uint64(len(n.Min)+len(n.Max)), 2))) {
			return nil, b.err
		}
		arity := 0
		switch n.Op {
		case OpEmpty, OpEpsilon:
		case OpClass:
			if len(n.Ranges) == 0 {
				return nil, &ConfigurationError{"empty class must use empty-language node"}
			}
		case OpAlternate, OpConcat:
			arity = 2
		case OpRepeat:
			arity = 1
		default:
			return nil, &ConfigurationError{"unknown opcode"}
		}
		if len(n.Children) != arity {
			return nil, &ConfigurationError{"invalid node arity"}
		}
		if n.Op != OpClass && len(n.Ranges) != 0 {
			return nil, &ConfigurationError{"ranges on non-class node"}
		}
		if n.Op != OpRepeat && (n.Min != "" || n.Max != "" || n.Unbounded) {
			return nil, &ConfigurationError{"bounds on non-repeat node"}
		}
		if n.Op == OpRepeat && (!canonicalDecimal(n.Min) || (!n.Unbounded && (!canonicalDecimal(n.Max) || decimalCompare(n.Min, n.Max) > 0)) || (n.Unbounded && n.Max != "")) {
			return nil, &ConfigurationError{"invalid repetition bounds"}
		}
		for _, c := range n.Children {
			if uint64(c) >= uint64(len(d.Nodes)) {
				return nil, &ConfigurationError{"child index out of bounds"}
			}
		}
		previous := rune(-1)
		for _, r := range n.Ranges {
			if r.Lo < 0 || r.Hi < r.Lo || r.Hi > utf8.MaxRune || r.Lo <= previous || (r.Lo <= 0xDFFF && r.Hi >= 0xD800) {
				return nil, &ConfigurationError{"invalid scalar ranges"}
			}
			previous = r.Hi
		}
	}
	color := make([]uint8, len(d.Nodes))
	nullable := make([]bool, len(d.Nodes))
	minimum := make([]uint64, len(d.Nodes))
	bounds := make([]repetitionBounds, len(d.Nodes))
	depths := make([]uint64, len(d.Nodes))
	var visit func(uint32, uint64) error
	visit = func(id uint32, depth uint64) error {
		if err := limitError("restore", "validate", "depth", l.Depth, depth); err != nil {
			return err
		}
		if color[id] == 1 {
			return &ConfigurationError{"cyclic program"}
		}
		if color[id] == 2 {
			return limitError("restore", "validate", "depth", l.Depth, addSat(depth, depths[id])-1)
		}
		if !b.charge(0, 1) {
			return b.err
		}
		color[id] = 1
		n := d.Nodes[id]
		height := uint64(1)
		for _, c := range n.Children {
			if err := visit(c, depth+1); err != nil {
				return err
			}
			height = max(height, depths[c]+1)
		}
		switch n.Op {
		case OpEmpty:
			minimum[id] = ^uint64(0)
		case OpEpsilon:
			nullable[id] = true
		case OpClass:
			minimum[id] = 1
		case OpAlternate:
			a, z := n.Children[0], n.Children[1]
			nullable[id] = nullable[a] || nullable[z]
			minimum[id] = min(minimum[a], minimum[z])
		case OpConcat:
			a, z := n.Children[0], n.Children[1]
			nullable[id] = nullable[a] && nullable[z]
			minimum[id] = addSat(minimum[a], minimum[z])
		case OpRepeat:
			c := n.Children[0]
			if nullable[c] {
				return &ConfigurationError{"nullable repetition child"}
			}
			// Canonical validation/comparison was reserved above. Cache numeric
			// bounds in a separately charged pass, once per immutable node, so no
			// matching engine scans arbitrary-length decimal metadata.
			if !b.charge(0, uint64(len(n.Min)+len(n.Max))) {
				return b.err
			}
			bounds[id] = repetitionBounds{lower: decimalUint(n.Min), upper: decimalUint(n.Max)}
			nullable[id] = n.Min == "0"
			minimum[id] = mulSat(bounds[id].lower, minimum[c])
			if n.Min == "0" {
				minimum[id] = 0
			}
		}
		color[id] = 2
		depths[id] = height
		return nil
	}
	for i := range d.Nodes {
		if err := visit(uint32(i), 1); err != nil {
			return nil, err
		}
	}
	p := &Program{data: copyData(d), nullable: nullable, minimum: minimum, bounds: bounds}
	// Native preparation is optional. Matching always rechecks its own policy.
	p.prepareNative(l.Bytes-b.bytes, l.Work-b.work)
	return p, nil
}
func canonicalDecimal(s string) bool {
	if s == "" || (len(s) > 1 && s[0] == '0') {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
func decimalCompare(a, b string) int {
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return strings.Compare(a, b)
}
func decimalUint(s string) uint64 {
	var n uint64
	for i := range len(s) {
		n = addSat(mulSat(n, 10), uint64(s[i]-'0'))
	}
	return n
}
