// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium

import (
	"fmt"
	"strings"

	"github.com/signalbreak-labs/cambium/go/internal/yangparse"
)

// TypedefName returns the immediate typedef name and whether the type came from
// a named typedef rather than a built-in.
func (t TypeInfo) TypedefName() (string, bool) {
	if t.typedefName == nil {
		return "", false
	}
	return *t.typedefName, true
}

// TypedefChain returns the typedef names traversed to reach the base type, from
// outermost to innermost.
func (t TypeInfo) TypedefChain() []string { return t.typedefChain.names() }

// typedefNames is an immutable list of typedef names, outermost first. Each
// typedef level prepends its name and shares the rest, so resolving a chain of
// n typedefs does not copy the chain at every level.
type typedefNames struct {
	name string
	next *typedefNames
}

func (l *typedefNames) names() []string {
	var out []string
	for ; l != nil; l = l.next {
		out = append(out, l.name)
	}
	return out
}

// TypedefDefinition is a declared top-level or included YANG typedef.
type TypedefDefinition struct {
	module *moduleData
	stmt   *yangparse.Statement
}

func (m *moduleData) addTypedefDefinition(scope, st *yangparse.Statement) error {
	name := st.Argument
	if builtinBase(name) != BaseTypeUnknown {
		return fmt.Errorf("typedef %q at %s collides with built-in type", name, st.Location())
	}
	if err := validateTypedefTypeCardinality(name, st); err != nil {
		return err
	}
	if err := validateTypedefDefaultCardinality(name, st); err != nil {
		return err
	}
	if err := validateDefinitionStatus("typedef", name, st); err != nil {
		return err
	}
	if err := validateDefinitionTextMetadata("typedef", name, st); err != nil {
		return err
	}
	if scope == nil {
		if prev := m.typedefs[name]; prev != nil {
			return duplicateDefinitionError("typedef", name, prev, st)
		}
		m.typedefs[name] = st
		m.typedefDefOrder = append(m.typedefDefOrder, st)
		return nil
	}
	if prev := m.typedefs[name]; prev != nil {
		return definitionCollisionError("typedef", name, "collides with top-level typedef", prev, st)
	}
	for parent := m.statementParents[scope]; parent != nil; parent = m.statementParents[parent] {
		if prev := m.typedefsByScope[parent][name]; prev != nil {
			return definitionCollisionError("typedef", name, "collides with ancestor scoped typedef", prev, st)
		}
	}
	defs := m.typedefsByScope[scope]
	if defs == nil {
		defs = make(map[string]*yangparse.Statement)
		m.typedefsByScope[scope] = defs
	}
	if prev := defs[name]; prev != nil {
		return duplicateDefinitionError("typedef", name, prev, st)
	}
	defs[name] = st
	return nil
}

func validateTypedefTypeCardinality(name string, st *yangparse.Statement) error {
	types := direct(st, "type")
	if len(types) == 0 {
		return fmt.Errorf("typedef %q has no type at %s", name, st.Location())
	}
	if len(types) > 1 {
		return fmt.Errorf("duplicate type in typedef %q at %s", name, types[1].Location())
	}
	return nil
}

func validateTypedefDefaultCardinality(name string, st *yangparse.Statement) error {
	defaults := direct(st, "default")
	if len(defaults) > 1 {
		return fmt.Errorf("typedef %q has multiple default statements at %s", name, defaults[1].Location())
	}
	return nil
}

func (m *moduleData) parseTypes() error {
	return m.parseNodeTypes(m.root)
}

func (m *moduleData) validateTypedefTypes() error {
	for _, td := range m.typedefDefinitionsInOrder() {
		typ := first(td, "type")
		if typ == nil {
			continue
		}
		if _, err := m.parseTypeSeen(typ, make(map[*yangparse.Statement]bool)); err != nil {
			return err
		}
	}
	return nil
}

func (m *moduleData) validateTypedefDefaultValues() error {
	for _, td := range m.typedefDefinitionsInOrder() {
		defaults := direct(td, "default")
		if len(defaults) == 0 {
			continue
		}
		typ := first(td, "type")
		if typ == nil {
			continue
		}
		info, err := m.parseTypeSeen(typ, make(map[*yangparse.Statement]bool))
		if err != nil {
			return err
		}
		node := &schemaNodeData{
			name:       td.Argument,
			kind:       SchemaNodeKindUnknown,
			module:     m,
			stmt:       td,
			typeInfo:   &info,
			typeModule: m,
		}
		def := DefaultValue{value: defaults[0].Argument, sourceModule: m}
		if err := validateDefaultValueForType(node, def); err != nil {
			return err
		}
	}
	return nil
}

