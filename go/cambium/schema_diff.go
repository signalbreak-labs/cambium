// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

import (
	"fmt"
	"strconv"
	"strings"
)

// SchemaDiffVersion is the stable version tag for the public schema diff
// projection.
const SchemaDiffVersion = "cambium.schema-diff.v1"

// SchemaDiffKind classifies one generic schema change.
type SchemaDiffKind string

const (
	// SchemaDiffNodeAdded means a schema node exists only in the new schema.
	SchemaDiffNodeAdded SchemaDiffKind = "node_added"
	// SchemaDiffNodeRemoved means a schema node exists only in the old schema.
	SchemaDiffNodeRemoved SchemaDiffKind = "node_removed"
	// SchemaDiffNodeKindChanged means a matched schema node changed YANG kind.
	SchemaDiffNodeKindChanged SchemaDiffKind = "node_kind_changed"
	// SchemaDiffTypeChanged means a leaf or leaf-list type changed.
	SchemaDiffTypeChanged SchemaDiffKind = "type_changed"
	// SchemaDiffKeyChanged means a list's key statement changed.
	SchemaDiffKeyChanged SchemaDiffKind = "key_changed"
	// SchemaDiffDefaultChanged means effective defaults changed.
	SchemaDiffDefaultChanged SchemaDiffKind = "default_changed"
	// SchemaDiffConfigChanged means effective config/read-only state changed.
	SchemaDiffConfigChanged SchemaDiffKind = "config_changed"
	// SchemaDiffConstraintChanged means validation constraints changed.
	SchemaDiffConstraintChanged SchemaDiffKind = "constraint_changed"
	// SchemaDiffAugmentChanged means augment provenance for a matched node changed.
	SchemaDiffAugmentChanged SchemaDiffKind = "augment_changed"
	// SchemaDiffDeviationChanged means deviation provenance/effects changed.
	SchemaDiffDeviationChanged SchemaDiffKind = "deviation_changed"
)

// SchemaDiff is a versioned, renderer-neutral comparison of two loaded schemas.
type SchemaDiff struct {
	Version string
	Changes []SchemaDiffChange
}

// IsEmpty reports whether the diff contains no changes.
func (d SchemaDiff) IsEmpty() bool { return len(d.Changes) == 0 }

// ByKind returns changes of kind in their original deterministic diff order.
func (d SchemaDiff) ByKind(kind SchemaDiffKind) []SchemaDiffChange {
	var out []SchemaDiffChange
	for _, change := range d.Changes {
		if change.Kind == kind {
			out = append(out, change)
		}
	}
	return out
}

// SchemaDiffChange is one changed schema fact. Path is the local module-relative
// schema path; QualifiedPath is the defining-module-qualified path; and
// NamespaceQualifiedPath is the namespace-expanded path where a node reference
// is available. OldNode/NewNode are set for changed and surviving nodes and left
// zero for the missing side of additions/removals.
type SchemaDiffChange struct {
	Kind                   SchemaDiffKind
	Path                   string
	QualifiedPath          string
	NamespaceQualifiedPath string
	OldNode                SchemaNodeRef
	NewNode                SchemaNodeRef
	OldValue               string
	NewValue               string
}

type schemaDiffEntry struct {
	key string
	ref SchemaNodeRef
}

