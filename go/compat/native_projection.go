// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat

import (
	"fmt"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// FromContext projects a live frozen native context without loading or
// recompiling sources. Nil, mutable, and closed contexts return an error.
// Requested modules come first, then transitive imports,
// in each group's load order. Each loaded module is projected once, including
// imports that are not implemented. All entries share a native-node index for
// ResolveLeafref. A sibling local-name collision, including after flattening
// choice/case nodes, returns an error and no roots because name-only lookups
// cannot represent both nodes. Keep the context alive while using its read-only
// projections or native handles.
func FromContext(ctx *cambium.Context) ([]*Entry, error) {
	if !ctx.IsFrozen() {
		return nil, fmt.Errorf("compat: FromContext requires a live frozen native context from ContextBuilder.Build")
	}
	report := ctx.LoadReport()
	seen := make(map[cambium.Module]bool)
	index := make(map[cambium.SchemaNodeRef]*Entry)
	var roots []*Entry
	for _, modules := range [][]cambium.ModuleLoadInfo{report.RequestedModules, report.TransitiveImports} {
		for _, info := range modules {
			if seen[info.Module] {
				continue
			}
			seen[info.Module] = true
			root := FromModule(info.Module)
			if err := indexNativeProjection(root, index); err != nil {
				return nil, err
			}
			roots = append(roots, root)
		}
	}
	return roots, nil
}

func indexNativeProjection(entry *Entry, index map[cambium.SchemaNodeRef]*Entry) error {
	for _, flatten := range []bool{false, true} {
		if err := checkNativeChildNames(entry, flatten); err != nil {
			return err
		}
	}
	entry.nativeEntries = index
	if node, ok := entry.NativeSchemaNode(); ok {
		index[node] = entry
	}
	for _, child := range entry.ordered {
		if err := indexNativeProjection(child, index); err != nil {
			return err
		}
	}
	return nil
}

func checkNativeChildNames(parent *Entry, flatten bool) error {
	seen := make(map[string]*Entry, len(parent.ordered))
	var check func([]*Entry) error
	check = func(children []*Entry) error {
		for _, child := range children {
			if flatten && (child.IsChoice() || child.IsCase()) {
				if err := check(child.ordered); err != nil {
					return err
				}
				continue
			}
			if previous := seen[child.Name]; previous != nil {
				return fmt.Errorf("compat: sibling name collision %q under %s between %s and %s; name-only lookup cannot represent both",
					child.Name, parent.Path(), previous.schemaNode.QualifiedPath(), child.schemaNode.QualifiedPath())
			}
			seen[child.Name] = child
		}
		return nil
	}
	return check(parent.ordered)
}

// NativeSchemaNode returns the native schema handle backing e. Nil entries,
// module roots, and entries constructed only from ASTs have no native node.
// Native synthetic nodes (such as implicit cases) retain their native handle.
func (e *Entry) NativeSchemaNode() (cambium.SchemaNodeRef, bool) {
	if e == nil {
		return cambium.SchemaNodeRef{}, false
	}
	return e.schemaNode, e.schemaNode != (cambium.SchemaNodeRef{})
}

// NativeModule returns the native defining module of e, or the projected
// module for a module root. Nil and AST-only entries have no native module.
func (e *Entry) NativeModule() (cambium.Module, bool) {
	if e == nil {
		return cambium.Module{}, false
	}
	return e.module, e.module != (cambium.Module{})
}

// ResolveLeafref resolves one native leafref hop to the existing Entry in the
// same FromContext projection. Repeated calls follow chains across modules and
// augments without reparsing paths. Native resolution errors are preserved.
// Entries from FromModule or AST projections lack the shared context index and
// return an unsupported-context error.
func (e *Entry) ResolveLeafref() (*Entry, error) {
	node, ok := e.NativeSchemaNode()
	if !ok {
		return nil, fmt.Errorf("compat: ResolveLeafref requires a native schema node from FromContext")
	}
	if e.nativeEntries == nil {
		return nil, fmt.Errorf("compat: ResolveLeafref requires the shared context from FromContext")
	}
	resolution, err := cambium.ResolveLeafref(node)
	if err != nil {
		return nil, err
	}
	target := e.nativeEntries[resolution.Target]
	if target == nil {
		return nil, fmt.Errorf("compat: leafref target %s is outside the FromContext projection", resolution.Target.QualifiedPath())
	}
	return target, nil
}

func projectNativeExtras(entry *Entry, node cambium.SchemaNodeRef) {
	if presence, ok := node.Presence(); ok {
		entry.Extra["presence"] = []any{&Value{Name: presence, Parent: entry.Node}}
	}
	for _, constraint := range node.Musts() {
		must := &Must{
			Name:         constraint.Expression(),
			Parent:       entry.Node,
			Description:  astValueFromOptional(constraint.Description()),
			Reference:    astValueFromOptional(constraint.Reference()),
			ErrorMessage: astValueFromOptional(constraint.ErrorMessage()),
			ErrorAppTag:  astValueFromOptional(constraint.ErrorAppTag()),
		}
		for _, value := range []*Value{must.Description, must.Reference, must.ErrorMessage, must.ErrorAppTag} {
			if value != nil {
				value.Parent = must
			}
		}
		entry.Extra["must"] = append(entry.Extra["must"], must)
	}
	for _, unique := range node.UniqueConstraints() {
		entry.Extra["unique"] = append(entry.Extra["unique"], &Value{Name: unique.Expression(), Parent: entry.Node})
	}
	for _, when := range node.Whens() {
		entry.Extra["when"] = append(entry.Extra["when"], &Value{
			Name:        when.Expression(),
			Parent:      entry.Node,
			Description: astValueFromOptional(when.Description()),
		})
	}
}
