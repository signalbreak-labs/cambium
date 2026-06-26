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
func (t TypeInfo) TypedefChain() []string { return append([]string(nil), t.typedefChain...) }

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
		resolveLeafRef(n, source, &r)
		m.validateResolvedLeafRef(n, &r)
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
		appendDerivedToIdentityAncestors(base, id, make(map[*identityData]bool))
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
		base, err := tdMod.parseTypeSeen(typ, seen)
		if err != nil {
			return TypeInfo{base: BaseTypeUnknown, resolved: ResolvedUnknown{}}, err
		}
		typedefName := localName(name)
		base.typedefName = ptr(typedefName)
		base.typedefChain = append([]string{typedefName}, base.typedefChain...)
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
		return DefaultValue{value: d.Argument, sourceModule: tdMod}, true
	}
	if typ := first(td, "type"); typ != nil {
		return tdMod.typedefDefaultEntryFromSeen(typ.Argument, typ, seen)
	}
	return DefaultValue{}, false
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
		_, target = source.ctx.findNodeBySourceSchemaPathFrom(source, lr.path, lr.sourceStmt)
	} else {
		target = findRelativeSchemaPathWithSeen(n, source, lr.path, lr.sourceStmt, seen)
	}
	if target == nil || target.typeInfo == nil {
		return
	}
	ref := SchemaNodeRef{node: target}
	lr.target = &ref
	rt := cloneTypeInfo(*target.typeInfo)
	lr.realtype = &rt
}

func (m *moduleData) applyTypeRestrictions(r ResolvedType, st *yangparse.Statement, base BaseType) (ResolvedType, error) {
	switch v := r.(type) {
	case ResolvedInt:
		rs, err := restrictionRanges(st, "range", base, 0)
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
		rs, err := restrictionRanges(st, "range", base, v.fractionDigits.Value())
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
		rs, err := restrictionRanges(st, "length", base, 0)
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
		rs, err := restrictionRanges(st, "length", base, 0)
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
		values, err := m.restrictedEnumBitValues(v.Values(), st, "enum", "value")
		if err != nil {
			return nil, err
		}
		if len(values) > 0 {
			v.def = EnumDef{values: values}
		}
		return v, nil
	case ResolvedBits:
		values, err := m.restrictedEnumBitValues(v.Values(), st, "bit", "position")
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
