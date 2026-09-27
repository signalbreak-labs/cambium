// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package codegen

import (
	"sort"
	"strconv"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

// runtimeIdentifiers are the exported package-level identifiers the generator
// emits from its fixed helper templates. They are always reserved, whether or
// not a given schema pulls the helper in, so adding an unrelated node to a
// schema never renames an existing generated type.
var runtimeIdentifiers = []string{
	"AnyData",
	"CambiumStruct",
	"Decimal64",
	"FromJSONIETF",
	"InstanceIdentifier",
	"MetadataAnnotation",
	"NewAnyData",
	"NewDecimal64",
	"NewInstanceIdentifier",
	"NewInstanceIdentifierWithXMLNS",
	"NewMetadataAnnotation",
	"NewUserOrderedVec",
	"UserOrderedVec",
	"ValidationError",
	"WithDefaultsAll",
	"WithDefaultsAllTagged",
	"WithDefaultsExplicit",
	"WithDefaultsMode",
	"WithDefaultsTrim",
}

// nameKey identifies one generated named entity: a schema node plus the role
// of the generated declaration (its struct, its scalar type, an operation
// document, ...).
type nameKey struct {
	node cambium.SchemaNodeRef
	role string
}

// identAllocator hands out the package-level Go identifiers of one generated
// file. Every generated declaration is reserved as a family (the type plus the
// identifiers derived from it, such as its FieldOrder manifest or enum
// constants), so no two families can overlap. A family keeps its natural name
// unless any member is already taken, in which case the smallest numeric
// suffix that frees the whole family is appended. Names are memoized per
// entity, so repeated lookups while rendering always agree.
type identAllocator struct {
	used  map[string]bool
	names map[nameKey]string
}

func newIdentAllocator() *identAllocator {
	a := &identAllocator{used: make(map[string]bool), names: make(map[nameKey]string)}
	for _, name := range runtimeIdentifiers {
		a.used[name] = true
	}
	return a
}

// allocate returns the name for key, reserving base (or base plus the first
// free numeric suffix) together with every identifier family derives from it.
func (a *identAllocator) allocate(key nameKey, base string, family func(name string) []string) string {
	if name, ok := a.names[key]; ok {
		return name
	}
	name := base
	for suffix := 2; ; suffix++ {
		members := family(name)
		free := true
		for _, member := range members {
			if a.used[member] {
				free = false
				break
			}
		}
		if free {
			for _, member := range members {
				a.used[member] = true
			}
			break
		}
		name = base + strconv.Itoa(suffix)
	}
	a.names[key] = name
	return name
}

func rootFamily(name string) []string {
	return []string{name, name + "FieldOrder", name + "ModuleNS", name + "ModuleName"}
}

func structFamily(name string) []string {
	return []string{name, name + "FieldOrder"}
}

func operationFamily(name string) []string {
	return []string{name, name + "FieldOrder", "From" + name + "JSONIETF"}
}

func restrictedScalarFamily(name string) []string {
	return []string{name, "New" + name, "Default" + name}
}

func bitsFamily(name string) []string {
	return []string{name, name + "BitPositions", "New" + name}
}

func enumFamily(values []cambium.EnumValue) func(string) []string {
	return func(name string) []string {
		out := []string{name, "Parse" + name}
		used := make(map[string]bool, len(values))
		for _, ev := range values {
			out = append(out, name+safeVariantIdent(ev.Name(), used))
		}
		return out
	}
}

func identityrefFamily(members []identityrefMember) func(string) []string {
	return func(name string) []string {
		out := []string{name, "Parse" + name}
		for _, m := range members {
			out = append(out, name+m.variant)
		}
		return out
	}
}

// unionFamily reserves a union type, its variant types, and every payload
// helper type derived from them (see unionPayloadType), recursively.
func unionFamily(members []cambium.TypeInfo) func(string) []string {
	return func(name string) []string {
		out := []string{name}
		if len(members) == 0 {
			return append(out, name+"String")
		}
		used := make(map[string]bool, len(members))
		for _, member := range members {
			variant := safeVariantIdent(unionMemberLabel(member), used)
			out = append(out, name+variant)
			out = append(out, unionPayloadFamily(name, variant, member)...)
		}
		return out
	}
}

func unionPayloadFamily(unionName, variant string, member cambium.TypeInfo) []string {
	payload := unionPayloadTypeName(unionName, variant, member)
	switch r := member.Resolved().(type) {
	case cambium.ResolvedLeafRef:
		if realtype, ok := r.Realtype(); ok {
			return unionPayloadFamily(unionName, variant, *realtype)
		}
	case cambium.ResolvedInt:
		if len(r.Range) > 0 {
			return restrictedScalarFamily(payload)
		}
	case cambium.ResolvedString:
		if len(r.Length) > 0 {
			return restrictedScalarFamily(payload)
		}
	case cambium.ResolvedEnumeration:
		return enumFamily(r.Values())(payload)
	case cambium.ResolvedBits:
		return bitsFamily(payload)
	case cambium.ResolvedIdentityRef:
		if len(r.Bases()) > 0 {
			return identityrefFamily(collectIdentityrefMembers(r.Bases()))(payload)
		}
	case cambium.ResolvedUnion:
		return unionFamily(r.Members())(payload)
	}
	return nil
}

// isOwnModuleChild reports whether child belongs to the module that owns the
// generated struct it becomes a field of: the module of its nearest data
// ancestor (choice/case are transparent), or the generated module for
// document top-level nodes.
func (g *goEmitter) isOwnModuleChild(child cambium.SchemaNodeRef) bool {
	owner := g.moduleName
	for parent, ok := child.Parent(); ok; parent, ok = parent.Parent() {
		kind := parent.Kind()
		if kind == cambium.SchemaNodeKindChoice || kind == cambium.SchemaNodeKindCase {
			continue
		}
		if kind != cambium.SchemaNodeKindModule {
			owner = parent.Module().Name()
		}
		break
	}
	return child.Module().Name() == owner
}

// allocationOrder returns the indexes of children in the order their
// identifiers are allocated: the owning module's children first, in schema
// order, then children contributed by other modules ordered by module name
// (then schema order). Field order is untouched; this only decides which of two
// colliding names keeps the unsuffixed form, so the choice never depends on
// the order modules were loaded (which orders augments from different modules).
func (g *goEmitter) allocationOrder(children []cambium.SchemaNodeRef) []int {
	order := make([]int, len(children))
	own := make([]bool, len(children))
	for i, child := range children {
		order[i] = i
		own[i] = g.isOwnModuleChild(child)
	}
	sort.SliceStable(order, func(x, y int) bool {
		a, b := order[x], order[y]
		if own[a] != own[b] {
			return own[a]
		}
		if own[a] {
			return false
		}
		return children[a].Module().Name() < children[b].Module().Name()
	})
	return order
}

// allocateNames reserves every generated identifier up front, in a canonical
// walk of the document (root, then each struct's children in allocation order,
// depth first) followed by the RPC/notification documents. Because names are
// memoized, rendering, Plan, and the validators' repeated collectFields calls
// all observe the same assignment regardless of the order they ask in.
func (g *goEmitter) allocateNames() {
	g.modulePascal = g.names.allocate(nameKey{role: "root"}, toPascalCase(g.moduleName), rootFamily)
	g.allocateRecordNames(g.collectFields(g.modulePascal, g.documentTopLevelChildren()))
	g.visitOperationDocuments(func(op, source cambium.SchemaNodeRef, suffix, _ string) {
		name := g.operationDocumentName(op, suffix)
		g.allocateRecordNames(g.collectFields(name, g.operationSourcePayloadChildren(op, source)))
	})
}

func (g *goEmitter) allocateRecordNames(fields []fieldInfo) {
	nodes := make([]cambium.SchemaNodeRef, len(fields))
	for i, f := range fields {
		nodes[i] = f.node
	}
	for _, i := range g.allocationOrder(nodes) {
		f := fields[i]
		if isStructKind(f.node.Kind()) {
			name := fieldConcreteType(f)
			g.allocateRecordNames(g.collectFields(name, g.recordChildren(f.node)))
		}
	}
}

func (g *goEmitter) operationDocumentName(op cambium.SchemaNodeRef, suffix string) string {
	return g.names.allocate(nameKey{node: op, role: "document " + suffix}, g.modulePascal+toPascalCase(op.Name())+suffix, operationFamily)
}