func (m *moduleData) typedefDefinitionsInOrder() []*yangparse.Statement {
	var out []*yangparse.Statement
	var walk func(*yangparse.Statement)
	walk = func(st *yangparse.Statement) {
		if st == nil {
			return
		}
		if st.Keyword == "typedef" {
			out = append(out, st)
		}
		for _, child := range st.SubStatements() {
			walk(child)
		}
	}
	for _, st := range m.sourceTopStatements() {
		walk(st)
	}
	return out
}

func (m *moduleData) resolveLeafRefs() {
	var typed []*schemaNodeData
	var walk func(*schemaNodeData)
	walk = func(n *schemaNodeData) {
		if n.typeInfo != nil {
			typed = append(typed, n)
		}
		for _, c := range n.children {
			walk(c)
		}
	}
	walk(m.root)
	for _, n := range typed {
		if n.typeInfo == nil {
			continue
		}
		typeMod := n.typeModule
		if typeMod == nil {
			typeMod = n.module
		}
		n.typeInfo.resolved = m.resolveLeafRefsInResolvedType(n, typeMod, n.typeInfo.resolved)
	}
}

func (m *moduleData) resolveLeafRefsInResolvedType(n *schemaNodeData, fallbackSource *moduleData, resolved ResolvedType) ResolvedType {
	switch r := resolved.(type) {
	case ResolvedLeafRef:
		source := r.sourceModule
		if source == nil {
			source = fallbackSource
		}
		if m.implemented {
			// A module whose nodes an implemented module's leafref path
			// uses is implemented (RFC 7950 §5.6.5).
			source.implementPrefixedModules(r.path, r.sourceStmt)
		}
		resolveLeafRef(n, source, &r)
		m.validateResolvedLeafRef(n, &r)
		if m.implemented && r.target != nil {
			if err := source.validateLeafRefPredicates(n, &r); err != nil {
				n.recordSchemaError(err)
			}
		}
		return r
	case ResolvedUnion:
		for i := range r.members {
			r.members[i].resolved = m.resolveLeafRefsInResolvedType(n, fallbackSource, r.members[i].resolved)
		}
		return r
	default:
		return resolved
	}
}

// implementPrefixedModules implements every module that a prefix in expr
// names, resolved from the source statement from.
func (m *moduleData) implementPrefixedModules(expr string, from *yangparse.Statement) {
	if m == nil || m.ctx == nil {
		return
	}
	for _, prefix := range referencedPrefixes(expr) {
		if target := m.resolveSourceQNameModuleFrom(prefix+":_", from); target != nil {
			m.ctx.markImplemented(target)
		}
	}
}

func (m *moduleData) validateResolvedLeafRef(n *schemaNodeData, lr *ResolvedLeafRef) {
	if lr == nil {
		return
	}
	if m.implemented && lr.target == nil {
		n.recordSchemaError(fmt.Errorf("leafref %q path %q target not found", n.name, lr.path))
	}
	if m.implemented && lr.target != nil && lr.target.node != nil {
		m.ctx.markImplemented(lr.target.node.module)
	}
	if n.representsConfigurationData() && lr.requireInstance && lr.target != nil && lr.target.node != nil && lr.target.node.config == ConfigRo {
		n.recordSchemaError(fmt.Errorf(
			"leafref %q with require-instance true cannot target config false %s %q",
			n.name,
			nodeStatementKeyword(lr.target.node),
			lr.target.node.name,
		))
	}
}

func (m *moduleData) resolveIdentities() {
	for _, id := range m.identities {
		m.resolveIdentity(id)
	}
}

