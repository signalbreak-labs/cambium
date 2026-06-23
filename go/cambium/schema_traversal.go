// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

// TraversalProfile names direct-child traversals in YANG/schema terms. Each
// profile returns a stable ordered slice backed by Cambium's ordered IR, never
// by map iteration.
type TraversalProfile int

const (
	// TraversalStructuralChildren preserves structural choice/case nodes.
	TraversalStructuralChildren TraversalProfile = iota
	// TraversalDataChildren flattens choice/case nodes to the data nodes that
	// can appear in a payload.
	TraversalDataChildren
	// TraversalSerializationOrder is the direct payload order serializers use:
	// choice/case flattened and list keys first within list entries.
	TraversalSerializationOrder
	// TraversalSchemaDeclarationOrder preserves effective schema declaration
	// order, including structural nodes.
	TraversalSchemaDeclarationOrder
	// TraversalListEntryOrder is data-child order for one list entry, with keys
	// emitted in key-statement order before non-key children.
	TraversalListEntryOrder
)

// Traverse returns direct children for profile. Unknown profiles fall back to
// structural schema-declaration order.
func (n SchemaNodeRef) Traverse(profile TraversalProfile) SchemaChildren {
	switch profile {
	case TraversalDataChildren:
		return n.DataChildren(true)
	case TraversalSerializationOrder:
		return serializationChildren(n)
	case TraversalListEntryOrder:
		return listEntryChildren(n)
	case TraversalStructuralChildren, TraversalSchemaDeclarationOrder:
		return n.Children()
	default:
		return n.Children()
	}
}

// Traverse returns module-level children for profile.
func (m Module) Traverse(profile TraversalProfile) SchemaChildren {
	switch profile {
	case TraversalDataChildren, TraversalSerializationOrder, TraversalListEntryOrder:
		return m.TopLevel()
	case TraversalStructuralChildren, TraversalSchemaDeclarationOrder:
		return m.Children()
	default:
		return m.Children()
	}
}

// Slice returns a defensive copy of the ordered children.
func (c SchemaChildren) Slice() []SchemaNodeRef {
	return append([]SchemaNodeRef(nil), c.nodes...)
}

func serializationChildren(n SchemaNodeRef) SchemaChildren {
	if n.IsList() {
		return listEntryChildren(n)
	}
	return n.DataChildren(true)
}

func listEntryChildren(n SchemaNodeRef) SchemaChildren {
	data := n.DataChildren(true)
	if !n.IsList() {
		return data
	}
	keys := n.ListKeys()
	if keys.Len() == 0 {
		return data
	}
	keySet := make(map[SchemaNodeRef]bool, keys.Len())
	out := make([]SchemaNodeRef, 0, data.Len())
	for key := range keys.Iter() {
		keySet[key] = true
		out = append(out, key)
	}
	for child := range data.Iter() {
		if keySet[child] {
			continue
		}
		out = append(out, child)
	}
	return SchemaChildren{nodes: out}
}