// DiffModules compares two loaded modules using Cambium's native ordered schema
// model. It walks structural children in effective schema declaration order and
// never uses map iteration as an ordering contract.
func DiffModules(oldModule, newModule Module) (SchemaDiff, error) {
	if oldModule.mod == nil {
		return SchemaDiff{}, wrap("schema diff", fmt.Errorf("old module is nil"))
	}
	if newModule.mod == nil {
		return SchemaDiff{}, wrap("schema diff", fmt.Errorf("new module is nil"))
	}
	if oldModule.mod.ctx != nil {
		if err := oldModule.mod.ctx.rebuildIfDirty(); err != nil {
			return SchemaDiff{}, wrap("schema diff", err)
		}
	}
	if newModule.mod.ctx != nil {
		if err := newModule.mod.ctx.rebuildIfDirty(); err != nil {
			return SchemaDiff{}, wrap("schema diff", err)
		}
	}

	diff := SchemaDiff{Version: SchemaDiffVersion}
	oldRefs := collectSchemaDiffRefs(oldModule)
	newRefs := collectSchemaDiffRefs(newModule)
	duplicateLocals := duplicateSchemaDiffLocalPaths(oldRefs, newRefs)
	oldEntries := schemaDiffEntries(oldRefs, duplicateLocals)
	newEntries := schemaDiffEntries(newRefs, duplicateLocals)
	oldByPath := schemaDiffEntryMap(oldEntries)
	newByPath := schemaDiffEntryMap(newEntries)

	for _, oldEntry := range oldEntries {
		newEntry, ok := newByPath[oldEntry.key]
		if !ok {
			continue
		}
		diff.Changes = append(diff.Changes, compareSchemaDiffNode(oldEntry.ref, newEntry.ref)...)
	}
	for _, newEntry := range newEntries {
		if _, ok := oldByPath[newEntry.key]; !ok {
			diff.Changes = append(diff.Changes, newSchemaDiffChange(SchemaDiffNodeAdded, SchemaNodeRef{}, newEntry.ref, "", "present"))
		}
	}
	for _, oldEntry := range oldEntries {
		if _, ok := newByPath[oldEntry.key]; !ok {
			diff.Changes = append(diff.Changes, newSchemaDiffChange(SchemaDiffNodeRemoved, oldEntry.ref, SchemaNodeRef{}, "present", ""))
		}
	}
	return diff, nil
}

// DiffContexts compares loaded modules from two contexts. Modules are matched by
// module name plus revision. Module additions/removals are reported as node
// additions/removals with Path set to "/<module>" or "/<module>@<revision>".
func DiffContexts(oldCtx, newCtx *Context) (SchemaDiff, error) {
	if oldCtx == nil {
		return SchemaDiff{}, wrap("schema diff", fmt.Errorf("old context is nil"))
	}
	if newCtx == nil {
		return SchemaDiff{}, wrap("schema diff", fmt.Errorf("new context is nil"))
	}
	if err := oldCtx.rebuildIfDirty(); err != nil {
		return SchemaDiff{}, wrap("schema diff", err)
	}
	if err := newCtx.rebuildIfDirty(); err != nil {
		return SchemaDiff{}, wrap("schema diff", err)
	}

	diff := SchemaDiff{Version: SchemaDiffVersion}
	oldModules := oldCtx.SchemaIR().Modules
	newModules := newCtx.SchemaIR().Modules
	oldByKey := schemaDiffModuleMap(oldModules)
	newByKey := schemaDiffModuleMap(newModules)

	for _, oldModule := range oldModules {
		newModule, ok := newByKey[schemaDiffModuleKey(oldModule)]
		if !ok {
			diff.Changes = append(diff.Changes, schemaDiffModuleChange(SchemaDiffNodeRemoved, oldModule, "present", ""))
			continue
		}
		moduleDiff, err := DiffModules(oldModule.Module, newModule.Module)
		if err != nil {
			return SchemaDiff{}, err
		}
		diff.Changes = append(diff.Changes, moduleDiff.Changes...)
	}
	for _, newModule := range newModules {
		if _, ok := oldByKey[schemaDiffModuleKey(newModule)]; !ok {
			diff.Changes = append(diff.Changes, schemaDiffModuleChange(SchemaDiffNodeAdded, newModule, "", "present"))
		}
	}
	return diff, nil
}

func collectSchemaDiffRefs(mod Module) []SchemaNodeRef {
	var out []SchemaNodeRef
	var walk func(SchemaNodeRef)
	walk = func(ref SchemaNodeRef) {
		if schemaDiffPath(ref) != "" {
			out = append(out, ref)
		}
		for child := range ref.Children().Iter() {
			walk(child)
		}
	}
	for child := range mod.Children().Iter() {
		walk(child)
	}
	return out
}

func duplicateSchemaDiffLocalPaths(oldRefs, newRefs []SchemaNodeRef) map[string]bool {
	out := make(map[string]bool)
	for _, refs := range [][]SchemaNodeRef{oldRefs, newRefs} {
		counts := make(map[string]int)
		for _, ref := range refs {
			local := schemaDiffPath(ref)
			if local == "" {
				continue
			}
			counts[local]++
		}
		for local, count := range counts {
			if count > 1 {
				out[local] = true
			}
		}
	}
	return out
}