func (m *moduleData) resolveIdentity(id *identityData) {
	if id == nil || id.resolved {
		return
	}
	source := id.module
	if source == nil {
		source = m
	}
	if id.resolving {
		source.recordSchemaError(fmt.Errorf("identity cycle involving %q", id.name))
		return
	}
	id.resolving = true
	defer func() {
		id.resolving = false
	}()
	baseStmts := direct(id.stmt, "base")
	// With several bases, one visited set spans their traversals so an
	// ancestor shared by two bases still receives id once.
	var ancestors map[*identityData]bool
	if len(id.baseNames) > 1 {
		ancestors = make(map[*identityData]bool)
	}
	for i, q := range id.baseNames {
		var baseStmt *yangparse.Statement
		if i < len(baseStmts) {
			baseStmt = baseStmts[i]
		}
		baseMod := source.resolveSourceQNameModuleFrom(q, baseStmt)
		if baseMod == nil {
			source.recordSchemaError(fmt.Errorf("unknown identity base %q for identity %q", q, id.name))
			return
		}
		base := baseMod.identityMap[localName(q)]
		if baseMod == source && base != nil && !source.definitionVisibleFrom(base.stmt, baseStmt) {
			base = nil
		}
		if base == nil {
			source.recordSchemaError(fmt.Errorf("unknown identity base %q for identity %q", q, id.name))
			return
		}
		baseMod.resolveIdentity(base)
		if baseMod.schemaErr != nil {
			return
		}
		id.bases = append(id.bases, base)
		appendDerivedToIdentityAncestors(base, id, ancestors)
	}
	if len(id.baseNames) > 1 && source.yangVersionForStatement(id.stmt) != "1.1" {
		source.recordSchemaError(fmt.Errorf("identity %q with multiple base statements requires yang-version 1.1 at %s", id.name, id.stmt.Location()))
		return
	}
	id.resolved = true
}

// TypedefDefinitions returns the module's typedefs in declaration order.
func (m Module) TypedefDefinitions() []TypedefDefinition {
	if m.mod == nil {
		return nil
	}
	out := make([]TypedefDefinition, 0, len(m.mod.typedefDefOrder))
	for _, st := range m.mod.typedefDefOrder {
		out = append(out, TypedefDefinition{module: m.mod, stmt: st})
	}
	return out
}
func (m *moduleData) parseType(st *yangparse.Statement) (TypeInfo, error) {
	return m.parseTypeSeen(st, make(map[*yangparse.Statement]bool))
}

// resolveTypedefType resolves typ, the type statement of typedef td in m. The
// result does not depend on where td is referenced from (a cycle through td
// fails wherever it starts), so during a rebuild each typedef is resolved once
// and later references get a copy: a chain of n typedefs resolves in O(n)
// rather than once per level per reference.
func (m *moduleData) resolveTypedefType(td, typ *yangparse.Statement, seen map[*yangparse.Statement]bool) (TypeInfo, error) {
	var memo map[typedefKey]TypeInfo
	if m.ctx != nil {
		memo = m.ctx.build.typedefTypes
	}
	key := typedefKey{module: m, stmt: td}
	if info, ok := memo[key]; ok {
		return cloneTypeInfo(info), nil
	}
	info, err := m.parseTypeSeen(typ, seen)
	if err == nil && memo != nil {
		memo[key] = cloneTypeInfo(info)
	}
	return info, err
}

