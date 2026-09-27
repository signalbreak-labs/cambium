// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

import (
	"fmt"
	"strings"

	"github.com/signalbreak-labs/cambium/go/internal/yangparse"
)

// validateLeafrefCycles rejects leafrefs whose chain of targets returns to a
// node already on the chain, which libyang rejects as a "circular chain of
// leafrefs". Vertices are leaves and leaf-lists whose type contains a
// leafref, directly or as a union member; each resolved leafref adds an edge to
// its target when the target's type also contains a leafref. A depth-first walk
// visits implemented modules in load order and nodes in schema order, so the
// first cycle reported depends only on the load order. Vendor-compatible mode
// records each cycle as a LoadReport warning instead of failing.
func (c *Context) validateLeafrefCycles() error {
	if c == nil {
		return nil
	}
	vendor := c.validationMode == ValidationVendorCompatible
	// pos is 0 for an unvisited node, the node's stack index plus one while
	// it is on the walk's stack, and leafrefCycleDone once it is finished.
	const leafrefCycleDone = -1
	pos := make(map[*schemaNodeData]int)
	var stack []*schemaNodeData
	var err error
	var visit func(start, n *schemaNodeData) bool
	visit = func(start, n *schemaNodeData) bool {
		stack = append(stack, n)
		pos[n] = len(stack)
		for _, target := range leafrefTargets(nil, n.typeInfo.resolved) {
			if !typeInfoContainsLeafref(target.typeInfo) {
				continue
			}
			switch p := pos[target]; {
			case p > 0:
				cycle := append(append([]*schemaNodeData(nil), stack[p-1:]...), target)
				if !vendor {
					err = leafrefCycleError(start, cycle)
					return true
				}
				recordLeafrefCycleWarning(start, cycle)
			case p == 0:
				if visit(start, target) {
					return true
				}
			}
		}
		stack = stack[:len(stack)-1]
		pos[n] = leafrefCycleDone
		return false
	}
	for _, mod := range c.loadOrder {
		if mod == nil || mod.root == nil || !mod.implemented {
			continue
		}
		complete := walkSchemaNodes(mod.root, func(n *schemaNodeData) bool {
			return pos[n] != 0 || !typeInfoContainsLeafref(n.typeInfo) || !visit(n, n)
		})
		if !complete {
			return err
		}
	}
	return nil
}

// walkSchemaNodes calls fn on root's descendants in schema order until fn
// returns false.
func walkSchemaNodes(root *schemaNodeData, fn func(*schemaNodeData) bool) bool {
	for _, child := range root.children {
		if !fn(child) || !walkSchemaNodes(child, fn) {
			return false
		}
	}
	return true
}

// leafrefTargets appends the resolved target of each leafref in resolved,
// following union members in declaration order.
func leafrefTargets(out []*schemaNodeData, resolved ResolvedType) []*schemaNodeData {
	switch r := resolved.(type) {
	case ResolvedLeafRef:
		if r.target != nil && r.target.node != nil {
			out = append(out, r.target.node)
		}
	case ResolvedUnion:
		for _, member := range r.members {
			out = leafrefTargets(out, member.resolved)
		}
	}
	return out
}

func typeInfoContainsLeafref(info *TypeInfo) bool {
	return info != nil && resolvedContainsLeafref(info.resolved)
}

func resolvedContainsLeafref(resolved ResolvedType) bool {
	switch r := resolved.(type) {
	case ResolvedLeafRef:
		return true
	case ResolvedUnion:
		for _, member := range r.members {
			if resolvedContainsLeafref(member.resolved) {
				return true
			}
		}
	}
	return false
}

// leafrefCycleError reports cycle, which starts and ends at the node where the
// walk from start closed it, in the LeafrefFailureCycle wording followed by the
// cycle's path chain.
func leafrefCycleError(start *schemaNodeData, cycle []*schemaNodeData) error {
	at := SchemaNodeRef{node: cycle[0]}
	cause := &LeafrefResolutionError{Reason: LeafrefFailureCycle, Node: at, Path: start.path}
	return &DiagnosticError{
		Kind:    DiagnosticSemanticSchemaError,
		Module:  at.Module().Name(),
		Path:    at.Path(),
		Source:  at.SourceLocation(),
		Related: sourceLocations(leafrefCycleStatements(cycle)),
		Err:     fmt.Errorf("%w: %s", cause, leafrefCyclePaths(cycle)),
	}
}

func recordLeafrefCycleWarning(start *schemaNodeData, cycle []*schemaNodeData) {
	at := cycle[0]
	at.recordVendorCompatibleWarning(at.stmt, leafrefCycleStatements(cycle),
		"%v; allowed in vendor-compatible mode", leafrefCycleError(start, cycle))
}

func leafrefCyclePaths(cycle []*schemaNodeData) string {
	paths := make([]string, len(cycle))
	for i, n := range cycle {
		paths[i] = n.path
	}
	return strings.Join(paths, " -> ")
}

// leafrefCycleStatements returns the statements of the cycle's nodes between
// the node it starts and ends at, for related source locations.
func leafrefCycleStatements(cycle []*schemaNodeData) []*yangparse.Statement {
	inner := cycle[1 : len(cycle)-1]
	out := make([]*yangparse.Statement, 0, len(inner))
	for _, n := range inner {
		out = append(out, n.stmt)
	}
	return out
}
