// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

// resolveLeafrefRealtypes snapshots targets in dependency order after path
// resolution has finished across all modules. Resolving paths and copying types
// in one pass leaves forward-reference snapshots incomplete. The resulting
// snapshots remain immutable and their public accessors return defensive copies.
func (c *Context) resolveLeafrefRealtypes() {
	const (
		active   = 1
		complete = 2
	)
	state := make(map[*schemaNodeData]int)
	var visit func(*schemaNodeData) bool
	var resolveType func(ResolvedType) ResolvedType
	resolveType = func(resolved ResolvedType) ResolvedType {
		switch r := resolved.(type) {
		case ResolvedLeafRef:
			r.realtype = nil
			if r.target != nil && r.target.node != nil && r.target.node.typeInfo != nil && visit(r.target.node) {
				r.realtype = new(cloneTypeInfo(*r.target.node.typeInfo))
			}
			return r
		case ResolvedUnion:
			for i := range r.members {
				r.members[i].resolved = resolveType(r.members[i].resolved)
			}
			return r
		default:
			return resolved
		}
	}
	visit = func(n *schemaNodeData) bool {
		if !typeInfoContainsLeafref(n.typeInfo) {
			return true
		}
		if state[n] != 0 {
			// Loading policies can retain cycles. Keep their Target handles
			// for diagnostics, but stop Realtype snapshots at a back-edge so
			// recursive consumers cannot follow an infinite type structure.
			return state[n] == complete
		}
		state[n] = active
		n.typeInfo.resolved = resolveType(n.typeInfo.resolved)
		state[n] = complete
		return true
	}
	for _, mod := range c.loadOrder {
		if mod == nil || mod.root == nil {
			continue
		}
		walkSchemaNodes(mod.root, func(n *schemaNodeData) bool {
			visit(n)
			return true
		})
	}
}
