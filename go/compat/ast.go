// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/signalbreak-labs/cambium/go/internal/yangparse"
	"github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream/indent"
	upstream "github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream/yang"
)

// Statement is a generic parsed YANG statement. The compat layer is the
// goyang-compatible surface, so it intentionally exposes the upstream-shaped
// statement type (and parses via upstream) rather than Cambium's own cgo-free
// parser, which the default schema/codegen tier uses.
type Statement = upstream.Statement

// Node is the goyang-style parsed AST node interface.
type Node = upstream.Node

// ErrorNode is a goyang-style AST node that carries an error.
type ErrorNode = upstream.ErrorNode

// Typedefer is a goyang-style AST node that defines typedefs.
type Typedefer = upstream.Typedefer

// ASTValue is the upstream-shaped parser value node. The shorter Value name is
// used by the compat Entry projection.
type ASTValue = upstream.Value

// ASTModule is the upstream-shaped parser module node. The shorter Module name
// is used by the compat Modules facade.
type ASTModule = upstream.Module

// ASTIdentity is the upstream-shaped parser identity node. The shorter Identity
// name is used by the compat Entry projection.
type ASTIdentity = upstream.Identity

type Import = upstream.Import
type Include = upstream.Include
type Revision = upstream.Revision
type BelongsTo = upstream.BelongsTo
type Typedef = upstream.Typedef
type Type = upstream.Type
type Container = upstream.Container
type Must = upstream.Must
type Leaf = upstream.Leaf
type LeafList = upstream.LeafList
type List = upstream.List
type Choice = upstream.Choice
type Case = upstream.Case
type AnyXML = upstream.AnyXML
type AnyData = upstream.AnyData
type Grouping = upstream.Grouping
type Uses = upstream.Uses
type Refine = upstream.Refine
type RPC = upstream.RPC
type Input = upstream.Input
type Output = upstream.Output
type Notification = upstream.Notification
type Augment = upstream.Augment
type Extension = upstream.Extension
type Argument = upstream.Argument
type Element = upstream.Element
type Feature = upstream.Feature
type Deviation = upstream.Deviation
type Deviate = upstream.Deviate
type Enum = upstream.Enum
type Bit = upstream.Bit
type Range = upstream.Range
type Length = upstream.Length
type Pattern = upstream.Pattern
type Action = upstream.Action

// BaseTypedefs contains goyang's manufactured typedefs for built-in YANG types.
var BaseTypedefs = upstream.BaseTypedefs

// Parse parses generic YANG source into ordered statements.
func Parse(input, path string) ([]*Statement, error) {
	return parseStatements(input, path)
}

// parseStatements parses YANG source into upstream-shaped statements, applying
// the same pre-parse bounds/safety checks as the default parser. The compat
// surface deliberately keeps the upstream parser; the default tier does not.
func parseStatements(input, name string) (stmts []*Statement, err error) {
	if err := yangparse.CheckInputBounds(input, name); err != nil {
		return nil, err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			stmts = nil
			err = fmt.Errorf("%s: YANG parser panic: %v", name, recovered)
		}
	}()
	return upstream.Parse(input, name)
}

// PathsWithModules returns directories under root that contain .yang files.
func PathsWithModules(root string) ([]string, error) {
	return upstream.PathsWithModules(root)
}

// Source returns the source location of n.
func Source(n Node) string {
	return upstream.Source(n)
}

// RootNode returns the module or submodule that n was defined in.
func RootNode(n Node) *Module {
	if n == nil {
		return nil
	}
	root := rootNode(n)
	if mod, ok := root.(*Module); ok {
		return mod
	}
	if mod, ok := root.(*ASTModule); ok {
		return moduleFromASTModule(mod)
	}
	return nil
}

// FindModuleByPrefix resolves prefix relative to n.
func FindModuleByPrefix(n Node, prefix string) *Module {
	root := rootNode(n)
	mod := RootNode(n)
	if mod != nil {
		if prefix == "" || prefix == mod.GetPrefix() || prefix == mod.Name {
			return mod
		}
		if mod.Modules != nil {
			for _, imp := range mod.Import {
				if imp != nil && imp.Prefix != nil && imp.Prefix.Name == prefix {
					if imported := mod.Modules.FindModule(imp); imported != nil {
						return imported
					}
				}
			}
		}
	}
	if _, ok := root.(*ASTModule); !ok {
		return nil
	}
	return moduleFromASTModule(upstream.FindModuleByPrefix(n, prefix))
}