func (m *moduleData) parseTypeSeen(st *yangparse.Statement, seen map[*yangparse.Statement]bool) (TypeInfo, error) {
	name := st.Argument
	if tdMod, td := m.lookupTypedefModuleFrom(name, st); td != nil {
		if seen[td] {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("typedef cycle involving %q at %s", localName(name), st.Location())
		}
		seen[td] = true
		defer delete(seen, td)
		typ, err := singletonChild(td, "type")
		if err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		if typ == nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("typedef %q at %s has no type", td.Argument, td.Location())
		}
		defaults := direct(td, "default")
		if len(defaults) > 1 {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("typedef %q has multiple default statements at %s", td.Argument, defaults[1].Location())
		}
		base, err := tdMod.resolveTypedefType(td, typ, seen)
		if err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		typedefName := localName(name)
		base.typedefName = ptr(typedefName)
		base.typedefChain = &typedefNames{name: typedefName, next: base.typedefChain}
		if err := validateTypeRestrictionPlacement(st, base.base); err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		restricted, err := m.applyTypeRestrictions(base.resolved, st, base.base)
		if err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		base.resolved = restricted
		return base, nil
	}
	if hasPrefix(name) {
		return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("unknown type %q at %s", name, st.Location())
	}
	base := builtinBase(name)
	if base == BaseTypeUnknown {
		return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("unknown type %q at %s", name, st.Location())
	}
	if err := validateTypeRestrictionPlacement(st, base); err != nil {
		return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
	}
	ti := TypeInfo{base: base}
	switch base {
	case BaseTypeString:
		lengths, err := restrictionRanges(st, "length", base, 0)
		if err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		ps, err := patterns(st)
		if err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		ti.resolved = ResolvedString{Length: lengths, Patterns: ps}
	case BaseTypeBoolean:
		ti.resolved = ResolvedBoolean{}
	case BaseTypeInt8, BaseTypeInt16, BaseTypeInt32, BaseTypeInt64, BaseTypeUint8, BaseTypeUint16, BaseTypeUint32, BaseTypeUint64:
		rs, err := restrictionRanges(st, "range", base, 0)
		if err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		ti.resolved = ResolvedInt{Kind: intKind(base), Range: rs}
	case BaseTypeDecimal64:
		fractionDigits := direct(st, "fraction-digits")
		if len(fractionDigits) != 1 {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("decimal64 type at %s must have exactly one fraction-digits statement", st.Location())
		}
		v, ok := parseUint32(fractionDigits[0].Argument)
		if !ok {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("decimal64 type at %s has invalid fraction-digits %q", fractionDigits[0].Location(), fractionDigits[0].Argument)
		}
		if v < 1 || v > 18 {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("decimal64 type at %s has fraction-digits %d outside 1..18", fractionDigits[0].Location(), v)
		}
		frac, _ := NewFractionDigits(uint8(v))
		fd := frac.Value()
		rs, err := restrictionRanges(st, "range", base, fd)
		if err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		ti.resolved = ResolvedDecimal64{fractionDigits: frac, Range: rs}
	case BaseTypeEmpty:
		ti.resolved = ResolvedEmpty{}
	case BaseTypeBinary:
		lengths, err := restrictionRanges(st, "length", base, 0)
		if err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		ti.resolved = ResolvedBinary{Length: lengths}
	case BaseTypeEnumeration:
		if len(direct(st, "enum")) == 0 {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("enumeration type at %s must define at least one enum", st.Location())
		}
		values, err := m.enumValues(st, "enum", "value")
		if err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		ti.resolved = ResolvedEnumeration{def: EnumDef{values: values}}
	case BaseTypeBits:
		if len(direct(st, "bit")) == 0 {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("bits type at %s must define at least one bit", st.Location())
		}
		values, err := m.enumValues(st, "bit", "position")
		if err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		ti.resolved = ResolvedBits{def: BitsDef{values: values}}
	case BaseTypeIdentityRef:
		var bases []Identity
		baseStmts := direct(st, "base")
		if len(baseStmts) == 0 {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("identityref type at %s must define at least one base", st.Location())
		}
		seenBases := make(map[string]*yangparse.Statement, len(baseStmts))
		for _, b := range baseStmts {
			if prev := seenBases[b.Argument]; prev != nil {
				return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("identityref type has duplicate base %q at %s; previous base at %s", b.Argument, b.Location(), prev.Location())
			}
			seenBases[b.Argument] = b
			id := m.identityForQNameFrom(b.Argument, b)
			if id == nil {
				return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("unknown identity base %q at %s", b.Argument, b.Location())
			}
			bases = append(bases, Identity{id: id})
		}
		if len(baseStmts) > 1 && m.yangVersionForStatement(st) != "1.1" {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("identityref type with multiple base statements requires yang-version 1.1 at %s", st.Location())
		}
		ti.resolved = ResolvedIdentityRef{bases: bases}
	case BaseTypeInstanceIdentifier:
		require, err := requireInstance(st)
		if err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		ti.resolved = ResolvedInstanceIdentifier{RequireInstance: require}
	case BaseTypeLeafRef:
		paths := direct(st, "path")
		if len(paths) != 1 || strings.TrimSpace(paths[0].Argument) == "" {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("leafref type at %s must define exactly one non-empty path", st.Location())
		}
		if !validLeafRefPathArg(paths[0].Argument) {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("invalid leafref path %q at %s", paths[0].Argument, paths[0].Location())
		}
		if err := m.validateLeafRefPathPrefixes(paths[0]); err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		require, err := requireInstance(st)
		if err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		if len(direct(st, "require-instance")) > 0 && m.yangVersionForStatement(st) != "1.1" {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("leafref type require-instance statement requires yang-version 1.1 at %s", st.Location())
		}
		ti.resolved = ResolvedLeafRef{path: paths[0].Argument, requireInstance: require, sourceModule: m, sourceStmt: paths[0]}
	case BaseTypeUnion:
		var members []TypeInfo
		memberTypes := direct(st, "type")
		if len(memberTypes) == 0 {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("union type at %s must define at least one member type", st.Location())
		}
		for _, mt := range memberTypes {
			member, err := m.parseTypeSeen(mt, seen)
			if err != nil {
				return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
			}
			if m.unionMemberRequiresYang11(st, member) {
				return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, fmt.Errorf("union member type %q requires yang-version 1.1 at %s", member.Base().String(), mt.Location())
			}
			members = append(members, member)
		}
		ti.resolved = ResolvedUnion{members: members}
	default:
		ti.resolved = ResolvedUnknown{}
	}
	return ti, nil
}