func schemaDiffEntries(refs []SchemaNodeRef, duplicateLocals map[string]bool) []schemaDiffEntry {
	entries := make([]schemaDiffEntry, 0, len(refs))
	for _, ref := range refs {
		key := schemaDiffPath(ref)
		if duplicateLocals[key] {
			key = ref.NamespaceQualifiedPath()
			if key == "" {
				key = ref.QualifiedPath()
			}
		}
		if key != "" {
			entries = append(entries, schemaDiffEntry{key: key, ref: ref})
		}
	}
	return entries
}

func schemaDiffEntryMap(entries []schemaDiffEntry) map[string]schemaDiffEntry {
	out := make(map[string]schemaDiffEntry, len(entries))
	for _, entry := range entries {
		out[entry.key] = entry
	}
	return out
}

func compareSchemaDiffNode(oldNode, newNode SchemaNodeRef) []SchemaDiffChange {
	var changes []SchemaDiffChange
	if oldValue, newValue := schemaNodeKindName(oldNode.Kind()), schemaNodeKindName(newNode.Kind()); oldValue != newValue {
		changes = append(changes, newSchemaDiffChange(SchemaDiffNodeKindChanged, oldNode, newNode, oldValue, newValue))
	}
	if oldValue, newValue := typeDiffFingerprint(oldNode), typeDiffFingerprint(newNode); oldValue != newValue {
		changes = append(changes, newSchemaDiffChange(SchemaDiffTypeChanged, oldNode, newNode, oldValue, newValue))
	}
	if oldValue, newValue := strings.Join(oldNode.KeyNames(), " "), strings.Join(newNode.KeyNames(), " "); oldValue != newValue {
		changes = append(changes, newSchemaDiffChange(SchemaDiffKeyChanged, oldNode, newNode, oldValue, newValue))
	}
	if oldValue, newValue := defaultsDiffFingerprint(oldNode), defaultsDiffFingerprint(newNode); oldValue != newValue {
		changes = append(changes, newSchemaDiffChange(SchemaDiffDefaultChanged, oldNode, newNode, oldValue, newValue))
	}
	if oldValue, newValue := configDiffFingerprint(oldNode), configDiffFingerprint(newNode); oldValue != newValue {
		changes = append(changes, newSchemaDiffChange(SchemaDiffConfigChanged, oldNode, newNode, oldValue, newValue))
	}
	if oldValue, newValue := constraintsDiffFingerprint(oldNode), constraintsDiffFingerprint(newNode); oldValue != newValue {
		changes = append(changes, newSchemaDiffChange(SchemaDiffConstraintChanged, oldNode, newNode, oldValue, newValue))
	}
	if oldValue, newValue := augmentDiffFingerprint(oldNode), augmentDiffFingerprint(newNode); oldValue != newValue {
		changes = append(changes, newSchemaDiffChange(SchemaDiffAugmentChanged, oldNode, newNode, oldValue, newValue))
	}
	if oldValue, newValue := deviationsDiffFingerprint(oldNode), deviationsDiffFingerprint(newNode); oldValue != newValue {
		changes = append(changes, newSchemaDiffChange(SchemaDiffDeviationChanged, oldNode, newNode, oldValue, newValue))
	}
	return changes
}

func newSchemaDiffChange(kind SchemaDiffKind, oldNode, newNode SchemaNodeRef, oldValue, newValue string) SchemaDiffChange {
	path := schemaDiffPath(newNode)
	if path == "" {
		path = schemaDiffPath(oldNode)
	}
	qualifiedPath := newNode.QualifiedPath()
	if qualifiedPath == "" {
		qualifiedPath = oldNode.QualifiedPath()
	}
	namespaceQualifiedPath := newNode.NamespaceQualifiedPath()
	if namespaceQualifiedPath == "" {
		namespaceQualifiedPath = oldNode.NamespaceQualifiedPath()
	}
	return SchemaDiffChange{
		Kind:                   kind,
		Path:                   path,
		QualifiedPath:          qualifiedPath,
		NamespaceQualifiedPath: namespaceQualifiedPath,
		OldNode:                oldNode,
		NewNode:                newNode,
		OldValue:               oldValue,
		NewValue:               newValue,
	}
}

func schemaDiffPath(ref SchemaNodeRef) string {
	path := ref.LocalPath()
	if path != "" {
		return path
	}
	path = ref.QualifiedPath()
	if path != "" {
		return path
	}
	return ref.Path()
}