// MatchingExtensions returns extension statements matching module and identifier.
func MatchingExtensions(n Node, module, identifier string) ([]*Statement, error) {
	if n == nil {
		return nil, nil
	}
	return matchingExtensions(n, n.Exts(), module, identifier)
}

func matchingExtensions(n Node, exts []*Statement, module, identifier string) ([]*Statement, error) {
	var out []*Statement
	for _, ext := range exts {
		if ext == nil {
			continue
		}
		names := strings.SplitN(ext.Keyword, ":", 2)
		mod := FindModuleByPrefix(n, names[0])
		if mod == nil {
			return nil, fmt.Errorf("matchingExtensions: module prefix %q not found", names[0])
		}
		if len(names) == 2 && mod.Name == module && names[1] == identifier {
			out = append(out, ext)
		}
	}
	return out, nil
}

func rootNode(n Node) Node {
	if n == nil {
		return nil
	}
	for n.ParentNode() != nil {
		n = n.ParentNode()
	}
	return n
}

// MatchingEntryExtensions returns extension statements on e matching module and identifier.
func MatchingEntryExtensions(e *Entry, module, identifier string) ([]*Statement, error) {
	if e == nil {
		return nil, nil
	}
	var out []*Statement
	for _, ext := range e.Exts {
		if ext == nil {
			continue
		}
		names := strings.SplitN(ext.Keyword, ":", 2)
		extModule := entryExtensionModule(e, names[0])
		if extModule == "" {
			return nil, fmt.Errorf("matchingExtensions: module prefix %q not found", names[0])
		}
		if len(names) == 2 && extModule == module && names[1] == identifier {
			out = append(out, ext)
		}
	}
	return out, nil
}

func entryExtensionModule(e *Entry, prefixOrModule string) string {
	if e == nil {
		return ""
	}
	if e.module.Name() == prefixOrModule {
		return prefixOrModule
	}
	if e.module.Prefix() == prefixOrModule {
		return e.module.Name()
	}
	for _, imp := range e.module.Imports() {
		if imp.Prefix == prefixOrModule || imp.Name == prefixOrModule {
			return imp.Name
		}
	}
	if root := e.rawRootEntry(); root != nil {
		if moduleName := rawEntryExtensionModule(root, prefixOrModule); moduleName != "" {
			return moduleName
		}
	}
	return ""
}

func rawEntryExtensionModule(root *Entry, prefixOrModule string) string {
	if root == nil || prefixOrModule == "" {
		return ""
	}
	switch module := root.Node.(type) {
	case *ASTModule:
		if rawModuleMatches(module, prefixOrModule) {
			return rawModuleName(module)
		}
		for _, imp := range module.Import {
			if imp == nil || imp.Prefix == nil {
				continue
			}
			if imp.Prefix.Name == prefixOrModule || imp.Name == prefixOrModule {
				return imp.Name
			}
		}
	case *Module:
		if module.Name == prefixOrModule || module.GetPrefix() == prefixOrModule {
			return module.Name
		}
		for _, imp := range module.Import {
			if imp == nil || imp.Prefix == nil {
				continue
			}
			if imp.Prefix.Name == prefixOrModule || imp.Name == prefixOrModule {
				return imp.Name
			}
		}
	}
	return ""
}

func rawModuleMatches(module *ASTModule, prefixOrModule string) bool {
	if module == nil {
		return false
	}
	return module.Name == prefixOrModule || module.GetPrefix() == prefixOrModule
}

func rawModuleName(module *ASTModule) string {
	if module == nil {
		return ""
	}
	if module.BelongsTo != nil {
		return module.BelongsTo.Name
	}
	return module.Name
}