func validateTypeRestrictionPlacement(st *yangparse.Statement, base BaseType) error {
	for _, child := range st.SubStatements() {
		if hasPrefix(child.Keyword) {
			continue
		}
		if !isKnownTypeRestrictionKeyword(child.Keyword) {
			continue
		}
		if typeRestrictionAllowedForBase(child.Keyword, base) {
			continue
		}
		return fmt.Errorf("%s is not valid for type %s at %s", child.Keyword, base.String(), child.Location())
	}
	return nil
}

func isKnownTypeRestrictionKeyword(keyword string) bool {
	switch keyword {
	case "base", "bit", "enum", "fraction-digits", "length", "path", "pattern", "range", "require-instance", "type":
		return true
	default:
		return false
	}
}

func (m *moduleData) lookupTypedefModuleFrom(qname string, from *yangparse.Statement) (*moduleData, *yangparse.Statement) {
	mod := m.resolveSourceQNameModuleFrom(qname, from)
	if mod == nil {
		return nil, nil
	}
	local := localName(qname)
	if mod != m {
		return mod, mod.typedefs[local]
	}
	if def := m.lookupScopedTypedef(local, from); def != nil {
		return m, def
	}
	def := m.typedefs[local]
	if !m.definitionVisibleFrom(def, from) {
		return m, nil
	}
	return m, def
}

func (m *moduleData) lookupScopedTypedef(name string, from *yangparse.Statement) *yangparse.Statement {
	for cur := from; cur != nil; cur = m.statementParents[cur] {
		if defs := m.typedefsByScope[cur]; defs != nil {
			if def := defs[name]; def != nil {
				return def
			}
		}
	}
	return nil
}

func (m *moduleData) typedefDefaultFrom(qname string, from *yangparse.Statement) (string, bool) {
	def, ok := m.typedefDefaultEntryFrom(qname, from)
	if !ok {
		return "", false
	}
	return def.value, true
}

func (m *moduleData) typedefDefaultEntryFrom(qname string, from *yangparse.Statement) (DefaultValue, bool) {
	return m.typedefDefaultEntryFromSeen(qname, from, make(map[*yangparse.Statement]bool))
}

func (m *moduleData) typedefDefaultEntryFromSeen(qname string, from *yangparse.Statement, seen map[*yangparse.Statement]bool) (DefaultValue, bool) {
	tdMod, td := m.lookupTypedefModuleFrom(qname, from)
	if td == nil {
		return DefaultValue{}, false
	}
	if seen[td] {
		return DefaultValue{}, false
	}
	seen[td] = true
	defer delete(seen, td)
	if d := first(td, "default"); d != nil {
		return DefaultValue{value: d.Argument, sourceModule: tdMod, origin: DefaultOriginTypedef}, true
	}
	if typ := first(td, "type"); typ != nil {
		return tdMod.typedefDefaultEntryFromSeen(typ.Argument, typ, seen)
	}
	return DefaultValue{}, false
}

// absolutePathStartsUnprefixed reports whether the first step of an absolute
// path has no module prefix.
func absolutePathStartsUnprefixed(path string) bool {
	parts := splitPath(path)
	return len(parts) > 0 && !hasPrefix(pathStepQName(parts[0]))
}

func resolveLeafRef(n *schemaNodeData, source *moduleData, lr *ResolvedLeafRef) {
	resolveLeafRefWithSeen(n, source, lr, nil)
}

