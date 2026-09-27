// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

import (
	"fmt"
	"math"
)

// SchemaIRTableVersion is the version tag for the bounded node-table schema
// projection. It is a separate version from SchemaIRVersion because its shape
// differs: nodes appear once in a table and views are ordered ID references.
const SchemaIRTableVersion = "cambium.schema-ir.v2"

// SchemaIRNodeID indexes SchemaIRTable.Nodes.
type SchemaIRNodeID int

// SchemaIRNoParent is the Parent of a module top-level node.
const SchemaIRNoParent SchemaIRNodeID = -1

// SchemaIRTable is a bounded value projection of the loaded ordered schema.
// Every schema node is materialized exactly once in Nodes, in pre-order of the
// structural (Children) walk over modules in context load order. Children,
// DataChildren and ListKeys are ordered references into Nodes, so record count
// equals the number of unique schema nodes and reference count equals the
// number of view relationships, whatever the depth or view overlap.
type SchemaIRTable struct {
	Version string
	Modules []SchemaIRTableModule
	Nodes   []SchemaIRTableNode
	Errors  []Diagnostic
}

// SchemaIRTableModule is one loaded module in context load order. Children
// lists its top-level nodes in effective schema declaration order.
type SchemaIRTableModule struct {
	Module      Module
	Name        string
	Namespace   string
	Prefix      string
	Revision    string
	Implemented bool
	Source      SourceLocation
	Imports     []Import
	Includes    []Include
	Children    []SchemaIRNodeID
}

// SchemaIRTableNode carries the same per-node facts as SchemaIRNode, with the
// three views expressed as ordered node IDs. Parent is the structural parent,
// or SchemaIRNoParent at module top level.
type SchemaIRTableNode struct {
	ID                     SchemaIRNodeID
	Parent                 SchemaIRNodeID
	Ref                    SchemaNodeRef
	Name                   string
	Kind                   SchemaNodeKind
	LocalPath              string
	QualifiedPath          string
	NamespaceQualifiedPath string
	QualifiedName          QualifiedName
	Children               []SchemaIRNodeID
	DataChildren           []SchemaIRNodeID
	ListKeys               []SchemaIRNodeID
	KeyNames               []string
	Type                   *TypeInfo
	Defaults               []DefaultValue
	Config                 Config
	Mandatory              bool
	ReadOnly               bool
	Musts                  []MustConstraint
	Whens                  []WhenConstraint
	Uniques                []UniqueConstraint
	Source                 SourceLocation
	Provenance             SchemaProvenance
}

// SchemaIRStats measures the size of the schema projections without
// materializing the legacy nested one.
type SchemaIRStats struct {
	// Nodes is the number of unique schema nodes (SchemaIRTable records).
	Nodes int
	// Relationships is the number of Children, DataChildren and ListKeys
	// references in the table.
	Relationships int
	// PathBytes is the total length of the three path strings over all nodes.
	PathBytes int
	// V1Records is the number of SchemaIRNode records SchemaIR would
	// materialize, saturating at math.MaxUint64.
	V1Records uint64
}

// SchemaIRTable returns the bounded, versioned node-table projection.
// Rebuild failures are reported in Errors, as for SchemaIR.
func (c *Context) SchemaIRTable() SchemaIRTable {
	table := SchemaIRTable{Version: SchemaIRTableVersion}
	if c == nil || c.closed {
		return table
	}
	if err := c.rebuildIfDirty(); err != nil {
		diag := DiagnosticFromError(wrap("schema tree", err))
		diag.Message = "schema rebuild: " + diag.Message
		table.Errors = append(table.Errors, diag)
	}
	b := schemaIRTableBuilder{table: &table, ids: make(map[*schemaNodeData]SchemaIRNodeID)}
	for _, mod := range c.loadOrder {
		if mod == nil || mod.stmt == nil {
			continue
		}
		header := schemaIRModuleHeader(Module{mod: mod})
		out := SchemaIRTableModule{
			Module:      header.Module,
			Name:        header.Name,
			Namespace:   header.Namespace,
			Prefix:      header.Prefix,
			Revision:    header.Revision,
			Implemented: header.Implemented,
			Source:      header.Source,
			Imports:     header.Imports,
			Includes:    header.Includes,
		}
		for child := range header.Module.Children().Iter() {
			out.Children = append(out.Children, b.add(child, SchemaIRNoParent))
		}
		table.Modules = append(table.Modules, out)
	}
	// Views are resolved after the structural walk so every reference points
	// at the canonical record; a view target outside the structural tree is
	// appended once.
	for i := 0; i < len(table.Nodes); i++ {
		ref := table.Nodes[i].Ref
		var data, keys []SchemaIRNodeID
		for child := range ref.DataChildren(true).Iter() {
			data = append(data, b.lookupOrAdd(child))
		}
		for key := range ref.ListKeys().Iter() {
			keys = append(keys, b.lookupOrAdd(key))
		}
		table.Nodes[i].DataChildren = data
		table.Nodes[i].ListKeys = keys
	}
	return table
}

type schemaIRTableBuilder struct {
	table *SchemaIRTable
	ids   map[*schemaNodeData]SchemaIRNodeID
}

