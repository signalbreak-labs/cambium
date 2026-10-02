// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat

import (
	"fmt"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// ContextProjectionOptions controls the read-only compatibility projection of
// a native context. Its zero value projects every loaded module.
type ContextProjectionOptions struct {
	// ExcludedModules omits module roots and schema nodes whose effective native
	// Module().Name() matches an entry. Names match exactly; duplicates and names
	// absent from the context have no effect. Omitting a node omits its entire
	// subtree, including descendants owned by other modules. Grouping nodes
	// instantiated in an included module remain even when their source module
	// is excluded. The native context and its types and validation are unchanged.
	ExcludedModules []string
}

// FromContext projects a live frozen native context without loading or
// recompiling sources. Nil, mutable, and closed contexts return an error.
// Requested modules come first, then transitive imports,
// in each group's load order. Each loaded module is projected once, including
// imports that are not implemented. All entries share a native-node index for
// ResolveLeafref. A sibling local-name collision, including after flattening
// choice/case nodes, returns an error and no roots because name-only lookups
// cannot represent both nodes. Keep the context alive while using its read-only
// projections or native handles. Use FromContextWithOptions to omit explicitly
// excluded modules before collision checking.
func FromContext(ctx *cambium.Context) ([]*Entry, error) {
	return FromContextWithOptions(ctx, ContextProjectionOptions{})
}

// FromContextWithOptions projects a live frozen native context with explicit
// module exclusions applied before sibling collision checks and shared index
// construction. Other behavior is the same as FromContext. Leafrefs to omitted
// targets return an outside-projection error from Entry.ResolveLeafref.
func FromContextWithOptions(ctx *cambium.Context, options ContextProjectionOptions) ([]*Entry, error) {
	if !ctx.IsFrozen() {
		return nil, fmt.Errorf("compat: FromContext requires a live frozen native context from ContextBuilder.Build")
	}
	excluded := make(map[string]bool, len(options.ExcludedModules))
	for _, name := range options.ExcludedModules {
		excluded[name] = true
	}
	report := ctx.LoadReport()
	seen := make(map[cambium.Module]bool)
	index := make(map[cambium.SchemaNodeRef]*Entry)
	var roots []*Entry
	for _, modules := range [][]cambium.ModuleLoadInfo{report.RequestedModules, report.TransitiveImports} {
		for _, info := range modules {
			if seen[info.Module] || excluded[info.Module.Name()] {
				continue
			}
			seen[info.Module] = true
			root := entryFromCambiumModule(info.Module, excluded)
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
// same FromContext or FromContextWithOptions projection. Repeated calls follow
// chains across modules and augments without reparsing paths. Native resolution
// errors are preserved. Targets omitted by projection options return an error
// identifying the target's qualified path outside the projection.
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