func resolveLeafRefWithSeen(n *schemaNodeData, source *moduleData, lr *ResolvedLeafRef, seen map[*schemaNodeData]bool) {
	if n == nil || lr == nil || lr.path == "" {
		return
	}
	if source == nil {
		source = n.module
	}
	var target *schemaNodeData
	if strings.HasPrefix(lr.path, "/") {
		// RFC 7950 section 6.4.1: a name without a prefix belongs to the
		// module of the current node, which is where a grouping is used or a
		// typedef referenced, not the module that wrote the path. Prefixes
		// still resolve through the writing module's imports.
		if current := n.module; current != nil && current != source && absolutePathStartsUnprefixed(lr.path) {
			target = findLeafrefDataPath(n, source, current, lr.path, lr.sourceStmt)
		}
		if target == nil {
			_, target = source.ctx.findNodeBySourceSchemaPathFrom(source, lr.path, lr.sourceStmt)
		}
	} else {
		target = findRelativeSchemaPathWithSeen(n, source, lr.path, lr.sourceStmt, seen)
	}
	if target == nil {
		// A leafref path is a data path: choice and case nodes are not steps.
		target = findLeafrefDataPath(n, source, source, lr.path, lr.sourceStmt)
	}
	if target == nil || target.typeInfo == nil {
		return
	}
	lr.target = new(SchemaNodeRef{node: target})
	// Underlying types are copied after every target has resolved, so a
	// forward reference cannot capture an unresolved intermediate leafref.
	lr.realtype = nil
}

// findLeafrefDataPath resolves a leafref path over the data tree, where ".."
// moves to the nearest data-node ancestor and each named step matches a data
// child, looking through choice and case nodes. It does not follow deref().
// Prefixes resolve in source; an absolute path whose first step has no prefix
// starts at unprefixedRoot's top level.
func findLeafrefDataPath(start *schemaNodeData, source, unprefixedRoot *moduleData, path string, fromStmt *yangparse.Statement) *schemaNodeData {
	if start == nil || source == nil || source.ctx == nil || unprefixedRoot == nil {
		return nil
	}
	parts := splitPath(path)
	cur := start
	if strings.HasPrefix(path, "/") {
		if len(parts) == 0 {
			return nil
		}
		first := pathStepQName(parts[0])
		root := unprefixedRoot
		if hasPrefix(first) {
			root = source.resolveSourceQNameModuleFrom(first, fromStmt)
		}
		if root == nil {
			return nil
		}
		cur = root.root
	}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		switch part {
		case "", ".":
			continue
		case "..":
			cur = dataParentNode(cur)
			if cur == nil {
				return nil
			}
			continue
		}
		if _, ok := derefArgument(part); ok {
			return nil
		}
		qname := pathStepQName(part)
		var wantModule *moduleData
		if hasPrefix(qname) {
			if wantModule = source.resolveSourceQNameModuleFrom(qname, fromStmt); wantModule == nil {
				return nil
			}
		}
		cur = dataChildNode(cur, localName(qname), wantModule)
		if cur == nil {
			return nil
		}
	}
	return cur
}

// validateLeafRefPredicates checks the predicates of lr, a leafref of n whose
// path m declares (RFC 7950 §9.9.2 and the path-predicate rule of §14): each
// follows a list with keys and equates one of its keys, at most once, with a
// leaf reached from current() by a path that starts with "..". The path is
// walked over the data tree like findLeafrefDataPath; a path that uses
// deref() or does not resolve step by step is left to target resolution.
func (m *moduleData) validateLeafRefPredicates(n *schemaNodeData, lr *ResolvedLeafRef) error {
	if n == nil || lr == nil || m == nil {
		return nil
	}
	parts := splitPath(lr.path)
	cur := n
	if strings.HasPrefix(lr.path, "/") {
		cur = nil
	}
	for _, part := range parts {
		switch part {
		case "", ".":
			continue
		case "..":
			if cur = dataParentNode(cur); cur == nil {
				return nil
			}
			continue
		}
		if _, ok := derefArgument(part); ok {
			return nil
		}
		qname := pathStepQName(part)
		if cur = m.leafrefDataChild(cur, qname, lr.sourceStmt); cur == nil {
			return nil
		}
		i := strings.IndexByte(part, '[')
		if i < 0 {
			continue
		}
		predicates, ok := splitLeafRefPredicates(part[i:])
		if !ok {
			return fmt.Errorf("leafref %q path %q has invalid predicate %q", n.name, lr.path, part[i:])
		}
		seen := make(map[*schemaNodeData]bool, len(predicates))
		for _, predicate := range predicates {
			if reason := m.leafRefPredicateProblem(n, cur, predicate, lr.sourceStmt, seen); reason != "" {
				return fmt.Errorf("leafref %q path %q has invalid predicate %q: %s", n.name, lr.path, predicate, reason)
			}
		}
	}
	return nil
}