// FindGrouping finds a grouping by YANG namespace rules from n.
func FindGrouping(n Node, name string, seen map[string]bool) *Grouping {
	if n == nil {
		return nil
	}
	if seen == nil {
		seen = make(map[string]bool)
	}
	name = trimLocalGroupingPrefix(n, name)
	for n != nil {
		for _, grouping := range groupingNodes(n) {
			if grouping != nil && grouping.Name == name {
				return grouping
			}
		}
		for _, imp := range importNodes(n) {
			if imp == nil || imp.Prefix == nil {
				continue
			}
			importedName := strings.TrimPrefix(name, imp.Prefix.Name+":")
			if importedName == name {
				continue
			}
			mod := moduleForImport(n, imp)
			if mod == nil {
				continue
			}
			if grouping := FindGrouping(mod, importedName, seen); grouping != nil {
				return grouping
			}
		}
		for _, inc := range includeNodes(n) {
			mod := moduleForInclude(n, inc)
			if mod == nil {
				continue
			}
			key := groupingSeenKey(mod)
			if seen[key] {
				continue
			}
			seen[key] = true
			if grouping := FindGrouping(mod, name, seen); grouping != nil {
				return grouping
			}
		}
		n = n.ParentNode()
	}
	return nil
}

func trimLocalGroupingPrefix(n Node, name string) string {
	mod := RootNode(n)
	if mod == nil {
		return name
	}
	prefix := mod.GetPrefix()
	if prefix == "" {
		return name
	}
	return strings.TrimPrefix(name, prefix+":")
}

func groupingNodes(n Node) []*Grouping {
	field := nodeField(n, "Grouping")
	if !field.IsValid() || field.IsNil() || !field.CanInterface() {
		return nil
	}
	groupings, _ := field.Interface().([]*Grouping)
	return groupings
}

func importNodes(n Node) []*Import {
	field := nodeField(n, "Import")
	if !field.IsValid() || field.IsNil() || !field.CanInterface() {
		return nil
	}
	imports, _ := field.Interface().([]*Import)
	return imports
}

func includeNodes(n Node) []*Include {
	field := nodeField(n, "Include")
	if !field.IsValid() || field.IsNil() || !field.CanInterface() {
		return nil
	}
	includes, _ := field.Interface().([]*Include)
	return includes
}

func nodeField(n Node, name string) reflect.Value {
	value := reflect.ValueOf(n)
	if value.Kind() != reflect.Pointer || value.IsNil() || value.Elem().Kind() != reflect.Struct {
		return reflect.Value{}
	}
	return value.Elem().FieldByName(name)
}

func moduleForImport(context Node, imp *Import) Node {
	if imp == nil {
		return nil
	}
	if imp.Module != nil {
		return imp.Module
	}
	mod := RootNode(context)
	if mod == nil || mod.Modules == nil {
		return nil
	}
	return mod.Modules.FindModule(imp)
}

func moduleForInclude(context Node, inc *Include) Node {
	if inc == nil {
		return nil
	}
	if inc.Module != nil {
		return inc.Module
	}
	mod := RootNode(context)
	if mod == nil || mod.Modules == nil {
		return nil
	}
	return mod.Modules.FindModule(inc)
}

func groupingSeenKey(n Node) string {
	if n == nil {
		return ""
	}
	if full, ok := n.(interface{ FullName() string }); ok {
		return n.Kind() + ":" + full.FullName()
	}
	return n.Kind() + ":" + n.NName() + ":" + Source(n)
}

// NodePath returns the path from the root node to n.
func NodePath(n Node) string {
	return upstream.NodePath(n)
}

