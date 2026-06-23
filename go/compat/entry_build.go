// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat

import (
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// UsesStmt associates a uses statement with the grouping entry it references.
type UsesStmt struct {
	Uses     *Uses
	Grouping *Entry
}

type deviationPresence struct {
	hasMinElements bool
	hasMaxElements bool
}

// DeviatedEntry stores a wrapped Entry that corresponds to a deviation.
type DeviatedEntry struct {
	Type         deviationType
	DeviatedPath string
	*Entry
}

// ToEntry projects a goyang-style AST node into an Entry tree.
func ToEntry(n Node) *Entry {
	switch value := n.(type) {
	case nil:
		return errorEntry(fmt.Errorf("ToEntry called on nil AST node"))
	case *Module:
		return entryFromCompatModule(value)
	case *ASTModule:
		return entryFromASTModule(value)
	case *Uses:
		return entryFromUsesNode(value)
	case *Grouping:
		return entryFromGroupingNode(value)
	case *Statement:
		return entryFromStatement(value, nil)
	default:
		return entryFromASTNode(value)
	}
}

// ToEntryInContext projects found by first projecting root, then returning the
// matching entry inside that root projection. It is a migration helper for
// goyang-shaped callers that find a non-root AST node and then need Entry.Path
// to retain module/root context.
func ToEntryInContext(root, found Node) *Entry {
	if found == nil {
		return ToEntry(nil)
	}
	if root == nil {
		return ToEntry(found)
	}
	rootEntry := ToEntry(root)
	if rootEntry == nil {
		return errorEntry(fmt.Errorf("ToEntryInContext failed to project root"))
	}
	if root == found {
		return rootEntry
	}
	if path, ok := nodePathBetween(root, found); ok {
		if entry := entryAtLocalPath(rootEntry, path); entry != nil {
			return entry
		}
	}
	if stmt := found.Statement(); stmt != nil {
		if entry := findEntryBySourceStatement(rootEntry, stmt, found.NName()); entry != nil {
			return entry
		}
	}
	return errorEntry(fmt.Errorf("ToEntryInContext could not locate %s %q under root %q", found.Kind(), found.NName(), root.NName()))
}

func nodePathBetween(root, found Node) ([]string, bool) {
	var reverse []string
	for cur := found; cur != nil; cur = cur.ParentNode() {
		if cur == root {
			path := make([]string, len(reverse))
			for i := range reverse {
				path[len(reverse)-1-i] = reverse[i]
			}
			return path, true
		}
		name := cur.NName()
		if name == "" {
			return nil, false
		}
		reverse = append(reverse, name)
	}
	return nil, false
}

func entryAtLocalPath(root *Entry, path []string) *Entry {
	cur := root
	for _, name := range path {
		if cur == nil {
			return nil
		}
		cur = cur.Lookup(name)
	}
	return cur
}

func findEntryBySourceStatement(root *Entry, stmt *Statement, name string) *Entry {
	if root == nil || stmt == nil {
		return nil
	}
	if root.Node != nil && root.Node.Statement() == stmt && (name == "" || root.Name == name) {
		return root
	}
	for _, child := range root.Children() {
		if found := findEntryBySourceStatement(child, stmt, name); found != nil {
			return found
		}
	}
	return nil
}

func entryFromCambiumModule(module cambium.Module) *Entry {
	root := &Entry{
		Name:       module.Name(),
		Kind:       DirectoryEntry,
		Config:     TSTrue,
		Prefix:     valueOrNil(module.Prefix()),
		Dir:        make(map[string]*Entry),
		Node:       entryNodeForModule(module),
		Exts:       statementsForExtensions(module.Extensions()),
		Extra:      make(map[string][]interface{}),
		Annotation: make(map[string]interface{}),
		module:     module,
	}
	if desc, ok := module.Description(); ok {
		root.Description = desc
	}
	seenIdentities := make(map[string]*Identity)
	for identity := range module.Identities() {
		if projected := identityFromCambium(identity, seenIdentities); projected != nil {
			root.Identities = append(root.Identities, projected)
		}
	}
	for child := range module.Children().Iter() {
		root.add(projectNode(child, root))
	}
	return root
}

func entryFromCompatModule(module *Module) *Entry {
	if module == nil {
		return errorEntry(fmt.Errorf("ToEntry called on nil module"))
	}
	if entry, ok := moduleEntry(module); ok {
		return entry
	}
	if schema, ok := moduleSchema(module); ok && schema.Name() != "" {
		entry := entryFromCambiumModule(schema)
		setModuleEntry(module, entry)
		return entry
	}
	if module.Source != nil {
		if entry := entryFromCompatModuleSource(module); entry != nil {
			setModuleEntry(module, entry)
			return entry
		}
	}
	entry := entryFromASTNode(module)
	if entry != nil {
		entry.Node = module
		entry.modules = module.Modules
	}
	return entry
}

func entryFromCompatModuleSource(module *Module) *Entry {
	if module == nil || module.Source == nil {
		return nil
	}
	entry := entryFromStatementWithScopes(module.Source, nil, nil, nil, module)
	if entry != nil {
		entry.Node = module
		entry.modules = module.Modules
		if module.Modules != nil && module.Modules.ParseOptions.StoreUses {
			attachStoredUsesFromNode(entry, module)
		}
	}
	return entry
}

func entryFromASTModule(module *ASTModule) *Entry {
	if module == nil {
		return errorEntry(fmt.Errorf("ToEntry called on nil AST module"))
	}
	if module.Source != nil {
		entry := entryFromStatementWithScopes(module.Source, nil, nil, nil, module)
		if entry != nil {
			entry.Node = module
			entry.modules = modulesFromASTModuleSet(module)
			if module.Modules != nil && module.Modules.ParseOptions.StoreUses {
				attachStoredUsesFromNode(entry, module)
			}
		}
		return entry
	}
	return errorEntry(fmt.Errorf("ToEntry AST module %q has no ordered source statement", module.Name))
}

func entryFromASTNode(node Node) *Entry {
	if node == nil {
		return errorEntry(fmt.Errorf("ToEntry called on nil AST node"))
	}
	if stmt := node.Statement(); stmt != nil {
		return entryFromStatementWithScopes(stmt, nil, nil, typedefScopesForNode(node), node)
	}
	return errorEntry(fmt.Errorf("ToEntry AST node %q has no ordered source statement", node.NName()))
}

func entryFromUsesNode(uses *Uses) *Entry {
	if uses == nil {
		return errorEntry(fmt.Errorf("ToEntry called on nil uses node"))
	}
	grouping := groupingFromResolver(uses, uses.Name)
	if grouping.stmt == nil {
		return errorEntry(fmt.Errorf("unknown group: %s", uses.Name))
	}
	entry := entryForGrouping(grouping, uses)
	metadata := metadataForUsesStatement(uses.Statement())
	metadata.apply(entry)
	entry.addGroupingChildren(grouping, nil, typedefScopesForNode(uses), map[*Statement]bool{}, uses, statementMetadata{})
	return entry
}

func attachStoredUsesFromNode(entry *Entry, node Node) {
	if entry == nil || node == nil {
		return
	}
	value := reflect.ValueOf(node)
	if value.Kind() != reflect.Pointer || value.IsNil() || value.Elem().Kind() != reflect.Struct {
		return
	}
	value = value.Elem()
	typ := value.Type()
	for i := 0; i < typ.NumField(); i++ {
		fieldType := typ.Field(i)
		yangTag := fieldType.Tag.Get("yang")
		if yangTag == "" {
			continue
		}
		tagParts := strings.Split(yangTag, ",")
		if hasYangTagOption(tagParts[1:], "nomerge") {
			continue
		}
		field := value.Field(i)
		if !field.IsValid() || field.IsNil() {
			continue
		}
		if tagParts[0] == "uses" {
			for _, uses := range usesNodesFromField(field) {
				grouping := entryFromUsesNode(uses)
				entry.Uses = append(entry.Uses, &UsesStmt{Uses: uses, Grouping: grouping.shallowDup()})
			}
			continue
		}
		for _, child := range childNodesFromField(field) {
			if childEntry := childEntryForASTNode(entry, tagParts[0], child); childEntry != nil {
				attachStoredUsesFromNode(childEntry, child)
			}
		}
	}
}

func usesNodesFromField(field reflect.Value) []*Uses {
	switch field.Kind() {
	case reflect.Pointer:
		if uses, ok := field.Interface().(*Uses); ok && uses != nil {
			return []*Uses{uses}
		}
	case reflect.Slice:
		out := make([]*Uses, 0, field.Len())
		for i := 0; i < field.Len(); i++ {
			if uses, ok := field.Index(i).Interface().(*Uses); ok && uses != nil {
				out = append(out, uses)
			}
		}
		return out
	}
	return nil
}

func entryFromGroupingNode(grouping *Grouping) *Entry {
	if grouping == nil {
		return errorEntry(fmt.Errorf("ToEntry called on nil grouping node"))
	}
	stmt := grouping.Statement()
	if stmt == nil {
		return errorEntry(fmt.Errorf("ToEntry grouping %q has no ordered source statement", grouping.Name))
	}
	ref := statementGrouping{stmt: stmt, node: grouping}
	entry := entryForGrouping(ref, grouping)
	entry.addGroupingChildren(ref, nil, typedefScopesForNode(grouping), map[*Statement]bool{}, grouping, statementMetadata{})
	return entry
}

func entryForGrouping(grouping statementGrouping, fallback Node) *Entry {
	node := grouping.node
	if node == nil {
		node = fallback
	}
	name := ""
	if grouping.stmt != nil {
		name = grouping.stmt.Argument
	}
	entry := &Entry{
		Name:       name,
		Kind:       DirectoryEntry,
		Config:     TSUnset,
		Dir:        make(map[string]*Entry),
		Node:       node,
		Exts:       extensionStatements(grouping.stmt),
		Extra:      make(map[string][]interface{}),
		Annotation: make(map[string]interface{}),
	}
	if root := RootNode(fallback); root != nil {
		entry.Prefix = valueOrNil(root.GetPrefix())
	}
	return entry
}

type statementGrouping struct {
	stmt *Statement
	node Node
}

type statementGroupingScopes []map[string]statementGrouping

func entryFromStatement(stmt *Statement, parent *Entry) *Entry {
	return entryFromStatementWithScopes(stmt, parent, nil, nil, nil)
}

func entryFromStatementWithScopes(stmt *Statement, parent *Entry, scopes statementGroupingScopes, typedefScopes statementTypedefScopes, resolver Node) *Entry {
	if stmt == nil {
		return errorEntry(fmt.Errorf("ToEntry called on nil statement"))
	}
	entry := entryForSchemaStatement(stmt, parent, typedefScopes, resolver)
	if entry == nil {
		return nil
	}
	scopes = append(scopes, groupingScopeForStatement(stmt))
	typedefScopes = append(typedefScopes, typedefScopeForStatement(stmt))
	for _, child := range stmt.SubStatements() {
		if child.Keyword == "uses" {
			grouping := scopes.findGrouping(child.Argument)
			if grouping.stmt == nil {
				grouping = groupingFromResolver(resolver, child.Argument)
			}
			if grouping.stmt == nil {
				entry.Errors = append(entry.Errors, fmt.Errorf("unknown group: %s", child.Argument))
				continue
			}
			entry.addGroupingChildren(grouping, scopes, typedefScopes, map[*Statement]bool{}, resolver, metadataForUsesStatement(child))
			continue
		}
		if child.Keyword == "augment" {
			augmentEntry := entryFromStatementWithScopes(child, entry, scopes, typedefScopes, resolver)
			if augmentEntry != nil {
				entry.Augments = append(entry.Augments, augmentEntry)
			}
			continue
		}
		if child.Keyword == "deviation" {
			deviationEntry := entryFromStatementWithScopes(child, entry, scopes, typedefScopes, resolver)
			if deviationEntry != nil {
				entry.Deviations = append(entry.Deviations, &DeviatedEntry{
					Entry:        deviationEntry,
					DeviatedPath: child.Argument,
				})
			}
			continue
		}
		if child.Keyword == "deviate" {
			deviateEntry := entryFromStatementWithScopes(child, entry, scopes, typedefScopes, resolver)
			if deviateEntry != nil {
				devType := toDeviation[child.Argument]
				if entry.Deviate == nil {
					entry.Deviate = make(map[deviationType][]*Entry)
				}
				entry.Deviate[devType] = append(entry.Deviate[devType], deviateEntry)
			}
			continue
		}
		childEntry := entryFromStatementWithScopes(child, entry, scopes, typedefScopes, resolver)
		if childEntry == nil {
			continue
		}
		if child.Keyword == "rpc" && childEntry.RPC == nil {
			childEntry.RPC = &RPCEntry{}
		}
		entry.add(childEntry)
		switch childEntry.Kind {
		case InputEntry:
			if entry.RPC == nil {
				entry.RPC = &RPCEntry{}
			}
			entry.RPC.Input = childEntry
		case OutputEntry:
			if entry.RPC == nil {
				entry.RPC = &RPCEntry{}
			}
			entry.RPC.Output = childEntry
		}
	}
	return entry
}

func (e *Entry) addGroupingChildren(grouping statementGrouping, scopes statementGroupingScopes, typedefScopes statementTypedefScopes, seen map[*Statement]bool, resolver Node, metadata statementMetadata) {
	if e == nil || grouping.stmt == nil {
		return
	}
	if grouping.node != nil {
		resolver = grouping.node
	}
	if seen[grouping.stmt] {
		e.Errors = append(e.Errors, fmt.Errorf("circular group: %s", grouping.stmt.Argument))
		return
	}
	seen[grouping.stmt] = true
	defer delete(seen, grouping.stmt)

	groupingScopes := make(statementGroupingScopes, 0, len(scopes)+1)
	groupingScopes = append(groupingScopes, scopes...)
	groupingScopes = append(groupingScopes, groupingScopeForStatement(grouping.stmt))
	groupingTypedefScopes := typedefScopes
	if grouping.node != nil {
		groupingTypedefScopes = typedefScopesForNode(grouping.node)
	}
	groupingTypedefScopes = append(groupingTypedefScopes, typedefScopeForStatement(grouping.stmt))
	for _, child := range grouping.stmt.SubStatements() {
		if child.Keyword == "uses" {
			nested := groupingScopes.findGrouping(child.Argument)
			if nested.stmt == nil {
				nested = groupingFromResolver(resolver, child.Argument)
			}
			if nested.stmt == nil {
				e.Errors = append(e.Errors, fmt.Errorf("unknown group: %s", child.Argument))
				continue
			}
			e.addGroupingChildren(nested, groupingScopes, groupingTypedefScopes, seen, resolver, mergeStatementMetadata(metadataForUsesStatement(child), metadata))
			continue
		}
		childEntry := entryFromStatementWithScopes(child, e, groupingScopes, groupingTypedefScopes, resolver)
		if childEntry != nil {
			metadata.apply(childEntry)
			e.add(childEntry)
		}
	}
}

func metadataForUsesStatement(stmt *Statement) statementMetadata {
	metadata := statementMetadata{extra: make(map[string][]interface{})}
	if stmt == nil {
		return metadata
	}
	for _, child := range stmt.SubStatements() {
		switch child.Keyword {
		case "if-feature", "reference", "status", "when":
			metadata.extra[child.Keyword] = append(metadata.extra[child.Keyword], child)
		default:
			if strings.Contains(child.Keyword, ":") {
				metadata.exts = append(metadata.exts, child)
			}
		}
	}
	return metadata
}

func groupingScopeForStatement(stmt *Statement) map[string]statementGrouping {
	scope := map[string]statementGrouping{}
	if stmt == nil {
		return scope
	}
	for _, child := range stmt.SubStatements() {
		if child.Keyword == "grouping" && child.Argument != "" {
			scope[child.Argument] = statementGrouping{stmt: child}
		}
	}
	return scope
}

func (scopes statementGroupingScopes) findGrouping(name string) statementGrouping {
	_, local := splitPrefix(name)
	for i := len(scopes) - 1; i >= 0; i-- {
		if grouping := scopes[i][name]; grouping.stmt != nil {
			return grouping
		}
		if local != name {
			if grouping := scopes[i][local]; grouping.stmt != nil {
				return grouping
			}
		}
	}
	return statementGrouping{}
}

func groupingFromResolver(resolver Node, name string) statementGrouping {
	if resolver == nil {
		return statementGrouping{}
	}
	grouping := FindGrouping(resolver, name, map[string]bool{})
	if grouping == nil || grouping.Statement() == nil {
		return statementGrouping{}
	}
	return statementGrouping{stmt: grouping.Statement(), node: grouping}
}

// Augment applies pending augment entries into their target entries.
func (e *Entry) Augment(addErrors bool) (processed, skipped int) {
	if e == nil {
		return 0, 0
	}
	var unapplied []*Entry
	for _, augment := range e.Augments {
		if augment == nil {
			continue
		}
		target := augment.Find(augment.Name)
		if target == nil {
			if addErrors {
				e.Errors = append(e.Errors, fmt.Errorf("%s: augment %s not found", Source(augment.Node), augment.Name))
			}
			skipped++
			unapplied = append(unapplied, augment)
			continue
		}
		processed++
		target.merge(nil, augment.Namespace(), augment)
		applied := augment.shallowDup()
		target.Augmented = append(target.Augmented, applied)
		target.AugmentedBy = append(target.AugmentedBy, applied)
	}
	e.Augments = unapplied
	return processed, skipped
}

// ApplyDeviate walks the deviations within e and applies them to the projected
// schema, matching goyang's migration behavior for cgo-free compat callers.
func (e *Entry) ApplyDeviate(deviateOpts ...DeviateOpt) []error {
	var errs []error
	appendErr := func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}
	for _, deviation := range e.Deviations {
		if deviation == nil {
			continue
		}
		deviatedNode := e.Find(deviation.DeviatedPath)
		if deviatedNode == nil {
			appendErr(fmt.Errorf("cannot find target node to deviate, %s", deviation.DeviatedPath))
			continue
		}
		for deviationType, deviateEntries := range deviation.Deviate {
			for _, devSpec := range deviateEntries {
				if devSpec == nil {
					continue
				}
				switch deviationType {
				case DeviationAdd, DeviationReplace:
					applyAddOrReplaceDeviation(deviatedNode, devSpec, deviationType, appendErr, Source(e.Node))
				case DeviationNotSupported:
					parent := deviatedNode.Parent
					if parent == nil {
						appendErr(fmt.Errorf("%s: node %s does not have a valid parent, but deviate not-supported references one", Source(e.Node), e.Name))
						continue
					}
					if !hasIgnoreDeviateNotSupported(deviateOpts) {
						parent.delete(deviatedNode.Name)
					}
				case DeviationDelete:
					applyDeleteDeviation(deviatedNode, devSpec, deviation.DeviatedPath, appendErr, Source(e.Node))
				default:
					appendErr(fmt.Errorf("invalid deviation type %s", deviationType))
				}
			}
		}
	}
	return errs
}