// leafrefDataChild returns the data child of parent named by qname, resolved
// from the source statement from. From the data tree root, which is a nil or
// module parent, a prefixed qname looks among the top-level nodes of the
// module it names and an unprefixed one among those of parent, or of m.
func (m *moduleData) leafrefDataChild(parent *schemaNodeData, qname string, from *yangparse.Statement) *schemaNodeData {
	var module *moduleData
	if hasPrefix(qname) {
		if module = m.resolveSourceQNameModuleFrom(qname, from); module == nil {
			return nil
		}
	}
	switch {
	case (parent == nil || parent.kind == SchemaNodeKindModule) && module != nil:
		parent = module.root
	case parent == nil:
		parent = m.root
	}
	return dataChildNode(parent, localName(qname), module)
}

// splitLeafRefPredicates splits the predicates that follow a leafref path
// step, each with its brackets.
func splitLeafRefPredicates(text string) ([]string, bool) {
	var out []string
	for text = strings.TrimSpace(text); text != ""; text = strings.TrimSpace(text) {
		if text[0] != '[' {
			return nil, false
		}
		end := -1
		var quote byte
		for i := 1; i < len(text) && end < 0; i++ {
			switch {
			case quote != 0:
				if text[i] == quote {
					quote = 0
				}
			case text[i] == '\'' || text[i] == '"':
				quote = text[i]
			case text[i] == '[':
				return nil, false
			case text[i] == ']':
				end = i
			}
		}
		if end < 0 {
			return nil, false
		}
		out = append(out, text[:end+1])
		text = text[end+1:]
	}
	return out, true
}

// leafRefPredicateProblem describes what is wrong with predicate, which
// follows list in a leafref path of n, or returns "" when it is valid. seen
// holds the keys earlier predicates of the step constrained.
func (m *moduleData) leafRefPredicateProblem(n, list *schemaNodeData, predicate string, from *yangparse.Statement, seen map[*schemaNodeData]bool) string {
	if list.kind != SchemaNodeKindList {
		return fmt.Sprintf("predicate on %s %q, which is not a list", nodeStatementKeyword(list), list.name)
	}
	if len(list.keys) == 0 {
		return fmt.Sprintf("predicate on list %q, which has no keys", list.name)
	}
	left, right, ok := strings.Cut(predicate[1:len(predicate)-1], "=")
	left, right = strings.TrimSpace(left), strings.TrimSpace(right)
	if !ok || !validYangIdentifierRef(left, true) {
		return "want key = current()/.. path"
	}
	var module *moduleData
	if hasPrefix(left) {
		module = m.resolveSourceQNameModuleFrom(left, from)
	}
	var key *schemaNodeData
	for _, candidate := range list.keys {
		if candidate.name == localName(left) && (module == nil || candidate.module == module) {
			key = candidate
			break
		}
	}
	switch {
	case key == nil:
		return fmt.Sprintf("%q is not a key of list %q", left, list.name)
	case seen[key]:
		return fmt.Sprintf("duplicate key %q", left)
	}
	seen[key] = true
	steps, ok := leafRefPathKeyExprSteps(right)
	if !ok {
		return fmt.Sprintf("right-hand side %q must be a current()/.. path", right)
	}
	cur := n
	for _, step := range steps {
		if step == ".." {
			cur = dataParentNode(cur)
		} else {
			cur = m.leafrefDataChild(cur, step, from)
		}
		if cur == nil {
			break
		}
	}
	if cur == nil || cur.kind != SchemaNodeKindLeaf {
		return fmt.Sprintf("right-hand side %q does not resolve to a leaf", right)
	}
	return ""
}