// FindNode resolves path relative to n using goyang AST traversal semantics.
func FindNode(n Node, path string) (Node, error) {
	if path == "" {
		return n, nil
	}
	if path == "/" || strings.HasSuffix(path, "/") {
		return nil, fmt.Errorf("invalid path %q", path)
	}
	parts := strings.Split(path, "/")
	if parts[0] == "" {
		parts = parts[1:]
		mod := RootNode(n)
		if mod == nil {
			return nil, fmt.Errorf("unknown root for path %q", path)
		}
		n = mod
		prefix, _ := splitPrefix(parts[0])
		if mod.Kind() == "submodule" {
			parentName := ""
			if mod.BelongsTo != nil {
				parentName = mod.BelongsTo.Name
			}
			parent := (*Module)(nil)
			if mod.Modules != nil {
				parent = mod.Modules.Modules[parentName]
			}
			if parent == nil {
				return nil, fmt.Errorf("%s: unknown module %s", mod.Name, parentName)
			}
			belongsPrefix := ""
			if mod.BelongsTo != nil && mod.BelongsTo.Prefix != nil {
				belongsPrefix = mod.BelongsTo.Prefix.Name
			}
			if prefix != "" && prefix != belongsPrefix {
				mod = parent
				n = parent
			}
		}
		if prefix != "" && prefix != mod.GetPrefix() {
			if mod.Modules == nil {
				return nil, fmt.Errorf("unknown prefix: %q", prefix)
			}
			found := false
			for _, imp := range mod.Import {
				if imp != nil && imp.Prefix != nil && imp.Prefix.Name == prefix {
					imported := mod.Modules.FindModule(imp)
					if imported == nil {
						return nil, fmt.Errorf("unknown prefix: %q", prefix)
					}
					n = imported
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("unknown prefix: %q", prefix)
			}
		}
	}
	for _, part := range parts {
		if n.Kind() == "rpc" {
			return &ErrorNode{Error: errors.New("rpc is unsupported")}, nil
		}
		if part == ".." {
		Loop:
			for {
				n = n.ParentNode()
				if n == nil {
					return nil, fmt.Errorf(".. with no parent")
				}
				switch n.Kind() {
				case "choice", "leaf", "case":
				default:
					break Loop
				}
			}
			continue
		}
		_, local := splitPrefix(part)
		n = ChildNode(n, local)
		if n == nil {
			return nil, fmt.Errorf("%s: no such element", part)
		}
	}
	return n, nil
}

// ChildNode returns the direct child named name, if present.
func ChildNode(n Node, name string) Node {
	if n == nil {
		return nil
	}
	v := reflect.ValueOf(n)
	if v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		return nil
	}
	v = v.Elem()
	t := v.Type()
	fieldCount := t.NumField()

Loop:
	for i := 0; i < fieldCount; i++ {
		fieldType := t.Field(i)
		yang := fieldType.Tag.Get("yang")
		if yang == "" {
			continue
		}
		parts := strings.Split(yang, ",")
		for _, part := range parts[1:] {
			if part == "nomerge" {
				continue Loop
			}
		}

		field := v.Field(i)
		if !field.IsValid() || field.IsNil() {
			continue
		}

		check := func(child Node) Node {
			if child.NName() == name {
				return child
			}
			return nil
		}
		if parts[0] == "uses" {
			check = func(child Node) Node {
				useName := child.NName()
				if !strings.HasPrefix(useName, "/") {
					useName = "/" + useName
				}
				found, _ := FindNode(child, useName)
				if found != nil {
					return ChildNode(found, name)
				}
				return nil
			}
		}

		switch fieldType.Type.Kind() {
		case reflect.Pointer:
			child, ok := field.Interface().(Node)
			if !ok {
				continue
			}
			if found := check(child); found != nil {
				return found
			}
		case reflect.Slice:
			for j := 0; j < field.Len(); j++ {
				child, ok := field.Index(j).Interface().(Node)
				if !ok {
					continue
				}
				if found := check(child); found != nil {
					return found
				}
			}
		}
	}
	if found := childNodeFromSource(n, name); found != nil {
		return found
	}
	return nil
}

func splitPrefix(s string) (string, string) {
	prefix, local, ok := strings.Cut(s, ":")
	if !ok {
		return "", s
	}
	return prefix, local
}

func childNodeFromSource(parent Node, name string) Node {
	if parent == nil || parent.Statement() == nil {
		return nil
	}
	for _, stmt := range parent.Statement().SubStatements() {
		if stmt == nil || stmt.Argument != name {
			continue
		}
		if node := nodeFromStatement(stmt, parent); node != nil {
			return node
		}
	}
	for _, stmt := range parent.Statement().SubStatements() {
		if stmt == nil || stmt.Keyword != "uses" {
			continue
		}
		uses := usesFromStatement(stmt, parent)
		if uses == nil {
			continue
		}
		useName := uses.NName()
		if !strings.HasPrefix(useName, "/") {
			useName = "/" + useName
		}
		grouping, _ := FindNode(uses, useName)
		if grouping != nil {
			if found := ChildNode(grouping, name); found != nil {
				return found
			}
		}
	}
	return nil
}

