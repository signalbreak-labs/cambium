// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

import "strings"

// SchemaIRVersion is the stable version tag for the public ordered schema
// projection. Additive fields may appear in later Go releases; incompatible
// projection changes must use a new version string.
const SchemaIRVersion = "cambium.schema-ir.v1"

// SchemaIR is a value projection of the loaded ordered schema context for
// downstream schema consumers and external renderers.
type SchemaIR struct {
	Version string
	Modules []SchemaIRModule
}

// SchemaIRModule is one loaded implemented module in context load order.
type SchemaIRModule struct {
	Module      Module
	Name        string
	Namespace   string
	Prefix      string
	Revision    string
	Implemented bool
	Source      SourceLocation
	Imports     []Import
	Includes    []Include
	Children    []SchemaIRNode
}

// SchemaIRNode is one schema node in effective schema declaration order.
// Children preserves structural nodes; DataChildren flattens choice/case nodes.
type SchemaIRNode struct {
	Ref                    SchemaNodeRef
	Name                   string
	Kind                   SchemaNodeKind
	LocalPath              string
	QualifiedPath          string
	NamespaceQualifiedPath string
	QualifiedName          QualifiedName
	Children               []SchemaIRNode
	DataChildren           []SchemaIRNode
	ListKeys               []SchemaIRNode
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

// SchemaProvenance explains why a materialized schema node exists.
type SchemaProvenance struct {
	Source              SourceLocation
	DefiningModule      string
	InstantiatingModule string
	AugmentingModule    string
	Grouping            string
	Deviations          []Deviation
}

// SchemaIR returns a versioned value projection of the implemented modules in
// context load order.
func (c *Context) SchemaIR() SchemaIR {
	ir := SchemaIR{Version: SchemaIRVersion}
	if c == nil || c.closed {
		return ir
	}
	_ = c.rebuildIfDirty()
	for _, mod := range c.loadOrder {
		if mod == nil || mod.stmt == nil {
			continue
		}
		ir.Modules = append(ir.Modules, schemaIRModule(Module{mod: mod}))
	}
	return ir
}

// SchemaIR returns a versioned value projection of one module.
func (m Module) SchemaIR() SchemaIRModule {
	return schemaIRModule(m)
}

// LocalPath returns the node's absolute schema path relative to its module
// root, without module or namespace qualification.
func (n SchemaNodeRef) LocalPath() string {
	return localPathForNode(n)
}

// NamespaceQualifiedPath returns an absolute schema path whose segments are
// qualified with the defining module namespace in expanded-name form:
// /{namespace}local/{namespace}local.
func (n SchemaNodeRef) NamespaceQualifiedPath() string {
	return namespaceQualifiedPathForNode(n)
}

func schemaIRModule(mod Module) SchemaIRModule {
	revision, _ := mod.Revision()
	out := SchemaIRModule{
		Module:      mod,
		Name:        mod.Name(),
		Namespace:   mod.Namespace(),
		Prefix:      mod.Prefix(),
		Revision:    revision,
		Implemented: mod.IsImplemented(),
		Source:      mod.SourceLocation(),
		Imports:     mod.Imports(),
		Includes:    mod.Includes(),
	}
	for child := range mod.Children().Iter() {
		out.Children = append(out.Children, schemaIRNode(child))
	}
	return out
}

func schemaIRNode(ref SchemaNodeRef) SchemaIRNode {
	out := SchemaIRNode{
		Ref:                    ref,
		Name:                   ref.Name(),
		Kind:                   ref.Kind(),
		LocalPath:              localPathForNode(ref),
		QualifiedPath:          ref.QualifiedPath(),
		NamespaceQualifiedPath: namespaceQualifiedPathForNode(ref),
		QualifiedName:          ref.QualifiedName(),
		KeyNames:               ref.KeyNames(),
		Defaults:               ref.DefaultEntries(),
		Config:                 ref.Config(),
		Mandatory:              ref.IsMandatory(),
		ReadOnly:               ref.ReadOnly(),
		Musts:                  ref.Musts(),
		Whens:                  ref.Whens(),
		Uniques:                ref.UniqueConstraints(),
		Source:                 ref.SourceLocation(),
		Provenance:             schemaProvenance(ref),
	}
	if typ, ok := ref.LeafType(); ok {
		info := typ
		out.Type = &info
	}
	for child := range ref.Children().Iter() {
		out.Children = append(out.Children, schemaIRNode(child))
	}
	for child := range ref.DataChildren(true).Iter() {
		out.DataChildren = append(out.DataChildren, schemaIRNode(child))
	}
	for key := range ref.ListKeys().Iter() {
		out.ListKeys = append(out.ListKeys, schemaIRNode(key))
	}
	return out
}

func schemaProvenance(ref SchemaNodeRef) SchemaProvenance {
	prov := SchemaProvenance{
		Source:              ref.SourceLocation(),
		DefiningModule:      ref.SourceModule().Name(),
		InstantiatingModule: ref.InstantiatingModule().Name(),
		Deviations:          ref.DeviationProvenance(),
	}
	if group, ok := ref.GroupingOrigin(); ok {
		prov.Grouping = group
	}
	if ref.node != nil && ref.node.groupOrigin == "" && ref.node.module != nil && ref.InstantiatingModule().Name() != "" && ref.node.module.name != ref.InstantiatingModule().Name() {
		prov.AugmentingModule = ref.node.module.name
	}
	return prov
}

func localPathForNode(ref SchemaNodeRef) string {
	path := ref.Path()
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = localName(part)
	}
	if len(parts) > 1 {
		parts = append(parts[:1], parts[2:]...)
	}
	return strings.Join(parts, "/")
}

func namespaceQualifiedPathForNode(ref SchemaNodeRef) string {
	if ref.node == nil {
		return ""
	}
	var parts []string
	for cur := ref.node; cur != nil && cur.kind != SchemaNodeKindModule; cur = cur.parent {
		if cur.name == "" {
			continue
		}
		namespace := ""
		if cur.module != nil {
			namespace = cur.module.namespace
		}
		parts = append(parts, "{"+namespace+"}"+cur.name)
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	if len(parts) == 0 {
		return ""
	}
	return "/" + strings.Join(parts, "/")
}