func applyAddOrReplaceDeviation(deviatedNode, devSpec *Entry, deviationType deviationType, appendErr func(error), source string) {
	if devSpec.Config != TSUnset {
		deviatedNode.Config = devSpec.Config
	}
	if len(devSpec.Default) > 0 {
		switch deviationType {
		case DeviationAdd:
			switch {
			case deviatedNode.IsLeafList():
				deviatedNode.Default = append(deviatedNode.Default, devSpec.Default...)
			case len(devSpec.Default) > 1:
				appendErr(fmt.Errorf("%s: tried to add more than one default to a non-leaflist entry at deviation", source))
			case len(deviatedNode.Default) != 0:
				appendErr(fmt.Errorf("%s: tried to add a default value to an entry that already has a default value", source))
			case len(devSpec.Default) == 1 && len(deviatedNode.Default) == 0:
				deviatedNode.Default = append([]string{}, devSpec.Default[0])
			}
		case DeviationReplace:
			deviatedNode.Default = append([]string{}, devSpec.Default...)
		}
	}
	if devSpec.Mandatory != TSUnset {
		deviatedNode.Mandatory = devSpec.Mandatory
	}
	if devSpec.deviatePresence.hasMinElements {
		if !deviatedNode.IsList() && !deviatedNode.IsLeafList() {
			appendErr(fmt.Errorf("tried to deviate min-elements on a non-list type %s", deviatedNode.Kind))
		} else if devSpec.ListAttr != nil && deviatedNode.ListAttr != nil {
			deviatedNode.ListAttr.MinElements = devSpec.ListAttr.MinElements
		}
	}
	if devSpec.deviatePresence.hasMaxElements {
		if !deviatedNode.IsList() && !deviatedNode.IsLeafList() {
			appendErr(fmt.Errorf("tried to deviate max-elements on a non-list type %s", deviatedNode.Kind))
		} else if devSpec.ListAttr != nil && deviatedNode.ListAttr != nil {
			deviatedNode.ListAttr.MaxElements = devSpec.ListAttr.MaxElements
		}
	}
	if devSpec.Units != "" {
		deviatedNode.Units = devSpec.Units
	}
	if devSpec.Type != nil {
		deviatedNode.Type = devSpec.Type
	}
}