func nodeFromStatement(stmt *Statement, parent Node) Node {
	switch stmt.Keyword {
	case "action":
		return actionFromStatement(stmt, parent)
	case "anydata":
		return anyDataFromStatement(stmt, parent)
	case "anyxml":
		return anyXMLFromStatement(stmt, parent)
	case "augment":
		return augmentFromStatement(stmt, parent)
	case "case":
		return caseFromStatement(stmt, parent)
	case "choice":
		return choiceFromStatement(stmt, parent)
	case "container":
		return containerFromStatement(stmt, parent)
	case "deviation":
		return deviationFromStatement(stmt, parent)
	case "deviate":
		return deviateFromStatement(stmt, parent)
	case "extension":
		return extensionDefFromStatement(stmt, parent)
	case "feature":
		return featureFromStatement(stmt, parent)
	case "grouping":
		return groupingFromStatementWithParent(stmt, parent)
	case "identity":
		return astIdentityFromStatementWithParent(stmt, parent)
	case "input":
		return inputFromStatement(stmt, parent)
	case "leaf":
		return leafFromStatement(stmt, parent)
	case "leaf-list":
		return leafListFromStatement(stmt, parent)
	case "list":
		return listFromStatement(stmt, parent)
	case "notification":
		return notificationFromStatement(stmt, parent)
	case "output":
		return outputFromStatement(stmt, parent)
	case "rpc":
		return rpcFromStatement(stmt, parent)
	case "typedef":
		return typedefFromStatementWithParent(stmt, parent)
	case "uses":
		return usesFromStatement(stmt, parent)
	default:
		return nil
	}
}

// PrintNode writes a human-readable AST node tree to w.
func PrintNode(w io.Writer, n Node) {
	if n == nil {
		return
	}
	value := reflect.ValueOf(n)
	if value.Kind() != reflect.Pointer || value.IsNil() || value.Elem().Kind() != reflect.Struct {
		return
	}
	value = value.Elem()
	typ := value.Type()
	_, _ = fmt.Fprintf(w, "%s [%s]\n", n.NName(), n.Kind())
Loop:
	for i := 0; i < typ.NumField(); i++ {
		fieldType := typ.Field(i)
		yangTag := fieldType.Tag.Get("yang")
		if yangTag == "" {
			continue
		}
		tagParts := strings.Split(yangTag, ",")
		for _, tagPart := range tagParts[1:] {
			if tagPart == "nomerge" {
				continue Loop
			}
		}
		if tagParts[0] == "" || tagParts[0][0] >= 'A' && tagParts[0][0] <= 'Z' {
			continue
		}

		field := value.Field(i)
		if !field.IsValid() || reflectValueIsNil(field) || !field.CanInterface() {
			continue
		}
		switch fieldType.Type.Kind() {
		case reflect.Pointer:
			child, ok := field.Interface().(Node)
			if !ok {
				continue
			}
			if name, ok := scalarValueName(child); ok {
				_, _ = fmt.Fprintf(w, "%s = %s\n", fieldType.Name, name)
			} else {
				PrintNode(indent.NewWriter(w, "    "), child)
			}
		case reflect.Slice:
			for j := 0; j < field.Len(); j++ {
				child, ok := field.Index(j).Interface().(Node)
				if !ok {
					continue
				}
				if name, ok := scalarValueName(child); ok {
					_, _ = fmt.Fprintf(w, "%s[%d] = %s\n", fieldType.Name, j, name)
				} else {
					PrintNode(indent.NewWriter(w, "    "), child)
				}
			}
		}
	}
}

func reflectValueIsNil(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func scalarValueName(node Node) (string, bool) {
	switch value := node.(type) {
	case *Value:
		if value == nil {
			return "", false
		}
		return value.Name, true
	case *upstream.Value:
		if value == nil {
			return "", false
		}
		return value.Name, true
	default:
		return "", false
	}
}

// CamelCase returns the goyang-compatible exported identifier spelling.
func CamelCase(s string) string {
	return upstream.CamelCase(s)
}