// add records ref and its structural descendants in pre-order.
func (b *schemaIRTableBuilder) add(ref SchemaNodeRef, parent SchemaIRNodeID) SchemaIRNodeID {
	if id, ok := b.ids[ref.node]; ok {
		return id
	}
	id := b.record(ref, parent)
	var children []SchemaIRNodeID
	for child := range ref.Children().Iter() {
		children = append(children, b.add(child, id))
	}
	b.table.Nodes[id].Children = children
	return id
}

func (b *schemaIRTableBuilder) lookupOrAdd(ref SchemaNodeRef) SchemaIRNodeID {
	if id, ok := b.ids[ref.node]; ok {
		return id
	}
	parent := SchemaIRNoParent
	if p, ok := ref.Parent(); ok {
		if id, ok := b.ids[p.node]; ok {
			parent = id
		}
	}
	return b.add(ref, parent)
}

func (b *schemaIRTableBuilder) record(ref SchemaNodeRef, parent SchemaIRNodeID) SchemaIRNodeID {
	id := SchemaIRNodeID(len(b.table.Nodes))
	b.ids[ref.node] = id
	node := schemaIRNodeFacts(ref)
	b.table.Nodes = append(b.table.Nodes, SchemaIRTableNode{
		ID:                     id,
		Parent:                 parent,
		Ref:                    node.Ref,
		Name:                   node.Name,
		Kind:                   node.Kind,
		LocalPath:              node.LocalPath,
		QualifiedPath:          node.QualifiedPath,
		NamespaceQualifiedPath: node.NamespaceQualifiedPath,
		QualifiedName:          node.QualifiedName,
		KeyNames:               node.KeyNames,
		Type:                   node.Type,
		Defaults:               node.Defaults,
		Config:                 node.Config,
		Mandatory:              node.Mandatory,
		ReadOnly:               node.ReadOnly,
		Musts:                  node.Musts,
		Whens:                  node.Whens,
		Uniques:                node.Uniques,
		Source:                 node.Source,
		Provenance:             node.Provenance,
	})
	return id
}

// SchemaIRStats reports projection sizes. It walks the schema once and never
// materializes the nested v1 projection, so it is safe to call before deciding
// whether SchemaIR is affordable.
func (c *Context) SchemaIRStats() SchemaIRStats {
	var stats SchemaIRStats
	if c == nil || c.closed {
		return stats
	}
	if err := c.rebuildIfDirty(); err != nil {
		return stats
	}
	seen := make(map[*schemaNodeData]bool)
	v1 := make(map[*schemaNodeData]uint64)
	var visit func(ref SchemaNodeRef)
	visit = func(ref SchemaNodeRef) {
		if seen[ref.node] {
			return
		}
		seen[ref.node] = true
		stats.Nodes++
		stats.PathBytes += len(localPathForNode(ref)) + len(ref.QualifiedPath()) + len(namespaceQualifiedPathForNode(ref))
		for child := range ref.Children().Iter() {
			stats.Relationships++
			visit(child)
		}
		for child := range ref.DataChildren(true).Iter() {
			stats.Relationships++
			visit(child)
		}
		for key := range ref.ListKeys().Iter() {
			stats.Relationships++
			visit(key)
		}
	}
	for _, mod := range c.loadOrder {
		if mod == nil || mod.stmt == nil {
			continue
		}
		for child := range (Module{mod: mod}).Children().Iter() {
			visit(child)
			stats.V1Records = saturatingAdd(stats.V1Records, schemaIRV1Records(child, v1))
		}
	}
	return stats
}

// schemaIRV1Records counts the records SchemaIR materializes for ref: itself
// plus full copies of every Children, DataChildren and ListKeys subtree.
func schemaIRV1Records(ref SchemaNodeRef, memo map[*schemaNodeData]uint64) uint64 {
	if n, ok := memo[ref.node]; ok {
		return n
	}
	total := uint64(1)
	for child := range ref.Children().Iter() {
		total = saturatingAdd(total, schemaIRV1Records(child, memo))
	}
	for child := range ref.DataChildren(true).Iter() {
		total = saturatingAdd(total, schemaIRV1Records(child, memo))
	}
	for key := range ref.ListKeys().Iter() {
		total = saturatingAdd(total, schemaIRV1Records(key, memo))
	}
	memo[ref.node] = total
	return total
}

func saturatingAdd(a, b uint64) uint64 {
	if a > math.MaxUint64-b {
		return math.MaxUint64
	}
	return a + b
}

// SchemaIRWithLimit returns SchemaIR only if the nested v1 projection would
// materialize at most maxRecords SchemaIRNode records. Otherwise it returns a
// DiagnosticResourceLimit error without materializing anything. Use
// SchemaIRTable for a projection whose size is linear in the schema.
func (c *Context) SchemaIRWithLimit(maxRecords uint64) (SchemaIR, error) {
	stats := c.SchemaIRStats()
	if stats.V1Records > maxRecords {
		return SchemaIR{Version: SchemaIRVersion}, wrap("schema tree", &DiagnosticError{
			Kind: DiagnosticResourceLimit,
			Err: fmt.Errorf("%s projection would materialize %d node records for %d unique schema nodes (limit %d); use SchemaIRTable (%s)",
				SchemaIRVersion, stats.V1Records, stats.Nodes, maxRecords, SchemaIRTableVersion),
		})
	}
	return c.SchemaIR(), nil
}