// leafRefPathKeyExprSteps returns the steps after current() of a
// path-key-expr (RFC 7950 §14): one or more ".." steps followed by one or
// more node identifiers.
func leafRefPathKeyExprSteps(expr string) ([]string, bool) {
	rest, ok := strings.CutPrefix(expr, "current")
	if !ok {
		return nil, false
	}
	rest, ok = strings.CutPrefix(strings.TrimSpace(rest), "(")
	if !ok {
		return nil, false
	}
	rest, ok = strings.CutPrefix(strings.TrimSpace(rest), ")")
	if !ok {
		return nil, false
	}
	rest, ok = strings.CutPrefix(strings.TrimSpace(rest), "/")
	if !ok {
		return nil, false
	}
	steps := strings.Split(rest, "/")
	parents := 0
	for i, step := range steps {
		step = strings.TrimSpace(step)
		steps[i] = step
		switch {
		case step == ".." && parents == i:
			parents++
		case !validYangIdentifierRef(step, true):
			return nil, false
		}
	}
	if parents == 0 || parents == len(steps) {
		return nil, false
	}
	return steps, true
}

func dataParentNode(n *schemaNodeData) *schemaNodeData {
	for p := n.parent; p != nil; p = p.parent {
		if p.kind != SchemaNodeKindChoice && p.kind != SchemaNodeKindCase {
			return p
		}
	}
	return nil
}

func dataChildNode(parent *schemaNodeData, name string, module *moduleData) *schemaNodeData {
	for _, child := range parent.children {
		if child.kind == SchemaNodeKindChoice || child.kind == SchemaNodeKindCase {
			if found := dataChildNode(child, name, module); found != nil {
				return found
			}
			continue
		}
		if child.name == name && (module == nil || child.module == module) {
			return child
		}
	}
	return nil
}

func (m *moduleData) applyTypeRestrictions(r ResolvedType, st *yangparse.Statement, base BaseType) (ResolvedType, error) {
	switch v := r.(type) {
	case ResolvedInt:
		rs, err := derivedRestrictionRanges(st, "range", base, 0, v.Range)
		if err != nil {
			return nil, err
		}
		if len(rs) > 0 {
			if err := validateDerivedRangeSubset(st, "range", base, v.Range, rs); err != nil {
				return nil, err
			}
			v.Range = rs
		}
		return v, nil
	case ResolvedDecimal64:
		rs, err := derivedRestrictionRanges(st, "range", base, v.fractionDigits.Value(), v.Range)
		if err != nil {
			return nil, err
		}
		if len(rs) > 0 {
			if err := validateDerivedRangeSubset(st, "range", base, v.Range, rs); err != nil {
				return nil, err
			}
			v.Range = rs
		}
		return v, nil
	case ResolvedString:
		rs, err := derivedRestrictionRanges(st, "length", base, 0, v.Length)
		if err != nil {
			return nil, err
		}
		if len(rs) > 0 {
			if err := validateDerivedRangeSubset(st, "length", base, v.Length, rs); err != nil {
				return nil, err
			}
			v.Length = rs
		}
		ps, err := patterns(st)
		if err != nil {
			return nil, err
		}
		if len(ps) > 0 {
			v.Patterns = append(v.Patterns, ps...)
		}
		return v, nil
	case ResolvedBinary:
		rs, err := derivedRestrictionRanges(st, "length", base, 0, v.Length)
		if err != nil {
			return nil, err
		}
		if len(rs) > 0 {
			if err := validateDerivedRangeSubset(st, "length", base, v.Length, rs); err != nil {
				return nil, err
			}
			v.Length = rs
		}
		return v, nil
	case ResolvedEnumeration:
		values, err := m.restrictedEnumBitValues(v.def.values, st, "enum", "value")
		if err != nil {
			return nil, err
		}
		if len(values) > 0 {
			v.def = EnumDef{values: values}
		}
		return v, nil
	case ResolvedBits:
		values, err := m.restrictedEnumBitValues(v.def.values, st, "bit", "position")
		if err != nil {
			return nil, err
		}
		if len(values) > 0 {
			v.def = BitsDef{values: values}
		}
		return v, nil
	case ResolvedIdentityRef:
		bases, err := m.restrictedIdentityBases(v.Bases(), st)
		if err != nil {
			return nil, err
		}
		if len(bases) > 0 {
			v.bases = bases
		}
		return v, nil
	case ResolvedInstanceIdentifier:
		if value, ok, err := requireInstanceOverride(st); err != nil {
			return nil, err
		} else if ok {
			v.RequireInstance = value
		}
		return v, nil
	case ResolvedLeafRef:
		if value, ok, err := requireInstanceOverride(st); err != nil {
			return nil, err
		} else if ok {
			v.requireInstance = value
		}
		return v, nil
	default:
		return r, nil
	}
}
