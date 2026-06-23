// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

import "fmt"

// LeafrefFailureReason classifies a leafref resolution failure.
type LeafrefFailureReason string

const (
	// LeafrefFailureNotLeafref means the source node is not a leafref.
	LeafrefFailureNotLeafref LeafrefFailureReason = "not_leafref"
	// LeafrefFailureUnresolvedTarget means the leafref path did not resolve to a
	// schema node.
	LeafrefFailureUnresolvedTarget LeafrefFailureReason = "unresolved_target"
	// LeafrefFailureCycle means following a leafref chain found a cycle.
	LeafrefFailureCycle LeafrefFailureReason = "cycle"
)

// LeafrefResolutionError is returned by leafref helpers for structured failure
// inspection with errors.As.
type LeafrefResolutionError struct {
	Reason LeafrefFailureReason
	Node   SchemaNodeRef
	Path   string
}

func (e *LeafrefResolutionError) Error() string {
	switch e.Reason {
	case LeafrefFailureNotLeafref:
		return fmt.Sprintf("%s is not a leafref", e.Node.Path())
	case LeafrefFailureCycle:
		return fmt.Sprintf("leafref chain from %s contains a cycle at %s", e.Path, e.Node.Path())
	default:
		if e.Path != "" {
			return fmt.Sprintf("leafref %s target %q did not resolve", e.Node.Path(), e.Path)
		}
		return fmt.Sprintf("leafref %s target did not resolve", e.Node.Path())
	}
}

// LeafrefTraceStep is one hop in leafref resolution.
type LeafrefTraceStep struct {
	From            SchemaNodeRef
	Path            string
	Target          SchemaNodeRef
	SourceModule    Module
	RequireInstance bool
}

// LeafrefResolution is the resolved target and trace for a leafref or leafref
// chain.
type LeafrefResolution struct {
	Source   SchemaNodeRef
	Target   SchemaNodeRef
	Realtype *TypeInfo
	Trace    []LeafrefTraceStep
}

// ResolveLeafref resolves one leafref hop from node.
func ResolveLeafref(node SchemaNodeRef) (LeafrefResolution, error) {
	lr, ok := leafrefForNode(node)
	if !ok {
		return LeafrefResolution{}, &LeafrefResolutionError{Reason: LeafrefFailureNotLeafref, Node: node}
	}
	target, ok := lr.Target()
	if !ok {
		path, _ := lr.Path()
		return LeafrefResolution{}, &LeafrefResolutionError{Reason: LeafrefFailureUnresolvedTarget, Node: node, Path: path}
	}
	realtype, _ := lr.Realtype()
	path, _ := lr.Path()
	return LeafrefResolution{
		Source:   node,
		Target:   target,
		Realtype: realtype,
		Trace: []LeafrefTraceStep{{
			From:            node,
			Path:            path,
			Target:          target,
			SourceModule:    lr.SourceModule(),
			RequireInstance: lr.RequireInstance(),
		}},
	}, nil
}

// ResolveLeafrefChain follows leafrefs until the terminal non-leafref target.
func ResolveLeafrefChain(node SchemaNodeRef) (LeafrefResolution, error) {
	source := node
	current := node
	seen := make(map[SchemaNodeRef]bool)
	var trace []LeafrefTraceStep
	var realtype *TypeInfo

	for {
		if seen[current] {
			return LeafrefResolution{}, &LeafrefResolutionError{Reason: LeafrefFailureCycle, Node: current, Path: source.Path()}
		}
		seen[current] = true
		lr, ok := leafrefForNode(current)
		if !ok {
			if len(trace) == 0 {
				return LeafrefResolution{}, &LeafrefResolutionError{Reason: LeafrefFailureNotLeafref, Node: source}
			}
			return LeafrefResolution{Source: source, Target: current, Realtype: realtype, Trace: trace}, nil
		}
		target, ok := lr.Target()
		path, _ := lr.Path()
		if !ok {
			return LeafrefResolution{}, &LeafrefResolutionError{Reason: LeafrefFailureUnresolvedTarget, Node: current, Path: path}
		}
		if resolved, ok := lr.Realtype(); ok {
			realtype = resolved
		}
		trace = append(trace, LeafrefTraceStep{
			From:            current,
			Path:            path,
			Target:          target,
			SourceModule:    lr.SourceModule(),
			RequireInstance: lr.RequireInstance(),
		})
		current = target
	}
}

func leafrefForNode(node SchemaNodeRef) (ResolvedLeafRef, bool) {
	info, ok := node.LeafType()
	if !ok {
		return ResolvedLeafRef{}, false
	}
	lr, ok := info.Resolved().(ResolvedLeafRef)
	return lr, ok
}

// Identity returns the identity named name from this module.
func (m Module) Identity(name string) (Identity, bool) {
	if m.mod == nil {
		return Identity{}, false
	}
	id := m.mod.identityMap[name]
	if id == nil || !m.mod.featureIncluded(id.stmt) {
		return Identity{}, false
	}
	return Identity{id: id}, true
}

// DerivedClosure returns the transitive derived identities in deterministic
// schema resolution order, skipping identities excluded by if-feature.
func (i Identity) DerivedClosure() []Identity {
	return i.Derived()
}