func typeDiffFingerprint(ref SchemaNodeRef) string {
	info, ok := ref.LeafType()
	if !ok {
		return ""
	}
	return typeInfoDiffFingerprint(info)
}

func typeInfoDiffFingerprint(info TypeInfo) string {
	parts := []string{"base=" + info.base.String()}
	if info.typedefName != nil {
		parts = append(parts, "typedef="+*info.typedefName)
	}
	if len(info.typedefChain) != 0 {
		parts = append(parts, "typedef-chain="+strings.Join(info.typedefChain, " > "))
	}
	parts = append(parts, "resolved="+resolvedTypeDiffFingerprint(info.resolved))
	return strings.Join(parts, ";")
}

func resolvedTypeDiffFingerprint(resolved ResolvedType) string {
	switch r := resolved.(type) {
	case nil:
		return "unknown"
	case ResolvedInt:
		return "int(kind=" + strconv.Itoa(int(r.Kind)) + ",range=" + rangeBoundsDiffFingerprint(r.Range) + ")"
	case ResolvedDecimal64:
		return "decimal64(fraction-digits=" + strconv.Itoa(int(r.FractionDigits().Value())) + ",range=" + rangeBoundsDiffFingerprint(r.Range) + ")"
	case ResolvedBoolean:
		return "boolean"
	case ResolvedEmpty:
		return "empty"
	case ResolvedBinary:
		return "binary(length=" + rangeBoundsDiffFingerprint(r.Length) + ")"
	case ResolvedString:
		return "string(length=" + rangeBoundsDiffFingerprint(r.Length) + ",patterns=" + patternsDiffFingerprint(r.Patterns) + ")"
	case ResolvedEnumeration:
		return "enumeration(" + enumValuesDiffFingerprint(r.Values()) + ")"
	case ResolvedBits:
		return "bits(" + enumValuesDiffFingerprint(r.Values()) + ")"
	case ResolvedIdentityRef:
		return "identityref(" + identitiesDiffFingerprint(r.Bases()) + ")"
	case ResolvedInstanceIdentifier:
		return "instance-identifier(require-instance=" + strconv.FormatBool(r.RequireInstance) + ")"
	case ResolvedLeafRef:
		path, _ := r.Path()
		target, ok := r.Target()
		targetPath := ""
		if ok {
			targetPath = target.QualifiedPath()
		}
		return "leafref(path=" + path + ",target=" + targetPath + ",require-instance=" + strconv.FormatBool(r.RequireInstance()) + ")"
	case ResolvedUnion:
		members := r.Members()
		parts := make([]string, len(members))
		for i, member := range members {
			parts[i] = typeInfoDiffFingerprint(member)
		}
		return "union(" + strings.Join(parts, "|") + ")"
	case ResolvedUnknown:
		return "unknown"
	default:
		return fmt.Sprintf("%T", resolved)
	}
}

func rangeBoundsDiffFingerprint(bounds []RangeBound) string {
	if len(bounds) == 0 {
		return ""
	}
	parts := make([]string, len(bounds))
	for i, bound := range bounds {
		parts[i] = bound.Min() + ".." + bound.Max()
	}
	return strings.Join(parts, "|")
}

func patternsDiffFingerprint(patterns []Pattern) string {
	if len(patterns) == 0 {
		return ""
	}
	parts := make([]string, len(patterns))
	for i, pattern := range patterns {
		parts[i] = pattern.Regex() + ":inverted=" + strconv.FormatBool(pattern.IsInverted())
	}
	return strings.Join(parts, "|")
}

func enumValuesDiffFingerprint(values []EnumValue) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = value.Name() + "=" + strconv.FormatInt(value.Value(), 10) + ":status=" + strconv.Itoa(int(value.Status())) + ":if-feature=" + strings.Join(value.IfFeatures(), ",")
	}
	return strings.Join(parts, "|")
}

func identitiesDiffFingerprint(identities []Identity) string {
	parts := make([]string, len(identities))
	for i, identity := range identities {
		parts[i] = identity.Module().Name() + ":" + identity.Name()
	}
	return strings.Join(parts, "|")
}

func defaultsDiffFingerprint(ref SchemaNodeRef) string {
	defaults := ref.DefaultEntries()
	parts := make([]string, len(defaults))
	for i, def := range defaults {
		parts[i] = def.Value() + "@" + def.SourceModule().Name()
	}
	return strings.Join(parts, "|")
}