func applyDeleteDeviation(deviatedNode, devSpec *Entry, deviatedPath string, appendErr func(error), source string) {
	if devSpec.Config != TSUnset {
		deviatedNode.Config = TSUnset
	}
	if len(devSpec.Default) > 0 {
		switch {
		case deviatedNode.IsLeafList():
			appendErr(fmt.Errorf("%s: deviate delete on default statements unsupported for leaf-lists, please use replace instead", source))
		case len(deviatedNode.Default) == 0:
			appendErr(fmt.Errorf("%s: tried to deviate delete a default statement that doesn't exist", source))
		case devSpec.Default[0] != deviatedNode.Default[0]:
			appendErr(fmt.Errorf("%s: tried to deviate delete a default statement with a non-matching keyword", source))
		default:
			deviatedNode.Default = nil
		}
	}
	if devSpec.Mandatory != TSUnset {
		deviatedNode.Mandatory = TSUnset
	}
	if devSpec.deviatePresence.hasMinElements {
		if !deviatedNode.IsList() && !deviatedNode.IsLeafList() {
			appendErr(fmt.Errorf("tried to deviate min-elements on a non-list type %s", deviatedNode.Kind))
		} else if devSpec.ListAttr != nil && deviatedNode.ListAttr != nil {
			if deviatedNode.ListAttr.MinElements != devSpec.ListAttr.MinElements {
				appendErr(fmt.Errorf("min-element value %d differs from deviation's min-element value %d for entry %v", devSpec.ListAttr.MinElements, deviatedNode.ListAttr.MinElements, deviatedPath))
			}
			deviatedNode.ListAttr.MinElements = 0
		}
	}
	if devSpec.deviatePresence.hasMaxElements {
		if !deviatedNode.IsList() && !deviatedNode.IsLeafList() {
			appendErr(fmt.Errorf("tried to deviate max-elements on a non-list type %s", deviatedNode.Kind))
		} else if devSpec.ListAttr != nil && deviatedNode.ListAttr != nil {
			if deviatedNode.ListAttr.MaxElements != devSpec.ListAttr.MaxElements {
				appendErr(fmt.Errorf("max-element value %d differs from deviation's max-element value %d for entry %v", devSpec.ListAttr.MaxElements, deviatedNode.ListAttr.MaxElements, deviatedPath))
			}
			deviatedNode.ListAttr.MaxElements = math.MaxUint64
		}
	}
}

func hasIgnoreDeviateNotSupported(opts []DeviateOpt) bool {
	for _, opt := range opts {
		if deviateOptions, ok := opt.(DeviateOptions); ok {
			return deviateOptions.IgnoreDeviateNotSupported
		}
	}
	return false
}