func configDiffFingerprint(ref SchemaNodeRef) string {
	switch ref.Config() {
	case ConfigRo:
		return "config=false"
	default:
		return "config=true"
	}
}

func constraintsDiffFingerprint(ref SchemaNodeRef) string {
	parts := []string{
		"mandatory=" + strconv.FormatBool(ref.IsMandatory()),
		"presence=" + strconv.FormatBool(ref.IsPresenceContainer()),
		"ordered-by=" + orderedByDiffName(ref.OrderedBy()),
		"if-feature=" + strings.Join(ref.IfFeatures(), ","),
	}
	if minElements, ok := ref.MinElements(); ok {
		parts = append(parts, "min-elements="+strconv.FormatUint(uint64(minElements), 10))
	}
	if maxElements, ok := ref.MaxElements(); ok {
		parts = append(parts, "max-elements="+strconv.FormatUint(uint64(maxElements), 10))
	}
	for _, must := range ref.Musts() {
		parts = append(parts, "must="+must.Expression()+"@"+must.SourceModule().Name())
	}
	for _, when := range ref.Whens() {
		var excluded []string
		for _, root := range when.ExcludedSubtreeRoots() {
			excluded = append(excluded, schemaDiffPath(root))
		}
		parts = append(parts, "when="+when.Expression()+"@"+when.SourceModule().Name()+":depth="+strconv.Itoa(when.ContextAncestorDepth())+":excluded="+strings.Join(excluded, ","))
	}
	for _, unique := range ref.UniqueConstraints() {
		var leafs []string
		for _, leaf := range unique.Leafs() {
			leafs = append(leafs, schemaDiffPath(leaf))
		}
		parts = append(parts, "unique="+strings.Join(leafs, ","))
	}
	return strings.Join(parts, ";")
}

func augmentDiffFingerprint(ref SchemaNodeRef) string {
	prov := schemaProvenance(ref)
	return prov.AugmentingModule
}

func deviationsDiffFingerprint(ref SchemaNodeRef) string {
	deviations := ref.DeviationProvenance()
	parts := make([]string, len(deviations))
	for i, deviation := range deviations {
		parts[i] = strings.Join([]string{
			deviation.TargetPath(),
			deviation.SourceModule(),
			deviation.Type(),
			deviation.Property(),
			deviation.NewValue(),
			strings.Join(deviation.IfFeatures(), ","),
		}, ":")
	}
	return strings.Join(parts, "|")
}

func schemaDiffModuleMap(modules []SchemaIRModule) map[string]SchemaIRModule {
	out := make(map[string]SchemaIRModule, len(modules))
	for _, module := range modules {
		out[schemaDiffModuleKey(module)] = module
	}
	return out
}

func schemaDiffModuleKey(module SchemaIRModule) string {
	if module.Revision == "" {
		return module.Name
	}
	return module.Name + "@" + module.Revision
}

func schemaDiffModuleChange(kind SchemaDiffKind, module SchemaIRModule, oldValue, newValue string) SchemaDiffChange {
	path := "/" + schemaDiffModuleKey(module)
	return SchemaDiffChange{
		Kind:                   kind,
		Path:                   path,
		QualifiedPath:          path,
		NamespaceQualifiedPath: path,
		OldValue:               oldValue,
		NewValue:               newValue,
	}
}

func schemaNodeKindName(kind SchemaNodeKind) string {
	switch kind {
	case SchemaNodeKindModule:
		return "module"
	case SchemaNodeKindContainer:
		return "container"
	case SchemaNodeKindLeaf:
		return "leaf"
	case SchemaNodeKindLeafList:
		return "leaf-list"
	case SchemaNodeKindList:
		return "list"
	case SchemaNodeKindChoice:
		return "choice"
	case SchemaNodeKindCase:
		return "case"
	case SchemaNodeKindAnyData:
		return "anydata"
	case SchemaNodeKindAnyXML:
		return "anyxml"
	case SchemaNodeKindRPC:
		return "rpc"
	case SchemaNodeKindAction:
		return "action"
	case SchemaNodeKindInput:
		return "input"
	case SchemaNodeKindOutput:
		return "output"
	case SchemaNodeKindNotification:
		return "notification"
	default:
		return "unknown"
	}
}

func orderedByDiffName(order OrderedBy) string {
	switch order {
	case OrderedByUser:
		return "user"
	default:
		return "system"
	}
}
